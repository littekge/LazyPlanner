# Pass 20 — store/sync/UI concurrent-write clobber sweep + post-pass-18 whole-app spec-diff

- **Date:** 2026-07-25
- **Prior pass:** Pass 19 (v1.4.0 SELECT/bulk-ops + v1.3.0 recurrence-rewrite + phase-3 rollback/reconcile clobber gaps) — HIGH 4 · MED 2 · LOW 2 (all eight FIXED 2026-07-24)
- **This pass:** HIGH 1 · MED 2 · LOW 2 (all five CONFIRMED, all UNFIXED; every repro ran RED)
- **Status (2026-07-25): findings CONFIRMED, all five UNFIXED.** The body below is the as-found evidence, not a verdict.

This pass took the ledger's least-audited high-value cells left after pass 19: the pass-19-flagged
**cross-package peer-write blind spot** (store conflict-resolution `PutRemote` + the sync reconcile
matrix beyond the CommitPush/pushDelete-412 windows), the explicitly-OPEN **`moveSubtreeOps` dest-Put
orphan/cross-collection residual**, the never-deep-audited **bulk-grab (GRAB-nested-in-SELECT)** temporal
axis, the **SELECT multi-write bulk ops under `-race`**, and the **whole-app spec-diff of the post-pass-18
feature set** (v1.2.0 quick-add / v1.4.0 SELECT / v1.5.0 polish, never checked as a whole).

Six method-diverse audits (data-loss, race, input-edge, spec-diff) landed **five confirmed findings**,
each with a repro that runs RED today. The headline: **the "concurrent-write signal has no
resource-is-gone case" class reopened a THIRD time** — the sync reconcile step-(A) `Forget` path (this
pass's sole HIGH) is the exact twin of pass-18's CommitPush `cur==nil` HIGH and pass-19's pushDelete-412
HIGH, the one reconcile write neither prior fix covered. Severity is trending **down** in raw counts
(HIGH 4→1, total 8→5), but a HIGH data-loss plus a reopened systemic class plus two canary escapes means
the surface is not converged.

---

## Coverage exercised

| Surface | Package | Method | Result |
|---|---|---|---|
| Store peer write paths — conflict-resolution `PutRemote` (ResolveKeepServer, conflict.go:177) + bare unconditional store writes | internal/store | data-loss | **no store-layer finding** (peer writes are synchronous user-resolution, not raced by a background pull; the racing-pull loss lives one layer up in sync) |
| Sync reconcile-vs-concurrent-pull matrix beyond the CommitPush/pushDelete-412 windows (step-(A) clean-branch Forget) | internal/sync | race | **1 HIGH** (step-(A) `Forget` clobbers a concurrent local edit — third reopening of the resource-is-gone class) |
| `moveSubtreeOps` destination-side fresh Put under partial-land + cross-collection subtree | internal/ui | data-loss | **1 MED** (a cross-collection RELATED-TO child is lost — dest Put clobbers + rollback Forgets its real resource) |
| Bulk grab (GRAB nested in SELECT), task date-shift axis | internal/ui | input-edge | **1 LOW** (recurring-todo DUE shift never re-anchors a day-pinning RRULE; same omission in single-item grab) |
| SELECT multi-write bulk ops (bulkDelete/bulkComplete) interleaved with a concurrent pull | internal/ui | race | **no new -race finding** on the bulk ops; **1 LOW** found separately (day-range highlight uncapped vs materialization) |
| Whole-app promise conformance for v1.2.0/v1.4.0/v1.5.0 vs main.md | (whole app) | spec-diff | **1 MED** (bare daily/weekly/monthly/yearly recurring TASK is not date-anchored → uncompletable, breaking the "daily → the base day" promise) |

---

## Confirmed findings (each carries a runnable repro; all ran RED)

### HIGH

