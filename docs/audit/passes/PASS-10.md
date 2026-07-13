# Pass 10 — stale-surface sweep: :config reload, encoder round-trip, store atomicity, yank/paste, mouse, spec-diff

- **Date:** 2026-07-13
- **Prior pass:** Pass 9 (pre-1.0 final unhardened-areas audit) — HIGH 5 · MED 6 · LOW 7
- **This pass:** HIGH 5 · MED 4 · LOW 0 (all confirmed with runnable, executed repros)

This pass deliberately targeted the surfaces the ledger still marked **stale** after the
pre-1.0 audit — the ones a "1.0-ready" verdict had skipped — rather than re-covering the
already-recent recurrence / decode / tz / BuildTree paths.

---

## Coverage exercised

| Surface | Package | Method | Result |
|---|---|---|---|
| `:config` reload / $EDITOR flow | internal/ui, internal/config | fault-injection | 1 MED |
| go-ical semantic encoder constraints (re-encode round-trip) | internal/model | fuzz | 4 HIGH + 1 MED |
| Store write pipeline atomicity (.ics + sidecar temp/rename) under disk fault | internal/store | fault-injection | 1 MED |
| Yank/paste cross-list move & copy rollback | internal/ui | data-loss | 1 HIGH + 1 MED |
| Mouse handling (coordinate→pane→mode dispatch) | internal/ui | input-edge | no new finding |
| Feature-promise conformance vs main.md/CLAUDE.md | (whole app) | spec-diff | no new finding |

---

## Confirmed findings (each carries a repro that was run and observed to fail)

### HIGH

1. **VEVENT with both DTEND and DURATION decodes but cannot re-encode** — `internal/model/decode.go:82`.
   Parse's healing enumerates go-ical's cardinality rules but not its mutual-exclusion rules;
   a VEVENT carrying both DTEND and DURATION decodes cleanly, then `Encode()` fails
   (`only one of DTEND and DURATION can be specified`), making the whole `.ics` resource
   unwritable — every later save (`store/mutate.go:101`) or push (`sync/sync.go:573`) of that
   item or any sibling in the same resource silently fails to persist.
   Repro: `TestDTEndAndDurationReEncode` (Decode→Encode), ran, failed as predicted.

2. **VTODO with both DUE and DURATION decodes but cannot re-encode** — `internal/model/decode.go:82`.
   Same healing gap for VTODO; `Encode()` fails `only one of DUE and DURATION can be specified`.
   Repro left in tree: `internal/model/repro_duedur_test.go` (`TestReproVTodoDueAndDuration`), ran, fails at Encode.
   NOTE: this repro is left in the working tree (untracked, not committed) and **will fail `make check` until the heal lands**.

3. **VTODO with DURATION but no DTSTART decodes but cannot re-encode** — `internal/model/decode.go:82`.
   go-ical requires DTSTART when DURATION is present; Parse never enforces the dependency.
   Repro left in tree: `internal/model/repro_durnodtstart_test.go` (`TestReproVTodoDurationNoDTStart`), ran, fails at Encode.

4. **Empty VTIMEZONE cannot be re-encoded — and `stripForbiddenNesting` can create the empty state itself** — `internal/model/decode.go:190`.
   go-ical rejects a VTIMEZONE with zero STANDARD/DAYLIGHT children. Case A: a naturally-empty
   VTIMEZONE alongside a valid VEVENT. Case B: `stripForbiddenChildren` (decode.go:196-209) removes
   a VTIMEZONE's only (forbidden) child, leaving it empty. Both make the sibling VEVENT / whole
   resource unwritable.
   Repro left in tree: `internal/model/emptyvtimezone_repro_test.go` (`TestReproEmptyVTimezoneBlocksEncode`, both subtests), ran, fail at Encode.

5. **Cross-list move drags co-resident todos to the destination and deletes them from the source** — `internal/ui/yankpaste.go:170`.
   `moveSubtree` re-keys the WHOLE multi-component resource object via `Put`, then `Delete` removes the
   entire source `.ics`, so any unrelated VTODO sharing that resource file (a bundled/imported/hand-edited
   vdir `.ics`) is silently relocated to the destination list and erased from the source — and on next
   sync the move propagates to the server. This is the exact data-displacement class the all-or-nothing
   rollback claims to prevent.
   Repro left in tree: `internal/ui/repro_coresident_move_test.go` (`TestReproCoResidentMoveDragsBystander`), ran, fails.

### MED

6. **EDITOR values containing arguments (e.g. `code --wait`) fail the `:config` reload** — `cmd/lazyplanner/main.go:184`.
   `exec.Command(editor, configPath)` treats the whole $EDITOR string as one executable name, so any
   flag-bearing EDITOR (`code --wait`, `subl -w`, `emacsclient -c`, `vim -f`) is ENOENT; `:config` is
   unusable for these common values. Fix: shell-split EDITOR (or run via `sh -c`).
   Repro: observed `editor: fork/exec /bin/echo hello: no such file or directory` (test written, ran, removed).

7. **VJOURNAL/VFREEBUSY carrying nested components decodes but cannot re-encode** — `internal/model/decode.go:190`.
   `allowedChildren` (decode.go:179-183) covers only VEVENT/VTODO/VTIMEZONE, so a VJOURNAL/VFREEBUSY with
   a nested child (e.g. VALARM) is never pruned; `Encode()` fails `nested components are forbidden`, poisoning
   a shared resource. Lower severity because these types are rare in the NextCloud calendars LazyPlanner
   manages, but the round-trip break is real.
   Repro: `TestReproVJournalNestedCannotEncode` (Decode→Encode), ran, failed (test removed after observation).

