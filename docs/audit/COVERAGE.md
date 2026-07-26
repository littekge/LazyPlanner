# Coverage ledger

The living record of which surfaces have been audited, by what method, and when —
the input the `hardening-audit` workflow reads to pick the *least-audited* surfaces
next. Keep it honest: `status` reflects real coverage, and blind spots are listed,
not hidden. See `PROTOCOL.md`.

`status`: **recent** = covered by a strong method in the last pass or two ·
**stale** = audited a while ago or only weakly/indirectly · **never** = no real audit.

Full finding narratives, repros, and commit-by-commit fix detail live in
`passes/PASS-N.md`. This ledger keeps only what a pass-planner needs: what's
covered, how well, how stale, and the standing risk in one or two lines.

| Surface | Package | Methods used | Last pass | Status |
|---|---|---|---|---|
| iCalendar decode/ingest | internal/model | fuzz, heal-on-ingest | 4 | recent |
| Recurrence expansion (read) | internal/model | fuzz, tz/DST sweep, scale-bound | 4,5,8,21,23 | recent (**Pass 23 MED, unfixed** — `StepBudget` is minted per `store` call, not per redraw; `dayItemsForDays`/`selRange` call it once per day, so a 7-day rebuild takes 1.85 s and a 366-day SELECT ≈1 m 40 s on the UI thread. Downstream `LayoutDay` is separately unbounded — see the Day-agenda/time-grid row. **Pass 21 fixed** the earlier per-event-only cap with a shared aggregate `model.StepBudget` (571b7ec) after a 50-event pathological rule froze a redraw for 5 s. Detail: PASS-21.md, PASS-23.md.) |
| Recurrence write-side (mutate/split/advance) | internal/model | fuzz, deep audit, spec-diff, input-edge | 9,14,19,20,21 | recent (**Pass 19 HIGH+MED, fixed (8051ddc)** — `ReanchoredRecurrence`'s monthly nth-weekday re-derivation could emit a rule contradicting the moved anchor, or an out-of-vocabulary `BYDAY=5<wd>`; now re-derives per-move and blocks unrepresentable moves. Pass 21 edge-swept the merged event/todo core across the weekday×position matrix — no new finding. Prior: 2 MED fixed pass 14 (RDATE/EXDATE split partitioning). Detail: PASS-14.md, PASS-19.md, PASS-21.md.) |
| Subtask tree build | internal/model | fuzz, scale | 4,5,17 | recent (pass 17 re-fuzzed `BuildTree` cycle/orphan/deep-chain classification against 12 passes of model evolution — no finding; canary closed, the this-&-future split COUNT-clamp boundary is now guarded.) |
| Quick-add parser | internal/model | fuzz, input-edge | 4,14,19 | recent (**Pass 19 LOW, fixed (acc0c6b)** — the "every &lt;month&gt; &lt;day&gt;" branch lacked the validYMD gate its sibling plain-date path had, so "every feb 30" silently produced a dead-anchored yearly rule; now rejected like the ISO form. Canary escape on `parsePriority`'s numeric 1–9 boundary closed same pass. Prior: MED fixed pass 14 (invalid day-of-month silently rolled forward). Detail: PASS-14.md, PASS-19.md.) |
| Timezone / DST | internal/model | exhaustive sweep, fuzz | 8,14,17 | recent (MED fixed pass 14 — multi-valued RDATE/EXDATE collapsed the series to its base instance; `resolveDateTimeValues` now splits per value. **Pass 17 MED, fixed** — an IANA-TZID `VALUE=PERIOD` RDATE mis-zoned to floating time; `resolveDateTime` gained an IANA-TZID recovery branch. Detail: PASS-14.md, PASS-17.md.) |
| Windows→IANA zone mapping + TZID resolution | internal/model | fuzz | 17 | recent (first direct fuzz of the lookup table + unknown-TZID fallback — no crash/panic; the one functional gap found (IANA-TZID `VALUE=PERIOD` mis-zone) is tracked in the Timezone/DST row.) |
| Color parsing (ParseHexColor / NearestANSI16 / ReadableFg / Luminance) | internal/model | input-edge | 17 | recent (first audit of model/color.go — boundary-swept malformed length/digits/channels, no panic/OOB finding. Distinct from the caldav color-PROPFIND decode audited pass 12.) |
| CalDAV network boundary (response-parse: multiget/PROPFIND/REPORT decode, hand-rolled ListObjectHrefs XML, truncated/oversized bodies, redirects) | internal/caldav | fault-injection, panic-guard, fuzz | 4,7,15,22 | recent (**Pass 22** data-loss-swept write-method conditional semantics (If-Match/If-None-Match, 412 surfacing) — no new finding, but a canary on `normalizeETag`'s weak-validator (`W/`) strip escaped, closed same pass (e7f3625). Pass 15 HIGH fixed: PUT/DELETE followed redirects, silently dropping writes on a 3xx; `NewClient` now installs a method-aware `CheckRedirect`. Detail: PASS-15.md, PASS-22.md.) |
| Sync engine (data-loss / TOCTOU) | internal/sync, internal/store | deep audit, race | 3,11,18 | recent (**Pass 18 HIGH, fixed** — `CommitPush` resurrected a resource deleted mid-push (`cur==nil` treated like `cur==pushed`); `honorMidPushDeleteLocked` now honors the deletion instead. Pass 11 HIGH fixed: `PullRemoteBatch` skips a Dirty resource. Detail: PASS-11.md, PASS-18.md.) |
| Sync reconcile state machine (reconcileCalendar/reconcileReadOnly case matrix, keep-both, Forget, read-only-twin branches) | internal/sync, internal/store | data-loss, race | 13,14,15,19,20,21,22 | recent (**Reopened three times as the "concurrent-write signal has no resource-is-gone case" class**: pass 18 `CommitPush` `cur==nil` HIGH (fixed), pass 19 `pushDelete` 412-resurrect HIGH (fixed, d39853d), pass 20 step-(A) `Forget` HIGH (fixed, 24500e8, via new `store.ForgetIfUnchanged`). **Pass 21** race-stressed the new primitive — no finding. **Pass 22** raced the keep-both/push-vs-pull matrix — no data-loss finding, but a canary on `reconcileReadOnly`'s dirty-discard guard escaped, closed (e7f3625). The deeper reconcile-vs-concurrent-pull matrix beyond these fixed windows is warm again but not fully cleared — a standing re-sweep target. Prior: pass 13 HIGH (degraded-fetch-as-deletion) fixed; pass 14 two fixes (tombstone-clear-on-412, keep-local convergence). Detail: PASS-13.md through PASS-22.md.) |
| CalDAV request-construction (MKCALENDAR/PROPPATCH/DELETE bodies, resolve()/href, color/name validation, idempotency) | internal/caldav | fault-injection | 13,21 | recent (**Pass 21 MED, fixed (ba4428a)** — `SetCalendarProps` returned success on any 207 without parsing the body, so a per-property rejection (403/409 inside the multistatus) silently discarded a rename/recolor; now parses propstats and fails on a positively-identified non-2xx status. Prior: 2 MED fixed pass 13 (DELETE/MKCALENDAR idempotency). Detail: PASS-13.md, PASS-21.md.) |
| Sync concurrency | internal/sync | -race stress | 3,11 | recent (re-run post batching/CTag; no new race; the store-level clobber finding is fixed) |
| CTag incremental short-circuit (skip DownloadAll) | internal/sync | data-loss, fault-injection | 11,16 | recent (pass 16 fault-injected a stale/duplicate/absent/lying server CTag driving the skip decision — no finding; the skip is fail-safe) |
| Background sync goroutines (startPeriodicSync timer + flushOnQuit quit push) | internal/ui, internal/sync | race | 11,16 | recent (pass 16 re-swept under -race against the pass 14–15 write-path changes (CheckRedirect, tombstone-412) — no deadlock/race found) |
| Store filesystem robustness (paths, revert, rollback, load-time stale-temp sweep) | internal/store | deep audit, race, fault-injection | 9,15,22 | recent (**Pass 22** fault-injected the pass-20/21 write primitives (compare-and-remove, tombstone create/advance, atomic temp/rename) under ENOSPC/rename-fail/partial-write — no new finding; a store canary (dropped tombstone guard) was caught. Pass 15 HIGH fixed: the stale-temp sweep could delete a real never-pushed resource whose name happened to contain `.tmp-`; `isStaleTempName` now requires the exact leftover shape `os.CreateTemp` produces. Detail: PASS-15.md, PASS-22.md.) |
| Local disk / config input boundaries | internal/config, internal/state, internal/store | deep audit, size caps, fuzz, fault-injection | 9,13,22 | recent (**Pass 22** first fault-injection of the `password_command` exec path and unreadable/partial config reads — no finding; failures surface cleanly, never hang/crash. Pass 13 fuzzed TOML parse/round-trip — no finding. Detail: PASS-13.md, PASS-22.md.) |
| State-file load/parse (widths/hidden-cals/hour-zoom) | internal/state | fuzz (adversarial values), deep audit, size cap | 9,12,17 | recent (pass 17 re-fuzzed adversarial widths/hidden-cals/hour-zoom against the later calendar-id changes — no parse finding; canary closed: `Load()`'s dropped unmarshal-error check is now guarded.) |
| Quick-field edits (sp/sd) — Locate→Put write | internal/ui, internal/model | data-loss | 12 | recent (HIGH STATUS-flatten + MED COMPLETED-restamp + MED TOCTOU all fixed) |
| Completion toggle (Space) + recurring-todo advance — Locate→Put | internal/ui | data-loss | 12 | recent (MED TOCTOU fixed: PutIfUnchanged) |
| Session undo stack (pushUndo / prev-snapshot restore replay) | internal/ui, internal/store | data-loss | 12 | recent (HIGH undo-of-synced-delete + MED undo-of-synced-edit fixed: RestoreDirty) |
| Calendar-color + display-name PROPFIND parsing (discoverColors/SyncCalendarName) | internal/caldav | fault-injection | 12 | recent (MED href-key encoding mismatch fixed: hrefKey decodes; also fixed the privileges fail-open + CTag miss) |
| `:calendar` command argument parsing (rename/color/hide/show) | internal/ui | input-edge | 12 | recent (no new finding) |
| Bulk-pull batching / scale | internal/store, internal/sync | benchmarks, data-loss | 5,11 | recent (HIGH fixed: PullRemoteBatch no longer clobbers a href-less pull-orphan's local edit) |
| Grab-mode temporal-manipulation state machine (per-nudge commit, snapshot/2-resource revert) | internal/ui | data-loss, input-edge | 11 | recent (MED+LOW x2 fixed pass 11: revert-error surfacing, version-checked commit, HasDue re-check. **Post-v1.3.0 fixed**: an all-scope day-move left a day-pinning `BY*` stale, vanishing the moved event from its own rule; `model.ReanchoredRecurrence` now re-anchors or blocks. Detail: PASS-11.md, PASS-19.md, PASS-20.md, PASS-21.md, PASS-23.md.) |
| SELECT mode + bulk ops + bulk grab (multi-select range derivation, bulk complete/delete/yank-paste/grab, GRAB nested inside SELECT) | internal/ui | data-loss, input-edge | 19,20,22 | recent (**Pass 22 canary escape, closed (e7f3625)** — `drillRange`'s bounds guard could panic the TUI on a bulk-select at a drilled day's terminal index. **Pass 20**: bulk-grab's todo DUE-shift skipped `ReanchoredRecurrence` (fixed, ab6ed06); SELECT's day-range highlight had no 366-day cap unlike the materialization it drives (fixed, 5cf043d). **Pass 19**: `bulkDelete`/`deleteWholeObject` deleted the WHOLE co-resident `.ics`, not just the selected component (HIGH, fixed 40b0803, `model.RemoveComponent`); a pending vim count leaked past a swallowed bulk-op key (LOW, fixed 33d01d3); a canary on `dayInRange`'s inclusive bound closed. v1.5.0 phase-2 key×context matrix landed surface-level fixes here too (still `never` for a deep pass). Detail: PASS-19.md, PASS-20.md, PASS-22.md.) |
| Recurrence-edit UI orchestration (scope picker + this-&-future split/detach) | internal/ui | data-loss | 11 | recent (HIGH x2 + MED fixed: commitSplit/commitDetach rollback; DetachTodoOccurrence preserves props) |
| v1.3.0 recurrence Custom repeat sub-form (recurcustom.go: count/until/unit/monthly-by/ends field validation) | internal/ui, internal/model | input-edge | 22 | recent (**Pass 22 MED, fixed (c6f79f0)** — "Ends on date" silently dropped the end-day occurrence for TIMED items (UNTIL stayed at midnight) while all-day items were inclusive; `readCustomRecur` now anchors UNTIL at the selected date plus the anchor's own time-of-day. Later found not to reach production — see the Feature-promise row (pass 23). Detail: PASS-22.md.) |
| UI draw paths (custom widgets) | internal/ui | display stress | 6,14 | recent (pass 14 re-swept the newer widgets — agendaboard/itemforms — no crash/freeze; no finding) |
| UI input handlers (keys/chords/commands) | internal/ui | deep audit, input-edge | 9,13,14 | recent (pass 14 input-edged raw non-command keypress/chord dispatch — no finding. v1.5.0 phase-2 key×context matrix (`docs/audit/specdiff/MATRIX.md`, 529 cells) landed 6 related fixes: `q` closes the account/color pickers, undo preserves calendar drill state, and a shared `motionArrow`/`modalMotionKey` refactor removed duplicate key-mapping.) |
| CLI wiring | cmd/lazyplanner | deep audit, input-edge | 9,16 | recent (pass 16 input-edged the flag.FlagSet subcommand dispatch — 1 MED + 1 LOW, both fixed via a shared `parseFlags` helper distinguishing `-h`/`--help` from a bad flag. Canary closed: `TestConnFlagsClientRequiresAllCredentials`.) |
| Mouse handling | internal/ui | input-edge | 10,16,18 | recent (pass 16 LOW, fixed: double-click edited the row left by the preceding single click; `treeNodeAtY` re-targets to the row under the cursor first. Canary on its upper-bound guard closed pass 18. v1.5.0 gap-closer added agenda-board click-to-select (`agendaBoard.itemAtY`, guarded by `internal/ui/agendaclick_test.go`), closing the earlier "board has no hit-testing" residual. Detail: PASS-16.md, PASS-18.md.) |
| `:config` reload / $EDITOR flow | internal/ui, internal/config | fault-injection | 10,16 | recent (MED fixed pass 10: $EDITOR shell-split. Pass 16 MED, fixed: `:config` reload discarded `Load`'s appearance/password warnings; now combined via `joinWarnings`. Canary closed.) |
| Store write pipeline atomicity (.ics + sidecar temp/rename) under disk fault | internal/store | fault-injection | 10,15 | recent (MED fixed pass 10: content-hash reconcile; delete-half left to safe re-pull, see below. Pass 15 re-swept the sidecar/delete-half under injected partial-write/ENOSPC/rename-fail — no new gap.) |
| Store write primitives under concurrent goroutines (mutate / PutIfUnchanged / RestoreDirty / tombstone racing PullRemoteBatch) | internal/store | race | 15,20,21 | recent (**Pass 21** race-stressed the new `ForgetIfUnchanged`/`removeLocked` compare-and-remove primitives against a concurrent pull/edit — no race/lost-update finding. **Pass 20** data-loss-swept the conflict-resolution `PutRemote` writes — reached only from a synchronous user action, not a racing pull, so no store-layer finding (the racing-pull data-loss lives one layer up — see Sync-reconcile row). Pass 15: first direct -race stress at the store layer — no race found; canary caught a dropped tombstone guard. Detail: PASS-15.md, PASS-20.md, PASS-21.md.) |
| Import ingest path (foreign/bundled external .ics via DownloadAll batch + per-resource GetObject fallback, ImportError collection) | internal/sync, cmd/lazyplanner | fuzz, fault-injection | 15,17 | recent (**Pass 17 MED, fixed** — the Import loop had no empty-href guard unlike its `reconcileCalendar` sibling; a hostile server returning empty `<href/>` collided multiple objects onto one filename. Pass 15 MED — accepted residual: a resource mixing UID-bearing/UID-less components fails to encode and is skipped — see Accepted residuals below. Detail: PASS-15.md, PASS-17.md.) |
| Yank/paste cross-list move & copy rollback | internal/ui | data-loss | 10,19,20 | recent (**Pass 20 MED, fixed (60f1191)** — `moveSubtreeOps` hard-coded the caller's source calendar and treated the dest Put as a guaranteed fresh create; a subtree spanning collections via cross-collection `RELATED-TO` could clobber or permanently delete the destination-side child. Now uses each member's own located calendar and rewrites in place when it already exists at the destination. **Pass 19**: undo of a co-resident multi-root move lost a root (HIGH, fixed 9de7ecc, undo ops now coalesce by resource); single-item `reparentTo` still used a bare Locate→Put (MED, fixed 18fbca3) — third reopening of the bare-Put-clobber class, see the Systemic-follow-up section below. Prior: HIGH+MED fixed pass 10. Detail: PASS-10.md, PASS-19.md, PASS-20.md.) |
| Feature-promise conformance vs main.md/CLAUDE.md | (whole app) | spec-diff | 10,13,17,18,20,23 | recent (**Pass 23**: re-diffed the v1.5.0 phase-2/3 work and five recent behavior-changing fixes — 1 HIGH + 2 MED, unfixed at audit time and resolved as part of the pass-23 arc (see Resolution section). HIGH: the pass-21 heal-set fix was still allow-by-default for containers go-ical has no case for (`X-*`/`VAVAILABILITY`/nested `VCALENDAR`) — fourth reopening of the go-ical heal-set class, see that row. MED: the pass-21 aggregate `StepBudget` doesn't bound the real per-day redraw loops (see Recurrence-expansion row). MED: the pass-22 timed-UNTIL fix never reaches production because both `anchorFn`s hand it a midnight-only date (see the Custom-repeat-sub-form row). **Pass 20 MED, fixed (c16ea6c)**: a bare-frequency recurring TASK ("daily") got no DUE anchor and could never be completed; fixed in the UI caller, not the model, to avoid making bare-frequency events snap to today. **Pass 18 MED, fixed**: `:config` reload didn't refresh the live account list. Pass 17 re-diffed passes 14/15/16 — held except the tz.go VALUE=PERIOD mis-zone (fixed same pass). Prior: 2 MED fixed pass 13 (`applyMutation`/`reparentSelected` bare-Put). Detail: PASS-13.md, PASS-17.md, PASS-18.md, PASS-20.md, PASS-23.md.) |
| Multi-account config parse (`[[account]]` schema, `[server]` migration rejection, validateAccounts nameless/dup, ResolveActiveAccount fallback, Account lookup, Account.ID cache-namespacing) | internal/config | fuzz | 18 | recent (first audit of the v1.1.0 multi-account TOML parse. **HIGH, fixed** — `toml.Decode` is O(depth²) on deeply nested inline tables, hanging startup for minutes-to-hours on a config well under the byte cap; `checkNestingDepth` now rejects structural nesting past 64 levels before decode. Account-name uniqueness/migration/ID-derivation held under adversarial input. Detail: PASS-18.md.) |
| Global state file (`global.json` LoadGlobal/SaveGlobal, corrupt/missing→zero, atomic temp+rename, capped read, ActiveAccountID round-trip) | internal/state | fault-injection | 18 | recent (first audit of the v1.1.0 cross-account state file — corrupt/missing degrades to zero and never blocks startup; no finding. Canary on the shared 0o600 file-mode contract closed. Detail: PASS-18.md.) |
| Account switch-and-rebuild loop (`runTUILoop` persist-active-id-before-open, previous-account fallback on failed switch-open, fatal initial-open, unknown-target clean quit) | cmd/lazyplanner | fault-injection | 18 | recent (first audit of the v1.1.0 switch state machine. Injected `store.Open` failures exercise the previous-account fallback and second-failure-fatal logic; the persist-before-open crash window (sub-ms) is an accepted residual. No finding. Canary closed: `components()`/`slugify`/`joinWarnings` now guarded.) |
| `:account` command + picker (switchAccount case-insensitive validation, already-active no-op, unknown flash, requestSwitch/RunResult.SwitchAccount, no-accounts path) | internal/ui | input-edge | 18 | recent (first input-edge of the v1.1.0 command handler; picker draw stress-covered separately. Adversarial names + switch-while-modal states surfaced no new command-handler defect, but exposed the `:config`-reload account-list staleness recorded in the Feature-promise row — since fixed.) |
| Full `sync-collection` incremental (token delta) | internal/sync | — (deliberately deferred) | — | never |
| go-ical semantic encoder constraints (DTEND/DUE+DURATION, empty VTIMEZONE, VJOURNAL/VFREEBUSY nesting) | internal/model | fuzz (re-encode round-trip), spec-diff | 10,16,21,23 | recent (**Reopened a fourth time.** Pass 23 HIGH — the pass-21 fix covered only containers `checkComponent` has a case for; `stripForbiddenChildren` was still allow-by-default, so a phantom nested under an unknown container (`X-*`/`VAVAILABILITY`/nested `VCALENDAR`) still bricked the whole resource on encode. Fixed as part of the pass-23 arc (0151ebd) — see Resolution section; also now guarded by a go/ast drift tripwire (`encoderdrift_test.go`) instead of manual re-diff. **Pass 21 HIGH, fixed (c8eae4f)** — third reopening: the required-prop/mutual-exclusion heals ran top-level-only, so a phantom component nested inside a VALARM/STANDARD/DAYLIGHT (no `allowedChildren` entry) bricked the resource; completed `allowedChildren` so `stripForbiddenNesting` removes it before go-ical's recursive `checkComponent` sees it. **Pass 16**: 2 more HIGH of the same class, fixed — an unencodable VTIMEZONE now stripped on ingest (owner-approved); VJOURNAL/VFREEBUSY DTSTAMP + dedupe healed. Prior: 4 HIGH + 1 MED fixed pass 10 (original ingest healers). A missing UID is deliberately never healed (would churn sync identity). Hard-won guardrail in CLAUDE.md now requires the heal set to mirror go-ical's `checkComponent` at its recursion depth. Detail: PASS-10.md, PASS-16.md, PASS-21.md, PASS-23.md.) |
| RRULE decomposition — `RecurSpecFromRule`/`decodeMonthly`/`decodeYearly`/`nthMatchesAnchor` (foreign rule → editable RecurSpec) | internal/model | fuzz | 23 | recent (**Pass 23 — first audit of this surface (the decode twin of the reanchor write side)** — 1 HIGH + 1 MED, unfixed at audit time and resolved as part of the pass-23 arc (see Resolution section). HIGH: `decodeYearly` validated BYMONTHDAY independently of BYMONTH, so `FREQ=YEARLY;BYMONTHDAY=15` (RFC-legal, means every month) was declared representable as a bare yearly rule — collapsing 73 occurrences over six years to 7 on re-serialization, and vanishing the event entirely after a grab day-move (the existing `TestRecurSpecFromRuleAnchorConsistent` pinned the wrong belief). MED: a negative `INTERVAL` was silently dropped rather than blocking the move, though rrule-go rejects it outright. Detail: PASS-23.md.) |
| Conflict-resolution UI orchestration (`conflicts.go`: captured Conflict snapshot, populate/refresh, keep-local/keep-server wiring) | internal/ui, internal/store | data-loss | 23 | recent (**Pass 23 — first audit at the UI layer** — 1 HIGH, unfixed at audit time and resolved as part of the pass-23 arc (see Resolution section). `store.ResolveKeepLocal`/`MarkConflict` apply their entire in-memory mutation before `writeSidecar` and don't revert on a write failure — unlike every other store write path — so a sidecar-write fault leaves the store believing the conflict is resolved (ETag advanced, Dirty set) while the UI reports failure and never refreshes; the next sync's If-Match then silently overwrites the server's diverging version. Detail: PASS-23.md.) |
| Sidecar metadata **parse** (`readSidecar`: dirty/href/etag/hash/conflict/tombstone JSON from disk → reconcile inputs) | internal/store | fuzz | 23 | recent (**Pass 23 — first audit of the sidecar READ side** — 2 HIGH + 1 MED, unfixed at audit time and resolved as part of the pass-23 arc (see Resolution section). HIGH: `readSidecar`'s decode is all-or-nothing — one wrong-typed field discards every Dirty/ETag/Href/tombstone/read_only flag for the whole calendar, and the crash-window heal can't rescue it. HIGH (path traversal): a tombstone's name is an unvalidated JSON key that reaches `filepath.Join` unguarded — a `../../../..` key made sync write a server VCALENDAR outside the cache root, and the escape persists back into the sidecar. MED: the conflict stash isn't byte-lossless — encoding/json rewrites invalid UTF-8 to U+FFFD, corrupting a non-UTF-8 `.ics` stashed as the server's version. Detail: PASS-23.md.) |
| `:goto` / `:view` / `:search` command-argument parsing + search navigation (`command.go` cmdGoto/cmdView, `search.go` runSearch/searchNext/matchIndices) | internal/ui | input-edge | 23 | recent (**Pass 23 — first audit** — 3 LOW, unfixed at audit time and resolved as part of the pass-23 arc (see Resolution section). A whitespace-only query slipped past both empty-query guards and matched every row; `searchNext` kept a positional index into a recomputed match list so `n` could stall or skip after a background sync; user-typed query/command text was concatenated unescaped into the dynamic-color status bar, so a bracket run made the message unreadable. Detail: PASS-23.md.) |
| Day agenda + time-grid layout (`model.DayAgenda`, `model.LayoutDay` overlap/lane packing, `layoutEnd`; `ui.splitOccs`, `timeGridView.drawBlock`) | internal/model, internal/ui | input-edge | 23 | recent (**Pass 23 — first direct audit** — 1 HIGH + 2 MED, unfixed at audit time and resolved as part of the pass-23 arc (see Resolution section). HIGH: `LayoutDay`'s lane-assignment is a linear scan per occurrence, Θ(n²) when a day's occurrences all overlap — 30k same-day occurrences (well inside the recurrence step budget) laid out in 1.14s, 100k in 9.7s, on every Draw and keypress; the recurrence bound doesn't protect the layout stage downstream. Two sibling quadratics found during the fix (`navCells`, `Draw`'s `inSelRange`) — see Pass 23 Resolution. MED: a zero-length all-day occurrence is dropped from the week/day band (a loop that never runs when `End == Start`) but still shows in the month grid and drill list — selectable/deletable but invisible. MED: lane blocks aren't clamped to their column or pane rect, so concurrent events can paint into the next day's column or past the primitive's own rect. Detail: PASS-23.md.) |
| Raspberry Pi target (on-device timing / kiosk) | (hardware) | — | — | never |

## Declared blind spots and accepted residuals

**Never audited**: Full `sync-collection` incremental sync (deliberately deferred
feature — the CTag short-circuit stands in as the fail-safe; audit once
implemented) and the Raspberry Pi target on real hardware (needs physical
hardware — the sole known-never surface with product risk).

**Accepted residuals** (owner-approved design decisions, not gaps to close):

- Import of a resource mixing a UID-bearing with a UID-less component (pass-15
  MED) fails the whole resource at encode and is skipped — surfaced via
  `res.Skipped`, not silent. Every fix crosses a hard invariant: fabricating a
  UID reverses a settled decision, per-component encode weakens the iron rule,
  and no raw bytes survive the CalDAV transport's decode. Revisit if a real
  server ever produces one. See PASS-15.md.
- A missing UID on any component is deliberately never healed (pass-16) —
  fabricating one would churn sync identity, the same reasoning as above.
- Live two-account end-to-end switch-and-sync was owner-verified manually
  against a live CalDAV server (2026-07-22, recorded in main.md's v1.1.0 build
  record) after pass 18 deferred it as unverifiable while the server was
  offline; headless unit coverage is the automated guard going forward.
- Delete-half of the write-atomicity finding: a crash between an `.ics` delete
  and its tombstone write re-pulls the item on the next sync — safe and
  recoverable. Synthesizing a tombstone from a missing-`.ics`-with-href would
  risk deleting server data whenever a `.ics` merely went missing, so the safe
  re-pull is kept by design.

**Open canary items carried from pass 23** (not yet closed — see the Mutation
canary escape log below for full detail): the `NewSeriesFrom` RECURRENCE-ID
carry-forward escape, and the `internal/store` `CommitPush` surface-local hole
(caught only by a cross-package `internal/sync` test).

**Coverage staleness carried from pass 23** (not re-swept that pass): `internal/caldav`
write paths for the bare-write/resource-is-gone class (second consecutive pass
unswept — see the Systemic-follow-up section); direct iCalendar decode/ingest
fuzz (last pass 4); quick-add grammar (19); timezone/DST + Windows→IANA (17);
colour parsing (17); `BuildTree` (17); pass-11/12-era UI data-loss surfaces
(single-item grab, recurrence-edit scope picker, quick-field sp/sd, completion
toggle, undo stack, bulk-pull batching); pass-18-era multi-account surfaces; UI
display stress (14, including `render.go`'s `calItems`, changed by the pass-21
StepBudget work) and mouse handling (18); `internal/ui/colorpicker.go` and
`help.go`, which still have no ledger row. Race and fault-injection methods
were not exercised at all in pass 23 (last run pass 22).

Pass 23's 15 confirmed findings (6 HIGH / 6 MED / 3 LOW) were all fixed the
same session — see "Pass 23 — Resolution" below for the commit-by-commit
table, and the surface rows above for per-surface detail.

## Systemic follow-up — the Locate→Put no-version-check pattern

This class reopened three times before being closed by an exhaustive sweep.
Pass 11 fixed grab; pass 12 fixed three more sites (quick-field sp/sd, Space
completion, recurring-todo advance) and declared it "structurally closed".
**Pass 13's spec-diff proved that false** — the sweep found two remaining
sites (`applyMutation`, `reparentSelected`) sharing the exact TOCTOU clobber
shape, both fixed, with a full `internal/ui` sweep. It reopened a third time
in pass 19 (`reparentTo`), triggering another full `internal/ui` sweep
(`grab.go`, `recur_edit.go`, `yankpaste.go`) and a CLAUDE.md Hard-won-guardrail
update. **Neither sweep crossed package boundaries** — untested peer write
paths in `internal/store`, `internal/model`, and `internal/caldav` were never
audited for the same two shapes (bare-Put clobber, and the sibling "no
resource-is-gone case" reconcile class) as of pass 19; that cross-package
sweep is a named target, not yet closed. See PASS-13.md, PASS-19.md, and the
CLAUDE.md Hard-won guardrails.

## Mutation canary escape log

Each pass injects known bugs into the surfaces it audited and checks whether
the regression suite catches them — a measurable, independent check on test-net
health (rule 6 of `PROTOCOL.md`). History below is condensed to outcome + root
cause; only currently-open escapes carry full detail. All commit hashes and
repro/test file names are in the cited `PASS-N.md`.

**Pass 23** — 4 canaries: 1 open escape, 1 caught-but-surface-local-hole, 2 caught cleanly.
- **OPEN** — `internal/model/recur_edit.go` `NewSeriesFrom`'s RECURRENCE-ID
  carry-forward filter (`t.Unix() <= occ.Unix()` → `<`) passed the whole repo.
  A real defect, not equivalent: on a this-&-future split at an occurrence that
  already had a per-instance override, the stale override is copied into the
  new series and outranks the master's freshly-mutated instance — the user's
  edit silently vanishes. Root cause: the only override-carry-forward test
  never exercises the at-the-split-instant boundary.
- **OPEN (surface-local hole)** — `internal/store/remote.go` `CommitPush`'s
  copy-on-write pointer-identity check, weakened to a value comparison, passed
  `go test ./internal/store/` in full (41 tests green); only one test in the
  whole repo catches it, one package away (`internal/sync`
  `TestSyncPushDoesNotClobberConcurrentEdit`). The store package's own coverage
  of "concurrent writes are version-checked" rests entirely on that external
  test — none of the package's own CommitPush tests exercise the mid-push-EDIT
  path (only mid-push-DELETE).
- Caught cleanly: `timegridview.go` `navCell.overlaps`'s half-open boundary;
  `timegrid.go` `LayoutDay`'s lane-reuse boundary (two independent failures).

**Pass 22** — 4 canaries: 3 escaped, all closed (e7f3625); 1 caught cleanly.
- `normalizeETag`'s weak-validator (`W/`) prefix strip; `drillRange`'s
  terminal-index bounds guard (could panic the TUI); `reconcileReadOnly`'s
  dirty-discard OR-vs-AND guard. All three were test-coverage holes (code
  correct, path untested); each now has a boundary test verified to fail under
  its exact mutation.
- Caught cleanly: `store.remove`'s never-synced tombstone guard.

**Pass 21** — 4 canaries: 1 genuine escape, closed (e8ec765); 1 reported escape
was a false positive from a stale worktree (re-verified caught on the current tree).
- `caldav.PutObject` didn't accept a 200 OK as a successful PUT (some real
  servers return it); closed via `TestPutObjectAccepts200OK`.
- Caught cleanly: the both-sides-changed conflict guard in `reconcileCalendar`;
  `store.remove`'s tombstone guard (inverted).

**Pass 20** — 4 canaries: 2 escaped, both closed (d81d432); 2 caught cleanly.
- `weekdayStrip.moveCursor`'s upper clamp (off-by-one, could index one past the
  7-day strip); `parseTimeHalf`'s 24-hour ceiling (accepted an invalid
  `24:00`). Both test-coverage holes, now closed.
- Caught cleanly: `store.remove`'s tombstone guard; `downloadResilient`'s
  degraded-fetch-vs-deletion guard.

**Pass 19** — 3 canaries: 2 escaped, both closed (2026-07-24); 1 caught cleanly.
- `dayInRange()`'s inclusive upper boundary (a draw-path-only helper, never
  tested); `parsePriority`'s numeric 1–9 upper bound (only `!high`/`!1` were
  ever tested). Both closed.
- Caught cleanly: `reconcileCalendar` step B's tombstone guard.

**Pass 18** — 4 canaries: all 4 escaped, all closed (2026-07-21).
- `permissionWarning()`'s loose-permission mask (missed a group-readable-only
  config); `state.Save()`'s file mode (untested); `components()`'s `--tasks`
  branch (zero coverage on CLI helpers); `treeNodeAtY`'s upper bound (off-by-one,
  could panic the TUI). All closed with boundary tests.

**Pass 17** — 4 canaries: all 4 escaped, all closed (2026-07-18).
- `reconcileReadOnly`'s degraded-download guard (read-only twin of an already-
  covered read-write path); `NewSeriesFrom`'s future-series COUNT clamp;
  `DayAgenda`'s todo-due upper bound (twin of a pass-14 escape); `state.Load()`'s
  dropped unmarshal-error check (a type-mismatch-on-a-later-field case). All closed.

**Pass 16** — 4 canaries: 2 escaped, both closed; 2 caught cleanly.
- `Server.Configured()`'s AND-vs-OR guard; `connFlags.client()`'s
  credential-required guard (CLI had zero direct tests). Both closed.
- Caught cleanly: `calendarview.go`'s `drawDayItems` off-by-one;
  `reconcileCalendar`'s conflict-detection inversion.

**Pass 15** — 3 canaries: 1 escaped, closed; 2 caught cleanly.
- `ListObjectHrefs`'s nested-collection filter (fixture never included a
  nested sub-collection). Closed.
- Caught cleanly: `store.remove`'s tombstone guard; `reconcileCalendar` step
  B's tombstone guard.

**Pass 14** — 3 canaries: 1 escaped, closed; 2 caught cleanly.
- `DayAgenda`'s todo due-time lower bound (midnight exactly). Closed.
- Caught cleanly: `grabNudge`'s J/K zero-duration guard; the sync CTag-cache guard.

**Pass 13** — 4 canaries: all 4 escaped, all closed (2026-07-16).
- `LayoutDay`'s cluster-flush/lane-reuse boundary; the sync CTag-cache guard
  (a per-resource failure could cache anyway); `DeleteObject`'s empty-ETag
  `If-Match: *` fallback; `config.Load`'s read-size cap. All closed.

## Pass 11 & Pass 12 — resolved (2026-07-15)

Pass 11: 7 findings (3 HIGH data-loss on the shared "multi-write op with no
rollback" class, 2 MED, 2 LOW) plus both canary holes — all fixed with
regression tests; full gate + `-race` green. See PASS-11.md and the Grab-mode
/ Recurrence-edit-UI / Sync-engine rows above.

Pass 12: 7 findings (2 HIGH, 5 MED) on the Locate→Put no-version-check pattern
outside grab, the undo stack, calendar-color/name PROPFIND decode, and
state-file/`:calendar`-arg fuzz — all fixed with regression tests; all 3
canary holes closed. See PASS-12.md and the Quick-field-edits /
Completion-toggle / Session-undo-stack / Calendar-color-PROPFIND rows above.

---

> The workflow updates this table and this list at the end of each pass.
> Hand-edits are welcome — it's a plain table on purpose.

## Pass 23 — RESOLUTION (2026-07-25)

All 15 confirmed findings fixed repro-first, plus the escaped canary, two
coverage holes, and **four defects the audit did not find**. Full gate green
(`go build`, `go test ./...`, `vet`, `staticcheck`, `gofmt`), additionally
verified across four timezones (UTC / New_York / Kolkata / Kiritimati). 18
commits. Full detail: `docs/audit/passes/PASS-23.md` § Resolution.

### Findings → commits

| Sev | Finding | Commit |
|-----|---------|--------|
| HIGH | Tombstone keys escape the cache root | `0a36ef3` |
| HIGH | Corrupt sidecar discards all sync state | `d55c1f3` |
| HIGH | Failed resolve still resolves in memory | `daaef8d` |
| HIGH | `allowedChildren` allow-by-default (4th reopening) | `0151ebd` |
| HIGH | `YEARLY;BYMONTHDAY` without BYMONTH | `33b3f42` |
| HIGH | `LayoutDay` O(n²) — **three** quadratics, not one | `1ec474d` `524c6e4` `311139c` |
| MED | StepBudget minted per store call, not per redraw | `e236972` |
| MED | Pass-22 timed-UNTIL never reaches production | `836bc6c` |
| MED | Negative `INTERVAL` accepted as representable | `33b3f42` |
| MED | Conflict stash not byte-lossless | `b6a105b` |
| MED | Lane bleed past column / pane rect | `a669f19` |
| MED | Zero-length all-day dropped from the band | `d1750ce` |
| LOW ×3 | Blank search · stale `searchIdx` · unescaped status text | `517eacc` |

### Test net

| Item | Commit |
|------|--------|
| Escaped canary — `NewSeriesFrom` split-instant boundary | `96179a2` |
| Hole — `internal/store` blind to a `CommitPush` lost update | `96179a2` |
| Hole — four hand-mirrored go-ical tables untested → **go/ast drift tripwire** | `96179a2` |
| Tag-escape class — 9 sites beyond the 5 reported | `bb76bd1` |
| Zone-dependent tests (green in CI's UTC, red east of it) | `bb76bd1` |

### Found by the fix arc, not the audit

| Defect | Commit |
|--------|--------|
| All-day "Ends on date D" dropped D — **two** bugs whose signs flip with the UTC offset, across four sites; recurring-todo twin marked itself done. **Falsifies the pass-22 close-out.** | `e079e50` |
| `navCells` per-keypress quadratic | `524c6e4` |
| `Draw`'s `inSelRange` quadratic (only with a SELECT range open) | `311139c` |
| Two zone-dependent tests | `bb76bd1` |

### Surface status changes

- **go-ical heal set / `checkComponent` mirror** — now **guarded by the gate**, not by manual re-diff:
  `internal/model/encoderdrift_test.go` parses the vendored encoder with go/ast and fails on drift.
  This is the first pass where this class is machine-checked; it had reopened four times, and the manual
  step is what failed in three of them. *Not* covered: `allowedChildren` (go-ical's nesting rules live in
  ad-hoc `if` guards, not an extractable table).
- **Day agenda + time-grid layout** — `never` → `recent`, and now benchmark/growth-ratio guarded at three
  levels (model lane packing, `navCells`, `inSelRange`).
- **Conflict-resolution store paths** — `never` → `recent`; UI orchestration half still uncovered.
- **Sidecar parse** — `never` → `recent`; salvage + quarantine + byte-lossless stash.

### Carried residual → Pass 24 targets

1. ~~UNVERIFIED PRODUCT-BUG LEAD — highest value.~~ **RESOLVED (2026-07-26)** — located by recovering the
   killed agent's transcript, not by re-auditing. It was **three linked recurrence-anchor defects, not
   one**, all rooted in `BY*` being derived from the user's local anchor while `DTSTART`/`DUE` was
   serialized in UTC (RFC 5545 evaluates `BY*` in the anchor's own zone): (a) the lead itself — pressing
   Save on a recurring item's edit form **without touching the Repeat dropdown** silently rewrote the rule
   (`BYDAY=MO`→`BYDAY=TU`), shifting the series a day and dropping orphaned overrides, in both the event
   and todo forms; (b) a New York user creating a Tue 20:00 weekly meeting via the "Weekly on Tue" dropdown
   got a series firing every **Monday**, whose first occurrence was not the event's own start; (c) that
   series drifted 20:00 EDT → 19:00 EST after the November DST transition. Fixed across
   `docs/superpowers/plans/2026-07-26-tzid-anchored-recurrence.md`'s five tasks (commits `1bbd338`, `c156480`,
   `bb11b75`, `5c63075`, `89ba317`, `9299ec3`, `86f594f`, `680f10a`) by writing a recurring item's anchor as
   local-time-with-`TZID` plus a generated `VTIMEZONE`, gated so an edit that leaves the rule alone never
   re-anchors. Residuals carried forward: already-UTC-anchored items keep wrong-day behavior until next
   rule edit; a Windows host without `$TZ` still writes UTC anchors; editing an Outlook-authored series now
   rewrites its Windows TZID to the IANA spelling.
2. ~~OPEN REGRESSION (HIGH, arc-introduced 2026-07-26) — a recurring item loses the day its zone's DST
   change starts on.~~ **RESOLVED (2026-07-26, `47dab30`)** — and **wider than recorded on three axes**.
   The root cause is in rrule-go's day enumeration, not `time.Date` at the anchor: it derives each
   occurrence's date as `firstyday.AddDate(0, 0, i).Date()` on local Jan-1 **midnight**, so a missing
   midnight collapses the date into the previous day, the instant duplicates the previous occurrence, and
   rrule-go's own `Set.Iterator` drops it as a duplicate. Therefore (a) **11 zones lose a day**, not 3 —
   add Coyhaique, Punta_Arenas, Palmer, Scoresbysund, Sao_Paulo, Asuncion, Campo_Grande, Cuiaba; (b) it is
   **independent of the anchor's time of day** (firstyday is always midnight) and of frequency, so it hit
   recurring **events on the read path** too, not only the todo advance; (c) eight further midnight-gap
   zones (Cairo, Beirut, Amman, Damascus, Gaza, Hebron, Tehran, Casey) normalize *forward* and never lost
   the day. Fixed by expanding in wall-clock space and resolving back into the real zone
   (`internal/model/wallclock.go`), plus gap-safe decoding of DATE / zone-less DATE-TIME values — without
   the second half the app wrote `DUE;VALUE=DATE:20260308` and immediately read it back as 03-07. Verified
   over all 485 IANA zones: recovered gap days are the only day-level difference, no instant-level
   differences elsewhere. Guards `internal/model/dstgap_test.go`, `internal/ui/dstgap_uipath_test.go`.
   **Deliberately not changed**: a 02:00-anchored series in an ordinary spring-forward zone still reads
   back an hour earlier, as today. More correct handling exists but would move existing events in nearly
   every DST zone — an owner decision, not a side effect of this fix.
3. ~~OPEN REGRESSION (MED, arc-introduced 2026-07-26) — `store` and the UI disagree about the local zone.~~
   **RESOLVED (2026-07-26, `7fca8a2`)**. Sizing (as the owner asked) changed the fix's shape: there are
   **nine** divergent decode sites, not the one recorded — `internal/store/{store.go,conflict.go}`,
   `internal/sync/sync.go` ×4, `internal/sync/import.go`, `internal/model/edit.go` ×2. Threading a location
   would touch 3 production + ~104 test call sites across 48 files and leave nine places free to drift, so
   `run()` installs `config.LocalZone()` as the process `time.Local` before any dispatch. Second half found
   while verifying the first: making them agree was not enough, they agreed on the **wrong** zone —
   `LocalZone()` resolved `$TZ` with `LoadLocation` alone, so `TZ=` empty, `TZ=:Asia/Tokyo` and
   `TZ=/abs/path` all fell through to `/etc/timezone` and silently overrode an explicit setting. All three
   confirmed divergent before and agreeing after. Guards `cmd/lazyplanner/processzone_test.go`,
   `internal/config/zone_test.go`.
4. **Pre-existing (MED) — local-midnight construction is unsafe in DST-gap zones, at *day-bucketing*.**
   The *decode* half of this was fixed with item 2 (a DATE value now names its own day). Still open:
   `model.DayStart` and quick-add's relative dates build local midnight with `time.Date`, so in the 11 gap
   zones "tomorrow" can resolve to today and a day window is shifted an hour at its edges. Not
   arc-introduced; fixing it means reworking `DayStart` + `AddDate(0,0,1)` day-window semantics across the
   calendar views, judged too wide for a pre-release fix.
5. `internal/caldav` write paths — unswept for the bare-write class, **second consecutive pass**.
6. **Race and fault-injection not exercised at all** this pass (last run pass 22).
7. `go test -race ./internal/model/` fails on `TestAggregateRecurrenceCapBounded` — a pass-21 absolute
   wall-clock budget that `-race` overhead exceeds. Pre-existing (fails at `ba274c1`), not a data race,
   not part of the official gate. Convert to the growth-ratio style adopted this pass.
8. A full refresh still mints ~4 budgets (bounded constant, no longer day-scaled). Write-side `safeAfter`
   has no aggregate budget — bulk grab over N recurring items is N × 1M steps, the same class on the
   write path. Two pathological events still exhaust a redraw's 2M ceiling and starve later events.
9. ~~`internal/store` decodes with hard-coded `time.Local`.~~ **Resolved with item 3** — `time.Local` is
   now the app's own zone process-wide, so the hard-coding is no longer a divergence. Still open:
   `internal/ui/conflicts.go` does not refresh on a failed resolve; `model.Decode` rejects a DTSTART-less
   VEVENT that RFC 5545 permits when the VCALENDAR carries METHOD.
10. Decomposer asymmetry: `FREQ=WEEKLY;BYDAY=TU,TH` with a Monday anchor is accepted though the anchor is
   outside its own set; monthly/yearly reject the equivalent.
11. ~~Owner decision outstanding: unparseable sidecar → read-only calendar.~~ **SETTLED (2026-07-25) —
   reverted by the owner.** Locking a calendar because its sidecar is unreadable is wrong for an
   offline-first app: that is precisely when the user is working from the cache with no server to ask.
   Only the server's recorded privilege makes a calendar read-only. Data protection is unaffected —
   unrecovered resources still load Dirty and the original bytes are still quarantined. A future
   hardening pass must not "restore" the lock as an improvement; the reasoning is pinned in
   `TestUnsalvageableSidecarTreatsStateAsUnknown`.
12. Surfaces deliberately skipped: ~37 of ~53 inventoried surfaces unexamined this pass.

### Note for the next pass

Nine resolved items were **not** in the audit's finding list. Five of six targets were `never`-audited
rows and all five produced findings, including a HIGH that wrote outside the cache root. **`never` rows
are high-yield, not low-yield.** The pass-19→22 downward severity trend described repeatedly-hardened
surfaces, not the codebase. `more_passes_recommended` stands.
