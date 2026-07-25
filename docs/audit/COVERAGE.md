# Coverage ledger

The living record of which surfaces have been audited, by what method, and when —
the input the `hardening-audit` workflow reads to pick the *least-audited* surfaces
next. Keep it honest: `status` reflects real coverage, and blind spots are listed,
not hidden. See `PROTOCOL.md`.

`status`: **recent** = covered by a strong method in the last pass or two ·
**stale** = audited a while ago or only weakly/indirectly · **never** = no real audit.

| Surface | Package | Methods used | Last pass | Status |
|---|---|---|---|---|
| iCalendar decode/ingest | internal/model | fuzz, heal-on-ingest | 4 | recent |
| Recurrence expansion (read) | internal/model | fuzz, tz/DST sweep, scale-bound | 4,5,8,21,23 | recent (**Pass 23 MED, UNFIXED — the pass-21 aggregate `StepBudget` is minted per *store call*, not per redraw**, and `dayItemsForDays`/`selRange` call it once per day (7-day rebuild 1.85 s, 30-day SELECT 7.8 s, 366-day ≈ 1 m 40 s on the UI thread). Pass 23 also found the *downstream* layout is unbounded and quadratic (`LayoutDay`, HIGH) — the expansion bound buys nothing if the layout of the bounded output hangs. Repros `internal/ui/redraw_budget_repro_test.go`, `internal/model/layoutday_scale_repro_test.go`; detail in the feature-promise and day-agenda/time-grid rows. **Pass 21 re-fuzzed the read-side `Occurrences`/`safeBetween` expansion against the pass-14 multivalue-RDATE/EXDATE + pass-17 IANA-TZID `VALUE=PERIOD` inputs (never re-fuzzed on the read side since pass 8) — 1 MED CONFIRMED, UNFIXED (repro-verified RED, LEFT IN TREE).** The scale guardrail bounds a *single* event's skip-forward at `maxOccurrenceSteps` (1<<20 ≈ 107 ms wall for a far-anchored `FREQ=SECONDLY` rule), but nothing caps the **aggregate** across a resource's events or across the store: `Event.Occurrences` loops every event in a resource and `store.EventOccurrencesVisible` loops that over every resource of every visible calendar, invoked synchronously from `ui/render.go` `calItems` on each grid rebuild/navigation. Measured: 50 far-anchored (`DTSTART:19000101T000000Z`) `FREQ=SECONDLY` VEVENTs in one resource → `EventOccurrences` over a one-month window took **5.07 s and returned 0 occurrences** (the events are also invisible — the freeze buys nothing); ~600 such events across the cache would freeze each redraw ~60 s. LazyPlanner is offline-first and ingests foreign/hostile `.ics` from any client/server, so this input is attacker-influenceable. The scale guardrail frames the invariant as "a pathological rule can't hang the UI" (singular) — the per-event bound holds, the per-resource/per-store aggregate does not. Repro `internal/model/aggcap_repro_test.go` (`TestAggregateRecurrenceCapRepro`, asserts ≤500 ms; observed 5.07 s) — **currently RED and left in the tree, so it breaks `make check`/`go test ./...` until an aggregate cap lands (owner may gate/delete it pending the fix)**. Fix direction: an aggregate step/deadline budget shared across the render-path expansion of the whole store (a `context` deadline or a summed step ceiling), not just the per-event cap. **FIXED (571b7ec)**: added `model.StepBudget` — a shared raw-step ceiling (`maxAggregateOccurrenceSteps`, 2<<20) drawn down across a batch of expansions. `safeBetween` now takes an explicit `maxSteps` and reports steps used; each event steps at most `min(remaining, per-event cap)`, and an exhausted budget degrades the remaining events to their base instance (the same graceful fallback as a bad rule). `Parsed.EventOccurrences` uses a fresh budget per call and `EventOccurrencesBudgeted` shares one across a whole redraw via `store.EventOccurrencesVisible`. The ceiling sits above the per-event cap so a lone pathological event never starves legit siblings. The left-in-tree repro was promoted to a permanent guard `internal/model/aggregate_cap_test.go` (bound is by budget not N — 50 vs 250 far-anchored SECONDLY events stay ~constant ~210 ms; plus a legit-weekly-event-not-starved case); `make check` is green again.) |
| Recurrence write-side (mutate/split/advance) | internal/model | fuzz, deep audit, spec-diff, input-edge | 9,14,19,20,21 | recent (**Pass 21 edge-swept the pass-20-merged shared `reanchoredRecurrence(raw, oldAnchor, newAnchor)` core (event + todo now delegate to one component-level function) across the 1st/2nd/3rd/4th/last/5th × weekly/monthly × event/todo matrix — no new finding.** The merged core's day-pinning-`BY*` consistency (re-anchor a weekly weekday set as a whole; re-derive a monthly nth-weekday from the new anchor; block a 5th-but-not-last / out-of-vocabulary rule `(nil,true)`; pass a daily/plain-weekly/monthly-by-day/yearly rule through unchanged) holds identically for the DUE-anchored todo twin as for the DTSTART-anchored event, confirming the pass-20 refactor did not regress the pass-19-fixed "last-weekday"/"positive-nth" behavior. **Pass 19 spec-diff of the v1.3.0 rewrite primitives — 1 HIGH + 1 MED CONFIRMED, both UNFIXED (repro-verified red this synthesis).** HIGH: `ReanchoredRecurrence` (recur_edit.go:51) blindly keeps `MonthlyNth==-1` for a monthly "last <weekday>" rule and only swaps the weekday, so a backward day-move onto a weekday whose true *last* occurrence is later in the month emits `BYDAY=-1<newWd>` that fires on a different day than the moved DTSTART — the exact vanishing-instance bug the function exists to prevent (repro `internal/model/reanchor_lastweekday_repro_test.go`, DTSTART 2024-04-24 last-Wed → -1 day → BYDAY=-1TU fires Apr 30, DTSTART Apr 23 outside its own set). MED: the positive-nth re-derivation `(newStart.Day()-1)/7+1` (recur_edit.go:52) can compute 5, emitting `BYDAY=5<wd>` outside the editable 1st–4th/last vocabulary (main.md:424) — RecurSpecFromRule rejects it so the rule becomes uneditable "Custom (kept)" and the series thins to months with a 5th weekday. Fix direction for both: block the move `(nil,true)` when the re-derived nth isn't in {1,2,3,4,-1} OR doesn't actually match newStart. The existing reanchor_test only covers the safe 1st-Mon→1st-Tue case. Prior: 2 MED fixed pass 14: this-&-future split added a phantom trailing occurrence past a pre-split EXDATE — now counts RRULE iterations via rruleIterationsBefore — and duplicated a trailing RDATE across both halves — now partitioned via filterRDates; both restore main.md:362. Codified as a Hard-won guardrail. **Both pass-19 findings FIXED same commit (8051ddc)**: the monthly nth-weekday branch now re-derives the position from newStart's own month every move — a "last of month" position always writes `BYDAY=-1<wd>` regardless of the original rule's nth, a 1st–4th position writes `BYDAY=n<wd>`, and a re-derived 5th-but-not-last position blocks the move `(nil,true)` instead of emitting an out-of-vocabulary rule. Regression `internal/model/reanchor_lastweekday_repro_test.go` (HIGH) + `reanchor_fifthweekday_repro_test.go` (MED, re-added 228dbbc after the auditor's synthesis-only repro was removed)) |
| Subtask tree build | internal/model | fuzz, scale | 4,5,17 | recent (pass 17 re-fuzzed BuildTree cycle/orphan/deep-chain classification against 12 passes of model evolution — no finding; pass-17 canary CLOSED: the this-&-future split COUNT-clamp boundary is now guarded by TestSplitAtSeriesEndKeepsFutureBounded) |
| Quick-add parser | internal/model | fuzz, input-edge | 4,14,19 | recent (**Pass 19 fuzzed the v1.2.0 grammar additions — 1 LOW CONFIRMED, UNFIXED (repro-verified red).** `parseEveryRecur`'s "every <month> <day>" branch (quickadd.go:641) validates only `1<=day<=31` with no validYMD/rollForwardMonthDay gate, unlike the sibling plain-date path (quickadd.go:584-590) fixed in pass 14. So "every feb 30 dentist" consumes+discards the impossible date (Title="dentist", HasDate=false) and produces a yearly RecurSpec whose Month/Day are dead data — FreqYearly ROption emits no BY*, so the series silently anchors to the caller's context day, not Feb 30, with the typed text lost. Affects feb 29/30, apr/jun/sep/nov 31. Fix: gate the month-day branch on validYMD like parseDate. Canary ESCAPE same surface: `parsePriority` `!9` upper-bound narrowed to `<=8` shipped undetected — quick-add priority tests only exercise `!high`/`!1`, the numeric 1–9 edge is unguarded. Prior: MED fixed pass 14: an invalid day-of-month in the slashed `2/30` and month-name `feb 30` forms was silently normalized by time.Date to a wrong date and rolled a year forward — validYMD/rollForwardMonthDay now reject it, matching the ISO form and the leave-text-in-title principle. **Pass-19 LOW FIXED (acc0c6b)**: the "every &lt;month&gt; &lt;day&gt;" branch now gates on the same validYMD check as the plain-date path, so "every feb 30 dentist" is rejected and left in the title instead of being consumed into a dead-anchored yearly RecurSpec. Canary escape CLOSED (29ca392): `parsePriority`'s numeric 1–9 boundary is now pinned by a table test verified to fail under the `n<=9`→`n<=8` mutation) |
| Timezone / DST | internal/model | exhaustive sweep, fuzz | 8,14,17 | recent (MED fixed pass 14: resolveDateTime parsed only a single date-time, so an RFC-5545-valid comma-listed multi-valued RDATE/EXDATE — or VALUE=PERIOD RDATE — errored and Occurrences collapsed the whole RRULE series to its base instance; resolveDateTimeValues now splits per value. Codified as a Hard-won guardrail. **Pass 17 MED — FIXED**: an RDATE;VALUE=PERIOD carrying an IANA TZID (e.g. America/New_York) was silently mis-zoned to floating time — resolveDateTimeValues left the VALUE=PERIOD param on the sub-prop so go-ical's prop.DateTime rejected it, and resolveDateTime had only a Windows-name recovery branch + floating fallback. Fixed: resolveDateTimeValues drops the stale VALUE=PERIOD param on the reduced sub-prop (cloning the shared map), and resolveDateTime gained an IANA-TZID time.LoadLocation recovery branch so the IANA and Windows spellings agree. internal/model/tz.go. Regression internal/model/rdate_period_tzid_test.go) |
| Windows→IANA zone mapping + TZID resolution | internal/model | fuzz | 17 | recent (first direct audit of windowszones.go's lookup table + tz.go's unknown-TZID fallback. No crash/panic on adversarial/unresolvable TZIDs — the floating fallback holds. One functional gap found on the same resolveDateTime path: the IANA-TZID VALUE=PERIOD RDATE mis-zone (see Timezone/DST row, MED, tz.go:68) — a Windows-name TZID resolves but the IANA-name equivalent falls through to floating) |
| Color parsing (ParseHexColor / NearestANSI16 / ReadableFg / Luminance) | internal/model | input-edge | 17 | recent (first audit of model/color.go — parses untrusted server-supplied CALENDAR-COLOR / hex strings. Boundary-swept malformed length, non-hex digits, out-of-range channels, empty input to NearestANSI16 — no panic/OOB finding; the parser rejects/clamps as expected. Distinct cell from the caldav color-PROPFIND decode audited pass 12) |
| CalDAV network boundary (response-parse: multiget/PROPFIND/REPORT decode, hand-rolled ListObjectHrefs XML, truncated/oversized bodies, redirects) | internal/caldav | fault-injection, panic-guard, fuzz | 4,7,15,22 | recent (**Pass 22 data-loss-swept the write-method conditional semantics (PutObject/DeleteObject If-Match/If-None-Match, 412 lost-update surfacing) — the caldav side of the standing cross-package resource-is-gone residual (prior caldav coverage was response-parse + request-construction only). No new CONFIRMED finding, but a mutation canary on `normalizeETag` (object.go) ESCAPED (see canary section): deleting the `strings.TrimPrefix(etag, "W/")` weak-validator strip ships undetected — a server ETag returned as a weak validator `W/"abc"` is stored verbatim (the leading `W/` defeats the later quote-strip so `etag[0]!='"'`), which breaks a subsequent If-Match compare against a bare stored ETag and surfaces spurious 412 conflicts. Coverage hole: every ETag-asserting test uses only strong, double-quoted ETags; no test exercises a `W/"…"` response header and a grep for `W/`/`weak`/`normalizeETag` in the test files returns zero. CLOSED (e7f3625) — TestNormalizeETag.** pass 15 HIGH fixed: `PutObject`/`DeleteObject` followed a 301/302/303 on writes — Go's default redirect policy downgrades PUT/DELETE to a bodyless GET dropping the body + If-Match/If-None-Match; a 200/204 on the followed GET landed in the success set, so the write silently vanished and sync cleared the dirty flag. `NewClient` now installs a method-aware `CheckRedirect` returning `http.ErrUseLastResponse` for write methods (`isWriteMethod`), and `PutObject`/`DeleteObject` treat any 3xx as an error; reads and RFC 6764 `.well-known` discovery still follow redirects. Repro `internal/caldav/redirect_test.go`. Canary CLOSED this pass: `ListObjectHrefs` nested-collection filter now guarded by `TestListObjectHrefsExcludesNestedCollection` — see below) |
| Sync engine (data-loss / TOCTOU) | internal/sync, internal/store | deep audit, race | 3,11,18 | recent (HIGH fixed pass 11: PullRemoteBatch skips a Dirty resource via store.ErrKeptLocalEdit. **Pass 18 HIGH — FIXED**: `store.CommitPush` treated `cur==nil` (resource deleted mid-push by the event-loop while the sync goroutine's PUT was in flight) identically to `cur==pushed`, rebuilding the deleted resource clean, and `stageResourceLocked` wiped the tombstone — a user delete landing during an edit-PUT was silently, permanently lost (next sync a no-op). Now `CommitPush` reads `cur` under the lock and, when `cur==nil`, honors the deletion instead of resurrecting: `honorMidPushDeleteLocked` ensures a tombstone carrying the post-PUT href/ETag — advancing an existing one's ETag (pushUpdate/synced case, so the next conditional DELETE's If-Match matches) or creating one (pushCreate/never-synced case, whose local delete left none, else the just-created server copy is re-pulled and resurrected). Repro `internal/store/commitpush_deletemidpush_test.go` (both push variants + a 200× concurrent Delete‖CommitPush invariant, green under -race)) |
| Sync reconcile state machine (reconcileCalendar/reconcileReadOnly case matrix, keep-both, Forget, read-only-twin branches) | internal/sync, internal/store | data-loss, race | 13,14,15,19,20,21,22 | recent (**Pass 22 raced the deeper reconcile matrix — the keep-both conflict staging and the (B) push-vs-pull branch racing a concurrent pull, the branches COVERAGE flagged "warm again but not cleared" — no new CONFIRMED data-loss finding. BUT a mutation canary on `reconcileReadOnly` ESCAPED (see canary section): weakening the read-only dirty-discard guard `if r.Dirty || r.Href == ""` → `&&` ships undetected — a previously-synced item (Dirty, Href≠"") edited after its calendar became read-only is then neither discarded nor pulled over, silently keeping an un-pushable local edit and violating the "read-only calendars never keep local changes" hard invariant. Coverage hole: the only read-only-discard test (`TestSyncReadOnlyDiscardsStuckAndMirrors`) uses a never-synced (Href="", Dirty) resource that satisfies BOTH the original OR and the mutated AND, so it cannot distinguish them; no test puts a synced-then-edited (Dirty, Href≠"") resource on a read-only calendar. CLOSED (e7f3625) — TestReadOnlyDiscardsSyncedThenEditedResource.** **Pass 21 data-loss-swept the READ-ONLY twin — `reconcileReadOnly`'s `!onServer` Forget, its dirty-discard Forget, and `handleWriteForbidden`'s Forget — for the pass-20 step-(A) Forget-clobber class (the textbook "mirror the guard onto the sibling" escape), never swept for this class before — no finding.** After the pass-20 fix, reconcile removals route through `store.ForgetIfUnchanged` (expectedPrev pointer-identity guard); the read-only twin's removals were confirmed to sit behind a *confirmed* server-deletion signal on a read-only collection (no concurrent local UI edit can land — read-only calendars are never written by the UI), so the stale-snapshot lost-update shape that bit the read-write step-(A) does not manifest on the read-only twin. The class stays closed on the reconcile side. **Pass 20 raced the reconcile step-(A)/(B) writes against a concurrent pull (the pass-19 next-target) — 1 HIGH CONFIRMED, UNFIXED (repro-verified red).** `reconcileCalendar`'s `case !onServer:` clean branch (sync.go:411) — a resource deleted on the server — calls `st.Forget(calID, r.Name)` unconditionally on the *stale* post-download snapshot pointer `r`, with no pointer-identity/expectedPrev guard, unlike the sibling PullRemote/PutIfUnchanged/CommitPush paths. A UI edit landing on that name during step-(A)'s in-flight PUT replaces `cs.resources[N]` with a new dirty `*Resource`; the loop still holds the clean snapshot pointer, takes the plain `!onServer` (not `!onServer && r.Dirty`) branch, and Forgets the name — the edit is silently discarded (converted to a tombstone) with NO conflict and NO skip (observed PulledDeletes=1, Conflicts=0, Skipped=0), and the next sync DELETEs it server-side. **Third member of the "concurrent-write signal has no resource-is-gone case" class** (twin of pass-18 CommitPush `cur==nil` HIGH and pass-19 pushDelete-412 HIGH) — this instance is the FORGET path, the one reconcile write the two prior fixes did not cover. Repro `internal/sync/stepa_forget_clobber_repro_test.go`. Fix direction: guard Forget by pointer identity (mirror PullRemote's expectedPrev) and, on mismatch, raise a serverDeleted markConflict against the surviving local edit. **Pass 19 raced the reconcile-vs-concurrent-pull matrix beyond the CommitPush window (the pass-18 next-target) — 1 HIGH CONFIRMED, UNFIXED (repro-verified red).** `pushDelete`'s 412 resurrect (sync.go:687) uses a bare unconditional `store.PutRemote` instead of the compare-and-set (expectedPrev) pattern every other reconcile write uses — it is the ONLY bare unconditional write left in the writable reconcile path. When a conditional DELETE loses to a server edit (412) and the UI re-creates/undoes the resource at the same name during the DELETE round-trip (undoLast→RestoreDirty), the resurrect silently overwrites the restored content with the server version AND stashes only the server version as the conflict — so both keep-local and keep-server yield server content; the user's undone content is unrecoverable, violating "sync never silently overwrites". Delete-conflict twin of the pass-18 CommitPush `cur==nil` HIGH. Repro `internal/sync/tombstone412_undo_race_test.go`. Fix direction: resurrect only when `cs.resources[N]` is still nil (mirror pullInto's expectedPrev guard); add a -race variant to TestConcurrentSyncAndEditsRace exercising Delete+RestoreDirty against a 412 server. **FIXED (d39853d)**: the 412 resurrect now reads the resource under lock and only resurrects when it is still absent (mirrors pullInto's expectedPrev guard); a concurrent RestoreDirty wins and the server version is surfaced as a conflict instead of silently overwriting it. Regression `internal/sync/tombstone412_undo_race_test.go`. This is the second reopening of the "concurrent-write signal has no resource-is-gone case" class (twin of pass-18's CommitPush `cur==nil` HIGH) — the FOUND site is fixed but untested peer reconcile-write paths beyond this window and the pass-18 CommitPush window were not re-swept this pass; see the blind-spot note below. Prior: HIGH fixed pass 13: degraded fetch no longer inferred as deletion. Pass 14 MED fixed: pushDelete's 412 branch cleared the tombstone unconditionally, silently dropping a delete-vs-server-change conflict when the server version was unparseable or absent from a degraded download — it now clears the tombstone only after resurrect+flag, else keeps it and records a skip. Pass 14 LOW fixed: keep-local of a server-deleted conflict never converged — ResolveKeepLocal now clears the Href on a ServerDeleted conflict so reconcile re-creates the item instead of re-raising the conflict. Pass 15 re-swept the keep-both / Forget / read-only-twin data-loss branches — no new finding; the tombstone-vs-server-edit / re-pull-guard paths are exercised by the -race canary below) |
| CalDAV request-construction (MKCALENDAR/PROPPATCH/DELETE bodies, resolve()/href, color/name validation, idempotency) | internal/caldav | fault-injection | 13,21 | recent (**Pass 21 fault-injected the least-covered half of the caldav boundary (all prior caldav coverage is response-parse; request-build touched only pass 13) with hostile server responses to the write-request paths — 1 MED CONFIRMED, UNFIXED (repro-verified RED).** `SetCalendarProps` (proppatch.go:48) returns `nil` on a 207 Multi-Status **without parsing the body**, so a server that accepts the PROPPATCH at the HTTP level (207) but rejects the `displayname`/`calendar-color` property *inside* the 207 (a `<propstat>` carrying `HTTP/1.1 403 Forbidden` or `409`) is treated as a successful push. The caller (`sync.go:225-233` → `MarkCalendarPropsSynced`) then clears `pendingName`/`pendingColor` because the pushed value equals the local value — the user's rename/recolor of a shared/limited-privilege calendar is **silently discarded and never retried**, diverging local from server. Contrast `discoverColors`, which DOES walk propstats. Repro provided in PASS-21.md (`TestSetCalendarPropsRejectedPropertyIsAnError`, ran RED — `SetCalendarProps` returned nil for a 207 whose propstat reported 403; auditor removed it post-run to keep the gate green). Fix direction: parse the 207 body and surface a non-2xx `<propstat>` status for a requested property as an error, mirroring `discoverColors`. **FIXED (ba4428a)**: `SetCalendarProps` now reads the 207 body and scans its propstats via `proppatchRejection`, failing on the first positively-identified non-2xx status (naming the property + status). A plain 200, or a 207 whose body carries no parseable status (a quirky but non-rejecting server), stays lenient — a genuine success is never turned into a false failure (the existing empty-body-207 test still passes). Regressions `internal/caldav/proppatch_test.go` (`TestSetCalendarPropsRejected207` RED-before/GREEN-after + `TestSetCalendarPropsAccepted207` guarding against over-correction). Prior: 2 MED fixed pass 13: DELETE now idempotent on 404/410, MKCALENDAR idempotent on 405 — no more pending-delete/pending-create wedge) |
| Sync concurrency | internal/sync | -race stress | 3,11 | recent (re-run post batching/CTag; no new race; the store-level clobber finding is fixed) |
| CTag incremental short-circuit (skip DownloadAll) | internal/sync | data-loss, fault-injection | 11,16 | recent (pass 16 fault-injected a stale/duplicate/absent/lying server CTag driving the skip decision — no finding; the skip is fail-safe) |
| Background sync goroutines (startPeriodicSync timer + flushOnQuit quit push) | internal/ui, internal/sync | race | 11,16 | recent (pass 16 re-swept under -race against the pass 14–15 write-path changes (CheckRedirect, tombstone-412) — no deadlock/race found) |
| Store filesystem robustness (paths, revert, rollback, load-time stale-temp sweep) | internal/store | deep audit, race, fault-injection | 9,15,22 | recent (**Pass 22 fault-injected the pass-20/21 write primitives (removeLocked/ForgetIfUnchanged compare-and-remove, tombstone create/advance, .ics+sidecar temp/rename) under ENOSPC / rename-fail / partial-write — no new finding; the atomic temp+rename and lock-held compare-and-remove degrade or roll back cleanly under injected disk faults. These primitives had only been race-stressed (pass 20/21), never fault-injected — the last fault-injection here was pass 15, before they existed. A store canary CAUGHT this pass (dropping the tombstone `r.Href != ""` guard → `TestDeleteNeverSyncedLeavesNoTombstone` + `TestCommitPushHonorsDeleteOfNeverSyncedCreate`), so the net on this surface has teeth.** pass 15 HIGH fixed: `loadCalendar`'s stale-temp sweep used the over-loose `isStaleTempName` (`HasPrefix(".") && Contains(".tmp-")`) and ran BEFORE the `.ics` extension filter, so a real resource whose sanitized name began with a dot and contained `.tmp-` (e.g. UID `.tmp-important@host` → `.tmp-important_host.ics`) was `os.Remove`'d on Open — permanent loss for an offline-created not-yet-pushed item, reachable from a hostile server href. `isStaleTempName` now requires the actual leftover shape — dot-prefixed and ENDING in `.tmp-<digits>` (what `os.CreateTemp` produces); a real resource ends in `.ics` so it can't match, and genuine leftovers are still swept (`TestOpenSweepsStaleTempFiles`). Repro `internal/store/staletemp_test.go`. Pass 15 -race stress on the write primitives found no race — see below) |
| Local disk / config input boundaries | internal/config, internal/state, internal/store | deep audit, size caps, fuzz, fault-injection | 9,13,22 | recent (**Pass 22 first fault-injection of the `password_command` external-process exec path (nonzero-exit, missing binary, empty/huge stdout, slow/hanging command) and unreadable/partial config-file reads — no finding: a failing or absent password_command surfaces its error and never hangs or crashes startup, and an unreadable/oversized config degrades to a surfaced error rather than a silent bad parse. Prior config coverage was TOML fuzz + size caps (pass 13); the subprocess and file-read I/O faults were an under-covered method on a real IO boundary.** pass 13 fuzzed TOML parse / first-run round-trip / password_command stdout — no finding; canary hole below: config-read size cap untested) |
| State-file load/parse (widths/hidden-cals/hour-zoom) | internal/state | fuzz (adversarial values), deep audit, size cap | 9,12,17 | recent (pass 17 re-fuzzed adversarial widths/hidden-cals/hour-zoom against the later calendar-id changes — no parse finding; pass-17 canary CLOSED: Load()'s json.Unmarshal error check is now guarded by TestLoadPartialParseThenErrorIsZero — a later-field type mismatch that populates then errors must yield a zero State) |
| Quick-field edits (sp/sd) — Locate→Put write | internal/ui, internal/model | data-loss | 12 | recent (HIGH STATUS-flatten + MED COMPLETED-restamp + MED TOCTOU all fixed) |
| Completion toggle (Space) + recurring-todo advance — Locate→Put | internal/ui | data-loss | 12 | recent (MED TOCTOU fixed: PutIfUnchanged) |
| Session undo stack (pushUndo / prev-snapshot restore replay) | internal/ui, internal/store | data-loss | 12 | recent (HIGH undo-of-synced-delete + MED undo-of-synced-edit fixed: RestoreDirty) |
| Calendar-color + display-name PROPFIND parsing (discoverColors/SyncCalendarName) | internal/caldav | fault-injection | 12 | recent (MED href-key encoding mismatch fixed: hrefKey decodes; also fixed the privileges fail-open + CTag miss) |
| `:calendar` command argument parsing (rename/color/hide/show) | internal/ui | input-edge | 12 | recent (no new finding) |
| Bulk-pull batching / scale | internal/store, internal/sync | benchmarks, data-loss | 5,11 | recent (HIGH fixed: PullRemoteBatch no longer clobbers a href-less pull-orphan's local edit) |
| Grab-mode temporal-manipulation state machine (per-nudge commit, snapshot/2-resource revert) | internal/ui | data-loss, input-edge | 11 | recent (MED+LOW x2 fixed pass 11: cancelGrab surfaces revert errors; PutIfUnchanged version-check; HasDue re-check. **Post-v1.3.0 user report FIXED**: an all-scope day-move (`h`/`l`) shifted DTSTART but left a day-pinning `BY*` (weekly BYDAY / monthly nth-weekday — every v1.3.0 preset carries one) stale, so the series kept firing on the old day, the moved DTSTART fell outside its own rule, and the event vanished from the calendar; `model.ReanchoredRecurrence` now re-anchors the rule to the moved day (weekly sets shift whole, monthly nth-weekday re-derives) or blocks an opaque "kept" rule with a hint. Codified as a Hard-won guardrail. Regression `internal/model/reanchor_test.go`, `internal/ui/grab_recur_reanchor_test.go`) |
| SELECT mode + bulk ops + bulk grab (multi-select range derivation, bulk complete/delete/yank-paste/grab, GRAB nested inside SELECT) | internal/ui | data-loss, input-edge | 19,20,22 | recent (**Pass 22 canary ESCAPE (see canary section): `drillRange`'s bounds guard `idx >= len(items)` weakened to `idx > len(items)` ships undetected — when the drilled cursor index equals `len(items)`, the guard no longer returns nil so `ci = len(items)` and the subsequent slice `items[ai : ci+1]` = `items[ai : len(items)+1]` goes out of range, panicking the single-threaded TUI on a bulk-select over a drilled day at the terminal index. Coverage hole: `drillRange` is never referenced by name in the ui tests and no test drills a day then extends a SELECT range to the terminal index. CLOSED (e7f3625) — TestDrillRangeAtTerminalIndexDoesNotPanic.** **Pass 20 re-swept bulk grab (phase-3 deep audit) + SELECT multi-write bulk ops under `-race` — 2 LOW CONFIRMED, both UNFIXED (repro-verified red); no new `-race` finding on the bulk ops.** LOW (bulk grab): `bulkGrabShift`'s todo branch (bulkgrab.go:181) shifts DUE via `AddDate`+`EditTodo` with NO `model.ReanchoredRecurrence` call, unlike the event branch of single-item grab (grab.go:289) — a recurring todo whose RRULE pins a weekday (`BYDAY` / monthly nth-weekday) is left with DUE contradicting its own `BY*`, so the next `AdvanceRecurringTodo` snaps back to the old day (repro: DUE Mon→Tue but `BYDAY=MO` untouched → next due Mon not Tue). Same omission in single-item grab (grab.go:247) — pre-existing shared behavior, not bulk-specific. Repro `internal/ui/bulkgrab_recur_reanchor_repro_test.go`. LOW (SELECT highlight): the day-range VISUAL highlight `dayInRange` (calendarview.go:245, timegridview.go:575) has NO 366-day cap while the bulk-op materialization `daysRange` DOES (selection.go:270 clamps `to = from.AddDate(0,0,maxSelectDays)`) — extending the cursor past anchor+366 with `f` highlights every day up to the cursor while bulkDelete/bulkComplete/startBulkGrab silently act only on the first 366, so items on the highlighted tail render selected but are never touched. Repro `internal/ui/daysrange_cap_repro_test.go`. Fix: share one clamp between the highlight predicate and daysRange. The SELECT multi-write bulk ops (bulkDelete/bulkComplete, N separate store writes) showed no new TOCTOU under a pull landing mid-loop this pass. **Pass 19: 1 HIGH + 1 LOW CONFIRMED, both UNFIXED (repro-verified red); 1 canary ESCAPE.** HIGH data-loss: `bulkDelete` (bulkops.go:305) and single-item `deleteWholeObject` (edit.go:468) call `store.Delete(loc.CalID, loc.Name)`, removing the WHOLE .ics resource per UID — they never isolate the selected component, so any co-resident top-level VTODO/VEVENT sharing a bundled resource is silently deleted with it (confirm says "Delete 1 item(s)?"); with a server identity the tombstone pushes a permanent server DELETE of the never-selected bystander. Contrast the move path (yankpaste.go:341) which correctly uses `model.RemoveComponent` to rewrite the resource sparing siblings — the delete path skips that step. Repro `internal/ui/coresident_delete_test.go`. Fix: use RemoveComponent when sibling items remain. LOW input-edge: a pending vim count accumulated in SELECT leaks past a swallowed bulk-op key (Space/d/y/Y/m) — globalKeys returns at the `handleSelectKey==nil` branch (app.go:780) BEFORE the count-reset (app.go:798), so after "3y" the next lone j/k moves 3 rows, silently landing on the wrong item. Repro `internal/ui/countleak_repro_test.go`. Canary ESCAPE: `dayInRange` upper-bound flip `!d.After`→`d.Before` (drops the cursor/`to` day from the highlighted SELECT band) shipped undetected — the draw-path highlight helper (calendarview.go:269/timegridview.go:575) has no test though the range *materialization* daysRange/selRange is well covered. Prior build-time finding already fixed during TDD: One build-time finding already fixed during TDD, not left for the next pass: `bulkDeleteRoots`'s ancestor-absorption walk trusted untrusted `RELATED-TO` parent data with no visited guard — a reciprocal parent cycle (hand-edited or foreign `.ics`) would spin the walk forever, freezing the single-threaded UI event loop; the sibling `descendants()` walk already carried this guard, so this was the same missing-guard-that-a-sibling-has shape as pass 17's Import/reconcileCalendar and resolveDateTime findings. Fixed same-day with a `seen` map, repro-first (`internal/ui/bulkops_test.go`). **The flagged moveSubtreeOps bare-Put gap is FIXED (v1.5.0 step 0, 2026-07-24)**: the source-side rewrite now routes through store.PutIfUnchanged against the loop's own Locate'd Prev, failing the move cleanly (caller rollback) when a concurrent pull lands mid-move. Guard: internal/ui/movesubtree_clobber_test.go (one-pull-per-iteration race under -race; no masking window). The !remaining branch's whole-resource Delete is deliberately unchanged — its version-check question belongs to the reconcile-matrix audit (phase 3) — and so does the same-class rollback path: the shared ops/rollback `Restore` (yankpaste.go, also used by reparentOps) reverts unconditionally, so a pull landing after a root's successful PutIfUnchanged but before a later root's failure triggers rollback would be clobbered by that restore; sweep rollback-Restore alongside the Delete question. **v1.5.0 phase 2 key×context matrix** (`docs/audit/specdiff/MATRIX.md`) surface-level fixes, not a deep audit: bulk grab's task date-shift axis now matches single-item grab (`bulkgrab.go`, finding #4) and `J`/`K` on a grabbed todo flashes feedback instead of a silent no-op (`grab.go`, finding #6) — both still `never` for a real audit pass, remaining phase-3 targets. **Pass-19 findings FIXED**: HIGH — `bulkDelete`/`deleteWholeObject` now isolate the selected component via `model.RemoveComponent`, rewriting the resource instead of deleting it whole, when co-resident siblings remain (40b0803); regression `internal/ui/coresident_delete_test.go`. LOW — the pending vim count is now reset on the SELECT bulk-op-swallow path in `globalKeys` (33d01d3, regression guard added c441b32 after being left untracked); regression `internal/ui/countleak_repro_test.go`. Canary escape CLOSED (fb7d8e2): `dayInRange`'s inclusive upper bound is now pinned by table tests verified to fail under the exclusive-flip mutation.) |
| Recurrence-edit UI orchestration (scope picker + this-&-future split/detach) | internal/ui | data-loss | 11 | recent (HIGH x2 + MED fixed: commitSplit/commitDetach rollback; DetachTodoOccurrence preserves props) |
| v1.3.0 recurrence Custom repeat sub-form (recurcustom.go: count/until/unit/monthly-by/ends field validation) | internal/ui, internal/model | input-edge | 22 | recent (**Pass 22 — first dedicated input-edge of the v1.3.0 Custom form's field validation (prior coverage was selection-legibility + specific-bug regressions only; recurrence-edit UI was last audited pre-v1.3.0, pass 11 data-loss) — 1 MED CONFIRMED, UNFIXED (repro-verified RED).** "Ends on date" silently drops the selected end date's occurrence for TIMED items: `readCustomRecur` (recurcustom.go:247) parses the end field via `parseDateField` (edit.go:1179) to midnight-local and stores it verbatim as `spec.Until` (ROption, quickadd.go:91-93), but `applyRecurrence` only rewrites UNTIL to an inclusive `VALUE=DATE` for all-day anchors (`dateOnlyUntil`, recur_edit.go:353); for a TIMED anchor UNTIL stays at 00:00, so every occurrence on the user-selected end date (which carries a nonzero time-of-day) falls after UNTIL and is excluded. A timed daily event starting 2026-07-20 15:00 with "Ends 2026-07-25" yields FREQ=DAILY;UNTIL=20260725T000000Z → occurrences only through 07-24 (the 07-25 15:00 instance the user explicitly asked for is dropped, no error/warning), while the same "Ends 2026-07-25" on an all-day item is inclusive — timed and all-day items disagree on what "Ends on date D" means. Repro (written, ran RED, removed post-run to keep the gate green): `TestReproEndsOnDateDropsTimedOccurrence` in internal/model — a timed daily series expanded to [07-20..07-24], 07-25 absent. Fix direction: for a timed anchor make UNTIL inclusive of the whole selected day (end-of-day, or the anchor's clock time on that day), mirroring `dateOnlyUntil` so both item types agree. **FIXED (c6f79f0)**: `readCustomRecur` now anchors UNTIL at the selected date + the anchor's own wall-clock time-of-day (built in `a.loc`) — a timed series includes its end-day occurrence, and an all-day (midnight) anchor still yields a midnight UNTIL that `dateOnlyUntil` truncates unchanged (one formula, no all-day flag). Fixed in the UI, not the model: `spec.Until` is also populated by RRULE decomposition of foreign rules, where a model-layer bump would risk an iron-rule rewrite. Regressions `internal/ui/ends_on_date_test.go` (`TestEndsOnDateIncludesTimedOccurrence` + `TestEndsOnDateAllDayUnchanged`).) |
| UI draw paths (custom widgets) | internal/ui | display stress | 6,14 | recent (pass 14 re-swept the newer widgets — agendaboard/itemforms — no crash/freeze; no finding) |
| UI input handlers (keys/chords/commands) | internal/ui | deep audit, input-edge | 9,13,14 | recent (pass 14 input-edged the raw non-command keypress/chord dispatch (keys.go navigation, grab activation, mode/drill transitions at boundary states) — no finding. **v1.5.0 phase 2 key×context matrix** (`docs/audit/specdiff/MATRIX.md`, 529 verified cells) landed 6 code fixes here — not a deep-audit pass, but relevant context for one: `q` now closes the account/color pickers (command.go, colorpicker.go, finding #7), undo (`u`) preserves calendar drill state (edit.go, finding #5), and a `j`/`k`→shared `motionArrow`/`modalMotionKey` DRY refactor removed duplicate key-mapping across the Conflicts list and account picker) |
| CLI wiring | cmd/lazyplanner | deep audit, input-edge | 9,16 | recent (pass 16 input-edged the flag.FlagSet subcommand dispatch — 1 MED + 1 LOW, both fixed: a shared `parseFlags` helper returns flag.ErrHelp unchanged and tags other parse errors `errFlagParsed`; `report()` maps ErrHelp→exit 0 and errFlagParsed→exit 2 without re-printing, so `-h`/`--help` succeeds cleanly and a bad flag prints once. Canary CLOSED — new `conn_test.go` `TestConnFlagsClientRequiresAllCredentials` guards the credential path) |
| Mouse handling | internal/ui | input-edge | 10,16,18 | recent (pass 16 LOW, fixed: a MouseLeftDoubleClick ran editSelected() *before* tview processed the event, so it edited the row left by the preceding single click. `treeNodeAtY` now re-targets the current node to the row under the cursor (public tview APIs only) before editing. Pass-18 canary CLOSED: `treeNodeAtY`'s `idx>=len(visible)` upper-bound guard is now pinned by `TestTreeNodeAtYPastLastNode` (a click one row past the last node must not index `visible[len]`/panic the TUI). v1.5.0 gap-closer A closed the board-hit-testing follow-up: `agendaBoard.itemAtY` maps a screen row to its item, `mouseCapture` wires a single left click to `agendaList.SetCurrentItem` (mode-guarded to `modeAgenda`) and a double-click to the same re-target before `editSelected`, guarded by `internal/ui/agendaclick_test.go`. mouse.go) |
| `:config` reload / $EDITOR flow | internal/ui, internal/config | fault-injection | 10,16 | recent (MED fixed pass 10: $EDITOR shell-split. Pass 16 MED, fixed: editConfigFn's reload path read `cfg, _, _, err := config.Load()`, discarding Load's warning — appearance-typo / world-readable-password warnings were silently lost on `:config` reload. Now combined with buildSyncFn's via `joinWarnings`. Canary CLOSED — `TestServerConfigured` asserts a partial (URL-only/username-only) config returns false) |
| Store write pipeline atomicity (.ics + sidecar temp/rename) under disk fault | internal/store | fault-injection | 10,15 | recent (MED fixed pass 10: content-hash reconcile; delete-half left to safe re-pull. Pass 15 re-swept the sidecar/delete-half under injected partial-write/ENOSPC/rename-fail — the accepted delete-half residual still degrades to a safe re-pull and no new partial-write gap opened; no finding) |
| Store write primitives under concurrent goroutines (mutate / PutIfUnchanged / RestoreDirty / tombstone racing PullRemoteBatch) | internal/store | race | 15,20,21 | recent (**Pass 21 race-stressed the brand-new pass-20 `ForgetIfUnchanged`/`removeLocked` compare-and-remove primitives (added AFTER the pass-20 store `-race` sweep, so never race-stressed) against a concurrent pull/edit — no race or lost-update finding.** The lock-held `removeLocked` core makes the pointer-identity check and the map removal atomic, so a concurrent `stageResourceLocked` swap either wins the whole compare-and-remove or loses it cleanly (Forget skipped, edit survives) — no interleave leaves a half-removed entry or a torn tombstone; the guard behaves as the reconcile-side fix relies on. **Pass 20 data-loss-swept the store peer write paths — conflict-resolution `PutRemote` (ResolveKeepServer, conflict.go:177) and the other bare unconditional store writes, the pass-19-flagged un-swept cross-package blind spot — no store-layer finding.** The conflict-resolution `PutRemote` writes are reached only from an explicit, synchronous user conflict-resolution action, not from a background pull racing them, so the "concurrent-delete-signal / bare-write clobber" shape does not manifest at the store layer; the racing-pull data-loss lives one layer up in the sync reconcile loop — see the new pass-20 HIGH (step-(A) Forget) in the Sync-reconcile row. first direct -race stress at the store layer, previously only exercised via the sync engine; no race/deadlock found. Canary CAUGHT: dropping the `r.Href != ""` guard on the tombstone write fails `TestDeleteNeverSyncedLeavesNoTombstone`) |
| Import ingest path (foreign/bundled external .ics via DownloadAll batch + per-resource GetObject fallback, ImportError collection) | internal/sync, cmd/lazyplanner | fuzz, fault-injection | 15,17 | recent (**pass 17 MED — FIXED**: the Import object loop (import.go) wrote every downloaded object with no empty-href guard, unlike its sibling reconcileCalendar (errEmptyHref). A malformed/hostile server returning empty <href/> elements yields caldav.Objects with Path=="" → resourceFileName("")=="resource.ics" → multiple such objects collided on that one name in PullRemoteBatch and silently overwrote each other, each counted as a successful pull; Import reported N imported while storing 1. Fixed: the import loop now skips obj.Path=="" and records it in res.Skipped with errEmptyHref, mirroring reconcileCalendar. Regression internal/sync/import_emptyhref_test.go. pass 15 MED — ACCEPTED RESIDUAL by owner decision 2026-07-18: a single resource mixing a UID-bearing component with a UID-less one fails to encode as a whole — the ingest healers deliberately never fabricate a UID (pass-3 #7), so go-ical's encoder rejects the ENTIRE resource ("want exactly one UID property, got 0") and import records the whole resource in `res.Skipped`, dropping a perfectly valid UID-bearing sibling. Surfaced (not silent), item-level loss, and reachable only from a malformed foreign/hand-edited `.ics` (RFC 5545 requires a UID). Not fixed because every fix crosses a hard invariant — fabricating a UID reverses a settled decision (churn), per-component encode weakens the iron rule (drops the UID-less component), and the CalDAV transport hands us an already-decoded `*ical.Calendar` so there are no raw bytes to preserve. Revisit if it ever bites a real server. See accepted gaps below) |
| Yank/paste cross-list move & copy rollback | internal/ui | data-loss | 10,19,20 | recent (**Pass 20 ran the flagged `moveSubtreeOps` dest-Put orphan-retry residual to ground — 1 MED CONFIRMED, UNFIXED (repro-verified red).** `moveSubtreeOps` (yankpaste.go) Locates each subtree member's real calendar (`loc.CalID`, line 323) but hard-codes the caller-supplied `srcCal` for every source-side write/delete/restore (lines 362/369/375) and treats the destination Put (line 344) as a guaranteed fresh create. Both premises break for a subtree that spans collections via a cross-collection `RELATED-TO` link (parent in list A, child in list B, both server-synced — a legitimate real-world state another client can create; `descendants()` links them globally). Pasting the parent into the child's OWN list makes: (a) the dest Put clobber the child's existing clean resource (bare unconditional Put, no version check); (b) the source Delete target the wrong calendar (`srcCal`) and error; (c) rollback's newest-first Forget then delete the child's REAL resource by name — the child vanishes from the local cache with no tombstone (permanent loss if it carried unsynced edits; self-heals only on next sync if clean). Repro `internal/ui/movesubtree_crosscoll_repro_test.go`. Fix: use `loc.CalID` (not `srcCal`) for source-side ops, and treat the dest Put as an existing-resource rewrite (PutIfUnchanged) when Locate finds a resource already at that name. This is the residual explicitly flagged OPEN by pass 19; now confirmed with a running repro. **Pass 19 swept the shared ops/rollback Restore + reparent paths (the COVERAGE-flagged phase-3 clobber gap) — 1 HIGH + 1 MED CONFIRMED, both UNFIXED (repro-verified red).** HIGH: undo of a co-resident multi-root move permanently loses a root (yankpaste.go:367) — when two top-level VTODOs co-reside in one bundle, `moveSubtreeOps` appends two source-side Restore ops for the SAME resource with intermediate prev snapshots (full, then r1-removed), and `undoLast` (edit.go:707) replays them in append order so the r1-removed snapshot clobbers the full restore — r1 gone from every list, no tombstone/conflict. Repro `internal/ui/undo_multiroot_clobber_test.go`. Fix: coalesce same-resource undo ops so the most-complete snapshot wins (reverse-apply or dedupe by (calID,name)). MED: single-item same-list `reparentTo` (yankpaste.go:142) still uses a bare Locate→Put — the one path the pass-13 exhaustive sweep and the v1.5.0-step-0 moveSubtreeOps fix missed — silently clobbering a concurrent sync pull; sibling `reparentOps` already uses PutIfUnchanged. Repro `internal/ui/reparent_clobber_test.go`. Fix: route reparentTo through `store.PutIfUnchanged(src.Prev)`, abort on applied==false. Prior: HIGH+MED fixed pass 10: per-component isolate/remove. **Both pass-19 findings FIXED**: HIGH — `undoLast` now coalesces same-resource undo ops so the most-complete (earliest) snapshot wins instead of the append-order last write clobbering it (9de7ecc); regression `internal/ui/undo_multiroot_clobber_test.go`. MED — `reparentTo` now commits via `store.PutIfUnchanged` (18fbca3); regression `internal/ui/reparent_clobber_test.go`. This third reopening of the bare-Put clobber class also triggered a full `internal/ui` sweep (aab75dd `grab.go`, e2861bb `recur_edit.go`, a814bd1/08f3870 annotating the remaining fresh-create sites) and a CLAUDE.md guardrail update (c50a42d) — see the Hard-won guardrails entry and the blind-spot note below on unswept peer packages) |
| Feature-promise conformance vs main.md/CLAUDE.md | (whole app) | spec-diff | 10,13,17,18,20,23 | recent (**Pass 23 re-ran the spec-diff over the v1.5.0 phase-2/3 work and the five behaviour-changing pass-21/22 fixes (last diffed pass 20; explicitly skipped in 21 and 22) — 1 HIGH + 2 MED CONFIRMED, all UNFIXED (repro-verified RED). Every one of the three is a *promise the recent fixes quietly failed to deliver* — the method earned its place.** HIGH (**FOURTH reopening of the heal-set class**): the pass-21 fix completed `allowedChildren` for the containers go-ical *knows about*, but `stripForbiddenChildren` (decode.go:234) prunes children **only when the container name has an `allowedChildren` entry** (`if allowed, ok := allowedChildren[comp.Name]; ok`), while go-ical's `encodeComponent` (vendor encoder.go:220/244) recurses into **every** child at every depth regardless of name. So a phantom component nested under a container go-ical has no `checkComponent` case for — `X-*`, `VAVAILABILITY`, RFC 9073 `PARTICIPANT`/`VLOCATION`, a nested `VCALENDAR` — is neither stripped nor reached by the top-level-only heals. The resource decodes, Parse surfaces the valid sibling event (so the phantom is invisible in the UI), and every subsequent `Encode()` fails: the item becomes permanently uneditable, uncompletable, un-grabbable and unpushable (`store.writeResource` mutate.go:145; sync push sync.go:609/637). Confirmed for all three shapes: `X-CUSTOM-CONTAINER`→`want exactly one "DTSTAMP" property, got 0`, `VAVAILABILITY`→same for a nested VTODO, nested empty `VCALENDAR`→`calendar is empty`. The pass-21 guardrail's "`allowedChildren` must carry an entry for every container `checkComponent` recurses into" is unsatisfiable as written — `checkComponent` recurses into containers it has no *case* for, so the strip must be **deny-by-default** (an unlisted container's children are stripped) rather than allow-by-default. Repro `internal/model/unknown_container_repro_test.go` (3 sub-cases). MED: the pass-21 aggregate `StepBudget` promise ("one budget shared across every resource, so the whole redraw's recurrence expansion is bounded in aggregate", main.md + the code comment) is **not delivered on the real redraw paths** — `store.EventOccurrencesVisible` mints a fresh `NewStepBudget()` per **call** (store.go:375) and two hot UI paths call it once **per day** in a loop: `dayItemsForDays` (render.go:189-193, every week/day grid rebuild) and `selRange`/`daysRange` (selection.go:281, up to `maxSelectDays`=366). Per-redraw cost is therefore N_days × 2M steps — exactly the N-multiplication the fix claimed to remove. Measured with ~60 far-anchored `FREQ=SECONDLY` events: one budgeted query 274 ms, a 7-day week rebuild **1.85 s** (6.8×), a 30-day SELECT materialization **7.8 s**, extrapolating to ~1 m 40 s for a 366-day range — all on the tview event loop, all returning zero occurrences. `internal/model/aggregate_cap_test.go` only ever exercises a *single* call, so it passes. Repro `internal/ui/redraw_budget_repro_test.go`. MED: the pass-22 "Ends on date includes the end day for timed items" fix (c6f79f0) **never fires on the real UI path** — `readCustomRecur` (recurcustom.go:258) builds UNTIL from `anchor.Hour()/Minute()/Second()`, but both production `anchorFn`s derive the anchor via `parseDateField(startDate)`/`parseDateField(dueDate)` (itemforms.go:241 events, :51 todos), which returns **midnight** and never consults the Start-time/Due-time field. So UNTIL is still stored at 00:00 and a timed daily event ending "on 2026-07-25" still stops at 07-24. The pass-22 regression test (`internal/ui/ends_on_date_test.go`) passes a full 15:00 anchor the real wiring cannot produce, so it is green while the reported bug is live — a textbook test-that-bypasses-the-defect. Nothing downstream rescues it (`RepeatChoices.Resolve` returns the stored spec verbatim, recurfield.go:114-121). Repro `internal/ui/repro_endsondate_uipath_test.go` (event + todo, both RED). **Pass 20 spec-diffed the post-pass-18 feature set (v1.2.0 quick-add / v1.4.0 SELECT / v1.5.0 polish) vs main.md — 1 MED CONFIRMED, UNFIXED (repro-verified red).** main.md:174/395 promises a quick-add recurrence with no explicit date anchors the start/due itself ("daily → the base day", "tasks and events alike"). Events honor it (createEvent always writes a base-day start); TASKS do NOT: `model.applyRecurAnchor` (quickadd.go:380) sets `HasDate` only for the weekday ("every mon") and month-day ("every jul 20") forms — a bare `daily`/`weekly`/`monthly`/`yearly` (and `every day/week/month/year`) leaves `HasDate=false`/`HasTime=false`, so createTask's `if qa.HasDate || qa.HasTime` gate (edit.go:233) omits DUE and NewTodoObject emits a VTODO carrying `RRULE=FREQ=DAILY` but NO DTSTART and NO DUE. Pressing Space to complete it routes to `AdvanceRecurringTodo` which errors `has no DTSTART/DUE to advance` (UI flashes "Complete failed: …") — the task can never be completed and has no due to display, silently dropping the spec-promised base-day anchor; bulkComplete fails identically. The existing `internal/ui/quickrecur_test.go` asserts only the RRULE, never the DUE, so it passes while the created task is broken. Repro `internal/model/bare_recur_todo_repro_test.go`. Fix: `applyRecurAnchor` should anchor bare daily/weekly/monthly/yearly to the base day so `HasDate` is set. **Pass 18 MED — FIXED**: main.md:340 promises a `:config` reload "re-parses the account list (picker/status bar update live)", but the reloaded list was discarded — `ConfigReload` carried only Sync/ColorMode/Warning and `applyConfigReload` never updated `a.accounts`/`a.activeAccount` (set once in Run), so a `:config`-added/renamed `[[account]]` stayed invisible in the picker + status bar and unreachable via `:account` until restart. Fixed: `ConfigReload` gained `Accounts`+`ActiveAccount`, `editConfigFn` returns the refreshed names + the running account's (possibly renamed) name, and `applyConfigReload` adopts them on a successful reload — switching is still `:account`'s teardown-rebuild, the active connection still can't be hot-swapped, and a reload error leaves the live list untouched. Repro `internal/ui/configreload_accounts_test.go`. Rest of the v1.1.0 promise set held under spec-diff (teardown-rebuild GC's the old store — no cross-account pointer leak; cache carry-over via unchanged ID derivation; last-active-by-id survives rename; :config never yanks a live store). Prior: 2 MED fixed pass 13: applyMutation (edit form + recur-scoped saves) and reparentSelected H/L now route through PutIfUnchanged — the "no Locate→Put clobber sites remain" invariant is now true again. Pass 17 re-diffed the passes 14/15/16 promises (method-aware redirect policy, heal-set-mirrors-validateComponent, RDATE/EXDATE multi-value independence): the redirect and reconcile promises hold; the RDATE/EXDATE-independence promise is confirmed intact for comma-listed values; the pass-17 tz.go VALUE=PERIOD-IANA-TZID mis-zone in the same resolveDateTime path (recorded as a finding in the Timezone/DST row) is now FIXED) |
| Multi-account config parse (`[[account]]` schema, `[server]` migration rejection, validateAccounts nameless/dup, ResolveActiveAccount fallback, Account lookup, Account.ID cache-namespacing) | internal/config | fuzz | 18 | recent (first audit of the v1.1.0 multi-account TOML parse added after pass 17. **HIGH — FIXED**: `toml.Decode` is O(depth²) on deeply nested inline tables, so a config well under the `maxConfigBytes` 4 MiB read cap hung `Load()` — and thus startup — for minutes to hours with no UI and no error (re-measured this session: 500→25 ms, 1000→100 ms, 2000→331 ms, 4000→1.1 s, clean quadratic; a sub-cap file extrapolates to >1 h). The read cap bounds *bytes*, not decode CPU. Fixed with a deterministic pre-decode guard `checkNestingDepth` that rejects structural `{}`/`[]` nesting past `maxTOMLNestingDepth` (64) before `toml.Decode`; brackets inside strings/comments are skipped so a real config (max nesting ~2) is never falsely rejected. Repro `internal/config/config_decode_bound_test.go` (a 2 s deadline test on a deep config + a deterministic boundary test). The account-name uniqueness / migration / ID-derivation logic itself held under adversarial input — no parse finding there) |
| Global state file (`global.json` LoadGlobal/SaveGlobal, corrupt/missing→zero, atomic temp+rename, capped read, ActiveAccountID round-trip) | internal/state | fault-injection | 18 | recent (first audit of the v1.1.0 cross-account state file. The corrupt/missing→zero fail-safe and atomic write held under injected faults — no finding; a corrupt/partial/oversized global state degrades to zero and never blocks startup. Canary CLOSED (pass 18): `Save`/`SaveGlobal`'s shared 0o600 file-mode contract is now guarded by `TestSaveFilesAre0600` — see below) |
| Account switch-and-rebuild loop (`runTUILoop` persist-active-id-before-open, previous-account fallback on failed switch-open, fatal initial-open, unknown-target clean quit) | cmd/lazyplanner | fault-injection | 18 | recent (first audit of the v1.1.0 switch state machine. Injected store.Open failures exercise the fallback (reopen previous working account) and second-failure-fatal logic; the documented persist-before-open crash-window residual (sub-ms) is accepted. No finding on the loop itself. Canary CLOSED (pass 18): the `components()` --tasks/--both helper in the sibling `calendar.go` is now guarded by `TestComponents` (plus `slugify`/`joinWarnings`) — see below) |
| `:account` command + picker (switchAccount case-insensitive validation, already-active no-op, unknown flash, requestSwitch/RunResult.SwitchAccount, no-accounts path) | internal/ui | input-edge | 18 | recent (first input-edge of the v1.1.0 command handler; picker draw was stress-covered by TestAccountPickerStress. Adversarial names + switch-while-modal states surfaced no *new* command-handler defect, but the input-edge exposed the `:config`-reload account-list staleness recorded in the feature-promise row above — MED, now FIXED) |
| Full `sync-collection` incremental (token delta) | internal/sync | — (deliberately deferred) | — | never |
| go-ical semantic encoder constraints (DTEND/DUE+DURATION, empty VTIMEZONE, VJOURNAL/VFREEBUSY nesting) | internal/model | fuzz (re-encode round-trip), spec-diff | 10,16,21,23 | recent (**REOPENED A FOURTH TIME — Pass 23 HIGH, UNFIXED**: the pass-21 fix closed the *known* containers, but `stripForbiddenChildren` is allow-by-default (`if allowed, ok := allowedChildren[comp.Name]; ok`) while go-ical's `encodeComponent` recurses into containers it has no `checkComponent` case for — so a phantom under `X-*`/`VAVAILABILITY`/`PARTICIPANT`/a nested `VCALENDAR` still bricks the whole resource. Repro `internal/model/unknown_container_repro_test.go`; full detail in the feature-promise-conformance row. The guardrail needs restating as deny-by-default, not "enumerate every container". **Pass 21 spec-diffed the heal set against the vendored `go-ical` `validateComponent` per the Hard-won guardrail (last verified pass 16, class reopened twice) — 1 HIGH CONFIRMED, UNFIXED (repro-verified RED, THIRD reopening of this class).** The guardrail promises the heal set mirrors go-ical's *full* validateComponent, but go-ical's `checkComponent` (encoder.go recursion) validates **every child at any depth**, whereas the required-prop (DTSTAMP/UID) and mutual-exclusion (DTEND+DURATION / DUE+DURATION / DURATION-without-DTSTART) heals run only on **top-level** `cal.Children`: `ensureDTStamp` (Parse loop, decode.go:89-105) and `healComponentConstraints` (decode.go:284) never walk nested components. `dedupeSingleValued` and `stripForbiddenNesting` DO recurse (so nested *dup* props are healed) — the gap is the required-prop/mutual-exclusion heals. Compounding it, `stripForbiddenNesting` has no `allowedChildren` entry for VALARM/STANDARD/DAYLIGHT, so a VEVENT/VTODO/VJOURNAL/VFREEBUSY illegally nested inside one of those is neither stripped nor healed. A foreign/hand-edited `.ics` with a valid, editable top-level VEVENT whose VALARM child holds a nested VEVENT lacking DTSTAMP decodes fine and surfaces the real event, but the first edit's `Encode()` recurses into the phantom and returns `want exactly one "DTSTAMP" property, got 0` — the **whole resource, incl. valid siblings, is unwritable**. Same brick for a nested VJOURNAL/VFREEBUSY missing DTSTAMP/UID, a nested DTEND+DURATION, or the same nested under STANDARD/DAYLIGHT in an otherwise-usable VTIMEZONE. Empirically confirmed both the VALARM-nested and VTIMEZONE/STANDARD-nested variants produce the DTSTAMP encode failure; the nested-*dup* variant is healed (dedupe recurses). This is the exact decode-but-can't-re-encode HIGH the guardrail says must not reopen — reopened a THIRD time (pass 10 top-level VEVENT/VTODO; pass 16 VJOURNAL/VFREEBUSY + VTIMEZONE-required-props; pass 21 the **nested-component recursion depth**). Repro provided in PASS-21.md (`TestNestedComponentMissingDTStampBricksResource`, ran RED; auditor removed it post-run to keep the gate green — re-add on fix). Fix direction: make `ensureDTStamp`/`healComponentConstraints` walk nested children the way `dedupeComponent`/`sanitizeComponent` already do (heal recursively across ALL components, not just top-level), and either add VALARM to the nesting-strip vocabulary or otherwise remove a component illegally nested under a VALARM. **FIXED (c8eae4f)** — chose the *strip* half of the fix direction (it fully subsumes the recursive-heal half): completed `model.allowedChildren` with the three childless container types (VALARM/STANDARD/DAYLIGHT → empty allow-set), so `stripForbiddenNesting` (which already recurses) removes any component illegally nested there before it reaches go-ical's recursive `checkComponent`. With every container `checkComponent` recurses into now covered, no VEVENT/VTODO/VJOURNAL/VFREEBUSY can survive below the top level, so the top-level-only required-prop/mutual-exclusion heals have nothing nested left to miss (verified across the VALARM-nested-missing-DTSTAMP, STANDARD-nested-missing-DTSTAMP, and VALARM-nested-DTEND+DURATION variants). A legit VALARM's own props are untouched. Regression `internal/model/nested_heal_test.go`. The Hard-won guardrail (CLAUDE.md) now requires the heal set to mirror `checkComponent` at its *recursion depth* and `allowedChildren` to cover every container go-ical recurses into. Prior: 4 HIGH + 1 MED fixed pass 10: ingest healers. Pass 16 re-fuzz found 2 more HIGH of the *same class* — the pass-10 healer set was component-incomplete — both now fixed: (a) a VTIMEZONE missing TZID, or a STANDARD/DAYLIGHT missing DTSTART/TZOFFSETTO/TZOFFSETFROM, bricked the resource at Encode() — `dropEmptyTimezones`→`dropUnusableTimezones` now strips such an unencodable VTIMEZONE on ingest (owner-approved; a referenced TZID degrades to floating time); (b) a VJOURNAL/VFREEBUSY missing DTSTAMP or carrying duplicate single-valued props — `ensureDTStamp` now runs for VJOURNAL/VFREEBUSY in `healComponentConstraints` and `singleValuedProps` gained CompJournal/CompFreeBusy entries. A missing UID on these components is still not healed (fabricating one would churn sync identity — accepted residual). Regression: `malformed_vtimezone_test.go`, `vjournal_encode_test.go`. **The healer table must mirror go-ical's full encoder.go validateComponent — codified as a Hard-won guardrail so this class does not reopen a third time**) |
| RRULE decomposition — `RecurSpecFromRule`/`decodeMonthly`/`decodeYearly`/`nthMatchesAnchor` (foreign rule → editable RecurSpec) | internal/model | fuzz | 23 | recent (**Pass 23 — FIRST audit of this surface (the decode twin of the five-times-audited reanchor write side) — 1 HIGH + 1 MED CONFIRMED, both UNFIXED (repro-verified RED, left in tree).** HIGH: `decodeYearly` (recurdecompose.go:138) validates BYMONTH and BYMONTHDAY *independently* against the anchor, but per RFC 5545 / rrule-go's `buildRRule` a `FREQ=YEARLY` rule carrying BYMONTHDAY with **no BYMONTH** does not default BYMONTH to DTSTART's month — the yearly iterator walks all 12 months and filters by day, so `FREQ=YEARLY;BYMONTHDAY=15` means "the 15th of **every** month". Because the BYMONTHDAY equals the anchor's day it is declared representable and collapses to a bare `RecurSpec{FreqYearly}`: the Detail pane/Repeat dropdown label it "Yearly on Jul 15" while the calendar correctly renders 12 occurrences/year, any re-serialization emits a bare `FREQ=YEARLY` (11 occurrences/year silently lost — 73→7 over six years), and because `ReanchoredRecurrence` falls to `default: return nil,false` a whole-series grab day-move shifts DTSTART leaving BYMONTHDAY stale — **the event vanishes from the month entirely** (0 occurrences in July after a +1-day nudge). This is the documented "never leave DTSTART contradicting its own `BY*`" guardrail class, reached from the *decode* side. Repro `internal/model/yearly_bymonthday_repro_test.go` (RED; 12 occurrences observed, summary lie, `blocked=false`, post-move July count 0). NOTE: the existing `TestRecurSpecFromRuleAnchorConsistent` (recurdecompose_test.go:145) actively pins the wrong belief (`FREQ=YEARLY;BYMONTHDAY=22` asserted representable) — a correct fix must move that case to the rejection table. MED: `RecurSpecFromRule` copies the interval only when `option.Interval > 1` (recurdecompose.go:49), so a **negative** INTERVAL is silently dropped. rrule-go's `StrToROption` parses `INTERVAL=-1` but `NewRRule` rejects it (`interval must be greater than 0`), so the item really expands to a single instance — yet the rule is declared representable, `RecurrenceSummary` renders "Weekly on Mon", and a one-day grab nudge rewrites it to a real unbounded `FREQ=WEEKLY;BYDAY=TU`, discarding the original bytes instead of blocking `(nil,true)`. (`INTERVAL=0` is unaffected — rrule-go clamps 0→1, so decode and expansion agree.) Repro `internal/model/negative_interval_repro_test.go`) |
| Conflict-resolution UI orchestration (`conflicts.go`: captured Conflict snapshot, populate/refresh, keep-local/keep-server wiring) | internal/ui, internal/store | data-loss | 23 | recent (**Pass 23 — FIRST audit at the UI layer (prior coverage was store-side `ResolveKeep*` only, pass 20) — 1 HIGH CONFIRMED, UNFIXED (repro-verified RED).** `store.ResolveKeepLocal` (conflict.go:109-126) — reachable only from `chooseResolution` (ui/conflicts.go:69) — applies its **entire in-memory mutation before** `writeSidecar` and returns the sidecar error **without reverting**: it adopts the server ETag, sets Dirty, clears Conflicted, clears Href on a ServerDeleted conflict, and `delete(cs.conflicts, name)`. Every other store write path reverts on exactly this failure (`writeResourceLocked` via `stageResourceLocked`'s revert closure; `removeLocked` via `revertMutation`). The UI compounds it: ui/conflicts.go:75-78 flashes "Resolve failed: …" and returns **without** `populateConflicts`/refresh, so the user is told nothing happened while the store believes the conflict is resolved in favour of local. Consequence on a sidecar-write fault (ENOSPC/EACCES/read-only mount/path clobber): the stashed server version is gone from memory, the resource is left Dirty with the ETag advanced to the server's, so the next sync's conditional PUT's If-Match **matches** and silently overwrites the server's diverging version (on the ServerDeleted flavour the cleared Href makes the next sync take the CREATE path and upload a duplicate); the stale conflicts row stays on screen and re-selecting it errors "store: no conflict for …". Repro `internal/store/resolvefail_repro_test.go` (two tests: keep-local and the ServerDeleted flavour). Adjacent, same ordering pattern, not asserted: `MarkConflict` (conflict.go:43-56) also commits the stash + Conflicted copy-on-write before `writeSidecar` with no rollback) |
| Sidecar metadata **parse** (`readSidecar`: dirty/href/etag/hash/conflict/tombstone JSON from disk → reconcile inputs) | internal/store | fuzz | 23 | recent (**Pass 23 — FIRST audit of the sidecar READ side (only its write atomicity was ever audited, passes 10/15/22) — 2 HIGH + 1 MED CONFIRMED, all UNFIXED (repro-verified RED).** HIGH (all-or-nothing discard): `readSidecar`'s decode is atomic-or-nothing, so **any** `json.Unmarshal` error — one stray byte, or a single wrong-typed field such as `"dirty":1` — makes `loadCalendar` substitute an empty sidecar (store.go:158-162) and drop **every** Dirty flag, ETag, Href, Hash, conflict stash, tombstone, display_name, sync_token and the cached `read_only` flag, even though encoding/json decoded the rest correctly. The first subsequent mutation rewrites the sidecar, destroying the recoverable original. Reconcile then sees every resource as clean+href-less — a documented "pull orphan to re-pull" — so unsynced local edits are overwritten from the server, and the lost tombstones let deleted items resurrect; the lost read_only flag lets the UI write to a read-only calendar before the first sync of a session. The crash-window heal (`meta.Hash != h ⇒ dirty`) cannot rescue it: on parse failure `sc.Resources` is nil so `meta.Hash == ""` and the heal is skipped. Repro `internal/store/sidecar_corrupt_repro_test.go` (observed: Dirty=false, ETag/Href empty, Tombstones empty, ReadOnly=false, and the rewritten file retains only `ctag` + one `hash`). Fix direction: quarantine the original bytes and fail safe (treat unparsed resources as dirty/unknown) rather than silently declaring them clean. HIGH (path traversal): a tombstone's **name is a raw JSON object key** from the sidecar, carried unvalidated into `store.Tombstone.Name` (tombstone.go:30) and thence to `filepath.Join(s.root, calID, name)` — resource names are safe only because they come from `os.ReadDir`; tombstone keys are not. `validCalendarID` guards the calendar id, nothing guards the resource name. On a delete-vs-server-change **412**, `pushDelete` → `ResurrectTombstone` (sync.go:703) → `writeResourceLocked` → `writeFileAtomic` writes server-supplied iCalendar **outside the cache root**; the confirmed-read-only **403** branch reaches the same place via `pullInto(…, t.Name, …)` (sync.go:595). Repro `internal/sync/tombstone_traversal_repro_test.go` — a tombstone keyed `"../../../../../victim.txt"` caused sync to overwrite a file two levels above the data dir with a server VCALENDAR, and the in-memory index gained a traversal-named resource (which `writeSidecar` then persists back, making the escape sticky across restarts). Fix direction: reject/`SafeName` non-single-element tombstone keys at ingest in `loadCalendar` **and** add a defense-in-depth single-element check in `stageResourceLocked`/`writeResourceLocked`. MED (not byte-lossless): `conflictMeta.ServerData` is a Go **string** persisted through `json.MarshalIndent` (sidecar.go:72/141), and encoding/json rewrites every invalid UTF-8 byte to U+FFFD — so a Latin-1/legacy `.ics` stashed as the server's version is silently corrupted, contradicting the "stashed losslessly" contract stated at sidecar.go:68-70 and conflict.go:23-25. `ResolveKeepServer` then writes the mojibake as the only surviving copy of the server's diverging content, returning nil (no error surfaced). Invisible in-process — the store must be **reopened** to observe it. Repro `internal/store/conflict_bytelossless_repro_test.go` (210→212 bytes, `caf\xe9`→`caf\xef\xbf\xbd`). Fix direction: store the stash as `[]byte` (base64 in JSON) or a side file) |
| `:goto` / `:view` / `:search` command-argument parsing + search navigation (`command.go` cmdGoto/cmdView, `search.go` runSearch/searchNext/matchIndices) | internal/ui | input-edge | 23 | recent (**Pass 23 — FIRST audit (the ledger had `:calendar`/`:account`/`:config` rows but never these three; prior evidence was a single pass-10 canary hole) — 3 LOW CONFIRMED, all UNFIXED (repro-verified RED).** (1) A **whitespace-only** query slips past both empty-query guards: `runSearch` stores `a.searchQuery = q` (search.go:69) *before* its `TrimSpace` blank check, `searchNext` guards only on `== ""` (line 87), and `matchIndices` trims the query (line 220) so `strings.Contains(label, "")` matches **every** row — including synthetic placeholders like "(no calendars)"/"(nothing today)". `/`+Space+Enter gives no feedback yet leaves an "active" search; `n` then jumps the selection and the status bar reads "/   (2/2)" for a search the user never made. Repro `internal/ui/search_blank_repro_test.go`. (2) `searchNext` keeps `a.searchIdx` as a raw **position** into a recomputed match list (search.go:97), so its doc comment's "the cycle survives edits between presses" is false: when a background sync removes matches ahead of the cursor, `(idx+dir+n)%n` lands back on the row already selected — `n` is a visible no-op with a misleading "(2/3)" flash, and the mirror case (an inserted match) skips the new match until the cycle wraps. Repro `internal/ui/search_staleidx_repro_test.go`. Fix: re-derive `searchIdx` from the current selection's position in the fresh match list before applying `dir`. (3) User-typed query/command text is concatenated **unescaped** into the dynamic-color status bar (`statusLeft`/`statusMid` both `SetDynamicColors(true)`, app.go:543-544) at search.go:77/81/94/100 and command.go:73/86/267/279, unlike every other data→status path (render.go, conflicts.go:96, sync.go:24 all use `tview.Escape`). Typing `/[white:black]` renders "no match: " — the query is **entirely invisible** — and the injected style repaints the rest of the widget; `:[red]bogus`, `:goto [red]nonsense`, `:search [red]zzz` all misreport what the user typed at exactly the moment the message matters. No crash, no data effect. Repro `internal/ui/statusbar_tagescape_repro_test.go` (5 sub-cases, all RED)) |
| Day agenda + time-grid layout (`model.DayAgenda`, `model.LayoutDay` overlap/lane packing, `layoutEnd`; `ui.splitOccs`, `timeGridView.drawBlock`) | internal/model, internal/ui | input-edge | 23 | recent (**Pass 23 — FIRST direct audit (prior coverage was two escaped canaries on DayAgenda's half-open day boundaries, passes 14/17) — 1 HIGH + 2 MED CONFIRMED, all UNFIXED (repro-verified RED).** HIGH (quadratic layout defeats the pass-21 recurrence bound): `LayoutDay` (timegrid.go:63-74) linearly scans `laneEnds` for a free lane per occurrence; when a day's occurrences all overlap, `lanes == n` and the pass is Θ(n²) — and `ui/timegridview.go:278` (`navCells`) is a second O(items × placements) scan on the same data, on both the Draw path (timegridview.go:663) and the navigation path. The model bounds *expansion* (`maxOccurrencesPerEvent` 10000, `maxAggregateOccurrenceSteps` 2<<20) explicitly so the UI stays responsive, but **nothing bounds the layout downstream**, so the guaranteed-deliverable occurrence count is enough to hang the app. Measured: 3 resources of `RRULE:FREQ=SECONDLY;COUNT=10000` + `DURATION:P1D` → 30,000 same-day occurrences, expansion 16.6 ms (the guard reports success) but `LayoutDay` 1.14 s; n=20,000 → 0.57 s, n=100,000 → 9.7 s (5× n ⇒ ~18× time). The aggregate step budget permits ~209 such resources (~2.09 M occurrences on one day) — a redraw that never returns. Repro `internal/model/layoutday_scale_repro_test.go`. Fix direction: min-heap on lane end (or a cap on placements per day) so lane assignment is sub-linear. MED (zero-length all-day dropped from the band but not the drill list): `ui.splitOccs` (render.go:239) buckets all-day occurrences with `for d := DayStart(o.Start); d.Before(o.End); …`, which runs **zero** times when `End == Start` — but `model.overlaps`/`OccurrencesOn` (and therefore `DayAgenda`) have an explicit zero-length case, so the same occurrence IS in the month grid and IS in `tg.items`. A `DTSTART;VALUE=DATE:20260723` + `DTEND;VALUE=DATE:20260723` resource (emitted by some exporters, common in hand edits) renders in the month grid but the week/day all-day band is empty (the band is skipped entirely at count 0, timegridview.go:588): drilling cycles onto an **invisible** item the user can edit or delete, and the band's "+N" count under-reports. Verified month=true, week=false, day=false. Repro `internal/ui/zerolen_allday_repro_test.go`. MED (lane bleed past the column and past the pane rect): `LayoutDay` returns `Lanes` = peak concurrency with no width awareness; `drawBlock` (timegridview.go:746) computes `laneW = (colW-1)/lanes`, floors it at 1, then draws at `bx = colX + Lane*laneW` with **no clamp** against `colX+colW` or the primitive's own rect, and the fill loop calls raw `screen.SetContent` with no clipping. Week view in a 60-column pane (colW=7) with 10 concurrent Monday events painted 6 cells of Monday's block colour inside **Tuesday's** column — events read as being on the wrong day; day view with a 20-wide rect and 24 concurrent events wrote 20 cells past the primitive's rect, which in the real layout overwrites the neighbouring pane. Repro `internal/ui/lanebleed_repro_test.go`) |
| Raspberry Pi target (on-device timing / kiosk) | (hardware) | — | — | never |

## Declared blind spots (not covered by any pass)

### Pass 23 — 15 CONFIRMED findings (6 HIGH / 6 MED / 3 LOW), ALL UNFIXED

Every one carries a repro test that was written, **run RED**, and **left in the tree** — so
`go test ./...` / `make check` is currently RED until the fixes land (or the owner gates/deletes the
repros). Re-verified in this synthesis: all 15 repro files exist and all 15 fail.

- **`decodeYearly` accepts `FREQ=YEARLY;BYMONTHDAY=<n>` with no BYMONTH** (pass-23 HIGH) — UNFIXED. The rule
  fires 12×/year, is labelled "Yearly on Jul 15", loses 11 occurrences/year on any re-serialization, and a
  grab day-move makes the event **vanish** (DTSTART moves, BYMONTHDAY stays). Repro
  `internal/model/yearly_bymonthday_repro_test.go`. Blast radius: the existing
  `TestRecurSpecFromRuleAnchorConsistent` pins the wrong belief and must be moved to the rejection table.
- **Negative INTERVAL silently dropped** (pass-23 MED) — UNFIXED. `Interval > 1` gate drops `-1`; rrule-go
  refuses to build the rule (1 real occurrence) but the app calls it "Weekly on Mon" and a day-move rewrites
  it into a real unbounded weekly series. Repro `internal/model/negative_interval_repro_test.go`.
- **A FAILED "Keep local" still resolves the conflict in memory** (pass-23 HIGH) — UNFIXED.
  `ResolveKeepLocal` mutates then writes the sidecar and returns the error **without reverting**; the UI
  shows "Resolve failed" while the store has discarded the server version, advanced the ETag and left the
  resource Dirty — the next sync's If-Match now matches and silently overwrites the server (or duplicates
  the item on the ServerDeleted flavour). Repro `internal/store/resolvefail_repro_test.go`. `MarkConflict`
  has the same unreverted ordering.
- **A sidecar that fails to parse discards ALL sync metadata, then overwrites the corrupt file**
  (pass-23 HIGH) — UNFIXED. One stray byte or one wrong-typed field (`"dirty":1`) empties the whole
  sidecar: unsynced edits are re-pulled over, tombstones resurrect, the cached `read_only` flag is lost,
  and the original is destroyed on the first mutation. Repro
  `internal/store/sidecar_corrupt_repro_test.go`.
- **Tombstone map keys are unvalidated file names → write outside the cache root** (pass-23 HIGH) —
  UNFIXED. Resource names are safe because they come from `ReadDir`; **tombstone keys come straight from
  sidecar JSON** and reach `filepath.Join(root, calID, name)` via `ResurrectTombstone` (412 path) and
  `pullInto` (403 read-only path). A key of `../../../../../victim.txt` made sync overwrite a file two
  levels above the data dir with server content, and the escape persists back into the sidecar. Repro
  `internal/sync/tombstone_traversal_repro_test.go`.
- **Conflict stash is not byte-lossless** (pass-23 MED) — UNFIXED. `ServerData` is a Go string marshalled
  by encoding/json, which rewrites invalid UTF-8 to U+FFFD, contradicting the documented "losslessly"
  contract; `ResolveKeepServer` then writes the mojibake as the only surviving copy. Repro
  `internal/store/conflict_bytelossless_repro_test.go`.
- **`allowedChildren` is allow-by-default — a phantom under an UNKNOWN container still bricks the resource**
  (pass-23 HIGH, **FOURTH reopening** of the heal-set class) — UNFIXED. `X-*` / `VAVAILABILITY` /
  `PARTICIPANT` / nested `VCALENDAR` containers have no `allowedChildren` entry, so their children are never
  stripped, and go-ical's encoder recurses into them anyway. Repro
  `internal/model/unknown_container_repro_test.go`. The guardrail must be restated: **strip
  deny-by-default**, do not try to enumerate every container.
- **Aggregate `StepBudget` is per store call, not per redraw** (pass-23 MED) — UNFIXED. `dayItemsForDays`
  and `selRange` loop the budgeted query once per day (7-day rebuild 1.85 s; 366-day SELECT ≈ 1 m 40 s on
  the UI thread). Repro `internal/ui/redraw_budget_repro_test.go`.
- **The pass-22 timed-UNTIL fix never fires on the real UI path** (pass-23 MED) — UNFIXED. Both production
  `anchorFn`s hand `readCustomRecur` a midnight, date-only anchor, so `anchor.Hour()` is always 0; the
  pass-22 regression test supplies an anchor the wiring cannot produce. Repro
  `internal/ui/repro_endsondate_uipath_test.go`.
- **`LayoutDay` lane packing is O(n²) — the bounded expansion still freezes the UI** (pass-23 HIGH) —
  UNFIXED. 30 k same-day occurrences (well inside the aggregate step budget): expansion 17 ms, layout
  1.14 s; n=100 k → 9.7 s. Runs on every Draw and every keypress (`navCells`). Repro
  `internal/model/layoutday_scale_repro_test.go`.
- **Zero-length all-day event dropped from the week/day band but still in the drill list** (pass-23 MED) —
  UNFIXED. `splitOccs`'s `d.Before(o.End)` loop runs zero times when `End == Start`, while `DayAgenda` has
  an explicit zero-length case — so the item is selectable and deletable but invisible. Repro
  `internal/ui/zerolen_allday_repro_test.go`.
- **Lane bleed: blocks paint into the next day column and past the pane rect** (pass-23 MED) — UNFIXED.
  `drawBlock` floors `laneW` at 1 but never clamps `bx`/`bw` to the column or the primitive's rect. Repro
  `internal/ui/lanebleed_repro_test.go`.
- **Three `:search` / status-bar LOWs** (pass-23) — UNFIXED: a whitespace-only query becomes an "active"
  search that matches every row (`internal/ui/search_blank_repro_test.go`); `searchNext` keeps a positional
  index into a recomputed list so `n` stalls or skips after a sync
  (`internal/ui/search_staleidx_repro_test.go`); user-typed text is interpolated unescaped into the
  dynamic-color status bar, so `/[white:black]` renders an empty error message
  (`internal/ui/statusbar_tagescape_repro_test.go`).
- **Pass-23 canary escape — override AT the this-&-future split point** — OPEN. See the pass-23 canary
  section below.
- **Pass-23 surface-local coverage hole — `CommitPush`'s mid-push-EDIT branch** — OPEN. See the pass-23
  canary section below.
- **Methods NOT exercised in pass 23**: race and fault-injection (both ran in pass 22 on the surfaces where
  they apply). A concurrency or I/O-fault defect introduced since pass 22 would be missed.
- **Not re-swept in pass 23** (carried forward): `internal/caldav` write paths for the bare-write /
  resource-is-gone class; direct iCalendar decode/ingest fuzz (last direct fuzz pass 4); quick-add grammar
  (19); timezone/DST + Windows→IANA (17); colour parsing (17); `BuildTree` (17); the pass-11/12-era UI
  data-loss surfaces (single-item grab, recurrence-edit scope picker, quick-field sp/sd, completion toggle,
  undo stack, bulk-pull batching); the pass-18-era multi-account surfaces; UI display stress (14) — including
  `render.go`'s `calItems`, changed by the pass-21 StepBudget work — and mouse handling (18);
  `internal/ui/colorpicker.go` and `help.go`, which still have no ledger row.
- **"Ends on date" drops the selected end date for TIMED recurring items** (pass-22 MED) — RESOLVED
  (c6f79f0): `readCustomRecur` now anchors UNTIL at the selected date + the anchor's own wall-clock
  time-of-day (built in `a.loc`), so a timed series includes its end-day occurrence and an all-day anchor
  (midnight) still yields a midnight UNTIL that `dateOnlyUntil` truncates unchanged — one formula, no
  all-day flag. Fixed in the UI (not the model) because `spec.Until` is also populated by RRULE
  decomposition of foreign rules, where a model-layer bump would risk an iron-rule rewrite. Regressions
  `internal/ui/ends_on_date_test.go` (`TestEndsOnDateIncludesTimedOccurrence` +
  `TestEndsOnDateAllDayUnchanged`). See the v1.3.0 Custom-sub-form row.
- **Three pass-22 mutation-canary escapes — CLOSED (e7f3625)** (each a boundary test verified to kill its
  exact mutation, RED under mutation / GREEN reverted; test-only, no behavior change):
  - `internal/caldav/object.go` `normalizeETag` — `TestNormalizeETag` pins the `W/` weak-validator rows
    (dropping the `TrimPrefix` would break If-Match / cause spurious 412s).
  - `internal/ui/selection.go` `drillRange` — `TestDrillRangeAtTerminalIndexDoesNotPanic` drives
    `idx == len(items)` with a valid anchor (the `>=`→`>` mutation panics the TUI via an out-of-range slice).
  - `internal/sync/sync.go` `reconcileReadOnly` — `TestReadOnlyDiscardsSyncedThenEditedResource` uses a
    synced-then-edited (Dirty, Href≠"") resource on a read-only calendar (the OR-only case the
    never-synced test can't distinguish from `&&`).
- **Nested-component required-prop heal is top-level-only** (pass-21 HIGH) — RESOLVED (c8eae4f):
  completed `allowedChildren` with VALARM/STANDARD/DAYLIGHT (empty allow-set) so `stripForbiddenNesting`
  removes any illegally-nested component before go-ical's recursive `checkComponent` sees it; no
  VEVENT/VTODO/VJOURNAL/VFREEBUSY survives below top level, so the top-level heals suffice. Regression
  `internal/model/nested_heal_test.go`; guardrail extended to require matching `checkComponent`'s
  recursion depth. Original finding: go-ical's `checkComponent` validates every child recursively, but the
  DTSTAMP/UID required-prop and DTEND/DUE+DURATION mutual-exclusion heals run only on top-level
  `cal.Children` (`ensureDTStamp`, `healComponentConstraints`), and `stripForbiddenNesting` has no
  VALARM/STANDARD/DAYLIGHT vocabulary — so a phantom VEVENT/VJOURNAL/VFREEBUSY nested in a VALARM (or a
  STANDARD/DAYLIGHT) missing DTSTAMP decodes fine but bricks the whole resource (incl. valid siblings)
  on the first edit's `Encode()`. THIRD reopening of the "heal set must mirror the full
  validateComponent" class (pass 10 top-level, pass 16 VJOURNAL/VFREEBUSY + VTIMEZONE-required-props,
  pass 21 nesting depth). Repro `TestNestedComponentMissingDTStampBricksResource` (in PASS-21.md, ran
  RED, removed post-run). Fix: heal recursively across all components + close the VALARM nesting gap.
  See the go-ical-encoder row.
- **Recurrence expansion has no aggregate/render-path budget** (pass-21 MED) — RESOLVED (571b7ec):
  `model.StepBudget` now caps total raw steps (2<<20) across one `EventOccurrences` call and across a
  whole redraw (`EventOccurrencesVisible` shares one budget); exhausted → remaining events degrade to
  their base instance. Left-in-tree repro promoted to the permanent guard
  `internal/model/aggregate_cap_test.go` (bound by budget not N); `make check` green again. Original
  finding: the scale guardrail bounds a
  single event's `safeBetween` skip-forward (1<<20 ≈ 107 ms), but `Event.Occurrences` × the
  per-resource loop × `store.EventOccurrencesVisible`'s per-visible-calendar loop (called on every grid
  redraw from `ui/render.go`) has no summed step/deadline cap — 50 far-anchored `FREQ=SECONDLY` VEVENTs
  froze a one-month expansion for 5.07 s returning 0 occurrences; ~600 across the cache ≈ 60 s per
  redraw. Attacker-influenceable via foreign `.ics`. Repro `internal/model/aggcap_repro_test.go`
  (`TestAggregateRecurrenceCapRepro`, asserts ≤500 ms, observed 5.07 s) — **RED and in the tree, so it
  breaks `make check` until an aggregate cap lands or the owner gates it.** Fix: a store-wide step/
  deadline budget shared across the render-path expansion. See the recurrence-expansion-read row.
- **PROPPATCH 207 treated as success without per-property status** (pass-21 MED) — RESOLVED (ba4428a):
  `SetCalendarProps` now parses the 207 body and fails on any positively-identified non-2xx propstat
  status (lenient on a plain 200 / statusless body so a real success is never turned into a false
  failure). Regressions `internal/caldav/proppatch_test.go` (`TestSetCalendarPropsRejected207` +
  `TestSetCalendarPropsAccepted207`). Original finding: `SetCalendarProps` (proppatch.go:48) returned nil on any 207 Multi-Status
  without parsing the body, so a server that rejects the `displayname`/`calendar-color` property inside
  the 207 (a `<propstat>` 403/409) is treated as pushed; the caller clears the pending flag and the
  user's rename/recolor of a shared/limited-privilege calendar is silently discarded and never retried.
  Repro `TestSetCalendarPropsRejectedPropertyIsAnError` (in PASS-21.md, ran RED, removed post-run). Fix:
  parse the 207 body and error on a non-2xx propstat for a requested property, like `discoverColors`.
  See the caldav-request-construction row.
- **Raspberry Pi on real hardware** — on-device timing, kiosk/autologin, bare-TTY
  color. Needs a physical Pi; the sole known-never surface with product risk.
- **Full `sync-collection` incremental sync** — a deliberate feature deferral, not a
  bug (the CTag short-circuit is in place); audit once implemented.
- **`DayAgenda` inclusive dayStart boundary** (pass-14 canary escape) — RESOLVED (pass 14):
  `TestDayAgendaIncludesTodoDueAtMidnight` now pins a todo due exactly at 00:00, verified to
  fail under the `Before→After` mutation. See the pass-14 canary section.
- **`ListObjectHrefs` nested-collection filter** (pass-15 canary escape) — RESOLVED (pass 15):
  `TestListObjectHrefsExcludesNestedCollection` adds a nested sub-collection href ≠ the query
  path, verified to fail under dropping `|| r.isCollection()`. See the pass-15 canary section.
- **Import empty-href / basename-collision** (pass-17 MED) — RESOLVED (pass 17): the Import object
  loop now mirrors `reconcileCalendar`'s empty-href guard — an `obj.Path==""` object is skipped and
  recorded in `res.Skipped` with `errEmptyHref` instead of collapsing onto the one `resource.ics`
  name. Repro `internal/sync/import_emptyhref_test.go` (`TestImportEmptyHrefNotSilentlyLost`), verified
  to fail before the fix (`res.Objects=2` / stored=1) and asserts the reported count never exceeds
  what is persisted.
- **IANA-TZID `VALUE=PERIOD` RDATE mis-zoned to floating** (pass-17 MED) — RESOLVED (pass 17):
  `resolveDateTimeValues` now drops the stale `VALUE=PERIOD` param on the reduced period-start
  sub-prop (cloning the shared params map first), and `resolveDateTime` gained an IANA-TZID
  `time.LoadLocation` recovery branch parallel to the Windows-name one, so the IANA and Windows
  spellings agree. Repro promoted to `internal/model/rdate_period_tzid_test.go`
  (`TestRDatePeriodTZIDZoned` + `TestResolveDateTimeIANATZIDRecovery`), verified to fail with either
  fix hunk neutered.
- **Four escaped pass-17 canaries** — ALL CLOSED (pass 17): read-only degraded-download guard
  (`reconcileReadOnly`), this-&-future split COUNT-clamp boundary (`NewSeriesFrom` `remaining==0`),
  `DayAgenda` inclusive-upper-bound (dayEnd) twin of the pass-14 lower-bound escape, and
  `state.Load`'s dropped `json.Unmarshal` error check — each now has a boundary test verified to
  fail under its exact mutation. See the pass-17 canary section.
- **SELECT bulk-delete drags co-resident bystanders** (pass-19 HIGH) — RESOLVED (40b0803): `bulkDelete`
  and `deleteWholeObject` now isolate the selected component via `model.RemoveComponent`, rewriting the
  resource instead of deleting it whole, when co-resident siblings remain. Repro
  `internal/ui/coresident_delete_test.go`, verified to fail before the fix.
- **ReanchoredRecurrence "last <weekday>" / positive-nth day-move** (pass-19 HIGH + MED) — RESOLVED
  (8051ddc): the monthly nth-weekday branch re-derives the position from newStart's own month on every
  move — a true "last of month" position always writes `BYDAY=-1<wd>`, a 1st–4th position writes
  `BYDAY=n<wd>`, and a re-derived 5th-but-not-last position blocks the move `(nil,true)` instead of
  emitting an out-of-vocabulary rule. Repros `internal/model/reanchor_lastweekday_repro_test.go` (HIGH),
  `reanchor_fifthweekday_repro_test.go` (MED, re-added 228dbbc).
- **pushDelete 412 resurrect clobbers a concurrent undo/re-create** (pass-19 HIGH) — RESOLVED (d39853d):
  the twin of the pass-18 CommitPush `cur==nil` HIGH; the 412 branch now reads the resource under lock
  and resurrects only when it is still absent (mirrors pullInto's expectedPrev guard), so a concurrent
  RestoreDirty wins and the server version surfaces as a conflict instead of silently overwriting it.
  Repro `internal/sync/tombstone412_undo_race_test.go`. Second reopening of this class — see the
  reopened-class residual note below for the unswept-peer-path caveat.
- **Undo of a co-resident multi-root move loses a root** (pass-19 HIGH) — RESOLVED (9de7ecc): `undoLast`
  coalesces same-resource undo ops so the most-complete (earliest) snapshot wins instead of the
  append-order last write clobbering it. Repro `internal/ui/undo_multiroot_clobber_test.go`.
- **reparentTo bare Put** (pass-19 MED) — RESOLVED (18fbca3): the single-item same-list reparent now
  commits via `store.PutIfUnchanged`. Repro `internal/ui/reparent_clobber_test.go`. Third reopening of
  the bare-Put clobber class — triggered a full `internal/ui` sweep (aab75dd, e2861bb, a814bd1, 08f3870)
  and a CLAUDE.md Hard-won-guardrail update (c50a42d); see the reopened-class residual note below.
- **parseEveryRecur impossible month-day** (pass-19 LOW) — RESOLVED (acc0c6b): the "every &lt;month&gt;
  &lt;day&gt;" branch now gates on validYMD like the plain-date path, rejecting "every feb 30" instead
  of consuming and misanchoring it.
- **SELECT vim-count leak** (pass-19 LOW) — RESOLVED (33d01d3, regression added c441b32): the pending
  count is now reset on the SELECT bulk-op-swallow path in `globalKeys`. Repro
  `internal/ui/countleak_repro_test.go`.
- **Pass-19 canary escapes** — BOTH CLOSED: `dayInRange`'s inclusive upper bound is pinned by table
  tests (fb7d8e2, `internal/ui/selection_test.go`), verified to fail under the exclusive-flip mutation;
  `parsePriority`'s numeric 1–9 boundary is pinned by a table test (29ca392,
  `internal/model/quickadd_test.go`), verified to fail under the `n<=9`→`n<=8` mutation.
- **Reopened-class residual — bare-Put clobber & concurrent-delete-signal classes**: both classes that
  reopened this pass (a third time for bare-Put; a second time for "no resource-is-gone case" in
  reconcile writes) had their *found* sites fixed, and the bare-Put class additionally got a full
  `internal/ui` sweep (grab.go, recur_edit.go, yankpaste.go) plus the CLAUDE.md guardrail strengthening.
  Neither sweep crossed package boundaries: untested peer write paths in `internal/store`, `internal/model`,
  and `internal/caldav` were not audited for the same two shapes this pass — that cross-package sweep is
  a named target for a future pass, not closed by this one.
- **`moveSubtreeOps` dest-Put cross-collection child loss** (pass-20 MED) — CONFIRMED, UNFIXED
  (repro-verified red): the residual pass-19 flagged OPEN was run to ground this pass. `moveSubtreeOps`
  Locates each subtree member's real calendar (`loc.CalID`) but hard-codes the caller's `srcCal` for
  every source-side write/delete/restore and treats the dest Put (yankpaste.go:344) as a guaranteed
  fresh create. A subtree that spans collections via a cross-collection `RELATED-TO` link (parent in
  list A, child in list B, both server-synced) breaks both premises: pasting into the child's own list
  makes the dest Put clobber the child's existing resource, the source Delete target the wrong calendar
  and error, and rollback's Forget delete the child's REAL resource by name — the child vanishes from
  the cache (permanent if it had unsynced edits). Repro
  `internal/ui/movesubtree_crosscoll_repro_test.go`. Fix: use `loc.CalID` (not `srcCal`) for source-side
  ops and treat the dest Put as an existing-resource rewrite when Locate finds one. See the Yank/paste row.
  **FIXED (60f1191)**: each member now uses `loc.CalID` for source ops (dead `srcCal` param dropped); a
  member already in `dstCal` is rewritten in place via `PutIfUnchanged` (no source delete) instead of
  create-then-delete-its-own-resource; added a per-member `calReadOnly` guard so a cross-collection
  descendant in an unchecked third calendar is never written. Regression `internal/ui/movesubtree_crosscoll_test.go`.
- **Sync reconcile step-(A) Forget clobbers a concurrent edit** (pass-20 HIGH) — CONFIRMED, UNFIXED
  (repro-verified red): `reconcileCalendar`'s `case !onServer:` clean branch (sync.go:411) calls
  `st.Forget` unconditionally on the stale post-download snapshot pointer; a UI edit landing on that
  name during step-(A)'s in-flight PUT is silently converted to a tombstone with no conflict/skip
  (PulledDeletes=1, Conflicts=0). Third member of the "concurrent-write signal has no resource-is-gone
  case" class (twin of pass-18 CommitPush `cur==nil` and pass-19 pushDelete-412) — the FORGET path the
  two prior fixes did not cover. Repro `internal/sync/stepa_forget_clobber_repro_test.go`. Fix: guard
  Forget by pointer identity and raise a serverDeleted conflict on mismatch. See the Sync-reconcile row.
  **FIXED (24500e8)**: added `store.ForgetIfUnchanged` (expectedPrev pointer-identity guard mirroring
  PullRemote; `remove` refactored into a lock-held `removeLocked` core for atomic compare-and-remove).
  Step (A) now skips the removal on a snapshot mismatch and raises a `serverDeleted` markConflict against
  the surviving edit. Regression `internal/sync/stepa_forget_clobber_test.go`. The reopened
  resource-is-gone class was codified as a dedicated Hard-won guardrail (CLAUDE.md), covering all three
  reopenings (CommitPush/pushDelete-412/Forget) and requiring `ForgetIfUnchanged` in reconcile removals.
- **Bulk-grab / single-grab recurring-todo DUE not re-anchored** (pass-20 LOW) — CONFIRMED, UNFIXED:
  `bulkGrabShift`'s todo branch (bulkgrab.go:181) and single-item grab (grab.go:247) shift DUE without
  calling `model.ReanchoredRecurrence`, so a recurring todo whose RRULE pins a weekday keeps DUE
  contradicting its own `BY*` and the next advance snaps back to the old day. Repro
  `internal/ui/bulkgrab_recur_reanchor_repro_test.go`. See the SELECT row.
  **FIXED (ab6ed06)**: added `model.ReanchoredRecurrenceTodo` (todo twin of `ReanchoredRecurrence`; both
  delegate to a shared component-level `reanchoredRecurrence` core), called in both grab todo branches
  when the todo is recurring; a *Custom rule (kept)* blocks the day-shift. Regressions
  `internal/ui/grab_todo_reanchor_test.go` (single) + `internal/ui/bulkgrab_recur_reanchor_test.go`
  (bulk). The reanchor guardrail (CLAUDE.md) now states the anchor is `DUE` for a recurring VTODO.
- **SELECT day-range highlight not capped at 366 days** (pass-20 LOW) — CONFIRMED, UNFIXED: the visual
  highlight `dayInRange` (calendarview.go:245, timegridview.go:575) has no cap while daysRange
  materialization clamps to anchor+366, so highlighted days past the cap are never acted on. Repro
  `internal/ui/daysrange_cap_repro_test.go`. Fix: share one clamp. See the SELECT row.
  **FIXED (5cf043d)**: `dayInRange` now applies the identical `maxSelectDays` clamp as `daysRange` (after
  normalizing endpoints to `DayStart`), so highlight and materialization can't diverge. Regressions
  `internal/ui/daysrange_cap_test.go` + `TestDayInRangeCap` (`internal/ui/selection_test.go`).
- **Bare daily/weekly/monthly/yearly recurring TASK is not anchored to a date** (pass-20 MED) —
  CONFIRMED, UNFIXED: main.md's "daily → the base day" anchor promise holds for events but not tasks —
  `model.applyRecurAnchor` (quickadd.go:380) leaves `HasDate=false` for a bare frequency, so createTask
  omits DUE and the VTODO carries an RRULE with no DTSTART/DUE and can never be completed
  (`has no DTSTART/DUE to advance`). Repro `internal/model/bare_recur_todo_repro_test.go`. Fix: anchor
  bare frequencies to the base day. See the Feature-promise row.
  **FIXED (c16ea6c)**: fixed in the UI caller, NOT `applyRecurAnchor` — `createTask`'s DUE gate now also
  fires on `qa.Recur != nil`, anchoring a recurring task's due to the base day like `createEvent`
  already does. Deliberately not fixed in the model: forcing `HasDate` there would make bare-frequency
  EVENTS ignore their selected-day base and snap to today. The audit's model-layer repro (which mirrored
  the buggy gate) was replaced by a UI-level guard `internal/ui/bare_recur_todo_test.go`.
- **Sync core deep concurrency / TOCTOU** — re-swept pass 18 (first deep TOCTOU re-visit since pass
  11) and it bit: the `CommitPush` mid-push-delete resurrection HIGH (see the Sync-engine row). The
  fix landed (repro-first, both push variants + a -race invariant test), but the deeper
  reconcile-vs-concurrent-pull matrix (tombstone/keep-both races beyond the CommitPush window) is
  still only shallowly covered — the surface is warm again but not cleared, the main target for the
  next re-sweep.
- **Multi-account config parse — startup-hang via O(depth²) TOML decode** (pass-18 HIGH) — RESOLVED:
  a deeply nested inline-table config well under the 4 MiB read cap hung `Load()`/startup for
  minutes-to-hours. `checkNestingDepth` now rejects structural `{}`/`[]` nesting past 64 levels
  before `toml.Decode` (brackets in strings/comments skipped, so a real config is never falsely
  rejected). See the multi-account config-parse row.
- **`:config` reload does not re-parse the account list live** (pass-18 MED) — RESOLVED: `ConfigReload`
  now carries `Accounts`+`ActiveAccount`, and `applyConfigReload` adopts them on a successful reload,
  so a `:config`-added/renamed account is visible and reachable via `:account` without a restart. See
  the feature-promise row.
- **v1.1.0 permission-mode / component-set canary holes** (pass-18) — RESOLVED: the config
  group-readable warning mask, the state-file 0o600 mode, the CLI `components()` --tasks/--both
  helper, and `treeNodeAtY`'s upper bound each now have a boundary test verified to fail under its
  exact mutation (see the pass-18 canary section).
- **Live two-account end-to-end switch-and-sync** — RESOLVED (2026-07-22): pass 18 deferred this as
  unverifiable while the CalDAV server was offline; the server returned and the owner live-verified
  two-account end-to-end sync against it as part of the v1.1.0 release verification (recorded in
  main.md's v1.1.0 build record). Headless unit coverage remains the automated guard.
- **Import of a resource mixing a UID-bearing with a UID-less component** (pass-15 MED) —
  ACCEPTED RESIDUAL (owner decision 2026-07-18): the whole resource fails go-ical's encoder and
  is skipped, dropping the valid sibling (surfaced in `res.Skipped`, not silent). Every fix
  crosses a hard invariant (fabricate-UID reverses a settled decision; per-component encode
  weakens the iron rule; no raw bytes survive the transport's decode). Reachable only from a
  malformed foreign/hand-edited `.ics`. Revisit if a real server ever produces one.
- **Mouse handling and `:config`/$EDITOR reload** (internal/ui) — audited pass 16, both findings
  fixed (mouse double-click re-targets via `treeNodeAtY`; `:config` reload surfaces Load's
  warning). RESOLVED except the noted center-agenda-board double-click limitation (below).
- **Center agenda-board click-to-select** (internal/ui) — the board has no position→item hit-
  testing, so neither a single nor double click on it selects the item under the cursor (a
  double-click edits the current agenda selection). Low-impact follow-up; needs board-level
  hit-testing.
- **CLI subcommand connection-flag validation** (`cmd/lazyplanner/conn.go`, …) — RESOLVED
  (pass 16): `conn_test.go` `TestConnFlagsClientRequiresAllCredentials` now covers the
  credential-required guard directly (the escaped canary is closed).
- **go-ical encoder healer coverage** — pass 16 closed the two component-incomplete HIGH
  (VTIMEZONE-required-props stripped via `dropUnusableTimezones`; VJOURNAL/VFREEBUSY DTSTAMP +
  dedupe). The healer table must mirror go-ical's full `encoder.go` validateComponent — now a
  Hard-won guardrail in CLAUDE.md so the class does not reopen a third time. Residual: a missing
  **UID** on any component is still not healed (fabricating one churns sync identity — the same
  accepted residual as the pass-15 import MED).
- **go-ical semantic encoder healing** — RESOLVED (pass 10 fix): the five
  decode-but-unencodable classes (VEVENT DTEND+DURATION, VTODO DUE+DURATION, VTODO
  DURATION-without-DTSTART, empty VTIMEZONE incl. the `stripForbiddenNesting` self-
  inflict, VJOURNAL/VFREEBUSY nesting) are now healed on ingest with regression tests.

## Escaped mutation canaries — pass 23 (1 of 4 escaped → OPEN; 1 caught-but-only-cross-package)

Four canaries injected across `internal/model` (×2), `internal/ui`, `internal/store`. One genuine escape,
plus one catch that exposes a **surface-local** coverage hole worth recording.

- **ESCAPE (OPEN)** — `internal/model/recur_edit.go` `NewSeriesFrom` (~line 488), RECURRENCE-ID
  carry-forward filter: weakening `t.Unix() <= occ.Unix()` → `t.Unix() < occ.Unix()` left the whole repo
  green (`go test ./...` all packages ok, `internal/model` 90.9% statement coverage). The mutant is a real
  defect, not equivalent: on "edit this and future occurrences" applied to an occurrence that already had a
  per-instance override, the stale override at the split instant is copied into the new series and re-keyed
  to the new UID, where it **outranks** the master's freshly-mutated instance — the user's edit silently
  vanishes and the pre-split customization resurrects. The code comment states the intent explicitly ("the
  occurrence at `occ` itself is redefined by mutate below, so its old override is intentionally not
  carried"). Root cause of the hole: `internal/model/recuroverridesplit_test.go`
  (`TestSplitCarriesFutureOverride`) is the only test exercising override carry-forward and only asserts the
  strictly-after case (override Jan 20, split Jan 13); the `recur_split_{count,exdate,rdate}` tests cover
  RRULE/RDATE/EXDATE partitioning, not override partitioning. Suggested closure: extend
  `TestSplitCarriesFutureOverride` with a sibling case asserting an override exactly at `occ` is **dropped**
  from the future series and that the split-point occurrence expands with the draft's summary.
- **CAUGHT, but only cross-package — surface-local hole (OPEN)** — `internal/store/remote.go` `CommitPush`:
  replacing the copy-on-write pointer-identity check `if cur == pushed` with a value comparison
  `if cur.Name == pushed.Name` (always true at that call site, making the "a concurrent edit replaced it →
  keep the newer content, stay Dirty" branch dead code) left **`go test ./internal/store/` fully green** —
  all 41 store tests pass. Only one test in the entire repo caught it, one package away:
  `internal/sync` `TestSyncPushDoesNotClobberConcurrentEdit`. So the full gate has teeth, but anyone running
  the store package suite in isolation (a normal inner-loop habit) sees green on a silent lost update, and
  the ledger overstates `internal/store`'s own hardening for the "concurrent writes are version-checked"
  invariant — the store-side half is asserted only through `internal/sync`. The three CommitPush tests in
  the package (`commitpush_deletemidpush_test.go`) all exercise the `cur == nil` mid-push-**DELETE** path;
  none exercises the `cur != pushed` mid-push-**EDIT** path. Suggested closure: a store-package test
  asserting that when a fresh `*Resource` has replaced `pushed` in the map, `CommitPush` keeps `cur.Object`
  and returns Dirty=true with the advanced ETag baseline.
- **CAUGHT** — `internal/ui/timegridview.go` `navCell.overlaps`: flipping the half-open overlap test to
  closed/inclusive (so back-to-back 09:00–10:00 and 10:00–11:00 count as overlapping for `h`/`l` lane
  navigation) → `TestTimeGridSpatialDrillNav` failed with a precise message (`Left from C = "A", want B`).
  Note the catch rests on a **single** assertion.
- **CAUGHT** — `internal/model/timegrid.go` `LayoutDay` lane-reuse boundary: `!le.After(start)` →
  `le.Before(start)` (a freed lane is no longer reused at a touching boundary, opening a spurious column)
  → two independent failures, one of them the dedicated boundary guard
  `TestLayoutDayTouchingBoundary`, plus `TestLayoutDay`'s peak-concurrency case. Strong net here.

## Escaped mutation canaries — pass 22 (3 of 4 escaped → all CLOSED)

Four canaries injected across `internal/store`, `internal/caldav`, `internal/ui`, `internal/sync`. Only
the `internal/store` one was caught — an unusually high **3-of-4 escape rate**. All three escapes are
test-coverage holes: the code is correct today, but a plausible regression on that exact path would ship
silently. **All three are now CLOSED (e7f3625)** — each with a boundary test verified to fail under its
exact mutation (RED under mutation, GREEN reverted); test-only, no behavior change.

- **ESCAPE → CLOSED (e7f3625)** — `internal/caldav/object.go` `normalizeETag`: deleting the
  `etag = strings.TrimPrefix(etag, "W/")` weak-validator-prefix strip passed `go test ./internal/caldav/`.
  A server ETag returned as a weak validator `W/"abc"` is then stored verbatim as `W/"abc"` (the leading
  `W/` defeats the subsequent quote-strip, `etag[0]!='"'`), so the wrong value persists — breaking a later
  If-Match comparison against a bare stored ETag and surfacing spurious 412 conflicts. Root cause of the
  hole: every ETag-asserting test (`TestPutObjectCreate`, `TestPutObjectUpdateSendsQuotedIfMatch`,
  `TestPutObjectAccepts200OK`) uses only strong, double-quoted ETags; none exercises a `W/"…"` header, and
  a grep for `W/`/`weak`/`normalizeETag` in the caldav test files returns zero. **Closed** by
  `TestNormalizeETag` (`internal/caldav/normalizeetag_test.go`) asserting `normalizeETag("W/\"abc\"") == "abc"`,
  verified to fail under the removed `TrimPrefix`.
- **ESCAPE → CLOSED (e7f3625)** — `internal/ui/selection.go` `drillRange` bounds guard: weakening `idx >= len(items)` →
  `idx > len(items)` passed `go test ./internal/ui/`. When the drilled cursor index equals `len(items)`,
  the guard no longer returns nil, so `ci = len(items)` and the slice `items[ai : ci+1]` =
  `items[ai : len(items)+1]` goes out of range — a runtime panic that freezes the single-threaded TUI on a
  bulk-select over a drilled day at the terminal index. Root cause of the hole: `drillRange` is never
  referenced by name in the ui tests, and no test drills a day then extends a SELECT range to the terminal
  index (tests cover `daysRange`/`maxSelectDays` but not `drillRange`'s upper-index boundary). **Closed** by
  `TestDrillRangeAtTerminalIndexDoesNotPanic` (`internal/ui/drillrange_bounds_test.go`) — drills a day and
  sets the cursor to `len(items)` with a valid anchor, verified to panic under the mutation.
- **ESCAPE → CLOSED (e7f3625)** — `internal/sync/sync.go` `reconcileReadOnly` (~line 513): weakening the read-only
  discard guard `if r.Dirty || r.Href == ""` → `if r.Dirty && r.Href == ""` passed `go test ./internal/sync/`.
  A locally-edited resource that already has an href (a previously-synced item edited after its calendar
  became read-only) is then no longer discarded — the un-pushable local edit survives (neither discarded
  nor pulled over, since the switch has no Dirty branch), silently violating the "read-only calendars never
  keep local changes" hard invariant. Root cause of the hole: the only read-only-discard test
  (`TestSyncReadOnlyDiscardsStuckAndMirrors`) uses a never-synced resource (Href="", Dirty=true) that
  satisfies BOTH the original OR and the mutated AND. **Closed** by
  `TestReadOnlyDiscardsSyncedThenEditedResource` (`internal/sync/readonly_dirty_discard_test.go`) putting a
  synced-then-edited (Dirty, Href≠"") resource on a read-only calendar and asserting the local edit is
  discarded, verified to fail (kept Dirty) under the `&&` mutation.
- **CAUGHT** — `internal/store/mutate.go` `remove()` tombstone guard: dropping the `r.Href != ""` clause
  (`if tombstone && r.Href != ""` → `if tombstone`, so a never-synced local delete leaves a tombstone that
  would trigger a pointless server DELETE) → `go test ./internal/store/` FAILED
  (`TestDeleteNeverSyncedLeavesNoTombstone`, `TestCommitPushHonorsDeleteOfNeverSyncedCreate`). The
  never-synced-delete net has teeth.

## Escaped mutation canaries — pass 21 (1 of 4 genuinely escaped → CLOSED)

Four canaries injected across `internal/sync`, `internal/store`, `internal/model`, `internal/caldav`.
The workflow's own summary reported **2** escapes; this synthesis **re-verified both against the current
tree** and found only **1 genuine escape** — the `internal/model` `parseTimeHalf` "escape" was a
**false positive** (the workflow ran it in a stale worktree predating the pass-20 fix; the pass-20 guard
catches it on the real tree).

- **ESCAPE → CLOSED (e8ec765)** — `internal/caldav/object.go` `PutObject` (line 87): removing
  `http.StatusOK` from the accepted-success set (`!= StatusCreated && != StatusNoContent && !=
  StatusOK` → drop the `StatusOK` clause) passed `go test ./internal/caldav/`. A server that answers a
  PUT with **200 OK** (RFC-legal and returned by some real CalDAV servers) would be treated as a write
  failure — surfacing a spurious error and leaving the edit marked dirty/conflicted though the write
  succeeded. The success-path tests covered only 201 (`TestPutObjectCreate`) and 204
  (`TestPutObjectUpdateSendsQuotedIfMatch`); no test returned 200 on a PUT. **Closed** by
  `TestPutObjectAccepts200OK` (`internal/caldav/object_test.go`) — a PUT answered 200 OK with an ETag
  must succeed and return the new ETag; verified RED with `StatusOK` removed from the accepted set,
  GREEN on correct code.
- **FALSE ESCAPE → actually CAUGHT (verified this synthesis)** — `internal/model/quickadd.go`
  `parseTimeHalf` (line 551): widening the 24-hour ceiling `h > 23` → `h > 24`. The workflow reported
  this as an escape, but the pass-20 guard `TestParseTimeHalfHourCeiling`
  (`internal/model/parsetimehalf_test.go`, closed `d81d432`) already pins `24:00` as rejected. Applying
  the mutation on the current tree and running the test yields
  `parseTimeHalf("24:00") ok = true, want false` — **FAIL**. The workflow's ESCAPE was a stale-worktree
  artifact (the worktree predated `d81d432`). No action needed; the boundary is guarded.
- **CAUGHT** — `internal/sync/sync.go` reconcileCalendar step-(A) both-sides-changed conflict guard
  (line 427): flipping `r.Dirty && serverObj.ETag != r.ETag` → `== r.ETag` (inverting conflict
  detection so a genuinely-changed-server dirty resource silently falls through to the push branch — a
  lost-update overwrite) → `go test ./internal/sync/` FAILED across 7 tests
  (`TestSyncConflictKeepsBoth`, `TestSyncPushesLocalEdit`, `TestSyncPushDoesNotClobberConcurrentEdit`,
  `TestSyncUnparseableServerConflictNotTreatedAsDeletion`, `TestSyncRefetchesOn412`,
  `TestReproPullBatchClobbersConcurrentEditToOrphan`, `TestUndoOfSyncedEditSurvivesNextSync`). The
  conflict-vs-push boundary net has teeth.
- **CAUGHT** — `internal/store/mutate.go` `remove()` tombstone guard (`tombstone && r.Href != ""` →
  `r.Href == ""`, inverting the tombstone-leaving policy) → `go test ./internal/store/` FAILED across 7
  tests (`TestDeleteSyncedResourceLeavesTombstone`, `TestDeleteNeverSyncedLeavesNoTombstone`,
  `TestRestoreClearsTombstone`, `TestHasPendingChanges/tombstone_is_pending`, plus three CommitPush
  mid-push-delete invariants). Both directions of the inverted guard are pinned.

## Escaped mutation canaries — pass 20 (2 of 4 escaped → OPEN)

Both escapes are test-coverage holes (the code is correct today; a plausible regression on that exact
path would ship silently). Neither has been closed yet — they are recorded here as OPEN coverage gaps
for the fix arc / next pass.

- **ESCAPE → CLOSED (d81d432)** — `internal/ui/weekdaystrip.go` `weekdayStrip.moveCursor` (line 166): weakening the
  upper clamp `if w.cursor > daysInWeek-1` → `if w.cursor > daysInWeek` passed `go test ./internal/ui/`.
  This lets the day cursor advance to index 7 (`daysInWeek`), one past the last valid cell (6) in the
  seven-day strip — an off-by-one that can produce an out-of-bounds index into the seven day cells. No
  test exercised `moveCursor`'s upper-bound clamp. **Closed** by `TestWeekdayStripCursorClampsAtRightEdge`
  (`internal/ui/weekdaystrip_test.go`), which steps Right exactly onto the boundary (overshooting clamps
  under either version — only landing on 7 exposes it) and asserts `cursor == 6`; verified RED (`cursor = 7`)
  under the mutation.
- **ESCAPE → CLOSED (d81d432)** — `internal/model/quickadd.go` `parseTimeHalf` 24-hour (no am/pm) branch: widening
  the hour ceiling `if h < 0 || h > 23` → `h > 24` passed `go test ./internal/model/`. This accepts an
  invalid hour 24 (e.g. a bare-colon `24:00`), which `time.Date` normalization would spill into the next
  day. The quick-add time-range tests exercised valid boundaries but none fed an out-of-range 24-hour value.
  **Closed** by `TestParseTimeHalfHourCeiling` (`internal/model/parsetimehalf_test.go`) pinning
  `0/23/23:59` accepted, `24:00/25:00/99:00` rejected, plus the 12-hour am/pm ceilings; verified RED under
  the `> 23`→`> 24` mutation.
- **CAUGHT** — `internal/store/mutate.go` `remove()` tombstone `r.Href != ""` guard: dropping it (`if
  tombstone && r.Href != ""` → `if tombstone`) — `go test ./internal/store/` FAILED
  (`TestDeleteNeverSyncedLeavesNoTombstone`, `TestCommitPushHonorsDeleteOfNeverSyncedCreate`). A
  never-synced local delete would otherwise leave a tombstone that triggers a pointless server DELETE.
- **CAUGHT** — `internal/sync/sync.go` `downloadResilient` degraded-fetch guard: deleting `unfetched[ref.Href]
  = true` (so a resource that merely failed its individual GET is mistaken for a server deletion) — `go test
  ./internal/sync/` FAILED across 3 tests (`TestDegradedDownloadNotTreatedAsDeletion`,
  `TestDegradedDownloadDirtyResourceNotFalselyConflicted`, `TestReadOnlyDegradedDownloadKeptVsDeleted`).
  The degraded-download-vs-deletion net has teeth.

## Escaped mutation canaries — pass 19 (2 of 3 escaped → both CLOSED 2026-07-24)

Both escapes were test-coverage holes on this pass's target surfaces (code correct at the time; a
plausible regression on that exact path would have shipped silently). Both are now closed with a
boundary test verified to fail under its exact mutation.

- **ESCAPE → CLOSED (fb7d8e2)** — `internal/ui/selection.go` `dayInRange()` upper-boundary flip
  `!d.After(model.DayStart(to))` (inclusive) → `d.Before(model.DayStart(to))` (exclusive) passed `go
  test ./internal/ui/`. `dayInRange`/`selDayAnchor` are draw-path-only helpers (calendarview.go:269,
  timegridview.go:575) that style in-range SELECT days; neither appeared in any `*_test.go`. The
  SELECT-range *materialization* (daysRange/selRange) was well covered (TestDaysRange*), but the
  *visual highlight* boundary it drives was unguarded — an off-by-one making the highlighted band stop
  one day short of the acted-on interval (last/cursor day not shown selected though bulk ops still hit
  it) would have shipped undetected. Closed by `TestDayInRange`, `TestDayInRangeReversedCursor`,
  `TestDayInRangeZeroAnchor` (`internal/ui/selection_test.go`), verified to fail under the mutation.
- **ESCAPE → CLOSED (29ca392)** — `internal/model/quickadd.go` `parsePriority` (line 434): narrowing the
  numeric upper bound `n <= 9` → `n <= 8` passed `go test ./internal/model/`. Quick-add priority tests
  (quickadd_test.go) exercised only `!high` and `!1`; no test parsed a numeric `!9`/`!8`, so the 1–9
  edge was entirely unguarded (priorityrange_test.go covers the separate iCal-clamp path; the fuzz
  only asserts 0–9, which a narrowed bound never violates). Closed by a table test through
  `parsePriority` covering `!9`/`!8`/`!10` (`internal/model/quickadd_test.go`), verified to fail under
  the mutation.
- **CAUGHT** — `internal/sync/sync.go` `reconcileCalendar` step B (pull-server-new loop, ~line 447):
  dropping the tombstone guard `|| tombstonedHref[o.Path]` re-pulls a locally-deleted-but-unpushed
  resource, resurrecting it and clearing its tombstone before step C can push the DELETE. `go test
  ./internal/sync/` FAILED across 5 tests (TestSyncPushesTombstoneDelete,
  TestSyncTombstoneVsServerEditIsConflict, TestSyncDeleteTransient403KeepsTombstone,
  TestSyncDeleteConfirmedReadOnlyDiscards, TestUndoOfSyncedDeleteSurvivesNextSync). The delete/tombstone
  path has real teeth.

## Escaped mutation canaries — pass 18 (4 of 4 escaped → all CLOSED 2026-07-21)

All four canaries this pass escaped — each a test-coverage hole on a v1.1.0 / sibling surface (code
correct today, but a plausible regression on that exact path would ship silently). All four are now
closed: each has a boundary test verified to fail under its exact mutation (mutation applied → RED,
reverted → GREEN).

- **CLOSED** — `internal/config/config.go` `permissionWarning()`: narrowing the loose-permission
  mask `mode.Perm()&0o077 != 0` → `&0o007` (dropping the group bits) passed `go test
  ./internal/config/`. The only permission test used mode 0o644, which still trips the narrowed
  0o007 mask via the other-readable bit — so a **group-readable-only** config (0o640) silently
  stopped warning "may hold a password". Closed by `permission_warning_test.go`
  `TestPermissionWarningFlagsGroupAndOther` (tables 0o600/0o640/0o604), verified RED under the
  0o077→0o007 mutation (the 0o640 case no longer warns).
- **CLOSED** — `internal/state/state.go` `Save()`: flipping the state-file mode 0o600 → 0o644
  (world-readable) passed `go test ./internal/state/`. No test asserted the written FileMode. Closed
  by `state_mode_test.go` `TestSaveFilesAre0600` (asserts both `Save` and `SaveGlobal`, which share
  `writeJSONFile`, produce 0o600 files; Unix-only via `runtime.GOOS`), verified RED under the
  0o600→0o644 mutation.
- **CLOSED** — `cmd/lazyplanner/calendar.go` `components()`: flipping the `--tasks` branch
  `[]string{"VTODO"}` → `[]string{"VEVENT"}` passed `go test ./cmd/lazyplanner/`. `components()`,
  `slugify()`, and `joinWarnings()` had zero coverage. Closed by `calendar_helpers_test.go`
  `TestComponents` (tables default/--tasks/--both/both-wins), plus `TestSlugify` and
  `TestJoinWarnings`, verified RED under the --tasks→VEVENT mutation.
- **CLOSED** — `internal/ui/mouse.go` `treeNodeAtY` (~line 101): off-by-one `if idx >=
  len(visible)` → `if idx > len(visible)` passed `go test ./internal/ui/`. A click on the row exactly
  one past the last visible tree node would index `visible[len]` → panic the whole TUI. Closed by
  `treenodeaty_test.go` `TestTreeNodeAtYPastLastNode` (probes the `idx==len(visible)` boundary row,
  asserts no panic / nil target), verified RED under the `>=`→`>` mutation (panic: index out of range
  [2] with length 2).

## Escaped mutation canaries — pass 17 (4 of 4 escaped → all CLOSED 2026-07-18)

All four canaries this pass escaped — each was a test-coverage hole (code correct; a plausible
regression on that exact path would have shipped silently). All four are now closed with boundary
tests each verified to fail under its exact mutation and pass when reverted; no production code
changed for the canary closes.

- **CLOSED** — `internal/sync/sync.go` `reconcileReadOnly` (line 514): inverting the degraded-
  download guard `case !onServer && unfetched[r.Href]:` → `!unfetched[r.Href]` escaped. The
  read-*write* twin was covered (`degraded_download_deletion_test.go`), but the **read-only** path
  had no test combining a read-only calendar with a degraded/partial download. Guard:
  `TestReadOnlyDegradedDownloadKeptVsDeleted` (`internal/sync/readonly_degraded_download_test.go`)
  exercises both sides on a read-only calendar — an unfetched (GET-failed) resource still on the
  server is KEPT; a genuinely server-absent one is Forgotten (`PulledDeletes==1`). Verified to fail
  under the inversion at the reconcileReadOnly site (distinct from the read-write site at line 396).
- **CLOSED** — `internal/model/recur_edit.go` `NewSeriesFrom` (this-&-future split): weakening the
  future-series COUNT clamp `if remaining < 1 { remaining = 1 }` → `< 0` escaped — at/after the final
  occurrence `pastCount == COUNT` so `remaining` computes to 0, and rrule-go treats `COUNT=0` as
  *unbounded*. Boundary confirmed empirically: `rruleIterationsBefore` counts iterations strictly
  before occ, so `remaining==0` requires occ *past* the last occurrence. Guard:
  `TestSplitAtSeriesEndKeepsFutureBounded` (`internal/model/recur_split_count_test.go`) splits a
  COUNT=3 series one day past its end and asserts the future series stays bounded (exactly one
  occurrence, not 176 under the mutation).
- **CLOSED** — `internal/model/agenda.go` `DayAgenda` (todo due-window *upper* bound): flipping
  `t.Due.Before(dayEnd)` → `!t.Due.After(dayEnd)` (exclusive → inclusive) escaped — the *upper*-bound
  twin of the pass-14 lower-bound escape. Guard: `TestDayAgendaExcludesTodoDueAtDayEnd`
  (`internal/model/agenda_test.go`) asserts a todo due exactly at `dayEnd` yields 0 items on the
  current day (it belongs to the next day). Verified to return 1 item under the mutation.
- **CLOSED** — `internal/state/state.go` `Load()` (line 52): dropping the `json.Unmarshal` error
  check escaped — the only bad-file test (`"{ not json"`) fails Unmarshal *before* mutating the
  struct. The canary's *suggested* trailing-garbage repro would also have escaped: `json.Unmarshal`
  runs `checkValid` over the whole input first, so trailing garbage is rejected before any decode and
  leaves the struct zero (verified empirically). The case that actually requires the check is a **type
  mismatch on a later field** (`{"left_width":5,"hidden_calendars":123}`) — `checkValid` passes, the
  decoder populates `left_width`, then errors, leaving a half-populated struct. Guard:
  `TestLoadPartialParseThenErrorIsZero` (`internal/state/state_test.go`) asserts Load rejects it to a
  zero State; verified to surface `{LeftWidth:5}` under the mutation.

## Escaped mutation canaries — pass 13 (4 of 4 escaped → all CLOSED 2026-07-16)

Code was correct; each path was unguarded so a plausible regression would ship silently.
Each is now closed with a boundary test verified to fail under its mutation before adding.

- **`internal/model/timegrid.go` `LayoutDay`** (cluster-flush / lane-reuse at a touching
  boundary): a `!start.Before`→`start.After` or `!le.After`→`le.Before` flip folds a
  touching occurrence into the prior cluster and inflates a standalone block's `Lanes`.
  CLOSED — `TestLayoutDayTouchingBoundary` asserts lane-minimality at the touching edge.
- **`internal/sync/sync.go` `Sync`** (CTag-cache guard): `len(res.Skipped) == skipsBefore`
  → `>=` collapses to always-cache, so a per-resource failure caches the CTag anyway and
  the next sync never retries. CLOSED — `TestDegradedDownloadDoesNotCacheCTagSoNextSyncRetries`
  (added with the HIGH #1 fix) asserts the CTag is not cached after a skip.
- **`internal/caldav/object.go` `DeleteObject`**: dropping the empty-ETag `If-Match: *`
  fallback turns a conditional delete into a blind unconditional DELETE. CLOSED —
  `TestDeleteObjectEmptyETagSendsIfMatchStar` inspects the outgoing header.
- **`internal/config/config.go` `Load`**: dropping `io.LimitReader(f, maxConfigBytes)`
  removes the 4 MiB read cap. CLOSED — `TestLoadCapsReadSize` feeds an oversized file
  (valid before the cap, garbage after) so an uncapped read would error.

## Escaped mutation canaries — pass 16 (2 of 4 escaped → now CLOSED)

Both escapes were test-coverage holes in the CLI/config surfaces this pass targeted (code
correct, guard untested); both are now closed with a boundary test verified to fail under its
mutation.

- **ESCAPE → CLOSED (pass 16)** — `internal/config/config.go` `Server.Configured()`: flipping
  `s.URL != "" && s.Username != ""` → `||` passed the suite, which only called `Configured()` on
  a fully-populated server; nothing asserted `false` for a partial (URL-only/username-only)
  config, which would then be synced against. CLOSED — `TestServerConfigured` (table:
  both/url-only/username-only/neither), verified to fail under the `||` flip.
- **ESCAPE → CLOSED (pass 16)** — `cmd/lazyplanner/conn.go` `connFlags.client()`: flipping the
  credential-required guard `*url=="" || *username=="" || *password==""` → `&&` passed — conn.go/
  import.go/sync.go/calendar.go had NO direct tests, so a URL+username-without-password would
  build a client with empty credentials. CLOSED — new `conn_test.go`
  `TestConnFlagsClientRequiresAllCredentials` asserts an error for each partial-credential
  combination, verified to fail under the `&&` flip.
- **CAUGHT** — `internal/ui/calendarview.go` `drawDayItems` (~line 307): off-by-one
  `if n <= avail` → `if n < avail` — `go test ./internal/ui/` FAILED
  (`TestCalendarViewDrawsMonth`, exactly-fitting item pushed into a spurious "+N more").
- **CAUGHT** — `internal/sync/sync.go` `reconcileCalendar` (line ~416): inverting both-sides-
  changed conflict detection `serverObj.ETag != r.ETag` → `==` — `go test ./internal/sync/`
  FAILED across 7 tests (incl. `TestSyncPushesLocalEdit`, `TestSyncConflictKeepsBoth`,
  `TestSyncPushDoesNotClobberConcurrentEdit`, `TestSyncRefetchesOn412`).

## Escaped mutation canary — pass 15 (1 of 3 escaped → now CLOSED)

The escape was a test-coverage hole (code correct, path unguarded); it is now closed.

- **ESCAPE → CLOSED (pass 15)** — `internal/caldav/listobjects.go` `ListObjectHrefs`: removing
  the `|| r.isCollection()` clause from the member-filter passed `go test ./internal/caldav/`.
  `TestListObjectHrefs`'s fixture had exactly one collection response whose href equaled the
  queried calendar path, so the surviving path-equality clause (`TrimRight(href,"/") ==
  collection`) still excluded it and the count was unchanged. Nothing exercised a **nested**
  sub-collection (a distinct href that is a collection, e.g. `/dav/cal/personal/inbox/`) —
  precisely what `isCollection()` exists to filter; a regression would leak nested-collection
  hrefs, and the per-resource download fallback would GET a collection URL as an event object.
  CLOSED — `TestListObjectHrefsExcludesNestedCollection` adds a nested-collection href ≠ the
  query path and asserts it is excluded, verified to fail under the dropped clause.
- **CAUGHT** — `internal/store/mutate.go` `Store.remove` (tombstone `r.Href != ""` guard):
  `if tombstone && r.Href != ""` → `if tombstone` — `go test ./internal/store/` FAILED,
  `TestDeleteNeverSyncedLeavesNoTombstone`. A never-synced local delete would otherwise leave a
  tombstone that a later sync tries to DELETE server-side for a resource that never existed.
- **CAUGHT** — `internal/sync/sync.go` `reconcileCalendar` step B (pull-server-absent-locally):
  dropping `|| tombstonedHref[o.Path]` → re-pulls a locally-deleted-but-unpushed resource,
  resurrecting it. `go test ./internal/sync/` FAILED across 5 tests
  (`TestSyncPushesTombstoneDelete`, `TestSyncTombstoneVsServerEditIsConflict`,
  `TestSyncDeleteTransient403KeepsTombstone`, `TestSyncDeleteConfirmedReadOnlyDiscards`,
  `TestUndoOfSyncedDeleteSurvivesNextSync`).

## Escaped mutation canary — pass 14 (1 of 3 escaped → now CLOSED)

The escape was a test-coverage hole (code correct, path unguarded); it is now closed.

- **`internal/model/agenda.go` `DayAgenda`** (todo due-time lower bound): flipping the
  inclusive lower bound `!t.Due.Before(dayStart)` (Due ≥ dayStart) → `t.Due.After(dayStart)`
  (Due > dayStart) drops any todo due *exactly* at the start of the day (midnight) — the
  natural due time for a date-only / all-day todo — silently vanishing it from that day's
  agenda. `TestDayAgenda`'s todos were due at 09:00 (inside the window) and dayEnd+1h
  (outside); none sat exactly on dayStart, so the flipped boundary was never exercised.
  CLOSED (pass 14): `TestDayAgendaIncludesTodoDueAtMidnight` pins a todo due exactly at
  dayStart, verified to fail under the mutation and pass after reverting.
- The other two canaries were **caught**: `internal/ui/grab.go` `grabNudge` J/K resize min-
  duration guard (`TestGrabResizeRejectsZeroDuration`), and `internal/sync/sync.go` CTag-cache
  guard (`TestDegradedDownloadDoesNotCacheCTagSoNextSyncRetries`, added with pass 13's HIGH #1).

## Pass 11 — RESOLVED (all 7 findings + both canary holes fixed 2026-07-15)

Every pass-11 finding was fixed with an adversarially-verified regression test and
its own commit; the full gate + `-race` on store/sync/ui pass. See `log.md`.

- **3 HIGH data-loss** (the shared "multi-write op with no rollback" class):
  `PullRemoteBatch` now skips a Dirty resource (`store.ErrKeptLocalEdit`) instead of
  clobbering a concurrent local edit; `commitSplit` restores the master when the
  future write fails; `commitDetach` (extracted from `editTodoDetachForm`) restores
  the series when the standalone write fails. All three now match the sibling
  `beginGrabFuture` rollback.
- **2 MED:** `cancelGrab` captures and surfaces revert errors (and restores before
  deleting so a failed un-cap can't compound); the todo detach uses the new
  `model.DetachTodoOccurrence`, which clones the original component so unmodeled iCal
  props (VALARM/X-/etc.) survive (iron rule).
- **2 LOW:** `grabNudge` commits via the new `store.PutIfUnchanged` (version-checked
  write; aborts without reverting on a concurrent pull); the todo nudge re-checks
  `HasDue` after re-locate.
- **2 escaped mutation canaries → closed:** boundary tests added for the month-grid
  event-drill `j`/`k` guard (`internal/ui/calendarview.go`, both KeyRune + arrow
  paths) and `clampIndex` at `i == n` (`internal/ui/edit.go`); each verified to fail
  under its mutation. (The both-sides-changed conflict-comparison canary in
  `internal/sync/sync.go` was already caught by 5 tests.)

## Pass 12 — RESOLVED (all 7 findings + all 3 canary holes fixed 2026-07-15)

Pass 12 audited the pass-11 named follow-up (the Locate→Put no-version-check pattern
outside grab) plus the session undo stack, the calendar color/name PROPFIND decode,
and (fuzz/edge) the state-file parse and `:calendar` arg parsing. 7 findings (2 HIGH,
5 MED) were confirmed with executed repros; **all are now fixed**, each adversarially
verified with a regression test and its own commit; full gate + `-race` on
caldav/state/store/sync/ui pass. See `log.md`.

- **HIGH — quick sp/sd flattened STATUS + dropped PERCENT-COMPLETE / restamped COMPLETED
  (MED):** `EditTodo` now calls `setCompleted` only when completed-ness actually changes
  (`isCompletedStatus`), preserving a foreign client's IN-PROCESS/CANCELLED status,
  PERCENT-COMPLETE, and the original COMPLETED timestamp.
- **HIGH — undo of a synced delete lost the item / MED — undo of a synced edit didn't
  stick:** new `store.RestoreDirty` marks the resurrection/revert Dirty; `undoLast` uses
  it, so sync pushes it or raises a keep-both conflict instead of Forgetting/pulling-back.
  Verbatim `Restore` stays for the rollback paths.
- **MED x2 — the systemic Locate→Put:** `applyTodoField` (sp/sd), `toggleComplete` (Space),
  and `advanceRecurringTodo` now commit via `store.PutIfUnchanged(loc.Prev)`, aborting on
  `applied==false`. All three of the systemic sites the pass-11/12 reports named are closed.
- **MED — raw-href keying:** new `caldav.hrefKey` decodes the href like go-webdav derives
  `Calendar.Path`; the color, privilege (discover + reactive re-check), and CTag maps plus
  the lookup key all use it. This also fixed the privileges **fail-open** (read-only shares
  looked writable) and the CTag miss.

No parse-level finding on the state file or `:calendar` arg parsing.

**All 3 escaped mutation canaries → closed** (code was correct; tests added, each verified
to fail under its mutation): `privileges.go` `writable()` per-grant table (write /
write-content / bind / all independently); `state.go` `Save` temp+rename atomicity; `grab.go`
K-resize rejecting a zero-duration event.

## Systemic follow-up — the Locate→Put no-version-check pattern (RESOLVED again, pass 13)

Pass 11 fixed grab; pass 12 fixed three sites (quick-field `sp`/`sd`, `Space`
completion, recurring-todo advance) via `store.PutIfUnchanged` and declared the class
"structurally closed". **Pass 13's spec-diff proved that claim FALSE** — the sweep found
two remaining unconverted sites sharing the exact TOCTOU clobber class, now both fixed:
- **`applyMutation`** (`internal/ui/edit.go`) — the shared tail of every form Save and
  the `commitMutation`/`commitMutationKeepingDrill` callers (edit form + all
  recurrence-scoped saves). FIXED: version-checks an edit (`prev != nil`) via
  `store.PutIfUnchanged`, surfaces a stale skip; creations still use plain `Put`.
- **`reparentSelected`** (`internal/ui/edit.go`, `H`/`L` indent/outdent). FIXED: commits
  via `store.PutIfUnchanged(loc.Prev)` with an `applied==false` retry flash.

Regression tests: `internal/ui/editclobber_test.go` (drives the real `applyMutation`)
and `internal/store/reparent_noclobber_test.go`. The lesson stands: "closed" requires an
exhaustive site sweep, not a spot check — do the sweep before declaring the class shut.
No known Locate→Put clobber sites remain after this exhaustive sweep.

## Delete-half of the write-atomicity finding (intentionally not "healed")

- A crash between an `.ics` **delete** and its tombstone write re-pulls the item on
  next sync — safe and recoverable. Synthesizing a tombstone from a missing-`.ics`-
  with-href would risk deleting server data whenever a `.ics` merely went missing, so
  the safe re-pull is kept by design (not a gap to close).

> The workflow updates this table and this list at the end of each pass. Hand-edits
> are welcome — it's a plain table on purpose.
