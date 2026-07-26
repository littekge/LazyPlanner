# Claude Code — Project Instructions

> How to work on this project. These rules apply to every task. What the project *is* lives in `main.md`.

---

## What This Project Is

LazyPlanner — a terminal TUI (lazygit-inspired) calendar + todo client written in Go, syncing two-way with a CalDAV server (NextCloud). TUI via `rivo/tview` on `tcell`; CalDAV via `emersion/go-webdav` + `emersion/go-ical` + `teambition/rrule-go`; config via `BurntSushi/toml`. Offline-first: a vdir-style cache of raw `.ics` files is the local source of truth. Linux is the primary target (incl. a Raspberry Pi dedicated terminal); Windows is secondary.

That is all the project context this file carries. What the program does, every design decision, and the current project phase live in `main.md`.

---

## Session Startup

When reading this file for the first time:

1. **Read `main.md`** — the spec: what the program is, all design decisions, the versioned Build Plan (a compressed history of everything done), and the current phase.
2. **Read `log.md`** — the change log: what has been done recently. On startup **READ ONLY** the 10 most recent entries, but parse older logs later if needed.
3. **Read `notes.md`** — in-progress work. It is normally empty; if it isn't, a previous session ended mid-task — pick that work up (or explicitly hand it back to the owner) before starting anything new.
4. **Confirm the branch**: `git branch --show-current` must show `ai-workspace` (or a branch off it) — see Git Branching Rules.
5. Give the user a short summary of the most recently completed task and the recommended next steps.

---

## The Documents

Each document has exactly one role. The maintenance rule attached to each is what keeps it from drifting. The general style rules for ALL DOCUMENTS are as follows:

- **Avoid long and wordy paragraphs** -> document sections should be written as
  short, yet descriptive paragraphs of prose. Long sections can ALMOST ALWAYS be
  broken up into smaller paragraphs or lists.
- **Generously use headings** to group related sections.
- **Use lists** as needed to break down longer paragraphs.

### `main.md` — WHAT (the master spec)

The single source of truth for the build: project identity, the complete design (all the nitty-gritty detail lives here), all settled decisions, and the versioned Build Plan tracking what has and hasn't been done.

- **Maintenance**: when behavior or a design decision changes, update main.md in the same increment — and update **in place**: a decision is rewritten, never left standing next to a newer one that nullifies it (project history belongs to `log.md`; the one exception is the Build Plan, which deliberately records completed versions/steps as history).
- Design detail goes in main.md, never duplicated into CLAUDE.md or the README. New feature work is planned as a new `### v1.x.0` subsection under the Build Plan *before* implementation begins.

### `CLAUDE.md` — HOW (this file)

Agent orientation: workflow, rules, and architecture guardrails, with minimal project context.

- **Maintenance**: update only when the way of working fundamentally changes — a new tool or workflow is adopted, a rule changes, a new hard-won guardrail is discovered. Nothing here may describe the current build state. If you find yourself writing project state or a design detail into this file, it belongs in `main.md`.

### `README.md` — the end-user guide

For a user of the program: a basic summary of what it does, build/install instructions, usage, and a detailed description of the keybindings. **Maintenance**: update it whenever user-visible behavior, usage, or build steps change — in the same increment. It never carries project history, version narrative, build-plan status, or development internals; a curious user reads `main.md` for those.

The README should be grouped into a few sections (in order, list indentation
corresponds to heading levels):

- **LazyPlanner** -> top level heading. Contains a short description of the
   project (~1-2 sentences).
  - **What it does** -> longer, bulleted description of the key features of the
    program.
  - **Configuration** -> how the configuration file works.
  - **Usage** -> general description of how to use the app. Should **NOT**
  conflict with the keybindings section. Keep it short and general -- it's ok to omit obscure or advanced behaviors to improve readability.
    - **Managing Calendars** -> description on how calendar addition/deletion
    works.
    - **Keybindings** -> table of all valid keystrokes and a description of what
      they do.
  - **Syncing** -> description of the programs online syncing behavior.
  - **Build and Install** -> instructions on building and installing the program. Each target gets its own subsection:
    - **Linux** -> primary build target.
    - **Windows** -> cross-platform target.
    - **Raspberry Pi** -> cross-platform target.
  - **Development** -> points to development documents (`main.md`, `log.md`, etc.).
  - **License** -> link to project license.

**Keep it tight — rules that fight the drift this file is prone to:**

- **The keybindings table is the canonical key reference; prose must not re-narrate it.** Usage prose covers only what a key list *can't* — the pane/overview→center→detail model, drilling, folders, the mode badge, quick-add tokens, type-locking. It must not walk key-by-key through bindings the table already lists (which key cycles calendars, which zooms, etc.); when a key is purely mechanical, let the table carry it. If you're describing a keystroke the table already has, delete the prose or move the *concept* it explains into the table's Action cell.
- **Prefer short sentences and bullet lists over long, parenthetical-laden run-ons.** A sentence that lists four behaviors with nested parentheticals (a recurring failure mode here — e.g. the every-sync-trigger sentence, the whole-Calendars-pane sentence) should be a lead line plus a bullet list. One idea per sentence; split when a clause needs its own parenthetical.

