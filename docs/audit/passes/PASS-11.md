# Pass 11 — never/stale sweep: grab-mode, recurrence-edit UI orchestration, sync concurrency + CTag + background goroutines

- **Date:** 2026-07-15
- **Prior pass:** Pass 10 (stale-surface sweep) — HIGH 5 · MED 4 · LOW 0
- **This pass:** HIGH 3 · MED 2 · LOW 2 (all confirmed with runnable, executed repros)
- **Resolution (2026-07-15):** ALL 7 findings + both escaped canaries fixed, each
  adversarially verified with a regression test and its own commit; full gate +
  `-race` on store/sync/ui pass. The three HIGH shared one root cause (a multi-write
  op with no rollback) — fixed to match the sibling `beginGrabFuture`. New primitives:
  `store.ErrKeptLocalEdit`, `store.PutIfUnchanged`, `model.DetachTodoOccurrence`,
  ui `commitDetach`/`abortGrabStale`. Follow-up for a later pass: the systemic
  Locate→Put no-version-check pattern in quick-field edits + completion toggles (see
  `COVERAGE.md`). See `log.md` for per-fix entries.

This pass targeted the surfaces the ledger marked **never** or **stale** — the
modal write-paths and background-goroutine machinery that a "recent" verdict on
adjacent surfaces had let slip: the grab-mode state machine (never dedicated),
recurrence-edit UI orchestration (pass-9 model-only), sync concurrency after the
pass-5 batching / step-12 CTag changes, the CTag short-circuit, and the step-12
background sync goroutines. It deliberately did NOT re-cover the already-recent
decode/heal (10), recurrence read (8), tz/DST (8), CalDAV boundary (7), store
atomicity (10), config/state (9), draw paths (6), mouse (10), or yank/paste (10).

---

## Coverage exercised

| Surface | Package | Method | Result |
|---|---|---|---|
| Grab-mode temporal-manipulation state machine | internal/ui | data-loss | 1 MED + 2 LOW |
| Grab-mode modal key dispatch (j/k/h/l/J/K, undated-skip, Enter/Esc) | internal/ui | input-edge | (folded into the data-loss findings) |
| Sync concurrency (sync-vs-edits) post batching/CTag | internal/sync | -race stress | 1 HIGH (store-level clobber) |
| CTag incremental short-circuit (skip DownloadAll) | internal/sync | data-loss | no new finding in the skip decision |
| Background sync goroutines (startPeriodicSync + flushOnQuit) | internal/ui, internal/sync | race | no deadlock/race found |
| Recurrence-edit UI orchestration (scope picker + split/detach + grab retarget) | internal/ui | data-loss | 2 HIGH + 1 MED |

---

## Confirmed findings (each carries a repro that was run and observed to fail)

Every finding below has a **failing_test** repro that was executed and observed
red against the unfixed code. None are fixed yet — the repros are proposed
regression tests, several of which replicate the buggy ordering and must be
adapted (assert recovery, or call the real function) once the fix lands.

### HIGH

1. **`PullRemoteBatch` unconditionally clobbers a concurrent local edit to a pull-orphan** — `internal/store/remote.go:175` (routed from `internal/sync/sync.go:441`).
   Reconcile step (B) routes "new on server" resources through `PullRemoteBatch`, whose
   `stageResourceLocked` write is unconditional (`Dirty:false`, no `expectedPrev` /
   pointer-identity / dirty check, unlike single-resource `PullRemote`). A href-less
   pull-orphan (clean `.ics` left by a crashed prior batch) is classed "new on server"
   because it's absent from the pre-loop snapshot's `localByHref`. An edit landing during
   loop (A)'s network I/O is invisible to the include-in-batch decision, and the
   unconditional batch write overwrites it and marks it clean — lost in memory and on disk,
   never pushed. The pass-5 comment (`sync/sync.go:418-419`) claiming these writes "can't
   clobber a concurrent local edit" is **false** for this case.
   Repro: `internal/sync/repro_pullbatch_clobber_test.go`
   (`TestReproPullBatchClobbersConcurrentEditToOrphan`), ran — observed
   `orphan summary="Server" dirty=false`, expected `"user-edit"` / dirty.