8. **Crash between the `.ics` rename and the sidecar rename loses the Dirty flag** — `internal/store/mutate.go:73`.
   `writeResourceLocked` renames the `.ics` durably, then writes the sidecar. A crash/power-loss in that
   window (realistic on the stated Pi/SD-card kiosk target) leaves the new `.ics` content on disk but the
   old sidecar (Dirty=false, prior ETag). On reload the edit looks clean-and-synced; sync reconcile
   (`sync.go:401-409`) never pushes it and the server keeps the stale version forever. Symmetric delete case:
   the `.ics` is removed before the tombstone is written, so the next sync re-pulls and silently undoes the
   deletion. The mutate.go:62-63 comment overstates the guarantee (covers a returned error, not a mid-write crash).
   Repro: `TestCrashBetweenIcsAndSidecarRenameLosesDirty` — after simulated crash, resource loads Dirty=false /
   ETag==server and `HasLocalChanges` returns false; ran, failed (test removed after observation).

9. **Copy duplicates co-resident sibling todos into the destination with their ORIGINAL UIDs** — `internal/ui/yankpaste.go:235`.
   `copySubtree` passes the whole multi-component resource to `model.CopyTodo` (which re-keys only the target
   component) and Puts it under the new single-todo resource name, so every other todo sharing the source `.ics`
   is duplicated into the destination calendar retaining its original UID — a phantom copy in an untouched list
   and a duplicate-UID-across-collections integrity violation on push.
   Repro left in tree: `internal/ui/bundled_copy_repro_test.go` (`TestCopyBundledSiblingRepro`), ran, fails.

**Findings 5, 7, and 9 share one root cause with the ingest healers:** the app treats a single CalDAV
resource as one object but the create/move/copy write paths and the encoder both assume single-component
resources. A bundled `.ics` (RFC-legal, and a supported vdir/import input) breaks the iron rule in two
directions — unencodable on ingest, and split/duplicated on structural edits.

> Repros left untracked in the working tree (fail `make check` until fixed; not committed):
> `internal/model/repro_duedur_test.go`, `internal/model/repro_durnodtstart_test.go`,
> `internal/model/emptyvtimezone_repro_test.go`, `internal/ui/repro_coresident_move_test.go`,
> `internal/ui/bundled_copy_repro_test.go`.

---

## Mutation-canary results (tests the net, not the code)

4 canaries injected across the audited packages; **3 genuine escapes** (1 canary produced no
signal — see note).

1. **ESCAPED — `internal/ui/search.go:97`**: removing `+ len(matches)` from the backward-wrap modulo
   in `searchNext()` still passed `go test ./internal/ui/`. `TestSearchTasksJumpsAndCycles` only drives
   `searchNext(1)` (forward `n`); the `N`-key negative-index wrap that panics at runtime is untested.

2. **ESCAPED — `internal/model/todo.go`**: dropping the `n > 9` upper clamp in `priority()` still passed
   `go test ./internal/model/`. No fixture or test exercises an out-of-range PRIORITY, so a corrupted
   smart-sort ranking would ship undetected.

3. **ESCAPED — `internal/store/calendar.go`**: changing `HasPendingChanges` from `r.Dirty || r.Href == ""`
   to just `r.Dirty` still passed `go test ./internal/store/`. No store-package test seeds a clean, href-less
   pull-orphan; the sibling `HasLocalChanges` has zero store-package tests (covered only indirectly via
   internal/sync).

**No-signal canary (not counted as an escape):** the `internal/ui,internal/config` canary reported
`caught:false` only because its worktree was checked out at a docs-only commit (89e5d21) with no `.go`
files — an environment/setup issue, not a coverage hole. It must be re-run from a base containing the
implementation.

Each escape is now recorded as a declared blind spot in `COVERAGE.md`; closing them is a one-test-each fix.

---

## Convergence

Total confirmed findings fell 18 → 9 (LOW went 7 → 0, MED 6 → 4), so severity volume is trending down.
But **HIGH held flat at 5** and the newly-opened HIGH class (decode-but-unencodable bundled/foreign `.ics`
and co-resident-bundle structural edits) is a real iron-rule breach, not tail risk. The code is **not**
converged: this pass found HIGHs on the first serious look at stale surfaces, which argues the earlier
"1.0-ready" framing was premature for these inputs.

## Residual risk (uncovered / unknown)

- **Raspberry Pi on real hardware** — never covered; needs a physical Pi.
- **Full `sync-collection` incremental (token delta)** — deliberately unimplemented; nothing to audit yet.
- **Sync concurrency (-race) and sync data-loss/TOCTOU** — last touched pass 3; not re-run this pass.
- **Grab-mode temporal-manipulation state machine** — no dedicated input-edge/data-loss fuzz this pass.
- **UI draw paths and the general key/chord dispatcher panic surface** — not re-covered (passes 6/9); this
  pass touched only the mouse and yank/paste corners.
- **Three canary escapes** — backward-search wrap, PRIORITY>9 clamp, and the href-less pull-orphan clause are
  all currently untested regressions-in-waiting.