When user-visible behavior changes, update the **table row first**, then add prose only if a *concept* (not a keystroke) needs explaining.

### `log.md` — the change log

Append an entry **every time you make a change**, newest at the top, in this format:

```markdown
## YYYY-MM-DD — Short Title

- What was done (bullet points)
- Files created or modified
- Tests added or updated
- Any issues encountered
```

**Every entry gets its own `## YYYY-MM-DD — Title` heading — no exceptions.**

- One entry per distinct group of changes, each with its own heading, even when several entries land on the same day or in the same session. Never append bullets under an existing entry's heading and never let an entry exist as a bare `---`-separated block without a heading.
- New entries are inserted at the top, directly below the file's intro blockquote. When inserting, do not touch the previous entry — its heading and content must remain intact and byte-identical.
- After editing `log.md`, verify the result: the number of `##` headings must equal the number of entries.

### `notes.md` — in-progress task state (short-term memory)

Working state for a task interrupted mid-arc: what's in progress, the remaining steps, blockers, and temporary context the next session needs. **Maintenance**: the healthy steady state is **empty** — write to it only when a session ends mid-task, and date every entry. When the task completes, delete its notes in the same increment that writes the `log.md` completion entry (resolution goes to `log.md`; nothing accumulates here). A note that survives more than a few sessions is a misplaced main.md fact — move it. Never design decisions, never completed work.

### `docs/audit/` — the hardening-audit record

`PROTOCOL.md` (the audit rules — **read it before running an audit**), `COVERAGE.md` (the living coverage ledger — **keep it current**; it drives which surfaces the next audit targets), and `passes/PASS-N.md` (the full per-pass reports). See Hardening Audits below.

### `examples/Spec_Examples/`

Spec files from a prior project, used as structural reference only — not project rules. Read-only.

---

## Workflow

1. **Session startup** (above): read `main.md` + `log.md` + `notes.md`, confirm the branch.
2. **Work in small increments** — one module, feature, or fix at a time.
3. **After every change**, append a dated entry to `log.md` (format above).
4. **Run tests and lints** after every code change: `go test ./...`, then `go vet ./...` and `staticcheck ./...`
5. **Run the program** to verify it still builds and launches: `go build ./...` (and `go run ./cmd/lazyplanner` for manual checks). A `Makefile` wraps the common tasks — `make build`, `make check` (the full gate), `make cross` (stripped Raspberry Pi arm64/armv7/armv6 binaries into `dist/`, also run in CI).
6. **Keep `main.md` and `README.md` current** — a design change updates main.md; a user-visible change updates the README (per The Documents above) — in the same increment.
7. **Commit often** with descriptive messages: `git add . && git commit -m "feat: ..."` — on `ai-workspace`, never `main` (see Git Branching Rules).
8. **Session end**: run `/cleanup` (`.claude/commands/cleanup.md`) — sweep residual worktrees/branches/scratch, verify every doc is current per The Documents, record any mid-arc task in `notes.md`, then commit and push everything to `ai-workspace`.

---

## Versioning

The project is versioned via Github tags/releases. The versioning convention
used in this project is as follows:

- **Structure:** vX.X.X versioning (e.g. v1.0.0 for a major release)
- **Major Version:** vX.0.0 — denotes a major release. A major release is
characterized by the addition of multiple large features, large breaking changes
to the codebase, or other major refactoring.
- **Intermediate Version:** v0.X.0 — denotes a minor release. A minor release
  may consist of the addition of a single large feature, moderate refactoring,
  additions to existing features, or large groups of bug fixes.
- **Minor Version:** v0.0.X — denotes a hotfix. A hotfix consists of only
  targeted bug fixes, no new features or major sweeping patches of bug fixes.

Every permanent feature or fix eventually becomes part of a versioned release.
The user manually manages releases and tags, **NEVER** edit or add Github tags
without the users explicit permission. The user defines the current version that
you work on.

## Git Branching Rules

- **`ai-workspace` is Claude's branch.** All Claude work — commits, experiments, build steps — happens on `ai-workspace` or on branches created off it. Feature/experiment branches off `ai-workspace` are fine; merge them back into `ai-workspace` when done.
- **NEVER merge to `main`. NEVER commit to `main`.** Merging `ai-workspace` into `main` is the owner's action, done by the owner after review — no exceptions, even if asked to "finish up" or "ship it."
- **`ai-init` is frozen.** It preserves the state of the workspace immediately before build step 1 (spec complete, no code). Never commit to it — it exists as a permanent reference point / reset target.

---

## Hardening Audits

