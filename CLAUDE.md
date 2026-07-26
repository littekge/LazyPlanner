# Claude Code — Project Instructions

> How to work on this project. These rules apply to every task. What the project *is* lives in `main.md`.

---

## What This Project Is

LazyPlanner — a terminal TUI (lazygit-inspired) calendar + todo client written in Go, syncing two-way with a CalDAV server (NextCloud). TUI via `rivo/tview` on `tcell`; CalDAV via `emersion/go-webdav` + `emersion/go-ical` + `teambition/rrule-go`; config via `BurntSushi/toml`. Offline-first: a vdir-style cache of raw `.ics` files is the local source of truth. Linux is the primary target (incl. a Raspberry Pi dedicated terminal); Windows is secondary.

That is all the project context this file carries. What the program does, every design decision, and the current phase live in `main.md`.

---

## Session Startup

On first reading this file:

1. **Read `main.md`** — the spec: what the program is, all design decisions, the versioned Build Plan, the current phase.
2. **Read `log.md`** — the change log. On startup **READ ONLY** the 10 most recent entries; parse older logs later if needed.
3. **Read `notes.md`** — in-progress work. Normally empty; if it isn't, a previous session ended mid-task — pick that work up (or explicitly hand it back to the owner) before starting anything new.
4. **Confirm the branch**: `git branch --show-current` must show `ai-workspace` (or a branch off it).
5. Give the user a short summary of the most recently completed task and the recommended next steps.

---

## The Documents

Each document has exactly one role; the maintenance rule attached to each is what keeps it from drifting. Style rules for **all** documents:

- **Avoid long and wordy paragraphs** — write short, descriptive paragraphs. Long sections can ALMOST ALWAYS be broken into smaller paragraphs or lists.
- **Generously use headings** to group related sections.
- **Use lists** to break down longer paragraphs.

### `main.md` — WHAT (the master spec)

The single source of truth for the build: project identity, the complete design (all nitty-gritty detail lives here), all settled decisions, and the versioned Build Plan.

- **Maintenance**: when behavior or a design decision changes, update main.md in the same increment, and update **in place** — a decision is rewritten, never left standing next to a newer one that nullifies it. Project history belongs in `log.md`; the one exception is the Build Plan, which deliberately records completed versions as history.
- Design detail goes in main.md, never duplicated into CLAUDE.md or the README. New feature work is planned as a new `### v1.x.0` Build Plan subsection *before* implementation begins.

### `CLAUDE.md` — HOW (this file)

Agent orientation: workflow, rules, and architecture guardrails, with minimal project context.

- **Maintenance**: update only when the way of working fundamentally changes — a new tool or workflow, a changed rule, a new hard-won guardrail.
- **Nothing here may describe the current build state.** Project state or design detail belongs in `main.md`; audit archaeology (which pass found what) belongs in `docs/audit/passes/`. A guardrail here is written **rule → mechanism → pointer**, never as a pass report.

### `README.md` — the end-user guide

For a user of the program: what it does, build/install instructions, usage, keybindings. **Maintenance**: update whenever user-visible behavior, usage, or build steps change — same increment. It never carries project history, version narrative, build-plan status, or development internals; a curious user reads `main.md` for those.

Section structure (in order; indentation = heading levels):

- **LazyPlanner** — ~1-2 sentences describing the project.
  - **What it does** — bulleted description of the key features.
  - **Configuration** — how the configuration file works.
  - **Usage** — general; must **NOT** conflict with the keybindings section. Keep it short; omitting obscure or advanced behaviors for readability is fine.
    - **Managing Calendars** — how calendar addition/deletion works.
    - **Keybindings** — table of all valid keystrokes and what they do.
  - **Syncing** — the program's online syncing behavior.
  - **Build and Install** — per-target subsections: **Linux** (primary), **Windows**, **Raspberry Pi**.
  - **Development** — points to `main.md`, `log.md`, etc.
  - **License** — link to the project license.

**Keep it tight — rules that fight the drift this file is prone to:**

- **The keybindings table is the canonical key reference; prose must not re-narrate it.** Usage prose covers only what a key list *can't* — the pane/overview→center→detail model, drilling, folders, the mode badge, quick-add tokens, type-locking. It must not walk key-by-key through bindings the table lists. If you're describing a keystroke the table already has, delete the prose or move the *concept* it explains into the table's Action cell.
- **Prefer short sentences and bullet lists over long, parenthetical-laden run-ons.** A sentence listing several behaviors with nested parentheticals should be a lead line plus a bullet list. One idea per sentence.
- When user-visible behavior changes, update the **table row first**, then add prose only if a *concept* (not a keystroke) needs explaining.

