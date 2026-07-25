# Pass 19 — v1.4.0 SELECT/bulk-ops + v1.3.0 recurrence-rewrite + the phase-3 rollback/reconcile clobber gaps

- **Date:** 2026-07-24
- **Prior pass:** Pass 18 (v1.1.0 multi-account: config-decode startup hang + `:config` account-list not live + sync-core mid-push-delete resurrection) — HIGH 2 · MED 1 · LOW 0
- **This pass:** HIGH 4 · MED 2 · LOW 2 (all eight CONFIRMED, all UNFIXED; every repro re-verified RED during synthesis)
- **Status (2026-07-24): findings CONFIRMED, all eight UNFIXED.** The body below is the as-found evidence.

This pass took the ledger's least-audited high-value cells — the brand-new v1.4.0 **SELECT mode +
bulk ops** surface (never audited), the post-pass-14 **v1.2.0 quick-add grammar** additions, the
post-pass-14 **v1.3.0 recurrence-rewrite primitives**, the pass-18-flagged **reconcile-vs-concurrent-pull
matrix beyond the CommitPush window**, and the COVERAGE-flagged **shared ops/rollback Restore + reparent**
clobber gap. Six method-diverse audits (data-loss, input-edge, fuzz, spec-diff, race) landed **eight
confirmed findings** — a sharp jump from pass 18's three — every one carrying a runnable repro that
runs RED today.

The news is bad across the board. **HIGH climbed 2→4**, total **3→8**. Two of the four HIGH are the
*same class as prior passes reopening a third and fourth time*: the pushDelete-412 resurrect is the
delete-conflict twin of pass-18's CommitPush `cur==nil` HIGH, and the reparentTo bare Put reopens the
Locate→Put clobber class the pass-13 exhaustive sweep declared "no sites remain". The co-resident
delete and multi-root undo HIGHs are new-surface data-loss on the v1.4.0 bulk feature.

---

## Coverage exercised

| Surface | Package | Method | Result |
|---|---|---|---|
| SELECT mode + bulk ops (bulkComplete/bulkDelete over a range; co-resident bundles) | internal/ui | data-loss | **1 HIGH** (bulkDelete/deleteWholeObject erase unselected co-resident items) |
| SELECT key dispatch + GRAB nested in SELECT (range derivation, mode transitions) | internal/ui | input-edge | **1 LOW** (vim count leaks past a swallowed bulk-op key); 1 canary ESCAPE (dayInRange highlight boundary) |
| Quick-add v1.2.0 grammar additions (time-range, relative-date, recurrence anchoring, @location, warnings) | internal/model | fuzz | **1 LOW** (parseEveryRecur accepts impossible month-days); 1 canary ESCAPE (parsePriority `!9` bound) |
| Recurrence write-side v1.3.0 rewrite primitives (ReanchoredRecurrence, NewSeriesFrom, DetachTodoOccurrence, Custom vocabulary) | internal/model | spec-diff | **1 HIGH + 1 MED** (last-weekday reanchor contradicts DTSTART; positive-nth escapes the editable vocabulary) |
| Reconcile-vs-concurrent-pull matrix beyond the CommitPush window (tombstone/keep-both/read-only-twin races) | internal/sync | race | **1 HIGH** (pushDelete 412 resurrect clobbers a concurrent undo/re-create) |
| Shared ops/rollback Restore in yankpaste/moveSubtree + reparentTo | internal/ui | data-loss | **1 HIGH + 1 MED** (undo of a co-resident multi-root move loses a root; reparentTo bare Locate→Put clobbers a pull) |

---

## Confirmed findings (each carries a runnable repro, all re-verified RED this synthesis)

### HIGH