2. **`commitSplit` (edit "this & future") half-completes with no rollback — all future occurrences lost** — `internal/ui/recur_edit.go:270`.
   `model.SplitEvent` yields (capped master, future series). `store.Put(capped)` succeeds
   and truncates the master (RRULE UNTIL just before the occurrence); `store.Put(future)`
   then fails (ENOSPC / permission / sidecar-write) → early return on a flash, `pushUndo`
   (line 280) never reached. The master is permanently truncated and the future tail was
   never created — unrecoverable from the UI. The sibling grab path (`beginGrabFuture`)
   already has this rollback (guarded by `TestGrabFutureRollbackRestoresMaster`);
   `commitSplit` was left unguarded.
   Repro: `internal/ui/commitsplit_repro_test.go`
   (`TestCommitSplitHalfCompleteLosesFuture`), ran — observed
   `master occurrences=2, futureExists=false`.

3. **`editTodoDetachForm` loses the edited occurrence when the standalone write fails after the series advanced** — `internal/ui/recur_edit.go:152`.
   "Edit this occurrence" for a recurring todo Puts the advanced series first
   (`AdvanceRecurringTodo`, consuming the current occurrence), then Puts the detached
   standalone carrying the edits. If the second Put fails there is no rollback and no undo:
   the occurrence is gone from the series and never became a one-off task — contradicting
   the confirm dialog's promise ("it becomes a separate one-off task").
   Repro: `internal/ui/detach_dataloss_repro_test.go`
   (`TestEditTodoDetachDataLossOnSecondPutFailure`), ran — observed the detached
   occurrence (due 2026-07-06) exists nowhere; series advanced to 07-13; no undo pushed.
   NOTE: this repro replicates the buggy ordering — it cannot be turned green by fixing
   the function; replace with a recovery-asserting test when the fix lands.

### MED

4. **`cancelGrab` swallows Restore/Delete errors and always reports success** — `internal/ui/grab.go:378`.
   `cancelGrab` discards the error returns of `store.Delete`/`store.Restore` (`_, _ =`) and
   unconditionally flashes "Grab cancelled". On a this-&-future grab, if the master
   `Restore` (un-cap) write fails after the new tail series was deleted, the grabbed
   occurrence AND all future occurrences are gone while the user is told the series is
   intact. Same class for a normal grab (`grab.go:385`).
   Repro: `internal/ui/grab_cancel_repro_test.go`
   (`TestGrabFutureCancelRestoreFailureIsSilent`), ran — flashed "Grab cancelled" while the
   master stayed capped at 2 occurrences instead of the restored 4.

5. **Detached recurring-todo occurrence drops unmodeled iCal properties (iron-rule violation)** — `internal/ui/recur_edit.go:160`.
   "Edit this occurrence" builds the standalone via `model.NewTodoObject(d, now)` from a form
   draft (modeled fields only) rather than cloning the original component, so every unmodeled
   prop (VALARM, ATTACH, URL, GEO, X-, non-PARENT RELATED-TO) is dropped from the one-off task.
   The parallel event path (`AddOccurrenceOverride`/`cloneOverrideComponent`) correctly clones.
   Repro: `internal/model/detach_repro_test.go`
   (`TestDetachOccurrenceDropsUnmodeledProps`), ran — the encoded standalone lacks
   `X-APPLE-SORT-ORDER` and the whole `VALARM`/`TRIGGER` block.

### LOW

6. **`grabNudge` read-modify-write (Locate→Put) has no version check** — `internal/ui/grab.go:189`.
   `grabNudge` Locates, derives `newObj` from that snapshot, then Puts with no unchanged-check.
   A background sync that pulls a remote edit into the same resource in that window is
   overwritten: `Put`'s `build(prev)` adopts the pulled ETag while writing stale-derived
   content, marking it Dirty; the next push's ETag CAS matches and clobbers the server copy.
   Systemic Locate-then-Put pattern (shared with quick-field edits / completion toggles),
   observed here. Timing-sensitive.
   Repro: `internal/store/grabclobber_repro_test.go`
   (`TestGrabNudgeClobbersConcurrentPull`), ran — server summary edit lost, resource adopted
   the pulled ETag and is Dirty.

7. **Todo nudge doesn't re-check `HasDue` after re-locate** — `internal/ui/grab.go:209`.
   `startGrab` gates on `HasDue`, but `grabNudge`'s todo branch rebuilds `draftFromTodo`
   without re-checking; if a concurrent sync cleared the due date, the nudge adds days to a
   zero time (year-1), `EditTodo` writes `HasDue=false` (no due persisted), and the flash
   reads a nonsensical "due Jan 1, year 1" — a confusing no-op that looks like it moved.
   Repro: `internal/ui/grab_duecleared_repro_test.go`
   (`TestGrabTodoDueClearedMidGrab`), ran — flash claimed a due date while `HasDue=false`.