Deep audits run through a reusable, coverage-first workflow (`.claude/workflows/hardening-audit.js`, launched with the `/audit` command): it picks the least-audited surfaces from `docs/audit/COVERAGE.md`, fans out method-diverse audits, verifies each finding adversarially with a runnable repro, runs mutation canaries that test whether the suite catches injected bugs, and reports bounded *residual risk* — never "clean". Rules of engagement:

- Read `docs/audit/PROTOCOL.md` before an audit; keep the `COVERAGE.md` ledger current afterwards.
- **Treat a workflow's own summary as unverified until checked** — confirm claimed repros and commits actually exist before relaying them.
- Every confirmed finding is fixed **repro-first**: a failing test demonstrating the bug, then the fix, then the test goes green and stays as a regression guard — one commit per fix, full gate every commit.
- **Recurring class → codify the rule.** When findings share a root cause that is a coding *practice* (not a one-off bug) — especially one seen across multiple passes — the fix is not complete until the banned practice / required pattern is added to Hard-won guardrails below, in the same increment. Tests keep existing code from regressing; the guardrail keeps future code from repeating the practice.

---

## Coding Standards

### Comment Rules

Follow three rules for comments:

- **Rule 1 — Names explain *what***: Choose clear, descriptive names for classes, methods, and variables. If the name is good enough, no comment is needed to explain what it does.
- **Rule 2 — Code explains *how***: The code itself should be readable enough to show how things work. Don't write comments that restate the code.
- **Rule 3 — Comments explain *why***: Only add comments when the reason behind a decision isn't obvious from the code. Explain *why* this approach was chosen, *why* a workaround exists, or *why* a non-obvious value is used.

```go
// BAD — restates what the code does
count := 0 // set count to zero

// GOOD — explains why
count := 0 // Reset per sync cycle; the running total lives in syncState.TotalSynced
```

### Other Conventions