### `log.md` — the change log

Append an entry **every time you make a change**, newest at the top:

```markdown
## YYYY-MM-DD — Short Title

- What was done (bullet points)
- Files created or modified
- Tests added or updated
- Any issues encountered
```

**Every entry gets its own `## YYYY-MM-DD — Title` heading — no exceptions.**

- One entry per distinct group of changes, each with its own heading, even when several land on the same day or in the same session. Never append bullets under an existing entry's heading; never leave an entry as a bare `---`-separated block without a heading.
- Insert new entries at the top, directly below the intro blockquote. Do not touch the previous entry — its heading and content must remain byte-identical.
- After editing `log.md`, verify: the number of `##` headings must equal the number of entries.

### `notes.md` — in-progress task state (short-term memory)

Working state for a task interrupted mid-arc: what's in progress, remaining steps, blockers, temporary context. **Maintenance**: the healthy steady state is **empty** — write only when a session ends mid-task, and date every entry. Delete a task's notes in the same increment that writes its `log.md` completion entry. A note surviving more than a few sessions is a misplaced main.md fact — move it. Never design decisions, never completed work.

### `docs/audit/` — the hardening-audit record

`PROTOCOL.md` (the audit rules — **read before running an audit**), `COVERAGE.md` (the living coverage ledger — **keep it current**; it drives which surfaces the next audit targets), and `passes/PASS-N.md` (the full per-pass reports, and the home of all audit history).

### `examples/Spec_Examples/`

Spec files from a prior project, structural reference only — not project rules. Read-only.

---

## Workflow

1. **Session startup** (above): read `main.md` + `log.md` + `notes.md`, confirm the branch.
2. **Work in small increments** — one module, feature, or fix at a time.
3. **After every change**, append a dated entry to `log.md`.
4. **Run tests and lints** after every code change: `go test ./...`, then `go vet ./...` and `staticcheck ./...`
5. **Run the program** to verify it builds and launches: `go build ./...` (and `go run ./cmd/lazyplanner` for manual checks). A `Makefile` wraps the common tasks — `make build`, `make check` (the full gate), `make cross` (stripped Raspberry Pi arm64/armv7/armv6 binaries into `dist/`, also run in CI).
6. **Keep `main.md` and `README.md` current** — a design change updates main.md; a user-visible change updates the README — in the same increment.
7. **Commit often** with descriptive messages: `git add . && git commit -m "feat: ..."` — on `ai-workspace`, never `main`.
8. **Session end**: run `/cleanup` (`.claude/commands/cleanup.md`) — sweep residual worktrees/branches/scratch, verify every doc is current, record any mid-arc task in `notes.md`, then commit and push to `ai-workspace`.

---

## Versioning

The project is versioned via GitHub tags/releases, `vX.Y.Z`:

- **Major (`vX.0.0`)** — multiple large features, large breaking changes, or major refactoring.
- **Minor (`v0.X.0`)** — a single large feature, moderate refactoring, additions to existing features, or a large group of bug fixes.
- **Hotfix (`v0.0.X`)** — targeted bug fixes only; no new features, no sweeping patches.

Every permanent feature or fix eventually becomes part of a versioned release. The user manually manages releases and tags — **NEVER** edit or add GitHub tags without the user's explicit permission. The user defines the current version you work on.

## Git Branching Rules

- **`ai-workspace` is Claude's branch.** All Claude work — commits, experiments, build steps — happens on `ai-workspace` or branches created off it. Feature/experiment branches off it are fine; merge them back when done.
- **NEVER merge to `main`. NEVER commit to `main`.** Merging `ai-workspace` into `main` is the owner's action, after review — no exceptions, even if asked to "finish up" or "ship it."
- **`ai-init` is frozen.** It preserves the workspace immediately before build step 1 (spec complete, no code). Never commit to it — it is a permanent reference point / reset target.

---

## Hardening Audits

