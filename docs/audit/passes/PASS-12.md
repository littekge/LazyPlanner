# Pass 12 — the Locate→Put systemic follow-up + undo replay + color/name PROPFIND parse

- **Date:** 2026-07-15
- **Run size:** 48 agents
- **Prior pass:** Pass 11 (never/stale sweep: grab + recurrence-edit UI + sync concurrency) — HIGH 3 · MED 2 · LOW 2
- **This pass:** HIGH 2 · MED 5 · LOW 0 (all 7 confirmed with runnable repros executed and observed red)
- **Status:** NONE fixed yet. Every repro was written, run, and observed to fail against
  the current code; several are proposed regression tests, some of which replicate the
  buggy sequence and must be adapted to assert recovery once the fix lands.
- **Resolution (2026-07-15):** ALL 7 findings + all 3 escaped canaries fixed, each
  adversarially verified with a regression test and its own commit; full gate + `-race`
  on caldav/state/store/sync/ui pass. New primitives: `store.RestoreDirty`,
  `caldav.hrefKey`, `model.isCompletedStatus`; the three systemic Locate→Put sites now
  route through `store.PutIfUnchanged`. A stale hallucinated repro the audit left in
  `internal/model` (`zz_completed_test.go`, wrong Parse/Encode API) was removed. See
  `log.md` for per-fix entries and `COVERAGE.md` for the resolved ledger.

This pass took the pass-11 named follow-up head-on — the systemic Locate→Put
no-version-check read-modify-write shared beyond the grab path — and paired it with the
under-covered edit/undo/parse surfaces the ledger flagged stale/never: the session undo
stack, the calendar color + display-name PROPFIND decode, and (fuzz/edge) the state-file
parse and `:calendar` argument parsing.

It deliberately did NOT re-cover the pass-11-recent surfaces (grab-mode, recurrence-edit
orchestration, sync concurrency/CTag/background goroutines) or the pass-10-recent ones
(store atomicity, config/$EDITOR, mouse, yank/paste, spec-diff), nor the fuzz-recent
model surfaces (decode/heal, recurrence read/write, tz/DST, subtask tree, quick-add).

---

## Coverage exercised

| Surface | Package | Method | Result |
|---|---|---|---|
| Quick-field edits (`sp`/`sd`) — Locate→Put write | internal/ui, internal/model | data-loss | 1 HIGH + 2 MED |
| Completion toggle (`Space`) + recurring-todo advance | internal/ui, internal/model | data-loss | 1 MED |
| Session undo stack (pushUndo / `Restore` replay) | internal/ui, internal/store | data-loss | 1 HIGH + 1 MED |
| Calendar-color + display-name PROPFIND parse (`discoverColors`) | internal/caldav | fault-injection | 1 MED |
| State-file load/parse (widths/hidden-cals/hour-zoom) | internal/state | fuzz (adversarial values) | no parse finding (canary hole below) |
| `:calendar` command argument parsing | internal/ui | input-edge | no finding |

---

## Confirmed findings (each carries a repro that was run and observed red)

### HIGH

1. **Quick `sp`/`sd` on an IN-PROCESS/CANCELLED task flattens STATUS and drops PERCENT-COMPLETE** — `internal/ui/quickfield.go:18` → `internal/model/edit.go:80,295-297`.
   `draftFromTodo` collapses the VTODO's tri/quad-state STATUS into a single `Completed`
   bool (`td.Completed()` is false for any non-COMPLETED status), so `EditTodo`→
   `setCompleted(false,…)` rewrites STATUS to NEEDS-ACTION and `Del`s PERCENT-COMPLETE for
   any task NextCloud/another client left at IN-PROCESS/50% or CANCELLED. A priority/due
   bump silently rewrites unrelated state and pushes it to the server — a direct breach of
   the quick-set's "change one field, leave everything else intact" contract and the iron
   rule.
   Repro: `internal/model/repro_quickfield_status_test.go`
   (`TestReproQuickFieldStatusLoss`), ran — output showed STATUS:NEEDS-ACTION (IN-PROCESS
   lost) and PERCENT-COMPLETE removed. **Repro caveat:** two pre-existing UNTRACKED stale
   test files (`zz_completed_test.go`, `repro_completed_test.go`) use an outdated
   Parse/Encode API and break the `internal/model` package build; they must be removed/fixed
   before the repro compiles.