- **Formatting**: `gofmt` is law — all code is formatted before committing. Use `goimports` ordering for imports (stdlib → third-party → project, separated by blank lines).
- **Linting**: `go vet ./...` and `staticcheck ./...` must pass after every code change. No other linters. staticcheck is a dev tool, not a vendored dependency — install with `go install honnef.co/go/tools/cmd/staticcheck@latest` if missing.
- **Dependencies are vendored**: all dependency source lives in `vendor/` and is committed. After adding or updating a dependency, run `go mod tidy && go mod vendor` and commit the result. Prefer the standard library; every new third-party dependency needs a reason (robustness first — fewer deps, fewer breakages).
- **Error handling**: check every error. Wrap with context when propagating: `fmt.Errorf("syncing calendar %q: %w", name, err)`. No `panic` outside of truly unrecoverable startup failures; the TUI must never crash on a bad server response or malformed .ics data — degrade gracefully and surface the error in the UI.
- **No global mutable state**: pass dependencies explicitly through constructors and function parameters. Package-level `const` and immutable lookup tables are fine; package-level `var` holding mutable state is not.
- **Naming**: standard Go style — `MixedCaps`, no underscores; short names for short scopes, descriptive names for wide scopes. Export only what another package actually needs.
- **Doc comments** on all exported identifiers, godoc style (start with the identifier's name).
- **Contexts**: all network and other I/O-bound operations (CalDAV sync above all) take a `context.Context` as their first parameter so they can be cancelled — the UI must never block uninterruptibly on the network.
- **Tests**: standard `testing` package only (no assertion frameworks). Prefer table-driven tests. Core logic (sync, recurrence, parsing) gets tests; thin UI glue may go without. Concurrency fixes get a real goroutine stress test run under `go test -race` (see `TestConcurrentSyncAndEditsRace`). The iCalendar ingest boundary has **native Go fuzz targets** (`internal/model/fuzz_test.go` — `FuzzDecode`/`FuzzEventOccurrences`/`FuzzBuildTree`/`FuzzParseQuickAdd`/`FuzzRecurrenceMutations`); the seed corpus (every `f.Add` case plus saved crashers under `internal/model/testdata/fuzz/`) runs as ordinary deterministic tests on the normal gate, and `go test -fuzz=Fuzz... ./internal/model/` explores new inputs — extend these rather than starting a parallel harness when hardening a parser path. A separate **opt-in live CalDAV suite** lives behind `//go:build live` (`internal/sync/live_test.go`), excluded from `make check`; run it only against a **test account** with `go test -tags live -run TestLive ./internal/sync/ -v` — it creates and deletes its own throwaway calendars and never touches existing ones.
- **No magic numbers**: named constants for anything with meaning; user-facing tunables belong in the TOML config file.

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

Cross-cutting rules the design depends on. A change that would weaken one needs the owner's sign-off, and any deliberate change to their hardening must update `docs/audit/COVERAGE.md`:

- **Iron rule**: never drop or mangle iCal properties the app doesn't understand — editing a known field preserves everything else.
- **The `.ics` files are the local source of truth** — never introduce a second store that can drift from them.
- **Sync never silently overwrites** in either direction — a true conflict keeps both versions and flags the item.
- **The app never writes the config file** — app-remembered state goes in the state file under the data dir; calendar names/colors are server-owned CalDAV data, not config.
- **Read-only calendars are never written to.**

### Hard-won guardrails (each guarded by regression tests — don't reintroduce)

- **tview freeze traps:** (1) **Never call an app-lock method (`a.tv.GetFocus()`, etc.) from a `SetDrawFunc`/draw path** — `Application.draw()` holds the write-lock and `RWMutex` isn't reentrant, so it self-deadlocks; read tracked plain fields instead (the mode indicator's `interactionMode` derives everything from `a.grabbing` + `a.gridDrilled()`, taking no app lock). (2) **The task tree runs with `SetGraphics(false)`** — tview v0.42.0 `TreeView.Draw` has an infinite loop when a node's indent exceeds the pane width; leave graphics off (nesting shows via indentation + ▸/▾ carets). Regression tests: `modedeadlock_test.go`, `treedraw_regress_test.go`, and the broader `displaystress_test.go` — every custom `Draw` path drawn with display-hostile content across 1×1→400×150 geometries under a panic-recover + watchdog; extend it when adding a widget or draw path so a new freeze/panic is caught on the normal gate.
- **Every selectable list must carry the theme-adaptive `selectionStyle` — this includes `tview.DropDown`s, not just `tview.List`s.** The app sets `tview.Styles.PrimitiveBackgroundColor = tcell.ColorDefault` (`useTerminalTheme`), so tview's *default* selected style — terminal-default foreground on a `PrimaryTextColor` (white) bar — renders illegibly (white-on-white) on a terminal-default background. The shared `selectionStyle` (`tcell.StyleDefault.Reverse(true)`) is theme-adaptive. A plain `List` sets it via `SetSelectedStyle(selectionStyle)` (`calendars`/`tasklists`/`agendaList`, the conflicts list, the `:account` picker); a **`DropDown`**'s embedded list sets it via `SetListStyles(tcell.StyleDefault, selectionStyle)` — done once in the shared `caretForm.addDropDown`, so every form dropdown (priority, Repeat, the Custom sub-form's Unit/Monthly-by/Ends) inherits it. **This class has reappeared twice** (the v1.1.0 `:account` picker shipped without it; the v1.3.0 recurrence dropdowns shipped with tview's illegible default). Any *new* selectable widget — or a dropdown created outside `addDropDown` — must set the style and get a reverse-video regression test. Tests: `TestSelectionIsLegible`, `TestAccountPickerSelectionIsLegible`, `TestDropDownSelectionIsLegible` (`internal/ui/{app,account,recurbugfix}_test.go`).
- **A modal nested over another modal must not restore focus to the calendar.** `captureFocus` records the calendar drill state (for `restoreFocus` to re-drill and re-focus the grid) **only when no modal is already open** (`a.mode == modeCalendar && !a.modalOpen()`) — the sole case the covered focus is the calendar grid. A modal opened over another modal (e.g. the Custom… repeat sub-form over the item form) covers the *outer modal*, so its captured `focusState` must point there; capturing the calendar drill state for a nested modal makes `restoreFocus` teleport to the calendar on close, stranding the outer form open-but-unreachable (a softlock). Regression test: `TestNestedModalOverDrilledCalendarKeepsFormFocus` (`internal/ui/recurbugfix_test.go`).
- **Scale invariants (guarded by benchmarks in `internal/model/scale_test.go` + `internal/sync/scale_bench_test.go`):** four hot paths must stay linear — recurrence expansion (`Event.Occurrences` via `safeBetween`) is **bounded** (a pathological rule can't hang the UI), `BuildTree` classifies cycles by memoized parent-chain (not a per-insert subtree walk), `LayoutDay`'s overlap-lane packing is a **sweep line over two heaps** (never a first-fit scan over `laneEnds`, which is Θ(n²) when everything overlaps), and a bulk pull uses `store.PullRemoteBatch` (one sidecar write per calendar, not per resource). **Bounding what a stage *produces* does not bound what the next stage *does with it*** — the pass-21 aggregate `StepBudget` capped recurrence output, and pass 23 still found the layout of that bounded output freezing the UI. When you bound one stage, check the whole pipeline to the pixel: expansion → layout → the UI's own per-item scans. `LayoutDay`'s packing is rendering-visible, so a faster algorithm must produce **identical** lanes; `TestLayoutDayMatchesFirstFitReference` keeps a copy of the naive first-fit and diffs against it, and is the tripwire a deliberate semantics change must update. The batch is **pull-only and single-lock** by design: never route a push through it (crash mid-batch would duplicate a create), and keep the reconcile rule that a clean, href-less local resource is a pull orphan to re-pull — not a create to push.
- **Malformed iCalendar is contained/healed at ingest, never fatal:** go-ical's decoder and rrule-go's iterator both **panic** on some malformed input — contained by recover guards at the byte→calendar boundaries (`model.decodeCalendar`, `Event.Occurrences`'s `safeBetween`, and `internal/caldav`'s `guardICalPanic` around `QueryCalendar`/`GetCalendarObject`); don't add a decode/expand path that can panic the app. go-ical's decoder is also more tolerant than its encoder, so `model.Parse` **heals** an object into an encodable shape on ingest (`ensureDTStamp`/`ensureCalendarProps`/`dedupeSingleValued`/`sanitizePropValues`/`stripForbiddenNesting`/`healComponentConstraints`/`dropUnusableTimezones`) — otherwise a foreign/hand-edited `.ics` loads but can't be edited or saved. Preserve this: add-only-when-missing, never mangle existing props (iron rule); an unencodable component that carries no usable data may be dropped when the alternative is bricking the whole resource (e.g. `dropUnusableTimezones` strips a VTIMEZONE go-ical would reject — but this is a deliberate, owner-approved exception to the iron rule, not the default). **This heal set must mirror go-ical's *full* encoder cardinality/required-prop rules — `vendor/.../go-ical/encoder.go` `checkComponent` (exactlyOne/atMostOne per component) and `model.singleValuedProps` — at go-ical's *recursion depth*, not just its per-component cardinality table.** `checkComponent` runs on **every component at every depth** (`encodeComponent` recurses into all children before emitting), so a phantom component nested at any depth bricks the whole resource. The required-prop/mutual-exclusion heals (`ensureDTStamp`, `healComponentConstraints`) run **top-level-only**; the safety net that makes that sufficient is `model.allowedChildren` (driving `stripForbiddenNesting`, which recurses), and it is consulted **deny-by-default, restricted to the types go-ical validates: a component type with no entry in the map admits no child whose name is in `model.encoderValidatedComponents`, and keeps every other child untouched.** Entries exist only to *permit* legal nesting — VEVENT/VTODO (admit VALARM), VTIMEZONE (STANDARD/DAYLIGHT), and the childless VJOURNAL/VFREEBUSY/VALARM/STANDARD/DAYLIGHT (empty allow-set) — while X-\*, VAVAILABILITY, and every type a future spec adds are unknown containers, policed without ever being listed. **Never invert this back to allow-by-default**: `checkComponent` has cases for only the RFC 5545 core set but `encodeComponent` recurses into *every* child regardless of name, so "list every container go-ical recurses into" is an unsatisfiable enumeration — the map cannot name a type nobody has invented yet. Equally, **never widen the strip to all children of an unknown container**: `checkComponent` has **no default case**, so a component whose name is outside `encoderValidatedComponents` gets nil cardinality lists and always returns nil — it *cannot* brick the resource, and stripping it would destroy real data (an RFC 7953 VAVAILABILITY's AVAILABLE sub-components, a vendor's X- payload) to buy nothing. Membership in that set — mirroring `checkComponent`'s switch cases, VALARM included against its `// TODO` case gaining rules — is precisely "can fail an encode", which is why it is the strip criterion. A nested VCALENDAR is a special case handled in `stripForbiddenNesting` itself (a VCALENDAR is the object, never a component inside one). With unknown containers policed this way, no VEVENT/VTODO/VJOURNAL/VFREEBUSY survives below the top level, so the top-level heals have nothing nested to miss. This class has reopened **four times** (pass 10 healed VEVENT/VTODO + empty/nested VTIMEZONE; pass 16 found VJOURNAL/VFREEBUSY + VTIMEZONE-required-props; pass 21 found the heals were top-level-only and `allowedChildren` lacked VALARM/STANDARD/DAYLIGHT, so a phantom VEVENT nested under a VALARM bricked the resource; pass 23 found the map itself was allow-by-default, so a phantom nested under an *unknown* container — X-\*, VAVAILABILITY, nested VCALENDAR — was never stripped at all). Whenever the app may ingest a new component type, or the go-ical dependency bumps, re-diff `singleValuedProps`/the DTSTAMP-heal set **and `encoderValidatedComponents`** against `checkComponent`, and confirm `allowedChildren` still only *permits* (never gates) — a decode-but-can't-re-encode gap is a HIGH (one bad component makes the whole resource, incl. valid siblings, unsavable). **Named accepted cost:** a *valid* VEVENT/VTODO nested under an unknown container is still stripped even though it would have encoded fine — it was never addressable (`Parse` walks only the calendar's direct children), so this loses weird unreachable data, never a real RFC shape. A missing **UID** is the one required prop deliberately *not* healed (a fabricated UID churns sync identity — accepted residual). Regression tests: `internal/model/{fuzz_test,harden_ingest_test,vjournal_encode_test,malformed_vtimezone_test,nested_heal_test,unknown_container_test}.go`, `internal/caldav/guardpanic_test.go`.
- **Concurrent writes are version-checked:** every UI write to an *existing* resource routes through `store.PutIfUnchanged` against its located `Prev` — never a bare `Locate→Put`, which silently clobbers a concurrent sync pull — and a multi-write operation either rolls back or skips cleanly when a later write fails. A bare `store.Put` is correct *only* for a fresh create (a brand-new resource under a UID/name minted in that same call, so no existing resource can be clobbered) and must carry a `// create: fresh UID, no existing resource to clobber` (or equivalent, e.g. a fresh `(calID, name)` pair) comment marking it as deliberate, not an oversight. **This class has reopened three times** (pass 13 fixed `reparentSelected` and declared it closed; pass 19 found the sibling `reparentTo` still bare, then swept `grab.go`/`recur_edit.go`/`yankpaste.go` for the rest) — treat every new or touched write path as suspect: when adding a write path, or touching any existing one, (1) classify it as an existing-resource rewrite or a fresh create, (2) route the former through `PutIfUnchanged` (follow the `applyMutation`/`reparentOps` pattern) and mark the latter with the comment above, and (3) grep `internal/ui` for bare `store.Put(` sites while you're in there — don't assume a prior sweep caught every call site. Regression tests: `TestApplyMutationDoesNotClobberConcurrentPull` (`internal/ui/editclobber_test.go`) and the pass-19 sweep's per-site clobber guards — `internal/ui/reparent_clobber_test.go`, `internal/ui/grab_split_clobber_test.go`, `internal/ui/recur_edit_clobber_test.go`, `internal/ui/movesubtree_clobber_test.go`, `internal/ui/movesubtree_crosscoll_test.go` (a cross-collection subtree member is moved from its OWN `loc.CalID`, not a hard-coded source, and rewritten in place — not create-then-delete — when it already lives in the destination; pass-20 residual).
- **A sync-reconcile "resource is gone on the server" signal must not unconditionally remove/overwrite the local resource — guard it by pointer identity against a concurrent local edit.** This is the *sibling* of the version-check rule above, on the **sync side** (`internal/sync`, `internal/store` reconcile writes) rather than UI writes, and it has **reopened three times**: pass 18 (`CommitPush` treated a delete-during-push as a resurrect — `cur == nil`), pass 19 (the pushDelete-412 tombstone resurrect clobbered a concurrent undo), pass 20 (reconcile **step-(A)'s `!onServer` clean branch** `Forget`ed a resource a UI edit had just replaced). The trap is identical every time: reconcile snapshots resource **pointers** (`Calendar()`), a concurrent UI edit builds a *new* `*Resource` and swaps the map entry (`stageResourceLocked`), and the reconcile branch acts on the **stale** snapshot — Forgetting/overwriting the fresh edit with no conflict and no skip (silent lost update). Any reconcile path that **removes or overwrites because the server no longer has the resource** must (1) take the `expectedPrev`/pointer-identity guard — `store.PullRemote` / `PutIfUnchanged` for writes, **`store.ForgetIfUnchanged` for removals** (never a bare `st.Forget` in reconcile) — and (2) on a mismatch, raise a `markConflict(serverDeleted=true)` against the surviving edit rather than dropping it. When adding or touching a reconcile branch, ask "what if a UI edit landed on this name during the network I/O?" — if the answer is a clobber, it's this bug. `internal/model` and `internal/caldav` peer write paths are **not yet swept** for this class (pass-20 blind spot). Regression tests: `internal/store/commitpush_deletemidpush_test.go` (`TestCommitPushDoesNotResurrectDeletedResource`, `TestCommitPushDeleteRaceInvariant`), `internal/sync/tombstone412_undo_race_test.go` (`TestTombstone412ResurrectDoesNotClobberConcurrentUndo`), `internal/sync/stepa_forget_clobber_test.go` (`TestReproStepAForgetClobbersConcurrentEdit`).
- **Moving a recurring item's anchor must keep the rule consistent with it — never leave `DTSTART` contradicting its own `BY*`.** A day-pinning rule (weekly `BYDAY`, monthly nth-weekday) fires independently of `DTSTART`; shifting `DTSTART` while leaving the `BY*` untouched makes the moved anchor fall outside its own recurrence set, so the series keeps firing on the old day and the moved instance disappears from the calendar. Any path that moves a recurring master's day (grab's `h`/`l` day-move was the offender) must **re-anchor the day-pinning `BY*` to the new start** (weekly weekday sets shift as a whole; monthly nth-weekday re-derives) via `model.ReanchoredRecurrence`, which returns `(nil,false)` when no rewrite is needed (daily, plain weekly, monthly-by-day, yearly — these carry no day-pinning `BY*` and re-anchor via `DTSTART` alone) and `(nil,true)` to **block** the move on a rule outside the editable vocabulary (a *Custom rule (kept)*) rather than corrupt it. This is the same "a rule can't contradict its start date" invariant the v1.3.0 Custom sub-form enforces. **The anchor is `DUE` for a recurring VTODO, not just `DTSTART` for an event** — a native LazyPlanner recurring todo carries only `DUE`, which *is* its rule anchor, so **every path that day-shifts a recurring todo's due must re-anchor too**, via `model.ReanchoredRecurrenceTodo` (the todo-facing twin of `ReanchoredRecurrence`; both delegate to the shared `reanchoredRecurrence(raw, oldAnchor, newAnchor)` core). This reopened once already: the event grab was hardened (pass 18-era) but single-item grab's `j`/`k` todo due-nudge (`grab.go`) and bulk grab (`bulkgrab.go`) both shipped without it (pass 20). When adding a path that moves a recurring todo's due, call `ReanchoredRecurrenceTodo(td, oldDue, newDue)` and block on `(nil,true)`. Regression tests: `internal/model/reanchor_test.go`, `internal/ui/grab_recur_reanchor_test.go` (event), `internal/ui/grab_todo_reanchor_test.go` (single grab, todo), `internal/ui/bulkgrab_recur_reanchor_test.go` (bulk grab, todo).
- **A recurrence anchor and the rule it anchors must be authored in the same zone.** RFC 5545 evaluates a rule's `BY*` parts in its anchor's own zone; deriving `BY*` from the user's local weekday while serializing `DTSTART`/`DUE` in UTC lets the two disagree whenever the local and UTC dates differ, so the rule fires on the wrong day and drifts an hour across DST. Three defects shared this one root cause: an untouched Repeat dropdown silently rewrote the rule on Save (also dropping orphaned per-occurrence overrides), a "Weekly on Tue" pick produced a Monday series whose first occurrence was not the event's own start, and a 20:00 EDT series drifted to 19:00 EST after the fall transition. Required pattern: write a recurrence anchor with `setAnchorDateOrTime` + `anchorZone` (`internal/model/edit.go`), never the plain `setDateOrTime` — and any writer that emits a TZID must call `ensureVTimezone` so the zone is *defined*, not just referenced. This was missed on three sibling writers (`RewriteEventRule`, `NewSeriesFrom`, `DetachTodoOccurrence`) and caught only by a whole-branch review; treat every new or touched anchor writer as suspect and grep for siblings. The gate is deliberately narrow: the TZID form is written only when this same call is authoring the rule, or the existing anchor already carries a resolvable TZID — an edit that leaves the rule alone must never re-anchor, since re-interpreting `BY*` against a different zone silently MOVES an existing series. A form's Repeat state must be **seeded and resolved on the same anchor basis**; seeding from the raw stored anchor while resolving against the form's local start is what made an untouched dropdown report a rewrite (`newEventRepeat`/`newTodoRepeat`, `internal/ui/itemforms.go`). `a.loc` (the display/authoring zone, now `config.LocalZone()`) can diverge from `time.Local` — every render path must read `a.loc`. Regression tests: `internal/model/{tzanchor,vtimezone}_test.go`, `internal/ui/{repeatanchor,tzanchor_uipath,vtimezone_writers_uipath}_test.go`. `TestNonRecurringEventStaysUTC` and `TestEditWithoutRuleChangeKeepsAnchorForm` (`internal/model/tzanchor_test.go`) are load-bearing and must never be weakened.
- **RDATE/EXDATE are multi-valued and independent of the RRULE's COUNT/UNTIL bound:** three pass-14 defects shared this root cause, so any code touching recurrence must respect all three facts. (1) An `RDATE`/`EXDATE` line may carry several comma-separated values (and an `RDATE` may be `VALUE=PERIOD`), so resolve them **per value** (`resolveDateTimeValues`, `filterRDates`) — never `prop.DateTime` on the whole line, which errors and collapses the series to its base instance. (2) `COUNT` bounds the RRULE *generator*, so an `EXDATE`'d instance still consumes `COUNT`: split/cap math counts RRULE *iterations* (`rruleIterationsBefore`), not the EXDATE-filtered *visible* set, or the future half gains a phantom occurrence. (3) `UNTIL` bounds only the RRULE, not `RDATE`s (rrule-go's `Set.Iterator` merges RDATEs independent of UNTIL), so capping/splitting a series must **partition RDATEs explicitly** (`filterRDates`), or a trailing RDATE lands in both halves. Regression tests: `internal/model/{multivalue_dates,recur_split_exdate,recur_split_rdate}_test.go`.
- **A regression test for a UI-reachable bug must drive the real entry point — the form, key handler, or command — not the helper in isolation.** A test that hand-builds a helper's inputs proves the helper's *arithmetic* and nothing about whether production ever supplies those inputs, so a fix can land, go green, and never reach a user: the bug ships behind a passing test. **This class has been confirmed in two consecutive passes** — pass 21's aggregate `StepBudget` (bounded expansion correctly, but was minted per `store` call rather than once per redraw, and its test handed the expansion helper its own budget) and pass 22's timed "Ends on date" UNTIL (`readCustomRecur` correctly anchors UNTIL at `D` + the anchor's wall-clock time, but both `wireRepeatCustom` anchorFns in `internal/ui/itemforms.go` parsed only the DATE field, so production always passed a midnight anchor and the shipped fix computed the pre-fix value; caught in pass 23). Rules: (1) the guard test starts where the user does — build the real form/handler, set the widgets, fire the button — and asserts on what gets **stored** or rendered, not on the helper's return value; (2) whenever a helper takes a value the UI *derives* (an anchor, a budget, a location, a selection), the test must exercise the derivation, since that wiring is where this class hides; (3) a direct-helper unit test may accompany the end-to-end guard but never substitute for it. When a fix is confined to a helper, ask "who calls this in production, and does the call site actually produce the input my test used?" — if you can't answer from the test, the test isn't guarding the bug. Pattern to copy: `internal/ui/endsondate_uipath_test.go` (form → Repeat → Custom… → Ends on date → OK → `readEventDraft`/`readTodoDraft` → the stored object's occurrences), backing the helper-only `internal/ui/ends_on_date_test.go`.
- **Text the app does not author must pass through `tview.Escape` at the call site before it reaches a dynamic-colour string.** tview parses `[...]` as a colour/region tag, so a calendar display name, task summary, typed query, parser warning, file path or server error containing a bracket run is silently **swallowed** from the status bar — the user sees a truncated or empty message with no indication anything was dropped. **Escaping centrally inside `flash`/`echo` is not possible**: `advanceRecurringTodo` (`recur_edit.go`) deliberately passes style tags, so a blanket escape would break intentional colour. The rules: (1) escape at each concatenation boundary, not in the sink; (2) `flashErr`/`failureMsg` is the funnel for the ~15 `"<Action> failed: " + err.Error()` sites — escaping there covers all of them, and no funnel caller may pass intentional tags; (3) **`%q` is not a substitute** — it quotes and backslash-escapes, but leaves a `[` run intact and mangles embedded quotes, so prefer plain quotes plus `tview.Escape` (the `calendar.go` flashes were converted for exactly this reason). This class was reported at 5 sites in pass 23 and the sweep found **9 more** across `calendar.go`, `edit.go`, `quickfield.go` and `yankpaste.go`; when adding any flash/echo, ask "did the app write every character of this string?" — if not, escape it. Regression guards: `internal/ui/tagescape_sweep_test.go` (table-driven over every site, hostile `[red]`/`[white:black]`/`[""]`/`[-]` runs, plus a byte-identical assertion that escaping does not alter ordinary text) and `internal/ui/statusbar_tagescape_test.go`.
- **A test must build its times in the zone the code under test uses.** `newApp` hard-codes `loc: time.Local`, so a test that constructs `now`/`anchor`/due dates with `time.UTC` makes its own day window disagree with the day the app buckets locally-timed items into. That disagreement is **invisible at or west of UTC and fails east of it** — and since CI runs UTC, the suite goes green while anyone running `make check` from an east-of-UTC zone sees a red gate on clean code. Pass 23 found two such tests (`TestUndoWhileDrilledKeepsDrill`, `TestHideCompletedAppliesToCalendarAndAgenda`). Build test clocks with `time.Local`, or pin an explicit zone and use it consistently on both sides of the assertion. **Recurrence and day-bucketing tests should run over several zones** (`UTC`, a western and an eastern offset) — the pass-23 all-day `UNTIL` defect was two bugs whose signs flipped with the offset, so a single-zone test passed on half the planet. Run `TZ=Asia/Kolkata go test ./...` before claiming a zone-sensitive change is done. The TZID-anchored-recurrence arc reopened this class three more ways: (1) **a zone-parameterised test must be shaped so every zone can actually fail** — a fixture anchored at 23:00Z only crosses the local day boundary for offsets ≥ +1h, so the `UTC` and `America/New_York` subtests were structurally incapable of failing, and New York was the zone the bug was reported from; choose the fixture per zone (a late-UTC-hour anchor for positive offsets, an early-UTC-hour one for negative offsets), and keep any deliberately non-failing zone explicitly commented as a control; (2) **never `t.Skip` on an unavailable zone in a load-bearing guard** — it silently converts the guard into a vacuous pass; `internal/model` and `internal/ui` now import `_ "time/tzdata"` in their zone tests so every named zone loads, and a load failure is `t.Fatalf`, not `t.Skipf`; (3) **when the input space is enumerable, enumerate it** — hand-picked zone samples missed the `Africa/Cairo`/`Asia/Beirut`/`America/Santiago` near-midnight-transition defect that a full sweep of the IANA database (482 zones × 11 anchors) found immediately, settling with zero ambiguity where five hand-picked zones had not. **Prove every new guard bites by mutation**: reapply the bug, confirm the test goes RED, revert, confirm GREEN — four zone/anchor tests in this arc passed review while vacuous, and only a mutation check caught them.