1. **`bulkDelete` (and single-item `deleteWholeObject`) erase unselected co-resident items bundled in
   the same `.ics` resource** — `internal/ui/bulkops.go:305`, `internal/ui/edit.go:468`. Both call
   `store.Delete(loc.CalID, loc.Name)`, removing the whole resource file per UID; neither isolates the
   selected component. Any co-resident top-level VTODO/VEVENT sharing that bundled resource is silently
   deleted — the confirm said "Delete 1 item(s)?" — and with a server identity the tombstone pushes a
   permanent server DELETE of the never-selected bystander. The cross-list move path (yankpaste.go:341)
   correctly uses `model.RemoveComponent` to rewrite the resource sparing siblings; the delete path
   skips that step. Reachable only from a bundled/foreign/migrated `.ics` (LazyPlanner writes one item
   per resource — exactly why IsolateComponent/RemoveComponent exist), but permanent silent loss when
   it lands.
   **Repro (RED):** `internal/ui/coresident_delete_test.go` `TestBulkDeleteDragsCoResidentBystander` —
   bundles two independent top-level VTODOs (mover + bystander) in one `personal/bundle.ics`, enters
   SELECT on the mover only, `bulkDelete()`, accepts "Delete 1 item(s)?"; the bystander no longer
   resolves via `store.Locate` (`coresident_delete_test.go:93`). Left in the tree, untracked.
   Fix: use `model.RemoveComponent` and rewrite the resource when sibling items remain.

2. **`ReanchoredRecurrence` "last <weekday>" backward day-move emits `BYDAY=-1<newWd>` that contradicts
   the moved DTSTART — the exact vanishing-instance bug the function exists to prevent** —
   `internal/model/recur_edit.go:51`. For a monthly `MonthlyNth==-1` rule the branch blindly keeps `-1`
   and only swaps the weekday to `newStart.Weekday()`, without checking newStart is actually the *last*
   occurrence of that weekday. A backward move onto a weekday whose true last occurrence is later in the
   month writes a rule that fires on a different day than DTSTART; DTSTART falls outside its own set, so
   the moved instance vanishes and the series jumps to the wrong day. The existing `reanchor_test.go`
   only covers the safe 1st-Mon→1st-Tue case, so the guard has a hole. Reachable end-to-end via grab
   `h` (grab.go commits the spec into `d.Recur`).
   **Repro (RED):** `internal/model/reanchor_lastweekday_repro_test.go`
   `TestReanchoredRecurrenceLastWeekdayBackwardMoveRepro` — DTSTART 2024-04-24 (last Wed) + `BYDAY=-1WE`,
   move −1 day to 2024-04-23 (Tue); re-anchored `BYDAY=-1TU` fires only 2024-04-30, and the moved
   2024-04-23 is not in its own set (`reanchor_lastweekday_repro_test.go:61`). Left in the tree.
   Fix: special-case `MonthlyNth==-1` — verify newStart is the last `<wd>` of its month before writing
   `-1<wd>`; otherwise re-derive the true nth or block the move `(nil,true)`.

3. **`pushDelete`'s 412 resurrect uses an unconditional `PutRemote`, clobbering a concurrent local
   re-create/undo** — `internal/sync/sync.go:687`. When a conditional DELETE loses to a server edit
   (412), reconcile resurrects the server version with a bare `store.PutRemote` instead of the
   compare-and-set (PullRemote/expectedPrev) pattern every other reconcile pull/write uses — the ONLY
   bare unconditional write left in the writable reconcile path. If the UI re-creates the resource at
   the same name during the DELETE round-trip (undoLast→RestoreDirty), the resurrect silently overwrites
   the restored content with the server version and stashes ONLY the server version as the conflict, so
   both keep-local and keep-server yield server content — the user's undone content is unrecoverable.
   Violates the "sync never silently overwrites" hard invariant. Delete-conflict twin of the pass-18
   CommitPush `cur==nil` HIGH.
   **Repro (RED):** `internal/sync/tombstone412_undo_race_test.go`
   `TestTombstone412ResurrectDoesNotClobberConcurrentUndo` — an `onDelete` hook fires `RestoreDirty`
   (standing in for `u`) the instant `DeleteObject` is called, before it returns 412; after the sync the
   live resource holds "ServerEdit" not "MyContent", Conflicts=1 Skipped=0 (`tombstone412_undo_race_test.go:84`).
   Left in the tree. Fix: resurrect only when `cs.resources[N]` is still nil; add a -race variant to
   `TestConcurrentSyncAndEditsRace`.

