# Claude Code — Project Instructions

> These rules apply to every task in this project.
> Claude Code reads this file automatically from the project root.

---

## Project Context

- **Spec**: `main.md` — the single source of truth for what to build.
- **Change Log**: `log.md` — append an entry every time you make a change.
- **README**: `README.md` — user-facing docs (what it does, usage, build/install for Linux and Windows). **Update it whenever user-visible behavior, usage, or build steps change.**
- **Examples**: `examples/spec_examples/` — spec files from a prior project, used as structural reference only (not project rules).
- **Platform**: Terminal TUI (lazygit-inspired). Language: **Go**. TUI: **tview** (`rivo/tview` on `tcell`); CalDAV via `emersion/go-webdav` + `emersion/go-ical` + `teambition/rrule-go`.
- **Local cache**: vdir-style raw `.ics` files (one file per event/todo, one dir per calendar) + JSON sidecar for sync state + in-memory index at startup. The `.ics` files are the local source of truth — never introduce a second store that can drift from them.
- **UI**: lazygit-style — a left "overview" column (Calendars / Tasks lists / Agenda) drives the center Main pane. **c**/**t**/**a** *focus the matching left panel* (the highlight lives in the overview); Main shows the matching view: **c**→calendar (month grid + week/day hourly time-grid), **t**→the selected list's subtask tree, **a**→full-detail day agenda (Detail pane hidden, center full-width). `Enter` dives from the overview into Main (in calendar mode, into the grid); `Esc` backs out. In agenda mode, moving the left-list highlight highlights the matching Main block (auto-scrolls). `[`/`]` cycle the highlighted calendar and `{`/`}` cycle the highlighted task list — both global (any mode, independent overview selectors); `Space` checks off the selected/drilled task in any view (tree, agenda, or a task drilled into in month/week/day), or — in calendar view with no task drilled — hides/shows the highlighted calendar (remembered in the state file; a drilled *event* can't be completed, so `Space` there flashes rather than toggling visibility); each calendar row is marked `[events]`/`[tasks]`/`[both]`/`[?]` by its supported component set (and a `●` bullet in the calendar's server color, drawn as exact truecolor by default (tcell downsamples per terminal) — that color also tints the calendar's items across the month grid, time-grid, and agenda; a **hidden** calendar drops the bullet, so its off-the-views state reads at a glance next to the `(hidden)` marker), and **item creation is gated to that type** (events only on VEVENT calendars, tasks only on VTODO lists, both on "both"; an unknown/unconfirmed type blocks creation until sync settles it, with a manual `i!`… force override (e.g. `i!e`) for the unknown-type case only — read-only and known-wrong-type are never forced). Right Detail pane shows the highlighted item's full fields. The left Tasks panel lists the task lists (not the tree); Main shows the selected list's tree with `>`/`<` zoom. Calendar selected-day is marked by an outline box (not a fill); a day can be drilled into (`Enter`) to cycle its events in month **and** week/day views. Un-drilled week/day: `h`/`l` move days, `j`/`k` do nothing (`Enter` drills). Drilled week/day is **2D spatial** over the day's layout — `j`/`k` by time, `h`/`l` between concurrent side-by-side events (overlap lanes via `model.LayoutDay`), all-day band as the top row; drilled month is 1D (`j`/`k` cycle). `f`/`b` changes the period and stays drilled; `Esc` backs out. Week/day gives every hour a uniform height (evenly-spaced axis, small blank margin possible below the last hour; scrolls to the drilled item / current time when the pane is too short for all 24h; `+`/`-` zoom the hour-row height, remembered in the state file — in other views `+`/`-` stay the accordion) and shows due tasks too (a `[ ]`/`[■]` task line at the due time, or the all-day band for all-day-due, in the list color). Completed tasks render with a filled checkbox `[■]` (still hidden by default; `.` toggles). Agenda selection is the same outline box (custom-drawn widget). Creation is object-keyed and works from any pane: create-task → selected task list, create-event → selected calendar, create-subtask → under the currently selected task (tree node, calendar drill, or agenda) and into that parent's own list (RELATED-TO shares a collection); tasks can also be completed/edited/deleted from the calendar+agenda views (drill in, then `Space`/`e`/`d`). Each create has a quick-add smart parser (date/`!priority`/`#tag`) and a full form, reached through the chorded keymap (`i`-prefix; Shift = full form). Folders: a task with ≥1 incomplete child renders `▸`/`▾` (not a checkbox) — the same `▸` caret in the calendar+agenda views too (one global folder set), can't be completed until its children are (enforced in every view), reverts to a task when empty/all-done, and deletes recursively (extra confirm). A folder **keeps its own due date** (folder-ness is orthogonal to the due date), so a dated task that gains a subtask still shows on the calendar — it just gains the caret; hiding folder due dates was rejected (loses data + makes the task vanish). Checking off a task while completed are hidden keeps it visible until you leave the list (not on a popup). Timed values stored UTC, displayed local. Bottom is two lines: the **status bar** (outlined like the primary panes) above an always-visible **help bar** (the controls line). The status bar has four sections: a leftmost **interaction-mode indicator** (`NORMAL` at rest · `DRILL` when drilled into a calendar day (merely focusing the tree or grid stays `NORMAL`, so the tree and grid agree — DRILL never shows in Tasks) · `GRAB` in grab mode — a filled high-contrast chip for the active modes so a mode-sensitive key like `hjkl` is never a surprise; custom-drawn via `drawModeIndicator`/`interactionMode`, computed from existing state rather than a new state machine), then general/results, the command-view, and sync-status (not-configured/syncing/synced/conflicts/offline). The mode indicator is the *interaction* mode (what the movement keys act on now); the general section still shows the *view* context (Calendar/Tasks/Agenda · period). Rounded (soft) borders everywhere; popups (forms/quick-add/confirm) share a terminal-default unified background with high-contrast text and an accent border, and the focused form field is marked by a `▸` caret (tview uses one field style for all fields, so the caret — not a color — is the per-field focus cue). Calendar colors render as exact truecolor by default (tcell auto-downsamples to 256/16, incl. a bare TTY; `[appearance] color_mode` = `auto`/`truecolor`/`16`/`off`, and dark foreground colors are lightened to a readability floor since they sit on the unknown default background); completed tasks hidden (`.` toggles); smart sort (due → priority → title); session undo stack (`u`). Two-way sync, the `config.toml` (`[server]` + `password_command`, first-run generation), the account-namespaced cache, the sync-status indicator, and offline-first in-app calendar/list creation+deletion landed in step 9. Step 10 landed the vim-style chorded keymap with a which-key popup, the `:` command line (`:sync`/`:view`/`:goto`/`:search`/`:config`/`:calendar`/`:conflicts`/`:help`/`:q`) with the status-bar command view, the `?` help overlay, interactive conflict resolution (`:conflicts`), pane sizing (accordion `+`/`-`, `Ctrl-←`/`Ctrl-→` resize, remembered in the state file), and a mouse pass. A step-10 finale refined the keymap: panels moved to mnemonic **c**/**t**/**a**, create to the **`i`**-prefix (`it`/`iT`/`ie`/`iE`/`is`/`iS`/`ic`/`il`), calendar period nav to `f`/`b`, `g`-prefix go (`gg` top · `gt` today · `gd` go-to-date) with `G` bottom, `z`-prefix fold (`zR`/`zM`/`za`), and vim **counts** (`3j`) now that the number row is free; contextual `d`; `r`=:sync. The finale also added incremental **search** (`/` with `n`/`N`, and `:search`, over the task tree / agenda / calendar names), the **calendar visibility toggle** (`Space`, remembered), **quick field-set** (`s`-prefix: `sp` priority, `sd` due — one-line inputs that change one task field, preserving the rest), **yank/paste** for tasks: `y` cut (move) / `Y` copy (duplicate with fresh UIDs, RELATED-TO remapped via `model.CopyTodo`) a task + its subtree; `p` pastes under the selected task, `P` at the list top level; the clipboard **persists after paste** (multi-paste); same-list move = re-parent, cross-list = recreate+delete, both all-or-nothing with rollback; undo-able (events are handled by grab mode, not yank). **Grab mode** (`m`, `grab.go`) is the temporal-manipulation layer, unified across tree/calendar/agenda via `currentTarget`: move an event (`j`/`k` ±hour in week/day, `h`/`l` ±day, `J`/`K` resize the end) or nudge a task's due date (`j`/`k` ±day, `h`/`l` ±week); modal (swallows keys), edits commit to the store on each nudge so views update live, `Enter` keeps and `Esc` reverts to the pre-grab snapshot (one undo step). Undated tasks are skipped. **Recurrence editing (step 11)**: editing/deleting/grabbing a recurring **event** opens a scope picker (`recur_edit.go` `pickRecurrenceScope`) — *this occurrence* (a `RECURRENCE-ID` override / `EXDATE`), *this & future* (edit/delete only — series split via `model.CapSeries`+`NewSeriesFrom`), *all* (edit the master); the write-side primitives live in `internal/model/recur_edit.go`. A recurring **todo** shows a single live instance and *advances* on `Space`-complete (`AdvanceRecurringTodo`, NextCloud-style), with edit-this-occurrence = detach-as-standalone + advance; grab's this-and-future is intentionally omitted (its single-snapshot revert can't undo a split). Structural moves stay on yank/paste, and **`:calendar rename`/`color`** which edit server-owned calendar metadata offline-first and push a CalDAV `PROPPATCH` on the next sync (`:calendar hide`/`show` mirror the `Space` toggle). A calendar's color is a field in the unified **create/edit calendar form** (`showCalendarForm`) with a "Pick color…" button opening a **swatch-grid picker** (`colorpicker.go`, a preset palette + "Custom hex…"); the color is set **at creation** (carried in the MKCALENDAR, not left default) — the field is pre-seeded with a default palette color (`defaultCalendarColor`, NextCloud blue) and blank-on-create falls back to it, so every created collection always has a color. `e` on the Calendars pane opens the edit form (name + color); `:calendar color` with no hex opens the picker directly (with a hex it sets directly). The picker can nest over the form because modal focus save/restore is a **stack** (`focusStack`). Calendar **names and colors** sync **both ways**: the color is pulled via a `calendar-color` PROPFIND (go-webdav doesn't surface it) and rendered as exact truecolor (or nearest-ANSI under `color_mode = "16"`); the display name is pulled too (`SyncCalendarName`, mirroring `SyncCalendarColor`) — both server-authoritative except when a local edit is still pending a push. Read-only calendars (e.g. NextCloud birthdays) are detected (privilege PROPFIND) and never written to. See `main.md` UI Design for layout + keymap.
- **Data model**: subtask hierarchy (arbitrary depth via RELATED-TO, file-explorer UI) is the centerpiece todo feature. **Iron rule: never drop or mangle iCal properties the app doesn't understand** — editing a known field preserves everything else. Display/create in local timezone; all-day items stay date-only.
- **Sync**: ETag-based, never silently overwrites; conflicts keep both versions + UI flag. Triggers: `:sync`, startup (background), periodic (default 15 min), debounced push after edits. Credentials: NextCloud app password in 0600 config or via `password_command` (owner uses Vaultwarden → `bw` CLI).
- **Config**: TOML via `BurntSushi/toml`, moderate scope (connection + appearance/behavior + optional per-calendar local overrides; keybindings hardcoded for now). **The app never writes the config file** — calendar names/colors are server-owned CalDAV properties (data, not config); app-remembered state goes in a state file under the data dir. Creating/deleting calendars is done in-app via CalDAV `MKCALENDAR`/`DELETE` (go-webdav's client lacks calendar creation, so `internal/caldav` issues these over its own authenticated HTTP client — verified against NextCloud). Runtime paths: config at `~/.config/lazyplanner/`, data at `~/.local/share/lazyplanner/` (XDG on Linux; resolved through one OS-aware helper — Windows is a secondary target).

---

## Iterative Build Workflow

1. **Before starting work**, read `main.md` to understand the spec and `log.md` to see what's already been done.
2. **Work in small increments** — one module or feature at a time.
3. **After every change**, append a dated entry to `log.md` describing what was added, changed, or fixed.
4. **Run tests and lints** after every code change: `go test ./...`, then `go vet ./...` and `staticcheck ./...`
5. **Run the program** to verify it still builds and launches: `go build ./...` (and `go run ./cmd/lazyplanner` for manual checks).
6. **Keep `README.md` current** — if the change altered user-visible behavior, usage, or build steps, update the README in the same increment.
7. **Commit often** with descriptive messages: `git add . && git commit -m "feat: ..."` — on `ai-workspace`, never `main` (see Git Branching Rules).

---

## Git Branching Rules

- **`ai-workspace` is Claude's branch.** All Claude work — commits, experiments, build steps — happens on `ai-workspace` or on branches created off it. Feature/experiment branches off `ai-workspace` are fine; merge them back into `ai-workspace` when done.
- **NEVER merge to `main`. NEVER commit to `main`.** Merging `ai-workspace` into `main` is the owner's action, done by the owner after review — no exceptions, even if asked to "finish up" or "ship it."
- **`ai-init` is frozen.** It preserves the state of the workspace immediately before build step 1 (spec complete, no code). Never commit to it — it exists as a permanent reference point / reset target.
- Before starting work, confirm the current branch is `ai-workspace` (or a branch off it): `git branch --show-current`.

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
- **Tests**: standard `testing` package only (no assertion frameworks). Prefer table-driven tests. Core logic (sync, recurrence, parsing) gets tests; thin UI glue may go without.
- **No magic numbers**: named constants for anything with meaning; user-facing tunables belong in the TOML config file.

---

## Log Format

When appending to `log.md`, use this format:

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
- After editing `log.md`, verify the result: the number of `## ` headings must equal the number of entries.

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
- **tview freeze traps (both hit and fixed 2026-07-10, guarded by tests — don't reintroduce):** (1) **Never call an app-lock method (`a.tv.GetFocus()`, etc.) from a `SetDrawFunc`/draw path** — `Application.draw()` holds the write-lock and `RWMutex` isn't reentrant, so it self-deadlocks; read tracked plain fields instead (the mode indicator's `interactionMode` derives everything from `a.grabbing` + `a.gridDrilled()`, taking no app lock). (2) **The task tree runs with `SetGraphics(false)`** — tview v0.42.0 `TreeView.Draw` has an infinite loop when a node's indent exceeds the pane width; leave graphics off (nesting shows via indentation + ▸/▾ carets). Regression tests: `modedeadlock_test.go`, `treedraw_regress_test.go`.
- **Never hand-edit `vendor/`** — it's silently reverted by `go mod vendor`. Fix library bugs in our own code, or (if unavoidable) via a `replace` directive.