Deep audits run through a coverage-first workflow (`.claude/workflows/hardening-audit.js`, launched with `/audit`): least-audited surfaces from `COVERAGE.md` → method-diverse audits → adversarial verification with a runnable repro → mutation canaries → bounded *residual risk*, never "clean". Rules of engagement:

- Read `docs/audit/PROTOCOL.md` before an audit; keep `COVERAGE.md` current afterwards.
- **Treat a workflow's own summary as unverified until checked** — confirm claimed repros and commits actually exist before relaying them.
- Every confirmed finding is fixed **repro-first**: a failing test demonstrating the bug, then the fix, then the test goes green and stays as a regression guard — one commit per fix, full gate every commit.
- **Recurring class → codify the rule.** When findings share a root cause that is a coding *practice* (not a one-off bug), the fix is not complete until the banned practice / required pattern is added to Hard-won guardrails below, in the same increment. Tests keep existing code from regressing; the guardrail keeps future code from repeating the practice — written **rule → mechanism → pointer**, with the pass narrative left in `docs/audit/passes/`.

---

## Coding Standards

### Comment Rules

- **Rule 1 — Names explain *what***: choose clear, descriptive names. A good name needs no comment.
- **Rule 2 — Code explains *how***: code should be readable enough to show how things work. Don't restate it.
- **Rule 3 — Comments explain *why***: comment only when the reason isn't obvious — *why* this approach, *why* a workaround exists, *why* a non-obvious value is used.

```go
// BAD — restates what the code does
count := 0 // set count to zero

// GOOD — explains why
count := 0 // Reset per sync cycle; the running total lives in syncState.TotalSynced
```

### Other Conventions