**1. Step-(A) remote-delete `Forget` clobbers a concurrent local edit (unguarded lost update)**
— `internal/sync/sync.go:411`.
`reconcileCalendar`'s `case !onServer:` clean branch (a resource deleted on the server) calls
`st.Forget(calID, r.Name)` — an unconditional remove-by-name — using the *stale* post-download snapshot
pointer `r`. `Calendar()` snapshots resource *pointers*; a concurrent UI edit builds a NEW `*Resource`
and replaces the map entry (`stageResourceLocked`), so the loop reads `r.Dirty` off the clean snapshot
and takes the unguarded `!onServer` (not `!onServer && r.Dirty`) branch, Forgetting whatever now lives
at that name and leaving a tombstone. Unlike the sibling push/pull paths (CommitPush / PullRemote /
PutIfUnchanged / ResurrectTombstone) there is no pointer-identity/expectedPrev guard.
- **Failure scenario:** calendar holds clean `a` (edited locally → dirty, so step-(A) pushes it) and
  `b` (deleted on the server). During `a`'s in-flight PUT the user edits `b`; the loop reaches `b` with
  the stale clean pointer, takes `!onServer`, and Forgets `b`. The user's edit is gone with **no conflict
  and no skip** (observed SyncResult: `PulledDeletes=1 Conflicts=0 Skipped=0 Pushed=1`), and the next
  sync issues a DELETE for it. Silent data loss.
- **Repro (ran RED):** `internal/sync/stepa_forget_clobber_repro_test.go` —
  `go test ./internal/sync/ -run TestReproStepAForgetClobbersConcurrentEdit -v`. Asserts `b` survives
  with the edited summary; fails because `b` was Forgotten. Logged the predicted signature exactly.
- **Class:** THIRD reopening of "concurrent-write signal has no resource-is-gone case" — twin of
  pass-18 CommitPush `cur==nil` and pass-19 pushDelete-412; this is the FORGET path.
