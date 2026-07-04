# LazyPlanner — Build Specification

> **Purpose**: This document is the single source of truth for LazyPlanner. It defines the project identity, current state, architecture, and the incremental build plan.
>
> **Status**: Spec complete and code-ready (2026-07-04). Implementation has not started — begin at Build Plan step 1. The UI Design section is a v1 draft to refine against real screens during build steps 6–8.

---

## Project Identity

- **Name**: LazyPlanner
- **Version**: 0.0.1
- **Module**: `github.com/littekge/LazyPlanner` (matches the GitHub repo)
- **Language**: Go (chosen for the Go 1 compatibility promise, single static binaries, easy ARM cross-compilation, and the lazygit-style TUI ecosystem). Minimum version: the stable release current at scaffold time, bumped only deliberately thereafter.
- **Framework/Libraries**:
  - **TUI**: tview (`rivo/tview`, on top of `gdamore/tcell`)
  - **CalDAV/iCalendar**: `emersion/go-webdav` (CalDAV client), `emersion/go-ical` (iCalendar parsing), `teambition/rrule-go` (recurrence rules)
  - **Config**: `BurntSushi/toml` (pure Go, API-stable for a decade)
- **Platform**: Terminal. **Linux is the primary target** (incl. a future Raspberry Pi dedicated terminal); Windows is a secondary compatibility build — features are tailored to Linux, and OS-specific paths go through one resolver (`os.UserConfigDir` + a data-dir helper) so the Windows build comes nearly free.
- **License**: MIT (confirmed; see `LICENSE`)
- **Docs**: `README.md` — user-facing docs (what it does, usage, build/install for Linux and Windows), **kept current as features land during the build**
- **Change Log**: `log.md`

---

## What This Program Is

LazyPlanner is a terminal-based todo-list and calendar management program. It is a full-screen interactive TUI in the style of **lazygit** — panes and views navigated entirely with the keyboard, designed to make managing tasks and a schedule fast and low-friction.

**Core Features (initial scope):**

- **CalDAV sync** — the must-have feature. Offline-first: a local cache is the working copy; syncs with a NextCloud CalDAV server in the background or on demand, so the app opens instantly and works without network. Existing calendars and todo lists on the server are imported, and changes remain accessible from the web via NextCloud.
- **Todo management** — create, edit, complete, and organize tasks. **Deep subtask hierarchy is the centerpiece feature**: arbitrary-depth nesting rendered as a collapsible tree and navigated like a file explorer, where a "folder" is simply a task with children. Fields surfaced: title, due date, status, priority, tags, notes, subtasks.
- **Calendar views** — day/week/month views showing tasks and events on a timeline
- **Recurring tasks/events** — repeat rules (daily, weekly, custom) for tasks and calendar entries

**Design Goals:**

- Lazygit-inspired UI: multi-pane layout, single-keystroke actions, discoverable keybindings, mouse support, and a `:` command mode for in-program commands
- Fast to open, fast to use — managing your day should take seconds, not minutes
- Robust and long-lasting: a single static binary with no interpreter or runtime dependencies, so OS and dependency updates don't break the program
- Fast on modest hardware — a future goal is a dedicated Raspberry Pi terminal running LazyPlanner
- A well-behaved CalDAV citizen: never corrupts or drops data it doesn't understand; other clients (phone, NextCloud web) keep working alongside it

---

## Current State

- **Spec complete (2026-07-04); no code exists yet.** The next action is Build Plan step 1 (scaffold). `log.md` records every decision made during spec development.

---

## Architecture

> A note on layout: Go does not use `src/`, `lib/`, `include/`, or a separate `test/` tree. The idiomatic layout (used by k9s, lazygit, and most Go tools) is: packages as directories, test files living **next to** the code they test (`foo.go` / `foo_test.go` — the Go toolchain requires this), fixtures in `testdata/` dirs, and no build directory (`go build` produces a single binary; output paths are gitignored).

