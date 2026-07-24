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
| Recurrence expansion (read) | internal/model | fuzz, tz/DST sweep, scale-bound | 4,5,8 | recent |
| Recurrence write-side (mutate/split/advance) | internal/model | fuzz, deep audit, spec-diff | 9,14 | recent (2 MED fixed pass 14: this-&-future split added a phantom trailing occurrence past a pre-split EXDATE — now counts RRULE iterations via rruleIterationsBefore — and duplicated a trailing RDATE across both halves — now partitioned via filterRDates; both restore main.md:362. Codified as a Hard-won guardrail) |
| Subtask tree build | internal/model | fuzz, scale | 4,5,17 | recent (pass 17 re-fuzzed BuildTree cycle/orphan/deep-chain classification against 12 passes of model evolution — no finding; pass-17 canary CLOSED: the this-&-future split COUNT-clamp boundary is now guarded by TestSplitAtSeriesEndKeepsFutureBounded) |
| Quick-add parser | internal/model | fuzz, input-edge | 4,14 | recent (MED fixed pass 14: an invalid day-of-month in the slashed `2/30` and month-name `feb 30` forms was silently normalized by time.Date to a wrong date and rolled a year forward — validYMD/rollForwardMonthDay now reject it, matching the ISO form and the leave-text-in-title principle) |
| Timezone / DST | internal/model | exhaustive sweep, fuzz | 8,14,17 | recent (MED fixed pass 14: resolveDateTime parsed only a single date-time, so an RFC-5545-valid comma-listed multi-valued RDATE/EXDATE — or VALUE=PERIOD RDATE — errored and Occurrences collapsed the whole RRULE series to its base instance; resolveDateTimeValues now splits per value. Codified as a Hard-won guardrail. **Pass 17 MED — FIXED**: an RDATE;VALUE=PERIOD carrying an IANA TZID (e.g. America/New_York) was silently mis-zoned to floating time — resolveDateTimeValues left the VALUE=PERIOD param on the sub-prop so go-ical's prop.DateTime rejected it, and resolveDateTime had only a Windows-name recovery branch + floating fallback. Fixed: resolveDateTimeValues drops the stale VALUE=PERIOD param on the reduced sub-prop (cloning the shared map), and resolveDateTime gained an IANA-TZID time.LoadLocation recovery branch so the IANA and Windows spellings agree. internal/model/tz.go. Regression internal/model/rdate_period_tzid_test.go) |
| Windows→IANA zone mapping + TZID resolution | internal/model | fuzz | 17 | recent (first direct audit of windowszones.go's lookup table + tz.go's unknown-TZID fallback. No crash/panic on adversarial/unresolvable TZIDs — the floating fallback holds. One functional gap found on the same resolveDateTime path: the IANA-TZID VALUE=PERIOD RDATE mis-zone (see Timezone/DST row, MED, tz.go:68) — a Windows-name TZID resolves but the IANA-name equivalent falls through to floating) |
| Color parsing (ParseHexColor / NearestANSI16 / ReadableFg / Luminance) | internal/model | input-edge | 17 | recent (first audit of model/color.go — parses untrusted server-supplied CALENDAR-COLOR / hex strings. Boundary-swept malformed length, non-hex digits, out-of-range channels, empty input to NearestANSI16 — no panic/OOB finding; the parser rejects/clamps as expected. Distinct cell from the caldav color-PROPFIND decode audited pass 12) |
| CalDAV network boundary (response-parse: multiget/PROPFIND/REPORT decode, hand-rolled ListObjectHrefs XML, truncated/oversized bodies, redirects) | internal/caldav | fault-injection, panic-guard, fuzz | 4,7,15 | recent (pass 15 HIGH fixed: `PutObject`/`DeleteObject` followed a 301/302/303 on writes — Go's default redirect policy downgrades PUT/DELETE to a bodyless GET dropping the body + If-Match/If-None-Match; a 200/204 on the followed GET landed in the success set, so the write silently vanished and sync cleared the dirty flag. `NewClient` now installs a method-aware `CheckRedirect` returning `http.ErrUseLastResponse` for write methods (`isWriteMethod`), and `PutObject`/`DeleteObject` treat any 3xx as an error; reads and RFC 6764 `.well-known` discovery still follow redirects. Repro `internal/caldav/redirect_test.go`. Canary CLOSED this pass: `ListObjectHrefs` nested-collection filter now guarded by `TestListObjectHrefsExcludesNestedCollection` — see below) |
| Sync engine (data-loss / TOCTOU) | internal/sync, internal/store | deep audit, race | 3,11,18 | recent (HIGH fixed pass 11: PullRemoteBatch skips a Dirty resource via store.ErrKeptLocalEdit. **Pass 18 HIGH — FIXED**: `store.CommitPush` treated `cur==nil` (resource deleted mid-push by the event-loop while the sync goroutine's PUT was in flight) identically to `cur==pushed`, rebuilding the deleted resource clean, and `stageResourceLocked` wiped the tombstone — a user delete landing during an edit-PUT was silently, permanently lost (next sync a no-op). Now `CommitPush` reads `cur` under the lock and, when `cur==nil`, honors the deletion instead of resurrecting: `honorMidPushDeleteLocked` ensures a tombstone carrying the post-PUT href/ETag — advancing an existing one's ETag (pushUpdate/synced case, so the next conditional DELETE's If-Match matches) or creating one (pushCreate/never-synced case, whose local delete left none, else the just-created server copy is re-pulled and resurrected). Repro `internal/store/commitpush_deletemidpush_test.go` (both push variants + a 200× concurrent Delete‖CommitPush invariant, green under -race)) |
| Sync reconcile state machine (reconcileCalendar/reconcileReadOnly case matrix, keep-both, Forget, read-only-twin branches) | internal/sync, internal/store | data-loss | 13,14,15 | recent (HIGH fixed pass 13: degraded fetch no longer inferred as deletion. Pass 14 MED fixed: pushDelete's 412 branch cleared the tombstone unconditionally, silently dropping a delete-vs-server-change conflict when the server version was unparseable or absent from a degraded download — it now clears the tombstone only after resurrect+flag, else keeps it and records a skip. Pass 14 LOW fixed: keep-local of a server-deleted conflict never converged — ResolveKeepLocal now clears the Href on a ServerDeleted conflict so reconcile re-creates the item instead of re-raising the conflict. Pass 15 re-swept the keep-both / Forget / read-only-twin data-loss branches — no new finding; the tombstone-vs-server-edit / re-pull-guard paths are exercised by the -race canary below) |
| CalDAV request-construction (MKCALENDAR/PROPPATCH/DELETE bodies, resolve()/href, color/name validation, idempotency) | internal/caldav | fault-injection | 13 | recent (2 MED fixed: DELETE now idempotent on 404/410, MKCALENDAR idempotent on 405 — no more pending-delete/pending-create wedge) |
| Sync concurrency | internal/sync | -race stress | 3,11 | recent (re-run post batching/CTag; no new race; the store-level clobber finding is fixed) |
| CTag incremental short-circuit (skip DownloadAll) | internal/sync | data-loss, fault-injection | 11,16 | recent (pass 16 fault-injected a stale/duplicate/absent/lying server CTag driving the skip decision — no finding; the skip is fail-safe) |
| Background sync goroutines (startPeriodicSync timer + flushOnQuit quit push) | internal/ui, internal/sync | race | 11,16 | recent (pass 16 re-swept under -race against the pass 14–15 write-path changes (CheckRedirect, tombstone-412) — no deadlock/race found) |
| Store filesystem robustness (paths, revert, rollback, load-time stale-temp sweep) | internal/store | deep audit, race | 9,15 | recent (pass 15 HIGH fixed: `loadCalendar`'s stale-temp sweep used the over-loose `isStaleTempName` (`HasPrefix(".") && Contains(".tmp-")`) and ran BEFORE the `.ics` extension filter, so a real resource whose sanitized name began with a dot and contained `.tmp-` (e.g. UID `.tmp-important@host` → `.tmp-important_host.ics`) was `os.Remove`'d on Open — permanent loss for an offline-created not-yet-pushed item, reachable from a hostile server href. `isStaleTempName` now requires the actual leftover shape — dot-prefixed and ENDING in `.tmp-<digits>` (what `os.CreateTemp` produces); a real resource ends in `.ics` so it can't match, and genuine leftovers are still swept (`TestOpenSweepsStaleTempFiles`). Repro `internal/store/staletemp_test.go`. Pass 15 -race stress on the write primitives found no race — see below) |
| Local disk / config input boundaries | internal/config, internal/state, internal/store | deep audit, size caps, fuzz | 9,13 | recent (pass 13 fuzzed TOML parse / first-run round-trip / password_command stdout — no finding; canary hole below: config-read size cap untested) |
| State-file load/parse (widths/hidden-cals/hour-zoom) | internal/state | fuzz (adversarial values), deep audit, size cap | 9,12,17 | recent (pass 17 re-fuzzed adversarial widths/hidden-cals/hour-zoom against the later calendar-id changes — no parse finding; pass-17 canary CLOSED: Load()'s json.Unmarshal error check is now guarded by TestLoadPartialParseThenErrorIsZero — a later-field type mismatch that populates then errors must yield a zero State) |
| Quick-field edits (sp/sd) — Locate→Put write | internal/ui, internal/model | data-loss | 12 | recent (HIGH STATUS-flatten + MED COMPLETED-restamp + MED TOCTOU all fixed) |
| Completion toggle (Space) + recurring-todo advance — Locate→Put | internal/ui | data-loss | 12 | recent (MED TOCTOU fixed: PutIfUnchanged) |
| Session undo stack (pushUndo / prev-snapshot restore replay) | internal/ui, internal/store | data-loss | 12 | recent (HIGH undo-of-synced-delete + MED undo-of-synced-edit fixed: RestoreDirty) |
| Calendar-color + display-name PROPFIND parsing (discoverColors/SyncCalendarName) | internal/caldav | fault-injection | 12 | recent (MED href-key encoding mismatch fixed: hrefKey decodes; also fixed the privileges fail-open + CTag miss) |
| `:calendar` command argument parsing (rename/color/hide/show) | internal/ui | input-edge | 12 | recent (no new finding) |
| Bulk-pull batching / scale | internal/store, internal/sync | benchmarks, data-loss | 5,11 | recent (HIGH fixed: PullRemoteBatch no longer clobbers a href-less pull-orphan's local edit) |
| Grab-mode temporal-manipulation state machine (per-nudge commit, snapshot/2-resource revert) | internal/ui | data-loss, input-edge | 11 | recent (MED+LOW x2 fixed pass 11: cancelGrab surfaces revert errors; PutIfUnchanged version-check; HasDue re-check. **Post-v1.3.0 user report FIXED**: an all-scope day-move (`h`/`l`) shifted DTSTART but left a day-pinning `BY*` (weekly BYDAY / monthly nth-weekday — every v1.3.0 preset carries one) stale, so the series kept firing on the old day, the moved DTSTART fell outside its own rule, and the event vanished from the calendar; `model.ReanchoredRecurrence` now re-anchors the rule to the moved day (weekly sets shift whole, monthly nth-weekday re-derives) or blocks an opaque "kept" rule with a hint. Codified as a Hard-won guardrail. Regression `internal/model/reanchor_test.go`, `internal/ui/grab_recur_reanchor_test.go`) |
| SELECT mode + bulk ops + bulk grab (multi-select range derivation, bulk complete/delete/yank-paste/grab, GRAB nested inside SELECT) | internal/ui | — | — | never (new v1.4.0 surface, built 2026-07-23, not yet audited. One build-time finding already fixed during TDD, not left for the next pass: `bulkDeleteRoots`'s ancestor-absorption walk trusted untrusted `RELATED-TO` parent data with no visited guard — a reciprocal parent cycle (hand-edited or foreign `.ics`) would spin the walk forever, freezing the single-threaded UI event loop; the sibling `descendants()` walk already carried this guard, so this was the same missing-guard-that-a-sibling-has shape as pass 17's Import/reconcileCalendar and resolveDateTime findings. Fixed same-day with a `seen` map, repro-first (`internal/ui/bulkops_test.go`). **Pre-existing adjacent finding flagged for the next hardening pass, not fixed here**: `moveSubtreeOps`'s source-side rewrite (`internal/ui/yankpaste.go`) commits via a bare `store.Put` on an existing resource rather than `store.PutIfUnchanged` — predates v1.4.0 (the single-item yank/paste path), and is now also exercised by the new multi-root paste (`pasteMultiRoot`), which correctly version-checks its own new same-list reparent path (`reparentOps`) but still calls through to the old unguarded `moveSubtreeOps` for the cross-list source-side delete/rewrite — a version-check-invariant gap on a path more surfaces now reach) |
| Recurrence-edit UI orchestration (scope picker + this-&-future split/detach) | internal/ui | data-loss | 11 | recent (HIGH x2 + MED fixed: commitSplit/commitDetach rollback; DetachTodoOccurrence preserves props) |
| UI draw paths (custom widgets) | internal/ui | display stress | 6,14 | recent (pass 14 re-swept the newer widgets — agendaboard/itemforms — no crash/freeze; no finding) |
| UI input handlers (keys/chords/commands) | internal/ui | deep audit, input-edge | 9,13,14 | recent (pass 14 input-edged the raw non-command keypress/chord dispatch (keys.go navigation, grab activation, mode/drill transitions at boundary states) — no finding) |
| CLI wiring | cmd/lazyplanner | deep audit, input-edge | 9,16 | recent (pass 16 input-edged the flag.FlagSet subcommand dispatch — 1 MED + 1 LOW, both fixed: a shared `parseFlags` helper returns flag.ErrHelp unchanged and tags other parse errors `errFlagParsed`; `report()` maps ErrHelp→exit 0 and errFlagParsed→exit 2 without re-printing, so `-h`/`--help` succeeds cleanly and a bad flag prints once. Canary CLOSED — new `conn_test.go` `TestConnFlagsClientRequiresAllCredentials` guards the credential path) |
| Mouse handling | internal/ui | input-edge | 10,16,18 | recent (pass 16 LOW, fixed: a MouseLeftDoubleClick ran editSelected() *before* tview processed the event, so it edited the row left by the preceding single click. `treeNodeAtY` now re-targets the current node to the row under the cursor (public tview APIs only) before editing. Pass-18 canary CLOSED: `treeNodeAtY`'s `idx>=len(visible)` upper-bound guard is now pinned by `TestTreeNodeAtYPastLastNode` (a click one row past the last node must not index `visible[len]`/panic the TUI). Known limitation: the center agenda board has no click-to-select even for single clicks, so a double-click there still edits the current selection — board hit-testing is a follow-up. mouse.go) |
| `:config` reload / $EDITOR flow | internal/ui, internal/config | fault-injection | 10,16 | recent (MED fixed pass 10: $EDITOR shell-split. Pass 16 MED, fixed: editConfigFn's reload path read `cfg, _, _, err := config.Load()`, discarding Load's warning — appearance-typo / world-readable-password warnings were silently lost on `:config` reload. Now combined with buildSyncFn's via `joinWarnings`. Canary CLOSED — `TestServerConfigured` asserts a partial (URL-only/username-only) config returns false) |
| Store write pipeline atomicity (.ics + sidecar temp/rename) under disk fault | internal/store | fault-injection | 10,15 | recent (MED fixed pass 10: content-hash reconcile; delete-half left to safe re-pull. Pass 15 re-swept the sidecar/delete-half under injected partial-write/ENOSPC/rename-fail — the accepted delete-half residual still degrades to a safe re-pull and no new partial-write gap opened; no finding) |
| Store write primitives under concurrent goroutines (mutate / PutIfUnchanged / RestoreDirty / tombstone racing PullRemoteBatch) | internal/store | race | 15 | recent (first direct -race stress at the store layer, previously only exercised via the sync engine; no race/deadlock found. Canary CAUGHT: dropping the `r.Href != ""` guard on the tombstone write fails `TestDeleteNeverSyncedLeavesNoTombstone`) |
| Import ingest path (foreign/bundled external .ics via DownloadAll batch + per-resource GetObject fallback, ImportError collection) | internal/sync, cmd/lazyplanner | fuzz, fault-injection | 15,17 | recent (**pass 17 MED — FIXED**: the Import object loop (import.go) wrote every downloaded object with no empty-href guard, unlike its sibling reconcileCalendar (errEmptyHref). A malformed/hostile server returning empty <href/> elements yields caldav.Objects with Path=="" → resourceFileName("")=="resource.ics" → multiple such objects collided on that one name in PullRemoteBatch and silently overwrote each other, each counted as a successful pull; Import reported N imported while storing 1. Fixed: the import loop now skips obj.Path=="" and records it in res.Skipped with errEmptyHref, mirroring reconcileCalendar. Regression internal/sync/import_emptyhref_test.go. pass 15 MED — ACCEPTED RESIDUAL by owner decision 2026-07-18: a single resource mixing a UID-bearing component with a UID-less one fails to encode as a whole — the ingest healers deliberately never fabricate a UID (pass-3 #7), so go-ical's encoder rejects the ENTIRE resource ("want exactly one UID property, got 0") and import records the whole resource in `res.Skipped`, dropping a perfectly valid UID-bearing sibling. Surfaced (not silent), item-level loss, and reachable only from a malformed foreign/hand-edited `.ics` (RFC 5545 requires a UID). Not fixed because every fix crosses a hard invariant — fabricating a UID reverses a settled decision (churn), per-component encode weakens the iron rule (drops the UID-less component), and the CalDAV transport hands us an already-decoded `*ical.Calendar` so there are no raw bytes to preserve. Revisit if it ever bites a real server. See accepted gaps below) |
| Yank/paste cross-list move & copy rollback | internal/ui | data-loss | 10 | recent (HIGH+MED fixed: per-component isolate/remove) |
| Feature-promise conformance vs main.md/CLAUDE.md | (whole app) | spec-diff | 10,13,17,18 | recent (**Pass 18 MED — FIXED**: main.md:340 promises a `:config` reload "re-parses the account list (picker/status bar update live)", but the reloaded list was discarded — `ConfigReload` carried only Sync/ColorMode/Warning and `applyConfigReload` never updated `a.accounts`/`a.activeAccount` (set once in Run), so a `:config`-added/renamed `[[account]]` stayed invisible in the picker + status bar and unreachable via `:account` until restart. Fixed: `ConfigReload` gained `Accounts`+`ActiveAccount`, `editConfigFn` returns the refreshed names + the running account's (possibly renamed) name, and `applyConfigReload` adopts them on a successful reload — switching is still `:account`'s teardown-rebuild, the active connection still can't be hot-swapped, and a reload error leaves the live list untouched. Repro `internal/ui/configreload_accounts_test.go`. Rest of the v1.1.0 promise set held under spec-diff (teardown-rebuild GC's the old store — no cross-account pointer leak; cache carry-over via unchanged ID derivation; last-active-by-id survives rename; :config never yanks a live store). Prior: 2 MED fixed pass 13: applyMutation (edit form + recur-scoped saves) and reparentSelected H/L now route through PutIfUnchanged — the "no Locate→Put clobber sites remain" invariant is now true again. Pass 17 re-diffed the passes 14/15/16 promises (method-aware redirect policy, heal-set-mirrors-validateComponent, RDATE/EXDATE multi-value independence): the redirect and reconcile promises hold; the RDATE/EXDATE-independence promise is confirmed intact for comma-listed values; the pass-17 tz.go VALUE=PERIOD-IANA-TZID mis-zone in the same resolveDateTime path (recorded as a finding in the Timezone/DST row) is now FIXED) |
| Multi-account config parse (`[[account]]` schema, `[server]` migration rejection, validateAccounts nameless/dup, ResolveActiveAccount fallback, Account lookup, Account.ID cache-namespacing) | internal/config | fuzz | 18 | recent (first audit of the v1.1.0 multi-account TOML parse added after pass 17. **HIGH — FIXED**: `toml.Decode` is O(depth²) on deeply nested inline tables, so a config well under the `maxConfigBytes` 4 MiB read cap hung `Load()` — and thus startup — for minutes to hours with no UI and no error (re-measured this session: 500→25 ms, 1000→100 ms, 2000→331 ms, 4000→1.1 s, clean quadratic; a sub-cap file extrapolates to >1 h). The read cap bounds *bytes*, not decode CPU. Fixed with a deterministic pre-decode guard `checkNestingDepth` that rejects structural `{}`/`[]` nesting past `maxTOMLNestingDepth` (64) before `toml.Decode`; brackets inside strings/comments are skipped so a real config (max nesting ~2) is never falsely rejected. Repro `internal/config/config_decode_bound_test.go` (a 2 s deadline test on a deep config + a deterministic boundary test). The account-name uniqueness / migration / ID-derivation logic itself held under adversarial input — no parse finding there) |
| Global state file (`global.json` LoadGlobal/SaveGlobal, corrupt/missing→zero, atomic temp+rename, capped read, ActiveAccountID round-trip) | internal/state | fault-injection | 18 | recent (first audit of the v1.1.0 cross-account state file. The corrupt/missing→zero fail-safe and atomic write held under injected faults — no finding; a corrupt/partial/oversized global state degrades to zero and never blocks startup. Canary CLOSED (pass 18): `Save`/`SaveGlobal`'s shared 0o600 file-mode contract is now guarded by `TestSaveFilesAre0600` — see below) |
| Account switch-and-rebuild loop (`runTUILoop` persist-active-id-before-open, previous-account fallback on failed switch-open, fatal initial-open, unknown-target clean quit) | cmd/lazyplanner | fault-injection | 18 | recent (first audit of the v1.1.0 switch state machine. Injected store.Open failures exercise the fallback (reopen previous working account) and second-failure-fatal logic; the documented persist-before-open crash-window residual (sub-ms) is accepted. No finding on the loop itself. Canary CLOSED (pass 18): the `components()` --tasks/--both helper in the sibling `calendar.go` is now guarded by `TestComponents` (plus `slugify`/`joinWarnings`) — see below) |
| `:account` command + picker (switchAccount case-insensitive validation, already-active no-op, unknown flash, requestSwitch/RunResult.SwitchAccount, no-accounts path) | internal/ui | input-edge | 18 | recent (first input-edge of the v1.1.0 command handler; picker draw was stress-covered by TestAccountPickerStress. Adversarial names + switch-while-modal states surfaced no *new* command-handler defect, but the input-edge exposed the `:config`-reload account-list staleness recorded in the feature-promise row above — MED, now FIXED) |
| Full `sync-collection` incremental (token delta) | internal/sync | — (deliberately deferred) | — | never |
| go-ical semantic encoder constraints (DTEND/DUE+DURATION, empty VTIMEZONE, VJOURNAL/VFREEBUSY nesting) | internal/model | fuzz (re-encode round-trip) | 10,16 | recent (4 HIGH + 1 MED fixed pass 10: ingest healers. Pass 16 re-fuzz found 2 more HIGH of the *same class* — the pass-10 healer set was component-incomplete — both now fixed: (a) a VTIMEZONE missing TZID, or a STANDARD/DAYLIGHT missing DTSTART/TZOFFSETTO/TZOFFSETFROM, bricked the resource at Encode() — `dropEmptyTimezones`→`dropUnusableTimezones` now strips such an unencodable VTIMEZONE on ingest (owner-approved; a referenced TZID degrades to floating time); (b) a VJOURNAL/VFREEBUSY missing DTSTAMP or carrying duplicate single-valued props — `ensureDTStamp` now runs for VJOURNAL/VFREEBUSY in `healComponentConstraints` and `singleValuedProps` gained CompJournal/CompFreeBusy entries. A missing UID on these components is still not healed (fabricating one would churn sync identity — accepted residual). Regression: `malformed_vtimezone_test.go`, `vjournal_encode_test.go`. **The healer table must mirror go-ical's full encoder.go validateComponent — codified as a Hard-won guardrail so this class does not reopen a third time**) |
| Raspberry Pi target (on-device timing / kiosk) | (hardware) | — | — | never |

## Declared blind spots (not covered by any pass)

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
