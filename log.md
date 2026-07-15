# LazyPlanner — Change Log

> Append a new entry every time a change is made. Newest entries at the top.

---

## 2026-07-15 — Pass 11 fix (HIGH): detaching a recurring-todo occurrence rolls back on a failed standalone write

- Fixes pass-11 HIGH #3 (`internal/ui/recur_edit.go` `editTodoDetachForm`): "edit this occurrence" for a recurring todo Puts the advanced series first (`AdvanceRecurringTodo` consumes the current occurrence), then Puts the detached standalone one-off carrying the edits. If the second Put failed there was no rollback and no undo — the occurrence was gone from the series and never became a one-off task, contradicting the confirm dialog's promise ("it becomes a separate one-off task"). Silent data loss on ENOSPC/permission/crash.
- **Fix:** extracted the store side into `commitDetach` (mirroring `commitSplit`), which on a failed standalone write `Restore`s the series from `loc.Prev` so the detach is atomic (both writes land or neither).
- Tests: `internal/ui/detach_rollback_test.go` (`TestCommitDetachRollsBackSeriesOnStandaloneWriteFailure`) — forces the standalone Put to fail (directory planted at its path) and asserts the series' due is unchanged (not advanced) and the standalone wasn't left behind. Verified red without the rollback (series advanced a week), green with it. Full gate on ui passes.
- Files: `internal/ui/recur_edit.go`, `internal/ui/detach_rollback_test.go`.

## 2026-07-15 — Pass 11 fix (HIGH): commitSplit rolls back the capped master when the future write fails

- Fixes pass-11 HIGH #2 (`internal/ui/recur_edit.go` `commitSplit`): an "edit this & future" event split does `model.SplitEvent` → `Put(capped master)` → `Put(future series)`. The capped Put truncates the master's RRULE (UNTIL just before the occurrence); if the future Put then fails (ENOSPC / permission / sidecar-write), the function returned early on a flash and `pushUndo` was never reached — the master was left **permanently truncated** and the future tail **never created**, unrecoverable from the UI. The sibling grab path (`beginGrabFuture`) already had this rollback; `commitSplit` was left unguarded.
- **Fix:** on a failed future Put, `Restore` the master from `loc.Prev` before returning, so the split is atomic (both writes land or neither), mirroring `beginGrabFuture`.
- Tests: `internal/ui/commitsplit_rollback_test.go` (`TestCommitSplitRollsBackMasterOnFutureWriteFailure`) — the pass-11 repro, adapted to assert recovery: it forces the second Put to fail (a directory planted at the future resource's path) and asserts the master is restored to its full 4 occurrences. Red pre-fix (master stuck at 2), green post-fix. Full gate on ui passes.
- Files: `internal/ui/recur_edit.go`, `internal/ui/commitsplit_rollback_test.go`.

## 2026-07-15 — Pass 11 fix (HIGH): PullRemoteBatch no longer clobbers a concurrent local edit

- Fixes pass-11 HIGH #1 (`internal/store/remote.go`): `PullRemoteBatch`'s per-resource `stageResourceLocked` write was unconditional (`Dirty:false`, no dirty/version check, unlike single-resource `PullRemote`). Sync builds its "new on server" pull list from a **pre-lock snapshot**, so a local edit that lands during step (A)'s network I/O — notably a crash-orphan (clean, href-less `.ics`) the user just re-edited — is invisible to the include-in-batch decision, and the batch write overwrote it and marked it clean. Silent data loss: the edit was gone in memory and on disk and never pushed. The pass-5 comment claiming these writes "can't clobber a concurrent local edit" was **false** for this case.
- **Fix:** each stage now skips a resource that already exists locally and is **Dirty** (a pending local edit), reporting the new sentinel `store.ErrKeptLocalEdit`; the edit survives and the next sync reconciles it (a href-less dirty resource is then a "new local resource, never pushed" → `pushCreate`). A **clean** local resource is still overwritten — that's the intended pass-5 crash-orphan self-heal (re-pull a clean, href-less `.ics`), so `Dirty` is the exact discriminator. Both callers (`sync.reconcileCalendar`, `sync.Import`) treat `ErrKeptLocalEdit` distinctly — neither count it as pulled/imported nor record it as a skipped failure (mirroring single-resource `PullRemote`'s silent `!applied`).
- Corrected the false pass-5 comment and the `PullRemoteBatch` doc comment to describe the guard.
- Tests: `internal/sync/pullbatch_clobber_test.go` (`TestReproPullBatchClobbersConcurrentEditToOrphan`) — the pass-11 repro, now green: an orphan edited during a sibling's in-flight PUT keeps its `"user-edit"` content and stays Dirty. Was red pre-fix. Full gate + `-race` on store/sync pass.
- Files: `internal/store/remote.go`, `internal/sync/sync.go`, `internal/sync/import.go`, `internal/sync/pullbatch_clobber_test.go`.

## 2026-07-13 — Docs: record pass 10 + the audit workflow across main/README/CLAUDE

- End-of-session doc refresh (no code change). `main.md`: added the Pass 10 entry and an "Audit tooling" note, corrected the "Not yet audited" section (the go-ical encoder healing it listed as unfixed is now done; added the stale surfaces — grab-mode, sync concurrency/TOCTOU — as the next targets). `README.md`: nine→ten hardening passes, softened "1.0-ready" to "hardening-ongoing, not yet 1.0-blessed" (pass 10 did not converge), added a "Hardening audits" subsection pointing at `/audit` and `docs/audit/`. `CLAUDE.md`: added the audit-tooling note to the Phase line (run `/audit`, keep `docs/audit/COVERAGE.md` current, treat a workflow summary as unverified until checked).
- Files: `main.md`, `README.md`, `CLAUDE.md`, `log.md`.

## 2026-07-13 — Pass 10 fix: close the 3 mutation-canary test-coverage holes