```
LazyPlanner/
  cmd/lazyplanner/     Entry point (main.go) — thin wiring only: load config,
                       open store, hand off to UI. No logic.
  internal/            All application packages (internal/ = not importable
                       by other projects; standard for apps vs libraries)
    config/            Config file loading + validation
    model/             Core types (Event, Todo, Calendar) + recurrence
                       expansion (wraps rrule-go). Pure data + logic; no I/O.
    store/             The vdir cache: read/write .ics files on disk,
                       sync-state JSON sidecar, in-memory index for
                       date-range and todo queries. Uses go-ical.
    caldav/            Thin CalDAV client wrapper around go-webdav
                       (auth, discovery, fetch/push of resources).
    sync/              Sync engine: diffs store vs server via ETags,
                       applies changes both ways, conflict handling.
    ui/                ALL tview/tcell code: app shell, panes, views
                       (calendar grids, todo lists), keybindings,
                       ':' command mode.
  vendor/              Vendored dependencies (committed)
  main.md  log.md  CLAUDE.md
```

**Separation rules:**

- Only `internal/ui` may import tview/tcell. Everything else must compile and test headlessly.
- `internal/model` does no I/O — pure types and logic, fully unit-testable.
- `internal/ui` never touches disk or network directly; it calls into `store` and `sync`.
- `store` is the only package that reads/writes the cache directory; `caldav` is the only package that talks HTTP.
- Test fixtures (sample `.ics` files, mini vdir trees) live in `testdata/` inside the package that uses them.

### Runtime File Locations

The repo layout above is source code only; at runtime the program is a single binary that reads/writes per-user paths. All path resolution goes through one helper so other OSes come free.

| What | Linux (primary) | Windows (secondary) |
|---|---|---|
| Config | `~/.config/lazyplanner/config.toml` (`$XDG_CONFIG_HOME`) | `%APPDATA%\lazyplanner\config.toml` |
| Calendar data (vdir cache + sync-state sidecar) | `~/.local/share/lazyplanner/calendars/<calendar>/<uid>.ics` (`$XDG_DATA_HOME`) | `%LOCALAPPDATA%\lazyplanner\calendars\...` |

The vdir data lives under *data* paths, **not** `~/.cache` — it can hold offline edits not yet synced to the server, so it must never be treated as disposable.

---

## UI Design (v1 draft — refine during build steps 6–8)

### Layout: lazygit-style three-region screen

```
┌─1 Calendars──┐┌─Main──────────────────────────┐┌─Detail─────────┐
│ Personal     ││                               ││                │
│ School       ││  Content follows the focused  ││  Selected      │
│ Work         ││  left panel:                  ││  event or      │
├─2 Tasks──────┤│                               ││  task:         │
│ ▾ School     ││  focus 1 → calendar grid      ││  title, when,  │
│   ▾ ECE384   ││            (month/week/day)   ││  location,     │
│     ☐ Lab 3  ││  focus 2 → zoomed task tree   ││  priority,     │
│   ▸ Thesis   ││  focus 3 → day agenda         ││  tags, ⏰,     │
├─3 Agenda─────┤│                               ││  notes         │
│ 2:30p Standup││                               ││                │
│ ☐ Grade labs ││                               ││                │
└──────────────┘└───────────────────────────────┘└────────────────┘
 a:add  e:edit  space:done  ::cmd  ?:help       ✓ synced 2m ago
```