---

## Mutation-canary results (2 of 3 escaped — test-coverage holes)

Canaries probe whether the *test net* would catch a plausible regression; an escape
means the code is currently correct but a future regression on that exact path would
ship silently.

- **CAUGHT** — `internal/sync/sync.go`: inverting the both-sides-changed conflict
  comparison (`ETag != r.ETag` → `== r.ETag`) failed **5 tests**
  (`TestSyncPushesLocalEdit`, `TestSyncPushDoesNotClobberConcurrentEdit`,
  `TestSyncRefetchesOn412`, `TestSyncConflictKeepsBoth`,
  `TestSyncUnparseableServerConflictNotTreatedAsDeletion`) — both directions covered.

- **ESCAPE** — `internal/ui/calendarview.go` (`handleEventMode`, ~line 173): flipping the
  month-grid event-drill `j`/down boundary guard (`< len(items)-1` → `<= len(items)-1`)
  let `eventIndex` advance one past the last item. `go test ./internal/ui/` PASSED. The
  drill tests set `eventIndex` directly / use Home/End but never step `j`/`k` down to the
  lower boundary. **Suggested test:** drill into a multi-item day, press `j` past the last
  item then `k`, assert the selection stays the last item (`eventIndex` never exceeds
  `len(items)-1`).

- **ESCAPE** — `internal/ui/edit.go` (`clampIndex`, line 1092): changing the upper bound
  (`i >= n` → `i > n`) makes `clampIndex(n, n)` return the out-of-bounds `n` instead of
  `n-1`. `clampIndex` backs vim-count nav (`keys.go`) and drilled-event selection
  (`calendarview.go`/`timegridview.go`); a count landing exactly on the list length (`2j`
  on a 2-item list) would yield an out-of-range index (plausible panic). `go test
  ./internal/ui/` PASSED — no test exercises the `i == n` boundary. **Suggested test:**
  table-driven `clampIndex` over `{i<0, i==0, i==n-1, i==n, i>n}` plus a vim-count test
  where `count-1 == item count`.

---

## Convergence

| Severity | Pass 10 | Pass 11 | Trend |
|---|---|---|---|
| HIGH | 5 | 3 | ↓ |
| MED  | 4 | 2 | ↓ |
| LOW  | 0 | 2 | ↑ |
| **Total** | **9** | **7** | ↓ |

HIGH and MED are both trending **down**, but three HIGH data-loss defects were
confirmed this pass — and all three (plus the MED `cancelGrab`) are the **same
recurring class**: a multi-write operation with no rollback when a later write fails,
while a sibling path (`beginGrabFuture`) already demonstrates the correct guard. The
LOW rise reflects finer-grained probing of a newly-audited modal surface, not new
regressions. **Not converged.**

---

## Residual risk

- **7 confirmed findings remain unfixed** (repros proposed, not committed). The three
  HIGH are silent data loss on realistic failure modes (ENOSPC / permission / crash /
  concurrent sync) — the highest-value items to fix, and they share one fix pattern.
- **2 escaped canaries** are live test-coverage holes in `internal/ui` navigation
  (`handleEventMode` `j`/`k` boundary, `clampIndex` upper edge) — the code is correct
  today but unguarded.
- **The Locate→Put no-version-check pattern is systemic** — this pass observed it in the
  grab path, but it is shared with quick-field edits and completion toggles, none of which
  were audited for the same concurrent-pull clobber.
- **Still never/unaudited:** full `sync-collection` token-delta sync (deliberately
  unbuilt); the Raspberry Pi hardware target (on-device timing, kiosk/autologin, bare-TTY
  color) — needs a physical Pi.
- **Not re-run this pass** (recent, but time-decaying): spec-diff feature-promise (10),
  CLI wiring (9), BuildTree scale / quick-add fuzz (4-5), low-reachability go-ical encoder
  constraints still unhealed (empty VCALENDAR, VTIMEZONE-needs-a-child — acknowledged
  residual).

**Recommendation: more passes recommended** — three HIGH and two MED were confirmed, two
mutation canaries escaped, and high-value surfaces remain never/deferred. Fix the shared
no-rollback data-loss class first, then re-audit the systemic Locate→Put pattern across
the other edit paths.