- Adds the missing regression tests the pass-10 canaries exposed (the code was already correct; the *tests* didn't cover these paths, so a future regression would ship silently):
  - **Backward search wrap** (`internal/ui/searchwrap_test.go`): drives `searchNext(-1)` from the first match; asserts it wraps to the last and cycles — guards the `(idx + dir + len) % len` negative-index path (a `+len` regression panics on `N`).
  - **PRIORITY out of range** (`internal/model/priorityrange_test.go`): PRIORITY `15`/`10`/`-1` parse to `PriorityUndefined`, `5` preserved — guards `priority()`'s `>9` clamp.
  - **Href-less pull orphan** (`internal/store/pendinghrefless_test.go`): a clean, href-less resource makes `HasLocalChanges`/`HasPendingChanges` true — guards the `|| r.Href == ""` reconcile clause (previously untested in the store package).
- Verified the net now has teeth: re-applied the priority canary mutation (`>9`→`>100`) and confirmed the new test **fails**, then reverted and confirmed it passes. Full gate passes.
- Files: `internal/ui/searchwrap_test.go`, `internal/model/priorityrange_test.go`, `internal/store/pendinghrefless_test.go` (all test-only).

## 2026-07-13 — Pass 10 fix (MED): reconcile a crash between the .ics and sidecar renames

- Fixes pass-10 MED #8 (edit half): `writeResourceLocked` renames the `.ics` durably, then writes the sidecar. A crash/power-loss in that window (a real Pi/SD-card risk) left the new `.ics` beside the old sidecar (`Dirty=false`, prior ETag), so on reload the offline edit looked clean-and-synced and sync **never pushed it** — silent data loss.
- **Fix:** the sidecar now records a per-resource content hash (`resourceMeta.Hash`, fnv-64 of the exact bytes written, set in `stageResourceLocked`). On load, if the `.ics` hash differs from the sidecar's recorded hash, the `.ics` was rewritten after the sidecar (the crash window) → the resource loads **Dirty** so sync pushes it. Empty hash (legacy sidecar / untracked resource) is not enforced, so it's backward-compatible and doesn't disturb the pass-5 href-less pull-orphan clause (that path has no recorded hash).
- **Delete half — deliberately not "healed":** the symmetric case (`.ics` removed before the tombstone) currently re-pulls on next sync, which is **safe and recoverable** (the item returns; no data lost). Synthesizing a tombstone from a missing-`.ics`-with-href would risk *deleting server data* whenever a `.ics` merely went missing for another reason, so the safe re-pull is kept. Documented.
- Corrected the `writeResourceLocked` doc comment that overstated the guarantee ("a crash can never leave … inconsistent") to describe the hash-reconcile.
- Tests: `internal/store/crashreload_test.go` — a synced resource whose `.ics` is overwritten (sidecar untouched) reloads Dirty and `HasPendingChanges` true; a clean reopen is not spuriously dirty. Full gate + `-race` on store/sync pass.
- Files: `internal/store/{store,mutate,sidecar}.go`, `internal/store/crashreload_test.go`.

## 2026-07-13 — Pass 10 fix (MED): :config honors a flag-bearing $EDITOR

- Fixes pass-10 MED #6: `:config` ran `exec.Command(editor, path)` with the whole `$EDITOR` string as the binary name, so any value carrying arguments — `code --wait`, `subl -w`, `emacsclient -c`, `vim -f` — failed with ENOENT and made `:config` unusable for those common editors.
- **Fix:** extracted `editorCommand(editorEnv, path)` which splits `$EDITOR` on whitespace into command + args (defaulting to `vi` when empty), so flags stay arguments. (Whitespace-in-path editor values remain unsupported — rare, and shelling out via `sh -c` would cost portability on the Windows target; documented.)
- Tests: `cmd/lazyplanner/main_test.go` `TestEditorCommandSplitsArgs` — `code --wait` → `[code --wait /cfg]`, bare `vim`, `emacsclient -c`, and the empty→`vi` default. Full gate passes.
- Files: `cmd/lazyplanner/main.go`, `cmd/lazyplanner/main_test.go`.

## 2026-07-13 — Pass 10 fix (HIGH + MED): yank/paste operates per-component on bundled resources

- Fixes the pass-10 bundled-resource data-loss class. LazyPlanner writes one item per `.ics`, but a foreign/hand-edited resource can bundle several top-level todos; `moveSubtree`/`copySubtree` acted on the whole `loc.Object`, so a cross-list **move** dragged co-resident bystanders to the destination and deleted them from the source (HIGH #5), and a **copy** duplicated them into the destination with their **original UIDs** (MED #9 — a phantom copy + a duplicate-UID-on-push integrity break).
- **New model primitives** (`internal/model/edit.go`): `IsolateComponent` (a copy holding only the selected item, co-resident sibling items dropped, non-item components like VTIMEZONE kept) and `RemoveComponent` (the object without the item, reporting whether any item remains). Both clone-first, so the store snapshot is never mutated.
- **Wiring** (`internal/ui/yankpaste.go`): copy isolates the item before `CopyTodo`; move isolates before the destination `Put`, and on the source side removes only that item — **rewriting** the resource when siblings remain, deleting the file only when it was the last item. Rollback/undo restore the full original either way. The normal one-item-per-file case is unchanged (isolate = identity, remove → empty → delete).
- Tests: the two untracked ui repros (`repro_coresident_move`, `bundled_copy_repro`) are now green; added `internal/model/isolate_test.go` (IsolateComponent drops siblings + doesn't mutate input; RemoveComponent reports remaining correctly). Full gate + `-race` on ui/store pass; the whole tree is green again.
- Files: `internal/model/edit.go`, `internal/model/isolate_test.go`, `internal/ui/yankpaste.go`, `internal/ui/{repro_coresident_move,bundled_copy_repro}_test.go`.

## 2026-07-13 — Pass 10 fix (HIGH x4 + MED x1): heal decode-but-unencodable go-ical shapes

- Fixes the pass-10 encoder-constraint class — objects that decode but fail `Encode()`, poisoning the whole resource (every edit/push re-encodes). All reachable only via a foreign/bundled/hand-edited `.ics` (LazyPlanner never writes these shapes), but each breaks the iron rule for those inputs. Extended `model.Parse`'s ingest healers (add-only/drop-redundant, never mangle real data):
  - **`healComponentConstraints`** — drops a redundant `DURATION` when the encoder's mutual-exclusion/dependency rules would reject it: VEVENT with `DTEND`+`DURATION`; VTODO with `DUE`+`DURATION`; VTODO with `DURATION` but no `DTSTART`. DTEND/DUE (what the typed parser reads) is kept.
  - **`dropEmptyTimezones`** — drops a `VTIMEZONE` with no STANDARD/DAYLIGHT child (natural, or left childless after `stripForbiddenNesting`); runs *after* strip. An empty VTIMEZONE has no offset data and the app resolves zones via the embedded tz DB, so nothing usable is lost.
  - **VJOURNAL/VFREEBUSY nesting** — added empty allow-sets to `allowedChildren` so `stripForbiddenChildren` strips their (encoder-forbidden) nested components.
- Tests: the three untracked repro files are now green regression tests (`repro_duedur`, `repro_durnodtstart`, `emptyvtimezone_repro`); added `heal_encoder_test.go` covering DTEND+DURATION and VJOURNAL/VFREEBUSY (whose workflow repros were run-then-removed). Full gate passes (the remaining red is the yank/paste repros, fixed next).
- Files: `internal/model/decode.go`, `internal/model/{repro_duedur,repro_durnodtstart,emptyvtimezone_repro,heal_encoder}_test.go`.

## 2026-07-13 — Hardening pass 10: stale-surface sweep (via the hardening-audit workflow) — findings pending fixes

- First run of the new `hardening-audit` workflow (63 agents, ~2.5M tokens, ~22 min). It targeted the surfaces the ledger still marked **stale** after pass 9. **9 findings confirmed with executed, RED repros (5 HIGH, 4 MED)** + **3 escaped mutation canaries** (test-coverage holes). Full report: `docs/audit/passes/PASS-10.md`; ledger updated in `docs/audit/COVERAGE.md`.
- **HIGH (all iron-rule / data-loss, reachable via a foreign/bundled `.ics` — LazyPlanner never writes these shapes itself):** four decode-but-unencodable go-ical shapes the pass-4 healers don't cover (VEVENT DTEND+DURATION, VTODO DUE+DURATION, VTODO DURATION-without-DTSTART, empty VTIMEZONE — incl. one `stripForbiddenNesting` self-inflicts), each poisoning a whole resource on every re-encode; and cross-list yank/paste **move** dragging co-resident todos out of a bundled resource + deleting the source.
- **MED:** `:config` reload fails for a flag-bearing `$EDITOR` (`code --wait`) — `exec.Command` treats the whole string as one binary; VJOURNAL/VFREEBUSY nested-child unencodable; a crash between the `.ics` rename and the sidecar rename loses the Dirty flag (offline edit never synced / delete silently undone — a real Pi/SD-card risk); copy duplicates co-resident bundled todos with their original UIDs.
- **Canary escapes (one test each closes them):** backward-search wrap (`searchNext(-1)`) untested; VTODO PRIORITY `>9` clamp untested; `HasPendingChanges`/`HasLocalChanges` href-less pull-orphan clause untested in the store package.
- **Convergence:** total findings 18→9 (LOW 7→0, MED 6→4) but **HIGH held at 5** and opened a new iron-rule class — **not converged**; the prior "1.0-ready" framing was premature for foreign/bundled `.ics` inputs.
- **Process notes:** the synthesis report over-claimed the repros were "committed" — verified false (they're untracked); corrected the wording. Cleaned up 3 stray canary git worktrees the run left behind. One canary was a no-signal false alarm (its worktree checked out a docs-only commit) — not counted. **No fixes applied yet** — the 5 repro test files are left untracked (they fail `make check`) pending a decision on the fix program; the committed tree stays green.
- Files (committed): `docs/audit/COVERAGE.md`, `docs/audit/passes/PASS-10.md`. Untracked (evidence): `internal/model/{repro_duedur,repro_durnodtstart,emptyvtimezone_repro}_test.go`, `internal/ui/{repro_coresident_move,bundled_copy_repro}_test.go`.

## 2026-07-13 — Tooling: /audit slash-command wrapper for the hardening-audit workflow

- Added `.claude/commands/audit.md` — a thin `/audit` slash command that launches the deterministic `hardening-audit` Workflow, giving the `/`-command ergonomics over the code-driven engine. It parses `$ARGUMENTS` into the workflow's `args` (empty = auto-pick least-audited surfaces; `surface [method]` = one explicit target; `key=value` for `maxTargets`/`maxCanaries`/`skeptics`), calls `Workflow({ name: "hardening-audit", args })`, and on completion relays the residual-risk recommendation, findings-with-repros, canary escapes, and any `ENFORCEMENT` warnings — never a bare "clean". Invoking the command is itself the multi-agent opt-in.
- Updated `docs/audit/PROTOCOL.md` "Running it" to show the `/audit` forms alongside the direct `Workflow(...)` calls.
- Files: `.claude/commands/audit.md`, `docs/audit/PROTOCOL.md`.

## 2026-07-13 — Tooling: coverage-first hardening-audit workflow

- Added a reusable multi-agent audit workflow that enforces the evidence-over-verdict protocol (motivated by a prior pass declaring "1.0-ready" while real HIGH bugs sat in un-audited surfaces). Phases: **Plan** (read the coverage ledger + repo, pick the *least-audited* surfaces) → **Audit** (one method per target: fuzz / fault-injection / race / data-loss / input-edge / spec-diff) → **Verify** (N skeptics refute each finding; survivors must carry a repro the verifier actually ran) → **Canary** (inject known bugs in isolated worktrees; the suite must catch them — tests the net, not the code) → **Report** (coverage-ledger update with explicit blind spots, findings with repros, convergence vs last pass, bounded residual risk). It structurally cannot return "clean" — the recommendation enum is `more_passes_recommended` | `residual_accepted_with_caveats` — and an enforcement gate flags a report missing a ledger, "confirmed" findings without an executed repro, or escaped canaries.
- Read-only on the working tree: audits only read, canaries run in disposable git worktrees, only the final synthesis writes (ledger + `passes/PASS-N.md`).
- Files: `.claude/workflows/hardening-audit.js` (JS workflow; syntax-checked wrapped as the runtime executes it), `docs/audit/PROTOCOL.md` (the rules + stop-rule + how to read output), `docs/audit/COVERAGE.md` (living ledger, seeded with the real pass-1..9 state + declared blind spots), `docs/audit/passes/README.md`.
- Invoke: `Workflow({ name: 'hardening-audit' })` (opt into multi-agent first). Not run here — authored only.

## 2026-07-13 — Hardening pass 9 (B2/B4 + audit close-out): CLI password flag guidance

- **B2 (LOW):** the `--password` CLI flag exposes the secret in `ps`/shell history. Kept the flag (dropping it would break documented scripting usage) but its help text now steers users to `$LAZYPLANNER_CALDAV_PASSWORD` and names the exposure. The env var and the config's `password_command` remain the non-exposing paths.
- **B4 (LOW, accepted by design):** `calendar create` slugifies a name to a collection path, and two names differing only in punctuation can slug alike. Left as-is: the server is authoritative on collection paths and rejects a duplicate with a clear error, so no local uniqueness logic is added (which could diverge from the server's own path assumptions).
- This closes out hardening pass 9 (the pre-1.0 audit). Audit items resolved: HIGH H1–H5, MED M1–M6, LOW L5/L6/L8/UI-1/UI-2/B1/B2 + local-read caps, plus the recurrence-mutation fuzz harness. Consciously not changed (documented): L7 (not a bug in practice), B3 (version number = owner's release call), B4 (server-authoritative), Audit-3 UI-3 (already correctly bounds-checked), the `password_command` output size cap (time-bounded; user-owned command).
- Files: `cmd/lazyplanner/conn.go`.

## 2026-07-13 — Hardening pass 9 (L-caps): bound local file reads

- Pre-1.0 audit finding (LOW): unlike the CalDAV response body (capped in pass 7), the local reads did an unbounded `os.ReadFile`/`toml.DecodeFile` — so a huge file, or a **symlink to an endless device** (`/dev/zero`) at any of those paths, could OOM or hang the app. Weaker threat model than the network (these are under the user's own dirs), but cheap symmetry.
- **Fix:** every local read now goes through `io.ReadAll(io.LimitReader(f, cap))`: the state file (4 MiB), `config.toml` (4 MiB, read-then-`toml.Decode`), and the sidecar + each `.ics` (64 MiB, mirroring the network cap). An over-cap file reads bounded bytes that then fail to parse and degrade exactly as a corrupt file already does (zero State / non-fatal `LoadError` / actionable config error). (The `password_command` output remains time-bounded by `WaitDelay`; a size cap there was judged unwarranted for a user-owned command.)
- Tests: `internal/state/statecap_test.go` — a state file symlinked to `/dev/zero` returns a zero `State` within a watchdog instead of hanging (skipped where `/dev/zero` is absent). Full gate passes.
- Files: `internal/state/state.go`, `internal/store/{sidecar,store}.go`, `internal/config/config.go`, `internal/state/statecap_test.go`.

## 2026-07-13 — Hardening pass 9 (B1): CLI reports unknown commands + adds help/version

- Pre-1.0 audit finding (LOW, CLI UX): an unrecognized first argument fell through to `runTUI()`, so a typo like `lazyplanner imprt` silently opened the TUI (exit 0) instead of reporting the mistake; there was also no `help`/`version`.
- **Fix:** extracted the dispatch into a testable `run(args) int`; `main` is now just `os.Exit(run(...))`. Added `help`/`-h`/`--help` and `version`/`-v`/`--version`, and a default branch that prints `unknown command %q` + usage and exits 2. Replaced `exitOnError` with a code-returning `report`. Added `printUsage`.
- **B3 (version string):** left `appVersion` as the owner's release decision (the project isn't released; per the branch rules I don't bump release identifiers), but `version` now makes it queryable.
- Tests: `cmd/lazyplanner/main_test.go` (new — the package had none) — unknown command → exit 2; help/version → exit 0 without launching the TUI; usage lists every subcommand. README updated with the new subcommands. Full gate passes.
- Files: `cmd/lazyplanner/main.go`, `cmd/lazyplanner/main_test.go`, `README.md`.

## 2026-07-13 — Hardening pass 9 (UI-1+UI-2): recurrence-edit UI robustness

- Two LOW UI findings from the input-handler audit:
  - **UI-1 — guard the split's empty result:** `grab.go` and `recur_edit.go` indexed `future.Events[0]` after `model.SplitEvent` without a length check. `SplitEvent` always yields one future event so it's currently unreachable, but the TUI must never index into an empty model result (crash-on-model-data rule). Both sites now flash an error and return if `future.Events` is empty. (Defensive guard; no injection seam for a dedicated test.)
  - **UI-2 — keep the drill on delete-occurrence:** deleting/skipping one occurrence of a recurring item goes through the scope picker (a `pageConfirm`, no form), but the shared `commitMutation` still called `closeModal(pageForm)`. Since the picker's own close already restored focus, that second `restoreFocus` popped an empty focus stack and fell through to the Calendars overview — kicking focus off the drilled calendar day (inconsistent with Space-complete). Added `commitMutationKeepingDrill` (extracted `applyMutation` core, uses `refreshKeepingDrill`, no form close) and routed the three delete/skip/this-and-future-delete paths through it.
- Tests: `internal/ui/recuruidrill_test.go` — deleting an occurrence from a drilled calendar grid keeps focus on the grid (not the Calendars overview) and preserves the drill. Full gate passes.
- Files: `internal/ui/grab.go`, `internal/ui/recur_edit.go`, `internal/ui/edit.go`, `internal/ui/recuruidrill_test.go`.

## 2026-07-13 — Hardening pass 9 (L5+L6): store name-length cap and stale-temp sweep

- Two LOW store-robustness findings, together:
  - **L5 — `SafeName` length cap:** an over-long UID/href (from another client) produced a file name past the filesystem's per-name limit, so that resource silently failed to cache and was retried fruitlessly every sync. `SafeName` now caps the sanitized prefix at `maxSafeNameLen` (200) and appends a deterministic FNV-64 hash suffix — distinct long inputs stay distinct and stable across runs, and the later `.ics` still fits under the common 255-byte limit.
  - **L6 — sweep stale temp files:** `writeFileAtomic` leaves a `.<base>.tmp-*` file if a write is interrupted before its rename. These are never loaded (not `.ics`) but accumulated across crashes (an SD-card concern on the Pi). `loadCalendar` now removes them on open (matched by `isStaleTempName`; real `.ics`/sidecar names never contain `.tmp-`).
- Tests: `internal/store/housekeeping_test.go` — a 1000-char name caps under 255, stays deterministic, and doesn't collide with a different long input; a planted stale temp file is swept on `Open` while the real resource still loads. Full gate passes.
- Files: `internal/store/mutate.go`, `internal/store/store.go`, `internal/store/housekeeping_test.go`.

## 2026-07-13 — Hardening pass 9 (L8): recurring-todo advance honors RDATE past COUNT=1

- Pre-1.0 audit finding (LOW, edge correctness): `AdvanceRecurringTodo` short-circuited "exhausted" on `roption.Count == 1`, ignoring an RDATE. A COUNT=1 todo that also carries an RDATE has a further occurrence, so completing it marked the whole series done one occurrence early.
- **Fix:** dropped the COUNT-only shortcut and always ask the full recurrence set (RRULE + RDATE − EXDATE) for the next instant via `nextInstantAfter`; exhaustion is now "no next instant". A plain COUNT=1 todo still exhausts correctly (no instant after the anchor); COUNT>1 roll-forward is unchanged.
- Tests: `internal/model/advancerdate_test.go` — a `COUNT=1` + `RDATE` todo advances to the RDATE occurrence instead of completing. Existing advance tests still pass. Full gate passes.
- Files: `internal/model/recur_edit.go`, `internal/model/advancerdate_test.go`.
- (Related audit item L7 — a `NewSeriesFrom` COUNT clamp "phantom occurrence" — was examined and found **not reachable**: the split point is always an actual occurrence, so the future half legitimately includes it; the clamp yields the correct count. No change.)

## 2026-07-13 — Hardening pass 9 (M6): harden password_command execution

- Pre-1.0 audit findings (MED/LOW): (1) `ResolvePassword` bounded the command with a context timeout but didn't set `Cmd.WaitDelay`, so a command that leaves a grandchild holding stdout open (e.g. one that backgrounds a process) could make `Output`'s internal `Wait` block past the deadline. (2) A command that exited 0 with no output silently produced an **empty password**, surfacing later only as an opaque auth failure.
- **Fix:** set `c.WaitDelay = passwordCommandTimeout` so a lingering child's pipes are force-closed and it's reaped shortly after cancellation; and treat empty trimmed output as an explicit `password_command produced no output` error instead of an empty secret.
- Tests: `internal/config/config_test.go` `TestResolvePassword` gains a failing-command case, an empty-output case, and a bounded-timeout case (returns promptly under a short deadline). Full gate passes.
- Files: `internal/config/config.go`, `internal/config/config_test.go`.

## 2026-07-13 — Hardening pass 9 (M5): roll back a failed in-app calendar create

- Pre-1.0 audit finding (MED/LOW): `CreateCalendarLocal` set `s.cals[id]` (with `pendingCreate:true`) and made the directory before writing the sidecar, but did not roll back on a sidecar-write failure. The orphan dir and the in-memory phantom lingered; on the next launch the dir loaded with no sidecar → `pendingCreate=false`, so the calendar was silently never `MKCALENDAR`'d on the server (a non-functional collection).
- **Fix:** on sidecar-write failure, `delete(s.cals, id)` and `os.RemoveAll` the directory — but only when the create actually made it (a `freshDir` stat check first), so a pre-existing directory with content is never destroyed by the rollback. A retry after the transient cause clears now succeeds.
- Tests: `internal/store/createrollback_test.go` — a create whose sidecar write fails leaves no phantom calendar, preserves a pre-existing dir's content, and a subsequent create succeeds. Full gate passes.
- Files: `internal/store/calendar.go`, `internal/store/createrollback_test.go`.

## 2026-07-13 — Hardening pass 9 (M3): surface a failed revert instead of swallowing it

- Pre-1.0 audit finding (MED): `revertMutation` — invoked when a sidecar write fails, so the disk is likely already failing (ENOSPC/EACCES) — swallowed the result of its own restore write (`_ = writeFileAtomic`, `_ = os.Remove`). If that restore also failed, the on-disk `.ics` kept the failed-edit content while the in-memory + on-disk sidecar held the prior state; on reload the new content loaded as clean, a silent local edit loss / server divergence with no signal to the caller.
- **Fix:** `revertMutation` now returns the restore error (in-memory restore still always runs); the `revert` closure and both callers (`writeResourceLocked`, `remove`) propagate it, and on a double failure return a distinct "cache may be inconsistent until the next successful sync" error (`errors.Join` of the sidecar + revert errors) instead of hiding it. The common single-failure case (revert succeeds) still returns the plain sidecar error and rolls back cleanly.
- Note: a true double failure requires a disk that fails mid-operation (initial write ok → sidecar write fails → revert write fails), which isn't reproducible with static filesystem permissions (initial-write success implies the dir is writable), so it's verified by inspection; the tests cover the single-failure branch selection + reload consistency.
- Tests: `internal/store/revertsurface_test.go` — a sidecar-only failure yields the clean (non-"inconsistent") error and reloads the reverted content clean; existing `rollback_test.go` still passes (regression guard for the refactor). Full gate + `-race` on store pass.
- Files: `internal/store/mutate.go`, `internal/store/revertsurface_test.go`.

## 2026-07-13 — Hardening pass 9 (M2): store.Open degrades when the cache root is unreadable

- Pre-1.0 audit finding (MED): `store.Open` returned a fatal error when `os.ReadDir(<dataDir>/calendars)` failed for any reason other than not-existing (root is a regular file, a symlink to a non-dir, or permission-denied) — locking the user out of the whole app, inconsistent with `loadCalendar`, which records a per-calendar `ReadDir` failure as a `LoadError` and continues.
- **Fix:** a non-`NotExist` root `ReadDir` error is now recorded as a `LoadError` and `Open` returns an empty store (matching the per-calendar tolerance). The UI surfaces the error; a later sync can repopulate. Safe: an empty store carries no tombstones, so this never deletes server data.
- Tests: `internal/store/openrobust_test.go` — opening a dataDir whose `calendars` entry is a regular file yields a non-fatal empty store with the failure in `LoadErrors`. Full gate passes.
- Files: `internal/store/store.go`, `internal/store/openrobust_test.go`.

## 2026-07-13 — Hardening pass 9 (M1): actionable error on a malformed config.toml

- Pre-1.0 audit finding (MED): a syntax error in `config.toml` aborted startup. Investigated the suggested "fall back to defaults" degradation and **rejected** it: the local cache is namespaced by account (server URL + username), so an unparseable config leaves the account — and thus which cache dir to open — unknown; defaulting would open an empty/wrong-account cache, more confusing than a clear failure. The fatal exit is correct here.
- **Fix:** kept the fatal behavior but made the message actionable — `config %q has a syntax error — fix it and run lazyplanner again: <toml error>` (the toml error already carries line info), and documented the account-cache rationale in-code so it isn't "fixed" into a silent-degrade later.
- Tests: `internal/config/config_test.go` `TestLoadMalformedTOMLIsActionableError` — malformed TOML returns a non-nil error, `configured=false`, and the message names the file. Full gate passes.
- Files: `internal/config/config.go`, `internal/config/config_test.go`.

## 2026-07-13 — Hardening pass 9 (M4): all-day series cap writes a DATE UNTIL, not DATE-TIME

- Pre-1.0 audit finding (MED, interop, confirmed): `CapSeries` set `roption.Until` and let rrule-go serialize it, which always emits a DATE-TIME (`UNTIL=…T235959Z`). For an all-day (`VALUE=DATE`) master this produced `RRULE:…;UNTIL=20260719T235959Z` against `DTSTART;VALUE=DATE:…`, violating RFC 5545 §3.3.10 (UNTIL's value type must match DTSTART). Expansion worked in-app, but a strict server or another client could reject/mishandle the object on push.
- **Fix:** after `SetRecurrenceRule`, when the master's DTSTART is date-only, rewrite the RRULE's UNTIL token to a DATE via the new `dateOnlyUntil` helper. Timed series are unaffected (still DATE-TIME).
- Tests: `internal/model/recuruntil_test.go` — an all-day series caps to `UNTIL=20260719` (no `T`); a timed series keeps its DATE-TIME UNTIL. Full gate passes.
- Files: `internal/model/recur_edit.go`, `internal/model/recuruntil_test.go`.

## 2026-07-13 — Hardening pass 9: fuzz the recurrence write-side (guards the H2–H5 class)

- The decode-only fuzzers (pass 4) structurally couldn't reach the recurrence *mutation* primitives, which is exactly where pass-9 H2–H5 lived. Added `FuzzRecurrenceMutations` (extending `internal/model/fuzz_test.go` per the "extend, don't fork" rule): for any body that decodes, it drives `AddOccurrenceOverride`, `AddException`, `SplitEvent`, and `AdvanceRecurringTodo`, asserting each (a) never panics and (b) yields an object that re-encodes — so a degenerate rule can't crash the app (H2) and a mutation can't produce an unsaveable object.
- Seeds added (`recurEditSeeds`): the near-zero anchor (H2), an alarmed recurring event (H3/H4), an all-day recurring event (H6), reused alongside the existing `icalSeeds`. Seed corpus runs on the normal gate; `go test -fuzz` explored ~4.8M execs in 26s with **no crash** after the H2–H5 fixes.
- Files: `internal/model/fuzz_test.go`.

## 2026-07-13 — Hardening pass 9 (H5): carry future RECURRENCE-ID overrides across a this-and-future split

- Pre-1.0 audit finding (HIGH, data-loss, confirmed): a this-and-future split lost any per-occurrence customization after the split point. `CapSeries` removes overrides with `rid >= until` from the (past) master half, and `NewSeriesFrom` rebuilt the future half from the master alone — so a `RECURRENCE-ID` override on a *future* occurrence vanished from **both** halves and that occurrence silently reverted to the series default.
- **Fix:** `NewSeriesFrom` now carries forward every override strictly after the split point (`rid > occ`), deep-copied (`deepCopyComponent`, incl. VALARM/nested children) and re-keyed to the new series' UID, so the customization moves with the occurrences it describes. The occurrence at `occ` itself is intentionally not carried — it's redefined by the this-and-future edit. Refactored the H3/H4 child-copy into a general `deepCopyComponent` reused here.
- Tests: `internal/model/recuroverridesplit_test.go` — a weekly series with a customized future occurrence, split before it, keeps that override (its `SUMMARY:custom` and `RECURRENCE-ID`) in the future series under the new UID. Full gate passes.
- Files: `internal/model/recur_edit.go`, `internal/model/recuroverridesplit_test.go`.

## 2026-07-13 — Hardening pass 9 (H3+H4): preserve VALARM/child components in recurrence overrides & splits

- Pre-1.0 audit finding (HIGH, iron-rule/data-loss, confirmed): the recurrence primitives that **hand-build** a component from a master copied only `master.Props`, never `master.Children` — so a nested `VALARM` (and any other child component) was silently dropped. Two reachable losses: (H3) "edit this occurrence" of an alarmed recurring event (`cloneOverrideComponent`) produced an override with **no reminder**; (H4) "this & future" split (`NewSeriesFrom`) produced a future series with **no reminder on any occurrence**. Root cause: these bypass the encode→decode `clone` round-trip that makes the `editComponent`-based paths iron-rule-safe.
- **Fix:** added `cloneChildren` (recursive deep-copy of child components) and a shared `cloneProp` (deep-copies the param map), and call `cloneChildren(master)` when building the override and the new series. Both now carry the master's VALARMs (and any unknown nested component/params) forward.
- Tests: `internal/model/recurchildren_test.go` — an alarmed recurring event keeps its VALARM (and the alarm's `X-CUSTOM` prop) through both "edit this occurrence" (override has 1 alarm; total 2 across master+override) and "this & future" (future series has 1). Full gate passes.
- Files: `internal/model/recur_edit.go`, `internal/model/recurchildren_test.go`.

## 2026-07-13 — Hardening pass 9 (H2): guard write-side recurrence expansion against panics

- Pre-1.0 audit finding (HIGH, reproduced): the recurrence *write* path expanded rules by calling rrule-go directly (`nextInstantAfter` → `set.After`, `occurrencesBefore` → `set.Between`), bypassing the `safeBetween` recover/bound guard the *read* path uses. A degenerate rule — e.g. a near-zero DTSTART year — panics rrule-go's `calcDaySet` (`index out of range [1] with length 0`, confirmed with a throwaway repro), so a malformed recurring item *displayed* fine (read path guarded) then **crashed the live app** on `Space`-complete (`AdvanceRecurringTodo`) or a this-and-future split (`SplitEvent`/`NewSeriesFrom`). Violates "the TUI must never crash on bad .ics data".
- **Fix:** added `safeAfter` (in `recurrence.go`, mirroring `safeBetween`: same panic-recover + `maxOccurrenceSteps` bound, matching `set.After(after, inc)` within the bound). `nextInstantAfter` now uses `safeAfter` and degrades to "no next occurrence" on a panic; `occurrencesBefore` now uses `safeBetween` and degrades to 0. Both are the same graceful fallback the read path already takes. Confirmed these were the only two unguarded rrule expansions in `internal/model`.
- Tests: `internal/model/recurpanic_test.go` — `AdvanceRecurringTodo` and `SplitEvent` on a near-zero-anchor recurring item complete without panicking (the pre-fix repro). Full gate passes.
- Files: `internal/model/recurrence.go`, `internal/model/recur_edit.go`, `internal/model/recurpanic_test.go`.

## 2026-07-13 — Hardening pass 9 (H1): neutralize path-traversal calendar ids (data-loss fix)

- Pre-1.0 audit finding (HIGH, verified): `store.SafeName` allowed `.` and `..` through unchanged, so a calendar id of `..` — reachable by typing `..` as a calendar name (`internal/ui/calendar.go` → `SafeName`) **or** from a hostile/buggy server collection href ending in `/..` (`sync.collectionID` guarded `"."` but not `".."`) — became a traversal segment joined onto the cache root. Create-then-delete such a calendar ran `RemoveCalendarLocal` → `os.RemoveAll(filepath.Join(root, ".."))`, which resolves to the **entire account data directory** (all calendars + state file). Confirmed the reachability trace end-to-end.
- **Fix (chokepoint + defense-in-depth):** `SafeName` now maps a result of exactly `"."`/`".."` to `"unnamed"` (legitimate names never sanitize to a bare dot-segment; `.ics` resource names are unaffected since they carry an extension). Added `validCalendarID` (rejects empty, `.`, `..`, or any `/`\`\x00`) and gated the three store paths that join a calendar id onto the root — `ensureCalendar`, `CreateCalendarLocal`, and (above all) `RemoveCalendarLocal`. `sync.collectionID` now also folds `".."` into the `"calendar"` fallback.
- Tests: `internal/store/pathsafety_test.go` — `SafeName` never yields a traversal/empty element; `RemoveCalendarLocal("..")` refuses and a sentinel file beside the calendars root survives (the catastrophe guard); `CreateCalendarLocal` rejects unsafe ids. `internal/sync/collectionid_internal_test.go` — traversal collection paths fold to `"calendar"`, normal paths keep their safe segment. Full gate (test/vet/staticcheck) passes.
- Files: `internal/store/mutate.go`, `internal/store/calendar.go`, `internal/sync/import.go`, + the two new tests.

## 2026-07-13 — Pre-1.0: best-effort push-flush on quit

- Closed the "edit then immediately quit" gap: previously pressing `q` stopped instantly and only cancelled work (`a.cancel()` + `stopSyncTimer`), so a local edit made inside the 3s debounce window — or while briefly offline — sat unpushed in the cache until the next launch (data-safe, but other devices didn't see it until reopen).
- **New `flushOnQuit`** (`internal/ui/app.go`): after the TUI stops (terminal restored — so it prints a plain notice and can't deadlock the event loop), it best-effort pushes pending changes. It's a **no-op when offline** (`syncFn == nil`) **or nothing is pending** (new `store.HasPendingChanges`), so quit stays instant in the common case; it uses its **own** context (so shutdown's `a.cancel()` doesn't abort it) with a **hard timeout** (`defaultQuitFlushTimeout` = 10s) enforced via a select/watchdog, so even a `syncFn` that ignores context cancellation can't trap the user (the process is exiting; a stuck goroutine is harmless). Nothing is ever lost — unpushed edits persist and sync next launch. Wired into `Run`: background workers are stopped (`cancel`+`stopSyncTimer`) before the flush so they don't race it; skipped on a TUI error.
- **`store.HasPendingChanges`** (`internal/store/calendar.go`): store-wide check — true for a dirty/never-pushed resource, a tombstone, or a pending calendar create/delete/rename/recolor (the per-calendar `HasLocalChanges` missed the calendar-level pending flags). Read-only, additive.
- Tests: `internal/ui/quitflush_test.go` — offline no-op, nothing-pending no-call (quit stays instant), pending → one bounded sync call with a deadline, sync-error note, and the **timeout watchdog** (a 2s-sleeping syncFn returns within a 100ms injected timeout). `internal/store/pending_test.go` — `HasPendingChanges` across all pending kinds + clean cases. Full gate + `-race` on ui/store pass; release binary builds.
- Files: `internal/ui/app.go`, `internal/store/calendar.go`, `internal/ui/quitflush_test.go`, `internal/store/pending_test.go`, docs (`README.md`, `main.md`, `CLAUDE.md`).

## 2026-07-13 — Pre-1.0: reorder the bottom help bar (help/quit first, then movement)

- Cosmetic, non-breaking. The help bar is still a single hardcoded string with wrap off, so a narrow terminal clips the right end. Reordered it so the two most important hints — `? help` (reveals the full keymap) and `q quit` — lead and survive clipping, followed by the basic movement/navigation a new user needs (`hjkl move · Enter open · Esc back · c/t/a panes · f/b prev/next · v view · [ ] cal · { } list`), then the editing actions, then the rest. Also newly surfaces `hjkl`/`Enter`/`Esc` on the bar (they weren't listed before). No behavior change; the `?` overlay remains the full reference.
- Tests: `internal/ui/hints_test.go` — asserts `? help · q quit` leads and the intended token order holds, plus the `comp:on/off` toggle. Full gate passes.
- Files: `internal/ui/render.go`, `internal/ui/hints_test.go`.

## 2026-07-13 — Hardening pass 8: exhaustive timezone/DST recurrence sweep (no bug found; regression guards added)

- Recurrence + DST is a classic bug farm, so swept it exhaustively (`internal/model/tzsweep_test.go`), first observing the model's actual output on the hard cases, then pinning the observed-correct behavior. All assertions are on the **local wall-clock** time (the user-facing truth, and the property that must survive an offset change).
- Cases, all **passing** (behavior confirmed correct): daily/weekly wall-clock preserved across the US spring-forward and fall-back; southern-hemisphere DST (Australia/Sydney, opposite direction); half-hour-offset zone (Australia/Adelaide); leap-day `FREQ=YEARLY` recurs only on leap years (2024/2028/2032, not normalized); `FREQ=MONTHLY` on the 31st skips short months; year-boundary daily; UTC (no shift); floating time interpreted in loc; Windows/Outlook zone name (`Eastern Standard Time`) resolved via the CLDR map; all-day weekly stays date-only on the right dates across DST.
- The two hard cases are pinned by `TestTZSweepGapAndFold`: a **spring-forward gap** time (02:30 on the skip day) and a **fall-back ambiguous** time (01:30, which occurs twice) each yield exactly one occurrence on each expected day — no crash, drop, or duplicate. (The gap-day instant is an hour off, a benign zone-arithmetic quirk in the underlying library; the invariant that matters — one-per-day, no error — holds.)
- No product code changed; the sweep is a permanent regression guard on the normal gate. Full gate passes.
- Files: `internal/model/tzsweep_test.go` (new).

## 2026-07-13 — Hardening pass 7: network fault-injection — cap response bodies, verify clean degradation

- Hardened the CalDAV network trust boundary against a hostile/buggy/compromised server.
- **Fix — response body size cap:** the four own-XML PROPFIND parsers (colors, ctag, privileges, listobjects) and go-webdav's calendar-data reads all did an unbounded `xml.NewDecoder(resp.Body).Decode(...)` / decode, so a server streaming an unbounded (or enormous) body could hang a sync or exhaust memory — a real risk on the Pi. Added a `cappingTransport` on the shared HTTP client (so it covers both go-webdav's requests and our own): every response body is bounded at `maxResponseBodyBytes` (64 MiB, far above any real listing), and exceeding it fails the request with an explicit error rather than silently truncating. A bulk download that trips it falls back to per-resource fetches (pass-3 #2); a metadata PROPFIND that trips it just degrades (best-effort).
- **Tests — hostile responses** (`internal/caldav/hostile_test.go`): an oversized/streaming body makes the call fail (bounded read) within a watchdog instead of hanging; malformed XML, non-XML bytes, an empty 207, 500/401 statuses, a Content-Length-lying truncated body, and a 5000-deep nested document each return an error without panicking or hanging (the deep-nest case confirms no stack overflow in the XML decode).
- **Tests — sync fault propagation** (`internal/sync/fault_test.go`): a discovery failure surfaces as a clean error without mutating the cache; a transient push failure leaves the local edit intact and still dirty (never marked clean or dropped) and it pushes cleanly once the server recovers. (Per-calendar reconcile failures were already record-and-continue from passes 2–3.)
- Files: `internal/caldav/client.go` (+`hostile_test.go`), `internal/sync/sync_test.go` (fake gained `discoverErr`), `internal/sync/fault_test.go`. Full gate passes.

## 2026-07-13 — Hardening pass 6: terminal/display robustness stress pass (no bug found; regression guards added)

- Targeted the layer with the worst historical track record — the six custom-drawn widgets (`calendarView`, `timeGridView`, `agendaBoard`, `colorPicker`, the mode indicator, `caretForm`), which previously produced two freeze bugs (draw-deadlock and the tree infinite-loop). Method mirrors the fuzz passes: drive display-hostile content through every draw path across a matrix of terminal geometries and assert no `Draw` panics or hangs (a panic in a draw path crashes the live app).
- **New stress tests** (`internal/ui/displaystress_test.go`), each drawing on a `SimulationScreen` with a panic-recover + 5s watchdog:
  - `TestDisplayStress` — drives every mode/view (tasks, calendar month/week/day, drilled, agenda) with hostile content (3000-char titles; double-width CJK/emoji; zero-width combining marks; RTL; control chars; regional-indicator flag pairs; 150 same-day events; a 30-deep subtask chain; 300 flat tasks) and draws the whole layout at geometries from **1×1 to 400×150**.
  - `TestMonthGridDrillScrollStress` / `TestTimeGridDrillScrollStress` — drive each grid's `InputHandler` directly (the real drill path, which the app forwards to the focused primitive) to the far index over 150 hostile items, then draw at 1–3-row heights — the scroll-window / "+N more" math at its extreme, including hour-zoom pushed to the max.
  - `TestEditFormStress`, `TestColorPickerStress` — the popup draw paths over a 3000-char/emoji prefill.
- **Result: no panic or hang found** — the custom widgets handle rune-width, clipping, and scroll boundaries correctly even at 1×1 with double-width content at the far scroll index. The value is the permanent regression guards: any future draw-path panic/hang (the historical freeze-bug classes) now fails the normal gate. Confirmed `SimulationScreen` honors 1×1 so the boundary math is genuinely exercised.
- No product code changed; full gate (test/vet/staticcheck) passes.
- Files: `internal/ui/displaystress_test.go` (new).

## 2026-07-13 — Hardening pass 5: batched bulk pull — initial sync/import now O(N), not O(N²)

- A scale benchmark (`internal/sync/scale_bench_test.go`, `BenchmarkInitialSyncPull`) confirmed a **quadratic** first-time sync/import: n=100→9ms, n=400→89ms, n=1000→457ms. Cause: every pulled resource went through `writeResourceLocked`, which re-serialized and atomically rewrote the **whole** calendar's sidecar — so N pulls × O(N) sidecar each = O(N²) work and disk bytes (brutal on a Pi's SD card, where every write also fsyncs).
- **Fix:** new `store.PullRemoteBatch` writes each `.ics` atomically but the sidecar **once** per calendar. Sync's step (B) "new on server" loop and `Import` collect their pulls and apply them in one batch. After: n=100→3.4ms, n=400→12.4ms, n=1000→29.7ms — **linear** (~15× faster at n=1000). Refactored the write core into `stageResourceLocked` (write `.ics` + in-memory, defer sidecar) shared by the single-write path and the batch.
- **Crash safety (the delicate part):** the batch is pull-only and holds `s.mu` for its whole duration, so a concurrent UI edit is fully serialized (never interleaved/lost — the pass-3 #3 hazard) and all writes are unconditional (no clobber). A crash mid-batch can leave an `.ics` whose sidecar entry wasn't flushed — a "pull orphan" that reloads clean and href-less. Reconcile step (A) now recognizes that state (`Href=="" && !Dirty`, which a genuine local create never is — those are dirty) and **does not re-upload it** (which would create a server-side duplicate); step (B) re-pulls the server's copy over it, healing it. If the server no longer has it, it stays an inert local item rather than being resurrected on the server.
- Tests: `internal/sync/orphan_test.go` — a pull orphan is healed by re-pull with **0 PUTs** (no duplicate), and an orphan the server lacks is still never pushed. Full gate + `-race` on sync/store pass.
- Files: `internal/store/{mutate,remote}.go`, `internal/sync/{sync,import}.go`, `internal/sync/{orphan,scale_bench}_test.go`.

## 2026-07-13 — Hardening pass 5: BuildTree is now linear, not quadratic

- `BenchmarkBuildTree` showed the subtask-forest build was **O(N²)**: n=100→36µs, n=1000→3.5ms, n=5000→**93ms** (per reload — and it runs on every tree reload). Cause: the per-insert `descends()` cycle guard walked the parent's entire current subtree, summing to O(N²) when many tasks share few parents.
- **Fix:** replaced the subtree walk with `classifyByAncestry` — a memoized, iterative parent-chain classification that marks each UID as either reaching a real root or trapped in a cycle, in linear total time (iterative, so a deep chain can't overflow the stack either). Behavior is **unchanged**: nodes reachable only through a cycle are still dropped (per the `TestBuildTreeBreaksCycles`/`TestBuildTreeCycleWithExtraChild` contract), duplicates and UID-less todos handled as before. After: n=5000→**2.35ms** (~40× faster) and cleanly linear.
- Tests: existing tree tests + `FuzzBuildTree` (re-fuzzed 40s clean) cover the preserved semantics; `internal/model/scale_test.go` adds the benchmark. Full gate passes.
- Files: `internal/model/tree.go`, `internal/model/scale_test.go`.

## 2026-07-13 — Hardening pass 5: bound recurrence expansion (scale + malformed-input safeguard)

- Scale review found `Event.Occurrences` had **no cap** on how many instances it materialized, and it runs on the render path. A syntactically valid but pathological rule — `FREQ=SECONDLY` with no COUNT/UNTIL (≈2.6M instances over a month view), or a rule anchored centuries before the window (an unbounded skip-forward) — would freeze the UI or exhaust memory. Reachable from a malformed/adversarial `.ics`, so this is a robustness/DoS bug as much as a scale one; the pass-4 fuzz harness structurally couldn't catch it (a huge-but-successful expansion trips neither the no-error nor no-panic assertion).
- **Fix:** `safeBetween` now iterates the rrule set manually (via `Set.Iterator()`) with two bounds — `maxOccurrenceSteps` (~1M raw steps, incl. skipped) and `maxOccurrencesPerEvent` (10000 collected) — so a pathological rule returns promptly with a bounded result instead of hanging. Within the bounds the output is identical to `set.Between`. (The existing panic-recover for degenerate rrule iteration is kept.)
- Tests (`internal/model/scale_test.go`): a `FREQ=SECONDLY` event and a far-anchored `FREQ=MINUTELY` event both expand within a 10s watchdog and return a capped count; `FuzzEventOccurrences` re-fuzzed 45s clean. Full gate passes.
- Files: `internal/model/recurrence.go`, `internal/model/scale_test.go`.

## 2026-07-13 — Hardening pass 4: fuzz the iCalendar ingest boundary — contain library panics

- Started a **fuzz pass** over LazyPlanner's input trust boundary (the decision to address fuzzing now: the app ingests arbitrary iCalendar from any other CalDAV client and any server response, yet had **zero** fuzz tests — the single largest robustness surface, and pass-3 already proved it harbors real bugs). Added native Go fuzz targets in `internal/model/fuzz_test.go`: `FuzzDecode` (decode → Encode → re-decode round-trip), `FuzzEventOccurrences` (recurrence expansion), `FuzzBuildTree` (subtask forest from a fuzzed topology), `FuzzParseQuickAdd` (smart parser). `go test` runs the seed corpus (incl. every saved crasher) on the normal gate; `go test -fuzz` explores.
- **Two crash bugs found and contained** (both violated the iron rule "the TUI must never crash on a bad server response or malformed .ics"):
  - **go-ical decoder panic** — its line decoder indexes past the buffer (`peek()` with no `empty()` guard) on a content line ending mid-parameter (e.g. `PROP;X=`), panicking instead of erroring. A malformed `.ics` on disk **or a hostile/buggy server response** (go-webdav decodes calendar-data with the same decoder) would crash the whole app. Contained at both byte→calendar boundaries: `model.decodeCalendar` (recover → error; covers the store load + conflict re-parse paths) and `internal/caldav`'s new `guardICalPanic` around `QueryCalendar`/`GetCalendarObject` (a bulk-query panic surfaces as a `DownloadAll` error, which sync already falls back from to per-resource fetches, so one poison object is skipped, not fatal).
  - **rrule-go iteration panic** — `Set.Between`→`calcDaySet` panics (index out of range) expanding some degenerate rules (e.g. a near-zero DTSTART year). `Event.Occurrences` now iterates via `safeBetween` (recover) and degrades to the event's base instance — the same graceful fallback it already uses for a rule that fails to *build*.
- Vendored code is never hand-edited (per CLAUDE.md); both fixes live at our own call boundaries.
- Tests: `internal/model/harden_ingest_test.go` (`TestDecodeContainsDecoderPanic`, `TestOccurrencesDegradeOnRrulePanic`); `internal/caldav/guardpanic_test.go` (guard converts the real go-ical decode panic to an error; passes a normal error through). Saved crashers under `internal/model/testdata/fuzz/`. Full gate + all four fuzzers clean (FuzzDecode 18.5M execs / 3 min).
- Files: `internal/model/{decode,recurrence}.go`, `internal/caldav/client.go`, tests + fuzz corpus.

## 2026-07-13 — Hardening pass 4: heal decode-but-unencodable iCalendar on ingest

- `FuzzDecode`'s round-trip invariant (anything that decodes must re-encode, so anything LazyPlanner can display it can also save) surfaced a class of **"loaded but uneditable"** bugs: go-ical's decoder is tolerant but its **encoder** is strict, so an item that parsed fine could fail to re-encode — and since every edit re-encodes the whole resource (`editComponent`→`clone`→`Encode`, and `store.writeResource`), that hard-failed the edit **and blocked editing every sibling in the same resource**. Downloads already re-encode (so the server can't inject these — they're rejected as a skip), but a `.ics` written by another vdir tool (vdirsyncer/khal) or hand-edited reaches the cache and displays.
- **Healed at ingest** (`model.Parse`, mirroring how `resolveDateTime` recovers an unknown TZID — add only what's missing, never mangle existing props, so the iron rule holds):
  - **Missing DTSTAMP** (`ensureDTStamp`) — synthesized from LAST-MODIFIED/CREATED, else a fixed epoch; a real edit's `touch()` overwrites it, so the placeholder rarely persists.
  - **Missing VERSION/PRODID** on the VCALENDAR (`ensureCalendarProps`) — LazyPlanner's own, only when absent (an existing PRODID naming another producer is preserved).
  - **Duplicate single-valued properties** (`dedupeSingleValued`, e.g. two UIDs) — drop all but the first (the one `text()`/typed parsing already read), via a table mirroring go-ical's encoder cardinality rules for the component types we emit.
  - **Raw CR/LF in a property value** (`sanitizePropValues`) — stripped; a real line break is the two-char escape `\n`, so a raw control char is structural corruption, never content.
  - **Illegally nested components** (`stripForbiddenNesting`, e.g. a VTODO inside a VTODO) — dropped; only VALARM may nest under VEVENT/VTODO (STANDARD/DAYLIGHT under VTIMEZONE), and a mis-nested item is unaddressable anyway.
- A UID-less component is **not** given a fabricated UID (that would churn identity under sync — pass-3 #7 deliberately keeps such todos display-only), so it remains the one documented non-round-trippable case. The remaining go-ical *semantic* encoder constraints (DUE+DURATION / DTEND+DURATION mutual exclusion, empty VCALENDAR, VTIMEZONE-needs-a-child) are not auto-healed — extremely low reachability (the fuzzer ran clean past them) — left as a documented follow-up.
- Tests: `TestDecodeHealsForEditability` (a DTSTAMP/VERSION/PRODID-less todo decodes, re-encodes, edits, and keeps an unknown `X-` prop), `TestDecodeDedupesAndStripsToEncodable` (two UIDs → first kept; nested VTODO + the rest re-encode). All existing tests unaffected (heals are no-ops on well-formed fixtures). Full gate passes.
- Files: `internal/model/decode.go`, `internal/model/harden_ingest_test.go` (+ fuzz corpus).

## 2026-07-12 — Session wrap-up: entering continuous hardening/audit phase

- End-of-day checkpoint. All 13 build steps are complete; the project is now explicitly in a **continuous hardening & audit phase** — bug-hunting, resilience, and consistency, not new features. Next session picks up with **continued auditing**.
- This session's hardening: three audit passes (promised-vs-implemented gaps; consistency; deep debugging — 9 adversarially-verified defects fixed, including sync-core data-loss/TOCTOU races), plus a concurrent `-race` stress test and an opt-in live CalDAV suite verified against the NextCloud test account. All on `ai-workspace`, pushed; nothing merged to `main`.
- **Next / not yet audited:** large-calendar performance/scale, and the Raspberry Pi target on real hardware.
- Docs updated to record the phase: `main.md` (Status, Current State, new "Hardening & audit phase" note), `CLAUDE.md` (Project Context phase line + live/`-race` test conventions), this `log.md` entry.

## 2026-07-12 — Live CalDAV integration tests (opt-in, real server)

- Added `internal/sync/live_test.go` behind a `//go:build live` tag (excluded from the normal build/gate). It reads the configured account via `config.Load` (no secret on the command line) and operates only inside a throwaway calendar it creates and deletes via `t.Cleanup` — never touching a pre-existing calendar.
- Verified **live against the owner's NextCloud test account** (`test_user@cloud.litteken-server.com`), all passing, throwaway calendars cleaned up:
  - `TestLiveDiscover` — discovery walk + the three side-channel PROPFINDs: colors (truecolor hex), CTags (all present), and privileges (the `contact_birthdays` calendar correctly detected read-only); component-set parsing (VEVENT vs VTODO).
  - `TestLiveRoundTrip` — full two-way sync: local create → push → confirmed on server; edit → push → confirmed; the **CTag incremental short-circuit** engaging on an idle repeat sync; delete → push → confirmed gone.
  - `TestLiveCalendarProps` — a calendar rename + recolor `PROPPATCH` round-trip, confirmed by re-discovery (server-authoritative name/color).
  - `TestLiveConflict` — a resource edited both locally and directly on the server syncs to a recorded keep-both conflict (server version stashed, not flagged deleted, no silent overwrite).
- Documented the opt-in suite in the README Development section. The normal `make check` gate is unaffected (build-tag excluded; staticcheck/vet clean).
- Files: `internal/sync/live_test.go` (new), `README.md`.

## 2026-07-12 — Hardening pass 3: concurrent sync-vs-edits stress test (-race)

- Added `TestConcurrentSyncAndEditsRace` (`internal/sync/sync_test.go`): the real scenario the compare-and-set writeback (#3) guards — a background goroutine looping `sync.Sync` while 4 goroutines hammer `store.Put` on the same resources (1000 edits/run). Previous #3 test only *simulated* the interleaving synchronously; this drives genuine goroutine concurrency so `-race` has something to inspect.
- Asserts: no data race (detector), no panic/deadlock (completes), and post-quiesce integrity — every seeded resource still present, parseable, carrying its own UID (no torn/mixed body), and a fresh `store.Open` of the same dir reloads the identical consistent set with zero load errors (proves concurrent sync + edits never leave the `.ics`/sidecar inconsistent or drop a resource).
- Each editor Puts pre-built per-goroutine `*model.Parsed` copies so no object is shared across goroutines — isolating the store's own locking as the thing under test. Passes under `go test -race -count=5`.
- Files: `internal/sync/sync_test.go` (test-only; added a `stdsync "sync"` alias). Full gate + race pass.

## 2026-07-12 — Hardening pass 3 (#2): one bad resource no longer stalls a whole calendar's download

- **Bug:** `DownloadAll` runs go-webdav's bulk calendar-query, whose `decodeCalendarObjectList` returns on the **first** resource whose iCalendar won't decode. So a single corrupt/truncated `.ics` made the whole calendar's download fail; `reconcileCalendar` recorded the entire calendar as one skip and none of its other (healthy) resources synced — every sync, until the bad item was fixed server-side. This contradicted the documented per-resource resilience (the decode happens in the transport before the app sees individual objects, so the per-item skip in `pullInto`/`model.Parse` never got a chance).
- **Fix:** new caldav `ListObjectHrefs` (a Depth-1 PROPFIND for `getetag`/`resourcetype`, no calendar-data → can't fail on a bad body) + a shared `downloadResilient` helper: on a bulk-download failure it enumerates hrefs and `GetObject`s each resource individually, skipping (and recording) only the ones that won't fetch/decode. Wired into both two-way sync (`downloadCalendar`) and one-way `Import`. The fallback records a skip so the slower degraded path isn't invisible (no silent truncation).
- Tests: `internal/sync/sync_test.go` — a failed bulk download falls back, syncs the good resource, and skips the bad one (via new `onPut`-style `getErr`/`failDownload`/`ListObjectHrefs` fakes); `internal/caldav/listobjects_test.go` — the PROPFIND parse excludes the collection and returns members with unquoted ETags. `Import`'s and `Sync`'s doc comments now match reality.
- Files: `internal/caldav/listobjects.go` (new, +test), `internal/sync/{sync,import}.go`, `internal/sync/{sync,import}_test.go`. Full gate passes.

## 2026-07-12 — Hardening pass 3 (#9): a concurrent calendar rename/recolor survives its PROPPATCH

- **Bug (metadata loss):** `pushCalendarProps` snapshotted the pending name/color, ran the network `SetCalendarProps` PROPPATCH, then `MarkCalendarPropsSynced` cleared `pendingName`/`pendingColor` **unconditionally**. If the user renamed/recolored the same calendar during the round-trip, the flag was cleared even though the value had changed — so the new value never re-pushed, and the next discovery's `SyncCalendarName`/`SyncCalendarColor` (which skip only while pending) then adopted the server's *older* value, overwriting the local edit. Silent metadata loss.
- **Fix:** `MarkCalendarPropsSynced` now takes the pushed name/color and clears a flag only if the field still equals what was PROPPATCHed; a concurrent change leaves the flag set so it re-pushes and the server value can't clobber it.
- Test (`internal/store/pendingflags_test.go`): rename B pushed, rename C lands mid-PROPPATCH, mark-synced(B) leaves C pending, and a discovery pull of B doesn't overwrite C.
- Files: `internal/store/calendar.go`, `internal/sync/sync.go`, `internal/store/pendingflags_test.go`. Full gate passes.

## 2026-07-12 — Hardening pass 3 (#4): keep-server can't misread an unparseable version as a deletion

- **Bug (silent local-edit loss):** `stashServerConflict` swallowed `model.Parse`/`Encode` errors, so a server version that ical-decodes but fails our stricter model (e.g. a VEVENT missing DTSTART written by another client) stashed with **empty** `ServerData`. `ResolveKeepServer` used `ServerData == ""` as the *sole* signal for "server deleted" → keep-server `Forget`s the local copy. So a present-but-unparseable server version was indistinguishable from a real deletion, and choosing "keep server" silently discarded the local edit with the server version never captured — a keep-both iron-rule violation.
- **Fix:** added an explicit `ServerDeleted` flag to the conflict (sidecar + `Conflict` + `MarkConflict`), set only on a genuine server deletion; `ResolveKeepServer` now branches on it, never on empty `ServerData`. `stashServerConflict` encodes the decoded server calendar **directly** (not via a typed re-parse) so an unparseable version is still preserved losslessly, and records a skip. Keep-server on an unparseable version now errors (surfaced) and leaves the local edit intact instead of deleting it; a truly empty non-deletion also refuses rather than dropping data.
- Tests (`internal/sync/sync_test.go`): a both-edited conflict whose server version lacks DTSTART is not flagged deleted, stashes the raw version, and keep-server errors without discarding the local edit. Updated the `MarkConflict` signature in store/ui conflict tests.
- Files: `internal/store/{conflict,sidecar}.go`, `internal/sync/sync.go`, tests in `internal/store`, `internal/ui`, `internal/sync`. Full gate passes.

## 2026-07-12 — Hardening pass 3 (#3): sync writeback can't clobber a concurrent edit

- **Bug (silent lost update):** `pushUpdate`/`pushCreate` encode the pre-sync snapshot, run a slow network PUT, then wrote that *same stale snapshot* back as clean (`PutRemote`). Sync runs on a background goroutine while the UI keeps editing on the event loop, so an edit that lands during the in-flight PUT was reverted on disk + in memory **and** marked clean (never pushed) — the edit was irrecoverably lost, no conflict raised. The 3s debounced push (fires while the user is still typing) makes the window reachable. `pullInto` had the same clobber pattern against a concurrent edit during reconcile.
- **Fix (compare-and-set):** every resource mutation swaps in a fresh `*Resource` (copy-on-write), so pointer identity is the concurrency signal. New store `CommitPush` adopts the server ETag+clean state only if the current resource is still the exact one that was pushed; if a concurrent edit replaced it, that newer content is kept **dirty** with the ETag baseline advanced to the server's value (next push is conditional on it — no revert, no lost update, no duplicate). New `PullRemote` takes an `expectedPrev` and skips the pull if a concurrent edit replaced it (leaving it to reconcile as a conflict next sync); read-only/server-authoritative pulls pass `nil` (unconditional). Refactored `writeResource` to expose a lock-held core (`writeResourceLocked`) shared by all three.
- Tests (`internal/sync/sync_test.go`): a concurrent edit injected mid-PUT (new `onPut` fake hook) survives, stays dirty, and adopts the new ETag baseline — instead of being reverted to the pushed snapshot. Also verified under `go test -race`.
- Files: `internal/store/mutate.go`, `internal/store/remote.go`, `internal/sync/sync.go`, `internal/sync/sync_test.go`. Full gate passes.

## 2026-07-12 — Hardening pass 3 (#8): skip server objects with an empty href

- **Bug:** a CalDAV response carrying an empty `<href/>` decoded to an object with `Path==""`. Reconcile step B didn't match it in `localByHref` and pulled it, storing it with `Href==""`; the **next** sync's step A then saw `r.Href == ""`, classified it as a never-pushed local resource, and `pushCreate`'d it — a spurious server-side duplicate from a malformed/buggy server response.
- **Fix:** step B now skips any server object with an empty `Path`, recording it (`errEmptyHref`) instead of storing an unaddressable resource.
- Test (`internal/sync/sync_test.go`): an empty-href server object is skipped (recorded, 0 pulled, 0 stored, 0 puts) rather than stored and re-uploaded.
- Files: `internal/sync/sync.go`, `internal/sync/sync_test.go`. Full gate passes.

## 2026-07-12 — Hardening pass 3 (#7): UID-less todos no longer collapse in the tree

- **Bug:** `BuildTree` keyed nodes by `Todo.UID`. A VTODO with no UID (malformed — UID is RFC 5545-required but nothing enforces it on read) hashed to the shared `""` slot: every UID-less todo overwrote `nodes[""]`, so only the **last** survived the map, and the roots loop then resolved each UID-less todo to that same node and appended it repeatedly — some tasks vanished, one duplicated. (A duplicate *non-empty* UID had a milder version of the same double-append.)
- **Fix:** UID-less todos are no longer keyed in the map; each gets its own standalone root node so all surface exactly once. A `placed` set ensures a duplicate non-empty UID places its node once.
- Tests (`internal/model/tree_test.go`): two UID-less todos + a keyed one produce three distinct roots (each once); a duplicate UID places one node.
- Files: `internal/model/tree.go`, `internal/model/tree_test.go`. Full gate passes.

## 2026-07-12 — Hardening pass 3 (#6): beginGrabFuture rolls back a half-completed split

- **Bug:** a this-and-future grab writes the split as two `store.Put`s — cap the master, then write the new future series. If the **first** succeeded and the **second** failed, `beginGrabFuture` flashed an error and returned with the master already capped (tail occurrences dropped), the future series never written, `grabbing` still false (so `cancelGrab`'s two-resource revert could never run), and no undo step pushed — the later occurrences were lost with no in-session recovery.
- **Fix:** on the second `Put` failing, `Restore` the master from `loc.Prev` before returning, so the split can't half-complete. Both error paths now use `flashErr("Grab", err)`.
- Test (`internal/ui/recur_edit_test.go`): after capping the master, `Restore(loc.Prev)` (the exact rollback the fix performs) brings the master back to its full 4 occurrences. (A real mid-operation write failure can't be induced deterministically — the new series' resource name uses a random UID — so the test exercises the recovery call directly; the live two-resource revert stays covered by `TestGrabFutureCancelRestores`.)
- Files: `internal/ui/grab.go`, `internal/ui/recur_edit_test.go`. Full gate passes.

## 2026-07-12 — Hardening pass 3 (#5): mouse can't bypass grab/resize modal gating

- **Bug:** `mouseCapture` guarded only on `modalOpen()` (an overlay page). Grab mode (`a.grabbing`) and the `Ctrl-W` resize sub-mode (`a.resizing`) are flag-only modal states with no overlay page, so the mouse was **not** swallowed during them: a click still fired `setMode` (switching the active pane) and a double-click still opened the edit form — two modal states coexisting, and grab reading the wrong `a.mode`. The keyboard path already gated on both flags.
- **Fix:** `mouseCapture` now swallows the event (`return nil, action`) when `a.grabbing || a.resizing`, matching the keyboard gating.
- Test (`internal/ui/mouse_test.go`): a click during each flag-state is swallowed and does not switch mode.
- Files: `internal/ui/mouse.go`, `internal/ui/mouse_test.go`. Full gate passes.

## 2026-07-12 — Hardening pass 3 (#10): Space on an event always gives feedback

- Key-contract fix (owner's explicit Pass-3 rule: a used key must act or flash, never a silent no-op).
- **Bug:** `toggleComplete` early-returned silently when the target was not a task. In Calendar mode Space pre-handles the event case ("Can't complete an event") in its own switch, but in **Agenda** (and Tasks) mode Space routes straight to `toggleComplete`, so pressing it on an event did nothing with no feedback — inconsistent with Calendar mode.
- **Fix:** `toggleComplete` now flashes `Can't complete an event` for a non-task target and `Select a task first` when nothing is selected. Calendar mode still pre-empts both cases, so no double message.
- Test (`internal/ui/lowfixes_test.go`): Space on an Agenda event flashes the event message.
- Files: `internal/ui/edit.go`, `internal/ui/lowfixes_test.go`. Full gate passes.

## 2026-07-12 — Hardening pass 3 (#1): malformed recurrence can't blank the calendar

- Deep debugging/hardening audit (multi-agent fan-out, adversarially verified) fix #1 of the confirmed set — the one **high**-severity finding.
- **Bug:** a cached VEVENT with a syntactically valid but semantically bad recurrence (`RRULE:FREQ=NONSENSE`, unknown key, wrong VALUE type) loads cleanly (RRULE isn't parsed at load) but errors at expansion time. `Event.Occurrences` → `Parsed.EventOccurrences` → `store.EventOccurrencesVisible` all returned on the first error, and the UI discards it (`occs, _ :=`), so a single bad event **blanked every event in every calendar** across month/week/day/agenda until the offending `.ics` was removed — a clear iron-rule (graceful-degradation) violation.
- **Fix:** degrade at the source — a recurrence that can't be built now falls back to the event's single **base instance** at DTSTART (`Event.baseInstance`), so the event stays visible, just un-expanded, instead of propagating a fatal error. Added defense-in-depth skip-and-continue at both aggregation loops (`Parsed.EventOccurrences` master loop, `store.EventOccurrencesVisible`) so no future expansion error can blank siblings/other calendars.
- Tests (`internal/model/recurrence_test.go`): a malformed-RRULE event yields its base instance (no error); a file with one bad + one good event surfaces both.
- Files: `internal/model/recurrence.go`, `internal/store/store.go`, `internal/model/recurrence_test.go`. Full gate passes.

## 2026-07-12 — Hardening pass 2: consistency across the program

- A deep consistency audit (fan-out over error-handling/messaging, UI cross-view, model/store API, sync/caldav patterns, coding standards) confirmed high consistency; fixed the real divergences (owner decided the forks).
- **Sync 403 handling (the headline fix)**: `pushDelete` trusted a bare 403 (flag read-only, resurrect the item, drop the tombstone → delete never retried) while create/update re-check privileges. Added `handleDeleteForbidden` (the delete twin of `handleWriteForbidden`): a transient/unconfirmed 403 keeps the tombstone and records a skip to retry; only a *confirmed* read-only calendar discards. `pushDelete` now takes the calendar path for the re-check.
- **Sync record-and-continue**: per-calendar metadata writes (SetCalendarMeta/ReadOnly/Components/Sync{Color,Name}) in the discovery loop now `recordSkip`+`continue` instead of `return`-aborting the whole run — matching reconcile.
- **Cancellable password command**: `config.ResolvePassword` now takes a `context.Context` and uses `exec.CommandContext` with a 10s timeout, so a hung `password_command` (Vaultwarden/`bw`, network) can't block startup/reload uninterruptibly; threaded ctx from `buildSyncFn`/main.
- **Conditional-write symmetry**: `DeleteObject` now sends `If-Match: *` when no ETag is stored (matching `PutObject`); `store.SetSyncToken` gained the family's unchanged-guard + `%w` error wrap.
- **Message normalization** (owner: full): centralized the `(u to undo)` hint (in `commitMutation` + the create/quick-set/re-parent/toggle paths), added a result flash to toggle-complete (was the one silent mutation), and a `flashErr("<Action>", err)` helper so every mutation failure reads `<Action> failed: <err>` (field-validation errors stay descriptive); unified the quick vs full create flashes; capitalized the two lowercase result/error stragglers. (Skipped the internal `store:` error-prefix — it would double-prefix inside the user-facing flash.)
- **Resize Esc reverts** (owner): the `Ctrl-W` sub-mode now snapshots widths on entry so `Esc` cancels (restores) and `Enter` keeps — matching grab's semantics. Badge/help/docs updated.
- **Small consistency**: fixed a stranded doc comment (`SetCalendarReadOnly` godoc had merged into `CalendarCTag`); unified the app-level display helpers (`dueTasksByDay`, `fmtWhen`, `fmtDate`) onto `a.loc` instead of a `time.Local` literal (the remaining `time.Local` uses live in the view structs / free helpers that don't carry `a.loc`; identical today since `a.loc == time.Local`, left as an accepted follow-up); factored the UTC/all-day date-write into `newDateOrTimeProp` (shared by `setDateOrTime` and the EXDATE writer); debounced push now also armed on calendar create/rename/recolor/delete; documented the recurring-todo scope asymmetry (grab/quick-set edit the series; use `e` for per-occurrence) as an accepted, intentional difference; named `defaultSyncIntervalMinutes`; noted the subtask `guardComponent` invariant.
- Accepted as-is (defensible): over-exported-for-tests identifiers, local-FS helpers without ctx, `Item`/`Task not found` split, yank-anywhere/paste-in-Tasks.
- Tests: delete-403 transient-keeps-tombstone vs confirmed-discards; resize Esc-reverts / Enter-keeps. Full gate passes.
- Files: `internal/sync/sync.go`(+test), `internal/caldav/object.go`, `internal/config/config.go`, `cmd/lazyplanner/main.go`, `internal/store/{calendar,remote}.go`, `internal/model/{edit,recur_edit}.go`, `internal/ui/{app,edit,keys,grab,quickfield,yankpaste,calendar,command,recur_edit,help}.go`(+tests), `README.md`, `main.md`.

## 2026-07-12 — Hardening pass 1: close promised-vs-implemented gaps

- A deep spec-vs-code audit (fan-out across model/sync/views/tasks/keymap/config) found the implementation very faithful — no missing keybindings or `:` commands. Closed the few real gaps and reconciled the docs; owner decided each fork.
- **Built #1 — debounced push after edits** (the one missing sync trigger): `scheduleSyncDebounced` (`internal/ui/sync.go`) arms a 3s one-shot background sync after any local mutation, hooked into `pushUndo` (the universal forward-mutation signal) and `undoLast`; no-op offline, coalesced with running/periodic sync, cancelled on quit. Shrinks the conflict window as promised.
- **Built #2 — `0` = auto-fit hour-zoom reset**: a bare `0` in the week/day grid resets the hour-row height to auto-fit (`resetHourZoom`); still extends a pending vim count otherwise.
- **Built #4 — Detail-pane resize via a `Ctrl-W` sub-mode**: a modal resize mode (badge `RESIZE`) where `←`/`→`(`h`/`l`) size the overview and `H`/`L` size the Detail pane, `Esc`/`Enter` exit — terminal-robust (no exotic modifier chords, works on a bare Pi console). Detail is now a fixed, persisted width (`state.DetailWidth`, `SaveState` gained a `detailWidth` param); `Ctrl-←`/`Ctrl-→` still quick-resize the overview.
- **Doc reconciliations** (owner decisions): #3 dropped the promised per-calendar *local color override* from the spec (colors are server-owned via `:calendar color`; hide-locally stays as a state-file toggle); #5 removed the "last-used view" state example (opening view is the config `default_view`); #6 aligned the folder-delete wording (one confirm naming the subtree count; deleting a task with any descendants removes the subtree). Fixed staleness: `help.go` DRILL badge ("calendar day", + `RESIZE`), main.md runtime-paths table now shows the `<account-id>` segment, and documented the new `Ctrl-W`/`0`/debounced-push behavior across README/main.md/CLAUDE.md.
- Two low/intentional items left as-is (documented): `RANGE=THISANDFUTURE` overrides uninterpreted, and event recurrence surfaced as a boolean "repeats" flag rather than rule text.
- Tests (`internal/ui/hardening_pass1_test.go`): `0` resets zoom (and still extends a count), debounced push arms only when configured, and the `Ctrl-W` resize sub-mode sizes overview + Detail and exits. Updated the `saveState` test closures to the new signature. Full gate passes.
- Files: `internal/ui/sync.go`, `internal/ui/edit.go`, `internal/ui/keys.go`, `internal/ui/app.go`, `internal/ui/render.go`, `internal/ui/help.go`, `internal/state/state.go`, `cmd/lazyplanner/main.go`, `internal/ui/hardening_pass1_test.go` (+ test-closure fixups), `README.md`, `main.md`, `CLAUDE.md`.

## 2026-07-12 — Revert #4: recurring tasks show a single live instance again

- Owner decision after a caveats review: showing every occurrence of a recurring task on the calendar (the earlier #4 change) introduced unneeded complexity, and recurring tasks-with-subtasks ("recurring folders") are a confusing fit for the tasks-as-folders model — so **recurring checklists will not be built**, and #4 is reverted to plain complete-to-advance (a recurring task shows once, at its current due, and advances one occurrence on completion). The current independent handling of a recurring task's subtasks is data-safe (verified: recurrence edits only the parent's own component, stable UID, iron-rule preservation, links never dangle); it just doesn't regenerate subtasks.
- **Reverted (the #4 parts of commit `c038393`, keeping its unrelated #5/#8):** removed `model.Todo.DuesInRange` (`internal/model/recur_todo.go` deleted); `model.DayAgenda` and `dueTasksByDay` back to the single-due path; `AdvanceRecurringTodo` back to advance-one (dropped the `completedOcc` occurrence-aware param, restored `nextInstantAfter`, COUNT decrements by one); UI callers (`advanceRecurringTodo`, `editTodoThisOccurrence`/`editTodoDetachForm`, `deleteOccurrence`, `toggleComplete`) no longer thread `occStart`.
- **Kept:** #5 (edit-this-occurrence event form re-seeds from the existing override) and #8 (grab This / This & future / All), which were bundled in the same commit but are independent; and #6 (COUNT-preserving split), a separate commit.
- Tests: removed `TestRecurringTodoShowsAllOccurrences` and `TestCompleteLaterOccurrenceAdvancesPast`; reverted the `AdvanceRecurringTodo`/`editTodoThisOccurrence` call signatures in the remaining tests. `TestRecurringTodoSpaceAdvances` (advance-one + flash) still passes.
- Docs: `README.md`, `CLAUDE.md` reverted to "single live instance, advances on complete"; `CLAUDE.md` records the deliberate decision not to reintroduce todo occurrence-expansion.
- Files: `internal/model/recur_edit.go`, `internal/model/agenda.go`, `internal/model/recur_edit_test.go`, `internal/model/recur_todo.go` (deleted), `internal/ui/recur_edit.go`, `internal/ui/edit.go`, `internal/ui/render.go`, `internal/ui/recur_edit_test.go`, `README.md`, `CLAUDE.md`. Full gate passes.

## 2026-07-12 — Recurrence UX round 2: all task occurrences shown, override re-seed, grab this-and-future

- Second batch of owner-requested recurrence-UX changes (caveats review, #4/#5/#8).
- **(#4) Recurring tasks show on every occurrence's due day** (was: only the current one). New `model.Todo.DuesInRange` (`internal/model/recur_todo.go`) expands a recurring todo's occurrences anchored on DUE; `model.DayAgenda` and `dueTasksByDay` now emit one entry per occurrence in the window. Completion stays advance-on-complete but is now **occurrence-aware**: `AdvanceRecurringTodo` takes the completed occurrence and skips the series *past* it (completing the 3rd of 4 jumps to the 4th; earlier ones count as passed), decrementing COUNT by the number consumed. Threaded `occStart` through `toggleComplete`/`advanceRecurringTodo`/`deleteOccurrence`/`editTodoThisOccurrence`.
- **(#5) Re-editing an occurrence pre-fills from its override** (`editEventScoped` scopeThis): seeds the form from the existing `RECURRENCE-ID` override (via `FindOverride`) when one exists — including its moved start — instead of always the master, so a prior per-occurrence edit isn't shown as reverted (and isn't silently overwritten on save).
- **(#8) Grab supports this-and-future for recurring events** (`beginGrabFuture`): the picker now offers all three scopes for grab. A future grab splits the series on start (cap master + new series via `model.SplitEvent`) and grabs the new series; cancel deletes the new series and restores the master, commit bundles both as one undo step. New grab state `grabSplitMaster`/`grabSplitMasterPrev`; removed the now-unused `nextInstantAfter`.
- Docs: `README.md`, `CLAUDE.md`, `main.md`, help overlay + grab.go comment (grab this/future/all; recurring tasks show all occurrences).
- Tests (`internal/ui/recur_edit_test.go`, `internal/model/recur_edit_test.go`): a recurring task lands on 4 weekly days; completing the 3rd occurrence advances to the 4th; re-edit seeds from the override; grab-future splits+moves and cancel restores. Full gate passes.
- Files: `internal/model/recur_todo.go` (new), `internal/model/recur_edit.go`, `internal/model/agenda.go`, `internal/ui/recur_edit.go`, `internal/ui/edit.go`, `internal/ui/grab.go`, `internal/ui/app.go`, `internal/ui/render.go`, `internal/ui/help.go`, tests, docs.

## 2026-07-12 — Recurrence UX refinements: obvious advance flash, detach confirm, COUNT-preserving split

- Owner-requested refinements to step 11's recurring-item UX (from a caveats review).
- **(#1) Obvious advance flash** (`internal/ui/recur_edit.go` `advanceRecurringTodo`): completing a recurring todo advances it rather than checking it off, which is easy to miss. The flash is now accent-colored with a glyph and the new due date — `↻ Recurring task advanced (not completed) → next due <date>` (or `✓ Recurring task done — final occurrence completed`).
- **(#3) Detach confirmation** (`recur_edit.go` `editTodoThisOccurrence` → new `editTodoDetachForm`; `edit.go` new `confirmOK` generic-affirmative-label confirm): editing "this occurrence" of a recurring todo splits it into a separate one-off task + advances the series, which isn't obvious — now it confirms first ("… becomes a separate one-off task and the recurring series advances …", Detach/Cancel).
- **(#6) COUNT-preserving split** (`internal/model/recur_edit.go` `NewSeriesFrom` now takes `occ` + new `occurrencesBefore` helper): a this-and-future split of a COUNT-bounded series previously left the future half open-ended. The future half's COUNT is now reduced by the occurrences that stay with the capped master, so the two halves sum to the original count (UNTIL and infinite series were already exact).
- Tests: model `TestSplitSeries` now asserts the future half keeps 2 of the original 4 (was open-ended); UI `TestRecurringTodoSpaceAdvances` asserts the flash says "advanced"; new `TestEditTodoThisOccurrenceConfirms` asserts the detach confirm appears. Full gate passes.
- Files: `internal/model/recur_edit.go`, `internal/model/recur_edit_test.go`, `internal/ui/recur_edit.go`, `internal/ui/edit.go`, `internal/ui/recur_edit_test.go`.

## 2026-07-12 — Step 13: Raspberry Pi target (cross-build, Makefile, kiosk notes)

- Build step 13. LazyPlanner is pure Go (no cgo) with the tz database embedded, so it cross-compiles to ARM from any machine with no extra toolchain — verified building statically-linked binaries for **arm64** (64-bit Pi OS), **armv7** (32-bit Pi OS), and **armv6** (Pi 1 / Zero). Stripped (`-ldflags "-s -w" -trimpath`) they're ~8.6 MB (vs 13 MB native debug).
- **Makefile** (new): `build` (native), `check` (test + vet + staticcheck — the gate), `run`, `fmt`, and `cross`/`pi-arm64`/`pi-armv7`/`pi-armv6` (stripped Pi binaries into `dist/`, gitignored), `clean`.
- **CI** (`.github/workflows/ci.yml`): added a `make cross` step so an ARM-specific build regression is caught on every push (compile-only, no emulation).
- **Docs** (`README.md`): a "Raspberry Pi / dedicated terminal" section — cross-compile (`make cross`), copy/install to the Pi, and a **kiosk** setup (console autologin on tty1 via `raspi-config`, a `~/.bash_profile` `exec lazyplanner` guarded to tty1, the equivalent getty autologin override) plus the `color_mode = "16"` tip for a bare framebuffer TTY and a note that on-hardware performance isn't benchmarked yet (the one part of step 13 that needs a physical Pi). `CLAUDE.md` build-workflow note about the Makefile.
- No app code changed; `make check` passes, all three cross-builds succeed.
- Files: `Makefile` (new), `.github/workflows/ci.yml`, `.gitignore` (`/dist/`), `README.md`, `CLAUDE.md`.

## 2026-07-12 — Step 12: periodic background sync + incremental CTag short-circuit

- Build step 12 (the CTag half of incremental sync + periodic sync; the full `sync-collection` REPORT is a deliberate follow-up per the owner's scope choice).
- **Periodic background sync**: `Options.SyncIntervalMinutes` (from `config.Behavior.SyncIntervalMinutes`, default 15, 0 = off) now drives `startPeriodicSync` (`internal/ui/sync.go`) — a ticker goroutine that queues `triggerSync` onto the event-loop goroutine each interval (triggerSync touches `a.syncing`, so it must not run off it) and stops on `a.ctx.Done()` (quit). The config field was previously read but unwired.
- **Incremental CTag short-circuit**: `caldav` now fetches each collection's CalendarServer `getctag` during discovery (`internal/caldav/ctag.go`, a Depth-1 PROPFIND mirroring the color/privilege queries; best-effort — absent CTag falls back to a full sync) into `caldav.Calendar.CTag`. The store persists the last-synced CTag in the sidecar (`ctag` field) with `CalendarCTag`/`SetCalendarCTag`, plus `HasLocalChanges`. `sync.Sync` skips a calendar's full `DownloadAll` when the server CTag matches the stored one **and** there's nothing local to push, counting it in the new `SyncResult.CalendarsUnchanged`; the CTag is cached only after a fully clean reconcile (a per-resource failure re-syncs next time).
- Docs: `README.md` (syncing section + `r`/status + status line), `CLAUDE.md`, config comment de-staled.
- Tests: `internal/sync/sync_test.go` — an unchanged CTag skips the second sync's download, a changed CTag forces a re-download, and a pending local change still pushes despite an unchanged CTag (fake gained a `downloads` counter). Full gate passes. (Network paths are exercised against the fake Syncer, as the existing sync tests are; the live NextCloud path is unverified in this environment.)
- Files: `internal/caldav/ctag.go` (new), `internal/caldav/client.go`, `internal/store/store.go`, `internal/store/sidecar.go`, `internal/store/calendar.go`, `internal/sync/sync.go`, `internal/ui/sync.go`, `internal/ui/app.go`, `cmd/lazyplanner/main.go`, `internal/config/config.go`, `internal/sync/sync_test.go`, docs.

## 2026-07-12 — Step 11 (UI): recurrence editing — this / this-and-future / all

- Build step 11, part 2 of 2 (UI). Wired the recurrence-editing scopes into edit (`e`), delete (`d`), grab (`m`), and complete (`Space`), for events and todos.
- **Scope picker** (`internal/ui/recur_edit.go` `pickRecurrenceScope`): a modal offering *This / This & future / All* (events) or *This / All* (todos — a todo shows one live instance, so future collapses into all). Opened by `editRecurring`/`deleteRecurring` when `currentTarget` reports a recurring item.
- **editTarget** gained `occStart`/`allDay`/`recurring`, populated by `targetFromItem` (the occurrence's instant = the RECURRENCE-ID target) and the tree branch of `currentTarget`.
- **Events**: this → `EditEventOccurrence` (override) / `AddException` (delete); future → `SplitEvent` (cap master + new series, one two-op undo step via `commitSplit`) / `CapSeries` (delete); all → the existing master edit / whole-object delete. The event form is reused via the extracted `presentEventForm`, seeded at the occurrence's start.
- **Todos**: `Space` on a recurring todo advances it (`advanceRecurringTodo` → `AdvanceRecurringTodo`) instead of completing; edit-this-occurrence detaches the instance as a standalone task (`presentTodoForm` + `NewTodoObject`) and advances the master; delete-this skips the occurrence (advancing), deleting the resource outright when it was the last.
- **Grab** on a recurring event prompts *This / All* (not future — a split spawns a second resource that grab's single-snapshot revert can't undo; the edit form covers this-and-future). `beginGrab` records the scope; `grabNudge` reads/moves the RECURRENCE-ID override for a this-scope grab (synthesizing the occurrence's slot before the first override exists) and `focusGrabbed` anchors on the moved override. New model helper `Parsed.FindOverride`.
- `recurScope`'s zero value is `scopeAll` deliberately, so any unset path (non-recurring items, tests that set grab state directly) behaves as the pre-step-11 whole-series edit.
- Docs: help overlay (recurrence rows), `README.md`, `main.md`, `CLAUDE.md`. gofmt'd the grab-field block in `app.go` (my field additions shifted its alignment).
- Tests (`internal/ui/recur_edit_test.go`): Space advances a recurring todo; delete-occurrence EXDATEs an event instance; a this-occurrence grab creates an override moving only that instance; `e` on a recurring item opens the scope picker. Full gate passes.
- Files: `internal/ui/recur_edit.go` (new), `internal/ui/edit.go`, `internal/ui/grab.go`, `internal/ui/app.go`, `internal/ui/help.go`, `internal/model/recur_edit.go` (+`EditEventOccurrence`/`SplitEvent`/`FindOverride`), `internal/ui/recur_edit_test.go`, docs.

## 2026-07-12 — Step 11 (model): recurrence-editing primitives

- Build step 11, part 1 of 2 (model layer). Added the write-side recurrence primitives for the three edit scopes, for VEVENTs and VTODOs (`internal/model/recur_edit.go`). Read-side expansion + RECURRENCE-ID overrides already existed (step 3); this is the editing half.
- **Events** (all occurrences displayed): `AddOccurrenceOverride` (this-occurrence → a RECURRENCE-ID override component sharing the master's UID, in the same object), `AddException` (delete this-occurrence → EXDATE + drop any override at that slot), `CapSeries` (this-and-future → cap the master's RRULE with UNTIL, drop COUNT and later overrides; also the whole of a future-delete), `NewSeriesFrom` (the future half of a split → a fresh-UID object cloned from the master, keeping an absolute UNTIL but dropping COUNT). "All" is the existing `EditEvent` on the master.
- **Todos** (shown once, complete = advance, NextCloud-style): `AdvanceRecurringTodo` rolls DTSTART/DUE to the next occurrence (preserving their offset), decrements COUNT, and marks the todo completed when the series is exhausted. The UI orchestrates "edit this occurrence" as a detached standalone task + advance (no override-on-read needed for todos).
- Helpers: `masterComponent`, `componentAnchor` (DTSTART, else DUE), `componentRecurrenceSet`/`nextInstantAfter` (write-side twins of the read-side set), `cloneOverrideComponent` (deep prop/param copy, drops series-level RRULE/RDATE/EXDATE). Known simplification (documented in code): splitting a COUNT-bounded series leaves the tail open-ended; UNTIL-bounded and infinite series split exactly.
- Tests (`internal/model/recur_edit_test.go`): override replaces one slot and preserves the rest; exception suppresses one; cap ends the series; split caps the master + spawns a fresh-UID future series; advance rolls a weekly todo forward and completes the last occurrence. Full gate passes.
- Files: `internal/model/recur_edit.go`, `internal/model/recur_edit_test.go`.

## 2026-07-12 — Cross-view consistency F6: paste target via currentTarget

- Drift-prevention refactor (no behavior change). `pasteUnderSelection` read `a.tree.GetCurrentNode()` directly to find the paste parent, while every other action resolves the selection via `currentTarget()`. It now uses `currentTarget()` (identical in Tasks mode, where the tree node is what currentTarget returns) so paste can't silently read a stale tree selection if it's ever ungated from Tasks-only. `paste()` still gates to Tasks mode.
- (F5 was effectively resolved by M3: `editSelected` and `deleteContextual` now both lead with `GetFocus()` for the overview panes. The one remaining divergence — `e` edits the highlighted calendar from a focused-but-undrilled grid, with no `d` equivalent — is an intentional convenience, documented in `editSelected`.)
- Existing yank/paste tests cover the unchanged tree behavior. Full gate passes.
- Files: `internal/ui/yankpaste.go`.

## 2026-07-12 — Cross-view consistency F4: unify the drilled-item read via calGrid

- Drift-prevention refactor (no behavior change). `currentTarget` read the month drill inline (`a.month.selectedItems()` + `eventIndex`) but the week/day drill via `a.timegrid.selectedItem()` — two hand-synced shapes for "the drilled item," despite the `calGrid` interface already unifying `drillState`/`reDrill`. Added `selectedItem() *model.AgendaItem` to `calGrid`, implemented it on `calendarView` (mirroring the existing `timeGridView` method), and collapsed `currentTarget`'s calendar branch to `a.calendarPrimitive().(calGrid).selectedItem()`.
- Existing drilled-target tests (month + week grid Space) cover the unified path. Full gate passes.
- Files: `internal/ui/calendarview.go`, `internal/ui/app.go` (interface), `internal/ui/edit.go` (`currentTarget`).

## 2026-07-12 — Cross-view consistency F1+F2: single source for folder + checkbox

- Drift-prevention refactors (no behavior change). (F1) `hasIncompleteChildren` (the "can't complete a folder" guard) reimplemented the same "has an incomplete child" predicate as `folderSet` (which drives the ▸ folder caret) — independent copies that could silently desync the caret from the guard. `hasIncompleteChildren` now delegates to `folderSet(a.store.Todos())` so both share one definition (computed fresh; it's a completion-time call, not a draw). (F2) `nodeLabel` reimplemented the `[ ]`/`[■]` checkbox literals inline; it now delegates the non-folder case to the shared `todoMark`, so the checkbox glyph has one source across tree/month/time-grid/agenda (the tree keeps its expand-aware ▾/▸ folder caret). Fixed the stale `[ ] / [x]` doc comment.
- Existing tests (folder-completion guard, glyph renders) cover the unchanged behavior. Full gate passes.
- Files: `internal/ui/edit.go`, `internal/ui/render.go`.

## 2026-07-12 — Cross-view consistency L1: agenda selection box follows focus

- The agenda selection box was hardwired to the focused border color, while the calendar selected-day box uses the idle color until its grid is focused. Gave `agendaBoard` an `active func() bool` closure (wired to `a.agendaList.HasFocus` — a plain field read, safe in a draw path unlike `Application.GetFocus`); `drawSelBox` now uses the focused color only when active, matching the calendar day box.
- Files: `internal/ui/agendaboard.go`, `internal/ui/app.go`, `internal/ui/lowfixes_test.go`. Committed together as the Low-tier polish batch.

## 2026-07-12 — Cross-view consistency L2: document the drilled-block highlight exception

- Doc-only: the drilled item is reverse-video in month cells / the all-day band / task-marker rows, but a filled accent chip on time-grid event blocks. Added a comment explaining the exception is deliberate (reverse-video is illegible over an already-filled color block), so it doesn't read as accidental drift. No behavior change.
- Files: `internal/ui/timegridview.go`. Low-tier batch.

## 2026-07-12 — Cross-view consistency L3: drilled all-day due task keeps its marker

- A selected (drilled) all-day due task in the time-grid's top band had its label overwritten with a bare title, dropping the `[ ]`/`[■]`/`▸` marker it shows when un-selected. Now the band keeps `taskMarkerLabel` for a selected todo (bare title only for a selected all-day event, which has no marker).
- Files: `internal/ui/timegridview.go`, `internal/ui/lowfixes_test.go`. Low-tier batch.

## 2026-07-12 — Cross-view consistency L4: grab time-hint no longer names a dead key

- Grabbing an event in the agenda and pressing `j`/`k`/`J`/`K` flashed "switch to week/day view (v)…", but `v` is a no-op in agenda mode. New `grabTimeHint` helper names `(v)` only in calendar mode and points to "the week/day calendar view" in agenda mode (no dead key). `grabStatus` already omitted `v` for this case; the transient nudge hint now agrees.
- Files: `internal/ui/grab.go`, `internal/ui/lowfixes_test.go`. Low-tier batch.

## 2026-07-12 — Cross-view consistency L5: Space on a drilled event flashes instead of hiding

- With no task drilled, `Space` in a calendar view toggles the highlighted calendar's visibility (by design). But when drilled into an *event*, `Space` also flipped visibility — a surprise. The Space handler now three-ways: drilled todo → complete, drilled event → flash "Can't complete an event", nothing drilled → toggle visibility.
- Docs: `README.md`, `CLAUDE.md` (Space description).
- Files: `internal/ui/app.go`, `README.md`, `CLAUDE.md`, `internal/ui/lowfixes_test.go`. Low-tier batch.

## 2026-07-12 — Cross-view consistency M3: `e` edits the task list from the Tasks pane

- Audit fix 6 of N. The Calendars and Tasks overview panes were asymmetric for edit vs delete: `d` (`deleteContextual`) branches on `GetFocus()` and deletes the focused pane's collection (calendar or list), but `e` (`editSelected`) never opened a list's edit form — in `modeTasks`, `currentTarget()` returns the current tree node regardless of which pane holds focus, so `e` always edited the highlighted *task*. There was no keyboard path to a task list's name/color form (only `:calendar rename`/`color`).
- **Fix** (`internal/ui/edit.go` `editSelected`): check `GetFocus()` first (mirroring `deleteContextual`) — the Calendars pane opens the calendar edit form, the Tasks pane the highlighted list's (both are calendars → same `showCalendarForm`). The existing `mode == modeCalendar` fallback stays, preserving the convenience of `e` editing the highlighted calendar from the focused-but-undrilled grid.
- Docs: `README.md`, `main.md` (`e` row + calendar-form prose now note the Tasks pane edits the list, symmetric with `d`).
- Tests (`internal/ui/editlist_test.go`, new): `e` with the Tasks pane focused opens the list form (first field "Name"); `e` with the tree focused still opens the task form (first field "Summary"). Full gate passes.
- Files: `internal/ui/edit.go`, `README.md`, `main.md`, `internal/ui/editlist_test.go`.

## 2026-07-12 — Cross-view consistency M2: `<count>G` honored in the tree and drilled grid

- Audit fix 5 of N. `<count>G` (vim "go to nth item") was handled only for `*tview.List`, so it worked in the overview/agenda lists but was silently discarded in the task tree (`5G` → last node) and calendar grid (→ last day/item).
- **Fix** (`internal/ui/keys.go` `gotoBottom`): the tree branch now selects the count-th visible node (`clampIndex(count-1, len(nodes))`) instead of always the last; added a branch so a **drilled** calendar day (a list of that day's events) honors the count via `reDrill(day, count-1)`. An undrilled grid is 2D, so a count there still lands on the last day (documented). The `*tview.List` branch was tidied to share the clamp.
- Docs: `README.md`, `main.md` `gg`/`G` rows (nth item of a list, the tree, or a drilled day).
- Tests (`internal/ui/countg_test.go`, new): `<count>G` selects the nth visible tree node, `G` the last, and an over-large count clamps. Existing `TestGotoTopAndBottom` (list) still passes. Full gate passes.
- Files: `internal/ui/keys.go`, `README.md`, `main.md`, `internal/ui/countg_test.go`.

## 2026-07-12 — Cross-view consistency M1: mode indicator — tree focus is NORMAL, not DRILL

- Audit fix 4 of N. The mode badge showed `DRILL` the instant the task tree was focused (one Enter from the overview), but the parallel calendar state — grid focused, undrilled — showed `NORMAL` (a day-drill needs a second Enter). So "just dived into Main, hjkl moves things" read differently for the tree vs the grid. Owner chose to align by making tree focus read NORMAL: `DRILL` now means uniformly "drilled into a sub-element" — the calendar-day drill — and merely focusing the tree or grid is ordinary Main navigation (NORMAL). The tree has no deeper level, so DRILL never shows in Tasks.
- **Fix** (`internal/ui/render.go`): dropped the `a.mode == modeTasks && a.focused == a.tree` case from `interactionMode` (now just `grabbing` → GRAB, `gridDrilled()` → DRILL, else NORMAL).
- Removed the now-dead `a.focused` field + its `setFocus` assignment (`internal/ui/app.go`): it existed only so the draw-time mode indicator could avoid a `GetFocus()` deadlock; `interactionMode` no longer reads focus at all (only `a.grabbing` + `a.gridDrilled()`, neither takes the app lock), so the draw path stays deadlock-safe and the field is unused. Updated the `CLAUDE.md` freeze-trap note that referenced `a.focused`.
- Docs: `README.md`, `main.md`, `CLAUDE.md` mode-indicator descriptions (DRILL = calendar-day drill only).
- Tests (`internal/ui/mode_test.go`): `TestInteractionMode` now asserts drilled calendar day = DRILL and focused tree = NORMAL (was DRILL). The `modedeadlock_test.go` regression still passes (no GetFocus in the draw path). Full gate passes.
- Files: `internal/ui/render.go`, `internal/ui/app.go`, `internal/ui/mode_test.go`, `README.md`, `main.md`, `CLAUDE.md`.

## 2026-07-12 — Cross-view consistency H3: quick-set (sp/sd) works in any view

- Audit fix 3 of N. The `s` quick-set chord (`sp` priority, `sd` due) was hard-gated to the Tasks view (`app.go` `case 's'` flashed "set: Tasks view only" everywhere else), even though a task drilled into in the calendar or selected in the agenda can already be completed (`Space`), edited (`e`), deleted (`d`), and grab-nudged (`m`) — all via `currentTarget()`, the same resolver `setPriorityPrompt`/`setDuePrompt` use through `quickTaskTarget()`. Only the one-line dispatch gate made it tree-only; the handlers were already view-agnostic. Especially odd: `sd` (set due) was blocked while `m` (grab, which also changes due) was allowed.
- **Fix** (`internal/ui/app.go`): `case 's'` now unconditionally enters the set prefix. `quickTaskTarget` already flashes "Select a task first" when no task is selected, so no mode gate is needed. `z`/fold stays Tasks-only (folds are genuinely tree-specific) — deliberately not changed.
- Docs (`README.md`): noted `sp`/`sd` act on the selected task in any view, parallel to the existing `Space` note.
- Tests (`internal/ui/quickset_crossview_test.go`, new): pressing `s` in calendar mode now enters the set prefix (was refused), and `quickTaskTarget` resolves a task drilled into in the month grid. Full gate passes.
- Files: `internal/ui/app.go`, `README.md`, `internal/ui/quickset_crossview_test.go`.

## 2026-07-12 — Cross-view consistency H2: `.` hide-completed now applies to calendar + agenda

- Audit fix 2 of N (with its coupled F-sticky). The `.` show/hide-completed toggle was honored only in the task tree; the month grid, week/day time-grid, and agenda always showed completed due tasks (`[■]`) regardless. `showCompleted` was consulted only in the tree build — the calendar/agenda data builders never filtered it (a comment in `dueTasksByDay` even documented the divergence as intentional).
- **Fix** (`internal/ui/render.go`): added `completedVisible(t)` (the single rule — shown unless completed-hidden and not stickyDone-pinned) and `visibleTodos(todos)`; applied across the tree build, `calItems` (month), `dayItems` (agenda + agenda-left), and `dueTasksByDay` (time-grid). The tree's inline filter now calls the shared helper (removes a duplicated condition). Updated the stale `dueTasksByDay` comment.
- **F-sticky** (`internal/ui/edit.go`): dropped the `a.mode == modeTasks` gate in `toggleComplete` so a just-completed task is pinned visible (`stickyDone`) in any view — otherwise checking one off in the calendar/agenda while completed are hidden would make it vanish instantly, violating "keeps it visible until you leave the view." stickyDone still clears on switching list or pane (`setMode`), which is the calendar/agenda analog of "leaving the list."
- `reloadCurrent` (`internal/ui/app.go`): the agenda case now rebuilds the left list too, so `.` updates both halves of the agenda together.
- No doc change: `main.md`/`README.md` already state completed tasks are "hidden by default; `.` toggles" in week/day — the code now matches the spec.
- Tests (`internal/ui/hidecompleted_test.go`, new): a completed due task is present in the agenda + time-grid builders when completed are shown, absent when hidden, and kept when sticky-pinned; and completing a task via Space in the agenda while hidden pins it (F-sticky). Full gate (build/vet/staticcheck/`go test ./...`) passes.
- Files: `internal/ui/render.go`, `internal/ui/edit.go`, `internal/ui/app.go`, `internal/ui/hidecompleted_test.go`.

## 2026-07-12 — Cross-view consistency H1: agenda board shows task glyphs

- Cross-view consistency audit, fix 1 of N. The full-detail Agenda center board (`agendaBoard`) was the one task renderer that drew neither the `[ ]`/`[■]` checkbox nor the `▸` folder caret — a task showed as `<when>  <summary>` with completion conveyed only by a status word, while the tree, month grid, week/day grid, and even the Agenda *left* list all route through the shared `todoMark`. The board struct was never given an `isFolder` closure (only `itemColor` was wired), so it structurally couldn't draw a caret.
- **Fix** (`internal/ui/agendaboard.go`): added an `isFolder func(uid string) bool` field + a `folderItem` helper on `agendaBoard`; `agendaItemLines` now takes a `folder bool` and prepends `todoMark(t, folder)` to the task title line (placed after the time label so the time column stays aligned between tasks and events). Events are unchanged (no marker). Wired `a.agenda.isFolder = a.isFolder` in `app.go` next to the existing `month`/`timegrid` wiring.
- No doc change: `README.md`/`main.md` already state the caret/checkbox appear in the agenda — this was the code failing to match the spec, not a behavior change to document.
- Tests (`internal/ui/agendaboard_test.go`, new): `agendaItemLines` renders `[ ]` incomplete / `[■]` completed / `▸` folder for tasks, and no marker for events. Full gate (build/vet/staticcheck/`go test ./...`) passes.
- Files: `internal/ui/agendaboard.go`, `internal/ui/app.go`, `internal/ui/agendaboard_test.go`.

## 2026-07-10 — Wrap-up: docs + freeze guardrails for the next agent

- End-of-session housekeeping. Added a CLAUDE.md Architecture Rules block documenting the two tview freeze traps fixed today (no app-lock calls from a draw func; keep the task tree on `SetGraphics(false)`) plus the "never hand-edit `vendor/`" rule, so they aren't reintroduced.
- Fixed a stale comment in `buildTreeForList` (it referred to tree "connector stems", which no longer render with branch-graphics off).
- Refreshed project memory: added `project-status.md` (steps 1–10 + this session's grab/yank/mode-indicator polish complete; next = step 11 recurrence editing, which also unblocks grab's deferred recurring-event/undated-task cases).
- Files: `CLAUDE.md`, `internal/ui/render.go` (comment), memory files.

## 2026-07-10 — Fix freeze on entering Tasks view (mode-indicator draw deadlock)

- Reported: the app hangs the instant `t` is pressed. Root cause is the **status-bar mode indicator**: its `SetDrawFunc` (`drawModeIndicator` → `interactionMode`) called `a.tv.GetFocus()`, which takes the tview app **read-lock**. But `Application.draw()` holds the **write-lock** for the whole draw, and Go's `sync.RWMutex` isn't reentrant — so reading focus during a draw self-deadlocks. It only triggered in Tasks mode because the `GetFocus()` call sat behind a short-circuited `a.mode == modeTasks && …` (calendar/agenda never evaluated it), and only in the live event loop (a one-shot `primitive.Draw()` in tests doesn't take the app lock — which is why the earlier draw tests missed it). Independent of tree depth/data.
- **Fix**: track the focused pane in `a.focused`, set in `setFocus` (the single focus path — mouse and modal-restore both route through it), and have `interactionMode` read `a.focused` instead of calling `GetFocus()` during the draw. No lock taken from a draw func.
- Test: `internal/ui/modedeadlock_test.go` runs the real event loop in Tasks mode against a simulation screen and waits for `SetAfterDrawFunc` to fire; a deadlocked draw never fires it, so a 5s watchdog trips. Verified it fails (times out) with the old `GetFocus()` call and passes with the fix. Full gate + `-race` pass.
- Files: `internal/ui/app.go` (field + `setFocus`), `internal/ui/render.go` (`interactionMode`), `internal/ui/modedeadlock_test.go`.

## 2026-07-10 — Fix app-freeze: disable tview tree branch-graphics

- Diagnosed a reported "crash" — actually a **100% CPU hang**, not a panic. Root cause is upstream `github.com/rivo/tview` v0.42.0 `TreeView.Draw`: the ancestor-branch-drawing loop does `if ancestor.graphicsX >= width { continue }` without advancing `ancestor`, so it spins forever whenever a node's ancestor indent reaches the tree pane's width. Triggered by a deep-enough subtask tree in a tree pane narrower than the deepest indent (~12–15 levels at 80 cols; far shallower in a narrow terminal or after widening the overview). Our recent yank/paste makes deep trees easy to build, which is why it surfaced now — but the faulty line is pre-existing library code (still present on tview master), and grab/yank/mode-indicator code is all correctly guarded (confirmed by a fuzzing sweep of the since-audit diff).
- **Fix**: `a.tree.SetGraphics(false)` in our own code — the entire buggy loop is gated behind `if t.graphics`, so this sidesteps it with **no edits to vendored/third-party source**. An earlier commit patched the vendored file directly; that was reverted (the vendored tview is now byte-identical to upstream v0.42.0) in favour of this in-code fix, since editing a vendored dep is silently lost on the next `go mod vendor`. Cost: the tree loses tview's `├─ │ └─` connector lines; nesting is still shown by indentation and our own `▸`/`▾` folder carets.
- Test: `internal/ui/treedraw_regress_test.go` builds a 20-deep subtask chain and draws the app's tree in an 8-col pane under a 5s watchdog — passes now, and (verified) hangs/times out if `SetGraphics(false)` is dropped.
- Files: `internal/ui/app.go`, `internal/ui/treedraw_regress_test.go`; reverted `vendor/github.com/rivo/tview/treeview.go` to pristine.

## 2026-07-10 — Status-bar mode indicator + outlined status bar

- Surfaced the **interaction mode** as a vim-style badge, prompted by grab mode making "modes" concrete. Distinguishes the *interaction* mode (what the movement keys act on now) from the *view* context (Calendar/Tasks/Agenda, already shown as text).
- **Impl** (`render.go`, `app.go`): new `interactionMode()` derives the mode from existing state — `GRAB` (`a.grabbing`), `DRILL` (calendar day drilled via `gridDrilled`, or dived into the task tree), else `NORMAL` — with no new state machine, so it doubles as the seam for a future dispatch cleanup. The badge is a custom-drawn `*tview.Box` (`drawModeIndicator`, `SetDrawFunc`) rather than a TextView, so it stays live every frame regardless of which transition path fired (drill/undrill and grab enter/exit don't all funnel through `updateStatus`). Filled high-contrast chip for the active modes (DRILL = teal, GRAB = yellow), dim label at rest.
- Status bar now has **four sections** (mode · general/results · command view · sync) and is **outlined** with a rounded border like the primary panes (3 rows instead of 1); renamed the very-bottom controls line to the "help bar" in the docs.
- Docs: help overlay row, `main.md` status-bar section, `CLAUDE.md` UI line, `README.md`.
- Tests (`mode_test.go`): `interactionMode` transitions (NORMAL/GRAB/DRILL) and a render test asserting the `NORMAL`/`GRAB` badge and the border paint to a simulation screen. Full gate passes.

## 2026-07-10 — Grab mode (`m`): move/resize events, nudge task due dates

- Update 2 of 2: the temporal-manipulation layer, unified across tree/calendar/agenda (the "grab mode" designed earlier). Complements yank/paste (structural) — grab only touches *when*.
- **Impl** (`internal/ui/grab.go`, `app.go`): `m` grabs the current target (via `currentTarget`); modal — `globalKeys` routes every key to `handleGrabKey` while `a.grabbing`. **Event** (week/day view): `j`/`k` ±hour, `h`/`l` ±day, `J`/`K` resize the end (min-duration guard); month/all-day = day-move only. **Task**: `j`/`k` due ±day, `h`/`l` ±week. Edits commit to the store on each nudge (via `EditEvent`/`EditTodo` + `draftFromEvent`/`draftFromTodo`, preserving all other props) so views update live; `focusGrabbed` re-anchors the calendar to the item's (possibly new) day and re-drills onto its block, or re-selects the task by UID. `Enter` keeps (pre-grab snapshot = one undo step); `Esc` `Restore`s the snapshot. Undated tasks and recurring events are skipped with a hint (recurrence editing is step 11).
- Docs: help overlay, `main.md` keymap, `CLAUDE.md`, `README.md`.
- Tests (`grab_test.go`): task due nudge (+2 days, commit), undated-task skip, event day-move + resize + Esc-reverts, and `m`/`j`/Enter wiring through `globalKeys`. Full gate + `-race` pass.

## 2026-07-10 — Yank/paste update: cut vs copy, top-level paste, persistent clipboard (tasks)

- Owner request (Update 1 of 2; grab mode is Update 2). Reworked task yank/paste around a small target-agnostic clipboard: cut vs copy, paste at the top level, and a clipboard that survives paste (multi-paste). Scoped to **tasks** (events get the planned grab mode).
- **Keys** (`internal/ui/app.go`): `y` = cut (move on paste), `Y` = copy (duplicate), `p` = paste under the selected task, `P` = paste at the list top level. `Y`/`P` were free.
- **UI** (`internal/ui/yankpaste.go`): `setClip(cut)` records the clipboard (`yankUID` + `yankCut`) from `currentTarget()`; `paste(targetParent)` dispatches to move (existing `reparentTo`/`moveSubtree`) or the new `copySubtree`. The clipboard is **no longer cleared** on paste (was `a.yankUID = ""`), so the same task can be pasted repeatedly. Cycle guards (onto-self / into-own-subtree) apply only to cut; a copy is an independent tree. `copySubtree` duplicates root+descendants with fresh UIDs, remapping each child's parent link to its copy, all-or-nothing with rollback; undo deletes the copies.
- **Model** (`internal/model/edit.go`): new `CopyTodo(obj, uid, newUID, newParentUID, …)` — re-keys UID + re-parents while preserving every other iCal property (iron rule), via the same clone-through-encode path as `EditTodo`.
- Docs: help overlay, `main.md` keymap, `CLAUDE.md`, `README.md`. Memory: recorded the grab-mode design for Update 2 ([[grab-mode-plan]]).
- Tests: `copypaste_test.go` (`Y` copy duplicates with a fresh UID + persists for multi-paste; `P` pastes at top level; subtree copy remaps children to the copied parent); `edit_test.go` `TestCopyTodo` (fresh UID + new parent, preserves summary/categories/X-props/VALARM); migrated the two existing move tests to the persistent-clipboard assertion and the renamed `pasteUnderSelection`. Full gate + `-race` pass.

## 2026-07-10 — Hidden calendars drop their color bullet

- Owner request: hiding a calendar should remove the `●` color bullet in the Calendars list, so a hidden calendar reads more clearly at a glance (alongside the existing `(hidden)` marker).
- **Fix** (`internal/ui/render.go` `buildCalendars`): only prepend the color bullet when the calendar isn't hidden (`ok && !a.hidden[cal.ID]`). Name/count/markers unchanged.
- Docs: `CLAUDE.md`, `README.md` (bullet/color-dot descriptions note it drops when hidden).
- Tests (`colorrender_test.go`): `TestHiddenCalendarDropsColorBullet` — a colored calendar shows the bullet when visible and drops it (with `(hidden)` shown) when hidden. Full gate pass.

## 2026-07-10 — Audit items 15 & 16: mouse — wheel-paging dropped, click-to-fold confirmed

- **15 — wheel paging the calendar grid**: owner chose to drop it from the spec rather than implement. Updated `main.md`'s Mouse section (keyboard `f`/`b` pages the grids; the custom widgets take no wheel handler).
- **16 — click a folder to expand/collapse**: audit finding was a **false positive** — this already works. `a.tree.SetSelectedFunc` (`app.go`) toggles a node's expansion and updates its `▸`/`▾` caret, and tview's TreeView fires that callback on a left-click (not just Enter). The agent missed it because the wiring is in `app.go`, not `mouse.go`. Verified by simulation (click flips a folder expanded→collapsed) and locked in with a regression test.
- Tests (`treeclick_test.go`): `TestTreeClickTogglesFolder` drives a left-click on a folder row and asserts its expansion toggles. Full gate + `-race` pass.
- **Audit follow-up plan complete** — all 16 items resolved (13 changes committed; item 9 deferred to step 12; item 16 was already implemented).

## 2026-07-10 — Audit item 14: `:calendar new` command

- Gap: main.md's command list included `:calendar new` but `cmdCalendar` only handled rename/color/hide/show (creation was only on the `ic`/`il` chords).
- **Fix** (`internal/ui/command.go`): `:calendar new` opens the create/edit calendar form (`showCalendarForm("", 0)`), handled before the "select a calendar first" guard since it needs no highlight. Fallback hint + help overlay updated to list `new`.
- Tests (`calendarcmd_test.go`): `TestCalendarNewOpensForm` — `:calendar new` opens the form page. Full gate pass.

## 2026-07-10 — Audit item 13: clearing an event's end removes DTEND

- Owner decision: make the event edit contract symmetric with the todo one — `applyEvent` only wrote DTEND when End was set, so a zero End left the old DTEND in place (couldn't make an event zero-duration). Benign today (the UI form always supplies End), but the asymmetry with `applyTodo`'s DUE handling was real.
- **Fix** (`internal/model/edit.go`): `applyEvent` now always drops DURATION, writes DTEND when End is set, and `Del`s DTEND when End is zero (mirroring how a missing DUE is deleted).
- Tests (`edit_test.go`): `TestEditEventClearsDTEND` — editing an event with a zero End removes DTEND while DTSTART remains. Full gate + `-race` pass.

## 2026-07-10 — Audit item 12: re-fetch the server version on a 412 conflict

- Owner decision: on a 412 (server changed since our download), the conflict was stashed with the `serverObj` fetched at the *start* of the sync — stale by definition of a 412, so the conflict view could show an out-of-date server side and keep-server needed an extra round.
- **Fix** (`internal/caldav/client.go`, `sync/sync.go`): new `Client.GetObject(ctx, href)` (wraps go-webdav's `GetCalendarObject`) fetches a single resource fresh; `Syncer` gained `GetObject`; `pushUpdate`'s 412 branch now re-fetches the current server version and stashes that (falls back to the start-of-sync `serverObj` if the re-fetch fails). Conflict now reflects the true server state and resolves in one round.
- Tests (`sync_test.go`): fake gained `GetObject` (+ a `getData` override so the re-fetched version can differ, and a `gets` spy); `TestSyncRefetchesOn412` asserts the stashed conflict carries the fresh `srv-2` ETag, not the stale `srv-1`. Full gate + `-race` pass.

## 2026-07-10 — Audit item 11: split the calendar pending-props flag (name vs color)

- Owner decision: the single `pendingProps` flag meant a pending local **name** edit blocked adopting the server's **color** (and vice-versa, now that name is pulled too). Split it.
- **Fix** (`internal/store`): `calState`/sidecar gained `pendingName` + `pendingColor` (`pending_name`/`pending_color`), replacing `pendingProps`; the legacy `pending_props` is still read and mapped onto both for backward compatibility. `UpdateCalendarMeta` sets each flag only for the field it changed; `SyncCalendarColor`/`SyncCalendarName` skip only on their own flag; `PendingCalendarProps` emits **only the pending field(s)** so a PROPPATCH can't clobber a concurrent server edit to the other; `MarkCalendarPropsSynced` clears both.
- Tests (`pendingflags_test.go`): a pending name doesn't block the color pull (and the pull-name is still blocked + only the name is pushed); a legacy `pending_props` sidecar loads as both pending. Full gate + `-race` pass.

## 2026-07-10 — Audit item 10: pull server-side calendar renames

- Owner decision: names are "server-authoritative" but only color/read-only/components were pulled each sync — a rename on NextCloud web or another client never showed up locally. Also confirmed in-app renaming already exists (`:calendar rename` and the `e` edit-form Name field). (Item 9 — debounced push — deferred to build step 12.)
- **Fix** (`internal/store/calendar.go`, `sync/sync.go`): new `SyncCalendarName` mirrors `SyncCalendarColor` — adopt the server's display name, server-authoritative except when a local rename is still pending a PROPPATCH (no-op on empty/unchanged). Called per calendar in the sync discovery loop alongside the color pull.
- Docs: `main.md` calendar-metadata decision + `CLAUDE.md` (names *and* colors sync both ways).
- Tests (`sync_test.go`): `TestSyncPullsCalendarRename` (server rename adopted) and `TestSyncDoesNotClobberPendingLocalRename` (pending local rename wins and is pushed). Full gate + `-race` pass.

## 2026-07-10 — Audit item 8: cancellable sync context (clean shutdown)

- Owner decision: honor the "all network I/O is cancellable" architecture rule at the one spot that didn't — the sync caller. (Data was already safe either way via atomic writes + ETag reconciliation; this is about a clean unwind vs a detach/hard-kill on quit.)
- **Fix** (`internal/ui/app.go`, `sync.go`): the app now holds `ctx`/`cancel` (`context.WithCancel`, created in `newApp`); `Run` defers `a.cancel()` so quitting cancels it. `triggerSync` passes `a.ctx` instead of `context.Background()`, so an in-flight background sync unwinds at its next `ctx.Err()` checkpoint (the sync/caldav stack already threads ctx everywhere).
- Tests (`sync_test.go`): `TestSyncUsesCancellableContext` — the sync receives a live context and `a.cancel()` cancels it. Full gate + `-race` pass.

## 2026-07-10 — Audit item 7: surface :config reload errors + validate appearance enums

- **7a — reload connection errors reach the UI** (`cmd/lazyplanner/main.go`, `ui.ConfigReload`, `command.go`): `buildSyncFn` now returns `(closure, warning)` instead of printing to stderr; on a `:config` reload the warning (e.g. "password_command failed (offline)") is carried in `ConfigReload.Warning` and flashed in the status bar, so a reload that dropped to offline isn't lost behind the suspended TUI. Startup still prints the warning to stderr.
- **7b — unknown [appearance] values warn** (`internal/config/config.go`): `appearanceWarnings` checks `first_day_of_week`/`default_view`/`time_format`/`date_format`/`color_mode` against their allowed sets and appends a non-fatal warning naming any typo (value still falls back to the default), joined with the permission warning.
- Tests: `config` — `TestLoadWarnsOnUnknownAppearance` (bad default_view/time_format named); `ui` — `TestApplyConfigReloadFlashesWarning`. Full gate + `-race` pass.

## 2026-07-10 — Audit item 6: wire the [appearance] config options

- The four `[appearance]` options were parsed but never consumed (the UI hardcoded them). Wired all four end-to-end.
- **Plumbing** (`cmd/lazyplanner/main.go`, `ui.Options`, `app`): pass `FirstDayOfWeek`/`DefaultView`/`TimeFormat`/`DateFormat`; `Run` resolves them into `a.weekStartMonday`, `a.viewMode`, `a.clock24`, `a.dateISO`, and mirrors `clock24` onto the three custom widgets.
- **Format helpers** (`internal/ui/format.go`): `clockStr` (12h/24h), `hourAxisLabel` (axis/cell hour), `dateStr`/`dateShortStr` (US `01/02/2006` vs ISO `2006-01-02`), plus `parseWeekStartMonday`/`parseDefaultView`. Replaced the literal `Format("3pm")`/`"3:04pm"`/`"15:04"`/date calls across `render.go`, `calendarview.go`, `timegridview.go`, `agendaboard.go`, `sync.go` (agenda times, hour axis, event-block span, month-cell times, due dates, detail When/Due, status-bar date, last-sync time). Editable form date/time fields keep their fixed ISO/24h layout (they round-trip through the parser).
- **Effects**: `first_day_of_week=sunday` → Sunday-start grid; `default_view=week|day` → opening view; `time_format=24h` → 14:30 clock everywhere; `date_format=iso` → 2026-07-04 dates. Note: `date_format` now renders **numeric** dates (default US `07/04/2026`) in the data displays (due dates, detail, status) — previously month-name `Jan 2`; the calendar/agenda prose headers stay month-name.
- Tests (`format_test.go`): `clockStr`/`hourAxisLabel`/`dateStr` tables, `parseWeekStartMonday`/`parseDefaultView`, and a detail-pane render asserting 24h+ISO take effect. Updated the sync-status test to set `clock24`. Full gate + `-race` pass.

## 2026-07-10 — Audit item 5: task-subtree zoom (`>`/`<`) implemented

- Closed the highest-value gap: `>`/`<` subtree zoom was documented (main.md/CLAUDE.md) but entirely unimplemented. Built it to spec (full re-root + breadcrumb).
- **Impl** (`internal/ui/render.go`, `app.go`): new `a.zoomUID` (task the tree is re-rooted at; "" = list root). `buildTreeForList` now, when zoomed, finds the node (`findTodoNode`), shows its children as the tree roots, and sets the root label to a `List / ancestor / task` breadcrumb (`zoomBreadcrumb`). `zoomInTree` (`>`) re-roots at the selected task; `zoomOutTree` (`<`) pops one level (to the task's parent, or the list root). A stale zoom (task deleted) resets; switching lists clears it. `>`/`<` wired in `globalKeys` (Tasks mode only) — they were inert before.
- Docs: help overlay (`> / <` row), `README.md` Tasks section. (main.md/CLAUDE.md already described it.)
- Tests (`zoom_test.go`): `TestTreeSubtreeZoom` — zoom-in re-roots with a breadcrumb and shows the subtask as the child, zoom-out returns to the list root, and a list switch clears the zoom. Verified the render visually (`Personal / ECE384` root over its subtasks). Full gate + `-race` pass.

## 2026-07-10 — Audit item 4: atomic .ics/sidecar mutations (rollback on sidecar failure)

- Owner decision: make each store mutation all-or-nothing across the two on-disk files. Before, the `.ics` was written/removed first, then the sidecar; a sidecar-write failure (disk-full/EIO) left the `.ics`+memory changed but the sidecar stale — across a restart a lost tombstone could resurrect a deleted item or a lost dirty flag strand an edit.
- **Fix** (`internal/store/mutate.go`): new `revertMutation` restores the `.ics` (rewrite previous content, or remove for a create) plus the in-memory resource/conflict/tombstone maps to their pre-write state. `writeResource` and `remove` capture the prior state and call it when `writeSidecar` fails, then return the error — so the two files never diverge.
- Tests (`rollback_test.go`): sabotage the sidecar by replacing it with a directory (atomic rename fails, `.ics` write still works); `TestDeleteRollsBackOnSidecarFailure` (resource + no tombstone survive, and a later delete works) and `TestPutRollsBackOnSidecarFailure` (previous content kept, not left dirty). Full gate + `-race` pass.

## 2026-07-10 — Audit item 3: one calendar's failure no longer aborts the whole sync

- Owner decision: a per-calendar download/REPORT failure should be recorded and skipped, not abort the entire sync — so a flaky calendar can't block healthy ones (with pending edits) from syncing.
- **Fix** (`internal/sync/sync.go`): the discovery loop now `recordSkip`s a failed `reconcileCalendar` and continues to the next calendar, instead of returning the error. A cancelled context still aborts the whole run (checked before skipping). `res.Calendars` counts only successfully-reconciled calendars.
- Tests (`sync_test.go`): fake gained a `failDownload` hook; `TestSyncSkipsFailedCalendarContinuesRest` puts the failing calendar first and asserts the healthy one still pushes its edit and the failure lands in `res.Skipped`. Full gate + `-race` pass.

## 2026-07-10 — Audit item 2: cross-list task move rolls back on partial failure

- Owner decision: make the cross-list yank/paste move **all-or-nothing**. Previously `moveSubtree` did Put(target)+Delete(source) per node and only recorded undo after the whole loop, so a mid-loop failure could leave nodes moved with no undo (or a node duplicated in both lists).
- **Fix** (`internal/ui/yankpaste.go`): accumulate a `rollback` list of reversals as each write commits (Put → `Forget` the copy; Delete → `Restore` the original, which clears its tombstone). On any error, run them newest-first so the subtree ends up entirely back in the source list; `yankUID` is kept so the user can retry. Undo is still pushed only on full success.
- Tests (`yankpaste_test.go`): `TestMoveSubtreeRollsBackOnFailure` forces a mid-move failure by making the source calendar dir read-only (source delete fails, dest Put succeeds) and asserts both nodes remain in the source with no stray copy in the dest (skips as root, where the perms trick doesn't hold). Full gate + `-race` pass.

## 2026-07-10 — Audit item 1: confirm read-only before discarding a 403'd edit

- Owner decision on the reactive-403 data-loss risk: don't trust a bare 403 (it can be transient — auth blip, rate-limit, WAF, maintenance); re-check the calendar's privileges and only discard the stuck local edit when read-only is *confirmed*.
- **caldav** (`privileges.go`): new `Client.CalendarWritable(ctx, calPath)` — a Depth-0 `current-user-privilege-set` PROPFIND for one calendar (reusing the existing privilege parsing), fail-open on an ambiguous answer.
- **sync** (`sync.go`): `Syncer` gained `CalendarWritable`; `markReadOnlyDiscard` → `handleWriteForbidden` re-checks on a 403: confirmed read-only → flag + `Forget` (as before); still-writable or the check errored → **keep the local edit** and `recordSkip` a "kept local change, will retry" message. `pushUpdate` now takes the calendar path so it can re-check.
- Tests (`sync_test.go`): fake gained `CalendarWritable` (+ `writable`/`writableErr` maps); `TestSyncReactiveReadOnlyOn403` now sets the re-check to confirm read-only; new `TestSyncTransient403KeepsEdit` asserts a writable-on-recheck 403 keeps the edit and doesn't flag read-only. Full gate + `-race` pass.

## 2026-07-10 — Full-codebase audit: bug + undefined-behavior fixes

- Ran a parallel multi-agent audit of the whole codebase (model, store, caldav+sync, ui, config/cmd) for genuine bugs, undefined behaviors, and spec-vs-impl feature gaps. Fixed the genuine bugs and obvious undefined behaviors automatically; gaps and design-call items are reported to the owner separately.
- **[BUG] crash — `model.BuildTree` stack-overflow on a malformed cycle** (`tree.go`): a 2-cycle B↔C plus a third child of B made the unguarded `descends` walk recurse forever (violates never-crash-on-bad-.ics). Added a visited set (`descendsSeen`); cyclic nodes are safely orphaned. Regression test `TestBuildTreeCycleWithExtraChild`.
- **[BUG] recurrence — Windows/Outlook TZID on RDATE/EXDATE/RECURRENCE-ID broke expansion** (`recurrence.go`): these parsed via `prop.DateTime` (fails on non-IANA zones) instead of the resilient `resolveDateTime` used for DTSTART, so an Outlook event could blank the calendar or drop a series. Switched all three to `resolveDateTime`. Fixture `recur_exdate_winzone.ics` + `TestOccurrencesExdateWindowsZone`.
- **[BUG] sync — "keep server" on a locally-edited-but-remotely-deleted conflict was unresolvable** (`store/conflict.go`): empty `ServerData` → `model.Decode` EOF → error forever. Now treats empty ServerData as "accept the deletion" (`Forget`). Test `TestResolveKeepServerAcceptsRemoteDeletion`.
- **[BUG] caldav — update PUT with no stored ETag was unconditional** (`caldav/object.go`): `create=false && ifMatch==""` sent no precondition (blind overwrite). Now sends `If-Match: *` (condition on existence) so it can't resurrect a server-deleted resource.
- **[BUG] ui — folder completion rule bypassed by the edit form's Completed checkbox** (`edit.go`): `showTodoForm` Save called `EditTodo` (no child check). Added the same guard `Space` uses (`hasIncompleteChildren`).
- **[UNDEFINED] ui — tview style-tag injection in labels** (`render.go`, `conflicts.go`): only the Calendars panel escaped user text; task/calendar/list names, agenda titles, tree nodes, the detail pane, and conflict rows passed raw strings, so a name like `Review [draft]` mis-rendered (and the Tasks-panel `[ro]` marker never showed). Wrapped every user-supplied field in `tview.Escape`. Tests `TestDetailEscapesTagLikeText`, `TestTreeLabelEscapesTagLikeText`.
- **[UNDEFINED] ui — search Esc didn't re-collapse folders it auto-expanded** (`search.go`): `currentSelectionRestore` now snapshots/restores every node's expansion. Also fixed a focus-stack leak: Enter-on-match popped nothing, slowly growing `focusStack`.
- **[UNDEFINED] store — `CreateCalendarLocal` kept the caller's `components` slice** (`calendar.go`): now copies it (matching `SetCalendarComponents`).
- **[UNDEFINED] config — `password_command` failure hid the cause** (`config.go`): capture stderr and fold the first line into the error (e.g. "bw not logged in").
- Full gate (`build`/`vet`/`staticcheck`) + `go test -race ./...` pass.

## 2026-07-10 — Color-path audit: RGB-based swatch matching (alpha/case-insensitive)

- Pre-commit sweep of the coloring behaviors for undefined edges. Findings were sound (new calendars render their color via `refresh`; `normalizeColor` matches `model.ParseHexColor`'s accepted forms; read-only calendars are guarded on every color path; blank-on-edit = unchanged) — except one.
- **Fix** (`internal/ui/colorpicker.go`, `calendar.go`): the picker matched the calendar's current color to a swatch with `strings.EqualFold`, so a server color carrying an **alpha suffix** (NextCloud stores `#RRGGBBFF`) — or a different case / missing `#` — failed to preselect the swatch or draw the `✓`, silently landing on "Custom". Added `sameColor` (compares parsed RGB, ignoring alpha/case/`#`) and a `colorPicker.preselect` method used by both the opener and the `✓` render.
- Tests (`colorpicker_test.go`): `TestSameColor` (case/`#`/alpha variants), `TestColorPickerPreselect` (alpha color → swatch 6, non-palette → Custom, empty → first). Full gate + `-race` pass.

## 2026-07-10 — Created calendars default to a palette color (never colorless)

- Owner report: creating a calendar/list without picking a color left it colorless (app default). It should always get a color.
- **Fix** (`internal/ui/colorpicker.go`, `calendar.go`): new `defaultCalendarColor = "#0082c9"` (NextCloud blue, a palette swatch). The create form's Color field is pre-seeded with it (so it's visible and the picker preselects it), and `createCalendarWithColor` falls back to it when the field is blank — so every created collection always has a color. Edit is unaffected (blank there still means "leave unchanged").
- Docs: `main.md`, `README.md`, `CLAUDE.md`.
- Tests (`colorpicker_test.go`): `TestCalendarCreateDefaultsColor` — creating with an empty color yields `defaultCalendarColor`. Full gate + `go test -race ./internal/ui` pass.

## 2026-07-10 — Color built into the calendar create/edit form (fixes colorless new calendars)

- Owner report + request: creating a calendar/list **never assigned a color** (it showed the default until manually recolored in NextCloud), and the color picker should be **part of the create/edit GUI** rather than a chained step. Root cause: `createCollection` called `CreateCalendarLocal` with `CalendarMeta{DisplayName: name}` — no color — so a new calendar (and its MKCALENDAR) carried none.
- **Unified form** (`internal/ui/calendar.go`): replaced `createCollection` with `showCalendarForm(editID, defaultType)` — one form for create *and* edit. Fields: Name, Type (create only; a calendar's component set can't change on the server), and a **Color** hex field with a **"Pick color…"** button that opens the swatch grid; the pick is written back into the field (which also accepts a typed hex). Create passes the color to `CreateCalendarLocal` (new `createCalendarWithColor` seam), so it's set from the start and carried in the MKCALENDAR; Save uses `UpdateCalendarMeta(name, color)`. `e` on the Calendars pane now opens this edit form (was: the bare picker).
- **Nested modals** (`internal/ui/edit.go`, `app.go`): the picker opens *over* the form, so modal focus save/restore became a **stack** (`focusStack []focusState`, push in `captureFocus` / pop in `restoreFocus`) instead of a single slot — backward-compatible for the existing single-level modals, and it lets form→picker→custom-hex-prompt nest and unwind cleanly.
- **Picker opener** refactored into `openColorPickerCallback(current, title, onPick)` (shared by the form's Pick button and the direct `:calendar color` recolor); `openColorPicker(calID)` now wraps it with `applyCalendarColor`. `:calendar color` no-arg still opens the picker; with a hex still sets directly.
- Docs: help overlay, `main.md` (Color section + `e` row), `CLAUDE.md`, `README.md`.
- Tests (`colorpicker_test.go`): `TestCalendarFormCreatesWithColor` (create seam stores the color + PendingCreate), `TestFocusStackNesting` (push/pop balance, extra pop is safe), `TestEditOnCalendarsPaneOpensForm` (was OpensPicker). Verified headlessly: the create form renders Name/Type/Color + Pick color…/Create/Cancel, and a nested picker-over-form opens both pages then unwinds leaving the form intact. Full gate (`build`/`vet`/`staticcheck`) + `go test -race ./internal/ui` pass.

## 2026-07-10 — Swatch-grid color picker for calendars (create + edit)

- Owner request: pick a calendar color in the UI instead of typing a hex. Chosen (via Q&A): a **swatch-grid picker** with a "Custom hex…" escape hatch, reachable when creating a calendar, from `e` on the Calendars pane, and from `:calendar color` with no hex.
- **Picker widget** (`internal/ui/colorpicker.go`): a custom tview `Box` primitive drawing `calendarPalette` (a 15-color NextCloud-like preset set, incl. NextCloud blue `#0082c9`) as a 5×3 grid of color-filled cells, a trailing "Custom hex…" entry, and a "current: #…" hint. Cursor is accent brackets around the selected swatch; the calendar's current color is marked with a contrasting `✓`. `hjkl`/arrows move (grid + drop-to/return-from Custom), `Enter` selects, `Esc` cancels — via `onSelect`/`onCustom`/`onCancel` callbacks.
- **Wiring** (`internal/ui/calendar.go`): `openColorPicker(calID)` preselects the current color (or the first swatch for a new calendar), applies a pick via `applyCalendarColor` (offline-first `UpdateCalendarMeta`, pushed as PROPPATCH on next sync — same path as `:calendar color`), and routes "Custom hex…" to `promptInput` + `normalizeColor`. It's a **standalone modal** (never nested — `openModal` uses a single saved-focus), so `createCollection` chains into it after the name/type form, and `editSelected` opens it when `e` is pressed on the Calendars pane with no item drilled. `cmdCalendar` "color" opens the picker with no arg and sets directly with a hex (backward compatible), sharing `applyCalendarColor`.
- Docs: help overlay (`e` + `:calendar`), `main.md` (Creation/Color section, `e` keymap row, `:calendar` command), `CLAUDE.md`, `README.md`.
- Tests (`colorpicker_test.go`): picker navigation (grid clamps, drop-to/return-from Custom), select/custom/cancel callbacks, `applyCalendarColor` sets the stored color, `e` on the Calendars pane opens the picker, and `:calendar color` routes (hex sets directly, no-arg opens the picker). Verified the render visually (5×3 grid, cursor brackets, `✓` on the current color, Custom entry). Full gate (`build`/`vet`/`staticcheck`) + `go test -race ./internal/ui` pass.

## 2026-07-10 — `:config` reload now applies color_mode live

- Follow-up to the truecolor change: `:config` (edit in `$EDITOR`, reload on exit) previously re-applied only the `[server]` connection; a `color_mode` change needed a full restart. Now it applies live.
- **Reload payload** (`internal/ui/app.go`): the `EditConfig` callback now returns a `ConfigReload{Sync, ColorMode}` struct instead of a bare sync closure — keeps config parsing in `main` (architecture rule) while letting the UI apply more than one reloaded setting.
- **Apply** (`internal/ui/command.go` `applyConfigReload`): still swaps the sync closure; additionally re-parses `color_mode` and, when it changed, updates `a.colorMode` and rebuilds the color index + Calendars list (the list bullets bake in the color tag; center views read the index live and repaint on resume). The highlighted calendar row is preserved across the rebuild. `auto`↔`truecolor` is a no-op for the mode (both parse to colorAuto) — 24-bit output is negotiated at tcell init, so that specific switch still needs a restart (documented).
- **main** (`cmd/lazyplanner/main.go`): `editConfigFn` returns `ui.ConfigReload{Sync: buildSyncFn(...), ColorMode: cfg.Appearance.ColorMode}`; the account-change guard is unchanged. Dropped the now-unused `sync` import from `command.go`.
- Docs: `README.md` (`:config` note — color_mode applies live, auto↔truecolor/account need restart), `main.md` config-editing decision.
- Tests (`configreload_test.go`): migrated the two existing tests to the `ConfigReload` signature; added `TestApplyConfigReloadAppliesColorMode` — a reload to `off` clears a calendar's color from the index, and back to `16` repopulates it, with `a.colorMode` tracking each change. Full gate (`build`/`vet`/`staticcheck`) + `go test -race ./internal/ui` pass.

## 2026-07-10 — Truecolor calendar colors (exact NextCloud hex) with 16-color fallback

- Owner request: move off the nearest-of-16 color mapping (which collapsed NextCloud's several blues/greens onto the same terminal color) to render the **exact** server hex. Chosen approach (via Q&A): **truecolor RGB with tcell auto-downsampling** (not a hand-built 256 table — tcell degrades RGB to 256/16 per terminal, incl. a bare TTY, in one code path), plus a **readability floor** and a **`color_mode` config knob**.
- **Model** (`internal/model/color.go`): added `ParseHexColor` (exported hex→RGB), `Luminance` (Rec. 601), and `ReadableFg` — lifts a dark color toward white until it clears a luminance floor (`minReadableLum = 96`). Kept `NearestANSI16`/`ANSI16IsDark` for the 16-color mode. The floor exists because item colors are drawn as **foreground text on the terminal's unknown default background**; a dark navy would otherwise be invisible on a dark terminal (assumes dark bg — the `16` mode is the light-terminal escape hatch).
- **UI** (`internal/ui/color.go`): `calColor` now carries both `fill` (exact color, for event-block backgrounds, which supply their own contrasting text) and `fg` (readability-lifted, for bullets/day-cell lines/agenda titles). New `colorMode` enum (`auto`/`16`/`off`) + `parseColorMode`; `resolveCalColor(hex, mode)` builds truecolor RGB in `auto`, nearest-ANSI named color in `16`, nothing in `off`. Only `drawBlock` uses `fill`; every other site already used `fg`. `dark` now reflects the exact fill's luminance.
- **Config** (`internal/config`): `[appearance] color_mode` (default `auto`; `truecolor`/`16`/`off`), added to `Default()` and the first-run template. Wired through `ui.Options.ColorMode`. `main.go` force-enables tcell truecolor (`COLORTERM=truecolor`) when `color_mode = "truecolor"`, for terminals that underreport; the UI renders RGB either way.
- Docs: `main.md` (Colors design note rewritten + calendar-metadata decision + config scope), `CLAUDE.md` UI line, `README.md` (calendars bullet + an `[appearance]`/`color_mode` note), config template. This reverses the earlier "terminal 16-color palette only" decision — recorded as such.
- Tests: model — `ParseHexColor`, `ReadableFg` (bright unchanged, dark lifted to the floor, white safe), `Luminance` via existing; ui — `resolveCalColor` per mode (exact truecolor + fill/fg split, dark color's fg lifted while fill stays exact, `16` named, `off`/empty don't resolve), `parseColorMode` table, and the render tests migrated to bright colors + `.Hex()` comparisons (SimulationScreen preserves the RGB value); config — default `color_mode == "auto"`. Full gate (`build`/`vet`/`staticcheck`) + `go test -race ./internal/ui ./internal/model ./internal/config` pass.

## 2026-07-10 — Week/day time-grid: `+`/`-` zoom the hour-row height (remembered)

- Owner follow-up to the uniform-hour change: let the hour-row height be adjusted, with `+`/`-`, and remember it between sessions. The `vScale` rework already renders any rows-per-hour with correct scrolling, so this is purely a control surface + persistence.
- **Zoom override** (`internal/ui/timegridview.go`): `timeGridView` gained `rowsPerHour` (explicit zoom, 0 = auto-fit) and `lastRowsPerHour` (the value actually drawn). `newVScale` uses the override when set, else the auto-fit `bodyH/24`, and records `lastRowsPerHour`. Bounds `minRowsPerHour=1`/`maxRowsPerHour=12` + `clampRowsPerHour`.
- **Contextual `+`/`-`** (`internal/ui/app.go`, `keys.go`): `+`/`-` were the accordion. Now, when the week/day time-grid is the active view (`timeGridActive`), they call `zoomHour(±1)`; in every other view (month/tasks/agenda, where hour-zoom is meaningless) they keep driving the accordion. `zoomHour` steps from the height currently in effect (explicit zoom, else the last auto-fit drawn), clamps, mirrors onto the grid, flashes "Hour rows: N", and persists.
- **Persistence** (`internal/state`, `cmd/lazyplanner/main.go`, `ui.Options`): `state.State` gained `RowsPerHour`; the `SaveState` callback signature is now `func(leftWidth int, hidden []string, rowsPerHour int)` and `ui.Options` carries `RowsPerHour`. `app.hourRows` seeds the grid at build and is written by `persistState` alongside pane width + hidden calendars. Taller hours simply scroll more of the day off-screen (the scroll machinery already handles it).
- Docs: help overlay, `main.md` (keymap `+`/`-` row, Week/Day view, pane-sizing note), `CLAUDE.md` UI line, `README.md`.
- Tests: `state` round-trip covers `RowsPerHour`; `sizing_test.go` `TestZoomHourAdjustsClampsAndPersists` (auto→2, clamps to max/min, persists); `keys_test.go` `TestPlusMinusContextual` (`+` zooms in week view / accordions in month view); `timegridview_test.go` `TestTimeGridRowsPerHourOverride` (explicit 3 → uniform 3-row spacing, `lastRowsPerHour` recorded). Migrated the four `saveState` stubs to the new signature. Full gate (`build`/`vet`/`staticcheck`) + `go test -race ./internal/ui ./internal/state` pass.

## 2026-07-10 — Week/day time-grid: uniform hour heights (even axis) + scroll when short

- Owner report: vertical hour spacing in the week/day view was uneven — the mapping `row = bodyY + hour*bodyH/24` error-diffuses the remainder, so adjacent hours differed by a row (e.g. a repeating 1-2-2 gap pattern) whenever the pane height wasn't a multiple of 24. Owner chose **uniform hour heights**, and confirmed **scrolling is fine** when the whole day can't fit.
- **Fix** (`internal/ui/timegridview.go`): replaced the fill-the-pane float scaling with a uniform `rowsPerHour` grid. New `vScale{bodyY, bodyH, rowsPerHour, scroll}` + `vScale.row(hourFloat)` maps hours to rows; `newVScale` picks `rph = max(1, bodyH/24)` — the largest whole rows-per-hour that fits — so every hour is exactly `rph` rows tall (evenly spaced), leaving a small blank margin below the last hour when `bodyH` isn't a whole number of hours. When even one row per hour overflows the pane (`24*rph > bodyH`, i.e. a very short body), the grid **scrolls**: `newVScale` centers `anchorHour()` — the drilled timed item's time, else the current time when a shown day is today, else mid-morning (`defaultAnchorHour = 8`) — clamped to the ends. `drawBlock`/`drawTaskMarker` now take the `vScale`, map through it, and clip to the visible pane (a block partly scrolled out is clipped; a marker fully out is skipped). Column separators stop at the grid's bottom so the blank margin stays clean. Navigation is unaffected (it's logical lane/time-based via `model.LayoutDay`); scroll is recomputed each draw from the selection, so a drilled item stays in view automatically.
- Docs: `main.md` Week/Day view + `CLAUDE.md` UI line (uniform hour heights, blank margin, short-pane scroll) — replaced the old "scaled to fill the pane height (no scrolling)" wording. README's time-grid description is high-level and unaffected.
- Tests (`timegridview_test.go`): `hourLabelRows` helper (exact gutter match, so "1am" ≠ "11am"); `TestTimeGridUniformHourSpacing` renders a body where the old mapping gave mixed gaps and asserts a constant 2 rows/hour across all 24 labels; `TestTimeGridScrollsShortPaneToDrilledItem` — a 9pm event is off-screen on a short pane and scrolls into view when drilled. Verified visually at heights 40 (rph 1) and 60 (rph 2): even axis, proportional blocks, clean bottom margin. Full gate (`build`/`vet`/`staticcheck`) + `go test -race ./internal/ui` pass.

## 2026-07-10 — Month view: "+N more" indicator at the top too (items hidden above)

- Owner request: the month grid already drew a `+N more` line at the **bottom** of an overflowing day cell (items below the scrolled window, correctly shrinking as you drill down); add the mirror-image `+N more` at the **top** once the window has scrolled down far enough to hide items above.
- **Fix** (`internal/ui/calendarview.go` `drawCell`): reworked the overflow render. A `drawItem`/`drawMore` closure pair removes the duplicated item-draw logic. When a day overflows and the cell has room for both markers (`avail >= 3`), the scroll window is chosen by regime — at the top of the list only a bottom marker shows, at the bottom only a top marker, in the middle both (selection pinned to the last item row, matching the prior single-indicator feel). The top marker counts items above the window (`start`), the bottom counts items below (`n - end`); each shrinks and disappears as you drill toward that edge, and the drilled item is always fully visible (never hidden under a marker). Cells too short for two markers (`avail < 3`) keep the original single bottom-indicator scroll behavior.
- Docs: `main.md` Month-view description updated (top + bottom `+N more`). README/CLAUDE.md don't mention the marker, so unchanged.
- Tests (`calendarview_test.go`): added `rowStrings`/`firstRowContaining` helpers; reworked `TestMonthGridOverflowIndicatorReflectsBelow` to assert the *below*-window marker specifically (sits below the first item at the top, gone once the last item is reached) and added `TestMonthGridTopOverflowIndicator` (drilled to the bottom, a `+N more` marker appears above the first visible item and Task0 is scrolled off the top). Existing scroll-to-drilled-item test unchanged. Full gate (`build`/`vet`/`staticcheck`) + `go test -race ./internal/ui` pass.

## 2026-07-09 — Calendar navigation overhaul: spatial 2D drill + f/b stays drilled

- Owner revised calendar-pane navigation. Rules now:
  - **Un-drilled week/day**: `h`/`l`/`←`/`→` move between days; `j`/`k`/`↑`/`↓` do **nothing** (days are horizontal). `Enter` drills in. Month un-drilled unchanged (2D day cursor, `↑`/`↓` = ±week).
  - **Drilled week/day**: fully **2D spatial** navigation of the day's on-screen layout — `j`/`k` move by time; `h`/`l` move between **concurrent** events (the side-by-side overlap lanes). Example: A (11–12, full width) above B/C (12–1, concurrent) → `↓` from A lands on B, `→`/`←` toggles B/C, `↑` returns to A. Deterministic (was a flat time-ordered list, so concurrent order was a non-deterministic title tiebreak). The all-day band is the top row (`h`/`l` between its items; `↓` enters the timed grid, `↑` from the top timed row returns to it); timed due-task markers are single-lane rows in the vertical flow. Movement stops at the day's edges.
  - **Drilled month**: 1D (the day's item list) — `j`/`k` cycle, `h`/`l` do nothing.
  - **`f`/`b`**: the way to change day/week/month; now **stays drilled**, re-entering on the new period's day (first item), or dropping to day nav if it has none.
- **Impl** (`timegridview.go`): new `navCell`/`navCells` spatial model built from `model.LayoutDay` lanes + start times; `spatialTarget`/`spatialMove` for up/down (by time level, nearest lane) and left/right (adjacent lane among overlapping events, or between band items). `handleEventMode` now calls these; `handleDayMode` no longer drills on `↑`/`↓`. `calendarview.go`: month drill drops `←`/`→` (stays 1D). `app.go` `shiftAnchor`: capture drill state, re-drill after the period change.
- Tests (`timegridview_test.go`, `taskcalendar_test.go`): the A/B/C spatial example (`↓`/`↑`/`←`/`→` + edge stop), un-drilled `↓` does nothing / `Enter` drills, all-day-band→timed drill order, and `f` stays drilled onto the next day's task. Full gate + `-race` pass.
- Memory: recorded the spatial-drill design + rules ([[calendar-drill-navigation]]).

## 2026-07-09 — Month overflow "+N more" now counts items below the window

- Follow-up to the drill-scroll fix: `+N more` counted every item *outside* the window (including ones scrolled off the top), so it lingered even after drilling to the bottommost item. Owner wanted it to update/disappear as you drill down.
- **Fix** (`calendarview.go` `drawCell`): the indicator now counts only items **below** the window (`n - end`) and is drawn only when that's > 0. So it shrinks as you drill down and vanishes once the last item is selected; items scrolled off the top are still reachable by drilling back up. (The reserved row is simply left blank at the bottom.)
- Tests (`calendarview_test.go`): with an overflowing day, `+N more` shows at the top and is gone when the bottom item is drilled. Full gate + `-race` pass.

## 2026-07-09 — Fix: month view lost the highlight when drilling into overflow items

- Owner report: on the month view, a day with more items than fit its cell drew the first few + a `+N more` line from the top; drilling (event-cycling) to an item in the overflow region left it **undrawn**, so the highlight vanished and you couldn't tell where the cursor was.
- **Fix** (`calendarview.go` `drawCell`): the day cell now **scrolls its visible window to the drilled item**. When the day is selected and in event mode, the window `[start, start+capItems)` is positioned so `eventIndex` is always inside it (`capItems` = rows minus one reserved for the indicator). Non-drilled days still render from the top as before. The `+N more` count now reports every item outside the window (above when scrolled, below otherwise) — and since the cursor is always inside the window, it's never hidden. Week/day was unaffected (blocks/markers draw at their time position, always visible).
- Tests (`calendarview_test.go`): a day with 8 items drilled to index 6 renders `Task6` on screen with the reverse (highlight) attribute — would fail before the fix (only the first ~3 drew). Full gate + `-race` pass.

## 2026-07-09 — Folder caret (▸) consistent across all views; folders keep due dates

- Owner design question: should folders render due dates (intuition: no), and doesn't hiding them make a dated task vanish from the calendar the moment it gains a subtask? Resolution (agreed): **folders keep their due dates** — a folder is still a real dated task (a deadline'd project with sub-steps), hiding it would lose user-set data and cause exactly that vanish. The rule stays uniform: a task shows on the calendar iff it has a due date, folder or not. Consistency instead comes from rendering the **folder metaphor** everywhere.
- **Global folder set** (`render.go`): replaced the per-list `treeFolders` with one global `a.folders` (`rebuildFolders` = `folderSet(store.Todos())`, run in `buildCalendars` and `buildTreeForList`), plus `isFolder(uid)`. Folder-ness of a task is the same in every view now (children share the parent's collection, so global == per-list for any UID).
- **Folder caret in calendar + agenda** (owner asked for the same caret as the Tasks pane): new shared `todoMark(t, folder)` → `▸` folder / `[■]` done / `[ ]` incomplete. Wired into the month-grid cells (`itemLabel`), week/day markers + all-day band (`taskMarkerLabel`), and the agenda list (`agendaLeftLabel`, now a method); the month/time-grid get an `isFolder` closure. The tree keeps its expand-aware `▸`/`▾`. So a dated task that gains a child now shows `▸ Proj` on the calendar (stays put, doesn't vanish) instead of `[ ]`.
- The completion gate was already view-independent (`toggleComplete` → `hasIncompleteChildren`), so Space on a folder in any view still refuses until its children are done.
- Docs: `README.md`, `main.md`, `CLAUDE.md` (folders keep due dates; ▸ caret across views).
- Tests (`taskcalendar_test.go`): a dated task with an incomplete child is a folder, still appears in the day's items, and renders `▸` in the month cell, agenda label, and week/day marker. Migrated `app_test.go` off `treeFolders`. Full gate + `-race` pass.

## 2026-07-09 — Fix: completing a drilled task no longer undrills the calendar day

- Owner report: in a calendar view, Space-to-complete kicked you back out to day navigation. Cause: `toggleComplete` ends in `refresh`, which rebuilds the grid (`setData` resets `eventMode`/`eventIndex`), dropping the drill. The modal create/edit/delete paths already re-drill via `captureFocus`/`restoreFocus`, but Space mutates directly with no modal.
- **Fix** (`edit.go`): new `refreshKeepingDrill` captures the grid's `drillState` before the rebuild and `reDrill`s the same day/index after (calendar mode only; a plain `refresh` elsewhere). `toggleComplete` uses it. The completed task stays in the day's items (calendar views don't hide completed), so the same index re-selects it, now shown `[■]`.
- Tests (`taskcalendar_test.go`): after Space-complete, the month grid stays in `eventMode` with `currentTarget` still the task, and the week grid keeps its task selection. Full gate + `-race` pass.

## 2026-07-09 — Manage tasks from the calendar views (check off · subtasks anywhere)

- Owner revised the "tasks are managed only from the Tasks pane" decision (it conflicted with the max-power philosophy). Three changes:
- **Check off a drilled task with Space in any calendar view** (`app.go`): in Calendar mode Space is now contextual — if a task is drilled/selected it toggles done, otherwise it toggles the highlighted calendar's visibility (unchanged when no task is selected or on an event). Agenda already checked off the selected task; the month grid already exposed the drilled item via `currentTarget`, so it just needed the Space routing.
- **Week/day tasks are now selectable** (`timegridview.go`): the time-grid drill was `Occurrence`-based (events only), so tasks weren't reachable there. Reworked it onto `model.AgendaItem` — the drill now cycles the day's events **and** due tasks (agenda order), matching the month grid. Added a per-day `items` drill list (`dayItemsForDays`, from `dayItems`), `selectedItem()` (replacing `selectedOcc`), task-marker highlight when selected, and all-day-band highlight for a selected all-day item (event or task). `currentTarget` now resolves the time-grid's drilled item too, so **edit/delete/complete all work on tasks in week/day** as well. This also makes the two grids structurally alike (helps the selection/focus glue noted earlier).
- **Subtasks created under the selected task in any context** (`edit.go`): dropped the Tasks-mode gate; `subtaskContext` now takes the parent from `currentTarget` (tree, calendar drill, or agenda) and creates the subtask in the **parent's own calendar** (via `store.Locate`) — never the Tasks-overview highlight — since RELATED-TO parent/child must share a collection.
- Not changed (already correct): top-level task creation targets the selected task list and event creation the selected calendar, from any pane (independent overview selectors); a "both" calendar already appears in both overview panes.
- Docs: `README.md`, `main.md`, `CLAUDE.md` (Space contextual, week/day task drill, subtask-anywhere). Memory: updated [[create-targets-independent-overviews]] and [[creation-gated-by-component-set]].
- Tests: `timegridview_test.go` migrated to the AgendaItem drill; `taskcalendar_test.go` — Space checks off a drilled task in the month grid and the week grid, and `subtaskContext` under a calendar-drilled task targets the parent's list. Full gate + `-race` pass.

## 2026-07-09 — Week/day grid: vertical motion cycles the day's events (counts work)

- Owner review of the count feature: `<count>` repeats motions and `<count>G` jumps to the Nth **list** item, but the week/day time-grid had **no vertical motion** (`j`/`k`/`↑`/`↓` were unbound — only `h`/`l` moved days), so counts were dead there and `j`/`k` felt broken.
- Decisions: keep `<count>G` = Nth item **lists-only** (tree/grids treat `G` as bottom; docs already say "nth list item"); add vertical motion to the week/day grid that **cycles the selected day's events**.
- **Fix** (`internal/ui/timegridview.go`): day-mode `↑`/`↓` now call `enterEventMode` — drill into the day's events (all-day first, then timed), selecting the first; once in event mode `handleEventMode` advances the cursor. Since `globalKeys` translates `hjkl`→arrows and `repeatKey` replays the arrow N times, a count like `2j` lands on the 2nd event for free (first press enters at index 0, the second advances). Horizontal `h`/`l` still move between days.
- Docs: `README.md`, `main.md`, `CLAUDE.md` (week/day vertical keys cycle events; counts work).
- Tests (`timegridview_test.go`): `↓` drills into events (first), a second `↓` advances, `↑` goes back. Full gate + `-race` pass.

## 2026-07-09 — Fix: `gg`/`G` in the task tree (+ stale tree on programmatic list select)

- Owner report: still couldn't use `gg`/`G` in the Tasks pane. Two distinct causes found:
- **Tree `gg`/`G` were scroll-only** (`internal/ui/keys.go`): `gotoTop`/`gotoBottom` feed `Home`/`End`, which tview's `List` honors — but its `TreeView.process()` has no `treeHome`/`treeEnd` case, so `Home`/`End` (and tview's native `g`/`G`) only adjust the scroll offset and never move the selection. So even after diving into the tree (`Enter`), `gg`/`G` did nothing visible. Fix: when the focused widget is the tree, select the first / last visible node directly via `SetCurrentNode` (new `visibleTreeNodes` walks selectable nodes in display order, descending only into expanded folders). Lists and the calendar grids are unchanged.
- **Stale tree on programmatic list selection** (`app.go` tasklists changed-callback, `render.go`): selecting a task list with `{`/`}` (or any `SetCurrentItem`) rebuilt the *previously* selected list's tree — tview fires `List.changed` **before** updating `GetCurrentItem`, and `buildTree()` read the stale current item. Split out `buildTreeForList(id)` and have the changed-callback build for the callback's `index` argument (always the new selection). This also means `{`/`}` now actually switch the visible tree, not just the highlight.
- Note (focus model, not a bug): `t` focuses the task-**list** column; `Enter` dives into the **tree**, where `gg`/`G`/`j`/`k` navigate tasks; `Esc` backs out.
- Tests (`keys_test.go`): `gg`/`G` move the tree cursor to first/last node; cycling to a list with `}` shows *that* list's tree (root name); plus the existing month-grid and bracket/brace tests. Full gate + `-race` pass.

## 2026-07-09 — Fix: `gg`/`G` were dead in the calendar grid

- Owner report: `gg`/`G` weren't properly implemented. Root cause: `gotoTop`/`gotoBottom` feed `Home`/`End` to the focused primitive, which tview's `List` and `TreeView` handle — so the overview lists and task tree already worked — but the **custom calendar widgets** (`calendarView` month grid, `timeGridView` week/day) had no `Home`/`End` handling, so once you `Enter`-dived into a grid, `gg`/`G` did nothing (confirmed via a headless probe: the month selection didn't move).
- **Fix**: `calendarView` and `timeGridView` now handle `Home`/`End` in both day-mode and event-mode. Day-mode: `gg` selects the first grid cell / first day-column, `G` the last (reusing the existing `onSelectDay`). Event-mode (drilled into a day): `gg`/`G` jump to the first / last event of that day. No change to `keys.go` — the app's `gotoTop`/`gotoBottom` already feed `Home`/`End`, so making the widgets honor them is all it took (consistent with the "reuse the widget's own navigation" design).
- Docs: `README.md` / `main.md` `gg`/`G` rows note they cover the calendar grid too.
- Tests: `keys_test.go` drives `gg`/`G` through `globalKeys` with the month grid focused (lands on first/last grid day); `calendarview_test.go` event-mode `Home`/`End` → first/last event; `timegridview_test.go` week `Home`/`End` → first/last day. Full gate + `-race` pass.

## 2026-07-09 — Global overview selectors: `[`/`]` calendar, `{`/`}` task list

- Owner request: make the bracket keys global calendar selectors and add curly braces as global task-list selectors, so either overview's highlight can be nudged from any pane (matching the existing independent-overview-targeting design, cf. create-event/create-task).
- **Keys** (`internal/ui/app.go` `globalKeys`): dropped the `a.mode == modeCalendar` guard on `[`/`]` so they cycle the Calendars highlight in every mode; added `{`/`}` → new `cycleTasklist`, the task-list counterpart to `cycleCalendar`. Both intercept at the app level (before the focused widget), so they work whether focus sits in an overview list or a dived-in grid; neither the tree nor the lists bind these keys, so nothing regresses.
- **`cycleTasklist`** cycles within `len(a.tasklistIDs)` (skipping the "(no task lists)" placeholder) and `SetCurrentItem`, which fires the tasklists changed-callback — so when Tasks mode is showing, the tree rebuilds for the newly-highlighted list; in other modes it just moves the (always-visible) left-column highlight, which is the target for the next action.
- Docs: help overlay (moved `[`/`]` into Panels & navigation and added `{`/`}`), controls line, `README.md`, `main.md`, `CLAUDE.md`.
- Tests (`keys_test.go` `TestBracketAndBraceCycleGlobally`): from Agenda mode (neither Calendar nor Tasks), `]`/`[` cycle the calendar highlight and `}`/`{` cycle the task-list highlight, wrapping back; creates a second VTODO list so the cycle has somewhere to go. Full gate + `-race` pass.

## 2026-07-09 — Unify the due-task checkbox across calendar views

- Owner report: inconsistent glyphs — the month view showed an uncompleted due task as `[ ]`, but the new week/day time-grid marked it with `◆` (and `◆ [■]` when completed). Task management lives in the Tasks pane, so the calendar views should just render tasks uniformly.
- **Fix** (`internal/ui/timegridview.go` `taskMarkerLabel`): the time-grid due-task line now uses the same checkbox convention as the month grid and task tree — `[ ]` uncompleted, `[■]` completed — dropping the `◆`. The line is still foreground-only text (not a filled block), which already sets a due task apart from an event.
- Docs: `README.md`, `main.md`, `CLAUDE.md` updated (checkbox, not `◆`).
- Tests (`timegridview_test.go`): asserts `[ ] Payrent` / `[ ] Renewpass` render (was `◆`); color assertion unchanged. Full gate + `-race` pass.

## 2026-07-09 — Show due tasks in the week/day time-grid (colored by list)

- Follow-up to the color work: due tasks were colored where they already showed (month grid, agenda) but the **week/day time-grid rendered events only** (`splitOccs` pulls event occurrences, never todos), so a task's due date was invisible in the hourly view. Owner chose to place a timed due task **at its due time**.
- **Time-grid** (`internal/ui/timegridview.go`): new `dueTasks map[string][]*model.Todo` + `taskColor` resolver. A **timed** due task draws a one-row `◆`-prefixed marker at its due hour (same hour→row mapping as event blocks), a foreground marker (no fill) so it reads as a due task distinct from the filled event blocks and can sit over an event at that time; an **all-day-due** task sits in the top all-day band alongside all-day events (lead item + `+N`). Both use the list's color (aqua fallback), and a completed one shows `◆ [■]`. `dueParts` splits a day's tasks into all-day vs timed. Markers are display-only — the `Enter` drill cycle still covers events (tasks aren't `Occurrence`s); the day's tasks remain in the Detail pane and month/agenda.
- **Wiring** (`render.go` `dueTasksByDay`, `app.go`, `color.go`): buckets tasks with a due date onto their due day for the visible range, excluding hidden calendars (via `TodosVisible`) and including completed ones (matching the month grid/agenda). `a.timegrid.taskColor = a.todoColor` (UID → list color).
- Docs: `README.md` (week/day now shows due tasks), `main.md` (Week/Day view), `CLAUDE.md` UI line.
- Tests (`timegridview_test.go`): a timed due task renders a `◆` marker at its due time in the list color (red) and an all-day-due task appears in the band. Full gate + `-race` pass. (Also confirmed en route — via a throwaway harness — that the month grid and agenda were already coloring due tasks from the previous change; no change needed there.)

## 2026-07-09 — Sync calendar colors from the server + render them everywhere

- Owner request: colors were **push-only** (in-app `:calendar color` → PROPPATCH; MKCALENDAR on create), never pulled, and were **never rendered** — events drew a fixed green, tasks aqua, and there was no palette mapping. Closed the gap both ways: pull the server's color, and draw every calendar's items in it. Fulfils the long-standing `main.md` intent ("server calendar colors are mapped to the nearest palette color").
- **Pull** (`internal/caldav/colors.go`, `client.go`): new `discoverColors` issues a Depth-1 PROPFIND for the Apple `calendar-color` under the calendar home set (over our own authenticated client, like the privilege/MKCALENDAR gap-fillers), best-effort so a failed/unsupported query never breaks discovery. `caldav.Calendar` gained a `Color` field, populated in `DiscoverCalendars`.
- **Store** (`internal/store/calendar.go`): `SyncCalendarColor(id, serverColor)` adopts the server color, **server-authoritative except when a local color edit is still pending** a PROPPATCH (that edit wins until pushed, so a routine pull can't clobber it); no-op on empty/unchanged (no needless sidecar rewrite), mirroring `SetCalendarComponents`/`SetCalendarReadOnly`.
- **Sync/import** (`internal/sync/{sync,import}.go`): `Sync` calls `SyncCalendarColor` per calendar during discovery (push-props already runs first, so a just-pushed color re-affirms rather than conflicts); `Import` records `cal.Color` in the initial `SetCalendarMeta`.
- **Mapping** (`internal/model/color.go`): pure `NearestANSI16(hex)` (nearest of the 16 ANSI colors by RGB distance; accepts `#rrggbb`/`#rrggbbaa`, alpha ignored) + `ANSI16IsDark(idx)` (Rec. 601 luminance) for fill contrast. Keeps LazyPlanner on the terminal palette (inherits the theme) — no truecolor.
- **Render** (`internal/ui`): new `color.go` maps a palette index → tcell named color + tview tag name + dark flag, and builds a calendar-id→color and item-UID→color index (`rebuildColorIndex`, run in `buildCalendars`). Applied in: the **Calendars list** (a `●` bullet in the calendar's color), the **month grid** day-cell lines, the **week/day time-grid** (event blocks filled with the color, contrasting black/white text; all-day band tinted), and the **agenda** title lines. Non-colored calendars keep the green/aqua defaults.
- **Bug found & fixed in passing**: the Calendars list's `[both]`/`[ro]`/`[events]`/`[tasks]` markers were **silently swallowed** by tview's style-tag parser (they live in the label string — why the string-contains tests passed — but never drew; only `[?]` survived, since `?` isn't a valid tag char). Now the description is `tview.Escape`d so the markers render literally, with the color bullet prepended as the one real tag.
- Tests: caldav — `discoverColors` (PROPFIND method/depth/body, set vs unset color); model — `NearestANSI16` table + `ANSI16IsDark`; sync — pulls a server color, and does **not** clobber a pending local recolor (pushes it instead); ui — `resolveCalColor`, the Calendars bullet renders red + `[both]` now renders literally, and the agenda title draws in the calendar color (SimulationScreen). Updated two marker tests to assert the escaped (now-visible) form. Full gate (build/vet/staticcheck) + `go test -race ./...` pass.

## 2026-07-08 — `i!` force override for creating on unknown-type calendars

- Owner request: a manual escape hatch from the block-until-known creation gate, for a calendar whose type isn't confirmed (`[?]`). Read-only stays hard-locked (no override), and a *known* wrong type is still refused.
- **Force chord** (`internal/ui/keys.go` `resolvePrefix`): after the `i` create prefix, `!` arms a one-shot force and keeps the prefix pending, so `i!e`/`i!t`/`i!s` (and the `i!E`/`i!T`/`i!S` full-form variants) force the create. New `pendingForce` (armed) and `forceCreate` (set for the duration of the action) app flags; the which-key hint shows `new (force)` and the command view echoes `i!e`. `startPrefix`/`clearPrefix` reset the flag.
- **Gate** (`calendar.go` `guardComponent`): honors `forceCreate` **only** in the unknown-type branch (empty component set → allow). A known wrong type still returns false regardless of force, and read-only is unaffected because `guardWrite` (checked first, ignores force) handles it. Blocked-flash now hints "(i! to force)".
- Docs: help overlay (`i ! e / i ! t` row), `main.md` (`i` keymap row + Creation section), `README.md`, `CLAUDE.md`. Memory: recorded the gating + force boundaries ([[creation-gated-by-component-set]]).
- Tests (`calendar_test.go`): `guardComponent` under `forceCreate` allows unknown-type but not a known wrong type; the `i` `!` `e` sequence arms force (prefix stays pending) and opens the event prompt on the fixture's unknown-type `work` calendar, while a plain `ie` there is refused. Full gate + `-race` pass.

## 2026-07-08 — hjkl move the highlight in every pane (incl. the overview lists)

- Follow-up: the keymap advertised `hjkl` as movement, but the three overview lists (Calendars/Tasks/Agenda) took **arrows only** — tview's `List` binds no `hjkl` (and `TreeView` binds `j`/`k` but not `h`/`l`). So `hjkl` movement was inconsistent across panes.
- **Fix** (`internal/ui/keys.go` `motionArrow`, `app.go` `globalKeys`): `hjkl` are now global **aliases for the arrow keys** — after the modal guard (so typing into forms is unaffected) and the count accumulator, a movement key is translated to its arrow and fed to the focused widget. `h`→Left, `j`→Down, `k`→Up, `l`→Right. This makes movement uniform in the lists, tree, and calendar grids without touching each widget, and a pending **count applies to `hjkl` too** (`3j` now works in a list). Arrow keys themselves are still only intercepted to apply a count (single presses fall through natively). Replaced the old `isMotion` helper.
- In a vertical list `j`/`k` move the selection; `h`/`l` map to the list's horizontal scroll (a no-op when content fits) — the meaningful "move the highlight" is `j`/`k`, matching vim. In the tree/grids all four move.
- Help overlay text corrected: `hjkl` "move the highlight (Enter expands/collapses a tree node)" — the previous "expand/collapse tree nodes" was wrong (Enter does that, not `hjkl`).
- Tests (`keys_test.go`): `j`/`k` move a stand-in overview list (which natively ignores them), and `3j` via the letter alias moves three rows. Full gate + `-race` pass. (Interactive pty verification was flaky in this environment again; the `globalKeys` dispatch is covered directly by the headless tests.)

## 2026-07-08 — Lock item creation to a calendar's type (events vs tasks)

- Owner request: sharpen the fuzzy calendar/task-list split — events only on event calendars, tasks/subtasks only on todo lists, either on a "both" calendar — using the component set now tracked per calendar.
- **Gate** (`internal/ui/calendar.go` `guardComponent`): event creation requires **VEVENT**, task/subtask requires **VTODO**; a wrong-type attempt is refused with an explanatory flash (e.g. *"Errands is a task list — can't add events"*). Wired into `eventCreateContext` (VEVENT) and `taskCreateContext`/`subtaskContext` (VTODO), alongside the existing read-only `guardWrite`. Owner's policy for an **unknown/unconfirmed** component set: **block until known** (declared set required) rather than guessing from contents — so it's refused with "unknown type — sync it first" until a sync settles it.
- **"Both" made explicit** (`componentsForType`): the in-app create form's "Both" now records `["VEVENT","VTODO"]` instead of an empty set, so a both-calendar's type is *known* immediately (empty = unknown under the block-until-known rule). MKCALENDAR still gets both components.
- **Type marker** (`render.go` `calTypeMarker`): the Calendars overview tags each row `[events]`/`[tasks]`/`[both]`, or `[?]` when the component set is unknown — making the gate self-explanatory (and why `[?]` blocks). Sits with the existing `[ro]`/`(hidden)` markers.
- Fixture: gave `personal`'s sidecar `"components": ["VEVENT","VTODO"]` (it holds a standup event + a grocery todo), matching what a real sync would record — so the chord create-task test isn't blocked by an unknown type.
- Tests: `guardComponent` table (event/task/both allowed correctly, unknown blocked both ways), `calTypeMarker` table, `componentsForType("Both")` = explicit set, and the Calendars panel renders `[both]` for personal / `[?]` for work. Full gate + `-race` pass.
- Note: interactive pty capture of this was flaky (tview + pty in this env); verified headlessly instead. Also observed (pre-existing, out of scope) that `j`/`k` don't move the overview *lists* — tview's List binds only arrows — while they do in the task tree.

## 2026-07-08 — Fixes: overview pane titles, empty task lists, visibility-toggle selection

- Three owner-reported bugs from exercising the step-10 finale keymap.
- **Pane titles still showed `1`/`2`/`3`** (`internal/ui/app.go` `build`): the overview boxes were decorated `1 Calendars`/`2 Tasks`/`3 Agenda` from before the remap. Now `c Calendars`/`t Tasks`/`a Agenda`, matching the actual focus keys.
- **Empty task lists were invisible** (`internal/ui/render.go` `buildTasklists`): the panel only listed calendars with `todos > 0`, so a freshly-created (or emptied) task list never appeared — you couldn't add tasks to it. New `supportsTodos` predicate: list a calendar when its supported component set includes **VTODO** (so an empty list shows), falling back to "has todos" when the component set is unknown. To make imported empty lists recognizable, sync now records the server's `supported-calendar-component-set` (already surfaced by go-webdav) via new `store.SetCalendarComponents` (no-op when unchanged), called per calendar in `Sync`.
- **Hiding a calendar jumped the selection to the top** (`internal/ui/keys.go` `afterVisibilityChange`): rebuilding the Calendars list (`Clear`/`AddItem`) parks the cursor at index 0. Now the current row is captured and restored around the rebuild — since hiding marks a calendar rather than removing it, its index is stable, so the cursor stays on the calendar you just toggled.
- Docs: `main.md` task-list description updated (task lists = VTODO-supporting calendars incl. empty).
- Tests: ui — pane titles are `c/t/a`, `supportsTodos` table, an empty in-app VTODO list appears in the Tasks panel, hiding keeps the calendar selection (`bugfix_test.go`); sync — an imported empty VTODO calendar records its component set. Full gate + `-race` pass. Pty: launch renders `c Calendars`/`t Tasks`/`a Agenda`, exit 0, no panic.

## 2026-07-08 — Build step 10 finale (part 12): `:calendar rename|color|hide|show` — step 10 finale complete

- Final finale increment: edit a calendar's server-owned metadata in-app. **Step 10 finale complete** (all seven owner-requested keybinds/commands landed).
- **CalDAV PROPPATCH** (`internal/caldav/proppatch.go`): `SetCalendarProps(path, displayName, color)` issues an RFC 4918 PROPPATCH (DAV:displayname + Apple calendar-color) over the authenticated HTTP client — the same gap-filling approach as MKCALENDAR (go-webdav's client doesn't expose it). Empty values are skipped; success = 207 (or 200).
- **Store** (`internal/store/calendar.go`, sidecar): `UpdateCalendarMeta(id, displayName, color)` edits the local name/color and flags `pending_props` for a server push (offline-first); a still-pending-create calendar just carries the new values into its MKCALENDAR. `PendingCalendarProps` (only calendars already on the server) + `MarkCalendarPropsSynced` drive/clear the push. New `pending_props` sidecar field.
- **Sync** (`internal/sync/sync.go`): `pushCalendarProps` runs before discovery (so a routine pull doesn't race the change) — PROPPATCH each pending calendar, then clear the flag. `Syncer` gained `SetCalendarProps`; `SyncResult.CalendarsUpdated` counts them.
- **UI** (`internal/ui/command.go`): `:calendar rename <name>` / `:calendar color <#rrggbb>` act on the highlighted calendar (guarded read-only), update it locally, and rebuild the lists; `:calendar hide`/`show` mirror the `Space` visibility toggle (shared `afterVisibilityChange`). `normalizeColor` validates the hex. `currentCalendarID` resolves the active panel's calendar.
- Docs: help overlay, `README.md` (command list + PROPPATCH/offline-first note), `CLAUDE.md`. (`main.md`'s `:` section already listed `:calendar`.)
- Tests: caldav — PROPPATCH method/body/error/no-op (`proppatch_test.go`); sync — local rename+recolor pushes one PROPPATCH with the right name/color and doesn't re-push once synced (fake server gained `SetCalendarProps`); ui — `normalizeColor` table, `:calendar rename` updates the local name, `:calendar hide`/`show` toggle visibility. Full gate + `-race` pass. Pty: `:calendar rename Home`, `:calendar color #ff8800`, `:calendar hide` → exit 0, no panic; the sidecar records the new name, color, and `pending_props:true`.

## 2026-07-08 — Build step 10 finale (part 11): `:config` (edit in $EDITOR, reload on exit)

- Sixth finale increment; delivers the settled `:config` convenience (open in `$EDITOR`, reload on exit) deferred out of step 10.
- **UI** (`internal/ui/command.go`): `:config` calls `a.tv.Suspend` (releases the terminal for the editor), runs the `EditConfig` callback, then `applyConfigReload` swaps in a fresh sync closure and flashes — all inside the suspend so it's applied before the redraw on resume. `applyConfigReload` is split out so it's unit-testable without a running app. A nil callback flashes "unavailable".
- **Wiring** keeps the architecture rule intact — the UI runs no editor and parses no config itself. `ui.Options.EditConfig` is provided by `main`: `editConfigFn` (`cmd/lazyplanner/main.go`) launches `$EDITOR` (default `vi`) on the config path, re-reads via `config.Load`, and returns a rebuilt sync closure. It **refuses an account change** (`config.AccountID` differs) with a "restart to switch caches" error, since the vdir cache is account-keyed — a mid-session account swap would point sync at the wrong cache. Added `config.ConfigPath()`.
- Docs: help overlay (`:config` row), `README.md` command list + a note on the account-change caveat, `CLAUDE.md` command list. (`main.md`'s `:` commands section already listed `:config`.)
- Tests (`configreload_test.go`): `:config` with no callback flashes "unavailable"; `applyConfigReload` swaps a non-nil sync closure and flashes "reloaded"; a reload error is surfaced and the sync closure left untouched. Full gate + `-race` pass. Pty (`EDITOR=true`): `:config⏎` suspends, runs the editor, reloads (status shows "reloaded"), exit 0, no panic.

## 2026-07-08 — Build step 10 finale (part 10): yank/paste to move tasks (`y`/`p`)

- Fifth finale increment: move a task (and its subtree) between parents and lists — the reorganize flow that `H`/`L` (one level, in-list) couldn't do.
- **Yank/paste** (`internal/ui/yankpaste.go`): `y` records the selected task (`a.yankUID`); `p` moves it under the current tree selection (or the list's top level when the root is selected). Target list = the selected task list; target parent = the highlighted task. Guards against pasting a task onto itself or into its own subtree (cycle).
  - **Same list** → `reparentTo`: just `SetTodoParent` (RELATED-TO); children follow because their links are UID-based (unchanged).
  - **Different list** → `moveSubtree`: recreate the root + every descendant in the target calendar (`Put` under `ResourceName(uid)`) and delete each from the source, as **one compound undo step** (per resource: delete-the-copy + restore-the-original). The moved root adopts the paste target as its parent; descendants keep their UID links, so the subtree stays intact across the move. Read-only source or target is refused via `guardWrite`.
- Keys `y`/`p` added to `globalKeys` (both freed earlier — `y` was unused, `p` was the retired prev-period). The clipboard clears after a successful move.
- Docs: help overlay, `README.md` (edit prose + key table), `CLAUDE.md` UI line (`main.md` already listed `y`/`p` from the keymap rewrite).
- Tests (`yankpaste_test.go`): same-list paste re-parents Mover under Parent and `u` restores top-level; cross-list `moveSubtree` moves a Mover+Child subtree to another calendar (links intact, root becomes top-level, clipboard cleared) and `u` restores both to the source. Full gate + `-race` pass. Pty: `t y j p u q`, exit 0, no panic.

## 2026-07-08 — Build step 10 finale (part 9): quick field-set (`sp` priority, `sd` due)

- Fourth finale increment: change one field of the selected task without the full edit form.
- **`s` ("set") chord** (`internal/ui/quickfield.go`): `sp` sets priority, `sd` sets/clears the due date — each a one-line `promptInput`. Tasks view only (events have no priority/due); the `s` prefix flashes elsewhere.
- **Field application** honors the property-preservation iron rule: `draftFromTodo` clones the task's current fields into a `TodoDraft`, a mutator changes just the one field, and `EditTodo` re-encodes (so unknown iCal props, VALARMs, RELATED-TO, etc. survive). `applyTodoField` relocates the task fresh, guards read-only calendars, writes, pushes an **undo** step, and refreshes.
- **Parsing** reuses the quick-add rules: `parseSetPriority` accepts `1`-`9` / `high`/`med`/`low` (leading `!` tolerated; blank/`0`/`none` clears); the due prompt runs `ParseQuickAdd` (`fri`, `jul 20`, `3pm`, …; blank clears). Consistent with `it`/`is` quick-add.
- Docs: help overlay, `main.md` keymap (`s` row), `README.md` (edit prose + key table), `CLAUDE.md` UI line.
- Tests (`quickfield_test.go`): `parseSetPriority` table (digits, aliases, clear tokens, out-of-range/garbage rejected); `applyTodoField` sets priority then `u` restores 0; due set (date round-trips) then cleared. Full gate + `-race` pass. Pty: `t sp 3⏎ sd fri⏎ q`, exit 0, no panic.

## 2026-07-08 — Build step 10 finale (part 8): calendar visibility toggle (remembered)

- Third finale increment; closes the visibility toggle promised for the Calendars panel in step 10 but never built.
- **Store** (`internal/store/store.go`): added `EventOccurrencesVisible(from, to, hidden)` and `TodosVisible(hidden)` — the same queries filtered by a set of hidden calendar ids (keyed by id, which the store already has but the old flatten-all queries discarded). `EventOccurrences`/`Todos` now delegate with a nil set, so existing callers are unchanged.
- **State** (`internal/state`): `State` gained `HiddenCalendars []string` (`hidden_calendars`). The `SaveState` callback signature changed to `func(leftWidth int, hidden []string)` and now rewrites the whole state file, so pane width and hidden calendars persist together; `ui.Options` gained `Hidden`. `main.go` loads/saves both.
- **UI**: `a.hidden` (map, seeded from `Options.Hidden`); `persistState` gathers both prefs (sorted for stable output) and calls the save callback — `resizeLeft` now routes through it too. **`Space` in Calendar mode** toggles the highlighted calendar via `toggleCalendarVisibility` (rebuilds the calendar+agenda and persists); in other modes `Space` still toggles task done. The month grid, time-grid, and agenda queries pass `a.hidden`, so a hidden calendar's events **and** due tasks drop out. The Calendars list shows a `(hidden)` marker.
- Docs: help overlay, controls line (adds `Space done/hide` + `/ find`), `main.md` (Space keymap row + Calendars description), `README.md`, and the `CLAUDE.md` UI line updated; the stale "visibility toggles land in step 10" note replaced.
- Tests: `state` round-trip covers `HiddenCalendars`; ui — `toggleCalendarVisibility` hides/shows, persists the id set, and renders the `(hidden)` marker; hiding every calendar yields zero occurrences from `EventOccurrencesVisible`; the sizing test's save stub updated to the new signature. Full gate + `-race` pass. Pty: Calendar mode → `Space` hides the highlighted calendar → exit 0; `state.json` records `hidden_calendars:["personal"]`.

## 2026-07-08 — Build step 10 finale (part 7): incremental search (`/` `n`/`N`)

- Second finale increment: search across the current view.
- **Search** (`internal/ui/search.go`): `/` opens a top-line input; the selection follows the first match **as you type** (incremental — `SetChangedFunc` runs the search on each keystroke, changing only the selection so the input keeps focus). Enter keeps the match (focus lands on the view); Esc cancels and restores the pre-search selection. `n`/`N` cycle matches afterward (matches recomputed each press, so a cycle survives edits). Case-insensitive substring match.
- **Per-mode targets**: Tasks → the task tree (walks every `*model.Todo` node in display order and **expands ancestor folders** to reveal a match inside a collapsed subtree); Agenda → the agenda list; Calendar → the calendars list (search by name). `searchWidget`/`searchItems` centralize the per-mode collection + selection.
- **`:search <text>`** wired into the command dispatcher (also `:find`), matching the `main.md` command list; echoes to the command view.
- Keys: `/` opens search, `n`/`N` next/prev (added to `globalKeys`; `n`/`N` were freed by moving period-nav to `f`/`b`). Help overlay + `:` command hint updated.
- Tests (`search_test.go`): tasks search jumps to the first match and `n` cycles with wrap-around; no-match flashes; `n` with no active query flashes; calendar-name search selects the matching calendar. Full gate + `-race` pass. Pty: drive `t / meet⏎ n N a /g⎋ q`, exit 0, no panic.

## 2026-07-08 — Build step 10 finale (part 6): keymap overhaul + counts / gg-G + fold-all

- Start of a "step 10 finale" (owner-requested extra keybinds, treated as the last UI-polish step before step 11). First increment: a keymap remap that frees the number row, plus vim counts, `gg`/`G`, and tree fold-all.
- **Keymap remap** (owner's mnemonic scheme): panel focus moved off `1`/`2`/`3` to **`c`/`t`/`a`** (Calendars/Tasks/Agenda); the create prefix moved off `a` to **`i`** ("insert" — `it`/`iT`/`ie`/`iE`/`is`/`iS`/`ic`/`il`, Shift = full form), freeing `a` for Agenda and keeping `n`/`N` for search; calendar period nav moved off `n`/`p` to **`f`/`b`** (forward/back). Freeing the digits is what makes counts possible.
- **Vim counts** (`internal/ui/app.go` `globalKeys`): `1`-`9` start a count and `0` extends one (`a.pendingCount`, capped at 999, shown in the status-bar left section); the next motion (`hjkl`/arrows) repeats via `repeatKey`, which feeds the event to the focused primitive N times — reusing tview's own List/TreeView navigation so counted movement matches a single keypress. A non-motion key drops the count.
- **`gg` / `G`** (`internal/ui/keys.go`): `g` is now a which-key prefix — `gg` top, `gt` today, `gd` go-to-date (the old standalone `g`=goto). `gg`/`G` feed `Home`/`End` to the focused list/tree (both handled natively by tview); `<count>G` jumps to the nth item of a list. `G` bottom is a standalone key.
- **Fold-all** (`z` prefix, Tasks view only): `zR` expand-all, `zM` collapse-all, `za` toggle the current node — walks the tree nodes, sets expansion, and keeps each folder's `▸`/`▾` marker in sync.
- **which-key**: the popup footer now varies by prefix (the "Shift = full form" note only shows for the `i` create prefix); `prefixLabel` gains `i`/`g`/`z`.
- Docs: help overlay (`help.go`), controls line (`render.go`), `main.md` keybinding table (rewritten from the stale "draft/future step 10" framing to the real vim-flavored scheme), `README.md` usage + key table, and the `CLAUDE.md` UI line all updated to the new keys.
- Tests (`keys_test.go`): count prefix repeats a motion (`3` then Down moves 3 and resets), `gg`/`G`/`<count>G` land on first/last/nth, fold-all collapses+expands a folder and `za` toggles it; existing chord tests migrated `a`→`i`. Full gate (build/vet/staticcheck) + `go test -race ./...` pass. Pty smoke: TUI drives `t zR zM za gg G 3j c f b [ ] gt a i⎋ q` against a seeded cache, exit 0, no panic.
- Remaining finale increments: search (`/` `n`/`N`), calendar visibility toggle, quick field-set keys, yank/paste (`y`/`p`), `:config`, `:calendar rename|color`.

## 2026-07-07 — Build step 10 (part 5): mouse pass + docs — step 10 complete

- **Mouse** (`internal/ui/mouse.go`): app-level `SetMouseCapture` makes the mouse coherent with the mode model on top of tview's built-in click-to-select/scroll — clicking a left overview panel switches to that mode (so the center follows), and a double-click on the task tree or agenda opens the edit form. Skipped while a modal/overlay is up.
- **Docs**: README rewritten for the chorded keymap (a-prefix create with which-key, contextual `d`, `:` commands, `g`/`?`, `+`/`-`/`Ctrl-arrows`, `:conflicts`, mouse) and the status blockquote marks step 10 complete; CLAUDE.md UI line updated. (Full-cell click mapping for the custom calendar grids and detail-pane accordion collapse noted as future niceties.)
- Test: `TestMouseClickSwitchesMode` draws the layout to a simulation screen so panels have rects, then simulates clicks that switch mode. Full gate + `-race` pass. Pty end-to-end: which-key on `a`, `:view week` echoes to the command view, `?` help opens, `+`/`-` accordion, clean exit.
- **Build step 10 complete.** Next: step 11 (recurrence editing semantics).

## 2026-07-07 — Build step 10 (part 4): interactive pane sizing + state file

- **State file** (`internal/state`): a new package persisting small UI prefs in `<dataDir>/<account-id>/state.json` (0600, atomic rename) — separate from config (never app-written) and the vdir cache. `Load` is best-effort (missing/corrupt → zero, never blocks startup). Wired in `main.go`; `ui` stays disk-free — it receives the remembered width and a `SaveState` callback via a new `ui.Options` (Run's signature is now `Run(Options)`).
- **Keyboard resize** (`Ctrl-←`/`Ctrl-→`): grow/shrink the left overview column by a step, clamped to [16, 50], persisted on each change (`resizeLeft`). Uses `Flex.ResizeItem`.
- **Accordion** (`+`/`-`): `+` collapses the left overview so the Main view fills the width and moves focus into the center; `-` restores it. Switching panels (`1`/`2`/`3`) also restores it. Gated out of Agenda mode (its center navigation is driven by the left agenda list). (Detail-pane collapse left as a future extension; the overview collapse delivers the main width win.)
- Help overlay gained a Layout section.
- Tests: `state` round-trip + bad-file-is-zero; `resizeLeft` clamps at both bounds and calls `SaveState`; accordion is restored on mode switch and blocked in Agenda. Full gate + `-race` pass.

## 2026-07-07 — Build step 10 (part 3): interactive conflict resolution (`:conflicts`)

- Closes the piece deferred from step 9 (sync detects conflicts and keeps both; now they're resolvable in-app).
- **Store** (`internal/store/conflict.go`): `ResolveKeepLocal` clears the conflict and adopts the server's current ETag so the next sync's conditional PUT overwrites the server with the local edit (local .ics untouched). `ResolveKeepServer` decodes the stashed server version and writes it clean via `PutRemote`, so the next sync is a no-op. `writeResource` now also clears a name's conflict stash (any deliberate write supersedes a conflict). Keep-both (preserve both as separate items) noted as a future refinement — needs a new-UID clone; keep-local/keep-server cover winner-picking.
- **UI** (`internal/ui/conflicts.go`): `:conflicts` opens a list of conflicted items (calendar — title); Enter opens a Keep local / Keep server / Cancel chooser; resolving refreshes the views (and the sync-status conflict count) and rebuilds the list, auto-closing when none remain. Added to `:help` and the command dispatcher. The status bar already shows the live conflict count.
- Tests: store — `ResolveKeepLocal` (dirty, adopts server etag, keeps local content, clears conflict, survives reload) and `ResolveKeepServer` (clean, server content adopted). ui — `:conflicts` flashes when none, opens and lists when a conflict is present. Full gate + `-race` pass.

## 2026-07-07 — Build step 10 (part 2): `:` command mode + `?` help overlay

- **Command line** (`internal/ui/command.go`): `:` opens an input near the top; Enter runs, Esc cancels. `runCommand` dispatches `:sync`, `:view month|week|day`, `:goto <date>` (smart-parsed via `ParseQuickAdd`), `:help`, `:q`. Each echoes its command form to the status-bar middle "command view". `g` opens the command line prefilled `goto `. (`:search`/`:config`/`:calendar`/`:conflicts` land in later step-10 increments.)
- **Help overlay** (`internal/ui/help.go`): `?` (and `:help`) open a scrollable cheat sheet grouped by area (panels/nav, create chords, edit, calendar, sync/commands); Esc/`q`/`?` closes, `j`/`k`/arrows scroll. Controls line now advertises `: cmd · ? help`.
- Tests (`command_test.go`): `:view week` switches to calendar/week and echoes; invalid arg flashes without changing state; `:goto 2026-12-25` moves the anchor + switches to calendar; unparseable goto and unknown commands flash; help overlay opens and closes.
- Full gate + `-race` pass.

## 2026-07-07 — Build step 10 (part 1): chorded keymap + which-key popup

- Start of step 10 (command mode & keybinding polish). First piece: the vim-style chord scheme, replacing the interim standalone create keys.
- **Chord dispatcher** (`internal/ui/keys.go`): `a` is now a prefix — `at`/`aT` task, `ae`/`aE` event, `as`/`aS` subtask (Shift = full form), `ac` calendar, `al` list. `globalKeys` claims the next key when a prefix is pending (before the modal/single-key handling); Esc or an unknown continuation cancels. Bindings live in a `chords` table (data) so the which-key popup and, later, the help screen render from the same source.
- **which-key popup**: after a prefix, a bottom overlay lists the continuations (non-focused — the next keystroke is intercepted by `globalKeys`, so it needs no focus). Chosen per the owner's "shift the object letter" convention.
- **Contextual delete**: `d` deletes the calendar/list when an overview list is focused, else the selected item — folding in the old `D`.
- **Command-view echo**: executing a chord writes its command form to the status bar's middle section (`echo`), the lazygit-style "what you just did" line (fleshed out with `:` command mode next).
- Retired interim keys `A`/`s`/`S`/`c`/`D`; split `addQuick`/`addFull` into typed `addTaskQuick`/`addTaskFull`/`addEventQuick`/`addEventFull`; `createCollection` takes a default-type arg (`ac`→calendar, `al`→list). `r` kept as a `:sync` alias.
- Tests (`keys_test.go`): prefix shows which-key then Esc cancels; `at` completes the chord, opens the quick-add prompt, and echoes the command view; an unknown `az` clears the prefix and flashes. Full gate + `-race` pass.

## 2026-07-07 — Timezone robustness: embed tzdata + Windows-zone map + floating fallback (no more dropped events)

- Follow-up to the read-only fix: another silent-data-loss quirk. go-ical's date parser calls `time.LoadLocation(TZID)` and **errors** on any non-IANA zone (`vendor/.../ical.go:150`); our `ParseEvent`/`ParseTodo` treated that as fatal, so a timed event/todo with an Outlook/Windows TZID (e.g. `Eastern Standard Time`), a custom `VTIMEZONE` label, or *any* TZID on a build without system zoneinfo was rejected and skipped — it silently vanished. Recorded in `main.md` (Timezones decision + step 12).
- **Embed tzdata** (`cmd/lazyplanner/main.go`): blank `import _ "time/tzdata"` bakes the IANA database into the binary, so zones resolve on a minimal Pi image or Windows — fits the "robust single static binary" goal. Verified the binary resolves zones with `ZONEINFO=/nonexistent`.
- **Windows→IANA map** (`internal/model/windowszones.go`): the CLDR windowsZones "001" defaults (~140 entries) map Outlook zone names to IANA.
- **Graceful resolution** (`internal/model/tz.go`, `resolveDateTime`): try go-ical first; on failure map a Windows TZID→IANA; if still unresolved, interpret the value as **floating/local** so the item is kept (at worst offset for an exotic unmapped zone) instead of dropped. Wired into DTSTART, DTEND (recovers an explicit DTEND with a bad TZID; DURATION still handled by go-ical), and DUE.
- Tests: `TestParseEventTimezones` (Windows name → correct IANA offset; real IANA still works; unknown TZID → kept as floating, not dropped); `windowsToIANA` lookups; and a guard that every mapped IANA name actually loads with the embedded db (catches table typos). Full gate + `-race` pass.
- **Owner action**: none required unless you have Outlook-authored events — if any were missing before, they should appear after the next sync.

## 2026-07-07 — Read-only calendars (NextCloud birthdays etc.): detect + block + pull-only

- Owner report: events added to NextCloud's generated "Contact Birthdays" calendar (read-only, no writes allowed in the web UI) were silently discarded during sync. Root cause: LazyPlanner pushed them, the server rejected/dropped them, and reconcile then treated the missing server copy as a remote deletion and `Forget`-ed them. Fix: **know a calendar is read-only and never write to it** (mirrors NextCloud web). Decision recorded in `main.md`. Owner approved discarding the already-stuck test events.
- **Detection** (`internal/caldav/privileges.go`): a Depth-1 `PROPFIND current-user-privilege-set` (RFC 3744) on the calendar home set, issued over our own authed HTTP client (go-webdav's client neither requests nor exposes privileges — same gap as MKCALENDAR). A calendar granting read but not write/write-content/bind/all is read-only. `caldav.Calendar` gained `ReadOnly`, set during `DiscoverCalendars`; a failed privilege query degrades gracefully (fail-open). Plus a **reactive safety net**: `PutObject`/`DeleteObject` map HTTP **403 → `ErrReadOnly`**.
- **Store** (`internal/store`): `Calendar.ReadOnly` + sidecar `read_only` (persists so the UI knows offline) + `SetCalendarReadOnly` (no-op when unchanged).
- **Sync** (`internal/sync/sync.go`): each sync persists the server's read-only status. A read-only calendar is reconciled **pull-only** (`reconcileReadOnly`): local dirty/never-synced resources are discarded, local deletions (tombstones) are reverted by re-pulling, and the server state is mirrored in. If a write ever returns `ErrReadOnly` (privilege detection missed it), the calendar is flagged read-only and the change discarded. New `SyncResult.Discarded` counter.
- **UI** (`internal/ui`): `guardWrite` blocks create/edit/complete/delete/re-parent (and delete-collection) on a read-only calendar with a "read-only" flash — at the source, before opening any form. Read-only calendars/task lists show a `[ro]` marker in the overview lists.
- Tests: caldav — `discoverWritable` parses privilege multistatus (writable vs read-only), 403→`ErrReadOnly`. store — read-only persists across reload. sync — read-only calendar discards a stuck local event and mirrors the server (no writes), reactive-403 marks read-only + discards. ui — `guardWrite` blocks + flashes, `[ro]` marker renders. Full gate + `-race` pass. Pty: read-only calendar blocks `a` (add) with a read-only flash. (Fixed a test-hygiene bug: the read-only UI tests must use the writable temp-copy app harness, not the shared in-place fixture.)
- **Owner action**: confirm against real NextCloud that the birthday calendar is now detected read-only (shows `[ro]`, refuses edits, mirrors birthdays in).

## 2026-07-07 — Build step 9 (part 5): in-app calendar / list creation + deletion (offline-first) — step 9 complete

- Final step-9 piece: create/delete calendars and task lists in-app, offline-first (local now, server round-trip on next sync). **Build step 9 is complete.**
- **Store calendar management** (`internal/store/calendar.go` + sidecar/store): per-calendar pending state in the sidecar (`pending_create`, `pending_delete`, `components`). `CreateCalendarLocal(id, meta, components)` makes the collection locally, flagged for MKCALENDAR. `MarkCalendarDeleted` hides the calendar from `Calendars()` immediately and flags it for a server DELETE (a never-pushed calendar is removed outright, no round-trip). `MarkCalendarSynced`/`RemoveCalendarLocal`/`PendingCalendarDeletes` support the sync push. `Calendars()` skips pending-deletes; the `Calendar` snapshot gained `PendingCreate`/`Components`.
- **Sync** (`internal/sync/sync.go`): before discovery, `pushCalendarDeletes` issues server `DELETE` for calendars marked deleted (then removes them locally; a failed delete stays pending and is not re-imported), and `pushCalendarCreates` issues `MKCALENDAR` (under the calendar-home-set) for locally-created calendars, then records the href so the following reconcile pushes their resources. `Syncer` extended with `CalendarHomeSet`/`CreateCalendar`/`DeleteCalendar` (all already on `*caldav.Client`). New result counters `CalendarsCreated`/`CalendarsDeleted`.
- **UI** (`internal/ui/calendar.go`, `app.go`): **`c`** opens a create form (Name + Type: Event calendar / Task list / Both — defaults to a task list in Tasks mode); **`D`** deletes the highlighted calendar (Calendars) or list (Tasks) with a confirm. Both offline-first. Interim keys (fold into the `a`-prefix `ac`/`al` in step 10); added to the controls line.
- Tests: store calendar API exercised via sync tests; sync — create-local-calendar-then-push-its-resources (MKCALENDAR spec + resources pushed), delete-local-calendar-on-server (DELETE issued, not re-imported), delete-never-pushed-skips-server. UI — `componentsForType`, delete-needs-collection-pane guard. Full gate + `-race` pass. Pty: `c` → typed name → Create writes `<account-id>/calendars/Groceries/` with `pending_create:true` + `components:["VEVENT"]`; exit 0.
- **Owner action**: real-NextCloud MKCALENDAR/DELETE-on-sync acceptance to be confirmed by the owner alongside the sync validation.

## 2026-07-07 — Build step 9 (part 4): in-app sync trigger + sync-status indicator

- Wired the sync engine into the TUI and the status bar.
- **UI sync** (`internal/ui/sync.go`, `app.go`): `Run` now takes a `syncFn` closure (nil = no server → app runs fully offline). `triggerSync` runs the sync on a background goroutine (UI never blocks on the network), coalesces overlapping requests, and on completion `QueueUpdateDraw`s a view refresh + status repaint. **Background sync on startup** fires from `Run` (offline-first: opens instantly from cache, refreshes when sync lands). Interim manual trigger on **`r`** (the real `:sync` command lands with command mode in step 10).
- **Sync-status indicator** (`render.go` `updateStatus` → `renderSyncStatus`): the status bar's right section now shows real state with color+words (TTY-safe, no glyphs): `not configured` (gray) · `syncing...` (yellow) · `synced HH:MM` (green) · `! N conflict(s)` (red, from `store.Conflicts()`) · `offline` (red, on error). Replaced the step-9 placeholder. Controls line gained `r sync`.
- **Wiring** (`cmd/lazyplanner/main.go`): `buildSyncFn` builds a caldav client from `[server]` (resolving `password_command`) and returns the closure; a failing password command or client build is a warning, not fatal — the app opens offline.
- Tests (`internal/ui/sync_test.go`): `renderSyncStatus` across all five states; `syncSummary` (quiet sync → empty, else up/down/conflict); `triggerSync` no-op + hint when unconfigured. Full gate + `-race` pass. Pty smoke: TUI launches, background startup sync against an unreachable server resolves to `offline`, `r` re-triggers, `q` exits 0 — no panic.

## 2026-07-07 — Build step 9 (part 3): two-way sync engine + `lazyplanner sync` CLI

- The must-have feature: ETag-based two-way reconciliation that never silently overwrites.
- **Sync engine** (`internal/sync/sync.go`): `Sync(ctx, Syncer, *store.Store)` reconciles the cache against the server resource by resource, keyed by href + ETag + the local dirty flag. Per-resource decisions: local create (no href) → `PUT If-None-Match:*`; local edit + server unchanged → `PUT If-Match:etag`; server edit + local clean → pull; **both edited → conflict (keep both, flag, no overwrite)**; server-new → pull; server-deleted + local clean → drop locally (`store.Forget`, no tombstone); server-deleted + local edited → conflict (keep local); tombstone → `DELETE If-Match:etag`; tombstone vs server edit (412) → resurrect the server copy + flag. A conflicted resource is skipped on later syncs until resolved. New server calendars are created + pulled; calendars only present locally are left untouched (in-app calendar management handles those). Discovery/listing errors abort; per-resource errors collect in `SyncResult.Skipped`. `Syncer` interface (DiscoverCalendars/DownloadAll/PutObject/DeleteObject) keeps go-ical out of `sync` — pushes go through `model.Parsed.Encode()` → `[]byte`.
- **Store conflict support** (`internal/store/{conflict,sidecar,store,mutate}.go`): `MarkConflict` stashes the server's diverging version losslessly in the sidecar and flags the local resource `Conflicted`; `Conflicts()` lists them (drives the status count → part 4); `Forget` deletes a resource locally without leaving a tombstone (server already lacks it); `remove` also clears any conflict on delete.
- **caldav.PutObject → `[]byte`**: takes the encoded body instead of `*ical.Calendar` so `sync` needs no ical import (architecture rule: go-ical confined to model/caldav).
- **CLI** (`cmd/lazyplanner/sync.go`): `lazyplanner sync` runs a two-way sync against the account-namespaced cache (flags/env creds), printing pushed/pulled/deleted/conflict counts — the runnable path to validate against real NextCloud before the UI drives it.
- Tests (`internal/sync/sync_test.go`): in-memory fake server; one test per branch — push-create (+ idempotent second sync), push-edit, pull-server-edit, pull-new-server-object, conflict-keeps-both (+ skipped next sync), server-delete-drops-clean (no tombstone), server-delete-vs-edit conflict, tombstone push, tombstone-vs-server-edit conflict (resurrect), new-server-calendar. Full gate + `-race` pass.
- **Owner action**: real-NextCloud sync acceptance to be confirmed by the owner (`lazyplanner sync`) — the engine is fake-tested, like the MKCALENDAR work.

## 2026-07-07 — Build step 9 (part 2): sync primitives — delete tombstones + conditional PUT/DELETE

- The store and caldav pieces the two-way engine needs; no sync logic yet.
- **Store tombstones** (`internal/store/{sidecar,store,mutate,tombstone}.go`): deleting a resource that was previously synced (has an `Href`) now records a **tombstone** (href + last ETag) in the sidecar, so sync can push the deletion instead of the resource silently reappearing as "new on server" on the next pull. A never-synced local delete (no Href) leaves no tombstone. `writeResource` clears a name's tombstone whenever it writes that name — so undo's `Restore` resurrecting a just-deleted resource cancels the pending delete for free. New store API: `Tombstones()` (sorted, cross-calendar) and `ClearTombstone` (after a successful server DELETE). Tombstones persist across reload.
- **caldav conditional writes** (`internal/caldav/object.go`): `PutObject(href, cal, ifMatch, create)` — issues the PUT over the authenticated HTTP client (go-webdav's `PutCalendarObject` can't set conditional headers) with `If-Match: <etag>` on update or `If-None-Match: *` on create, so the app never blindly overwrites; returns the new bare ETag. `DeleteObject(href, ifMatch)` — conditional DELETE; 404 is idempotent success. Both map HTTP **412 → `ErrPreconditionFailed`** so sync can turn a lost race into a conflict. ETag representation pinned: the store keeps **bare** ETags (matching go-webdav's unquoting download path) and the header layer quotes/unquotes at the boundary (`normalizeETag`/`httpETag`), so ETags from every code path compare equal.
- Tests: store — synced-delete leaves a tombstone that survives reload and clears; never-synced delete leaves none; `Restore` clears a tombstone. caldav — create sends `If-None-Match: *`; update sends a **quoted** `If-Match` from a bare stored etag and returns the bare new etag; 412 → `ErrPreconditionFailed` (PUT + DELETE); 404 delete is success. Full gate passes.

## 2026-07-07 — Build step 9 (part 1): config module + account-keyed cache path

- Start of two-way sync (step 9). First two sub-parts: the config file and the account-namespaced cache path (both prerequisites — sync needs credentials, and a mismatched cache would corrupt conflict detection).
- **Config module** (`internal/config/config.go`, `template.go`): added `BurntSushi/toml` (vendored). `Config` = `[server]` (url/username/password/**password_command**) + `[appearance]` (first_day_of_week, default_view, time_format, date_format) + `[behavior]` (sync_interval_minutes). `Load()` overlays the file on owner-preferred `Default()`s (a working config needs only `[server]`); a missing file returns `configured=false`. `GenerateDefault()` writes a **fully-commented starter config.toml** (every option at its default, commented) `0600`, never overwriting an existing file. Loose-permission (`&0o077`) files get a non-fatal chmod-600 warning (POSIX-only). `Server.ResolvePassword()` runs `password_command` via `sh -c` (owner's `bw get password lazyplanner`), else inline password — resolved at connect time, not load.
- **Account-keyed cache** (`config.AccountID`, `AccountDataDir`): opaque 12-hex-char sha256 of normalized `url\x00username` (trailing-slash/case-insensitive). Cache root is now `<dataDir>/<account-id>/calendars/…`. Wired into `runTUI` (loads config; on first run writes the starter config and exits so the user fills in `[server]`) and the `import` CLI (same id so import and TUI agree). **No auto-migration** of the old un-namespaced `<dataDir>/calendars/` — the server is source of truth, so a re-import repopulates the new path.
- Tests (`internal/config/config_test.go`): missing→defaults, file-overlay-keeps-omitted-defaults, loose-permission warning, `ResolvePassword` (command precedence + trim, inline fallback), `AccountID` (normalization + distinctness), `GenerateDefault` (parses, 0600, no-overwrite). Full gate (build/vet/staticcheck/test) passes.

## 2026-07-07 — Spec: account model (single-account, server-keyed cache) folded into step 9

- Owner asked to record the account-switching plan before starting step 9. Decision: LazyPlanner stays **single-account** (one `[server]`, no in-app switcher), but account switching — expected rare — **must be safe**, so the local vdir cache is namespaced by a stable `<account-id>` derived from server URL + username (`<dataDir>/<account-id>/calendars/…`). Changing the server connection then maps to a separate cache; two accounts can never share one directory. Rationale: sidecar ETags/hrefs are server-specific, so a mixed cache would corrupt two-way-sync conflict detection.
- Scoped as a **cheap safeguard, not a feature**: full multi-account profiles (`[[account]]` blocks + `:account` switcher) are noted as a future enhancement, explicitly out of initial scope.
- `main.md`: new **Account model** entry in Settled Decisions; **Build Plan step 9** folds in the account-keyed cache path (wired with two-way sync, when a mismatched cache first becomes dangerous).
- Spec-only change (no code). Verified `main.md` reads cleanly and `log.md` heading count matches entry count.

## 2026-07-06 — UI: all-day drill, filled-box completed glyph, full-day time-grid

- Three owner-requested UI changes before step 9.
- **All-day events in the week/day drill cycle** (`timegridview.go`): `dayOccs` now returns the selected day's all-day items first, then timed ones, so `Enter`-to-cycle covers all-day events too. The cycled all-day event is shown highlighted (reverse) in the top band; timed events highlight their block as before. Detail pane follows the selection.
- **Completed-task glyph**: the checkbox now fills with `[■]` when done (was `[x]`) — in the task tree (`nodeLabel`), the month-grid day cells (`itemLabel`), and the agenda list (`agendaLeftLabel`). Hide-by-default behavior is unchanged (glyph only).
- **Week/day fills the height**: the time-grid now scales the full 24-hour day across the pane body (`row = bodyY + hour*bodyH/24`) instead of one fixed row per hour with a scroll window — the day always fills the screen, hour rows grow with the window, and event blocks are sized proportionally. Removed `scrollHour` and the scroll keys (nothing to scroll).
- Tests: `TestTimeGridDrillsAllDayFirst` (all-day cycles before timed), `TestNodeLabelCompletedGlyph` (`[■]`), and `TestTimeGridDrawsDay` now asserts the whole day renders (12am..11pm). Full gate + `-race` pass; pty confirms the day view fills top-to-bottom with the all-day band and a timed block, no panic.

## 2026-07-05 — Fix: legible selection highlight on any theme (reverse video)

- Report: the terminal-background fix made highlighted (selected) text illegible on every tested terminal — a latent bug the black background had masked.
- Cause: tview's default selected style (List `selectedStyle`, TreeNode `selectedTextStyle`) is `Foreground(Styles.PrimitiveBackgroundColor).Background(Styles.PrimaryTextColor)`. With `PrimitiveBackgroundColor` now `ColorDefault`, the selected *foreground* became the terminal's default text color (usually light) on a light bar → unreadable. Previously it was black (the old default), which happened to be legible.
- Fix: select with **reverse video** (`tcell.StyleDefault.Reverse(true)`) for the overview lists (`SetSelectedStyle`) and every task-tree node (`SetSelectedTextStyle`). Reverse is the inverse of the already-legible normal text, so it stays readable on any light or dark scheme and doesn't depend on the palette. The calendar/agenda/time-grid selections were already independent of the primitive background (outline box / explicit fill / reverse) and were unaffected.
- Test: `TestSelectionIsLegible` asserts the highlighted list row renders with the reverse attribute. Full gate + `-race` pass.

## 2026-07-05 — Fix: inherit the terminal background (no more shaded text boxes)

- Report: on some terminal color schemes, text sat in a shaded box (text background ≠ overall background).
- Cause: tview's default `Styles.PrimitiveBackgroundColor` is solid **black**, so every pane/box filled black, while our custom-drawn text (calendar/agenda/time-grid via `printStyled` with `tcell.StyleDefault`) uses the **terminal default** background. On any scheme whose background isn't pure black, the black fill vs. default-bg text cells showed as boxes behind the text.
- Fix: set `tview.Styles.PrimitiveBackgroundColor = tcell.ColorDefault` (in `useTerminalTheme`, folded in with the rounded-border setup and run before any widget is created), so panes, boxes, and text all inherit the terminal's background. Deliberate fills (time-grid event blocks, selection highlights, list selection) still use explicit colors.
- Test: `TestTextInheritsPaneBackground` renders the agenda and asserts a text cell shares the surrounding pane's background (both terminal default). Full gate + `-race` pass.

## 2026-07-05 — Fix: H/L re-parent now reads the on-screen tree (WYSIWYG)

- Bug: after the folders/sticky-complete work, H/L (indent/outdent) misbehaved — often "can't indent"/"already top level" or nesting under the wrong task.
- Cause: `reparentSelected` recomputed sibling/parent structure with `model.BuildTree(todos, showCompleted)`, but `buildTree` now renders a *different* forest (`BuildTree(visible = incomplete + sticky, true)`). Before folders/sticky both used the same call, so they matched; now they diverge (e.g. the row visually above you is a sticky-completed task the reparent forest omits), so H/L computed the wrong sibling/parent or a no-op.
- Fix (owner chose WYSIWYG): compute parent + previous-sibling directly from the displayed tview tree via `treeNodeContext` (walks `a.tree` for the node's parent + index) — indent nests under the row shown directly above at the same level; outdent moves to the parent's parent (or top level when the parent node is the list root). No second forest, so H/L can't drift from the rendering again. Removed the now-unused `findInForest`.
- Test: `TestReparentUsesOnScreenSibling` indents below a sticky-completed row and asserts it nests under that on-screen row (would fail under the old forest-mismatch logic). Existing reparent test still passes; full gate + `-race` pass.

## 2026-07-05 — Popup restyle: terminal-themed forms with a ▸ focus caret

- Owner review of the popups. Reworked the edit/create forms, quick-add line, and confirm dialog to a single look: the terminal's **default (unified) background**, **high-contrast default text**, and an **accent (teal) rounded border/title** — no more white card.
- **Focus caret**: tview reapplies one field style to every form field each frame (and `FormItem` has no `SetLabel`), so a per-field "white when focused" isn't possible. New `caretForm` (`internal/ui/forms.go`) wraps `tview.Form` and, in `Draw`, marks the focused field (`GetFocusedItemIndex`) with a `▸` in a fixed two-column label gutter; the focused button is reversed. Forms now hold explicit field references (`todoFields`/`eventFields`) and read values from them, since the moving caret changes labels and label-lookup would break.
- Removed the old `styleBWForm`/`formText`.
- Tests: `TestCaretFormGutter` exercises the Draw override + gutter (the ▸-on-focus placement needs the live app, verified via pty). Full gate + `-race` pass; pty confirms the edit form shows the caret, labels, title, and Save on a terminal-themed background.

## 2026-07-05 — Fix: sticky-complete worked only on the first task list

- Bug: checking off a task while completed are hidden only kept it visible on the first list; later lists reverted to the old (immediate-hide) behavior.
- Cause: the list-change detection (which drops the sticky pins) lived in `buildTree` and compared `selectedTasklistID()` to `treeListID`. During a panel rebuild, `List.Clear`/`AddItem` park the selection at index 0, and — critically — `List.SetCurrentItem` fires its changed callback *before* updating the current item, so `GetCurrentItem()` was stale (returned the first list). Both made `buildTree` see the first list's id mid-refresh and wipe `stickyDone` for any other list.
- Fix: moved the sticky-clear out of `buildTree` into the tasklist changed-callback, keyed on the callback's **index argument** (reliable) rather than `GetCurrentItem`; suppressed the callback during panel rebuilds (`suspendTree`); and sync `treeListID` when entering tasks mode so restore events aren't misread as a list switch.
- Test: `TestStickyWorksOnNonFirstList` completes a task on a second list and asserts it stays visible. Full gate + `-race` pass.

## 2026-07-05 — UI polish pass (3/3): week/day drill-in, agenda outline box, modal focus

- **Week/day drill-in** (`timegridview.go`): the time-grid now mirrors the month grid — `Enter` on the selected day enters event mode, `↑`/`↓` (`k`/`j`) cycle its timed events with the current one boxed/highlighted and shown in the Detail pane, `Esc`/`←`/`→` back out. New `onSelectEvent` callback + `eventMode`.
- **Agenda outline box** (`agendaboard.go`, new): replaced the agenda center's tview.TextView with a custom-drawn widget that draws a **rounded outline box** around the selected item (matching the calendar's day cursor) instead of a filled bar; items keep their green/aqua colors. It manages its own scroll to keep the selection visible; selection is driven by the left Agenda list.
- **Modal return-focus** (`edit.go`): closing a dialog returns focus to where you were — including a drilled-into calendar day — via a `calGrid` interface (`drillState`/`reDrill`) implemented by both the month and time-grid views, captured on open and restored on close (create/edit refresh first, then restore so the grid can re-drill).
- Tests: time-grid drill-in (Enter → event mode + emit, Esc → exit); agenda selection is outlined (rounded corner drawn, title keeps its color, not inverted). Full gate + `-race` pass; pty smoke test confirms folder arrows, rounded corners, the agenda box, and week drill-in all render with no panics.

## 2026-07-05 — UI polish pass (2/3): create task vs subtask, folders, sticky-complete

- **Create keys** (`edit.go`, `app.go`): split creation into distinct actions — `a` quick-add top-level task (or event in calendar/agenda), `s` quick-add subtask under the highlighted task, `A`/`S` the same via the full form. Refactored the forms into reusable builders (`newTodoForm`/`newEventForm`) + readers (`readTodoDraft`/`readEventDraft`) shared by edit and create; `commitMutation` is the shared write/undo/refresh tail.
- **Folders**: a task with ≥1 incomplete child renders `▸`/`▾` (in place of `[ ]`/`[x]`), the marker flips on expand/collapse; folders can't be completed until their children are (guarded in `toggleComplete`), and revert to a normal task when empty/all-done (`folderSet` recomputed each build). Deleting a task now takes its whole subtree — `descendants` gathers them, the confirm counts them ("Delete X and its N subtask(s)?"), and undo restores them all.
- **Undo** generalized to compound steps (`undoStep.ops []undoOp`) so a recursive delete undoes in one `u`; `pushUndo` helper; all sites migrated.
- **Sticky-complete**: checking off a task while completed are hidden pins it visible (`stickyDone`) until the list is left (switching list or pane), not on a popup/refresh. Fixed a subtle bug where the panel-rebuild's transient empty selection cleared the pins.
- Tests (`edit_test.go`): folder blocks completion until children done then completes; sticky keeps a completed task visible then hides it after leaving the list; `descendants` depth. All pass incl. `-race`.

## 2026-07-05 — UI polish pass (1/3): status bar, cosmetics, tz + Space bugs

- First of a multi-part UI refinement batch (owner feedback). Spec/plan updates + localized fixes + chrome; the behavioral pieces (create-task-vs-subtask, folders, agenda outline widget, week/day drill-in, modal focus restore) land in follow-up commits.
- **Spec/plan** (`main.md`, `CLAUDE.md`): deferred to their proper steps with documentation — in-app **calendar/list creation → step 9** (server MKCALENDAR), **command view + `:` line + full vim-style chorded keymap → step 10**, **sync-status indicator → step 9/12**. Documented (for build now): two-line status bar, create-task vs create-subtask, quick-add/full-form toggle via distinct keys, folder semantics, rounded borders, B/W dialogs, agenda outline box, week/day drill-in, "keep completed visible until leaving list", UTC-store/local-display.
- **Status bar** (`app.go`, `render.go`): the bottom is now two lines — a 3-section bar (left general/results, middle command-view stub → step 10, right sync stub → step 9) above an always-visible controls line. `flash()` writes the left section; it persists until the next `updateStatus`.
- **Cosmetics**: rounded (soft) corners on all borders (`tview.Borders` + custom `drawBox`); monochrome confirm modal and edit forms (white card, black labels, black input boxes). Note: tview applies one field style to every form field per frame, so per-field "white when focused" isn't possible without a custom form; the black boxes on a white card read clearly.
- **Bugs**: timed values are stored UTC but were rendered without converting to local (a created `12:00am` showed as 4am on a UTC-4 box) — all event/occurrence render sites now convert to local. `Space` no longer flashes "select a task" in views where nothing is toggleable (silent no-op). Edit-form fields use `fieldWidth 0` so they fit the dialog instead of overflowing. Shortened the controls line so it doesn't truncate.
- Tests: existing model/store/ui suites pass; pty check confirms rounded corners render, the two-line status bar + sync stub show, and the B/W confirm opens.

## 2026-07-05 — Build step 8: editing (create/edit/complete/delete + undo + re-parent)

- Editing from the UI; writes go to the local vdir only (server push is step 9). Scope confirmed with owner: core create/edit/complete/delete plus session undo (`u`) and indent/outdent re-parent (`H`/`L`).
- **model** (`internal/model/edit.go`, `quickadd.go`): field-mutation + construction helpers honoring the property-preservation iron rule — `EditTodo`/`EditEvent`/`SetTodoCompleted`/`SetTodoParent` clone via encode→decode and mutate the raw component (unknown props, VALARMs survive); `NewTodoObject`/`NewEventObject` build fresh objects (`NewUID`, VERSION/PRODID, DTSTAMP). Timed values stored UTC (Z), all-day date-only. Int props (PRIORITY/PERCENT-COMPLETE/SEQUENCE) written without a VALUE=TEXT tag so they round-trip. Quick-add parser: conservative/documented tokens (dates, times requiring am/pm-or-colon, `!priority`, `#tag`); ambiguity stays in the title; `QuickAdd.At` combines the parsed date/time onto a context day.
- **store** (`internal/store/mutate.go`): `Locate(uid)` finds the resource holding an event/todo; `Restore` writes a prior snapshot back exactly (ETag/Href/Dirty) — the undo primitive. (`Put`/`Delete` already existed from step 4.)
- **ui** (`internal/ui/edit.go`, `app.go`): keys `a` quick-add (contextual), `e` full form (tview.Form modal), `Space` complete-toggle, `d` delete-with-confirm, `u` multi-level session undo (memento of the pre-change snapshot; create→delete, else Restore), `H`/`L` re-parent via RELATED-TO. Top-level `Pages` root hosts centered modal overlays; `globalKeys` yields all keys while a modal is open. New events target the highlighted calendar, new tasks the selected list; a new task nests under the selected tree node. `refresh` rebuilds panels preserving selection and reselects the touched item by UID.
- Tests: model — iron-rule preservation, clone independence, completion round-trip, re-parent preserving other relations, quick-add table. store — Locate, Restore-undoes-edit. ui — create+undo, complete-toggle+undo, indent+undo (headless app harness over a temp copy of the fixture). Full gate + `-race` pass.
- Verified end-to-end via pty against a seeded writable cache: quick-add task modal wrote a file with DUE/PRIORITY:1/NEEDS-ACTION, edit form opened and cancelled, quick-add event wrote DTSTART; exit 0, no panic.

## 2026-07-05 — UI: legible agenda selection + task-tree rooted at list name

- Two owner-noticed polish items.
- **Agenda highlight legibility**: the selected agenda block's title was illegible under tview's region highlight. Root cause: tview derives highlight contrast from a color's *nominal* RGB, but the terminal's 16-color palette remaps those colors, so e.g. a green title became low-contrast under the auto-picked highlight. Fix: stop using tview's region highlight for the agenda; render the selected block ourselves as an explicit **black-on-white** bar (`agendaItemBlock(it, plain)` emits no color tags for the selected block so the uniform wrap wins), and scroll it into view manually (`scrollAgendaTo` — keeps the block visible like a list cursor instead of jumping to top). Non-selected blocks keep their green/aqua colors.
- **Task tree root**: top-level tasks previously dangled from an empty, invisible root node (stems connecting to nothing). The tree root now shows the **list's own name** (teal), so the top-level tasks attach to it like a file tree rooted at its directory.
- Refactor: extracted `newApp(store, title, now)` from `Run` so the UI can be built + loaded headlessly with a fixed clock for tests.
- Files: `internal/ui/{render.go (renderAgenda/scrollAgendaTo/currentAgendaIndex, agendaItemBlock plain mode, buildTree root name), app.go (newApp, drop SetRegions + syncAgendaHighlight, SetChangedFunc→renderAgenda)}`; spec note in `main.md` (task tree rooted at list name).
- Tests: new `internal/ui/app_test.go` — `TestAgendaSelectedBlockLegible` asserts the selected title renders `fg=black,bg=white` on a `SimulationScreen` (guards the contrast fix), `TestTaskTreeRootIsListName` asserts the root text equals the list's display name with children attached. Full gate + `-race` pass; pty smoke test (agenda up/down, tasks) exits 0, no panic.

## 2026-07-05 — UI: focus lives in the overview (calendar + agenda)

- Owner-requested tweak before step 8 (spec updated in `main.md` UI Design + `CLAUDE.md` + `README.md`). Previously `1` and `3` jumped focus straight into the center pane; now all three modes focus their **left overview panel** first (matching how Tasks already worked), so the highlight lives in the overview.
- **Calendars (`1`)** → focus the left Calendars list; arrows highlight each calendar (per-calendar visibility toggles land in step 10). `Enter` dives into the grid (arrows→days, `Enter`→cycle the day's events, `Esc`→back to the list). Added `[` / `]` to cycle the highlighted calendar from anywhere in calendar mode — including while diving in the grid (fast-nav, owner's request). `v`/`n`/`p`/`t` no longer yank focus back to the grid: new `refocusCalendar` keeps focus on the list unless the grid itself was focused (then it follows the swapped month/time primitive).
- **Agenda (`3`)** → focus the left Agenda list; moving its highlight highlights the matching block in the center pane and auto-scrolls to it. Center agenda blocks are now wrapped in tview text regions (`["item-N"]`, `SetRegions(true)`), driven by `syncAgendaHighlight` via the list's `SetChangedFunc`. Detail pane stays hidden (full-width center) as before.
- `Enter`/`Esc` wiring: `calendarView` and `timeGridView` gained an `onExit` callback (Esc in day-mode / time-grid hands focus back to the Calendars list); the month grid's existing two-level Esc (event-mode → day-mode) still works, then a further Esc returns to the list.
- Files: `internal/ui/{app.go (focus model, refocusCalendar/gridFocused/cycleCalendar/syncAgendaHighlight, `[`/`]` keys, agendaCount), render.go (agenda region tags, agendaCount, status hints), calendarview.go (onExit + Esc), timegridview.go (onExit + Esc)}`
- Tests: existing `model` + UI `SimulationScreen` suites pass, incl. `-race`. Smoke-tested the compiled binary through a pty against seeded data (today's `Project meeting` + `Buy groceries` todo): drove `1`→cal nav→`[`/`]`→`Enter`/`Esc` dive→`v v t`→`3` agenda highlight→`2`→`Tab`→`q`; exits 0, no panic, expected labels render.

## 2026-07-04 — UI refinement: center-follows-focus, time-grid, highlight

- Owner-driven UI refinement before step 8 (spec updated in `main.md` UI Design + `CLAUDE.md`). Implements the spec's "Main pane follows focus" properly and adds the requested behaviors. All in one pass.
- **Center follows the active overview panel** (`1`/`2`/`3`), rebuilt around a `mode` + a center `Pages`:
  - **Calendars** → the calendar view (month grid / week·day time-grid). Left Calendars panel lists calendars.
  - **Tasks** → left Tasks panel now lists **only the task lists** (calendars with todos); selecting one opens that list's full subtask tree in the center (inline `[ ]`/`[x]`, `!priority`, `due`), and the Detail pane shows the highlighted task's full fields/description.
  - **Agenda** → center shows the day's events/tasks with **full descriptions at full width**; the Detail pane is **hidden** (`Flex.ResizeItem` to 0) and the view scrolls (PageUp/PageDown).
- **Week/Day = hourly time-grid** (`internal/ui/timegridview.go`, new custom primitive): hour axis, all-day band at top, events drawn as duration-sized blocks, overlapping events placed side-by-side. Overlap layout is a pure, tested `model.LayoutDay` (greedy lane assignment + per-cluster lane count). v1: one row/hour, `scrollHour` window (PgUp/PgDn/arrows scroll), simple overlap — proportional/refined overlap can follow.
- **Highlight fix**: the selected calendar day is now an **outline box** (`drawBox`), not a solid teal fill, so event text stays readable (fixes the "events invisible" complaint).
- **Day → event cycling** (point 5): in the month grid, `Enter` on a selected day enters "event mode" — up/down cycle that day's events and the Detail pane shows the highlighted event/task; `Esc`/left/right exits. (Time-grid event cursor deferred; blocks already show details.)
- Focus/navigation kept interim (finalized in step 10): `1`/`2`/`3` select mode, `Tab` cycles, `Enter`/`Esc` dive into/out of the tree and day-events.
- Files: `internal/ui/{app.go (rewritten: modes, center Pages, detail hide, focus/borders), render.go (rewritten: per-mode build + detail), calendarview.go (outline + event cursor), timegridview.go (new)}`; `internal/model/timegrid.go` (+ test)
- Tests: `model.LayoutDay` (non-overlap, 2-way, 3-way peak) + empty; UI `SimulationScreen` tests — month render, week render, month arrow-select, **time-grid day render** (headers/all-day/hour labels/event block), time-grid arrow. All pass, incl. `-race`
- Verified end-to-end via pty against seeded data: mode 1 shows the calendar + today's event + day detail; mode 2 the Work list's tree ("Write report" → "Draft section") with full task detail; mode 3 the full-width agenda (event location + task description), detail hidden; cycle/nav stress exits 0
- Verification: `gofmt`/`go build`/`go vet`/`staticcheck`/`go test ./...` (+`-race`) all pass
- Deferred/notes: proportional time-grid overlap columns and a time-grid event cursor; the calendar Calendars/Agenda left panels are informational for now (drive nothing) pending the step-10 navigation pass
- Issues: none

---

## 2026-07-04 — Calendar grid: custom spacious primitive (replaces the Table)

- Refinement to step 7's calendar (owner chose the "spacious grid" option). The `tview.Table`-based grid rendered content-width, single-line cells that didn't fill the pane; replaced it with a custom-drawn primitive that fills width and height and lists each day's events/tasks in the cell
- New `internal/ui/calendarview.go` — `calendarView` embeds `*tview.Box` and implements `Draw` + `InputHandler`:
  - Draws weekday headers, a header rule, vertical column separators, and one cell per day; each cell shows the day number then event/task lines (`3pm Title`, `[] Task`), with a `+N more` overflow note and a `N (count)` fallback when the cell is only one line tall
  - Today highlighted (bold), adjacent-month days dimmed, selected day background-filled (brighter when focused); colors from the 16-color palette
  - Arrow / `hjkl` move the selection by ±1 / ±7 days via an `onSelect` callback; the app re-anchors the grid only when the day leaves the visible range (period stays put while navigating within it)
  - `printStyled` helper draws background-aware, width-clipped text (tview's `Print` only sets a foreground color); uses `mattn/go-runewidth` (promoted to a direct dep — already vendored via tcell) for correct truncation
- `app.go`/`render.go`: swapped the Table for `calendarView`; `buildCalendar` now computes each visible day's agenda once (`calItems`) and calls `setData`; removed the Table-era `renderGrid`/`countsFor`/`dayCellLabel`/`onDaySelected`. Left column narrowed to 26 and the calendar given proportion 3 (was 2) so it has more room by default
- Files: `internal/ui/calendarview.go` (+ `calendarview_test.go`), `internal/ui/{app,render}.go`
- Tests added (**headless via tcell `SimulationScreen`** — first real UI unit tests): month render (weekday headers, a day number, an event title all present at 140 cols), week render, and arrow-key selection movement — all pass, including `-race`
- Verification: `gofmt`/`go build`/`go vet`/`staticcheck`/`go test ./...` (+`-race`) pass; `go mod verify` clean; pty smoke exits 0
- Note: at ~80 columns the cells are narrow and titles truncate to day numbers; the grid shows full detail on a wide terminal, and step-10 pane resizing (accordion/keyboard) will let the calendar take the whole screen
- Issues: none

---

## 2026-07-04 — Spec: interactive pane sizing added to step 10

- Owner requested interactive pane resizing; agreed to build it in **step 10** (keybinding polish). Recorded in the spec:
  - `main.md`: new **Pane sizing** subsection under UI Design — (A) **accordion expand** (`+`/`-`, lazygit idiom) collapses side panels/Detail so the Main view fills the screen; (B) **keyboard resize** (`Ctrl-←`/`Ctrl-→`) grows/shrinks left-column & Detail widths via `Flex.ResizeItem`, clamped. Sizes remembered in the state file (not config). Mouse drag-to-resize declared out of scope (keyboard-first). Two keymap rows added; Build Plan step 10 updated
  - `CLAUDE.md`: UI Project Context line notes pane sizing lands in step 10
- Feasibility confirmed in tview: `Flex.ResizeItem` (runtime resize), `Application.SetMouseCapture` + `Box.GetRect` (would enable mouse drag, but that's out of scope). No code yet — spec change only
- Also: terminal-resize reflow already works automatically (tview redraws the Flex tree on resize)

---

## 2026-07-04 — Build step 7: calendar views (month/week/day)

- **Build Plan step 7 complete.** Added the center "Main" pane with month/week/day calendar grids and movement keys, moving to the spec's four-region layout (left panels · calendar · detail · status).
- `model` additions (pure, tested): `MonthGrid(anchor, mondayFirst)` (6×7 days, padded with adjacent-month days, DST-safe midnight arithmetic), `Week`, `StartOfWeek`, `DayStart`, `SameDay`, `OccurrencesOn` (occurrences overlapping a day, multi-day aware)
- `internal/ui`:
  - Layout reworked: left column now stacks **Calendars** / **Tasks tree** / **Agenda**; center **Main** is a `Pages` holding the month/week grid (`tview.Table`) and the day view (`TextView`); **Detail** on the right; status bar
  - Month grid: weekday headers (Monday-first default per spec), one selectable cell per day showing the day number + `*N` event/due-task marker, today highlighted, adjacent-month days dimmed; arrow keys move between days and update Detail with that day's agenda
  - Week view = one-week grid; Day view = that day's agenda text
  - Keys: `v` cycles month→week→day, `n`/`p` prev/next period, `t` today (all global — they only affect the calendar); `1`/`2`/`3` focus left panels, `Tab` cycles all four regions including the calendar; focused pane border highlights
  - Event/due-task counts bucketed across the visible grid range (multi-day events mark every covered day; DTEND treated as exclusive; zero-length events counted on their start day)
- Files: `internal/model/{calendar.go,calendar_test.go}`; `internal/ui/{app.go,render.go}` reworked
- Tests added: `StartOfWeek`, `MonthGrid` (6×7, contiguity, correct padding, covers the month), `OccurrencesOn` (incl. a multi-day span) — all pass. UI is thin glue (no unit tests) but **pty-verified**: month grid renders with Monday-first headers + `*1` marker on today + today's agenda in Detail; week view ("Week of Jun 29, 2026") and day view ("Saturday, Jul 4 2026", "2:00pm Today Meeting") render; a cycle/navigate/tab stress run exits 0 (no crash)
- Verification: `gofmt`/`go build`/`go vet`/`staticcheck`/`go test ./...` all pass
- Note: the task tree moved from center (step 6) back to the left column to give the calendar the Main space; deep-tree ergonomics improve with `>` zoom in a later step. UI remains a v1 draft to refine against real screens
- Issues: none

---

## 2026-07-04 — Calendar creation (MKCALENDAR); resolves the go-webdav gap

- **Resolved the spec's flagged verification** ("verify go-webdav calendar creation"). Finding: go-webdav v0.7.0's CalDAV *client* has no calendar-creation method (only its server code handles MKCOL; `webdav.Client.Mkdir` sends a plain MKCOL = generic collection, not a calendar; the low-level request helpers are in go-webdav's unimportable `internal/` package). Owner opted to verify a fix before proceeding to step 7
- **Fix (no NextCloud web UI needed)**: `caldav.Client` now retains the authenticated HTTP client + parsed endpoint, so it can issue verbs go-webdav doesn't expose. Added:
  - `CreateCalendar(ctx, path, CalendarSpec)` — RFC 4791 **MKCALENDAR** with displayname, description, Apple `calendar-color`, and `supported-calendar-component-set` (VEVENT / VTODO / both). Generated XML eyeball-checked for correct namespaces; success = 201, errors surface the server's response body
  - `DeleteCalendar(ctx, path)` — DELETE on the collection (so calendars can be removed in-app too)
  - `CalendarHomeSet(ctx)` — extracted from DiscoverCalendars (principal → home set), reused by create
- **CLI**: new `lazyplanner calendar <list|create|delete>` subcommand (`create` flags: `--name`, `--tasks`, `--both`, `--color`, `--desc`, `--path`; slugifies the name into the collection path under the home set). `main.go` dispatch tidied (`exitOnError`); shared `connFlags` helper extracted and `import` refactored onto it
- Files: `internal/caldav/{client.go (endpoint/http fields, CalendarHomeSet),mkcalendar.go}`; `cmd/lazyplanner/{calendar.go,conn.go}`, `import.go`/`main.go` updated; tests `internal/caldav/mkcalendar_test.go`; README documents the new commands
- Tests added: `CreateCalendar` (method=MKCALENDAR, path, body contains displayname/color/comp set), default-components (VEVENT+VTODO), error surfacing (non-201 includes server hint), `DeleteCalendar` (method=DELETE) — all pass via httptest
- Verification: `gofmt`/`go build`/`go vet`/`staticcheck`/`go test ./...` all pass. **Real-server MKCALENDAR acceptance to be confirmed by the owner** against their NextCloud (`lazyplanner calendar create`) before relying on it
- Memory: recorded the decision + plan ([[calendar-creation-mkcalendar]])

---

## 2026-07-04 — Build step 6: read-only UI shell

- **Build Plan step 6 complete.** First real tview UI: a read-only shell over the imported cache, showing a subtask tree and a day agenda. `lazyplanner` (no args) now opens it.
- **Decomposition**: the testable logic lives in `model` (pure, unit-tested); `internal/ui` is thin tview glue verified by launch. Only `ui` imports tview/tcell (architecture rule holds)
- `model` additions (tested):
  - `BuildTree(todos, includeCompleted)` → `[]*TodoNode` — assembles the subtask forest from `ParentUID`, smart-sorts siblings, hides completed by default (their incomplete descendants surface as roots), and **breaks cycles** in malformed data (guarded against infinite recursion)
  - `SortTodos` — smart sort: due (soonest, undated last) → priority (1 highest, 0/undefined last) → title
  - `DayAgenda(occs, todos, dayStart, dayEnd)` → `[]AgendaItem` — merges event occurrences with todos due that day, all-day first then by time
- `internal/ui` (`app.go` + `render.go`): three-pane read-only shell —
  - **Left column**: Calendars list + Agenda list; **center**: the Tasks tree (centerpiece, given the width) with each calendar as a top-level folder; **right**: Detail pane; **bottom**: status bar with key hints + live counts + load-error indicator
  - Focus with `1`/`2`/`3` and `Tab`/`Shift-Tab` (focused pane border turns yellow); Detail tracks the focused pane's selection; `Enter`/`Space` expand/collapse tasks; `.` toggles completed; `q`/`Ctrl-C` quit; mouse enabled
  - Colors use the terminal 16-color palette (per spec); labels use ASCII markers (`[ ]`/`[x]`) to render on a bare TTY
- Wiring: `ui.Run` now takes `*store.Store`; `cmd/lazyplanner` `runTUI` opens the cache at `config.DataDir()` and hands it to the UI (UI reads only through the store)
- Files: `internal/model/{tree,agenda}.go` + tests; `internal/ui/{app,render}.go` (replaced the placeholder); `cmd/lazyplanner/main.go` updated; README Usage documents the TUI + current keymap
- Tests added: `BuildTree` (hierarchy, hide/show completed, cycle-breaking), `SortTodos`, `DayAgenda` — all pass. UI is thin glue (no unit tests) but **launch-verified** via pty: renders panes + calendar/tree/agenda from a populated cache and quits 0; empty cache shows the welcome/"nothing today" and quits 0; no-TTY still errors gracefully (exit 1, no panic)
- Verification: `gofmt`/`go build`/`go vet`/`staticcheck`/`go test ./...` all pass
- Scope note: the big "Main" pane + month/week/day calendar grids are step 7; this step is the shell + task tree + agenda
- Issues: none

---

## 2026-07-04 — Build step 5: CalDAV one-way import

- **Build Plan step 5 complete.** LazyPlanner can now connect to NextCloud, discover calendars, and download everything into the local vdir — a one-way pull, done before the UI so the model is validated against real server data early. First code that talks to a real server.
- Added and vendored `github.com/emersion/go-webdav` v0.7.0 (go-vcard correctly pruned as unused; MVS keeps the newer go-ical across the module)
- `internal/caldav` — the only package that speaks HTTP. `Client` wraps go-webdav:
  - `NewClient(Config)` — basic-auth (app password) over `webdav.HTTPClientWithBasicAuth`; injectable `*http.Client` for tests; default 30s timeout
  - `DiscoverCalendars(ctx)` — walks current-user-principal → calendar-home-set → calendars
  - `DownloadAll(ctx, path)` — one calendar-query REPORT returning full data + ETags for every resource
  - Types `Calendar` and `Object{Path, ETag, Data *ical.Calendar}`; go-ical stays confined to model/caldav
- `internal/sync` — seeded with the orchestration layer (imports caldav + store + model, the higher tier):
  - `Import(ctx, Source, *store.Store)` — discovers calendars, sets metadata, downloads and upserts every resource as clean/synced. **Pull-only** (no local-change push, no deletion of server-absent locals — that's the two-way sync step). Unparseable/unwritable resources are **skipped and collected**; only discovery/listing failures abort
  - `Source` interface (satisfied by `*caldav.Client`) makes the import unit-testable with a fake
- `internal/store` additions: `PutRemote` (writes a resource clean — not dirty, with server ETag/href), `SetCalendarMeta`, `SetSyncToken`; refactored `Put`/`PutRemote` onto a shared `writeResource`; exported `SafeName`
- `internal/model`: added `Parse(*ical.Calendar, loc)` (Decode now = decode-bytes + Parse) so the sync layer can consume go-webdav's already-decoded calendars
- `internal/config`: added the OS-aware path helpers `DataDir()` / `ConfigDir()` (XDG on Linux, `%LOCALAPPDATA%`/`%APPDATA%` on Windows, `Application Support` on macOS) — needed for a default data location
- **Runnable now**: `lazyplanner import` subcommand (thin wiring in `cmd/lazyplanner`) reads `--url/--username/--password` or `LAZYPLANNER_CALDAV_*` env vars, uses a NextCloud app password, cancels cleanly on Ctrl-C, and prints a summary. README documents it. The owner can validate against real NextCloud immediately
- Files: `internal/caldav/client.go`, `internal/sync/import.go`, `internal/store/remote.go`, `internal/config/paths.go`, `cmd/lazyplanner/import.go`; tests `internal/caldav/client_test.go` (httptest canned multistatus), `internal/sync/import_test.go` (fake source), `internal/store/remote_test.go`
- Tests added: `DownloadAll` against a canned CalDAV REPORT (validated the real query→parse path — and surfaced that go-webdav unquotes ETags via `strconv.Unquote`, so the store holds unquoted etags); `Import` (2 calendars, skips a bad resource, clean state persisted across reload); import discovery-error; `PutRemote`/`SetCalendarMeta` round-trip. All pass, including `-race`
- Verification: `gofmt`/`go build`/`go vet`/`staticcheck`/`go test ./...` (+`-race`) all pass; `go mod verify` clean; `lazyplanner import` with no creds returns a clean error (exit 1)
- Not yet handled (noted for later steps): calendar color (go-webdav's FindCalendars doesn't surface it), pruning of server-absent local resources, and pushing local edits — all part of two-way sync (step 9)
- Issues: none

---

## 2026-07-04 — Build step 4: vdir store

- **Build Plan step 4 complete.** `internal/store` is the local vdir cache — the first package with filesystem I/O. Reads a vdir tree into an in-memory index, writes resources back atomically, and persists sync state in a per-calendar JSON sidecar. Concurrency-safe (RWMutex; passes `go test -race`) since background sync will mutate it while the UI reads.
- Layout: `<dataDir>/calendars/<calendar-id>/<name>.ics` (one file per event/todo object, the local source of truth) + `.lazyplanner.json` sidecar per calendar (server-owned display name/color, sync token, href, and per-resource ETag/href/dirty). The `.ics` files win: sidecar is derived data, rebuilt on sync if lost/corrupt
- Types (all snapshots immutable; resources replaced copy-on-write, never mutated in place): `Store`, `Calendar` (metadata + `[]*Resource`), `Resource` (Name, ETag, Href, Dirty, parsed `*model.Parsed`), `LoadError`
- API:
  - `Open(ctx, dataDir)` — scans calendars, parses each `.ics`, merges sidecar sync state; missing dir → empty store (first run); unparseable/unreadable files are **skipped and recorded** in `LoadErrors()` (a corrupt file never blocks startup)
  - `Calendars()`, `Calendar(id)` — sorted snapshots; DisplayName falls back to id
  - `Todos()`, `EventOccurrences(from, to)` — the in-memory index backing task and calendar-view queries (occurrences via the step-3 recurrence engine, sorted)
  - `Put(ctx, calID, name, obj)` — atomic write-temp-fsync-rename (+ dir fsync), creates the calendar dir on first write, marks the resource `Dirty`, **preserves server identity (ETag/Href) on overwrite** so sync can detect local edits; updates index + sidecar
  - `Delete(ctx, calID, name)` — local delete (server propagation is the sync layer's job)
  - `ResourceName(uid)` — filesystem-safe `.ics` name for new resources
- `model`: added `(*Parsed).Encode()` (symmetric with `Decode`), keeping go-ical confined to `model`; the store round-trips resources through it, so unknown properties are preserved on write (verified by test — an `X-` property survives Put)
- Design decisions: I/O entry points take `context.Context` (checked for cancellation) per the no-uninterruptible-blocking rule; data files `0600` / dirs `0700` (private by default); filename keyed by sanitized UID for new resources, but existing resources keep their on-disk name so they map back to the same server resource
- Files: `internal/store/{store,mutate,sidecar}.go`; tests `internal/store/store_test.go` with a committed fixture vdir tree under `testdata/vdir/` (two calendars, a todo, an untracked file, a corrupt file) plus `t.TempDir()` round-trip tests
- Tests added: load tree (metadata, tracked/untracked ETags, sidecar fallback, load-error surfacing), queries, missing-dir, Put+reload+preservation, server-identity preservation on overwrite, Delete (+reload), cancelled-context — all pass, including `-race`
- Verification: `gofmt`/`go build`/`go vet`/`staticcheck`/`go test ./...` (and `-race`) all pass; `go mod verify` clean (no new deps)
- Issues: none

---

## 2026-07-04 — Build step 3: recurrence expansion

- **Build Plan step 3 complete.** `internal/model` now expands recurring events into concrete occurrences over a date window, wrapping `teambition/rrule-go` behind a small model API. Timezone-aware and heavily tested (recurrence is a classic bug farm).
- `rrule-go` promoted from indirect to a direct dependency; re-vendored (`go mod verify` clean)
- API (`internal/model/recurrence.go`):
  - `Occurrence{Start, End, Event}` — one materialized instance; `Event` points to the underlying component so the UI can show details and route edits
  - `(*Event).Occurrences(from, to)` — expands a single component's RRULE + RDATE − EXDATE within the half-open window `[from, to)`, anchored at DTSTART; non-recurring events yield at most one instance. Queries start one duration early so an instance that begins before the window but overlaps it is still returned
  - `(*Parsed).EventOccurrences(from, to)` — object-level expansion that applies **RECURRENCE-ID overrides**: an override replaces the instance it targets (moved time / changed details) and the original slot is suppressed; orphan overrides stand alone. Results sorted by start
  - `(*Event).Duration()` helper
- Correctness decisions, grounded by probing rrule-go's actual behavior first (then the probe was removed):
  - **DST**: instances keep wall-clock time across transitions (weekly 09:00 stays 09:00 local; the UTC instant shifts an hour). Verified explicitly in a spring-forward test
  - **DTSTART inclusion**: rrule-go emits DTSTART only via an RRULE, so for RDATE-only events DTSTART is added explicitly (it belongs to the recurrence set per RFC 5545)
  - **Must set `ROption.Dtstart`** before building the rule — rrule-go otherwise defaults it to "now"
  - **UNTIL** boundary is inclusive; **EXDATE** must match the instance instant (incl. TZID); all handled
  - **Not yet handled**: `RANGE=THISANDFUTURE` on an override (affects only its own instance for now — noted for the recurrence-editing step); recurring *todos* (deferred — their occurrence semantics tie to completion)
- Files: `internal/model/recurrence.go`; tests `internal/model/recurrence_test.go` with five new fixtures (`recur_weekly_dst`, `recur_exdate`, `recur_allday`, `recur_rdate`, `recur_override`)
- Tests added: weekly-DST, EXDATE, all-day recurring, RDATE-only, windowing (narrow / empty), non-recurring multi-day overlap, and RECURRENCE-ID override — all pass
- Verification: `gofmt`/`go build`/`go vet`/`staticcheck`/`go test ./...` all pass; `go mod verify` clean
- Issues: none

---

## 2026-07-04 — Build step 2: core model (iCalendar parsing)

- **Build Plan step 2 complete.** `internal/model` now parses events and todos from iCalendar data via `emersion/go-ical`. Pure logic, no filesystem/network I/O — fully headless.
- Added and vendored `github.com/emersion/go-ical` (pulls `teambition/rrule-go` as an indirect dep, ready for the recurrence step)
- Types: `Event` (UID, Summary, Start/End, AllDay, Location, Description, HasAlarm, Recurring) and `Todo` (UID, Summary, Due/HasDue/DueAllDay, Status, Priority, Categories, Description, ParentUID, Recurring, `Completed()`); `TodoStatus` enum + `PriorityUndefined`
- Parsers: `ParseEvent`/`ParseTodo` (per-component units) and `Decode` (whole-stream convenience). Design choices honoring the spec:
  - **Property-preservation iron rule**: each type keeps its source `*ical.Component` as `Raw`, and `Parsed.Calendar` retains the whole decoded calendar, so unknown properties/components survive a future re-encode. A decode→encode round-trip test proves an `X-` property and a `VALARM` are preserved
  - **All-day detection** via `VALUE=DATE` (with a bare-`YYYYMMDD` fallback); all-day/date-only values interpreted at local midnight
  - **Timezones**: TZID and UTC (`Z`) instants parsed correctly; floating times interpreted in a caller-supplied `loc` (defaults to `time.Local` per the local-timezone rule)
  - **Subtask hierarchy**: `ParentUID` from `RELATED-TO`, treating absent-or-`RELTYPE=PARENT` as the parent per RFC 5545 (matches NextCloud Tasks)
  - **Graceful degradation**: a malformed *required* field (event DTSTART) errors; malformed *optional* fields degrade to zero values rather than discarding the item. `Decode` fails on the first bad component; per-item resilience is left to the store layer (step 4)
  - **Reminder indicator**: `HasAlarm` reflects VALARM presence only — LazyPlanner never fires notifications
- Files: `internal/model/{decode,event,todo}.go`; tests `internal/model/model_test.go` (table-driven, `package model_test` against the public API) with six `testdata/*.ics` fixtures (timed/all-day/UTC-recurring events; timed-due/completed-subtask/minimal todos)
- Tests added: `TestParseEvents`, `TestParseTodos`, `TestRoundTripPreservesUnknownData`, `TestDecodeMalformedStreamErrors` — all pass (8 subtests, no skips; timezone DB present)
- Verification: `gofmt -l` clean; `go build`/`go vet`/`staticcheck`/`go test ./...` all pass
- Issues: none

---

## 2026-07-04 — Build step 1: project scaffold

- **Build Plan step 1 complete.** First code in the repo; toolchain proven end to end (build, vet, staticcheck, test, and a launchable tview window).
- `go mod init github.com/littekge/LazyPlanner` (Go 1.26.4). Added and **vendored** the UI deps: `rivo/tview` v0.42.0 + `gdamore/tcell/v2` v2.13.10 (with transitive deps); `go mod tidy && go mod vendor`, `vendor/` committed
- Package skeleton created per the `main.md` architecture: `cmd/lazyplanner/main.go` (thin wiring — app identity + hand-off to UI) and `internal/{config,model,store,caldav,sync,ui}`. The not-yet-implemented packages carry a `doc.go` with the package doc comment stating each one's responsibility and separation rule
- `internal/ui/app.go`: `Run(title)` builds a tview Application showing a centered placeholder window; quits cleanly on `q` (explicit) and `Ctrl-C` (tview default); mouse enabled. Only `ui` imports tview/tcell, per the architecture rules
- `.gitignore` (build output, go.work, coverage, editor/OS cruft; `vendor/` deliberately **not** ignored)
- CI: `.github/workflows/ci.yml` — GitHub Actions running `go build`/`go test`/`go vet` + `dominikh/staticcheck-action` on push and PR, using `go-version-file: go.mod`
- Files created: `go.mod`, `go.sum`, `vendor/**`, `cmd/lazyplanner/main.go`, `internal/{config,model,store,caldav,sync}/doc.go`, `internal/ui/app.go`, `.gitignore`, `.github/workflows/ci.yml`. `README.md` updated (status → scaffolding; real build/run instructions)
- Verification: `gofmt -l` clean; `go build ./...`, `go vet ./...`, `staticcheck ./...` all pass; `go test ./...` passes (no test files yet — thin UI glue and empty stubs; real tests begin at step 2). Manually confirmed the TUI: renders `LazyPlanner 0.0.1` and exits 0 on `q` in a pty; exits 1 with a wrapped error (no panic) when no TTY is available
- Issues: none. `go get` auto-upgraded tcell to v2.13.10 (from an initial v2.8.1 resolution) and pulled newer `golang.org/x` deps — expected, all vendored

---

## 2026-07-04 — Log repair: restored per-entry headings; format rule hardened

- Fixed `log.md`: 14 of 15 entries had lost their `## YYYY-MM-DD — Title` headings (an insert-at-top editing mistake repeatedly consumed the previous entry's heading), leaving anonymous `---`-separated blocks. All headings restored from session history; content unchanged
- `CLAUDE.md` Log Format section hardened: every entry gets its own heading (even same-day/same-session), never append under an existing heading, inserts must leave the previous entry byte-identical, and verify heading-count == entry-count after editing

---

## 2026-07-04 — Git branching rules for the build

- `CLAUDE.md`: new Git Branching Rules section — all Claude work happens on **`ai-workspace`** (or branches off it, merged back into it); **never merge or commit to `main`** (owner-only, after review); **`ai-init`** is a frozen branch preserving the pre-build-step-1 state (spec complete, no code) as a permanent reference/reset point
- Workflow commit step updated to name the branch
- Branches created: `ai-init` (frozen at this commit) and `ai-workspace` (checked out, ready for build step 1)

---

## 2026-07-04 — Final pre-build pass: handoff readiness audit

- Audited all spec files with fresh eyes ahead of a new build session; fixed staleness that would mislead a fresh reader:
  - `main.md` header status ("early skeleton" → "spec complete and code-ready, begin at Build Plan step 1"), Current State updated, leftover "TBD — more goals" design-goal bullet replaced with the well-behaved-CalDAV-citizen goal
  - `CLAUDE.md`: removed stale "will be expanded once language decided" note, fixed run command for the cmd/ layout (`go run ./cmd/lazyplanner`), added staticcheck install command (dev tool, not vendored), "config format TBD" → TOML
- Final decisions closed:
  - **License: MIT confirmed** — `LICENSE` (MIT, Gabriel Litteken) already existed from the initial commit and matches
  - **examples/ committed** — reference specs kept in the repo
  - **README.md is a living document**: what the program does, usage docs, build/install for Linux + Windows; updated in the same increment as any user-visible change. Rule added to CLAUDE.md workflow (step 6); starter README written (pre-build status, planned sections stubbed)
  - **CI: deferred to scaffold** — GitHub Actions (test/vet/staticcheck) added to Build Plan step 1, alongside `.gitignore`
- Spec is handoff-ready for the build session

---

## 2026-07-04 — UI follow-up decisions: colors, completed tasks, sorting, undo

- **Colors**: terminal 16-color palette (inherits terminal theme, works on TTY/Pi); server calendar colors mapped to nearest palette color. Truecolor theme rejected
- **Completed tasks**: hidden by default, `.` toggles struck-through display (dotfiles gesture)
- **Sibling task sort**: smart sort — due date, then priority, then title; manual ordering rejected (no standard iCal order field; wouldn't survive other clients)
- **Undo**: session-scoped undo stack (`u`) — every local mutation pushes the prior .ics version onto an in-memory stack; persistent trash deferred
- `main.md`: new subsection under UI Design; `u` and `.` added to keymap. `CLAUDE.md` UI line updated
- Remaining UI details (pane proportions, cell rendering, truncation) deliberately deferred to build steps 6–8 against real screens

---

## 2026-07-04 — UI design v1 draft: spec is code-ready

- **Layout**: lazygit-style three regions — left column of focusable panels (1 Calendars, 2 Tasks tree, 3 Agenda), Main pane whose content follows focus (calendar grid / zoomed task tree / day agenda), always-visible right Detail pane (owner requested event detail next to calendar), status bar with contextual hints + sync state. Chosen over "two workspaces" and "dashboard" alternatives
- **Task tree navigation**: full collapsible tree + `>`/`<` zoom (re-root at selected task, breadcrumb, like cd-ing into a directory). Chosen over ranger-style drill-in and plain tree
- **Creation UX**: `a` quick-add one-liner with smart parsing (dates, times, `!priority`, `#tag`; unparsed text → title; predictable rules documented in `:help`), `e` full form. Chosen over title-only quick-add and form-always
- Draft keybinding table and `:` command set written into `main.md` (hardcoded v1; `[keys]` config section deferred)
- Open Decisions section now empty — **spec is code-ready**; UI marked as v1 draft to refine against real screens during build steps 6–8. Non-blocking loose ends: confirm MIT, verify go-webdav calendar creation
- `CLAUDE.md`: UI summary line added to Project Context

---

## 2026-07-04 — Data model: fields, subtask hierarchy, preservation rule, recurrence scopes

- **Task fields surfaced**: title, due, status, priority (iCal 1–9), tags (CATEGORIES), notes, subtasks. **Subtasks are the owner's centerpiece feature** — arbitrary-depth nesting via RELATED-TO (RELTYPE=PARENT, same as NextCloud Tasks so existing data imports as-is), UI presents the tree like a file explorer; "folders" are just tasks with children
- **Event fields surfaced**: title, start/end, all-day, recurrence, location, notes, reminder indicator (LazyPlanner shows alarms exist but never fires notifications — phone/NextCloud handle that)
- **Property preservation iron rule**: never drop/mangle unrecognized iCal properties; edits to known fields preserve everything else. Added to CLAUDE.md as a hard rule
- **Timezones**: store server's data, display/create in system local timezone, all-day items date-only
- **Recurrence editing**: all three scopes (only-this via RECURRENCE-ID, this-and-future via series split, all via master edit)
- `main.md`: core features bullet rewritten around the subtask tree; six data-model entries added to Settled Decisions; Open Decisions down to UI design only
- Also: committed the spec files (d9cc198) — examples/ left untracked pending owner preference

---

## 2026-07-04 — Sync design: credentials, conflict resolution, triggers

- **Credentials**: NextCloud app password only (never the real password), stored in `config.toml` with enforced-0600 warning; optional `password_command` escape hatch (owner runs Vaultwarden, so `bw get password lazyplanner` works — Vaultwarden speaks the Bitwarden API). OS keyring rejected (daemon breaks headless Pi, extra dep + failure modes)
- **Conflicts**: ETag-based detection with conditional writes — never silently overwrite either direction; true conflicts keep both versions, flag the item, and surface a UI indicator for resolution at leisure (pick winner or keep both). "Newest wins" / "server wins" rejected as silent data-loss paths
- **Triggers**: manual `:sync` + all three automatic: background sync on startup (open instantly from cache), periodic while open (default 15 min, 0 = off), debounced push a few seconds after local edits
- `main.md`: three sync decisions added to Settled Decisions; Open Decisions down to data model details + UI design
- `CLAUDE.md`: sync summary line added to Project Context

---

## 2026-07-04 — Default config values set to owner's preferences

- Principle recorded in `main.md`: all moderate-scope options stay configurable in the config file; the *default value* of each option (when unset) is the owner's preference, so a working config needs only the `[server]` section (the one unavoidable first-run edit). Initially phrased as "hardcoded defaults" — corrected after owner feedback: reducing needed edits must not reduce config capability
- Defaults chosen: week starts Monday, 12-hour time display (2:30pm), month view on open, US date format (07/04/2026), sync all calendars with server names/colors

---

## 2026-07-04 — Config editing model; calendar metadata is server-owned

- **Config editing**: hand-edit + read-once-at-startup; the app never writes the config file. Planned conveniences: first-run generation of a fully-commented default config, and a `:config` command (open in `$EDITOR`, reload on exit). Auto-reload/file-watching explicitly rejected. App-remembered state (e.g., last view) goes in a state file under the data dir, not config.
- **Calendar metadata**: resolved the apparent conflict between "app never writes config" and "rename/recolor/create calendars in-app" — calendar identity, display name, and color are CalDAV properties owned by the server (cached in the vdir via sidecar convention), so in-app changes go through sync, not the config file, and propagate to NextCloud web/other clients. Config `[[calendars]]` sections demoted to optional local overrides (hide locally, override color locally); default is sync-everything with server names/colors. New calendars: CalDAV make-calendar call, with create-in-NextCloud-web as fallback if go-webdav lacks client support (verify at build time).
- `main.md`: config settled-decision entry updated (overrides, not definitions); two new settled decisions added (config editing model, server-owned calendar metadata)
- `CLAUDE.md`: config context line updated with the never-writes-config rule

---

## 2026-07-04 — Config decision: TOML, moderate scope; runtime paths; Windows as secondary target

- **Config file**: TOML via `BurntSushi/toml`, moderate scope — server connection, calendar selection/colors/visibility, appearance/behavior options (first day of week, default view, date/time formats, sync interval). Keybindings hardcoded for now; schema leaves room for a future `[keys]` section. Rejected: INI (no standard spec), YAML (footgun spec + heavy dep), JSON (no comments)
- **Platform scope**: Linux is primary (features tailored to it); Windows is a secondary compatibility build. All path resolution through one OS-aware helper (`os.UserConfigDir` + data-dir helper)
- **Runtime file locations** documented in `main.md`: config at `~/.config/lazyplanner/config.toml`, calendar data at `~/.local/share/lazyplanner/calendars/` (XDG data, NOT ~/.cache — offline edits live there, never disposable); Windows equivalents `%APPDATA%` / `%LOCALAPPDATA%`
- `main.md`: platform line updated, BurntSushi/toml added to libraries, config decision in Settled Decisions, Runtime File Locations section added under Architecture, Open Decisions now: sync details → data model → UI design
- `CLAUDE.md`: config + runtime paths line added to Project Context

---

## 2026-07-04 — Drafted: architecture, build plan, housekeeping

- `main.md` Architecture section: idiomatic Go layout (`cmd/lazyplanner/` entry point, `internal/{config,model,store,caldav,sync,ui}`, committed `vendor/`, tests beside code with `testdata/` fixtures) with separation rules — only `ui` imports tview; `model` does no I/O; `store` owns the cache dir; `caldav` owns HTTP. Note added explaining why Go doesn't use src/lib/include/test dirs.
- `main.md` Build Plan: 13 incremental steps — scaffold → model → recurrence → vdir store → CalDAV one-way import (early, to validate against real NextCloud data) → read-only UI shell → calendar views → editing → two-way sync (completes the must-have) → command mode → recurrence editing → background sync → Raspberry Pi target
- `main.md` housekeeping: module path `github.com/littekge/LazyPlanner` (matches GitHub remote), Go minimum = stable at scaffold time bumped only deliberately, license MIT (proposed, pending confirmation)
- `CLAUDE.md` Architecture Rules section filled in with the hard separation rules + "code is hand-edited by the user; keep it conventional and boring"
- Open Decisions reordered: config file (in discussion) → sync details → data model → UI design

---

## 2026-07-04 — Cache storage decision: vdir-style raw .ics files

- Chose **vdir-style raw `.ics` files** for the offline-first local cache: one file per event/todo, one directory per calendar (vdirsyncer/khal convention), JSON sidecar for sync state (ETags, sync tokens), in-memory index built at startup
- Reasons: 1:1 mapping onto CalDAV resources keeps sync logic simple, zero extra dependencies (pure Go, easy Pi cross-compile), human-readable/greppable when debugging sync
- Rejected: SQLite (cgo driver breaks cross-compile, pure-Go driver is a huge vendored dep, indexed queries unneeded at personal scale, binary format not inspectable); custom JSON (lossy translation away from native iCalendar format, breaks file-per-resource sync correspondence)
- `main.md`: decision added to Settled Decisions; Open Decisions rewritten as an ordered list of next decisions (architecture/package layout, build plan, sync design details, data model details, UI design, config file, housekeeping)
- `CLAUDE.md`: local cache rule added to Project Context (.ics files are the local source of truth)

---

## 2026-07-04 — TUI library decision: tview

- Chose **tview** (`rivo/tview` on `tcell`) over Bubble Tea and gocui:
  - tview: years of backwards compatibility, widgets (Table/Grid/Flex/InputField/Pages) that fit calendar/task views, k9s proves the target UX (`:` command mode, single-key shortcuts, mouse, panes)
  - Bubble Tea rejected: v2.0.0 (released 2026-07-03) is a breaking major version that also moved the module path to the vanity domain `charm.land` — churn profile conflicts with the robustness requirement
  - gocui rejected: original unmaintained; the active fork is tailored to lazygit and was recently absorbed into lazygit's own repo
- `main.md`: framework line filled in, decision + reasoning added to Settled Decisions, TUI item removed from Open Decisions
- `CLAUDE.md`: platform line updated with tview

---

## 2026-07-04 — Coding standards: Go conventions filled in

- `CLAUDE.md` "Other Conventions" section written: gofmt/goimports, `go vet` + `staticcheck` as the only linters, **vendored dependencies** (`vendor/` committed; `go mod tidy && go mod vendor` after dep changes; stdlib preferred), error wrapping with `%w` and no-panic policy, no global mutable state, Go naming + godoc comments on exports, `context.Context` on all I/O (UI must never block on network), table-driven tests with stdlib `testing` only, named constants over magic numbers
- `CLAUDE.md` workflow step 4 updated to include vet + staticcheck alongside tests
- Decisions made: vendoring chosen for build-forever robustness; staticcheck chosen over golangci-lint (less tooling churn) and over vet-only (better bug-finding)

---

## 2026-07-04 — Language decision: Go; offline-first CalDAV sync model

- Chose **Go** as the implementation language, driven by four requirements: lazygit-style interactive TUI, long-term robustness (static binary, Go 1 compatibility promise), CalDAV sync with an existing NextCloud server (the must-have feature), and speed on modest hardware (future Raspberry Pi terminal). Rust was runner-up; Python ruled out on robustness/speed.
- Chose **offline-first sync**: local cache is the working copy, NextCloud CalDAV server syncs in background/on demand.
- `main.md`: filled in language and libraries (`emersion/go-webdav`, `emersion/go-ical`, `teambition/rrule-go`; TUI lib TBD — Bubble Tea vs tview), added CalDAV sync as the top core feature, expanded design goals (`:` command mode, mouse, static-binary robustness, Pi target), added Settled Decisions section
- `CLAUDE.md`: platform line updated to Go, workflow test/run commands filled in (`go test ./...`, `go build ./...`), comment example converted to Go
- No code yet — next open decisions: TUI library, local cache storage format

---

## 2026-07-04 — Initial project structure: spec, log, and project rules

- `main.md` (new): minimal starting spec — project identity (language/libraries/license TBD), lazygit-inspired TUI description, initial core features (todo management, calendar views, recurring tasks/events), open decisions list
- `log.md` (new): change log initialized with this format
- `CLAUDE.md` (new): project context, iterative build workflow (test/run commands TBD), coding standards with Comment Rules (rest TBD), log entry format, architecture rules placeholder
- No code or tests yet — spec development is the next step

---
