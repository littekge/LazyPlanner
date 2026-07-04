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
- **UI**: lazygit-style — left column of focusable panels (Calendars/Tasks/Agenda), Main pane follows focus, right Detail pane, status bar. Task tree: full tree + `>`/`<` zoom. Creation: `a` quick-add with smart date/`!priority`/`#tag` parsing; `e` full form. Terminal 16-color palette only; completed tasks hidden (`.` toggles); smart sort (due → priority → title); session undo stack (`u`). See `main.md` UI Design for layout + keymap.
- **Data model**: subtask hierarchy (arbitrary depth via RELATED-TO, file-explorer UI) is the centerpiece todo feature. **Iron rule: never drop or mangle iCal properties the app doesn't understand** — editing a known field preserves everything else. Display/create in local timezone; all-day items stay date-only.
- **Sync**: ETag-based, never silently overwrites; conflicts keep both versions + UI flag. Triggers: `:sync`, startup (background), periodic (default 15 min), debounced push after edits. Credentials: NextCloud app password in 0600 config or via `password_command` (owner uses Vaultwarden → `bw` CLI).
- **Config**: TOML via `BurntSushi/toml`, moderate scope (connection + appearance/behavior + optional per-calendar local overrides; keybindings hardcoded for now). **The app never writes the config file** — calendar names/colors are server-owned CalDAV properties (data, not config); app-remembered state goes in a state file under the data dir. Runtime paths: config at `~/.config/lazyplanner/`, data at `~/.local/share/lazyplanner/` (XDG on Linux; resolved through one OS-aware helper — Windows is a secondary target).

---

## Iterative Build Workflow

1. **Before starting work**, read `main.md` to understand the spec and `log.md` to see what's already been done.
2. **Work in small increments** — one module or feature at a time.
3. **After every change**, append a dated entry to `log.md` describing what was added, changed, or fixed.
4. **Run tests and lints** after every code change: `go test ./...`, then `go vet ./...` and `staticcheck ./...`
5. **Run the program** to verify it still builds and launches: `go build ./...` (and `go run ./cmd/lazyplanner` for manual checks).
6. **Keep `README.md` current** — if the change altered user-visible behavior, usage, or build steps, update the README in the same increment.
7. **Commit often** with descriptive messages: `git add . && git commit -m "feat: ..."`

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