- **Fix direction:** guard `Forget` by pointer identity (mirror PullRemote's expectedPrev) and, on
  mismatch, raise a `serverDeleted` markConflict against the surviving local edit.

### MED

**2. `moveSubtreeOps` cross-collection child is lost** — `internal/ui/yankpaste.go:344`.
`moveSubtreeOps` Locates each subtree member's real calendar (`loc.CalID`, line 323) but hard-codes the
caller-supplied `srcCal` for every source-side write/delete/restore (lines 362/369/375) and treats the
destination Put (344) as a guaranteed fresh create. Both premises break for a subtree spanning
collections via a cross-collection `RELATED-TO` link (parent in list A, child in list B, both
server-synced — a legitimate state another client can create; `descendants()` links them globally).
- **Failure scenario:** user cuts the parent and pastes into the child's OWN list. For the child,
  `Locate` finds `B/C.ics` — NOT a fresh create — so the bare Put clobbers the child's existing resource;
  the source Delete then targets `srcCal=A` (which has no `C.ics`) and errors; rollback runs newest-first
  and its `Forget(B, C.ics)` deletes the child's REAL resource with no tombstone. The child vanishes from
  the cache (permanent if it had unsynced edits; self-heals only on next sync if clean).
- **Repro (ran RED):** `internal/ui/movesubtree_crosscoll_repro_test.go` —
  `go test ./internal/ui/ -run TestMoveSubtreeCrossCollectionChildIsLost -v`. Confirms the child is no
  longer locatable after the failed cross-collection move.
- **Fix direction:** use `loc.CalID` (not `srcCal`) for source-side ops, and route the dest Put through
  `store.PutIfUnchanged` (existing-resource rewrite) when Locate finds a resource already at that name.
- This is the residual pass 19 explicitly flagged OPEN — now confirmed with a running repro.

**3. Bare daily/weekly/monthly/yearly recurring TASK is not date-anchored** — `internal/ui/edit.go:233`
(root cause `internal/model/quickadd.go:380` `applyRecurAnchor`).
main.md:174/395 promises a quick-add recurrence with no explicit date anchors the start/due itself
("daily → the base day", "tasks and events alike"). Events honor it; TASKS do not. `applyRecurAnchor`
sets `HasDate` only for the weekday ("every mon") and month-day ("every jul 20") forms — a bare
`daily`/`weekly`/`monthly`/`yearly` leaves `HasDate=false`/`HasTime=false`, so createTask's
`if qa.HasDate || qa.HasTime` gate omits DUE and `NewTodoObject` emits a VTODO with an RRULE but NO
DTSTART and NO DUE.
- **Failure scenario:** quick-add `water plants daily`. Pressing Space to complete routes to
  `AdvanceRecurringTodo`, which errors `has no DTSTART/DUE to advance` (UI flashes "Complete failed: …").
  The task can never be completed and shows no due date; `bulkComplete` fails identically. The existing
  `internal/ui/quickrecur_test.go` asserts only the RRULE, never the DUE — it passes while the created
  task is broken.
- **Repro (ran RED):** `internal/model/bare_recur_todo_repro_test.go` —
  `go test ./internal/model/ -run TestBareDailyRecurringTodoCanBeCompleted -v`. All four bare frequencies
  error identically.
- **Fix direction:** `applyRecurAnchor` should anchor bare frequencies to the base day so `HasDate` is set.

### LOW

**4. Bulk-grab of a recurring todo shifts DUE without re-anchoring a day-pinning RRULE** —
`internal/ui/bulkgrab.go:181`.
`bulkGrabShift`'s todo branch does `d.Due = d.Due.AddDate(0,0,todoDays)` + `model.EditTodo` with no
`model.ReanchoredRecurrence` call, unlike the event branch of single-item grab (grab.go:289). Shifting
the DUE of a recurring todo whose rule pins a weekday (`BYDAY` / monthly nth-weekday) leaves DUE/DTSTART
contradicting its own `BY*` — the same "a rule can't contradict its start date" invariant the events
path was hardened for.
- **Failure scenario:** bulk-select a `FREQ=WEEKLY;BYDAY=MO` todo (due Monday), bulk-grab, shift +1 day
  → DUE becomes Tuesday but `BYDAY=MO` is untouched, so the next `AdvanceRecurringTodo` snaps back to
  Monday. No crash, no vanish (todos render at DUE, not grid-expanded) — a subtle series-advance
  inconsistency.
- **Repro (ran RED):** `internal/ui/bulkgrab_recur_reanchor_repro_test.go` —
  `go test ./internal/ui/ -run TestBulkGrabRecurringTodoReanchorsByday -v`. Next due comes back 2026-07-13
  (Mon) instead of 2026-07-14 (Tue).
- **Note:** identical omission in single-item grab (grab.go:247) — pre-existing shared behavior.
- **Fix direction:** call `model.ReanchoredRecurrence` in both grab todo branches.

**5. SELECT day-range visual highlight is not capped at 366 days while materialization is** —
`internal/ui/calendarview.go:245` (also `timegridview.go:575`).
main.md:205 caps a calendar day-range selection at 366 days from the anchor. `daysRange` enforces this
on materialization (selection.go:270 clamps `to = from.AddDate(0,0,maxSelectDays)`), but the on-screen
highlight is drawn independently via `dayInRange(anchor, cursor, day)` with no cap.
- **Failure scenario:** extend the cursor past anchor+366 with repeated `f`; every day up to the cursor
  renders reverse-video/selected, but bulkDelete/bulkComplete/startBulkGrab materialize only
  anchor..anchor+366, so items on the highlighted tail are silently never acted on — the acted-on set
  does not match what the UI shows as selected.
- **Repro (ran RED):** `internal/ui/daysrange_cap_repro_test.go` —
  `go test ./internal/ui/ -run TestDaysRangeHighlightMatchesMaterialization -v`. An item on anchor+400
  is `dayInRange`-highlighted but absent from `daysRange()`.
- **Fix direction:** cap the highlight the same way — factor the clamp into a shared helper both use.

---

## Surfaces swept with no finding

- **Store peer write paths** (conflict-resolution `PutRemote` at `conflict.go:177` + the other bare
  unconditional store writes) — the pass-19-flagged cross-package blind spot. No store-layer finding:
  the conflict-resolution `PutRemote` is reached only from an explicit, synchronous user
  conflict-resolution action, not from a background pull racing it, so the concurrent-delete-signal /
  bare-write clobber shape does not manifest at the store layer. The racing-pull data-loss lives one
  layer up — see finding #1 in the sync reconcile loop.
- **SELECT multi-write bulk ops under `-race`** (bulkDelete/bulkComplete issue N separate store writes;
  a pull landing mid-loop was the untested TOCTOU window) — no new race finding this pass. (The
  separate day-range-highlight LOW #5 was found by inspection, not by the race harness.)

---

## Mutation canaries — 2 of 4 escaped (OPEN)

Full detail (with close-out guidance) is in `COVERAGE.md` → "Escaped mutation canaries — pass 20".

- **CAUGHT** — `internal/store/mutate.go` `remove()` tombstone `r.Href != ""` guard drop →
  `TestDeleteNeverSyncedLeavesNoTombstone` + `TestCommitPushHonorsDeleteOfNeverSyncedCreate` FAILED.
- **CAUGHT** — `internal/sync/sync.go` `downloadResilient` `unfetched[ref.Href]=true` deletion (degraded
  fetch mistaken for server deletion) → 3 tests FAILED
  (`TestDegradedDownloadNotTreatedAsDeletion`, `TestDegradedDownloadDirtyResourceNotFalselyConflicted`,
  `TestReadOnlyDegradedDownloadKeptVsDeleted`).
- **ESCAPE (OPEN)** — `internal/ui/weekdaystrip.go` `moveCursor` (line 166): upper-clamp weakened
  `> daysInWeek-1` → `> daysInWeek`, letting the day cursor reach index 7 (one past the last valid cell
  6) — a potential OOB index into the seven day cells. No test exercises `moveCursor`'s upper bound.
- **ESCAPE (OPEN)** — `internal/model/quickadd.go` `parseTimeHalf` 24-hour branch: hour ceiling widened
  `h > 23` → `h > 24`, accepting an invalid hour 24 (`24:00`) that `time.Date` would spill into the next
  day. The time-range tests exercise valid boundaries but never feed an out-of-range 24-hour value; the
  sibling 12-hour `h>12` bound IS pinned.

---

## Convergence & recommendation

- **Severity trend (raw counts):** DOWN — HIGH 4→1, MED 2→2, LOW 2→2; total 8→5.
- **But not converged:** a HIGH data-loss was confirmed, the "resource-is-gone" reconcile class reopened
  a third time (the FORGET path neither prior fix covered), and 2 of 4 canaries escaped (unguarded
  boundaries on `weekdayStrip.moveCursor` and `parseTimeHalf`'s 24-hour ceiling).
- **Recommendation:** `more_passes_recommended`. Fix arc: all five findings repro-first (one commit each,
  full gate every commit); close both canary escapes with boundary tests verified RED under their exact
  mutations. Then re-sweep the still-un-audited peer write paths in `internal/model` and `internal/caldav`
  for the same bare-write/resource-is-gone shapes (this pass covered only `internal/store`), and the
  never-audited full `sync-collection` incremental once implemented.

---

## Residual risk (what remains unknown / uncovered)

- **Cross-package resource-is-gone sweep incomplete:** this pass cleared `internal/store` peer writes and
  found the sync-layer FORGET HIGH, but `internal/model` and `internal/caldav` peer write paths were not
  swept for the same class — deferred to a future pass.
- **Two boundary paths unguarded** (`weekdayStrip.moveCursor` upper clamp; `parseTimeHalf` 24-hour
  ceiling) until the escaped canaries are closed.
- **Permanently accepted / deferred blind spots** (unchanged): Raspberry Pi on real hardware
  (on-device timing, kiosk, bare-TTY color); full `sync-collection` incremental (feature deferral);
  the pass-15 import UID-bearing/UID-less MED (owner-accepted residual); center agenda-board
  click-to-select (low-impact UI follow-up).

---

## Resolution (2026-07-25)

All five confirmed findings fixed repro-first (one commit each, full gate — `go test ./...`, `go vet`,
`staticcheck`, `go build`, `gofmt -l` — green on every commit), and both escaped mutation canaries closed
with boundary tests verified RED under their exact mutations.

| # | Sev | Finding | Fix commit | Regression guard |
|---|---|---|---|---|
| 1 | HIGH | Step-(A) remote-delete `Forget` clobbers a concurrent edit | `24500e8` | `internal/sync/stepa_forget_clobber_test.go` |
| 2 | MED | `moveSubtreeOps` loses a cross-collection RELATED-TO child | `60f1191` | `internal/ui/movesubtree_crosscoll_test.go` |
| 3 | MED | Bare-frequency recurring TASK has no DUE, uncompletable | `c16ea6c` | `internal/ui/bare_recur_todo_test.go` |
| 4 | LOW | Grab of a recurring todo doesn't re-anchor a day-pinning rule | `ab6ed06` | `internal/ui/grab_todo_reanchor_test.go` + `bulkgrab_recur_reanchor_test.go` |
| 5 | LOW | SELECT day-range highlight exceeds the 366-day acted-on cap | `5cf043d` | `internal/ui/daysrange_cap_test.go` + `TestDayInRangeCap` |

Canary escapes closed (`d81d432`): `weekdayStrip.moveCursor` upper clamp (`TestWeekdayStripCursorClampsAtRightEdge`)
and `parseTimeHalf` 24-hour ceiling (`TestParseTimeHalfHourCeiling`).

**Notes on fix choices that diverged from the audit's stated fix direction:**

- **#3** was fixed in the UI caller (`createTask`'s DUE gate now fires on `qa.Recur != nil`), NOT in
  `model.applyRecurAnchor` as the report suggested. Forcing `HasDate` in the model would make
  bare-frequency EVENTS ignore their selected-day base and snap to today, breaking the documented
  "base day = caller's context day" semantics. The base-day anchor belongs in the caller. The audit's
  model-layer repro (which mirrored the buggy gate) was replaced by a UI-level guard.
- **#4** also fixed the single-item grab twin (`grab.go`) the report flagged as a pre-existing shared
  omission, not just the bulk path; added a dedicated single-grab regression test.

**Guardrails codified (recurring-class rule):**

- New Hard-won guardrail for the **sync-reconcile resource-is-gone class** (three reopenings: CommitPush
  `cur==nil` → pushDelete-412 → step-(A) Forget) — reconcile removals must use `ForgetIfUnchanged`, never
  a bare `Forget`, and raise a `serverDeleted` conflict on a snapshot mismatch.
- The **reanchor guardrail** extended to state the recurrence anchor is `DUE` for a recurring VTODO (not
  just `DTSTART`), so every todo due-shift path must call `ReanchoredRecurrenceTodo`.
- The **version-check guardrail**'s citation list gained `movesubtree_crosscoll_test.go`.

**Residual carried forward (unchanged from the pass body):** the cross-package resource-is-gone sweep is
still incomplete — `internal/model` and `internal/caldav` peer write paths were not audited for the
bare-write/concurrent-delete-signal class this pass (only `internal/store`, which came back clean, and the
`internal/sync` FORGET HIGH). That cross-package sweep, plus the never-audited full `sync-collection`
incremental once implemented, are the named targets for the next pass. Recommendation stands:
`more_passes_recommended` — this pass's findings are fully resolved, but convergence trended up (a HIGH
data-loss + a third reopening of the reconcile class), so hardening continues.