- **Left column** — three small focusable panels: **Calendars** (list, with visibility toggles), **Tasks** (the subtask tree), **Agenda** (today's events + due tasks). Number keys jump focus; the Main pane's content follows focus.
- **Main pane** — the large workspace: calendar grid (month default, week/day switchable), the task tree zoomed view, or the day agenda.
- **Detail pane** — always shows the selected item's full fields (event: time/location/reminders/notes; task: due/priority/tags/notes).
- **Status bar** — contextual key hints + sync status (`✓ synced 2m ago`, `↻ syncing`, `⚠ 2 conflicts`, `⚠ offline`).

### Task tree: full tree + zoom

The whole hierarchy renders as a collapsible tree (`→`/`←` expand/collapse). `>` **zooms** — re-roots the view at the selected task like `cd`-ing into a directory (breadcrumb shows `School / ECE384`); `<` zooms back out. Lists (CalDAV calendars) are the root level; a "folder" is any task with children.

### Creation: quick-add with smart date parsing

`a` opens a one-line input scoped to the current context (task under the selected tree node; event on the selected day). Tokens parsed from the text: dates ("fri", "jul 20", "tomorrow"), times ("5pm"), `!high`/`!1`–`!9` priority, `#tag`. Everything unparsed becomes the title. `e` on any item opens the full form for detailed editing. Parsing rules must be predictable and documented in `:help` — when in doubt, leave text in the title rather than guess.

### Colors, completed tasks, sorting, undo

- **Colors**: draw with the terminal's standard 16-color palette so LazyPlanner inherits the terminal theme and renders correctly everywhere (including a bare Pi console/TTY) — lazygit's approach. Server calendar colors are mapped to the nearest palette color.
- **Completed tasks**: hidden from the tree by default (keeps deep trees clean); `.` toggles showing them struck-through in place — the dotfiles gesture, fitting the file-explorer metaphor. Completion state always remains in the data and on the server.
- **Sibling task order**: smart sort — due date (soonest first), then priority, then title. Predictable and zero-maintenance; the sort key can become configurable later. Manual ordering rejected: iCal has no standard order field, so hand-arranged order wouldn't reliably survive other clients.
- **Undo**: session-scoped undo stack on the `u` key — every local mutation (edit, delete, complete, re-parent) pushes the prior `.ics` version onto an in-memory stack. Cheap on this storage model, and the safety net that makes single-key actions trustworthy. Persistent trash deferred unless it proves needed.

### Keybindings (draft — hardcoded v1; config `[keys]` section possible later)

| Key | Action |
|---|---|
| `↑↓←→` / `hjkl` | Move within pane / expand-collapse tree nodes |
| `1` `2` `3` | Focus Calendars / Tasks / Agenda panel |
| `Tab` / `Shift-Tab` | Cycle pane focus |
| `Enter` | Select / open in Main |
| `a` | Quick-add (contextual) |
| `e` | Edit selected (full form) |
| `Space` | Toggle task done |
| `d` | Delete selected (with confirm) |
| `>` / `<` | Zoom into / out of task subtree |
| `H` / `L` | Outdent / indent task (re-parent) |
| `u` | Undo last local change (session stack) |
| `.` | Show/hide completed tasks |
| `v` | Cycle calendar view: month → week → day |
| `n` / `p` | Next / previous month(/week/day) |
| `t` | Jump to today |
| `g` | Go to date (smart-parsed input) |
| `/` | Filter/search current pane |
| `S` | Sync now |
| `:` | Command mode |
| `?` | Help overlay |
| `q` / `Esc` | Quit / back out (zoom, dialogs) |

### `:` commands (draft)

`:sync` · `:config` (open in `$EDITOR`, reload on exit) · `:view month|week|day` · `:goto <date>` (smart-parsed) · `:search <text>` · `:calendar new|rename|color|hide|show` (server-side via sync where applicable) · `:conflicts` (list/resolve conflicted items) · `:help` · `:q`

### Mouse

Click focuses panes and selects items; click `▸`/`▾` expands/collapses; double-click opens the edit form; scroll wheel scrolls panes and pages the calendar grid.

---

## Build Plan

Incremental steps; each ends with passing tests (`go test ./...`, vet, staticcheck) and a buildable program. Log every step in `log.md`.

1. **Scaffold** — `go mod init github.com/littekge/LazyPlanner`, directory skeleton, vendor setup, `.gitignore`, CI (GitHub Actions running test/vet/staticcheck on push), and a hello-world tview window that opens and quits cleanly (`q` / `Ctrl-C`). Proves the toolchain end to end.
2. **Core model** — `internal/model` types (Event, Todo, Calendar) parsed from `.ics` via go-ical; tests against fixture files covering the basics (all-day vs timed events, todos with due dates, timezones).
3. **Recurrence** — RRULE expansion via rrule-go wrapped behind a model API ("occurrences of X between dates A–B"); timezone-aware; heavily tested (recurrence is a classic bug farm).
4. **vdir store** — `internal/store`: read a vdir tree into the in-memory index, atomic writes back to disk (write-temp-then-rename), sync-state sidecar read/write; tests against fixture vdir trees.
5. **CalDAV one-way import** — `internal/caldav` + a first `:import`-style path: connect to NextCloud, discover calendars/todo lists, download everything into the vdir. Doing this *before* building real UI validates the model against real-world NextCloud data early, when fixing parsing assumptions is cheap.
6. **UI shell (read-only)** — pane layout, navigation between panes, a todo-list view and a simple agenda view over real imported data.
7. **Calendar views** — month / week / day grids with movement keys.
8. **Editing** — create / edit / complete / delete todos and events from the UI; writes go to the local vdir only.
9. **Two-way sync** — ETag-based diff, push local changes, pull remote changes, conflict handling, manual sync trigger. **This completes the must-have feature.**
10. **Command mode & keybinding polish** — `:` command line, single-key shortcut coverage, help screen, mouse support pass.
11. **Recurrence editing semantics** — "this occurrence / this and future / all" editing flows.
12. **Background sync + polish** — periodic sync, sync status indicator, error surfacing in the UI.
13. **Raspberry Pi target** — ARM cross-compile, performance check on hardware, dedicated-terminal (kiosk) setup notes.

---

## Settled Decisions

- **Language**: Go — best fit for the four driving requirements (lazygit-style TUI ecosystem, long-term stability via the Go 1 compatibility promise, workable CalDAV libraries, fast + trivial ARM cross-compilation). Rust was the runner-up; Python ruled out on robustness/speed despite having the most mature CalDAV library.
- **Sync model**: Offline-first with a local cache; NextCloud CalDAV server is the source of truth for sync.
- **TUI library**: tview — years of backwards-compatible stability (vs Bubble Tea's breaking v2 + module-path move to a vanity domain in July 2026), a widget set (Table/Grid/Flex/InputField/Pages) that maps naturally onto calendar and task views, and k9s as proof that the target UX (`:` command mode, single-key shortcuts, mouse, full-screen panes) works on it. gocui ruled out: effectively a lazygit-internal library now.
- **Config file**: TOML (via `BurntSushi/toml`), **moderate scope** — server connection, appearance and behavior options (first day of week, default view, date/time formats, sync interval), and *optional per-calendar local overrides* (hide locally, override color locally). Keybindings hardcoded for now but the schema is structured so a `[keys]` section can be added later without breaking existing configs. TOML chosen over INI (no standard spec), YAML (footgunny spec, heavy dependency), and JSON (no comments, hand-edit hostile).
- **Default values match the owner's workflow** — every moderate-scope option remains fully configurable in the config file, but the *default* each option takes when unset is the owner's preference, so a working config needs nothing beyond the `[server]` section: week starts **Monday**, **12-hour** times (2:30pm), **month view** on open, **US dates** (07/04/2026), sync all calendars with server names/colors. The first-run generated config lists every option set to its default (commented), ready to change. The one unavoidable edit is filling in the server connection.
- **Config editing model**: the config file is hand-edited; the app reads it at startup and **never writes it**. Two conveniences are planned: (1) on first run with no config, generate a fully-commented default `config.toml` documenting every option; (2) a `:config` command that opens the file in `$EDITOR` and reloads on exit. Auto-reload via file-watching was considered and **rejected** (extra dependency + mid-operation edge cases for marginal benefit). Anything the app must remember on its own (e.g., last-used view) goes in a small state file under the data directory, never the config.
- **Calendar metadata is server-owned**: calendar identity, display name, and color are CalDAV properties on the NextCloud server, cached locally in the vdir (sidecar convention) — they are data, not config. Renaming/recoloring a calendar in-app updates the server via sync (propagating to NextCloud web and other clients); creating a calendar in-app issues a CalDAV **MKCALENDAR** request and deleting one issues **DELETE**. (go-webdav's client does not expose calendar creation, so LazyPlanner sends these over its own authenticated HTTP client, held in `internal/caldav`; verified working against NextCloud. Creating in NextCloud web remains a fallback but is not needed.) Default behavior with no calendar config sections: sync all calendars using server names/colors.
- **Credentials**: always a NextCloud **app password** (Settings → Security), never the real account password — revocable per-app. Stored in `config.toml`, which must be `0600` (the app warns on looser permissions). Escape hatch: an optional `password_command` whose stdout is the secret — with the owner's Vaultwarden server, that's `password_command = "bw get password lazyplanner"` (Vaultwarden speaks the Bitwarden API, so the standard `bw` CLI works). OS keyring rejected: daemon requirement breaks headless Pi, extra dependency, extra failure modes.
- **Conflict resolution**: ETag-based detection (conditional writes) — the app **never silently overwrites** in either direction. On a true conflict (same item edited locally and remotely between syncs), keep both versions, mark the item conflicted, and show a UI indicator; the owner resolves at leisure (pick a winner or keep both as separate items). Sync never blocks waiting for resolution. "Newest wins" and "server wins" rejected as silent data-loss paths.
- **Sync triggers**: manual `:sync` always available, plus all three automatic triggers — background sync on startup (UI opens instantly from cache, refreshes when sync lands), periodic while open (default 15 min, configurable, 0 = off), and debounced push a few seconds after local edits (other devices see changes fast; shrinks the conflict window).
- **Data model — surfaced fields**: tasks show title, due date, status, **priority** (iCal 1–9), **tags** (CATEGORIES), **notes**, and **subtasks**; events show title, start/end, all-day flag, recurrence, **location**, **notes**, and a **reminder indicator** (shows that alarms exist; LazyPlanner does not fire notifications itself — phone/NextCloud handle that). Everything else in the `.ics` round-trips untouched.
- **Subtask hierarchy**: arbitrary-depth nesting via `RELATED-TO` (RELTYPE=PARENT) — the same mechanism NextCloud Tasks uses, so existing nested tasks import as-is. The owner's most-used feature: the UI treats the task tree like a file explorer (collapsible nodes, indent/outdent, drill-in), and "folders" are just tasks with children — no new storage concept needed.
- **Property preservation (iron rule)**: LazyPlanner never drops or mangles iCal properties it doesn't understand (X- properties, VALARMs, other clients' metadata). Editing a known field preserves everything else byte-for-byte-equivalent. This is what keeps LazyPlanner a well-behaved CalDAV citizen.
- **Timezones**: store what the server has; always display in the system's local timezone; create new items in the local timezone; all-day items stay date-only with no timezone math.
- **Recurrence editing**: all three scopes — "only this occurrence" (RECURRENCE-ID override), "this and future" (series split), "all occurrences" (edit master) — so LazyPlanner never forces a reach for another client.
- **Local cache storage**: vdir-style raw `.ics` files (one file per event/todo, one directory per calendar — the vdirsyncer/khal convention), with a JSON sidecar for sync state (ETags, sync tokens) and an in-memory index built at startup. Chosen for the 1:1 mapping onto CalDAV resources (simplest possible sync logic), zero extra dependencies, and human-readable/debuggable storage. SQLite rejected (cgo vs huge pure-Go dep; query speed unneeded at personal-calendar scale); custom JSON rejected (lossy translation away from the data's native iCalendar format).

## Open Decisions

**None — the spec is code-ready.** All major decisions are settled (see Settled Decisions); the UI Design section is a v1 draft expected to be refined against real screens during build steps 6–8. The one flagged build-time verification is now **resolved**: `go-webdav`'s client does not support calendar creation, so LazyPlanner issues `MKCALENDAR`/`DELETE` itself (see the calendar-metadata decision above), verified working against NextCloud.