2. **Undo of a delete after the delete has synced silently loses the resurrected item** — `internal/store/mutate.go:225` (routed via `internal/ui/edit.go:950` undoLast → `internal/sync/sync.go:394`).
   `store.Restore` replays the undo snapshot verbatim with `Dirty=false` + old Href/ETag.
   If the delete's tombstone was already pushed (debounced push, >3s), the restored resource
   is a clean, Href-bearing, server-absent orphan → `reconcileCalendar`'s `case !onServer:`
   `Forget`s it on the next sync. The item the user explicitly restored disappears
   permanently and is never re-created on the server. Fix direction: a resurrection must be
   marked `Dirty=true` so sync re-creates it / raises a conflict rather than dropping it.
   Repro: `internal/sync/undo_after_delete_sync_repro_test.go`
   (`TestUndoOfSyncedDeleteSurvivesNextSync`), ran — after Restore the resource is clean with
   its old Href/ETag; after the next sync it is absent from both store and server. **This
   repro was written, run red, then removed** — re-add alongside the fix.

### MED

3. **Quick `sp`/`sd` on a completed task restamps COMPLETED to now** — `internal/model/edit.go:293`.
   For a completed task `draftFromTodo` sets `Completed=true`; `EditTodo`→`setCompleted`
   unconditionally `setDateTimeUTC(COMPLETED, now)`, so editing priority/due overwrites the
   real completion date with the edit time and propagates it to the server — silent,
   permanent history corruption.
   Repro: `internal/model/repro_completed_test.go`
   (`TestReproQuickFieldSetPreservesCompleted`), ran — `COMPLETED` rewritten `20250101…` →
   `20260715…`. Same stale-untracked-file build caveat as finding 1.

4. **`applyTodoField` (sp/sd) uses `store.Put`, not `PutIfUnchanged` — TOCTOU lost edit** — `internal/ui/quickfield.go:55`.
   Locate (l.35) → draft/EditTodo (CPU) → `store.Put` (l.55) with no version guard. A
   background pull landing in that window makes `Put`'s `build(prev)` adopt the freshly
   pulled ETag onto stale-derived content and mark it Dirty; the next push's ETag CAS matches
   the server and clobbers its edit. `grab.go:304` avoids this with `PutIfUnchanged(loc.Prev)`;
   quickfield does not.
   Repro: `internal/store/quickfieldclobber_test.go`
   (`TestQuickFieldSetClobbersConcurrentPull`), ran red — stale content wears the pulled
   `etag-v2`, Dirty=true; server edit lost. Modeled on `grabclobber_test.go`.

5. **Space-complete and recurring-todo advance use unguarded `Put`** — `internal/ui/edit.go:325` and `internal/model/recur_edit.go:262` (advance).
   Same TOCTOU class as #4 on the completion path: `toggleComplete` (Locate l.295 → Put
   l.325) and `advanceRecurringTodo` build from a stale Located snapshot and `store.Put`,
   so a concurrent pull's remote edit is silently overwritten with no conflict flag. Narrow
   (Locate→Put is CPU-only) but silent data loss when hit.
   Repro: `internal/store/completeclobber_test.go`
   (`TestSpaceCompleteClobbersConcurrentPull`), ran red — pulled remote DESCRIPTION lost,
   resource wears `etag-v2` Dirty=true.

6. **Undo of an edit after it synced doesn't stick** — `internal/store/mutate.go:225`.
   `Restore` re-applies the pre-edit content with the OLD ETag + `Dirty=false`. If the edit
   already pushed (server at a newer ETag), the next reconcile hits
   `case serverObj.ETag != r.ETag:` and pulls the server copy back over the undo — the
   revert is silently reverted. (Data-integrity/UX; the server content survives, so not
   destructive.) Same root cause reaches grab `commitGrab` undo and recur-edit
   delete/edit undo.
   Repro: `internal/sync/undo_after_edit_sync_repro_test.go`
   (`TestUndoOfSyncedEditSurvivesNextSync`), ran red — after the second sync the local
   summary is "Edited" not the undone "Original".

7. **`discoverColors` keys by raw href; `DiscoverCalendars` looks up by decoded path** — `internal/caldav/colors.go:53` (lookup at `client.go:219`).
   The color map is keyed on the raw `<href>` text; go-webdav produces `dc.Path` by
   percent-decoding via `url.Parse(href).Path`. Any percent-encoded segment or absolute-URL
   href (Google `user%40gmail.com`, a NextCloud URI with `%20`, a proxy-rewritten
   `https://host/…`) yields a key that can never match → `Calendar.Color` stays `""` with no
   error surfaced. The same raw-href keying divergence is in `privileges.go` (read-only
   detection fails open) and `ctag.go`.
   Repro: `internal/caldav/colors_encoding_repro_test.go`
   (`TestDiscoverColorsPercentEncodedHrefMismatch`), ran red —
   `colors["/caldav/v2/user@gmail.com/events"] = ""`, key stored was the `%40` form.
   **Written, run red, then removed** — re-add a corrected version with the fix.

---