- **Formatting**: `gofmt` is law — format before committing. `goimports` ordering (stdlib → third-party → project, blank-line separated).
- **Linting**: `go vet ./...` and `staticcheck ./...` must pass after every code change. No other linters. staticcheck is a dev tool, not a vendored dependency — `go install honnef.co/go/tools/cmd/staticcheck@latest` if missing.
- **Dependencies are vendored** in `vendor/`, committed. After adding or updating one, run `go mod tidy && go mod vendor` and commit the result. Prefer the standard library; every new third-party dependency needs a reason (robustness first — fewer deps, fewer breakages).
- **Error handling**: check every error. Wrap with context when propagating: `fmt.Errorf("syncing calendar %q: %w", name, err)`. No `panic` outside truly unrecoverable startup failures; the TUI must never crash on a bad server response or malformed `.ics` — degrade gracefully and surface the error in the UI.
- **No global mutable state**: pass dependencies explicitly through constructors and parameters. Package-level `const` and immutable lookup tables are fine; package-level `var` holding mutable state is not.
- **Naming**: standard Go style — `MixedCaps`, no underscores; short names for short scopes, descriptive for wide. Export only what another package needs.
- **Doc comments** on all exported identifiers, godoc style (start with the identifier's name).
- **Contexts**: all network and I/O-bound operations (CalDAV sync above all) take a `context.Context` first, so they can be cancelled — the UI must never block uninterruptibly on the network.
- **No magic numbers**: named constants for anything with meaning; user-facing tunables go in the TOML config file.

### Tests

- Standard `testing` package only (no assertion frameworks). Prefer table-driven tests.
- Core logic (sync, recurrence, parsing) gets tests; thin UI glue may go without.
- Concurrency fixes get a real goroutine stress test under `go test -race` (see `TestConcurrentSyncAndEditsRace`).
- The iCalendar ingest boundary has **native Go fuzz targets** in `internal/model/fuzz_test.go` — `FuzzDecode`/`FuzzEventOccurrences`/`FuzzBuildTree`/`FuzzParseQuickAdd`/`FuzzRecurrenceMutations`. The seed corpus (every `f.Add` case plus saved crashers under `internal/model/testdata/fuzz/`) runs as deterministic tests on the normal gate; `go test -fuzz=Fuzz... ./internal/model/` explores new inputs. **Extend these rather than starting a parallel harness** when hardening a parser path.
- An **opt-in live CalDAV suite** sits behind `//go:build live` (`internal/sync/live_test.go`), excluded from `make check`. Run it only against a **test account**: `go test -tags live -run TestLive ./internal/sync/ -v`. It creates and deletes its own throwaway calendars and never touches existing ones.

---

## Architecture Rules

See `main.md` for the full package layout. The hard rules:

- **Only `internal/ui` imports tview/tcell.** Every other package compiles and tests headlessly.
- `internal/model` — pure types and logic, **no I/O** (no filesystem, no network).
- `internal/ui` never touches disk or network directly — it goes through `store` and `sync`.
- `internal/store` is the only package that reads/writes the cache directory; `internal/caldav` is the only package that speaks HTTP.
- `cmd/lazyplanner/main.go` is thin wiring only — no logic.
- Tests live next to the code (`foo_test.go`); fixtures in per-package `testdata/` dirs.
- The user hand-edits this code too: keep the structure conventional and boring, prefer obvious code over clever code.
- **Never hand-edit `vendor/`** — it's silently reverted by `go mod vendor`. Fix library bugs in our own code, or (if unavoidable) via a `replace` directive.

### Hard invariants

Cross-cutting rules the design depends on. Weakening one needs the owner's sign-off, and any deliberate change to their hardening must update `docs/audit/COVERAGE.md`:

- **Iron rule**: never drop or mangle iCal properties the app doesn't understand — editing a known field preserves everything else.
- **The `.ics` files are the local source of truth** — never introduce a second store that can drift from them.
- **Sync never silently overwrites** in either direction — a true conflict keeps both versions and flags the item.
- **The app never writes the config file** — app-remembered state goes in the state file under the data dir; calendar names/colors are server-owned CalDAV data, not config.
- **Read-only calendars are never written to.**

---

## Hard-won guardrails

Each is a banned practice or required pattern, backed by regression tests. Don't reintroduce. Audit history — which pass found what — lives in `docs/audit/passes/`.

### tview freeze traps

- **Never call an app-lock method (`a.tv.GetFocus()`, etc.) from a `SetDrawFunc`/draw path.** `Application.draw()` holds the write-lock and `RWMutex` isn't reentrant, so it self-deadlocks. Read tracked plain fields instead — the mode indicator's `interactionMode` derives from `a.grabbing` + `a.gridDrilled()`, taking no app lock.
- **The task tree runs with `SetGraphics(false)`** — tview v0.42.0 `TreeView.Draw` infinite-loops when a node's indent exceeds the pane width. Leave graphics off; nesting shows via indentation + ▸/▾ carets.
- **Extend `internal/ui/displaystress_test.go` when adding a widget or draw path** — it drives every custom `Draw` path with display-hostile content across 1×1→400×150 geometries under panic-recover + watchdog, so a new freeze/panic is caught on the normal gate.
- Guards: `internal/ui/{modedeadlock,treedraw_regress,displaystress}_test.go`.

### Every selectable list must carry `selectionStyle` — dropdowns included

- **Rule**: every selectable widget sets the theme-adaptive shared `selectionStyle` (`tcell.StyleDefault.Reverse(true)`) and gets a reverse-video regression test — `tview.DropDown`s too, not just `tview.List`s.
- **Mechanism**: the app sets `tview.Styles.PrimitiveBackgroundColor = tcell.ColorDefault` (`useTerminalTheme`), so tview's *default* selected style — terminal-default foreground on a `PrimaryTextColor` (white) bar — renders white-on-white.
- A `List` uses `SetSelectedStyle(selectionStyle)` (`calendars`/`tasklists`/`agendaList`, the conflicts list, the `:account` picker); a `DropDown`'s embedded list uses `SetListStyles(tcell.StyleDefault, selectionStyle)`, done once in `caretForm.addDropDown` so every form dropdown inherits it. **A dropdown created outside `addDropDown` must set it explicitly.**
- Guards: `TestSelectionIsLegible`, `TestAccountPickerSelectionIsLegible`, `TestDropDownSelectionIsLegible` (`internal/ui/{app,account,recurbugfix}_test.go`). Reappeared twice.

### A modal nested over another modal must not restore focus to the calendar

- **Rule**: `captureFocus` records the calendar drill state **only when no modal is already open** (`a.mode == modeCalendar && !a.modalOpen()`) — the sole case where the covered focus is the calendar grid. A nested modal's captured `focusState` must point at the *outer modal*.
- **Mechanism**: capturing calendar drill state for a nested modal makes `restoreFocus` re-drill and teleport to the calendar on close, stranding the outer form open-but-unreachable (a softlock).
- Guard: `TestNestedModalOverDrilledCalendarKeepsFormFocus` (`internal/ui/recurbugfix_test.go`).

### Scale invariants

Four hot paths must stay linear:

- Recurrence expansion (`Event.Occurrences` via `safeBetween`) is **bounded** — a pathological rule can't hang the UI.
- `BuildTree` classifies cycles by memoized parent-chain, **never** a per-insert subtree walk.
- `LayoutDay`'s overlap-lane packing is a **sweep line over two heaps** — **never** a first-fit scan over `laneEnds` (Θ(n²) when everything overlaps).
- A bulk pull uses `store.PullRemoteBatch` — one sidecar write per calendar, not per resource.

Attached rules:

- **Bounding what a stage *produces* does not bound what the next stage *does with it*.** When you bound one stage, check the whole pipeline to the pixel: expansion → layout → the UI's own per-item scans.
- `LayoutDay`'s packing is rendering-visible, so a faster algorithm must produce **identical** lanes. `TestLayoutDayMatchesFirstFitReference` keeps the naive first-fit and diffs against it — a deliberate semantics change must update that tripwire.
- The batch is **pull-only and single-lock**: **never route a push through it** (a crash mid-batch would duplicate a create). Keep the reconcile rule that a clean, href-less local resource is a pull orphan to re-pull, not a create to push.
- Guards: benchmarks in `internal/model/scale_test.go` + `internal/sync/scale_bench_test.go`.

### Malformed iCalendar is contained and healed at ingest, never fatal

**Rule**: no decode or expand path may panic the app, and `model.Parse` must leave every ingested object in an encodable shape.

- **Panic containment**: go-ical's decoder and rrule-go's iterator **panic** on some malformed input. Recover guards sit at the byte→calendar boundaries — `model.decodeCalendar`, `safeBetween` in `Event.Occurrences`, `internal/caldav`'s `guardICalPanic` around `QueryCalendar`/`GetCalendarObject`. **Never add a decode/expand path outside a guard.**
- **Healing**: go-ical's decoder is more tolerant than its encoder, so `model.Parse` heals on ingest (`ensureDTStamp`/`ensureCalendarProps`/`dedupeSingleValued`/`sanitizePropValues`/`stripForbiddenNesting`/`healComponentConstraints`/`dropUnusableTimezones`); otherwise a foreign or hand-edited `.ics` loads but can't be edited or saved.
- Heals are **add-only-when-missing and never mangle existing props** (iron rule). Sole exception: an unencodable component with no usable data may be dropped rather than brick the resource (`dropUnusableTimezones`) — owner-approved, not the default.
- **Mirror go-ical's *full* encoder rules at its *recursion depth***: `vendor/.../go-ical/encoder.go` `checkComponent` (exactlyOne/atMostOne) plus `model.singleValuedProps`. `checkComponent` runs on **every component at every depth** (`encodeComponent` recurses before emitting), so a phantom component nested anywhere bricks the whole resource.

`ensureDTStamp`/`healComponentConstraints` are **top-level-only**; the safety net is `model.allowedChildren`, driving the recursive `stripForbiddenNesting`:

- **Deny-by-default, restricted to the types go-ical validates.** A type with no map entry admits no child named in `model.encoderValidatedComponents`, and keeps every other child untouched. Entries only *permit* legal nesting — VEVENT/VTODO (VALARM), VTIMEZONE (STANDARD/DAYLIGHT), childless VJOURNAL/VFREEBUSY/VALARM/STANDARD/DAYLIGHT. X-\*, VAVAILABILITY and every future type stay unknown containers, policed without being listed.
- **Never invert to allow-by-default**: `encodeComponent` recurses into *every* child regardless of name, so enumerating containers is unsatisfiable — the map cannot name a type nobody has invented yet.
- **Never widen the strip to all children of an unknown container**: `checkComponent` has **no default case**, so a component outside `encoderValidatedComponents` always returns nil — it *cannot* brick the resource, and stripping it would destroy real data (an RFC 7953 VAVAILABILITY's AVAILABLE children, a vendor X- payload) for nothing.
- Membership in `encoderValidatedComponents` = "can fail an encode", mirroring `checkComponent`'s switch cases (VALARM included, against its `// TODO` case gaining rules). That is why it is the strip criterion.
- A nested VCALENDAR is special-cased inside `stripForbiddenNesting` (a VCALENDAR is the object, never a component in one).
- Policed this way, no VEVENT/VTODO/VJOURNAL/VFREEBUSY survives below the top level, so the top-level heals miss nothing nested.

**On a new ingestable component type or a go-ical bump**: re-diff `singleValuedProps`, the DTSTAMP-heal set **and `encoderValidatedComponents`** against `checkComponent`, and confirm `allowedChildren` still only *permits* (never gates). A decode-but-can't-re-encode gap is a **HIGH** — one bad component makes the whole resource, valid siblings included, unsavable.

**Accepted costs**: a *valid* VEVENT/VTODO under an unknown container is still stripped (never addressable — `Parse` walks only direct children); a missing **UID** is deliberately not healed (a fabricated UID churns sync identity).

Guards: `internal/model/{fuzz_test,harden_ingest_test,vjournal_encode_test,malformed_vtimezone_test,nested_heal_test,unknown_container_test}.go`, `internal/caldav/guardpanic_test.go`. Reopened four times.

### Concurrent UI writes are version-checked

- **Rule**: every UI write to an *existing* resource routes through `store.PutIfUnchanged` against its located `Prev`. **Never a bare `Locate→Put`** — it silently clobbers a concurrent sync pull. A multi-write operation either rolls back or skips cleanly when a later write fails.
- A bare `store.Put` is correct *only* for a fresh create (a new resource under a UID/name minted in that same call) and **must** carry a `// create: fresh UID, no existing resource to clobber` comment (or equivalent, e.g. a fresh `(calID, name)` pair) marking it deliberate.
- **When adding or touching any write path**: (1) classify it as an existing-resource rewrite or a fresh create; (2) route the former through `PutIfUnchanged` following the `applyMutation`/`reparentOps` pattern, mark the latter with that comment; (3) grep `internal/ui` for bare `store.Put(` while you're in there — don't assume a prior sweep caught every call site.
- **Cross-collection subtree moves**: move each member from its OWN `loc.CalID`, never a hard-coded source, and rewrite in place — not create-then-delete — when it already lives in the destination.
- Guards: `TestApplyMutationDoesNotClobberConcurrentPull` (`internal/ui/editclobber_test.go`) plus per-site clobber guards `internal/ui/{reparent_clobber,grab_split_clobber,recur_edit_clobber,movesubtree_clobber,movesubtree_crosscoll}_test.go`. Reopened three times.

### A sync-reconcile "gone on the server" signal must be guarded by pointer identity

The sibling of the rule above, on the **sync side** (`internal/sync`, `internal/store` reconcile writes) rather than UI writes.

- **The trap**: reconcile snapshots resource **pointers** (`Calendar()`); a concurrent UI edit builds a *new* `*Resource` and swaps the map entry (`stageResourceLocked`); the reconcile branch acts on the **stale** snapshot, Forgetting or overwriting the fresh edit with no conflict and no skip (a silent lost update).
- **Rule**: any reconcile path that removes or overwrites *because the server no longer has the resource* must (1) take the `expectedPrev`/pointer-identity guard — `store.PullRemote` / `PutIfUnchanged` for writes, **`store.ForgetIfUnchanged` for removals (never a bare `st.Forget` in reconcile)** — and (2) on a mismatch, raise `markConflict(serverDeleted=true)` against the surviving edit rather than dropping it.
- When adding or touching a reconcile branch, ask: "what if a UI edit landed on this name during the network I/O?" If the answer is a clobber, it's this bug.
- **Known blind spot**: `internal/model` and `internal/caldav` peer write paths are **not yet swept** for this class.
- Guards: `internal/store/commitpush_deletemidpush_test.go` (`TestCommitPushDoesNotResurrectDeletedResource`, `TestCommitPushDeleteRaceInvariant`), `internal/sync/tombstone412_undo_race_test.go` (`TestTombstone412ResurrectDoesNotClobberConcurrentUndo`), `internal/sync/stepa_forget_clobber_test.go` (`TestReproStepAForgetClobbersConcurrentEdit`). Reopened three times.

### Moving a recurring item's anchor must re-anchor its day-pinning `BY*`

- **Rule**: never leave an anchor contradicting its own rule. Any path that day-shifts a recurring master re-anchors the day-pinning `BY*` — `model.ReanchoredRecurrence` for an event's `DTSTART`, `model.ReanchoredRecurrenceTodo` for a todo's `DUE` (both delegate to the shared `reanchoredRecurrence(raw, oldAnchor, newAnchor)`).
- **Mechanism**: a day-pinning rule (weekly `BYDAY`, monthly nth-weekday) fires independently of the anchor, so shifting the anchor alone puts it outside its own recurrence set — the series keeps firing on the old day and the moved instance vanishes. Weekly weekday sets shift as a whole; monthly nth-weekday re-derives.
- **Return contract**: `(nil,false)` = no rewrite needed (daily, plain weekly, monthly-by-day, yearly carry no day-pinning `BY*`); `(nil,true)` = **block the move** — the rule is outside the editable vocabulary (a *Custom rule (kept)*) and must not be corrupted.
- **`DUE` is the anchor for a recurring VTODO**, not just `DTSTART` for an event: a native recurring todo carries only `DUE`, which *is* its rule anchor. Every path that day-shifts a recurring todo's due — single grab `j`/`k`, bulk grab, any new one — must call `ReanchoredRecurrenceTodo(td, oldDue, newDue)` and block on `(nil,true)`.
- Guards: `internal/model/reanchor_test.go`, `internal/ui/grab_recur_reanchor_test.go` (event), `internal/ui/grab_todo_reanchor_test.go` (single grab, todo), `internal/ui/bulkgrab_recur_reanchor_test.go` (bulk grab, todo).

### A recurrence anchor and the rule it anchors must be authored in the same zone

- **Mechanism**: RFC 5545 evaluates a rule's `BY*` parts in its anchor's own zone. Deriving `BY*` from the user's local weekday while serializing `DTSTART`/`DUE` in UTC lets the two disagree whenever the local and UTC dates differ — the rule fires on the wrong day and drifts an hour across DST.
- **Required pattern**: write a recurrence anchor with `setAnchorDateOrTime` + `anchorZone` (`internal/model/edit.go`), **never** the plain `setDateOrTime`. Any writer that emits a TZID **must** call `ensureVTimezone` so the zone is *defined*, not just referenced.
- **Sweep siblings**: treat every new or touched anchor writer as suspect and grep for the others — `RewriteEventRule`, `NewSeriesFrom` and `DetachTodoOccurrence` were all missed once.
- **The gate is deliberately narrow**: write the TZID form only when this same call authors the rule, or the existing anchor already carries a resolvable TZID. **An edit that leaves the rule alone must never re-anchor** — re-interpreting `BY*` against a different zone silently MOVES an existing series.
- **A form's Repeat state must be seeded and resolved on the same anchor basis** (`newEventRepeat`/`newTodoRepeat`, `internal/ui/itemforms.go`). Seeding from the raw stored anchor while resolving against the form's local start makes an untouched dropdown report a rewrite.
- **`a.loc` (the display/authoring zone, `config.LocalZone()`) can diverge from `time.Local`** — every render path must read `a.loc`.
- Guards: `internal/model/{tzanchor,vtimezone}_test.go`, `internal/ui/{repeatanchor,tzanchor_uipath,vtimezone_writers_uipath}_test.go`. `TestNonRecurringEventStaysUTC` and `TestEditWithoutRuleChangeKeepsAnchorForm` (`internal/model/tzanchor_test.go`) are load-bearing and **must never be weakened**.

### RDATE/EXDATE are multi-valued and independent of the RRULE's COUNT/UNTIL bound

Any code touching recurrence must respect all three facts:

1. An `RDATE`/`EXDATE` line may carry several comma-separated values (and an `RDATE` may be `VALUE=PERIOD`), so resolve them **per value** (`resolveDateTimeValues`, `filterRDates`) — **never** `prop.DateTime` on the whole line, which errors and collapses the series to its base instance.
2. `COUNT` bounds the RRULE *generator*, so an `EXDATE`'d instance still consumes `COUNT`. Split/cap math counts RRULE *iterations* (`rruleIterationsBefore`), not the EXDATE-filtered *visible* set, or the future half gains a phantom occurrence.
3. `UNTIL` bounds only the RRULE, not `RDATE`s (rrule-go's `Set.Iterator` merges RDATEs independent of UNTIL). Capping or splitting a series must **partition RDATEs explicitly** (`filterRDates`), or a trailing RDATE lands in both halves.

Guards: `internal/model/{multivalue_dates,recur_split_exdate,recur_split_rdate}_test.go`.

### A regression test for a UI-reachable bug must drive the real entry point

- **Rule**: the guard test starts where the user does — build the real form, key handler or command, set the widgets, fire the button — and asserts on what gets **stored** or rendered, not on a helper's return value.
- **Mechanism**: hand-building a helper's inputs proves the helper's *arithmetic* and nothing about whether production ever supplies those inputs. A fix can land, go green, and never reach a user — the bug ships behind a passing test.
- **Whenever a helper takes a value the UI *derives*** (an anchor, a budget, a location, a selection), the test must exercise the derivation — that wiring is where this class hides.
- A direct-helper unit test may accompany the end-to-end guard but **never substitute for it**.
- When a fix is confined to a helper, ask: "who calls this in production, and does that call site produce the input my test used?" If the test can't answer, it isn't guarding the bug.
- Pattern to copy: `internal/ui/endsondate_uipath_test.go` (form → Repeat → Custom… → Ends on date → OK → `readEventDraft`/`readTodoDraft` → the stored object's occurrences), backing the helper-only `internal/ui/ends_on_date_test.go`. Confirmed in two consecutive passes.

### Text the app did not author must pass through `tview.Escape` at the call site

- **Mechanism**: tview parses `[...]` as a colour/region tag, so a calendar display name, task summary, typed query, parser warning, file path or server error containing a bracket run is silently **swallowed** from the status bar — the user sees a truncated or empty message with no sign anything was dropped.
- **Escaping centrally inside `flash`/`echo` is not possible**: `advanceRecurringTodo` (`recur_edit.go`) deliberately passes style tags, so a blanket escape would break intentional colour.
- Rules: (1) **escape at each concatenation boundary, not in the sink**; (2) `flashErr`/`failureMsg` is the funnel for the `"<Action> failed: " + err.Error()` sites — escaping there covers all of them, and **no funnel caller may pass intentional tags**; (3) **`%q` is not a substitute** — it quotes and backslash-escapes but leaves a `[` run intact and mangles embedded quotes; use plain quotes plus `tview.Escape`.
- When adding any flash/echo, ask: "did the app write every character of this string?" If not, escape it.
- Guards: `internal/ui/tagescape_sweep_test.go` (table-driven over every site, hostile `[red]`/`[white:black]`/`[""]`/`[-]` runs, plus a byte-identical assertion that escaping does not alter ordinary text) and `internal/ui/statusbar_tagescape_test.go`.

### A test must build its times in the zone the code under test uses

- **Mechanism**: `newApp` hard-codes `loc: time.Local`, so a test building `now`/`anchor`/due dates in `time.UTC` makes its own day window disagree with the day the app buckets locally-timed items into. The disagreement is **invisible at or west of UTC and fails east of it** — CI runs UTC, so the suite goes green while `make check` from an east-of-UTC zone reds out on clean code.
- **Rule**: build test clocks with `time.Local`, or pin an explicit zone and use it consistently on both sides of the assertion. Run `TZ=Asia/Kolkata go test ./...` before claiming a zone-sensitive change is done.
- **Recurrence and day-bucketing tests run over several zones** (`UTC`, a western and an eastern offset) — a single-zone test passes on half the planet when two bugs' signs flip with the offset.
- **A zone-parameterised test must be shaped so every zone can actually fail.** A fixture anchored at 23:00Z crosses the local day boundary only for offsets ≥ +1h, making other subtests structurally incapable of failing. Choose the fixture per zone — late-UTC-hour anchor for positive offsets, early for negative — and comment any deliberately non-failing zone as a control.
- **Never `t.Skip` on an unavailable zone in a load-bearing guard** — it silently makes the guard a vacuous pass. `internal/model` and `internal/ui` import `_ "time/tzdata"` in their zone tests so every named zone loads; a load failure is `t.Fatalf`, never `t.Skipf`.
- **When the input space is enumerable, enumerate it** — hand-picked zone samples miss near-midnight-transition defects that a sweep of the whole IANA database catches at once. This is a *verification* technique for when a property is cheap to check and the space is closed, not a requirement for every guard; the committed `TestObservanceRuleGeneratesItsOwnDTSTART` samples named zones instead.
- **Prove every new guard bites by mutation**: reapply the bug, confirm RED, revert, confirm GREEN. Vacuous zone/anchor tests have passed review before; only a mutation check catches them.