4. **Undo of a co-resident multi-root move permanently loses a root** — `internal/ui/yankpaste.go:367`,
   replayed forward by `internal/ui/edit.go:707`. When two independent top-level VTODOs co-reside in one
   bundle, a bulk cut+paste appends two `undoOp`s for the SAME source resource with intermediate prev
   snapshots (root1: prev=full bundle; root2: prev=bundle-minus-root1, since it re-Locates after
   root1's write). `undoLast` applies `step.ops` in *append order* — oldest prev first, newest last — so
   the bundle-minus-root1 snapshot wins and root1's destination copy was already Deleted. Undo erases
   root1 entirely with no tombstone/conflict. (The rollback slice is immune — it iterates reverse; only
   `undoLast` replays forward.) The same root cause makes `reparentOps`' same-list variant leave the
   earlier root re-parented after undo (milder: wrong parent, no content loss).
   **Repro (RED):** `internal/ui/undo_multiroot_clobber_test.go` `TestReproUndoMultiRootMoveLosesRoot` —
   bulk-cut two co-resident VTODOs, paste into "work", undo; r1 present=false, r2 present=true
   (`undo_multiroot_clobber_test.go:76`). Left in the tree. Fix: coalesce/order same-resource undo ops
   so the most-complete snapshot wins (reverse-apply, or dedupe by (calID,name) keeping the earliest
   prev).

### MED

5. **`ReanchoredRecurrence` positive-nth day-move can emit `MonthlyNth=5`, escaping the editable 1st–4th
   vocabulary and silently thinning the series** — `internal/model/recur_edit.go:52`. The re-derivation
   `(newStart.Day()-1)/7 + 1` computes 5 when the moved anchor lands in a month's fifth week, producing
   `BYDAY=5<wd>` — outside the spec's editable set (1st–4th/last, main.md:424). `RecurSpecFromRule`
   rejects `n==5`, so the rule becomes a non-editable "Custom rule (kept)", and `BYDAY=5<wd>` fires only
   in months with a 5th `<wd>` (~4–5/year), silently dropping the event from the rest. No crash and the
   moved instance itself is consistent (it *is* the 5th occurrence), so this is a semantic-fidelity /
   interop degradation, not an outright contradiction.
   **Repro (RED):** verified this synthesis via `eventForReanchor` (reanchor_test.go helper) —
   `FREQ=MONTHLY;BYDAY=4MO`, DTSTART 2020-09-28 (4th Mon, day 28), +1 day to 2020-09-29 → `spec.MonthlyNth=5`,
   `MonthlyWeekday=Tuesday`, and `RecurSpecFromRule(spec.ROption(), newStart)` returns ok=false. The
   auditor removed the repro file; re-add it (the finding text carries the full source) as the guard.
   Fix (shared with #2): block the move `(nil,true)` when the re-derived nth isn't in {1,2,3,4,-1} or
   doesn't actually match newStart, instead of writing an out-of-vocabulary rule.

6. **Single-item same-list reparent (`reparentTo`) uses a bare Put, silently clobbering a concurrent
   sync pull** — `internal/ui/yankpaste.go:142`. `reparentTo` does `Locate` (at `paste()`, line 96) →
   `SetTodoParent` → `store.Put(...)` on an existing resource — a bare Locate→Put. A background pull
   landing between the Locate and the Put is silently overwritten: Put adopts the freshly-pulled ETag
   while persisting the stale-derived content, so the next push's CAS matches the server and clobbers
   the remote edit. This is the one write path the pass-13 exhaustive Locate→Put sweep and the
   v1.5.0-step-0 moveSubtreeOps fix both missed — the sibling `reparentOps` (multi-root) already uses
   `PutIfUnchanged` (its own comment at yankpaste.go:255-256 flags the contrast). The pass-13 "no
   Locate→Put clobber sites remain" claim is again FALSE for this path.
   **Repro (RED):** `internal/ui/reparent_clobber_test.go` `TestReparentToDoesNotClobberConcurrentPull`
   — a pull advances a co-resident bystander to rev-1 in the window after `paste()`'s Locate; reparentTo
   rewrites from the stale snapshot via bare Put; the bystander reads "rev-0" not "rev-1"
   (`reparent_clobber_test.go:66`). Left in the tree. Fix: route reparentTo through
   `store.PutIfUnchanged(ctx, src.CalID, src.Name, obj, src.Prev)`, abort on applied==false.

### LOW

7. **Vim count leaks past a SELECT bulk-op key into the next motion** — `internal/ui/app.go:781`. A
   pending count accumulated in SELECT is not cleared when a swallowed bulk-op key (Space/d/y/Y/m) fires:
   `handleSelectKey` returns nil, so `globalKeys` returns at the `if a.handleSelectKey(ev) == nil { return
   nil }` branch (app.go:780-782) BEFORE the count-reset (app.go:798-799); the bulk op then exits SELECT
   into normal mode with `pendingCount` still >0, so the next single j/k moves N rows instead of 1,
   silently landing on the wrong item. Normal-mode `m`/`y` don't leak because their handling runs after
   the count reset; only the SELECT swallow path returns early.
   **Repro (RED):** `internal/ui/countleak_repro_test.go` `TestSelectBulkOpDoesNotLeakCount` — enterSelect,
   `runeKey('3')` (pendingCount==3), `runeKey(' ')` (bulkComplete); SELECT exits but pendingCount stays 3
   (`countleak_repro_test.go:42`). Left in the tree. Fix: reset the count on the SELECT-swallow path.

8. **`parseEveryRecur` accepts impossible month-days (feb 30, apr 31), silently dropping the typed date
   and misanchoring the recurrence** — `internal/model/quickadd.go:641`. The "every <month> <day>" path
   validates only `1<=day<=31` with no validYMD/rollForwardMonthDay gate, unlike the sibling plain-date
   path (quickadd.go:584-590, fixed in pass 14). So "every feb 30 dentist" is consumed as a yearly
   RecurSpec and stripped from the title, but `applyRecurAnchor`'s rollForwardMonthDay fails so HasDate
   stays false; FreqYearly `ROption()` emits no BY* and relies solely on the caller-set DTSTART, so the
   Month/Day fields are dead data and the series silently anchors to the caller's context day — a day the
   user never specified — while the typed "feb 30" text is lost. Compare "feb 30 dentist" which correctly
   stays entirely in the title. Affects apr/jun/sep/nov 31 and feb 29/30.
   **Repro (RED):** verified this synthesis — `ParseQuickAdd("every feb 30 dentist", …)` →
   `Title="dentist", HasDate=false, Recur=yearly{Month:February, Day:30, HasMonthDay:true}` while the
   plain-date baseline stays in the title. The auditor removed the file; the finding text carries the full
   source to re-add. Fix: gate the month-day branch on validYMD/daysInMonth like `parseDate`.

---

## Mutation-canary results — 2 of 3 escaped (2 OPEN test-coverage holes)

Canaries probe the *test net*; an escape means the code is correct today but a plausible future
regression on that exact path would ship silently. Neither escape is closed yet.

- **ESCAPE** — `internal/ui/selection.go` `dayInRange()`: upper-boundary flip
  `!d.After(model.DayStart(to))` (inclusive) → `d.Before(...)` (exclusive) passed `go test
  ./internal/ui/`. `dayInRange`/`selDayAnchor` are draw-path-only helpers (calendarview.go:269,
  timegridview.go:575) that style in-range SELECT days and appear in no `*_test.go` — the range
  *materialization* (daysRange/selRange) is well covered, but the *visual highlight* boundary it drives
  is unguarded, so an off-by-one making the highlighted band stop one day short of the acted-on interval
  ships undetected. Guard: assert the cursor/`to` day is highlighted at the band's upper edge.
- **ESCAPE** — `internal/model/quickadd.go` `parsePriority` (line 434): narrowing `n <= 9` → `n <= 8`
  passed `go test ./internal/model/`. Quick-add priority tests exercise only `!high`/`!1`; no test
  parses a numeric `!9`/`!8`, so the 1–9 edge is unguarded (priorityrange_test.go covers the separate
  iCal-clamp; the fuzz only asserts 0–9). Guard: table `!9`/`!8`/`!10` through parsePriority.
- **CAUGHT** — `internal/sync/sync.go` `reconcileCalendar` step B (~line 447): dropping the tombstone
  guard `|| tombstonedHref[o.Path]` re-pulls a locally-deleted-but-unpushed resource, resurrecting it
  and clearing its tombstone before step C can push the DELETE. `go test ./internal/sync/` FAILED across
  5 tombstone/delete-path tests (TestSyncPushesTombstoneDelete, TestSyncTombstoneVsServerEditIsConflict,
  TestSyncDeleteTransient403KeepsTombstone, TestSyncDeleteConfirmedReadOnlyDiscards,
  TestUndoOfSyncedDeleteSurvivesNextSync). The delete/tombstone net has real teeth.

---

## Convergence

| Severity | Pass 18 | Pass 19 | Trend |
|---|---|---|---|
| HIGH | 2 | 4 | ↑ |
| MED  | 1 | 2 | ↑ |
| LOW  | 0 | 2 | ↑ |
| **Total** | **3** | **8** | ↑ |

Severity is **trending UP sharply, not down.** HIGH doubled 2→4 and total rose 3→8. This is partly
expected — the pass deliberately opened the never-audited v1.4.0 SELECT/bulk-ops surface and the
post-pass-14 v1.2.0/v1.3.0 additions — but two of the four HIGH are *class reopenings*, which is the
worrying signal: the pushDelete-412 resurrect is the delete-conflict twin of pass-18's CommitPush HIGH
(the "concurrent-write signal has no resource-is-gone case" class, now hit twice), and the reparentTo
bare Put reopens the Locate→Put clobber class that pass 13 declared exhaustively closed ("no clobber
sites remain" — FALSE a third time). Bulk data-loss (co-resident delete, multi-root undo) is new-surface
but shares the "multi-write op operates on the whole resource, not the selected component" shape with
the pass-10 yank/paste HIGHs. The readiness trend the pass-15/16/17 streak was building remains broken.

---

## Residual risk

- **4 HIGH + 2 MED + 2 LOW confirmed, all EIGHT UNFIXED.** Four data-loss HIGHs are live in normal
  operation: a bulk/single delete drags co-resident bystanders to a permanent server DELETE; a recurring
  "last <weekday>" day-move makes the moved instance vanish; a 412 delete-resurrect clobbers a concurrent
  undo unrecoverably; and undo of a co-resident multi-root move permanently loses a root. Two of these
  (bulk-delete, multi-root undo) require a bundled/foreign `.ics`; two (last-weekday reanchor via grab
  `h`, the 412 race) are reachable from single-resource, LazyPlanner-native data.
- **Two class reopenings.** The Locate→Put clobber class (reparentTo, MED) and the concurrent-delete-signal
  class (pushDelete-412 vs pass-18 CommitPush, HIGH) each reopened on a sibling path a prior "closed"
  claim missed. Fixing the specific path is not enough — sweep every peer (all bare Put write sites; every
  reconcile write for the `cur==nil`/resource-gone case) before re-declaring either class shut.
- **Two canaries escaped — two live test-net holes**, neither guarded: the SELECT day-range *highlight*
  boundary (draw-path helper, zero tests) and the quick-add numeric-priority 1–9 edge. A comparison /
  off-by-one regression on either ships silently.
- **The reconcile-vs-concurrent-pull matrix is warmer but still not cleared.** Pass 19 hit the 412-resurrect
  HIGH within it; the tombstone/keep-both/read-only-twin races beyond both the CommitPush *and* the
  pushDelete windows remain shallowly covered.
- **Surfaces that produced no finding are warm, not proven correct.** The v1.2.0 time-range / relative-date
  / @location recognizers and the warnings channel were fuzzed and yielded only the one LOW; NewSeriesFrom
  and DetachTodoOccurrence were spec-diffed and held — absence of a finding through one lens is not absence
  of a bug.
- **Repro hygiene:** six repros are left in the tree, untracked, and currently RED — they will break `make
  check` until their fixes land (`internal/ui/coresident_delete_test.go`,
  `internal/ui/countleak_repro_test.go`, `internal/model/reanchor_lastweekday_repro_test.go`,
  `internal/sync/tombstone412_undo_race_test.go`, `internal/ui/undo_multiroot_clobber_test.go`,
  `internal/ui/reparent_clobber_test.go`); keep them as regression guards. The two removed repros
  (parseEveryRecur LOW, positive-nth MED) were re-verified RED during synthesis and must be re-added with
  their fixes.
- **Not covered / still deferred:** full `sync-collection` token-delta sync (unbuilt); the Raspberry Pi
  hardware target (needs a physical Pi); the recent model/caldav/store cells (passes 15–18) were
  deliberately left to cool.

**Recommendation: more passes recommended** — 4 HIGH + 2 MED + 2 LOW confirmed (all unfixed) and 2/3
canaries escaped. Fix repro-first, one commit per fix, full gate every commit: isolate the selected
component in bulkDelete/deleteWholeObject via `model.RemoveComponent`; special-case `MonthlyNth==-1` and
clamp/block out-of-vocabulary nths in `ReanchoredRecurrence`; guard the pushDelete-412 resurrect on the
resource still being absent (mirror pullInto's expectedPrev); coalesce same-resource undo ops so the
full-bundle snapshot wins; route reparentTo through `PutIfUnchanged`; reset the vim count on the
SELECT-swallow path; gate parseEveryRecur on validYMD; and close both canary holes with boundary tests.
After the two class-reopening fixes, sweep every peer path before re-declaring either class shut.

---

## Resolution (2026-07-24)

All eight confirmed findings were fixed repro-first, one commit each, full gate green every commit:

1. HIGH — bulkDelete/deleteWholeObject co-resident bystander: **40b0803**
2. HIGH — ReanchoredRecurrence "last &lt;weekday&gt;" backward-move contradiction: **8051ddc**
3. HIGH — pushDelete 412 resurrect clobbers concurrent undo: **d39853d**
4. HIGH — undo of a co-resident multi-root move loses a root: **9de7ecc**
5. MED — ReanchoredRecurrence positive-nth escapes the editable vocabulary: **228dbbc** (fixed same
   commit as #2, 8051ddc; 228dbbc re-adds the regression test the auditor's synthesis-only repro left
   out of the tree)
6. MED — reparentTo bare Put: **18fbca3**, plus the class sweep this reopening triggered across
   `internal/ui` (**18fbca3..c50a42d**: grab.go, recur_edit.go, yankpaste.go fresh-create annotations,
   and the CLAUDE.md Hard-won-guardrail update)
7. LOW — SELECT vim-count leak: **33d01d3** (regression test added **c441b32** after being left
   untracked in 33d01d3)
8. LOW — parseEveryRecur impossible month-day: **acc0c6b**

Both escaped mutation canaries are closed with regression tests: the `parsePriority` numeric-boundary
hole — **29ca392** (`internal/model/quickadd_test.go`) — and the `dayInRange` highlight-boundary hole
— **fb7d8e2** (`internal/ui/selection_test.go`).

A final whole-arc review (commit range 66222d9..c441b32) came back **READY TO MERGE**: 0
Critical/Important findings, 7/7 cross-cutting checks PASS, 3 non-blocking minors (the CLAUDE.md
missing test-citation, addressed in this close-out increment; the pre-existing `moveSubtreeOps`
dest-Put orphan-retry residual, recorded as a named blind spot in `COVERAGE.md`; an unreachable
defensive branch in the reanchor code, no action needed).

The review's recommendation was **more_passes_recommended** — this pass's own convergence trend ran
UP (HIGH 2→4, total 3→8), not down, so hardening continues in a future pass even though every finding
this pass produced is now fixed. Residual risk carried forward, per `COVERAGE.md`:

- The two reopened classes (bare-Put clobber; the "concurrent-write signal has no resource-is-gone
  case" reconcile class) had their *found* sites fixed and, for bare-Put, a full `internal/ui` sweep —
  but neither sweep crossed into peer packages (`internal/store`, `internal/model`, `internal/caldav`)
  for the same two shapes. That cross-package sweep is a target for a future pass.
- The pre-existing `moveSubtreeOps` dest-Put orphan-retry residual (yankpaste.go:344) — a swallowed-
  error rollback gap where a retried move's fresh-create bare Put could clobber an orphaned
  destination-side copy left by a prior failed attempt — was not a pass-19 finding (no repro run
  against it) and is now a named blind spot for a future pass.
- The recent cells from passes 15–18 were deliberately left to cool, per the pass-19 body's own
  "not covered / still deferred" note.