## Mutation-canary results — 3 of 3 escaped (all test-coverage holes)

Canaries probe the *test net*; an escape means the code is correct today but a future
regression on that exact path would ship silently.

- **ESCAPE** — `internal/ui/grab.go` (grabNudge J/K resize): weakening the min-duration
  guard `!d.End.After(d.Start)` → `d.End.Before(d.Start)` (allowing a zero-duration event
  via `K`-resize to `end == start`) — `go test ./internal/ui/` PASSED. The only resize test
  (`TestGrabEventMoveResizeCancel`) grows the end with one `J` and never shrinks to the
  equal boundary. **Suggested test:** `K`-resize an event down to `end == start` and assert
  the guard rejects it (end must be strictly after start).
- **ESCAPE** — `internal/state/state.go` (`Save`): replacing the temp-file+rename with a
  direct `os.WriteFile(path,…)` — `go test ./internal/state/` PASSED. The doc comment
  promises "writes to a temp file and renames so a crash never leaves a half-written state
  file," but no test asserts the atomicity (no `.tmp` sibling check, no
  never-partially-observed check). **Suggested test:** assert `Save` never leaves a
  half-written `state.json` (e.g. inspect the temp/rename behavior or a fault-injected
  mid-write).
- **ESCAPE** — `internal/caldav/privileges.go` (`writable()`): dropping the
  `p.WriteContent != nil` term from the OR-chain — `go test ./internal/caldav/` PASSED.
  The only writability fixture grants BOTH `write` AND `write-content`, so removing either
  term individually leaves it writable via the survivor. A write-content-only or bind-only
  share (a common NextCloud share grant) would be misclassified read-only, silently blocking
  all writes. **Suggested test:** table cases asserting each of write / write-content / bind
  / all independently yields writable, plus a read-only-only case.

---

## Convergence

| Severity | Pass 11 | Pass 12 | Trend |
|---|---|---|---|
| HIGH | 3 | 2 | ↓ |
| MED  | 2 | 5 | ↑ |
| LOW  | 2 | 0 | ↓ |
| **Total** | **7** | **7** | flat |

HIGH is trending **down** (3→2) but MED **rose** (2→5) and the total is flat. The rise is
concentrated, not diffuse: four of the seven findings (#1,#3,#4,#5 and the store-side #2/#6)
are two recurring root-cause classes — the systemic **Locate→Put no-version-check** the
pass-11 report predicted, and the **`Restore` replays clean+stale** undo defect — plus the
STATUS-flatten iron-rule breach and the raw-href keying bug that also affects privileges
and CTag. This confirms the pass-11 prediction rather than surfacing new territory.
**Not converged.**

---

## Residual risk

- **7 confirmed findings remain UNFIXED** (repros proposed; some removed pending the fix,
  some replicate the buggy sequence and must be adapted to assert recovery). The two HIGH
  are silent, permanent data loss on realistic paths (a foreign-client task's state
  flattened by a routine `sp`; an explicit undo-of-a-synced-delete vanishing on next sync).
- **3 escaped canaries** are live test-coverage holes: grab J/K min-duration guard, state
  `Save` atomicity, privileges `write-content` term — all correct today, all unguarded.
- **The Locate→Put pattern is now confirmed systemic** across grab (fixed p11), quick-field,
  completion, and advance — three call sites still on plain `Put`. `store.PutIfUnchanged`
  exists; routing them through it is the direct fix.
- **The raw-href keying divergence is shared** by `colors.go`, `privileges.go`, and
  `ctag.go` — the color repro is proven; the privilege/CTag mismatches are analyzed but not
  independently repro'd this pass (flagged, not counted).
- **Not audited this pass** (accepted gaps): whole-app **spec-diff was NOT re-run** even
  though pass-11 added new invariants (PutIfUnchanged routing, DetachTodoOccurrence, sibling
  rollbacks) — a known deferral. State-file parse and `:calendar` args produced no finding
  but only the canary holes above were exercised for atomicity/privilege breadth.
- **Still never/deferred:** full `sync-collection` token-delta sync (deliberately unbuilt);
  the Raspberry Pi hardware target (on-device timing, kiosk/autologin, bare-TTY color) —
  needs a physical Pi.

**Recommendation: more passes recommended** — two HIGH and five MED confirmed, all three
mutation canaries escaped, and a systemic Locate→Put class plus a shared raw-href keying bug
span multiple call sites still unfixed. Fix the Locate→Put trio via `PutIfUnchanged`, make
undo `Restore` mark resurrections Dirty, decode the href before keying the color/privilege/
CTag maps, and stop `setCompleted` from flattening non-COMPLETED status / restamping
COMPLETED — then re-run spec-diff against the new invariants.
