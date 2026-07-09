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
│ School       ││  Center follows the active    ││  Full details  │
│ Work         ││  overview panel (1 / 2 / 3):  ││  of the        │
├─2 Tasks──────┤│                               ││  highlighted   │
│ School       ││  1 → calendar (month/week/day)││  item          │
│ Personal     ││  2 → selected list's tree     ││  (hidden in    │
│ Work         ││  3 → day agenda, full detail  ││  Agenda mode)  │
├─3 Agenda─────┤│                               ││                │
│ 2:30p Standup││                               ││                │
│ ☐ Grade labs ││                               ││                │
└──────────────┘└───────────────────────────────┘└────────────────┘
 a:add  e:edit  space:done  ::cmd  ?:help       ✓ synced 2m ago
```

- **Left column (the "overview")** — three small panels: **Calendars** (list, with visibility toggles), **Tasks** (the **task lists** — calendars that support todos, incl. empty ones, *not* the full tree), **Agenda** (today's events + due tasks). `c`/`t`/`a` **focus the corresponding overview panel** — the highlight lives there and moves through its rows — while the Main pane shows the matching view. `Enter` dives from the overview into the Main pane where that makes sense; `Esc` backs out to the overview.
- **Main pane** — follows the active overview panel:
  - **Calendars** → focus lands on the **Calendars list** (arrow keys highlight each calendar; `Space` hides/shows the highlighted calendar on the calendar+agenda views, remembered in the state file). The Main pane shows the calendar view: month grid (default) or the week/day hourly time-grid. `Enter` dives into the grid (arrows move days, `Enter` cycles the selected day's events, `Esc` returns to the list); `[` / `]` cycle the highlighted calendar and `{` / `}` cycle the highlighted task list from any pane (independent overview selectors), and `v`/`f`/`b`/`gt` (view/forward/back/today) work regardless of where focus sits.
  - **Tasks** → focus lands on the **Tasks list**; selecting a list opens its full collapsible subtask tree in the Main pane, with inline priority / due date / completion status.
  - **Agenda** → focus lands on the **Agenda list**; moving its highlight draws an **outline box** around the matching block in the Main pane (the same cursor style as the calendar's selected day), auto-scrolling to it. The Main pane shows the day's events and tasks with full descriptions, at **full width** (the Detail pane is hidden), scrollable when a day overflows.
- **Detail pane** — the highlighted item's full fields (event: time/location/reminders/notes; task: due/priority/tags/status/notes). **Hidden in Agenda mode** so the center gets the whole width.
- **Bottom two lines**:
  - **Status bar** (upper of the two) — three sections: **left** a general status line (results of the last action — "task created", "saved", errors — and the current context when idle), **middle** a *command view* that echoes the most recently executed action in command form (lazygit-style; populated in step 10), **right** the *sync status* (`✓ synced 2m ago`, `↻ syncing`, `⚠ 2 conflicts`, `⚠ offline`; wired in step 9). The `:` command line opens as a separate input near the top of the screen (step 10).
  - **Controls line** (very bottom) — the key hints, always visible so the basic controls never scroll away.

### Calendar views

The calendar (active when Calendars is selected) has three views, cycled with `v`:

- **Month** — a custom-drawn grid that fills the pane: one cell per day listing that day's events/tasks (with a `+N more` overflow line), today emphasized and adjacent-month days dimmed. The selected day is marked with an **outline box** (a cursor), never a solid fill, so event text stays readable. Selecting a day lets you cycle through *that day's* events; the Detail pane then shows the highlighted event/task's full info.
- **Week / Day** — an **hourly time-grid** like a conventional calendar: an hour axis down the side with events drawn as blocks sized by their duration; all-day items sit in a band across the top; overlapping events are placed side-by-side. Day view is one column, week view seven. The **full 24-hour day is scaled to fill the pane height** (hour rows grow/shrink with the window; no vertical scrolling). Tasks with a due date also appear here: a timed-due task draws a `[ ]`/`[■]` task line at its due time, an all-day-due task sits in the top all-day band, both tinted by their list's color and using the same checkbox convention as the month grid (drill-in still cycles events only). Like the month grid, a day can be **drilled into** (`Enter` on the selected day) to cycle its events — **all-day items included** (highlighted in the top band), then timed events — with the highlighted event shown and its full info in the Detail pane; `Esc` backs out. (v1 uses simple side-by-side overlap handling; proportional-lane refinement can follow.)

All timed values are stored in UTC and **displayed in the local timezone**; all-day items stay date-only. (A created "3pm" event is written as the equivalent UTC instant and rendered back as 3pm locally.)

### Task tree: lists in the overview, tree in Main

The left **Tasks** panel lists the task lists (calendars whose supported component set includes VTODO — an empty list still appears so you can add to it; when the component set is unknown, a calendar shows once it holds a todo). Selecting a list opens its full collapsible subtask tree in the **Main** pane, **rooted at the list's own name** so the top-level tasks attach to it (`→`/`←` expand/collapse), with inline priority/due/status; the Detail pane shows the highlighted task's full fields. `>` **zooms** — re-roots the Main tree at the selected task like `cd`-ing into a directory (breadcrumb shows `School / ECE384`); `<` zooms back out.

**Folders**: a task with at least one *incomplete* child is a "folder" and behaves like one:

- Rendered with a `▸`/`▾` disclosure marker in place of the `[ ]`/`[x]` checkbox (it doubles as the expand/collapse indicator).
- **Cannot be completed** while it still has incomplete children — finish or remove them first.
- **Reverts to an ordinary task** (checkbox, completable) once it has no children or all its children are complete.
- **Deleting a folder** with incomplete children requires an extra recursive confirmation and removes the whole subtree (the folder and all descendants). Deleting an ordinary task never cascades.

### Creation: quick-add with smart date parsing

Two creation targets in the task tree: **create task** makes a task at the **top level** of the current list; **create subtask** makes one **under the highlighted task**. In calendar/agenda context, creation makes an **event** on the selected/current day. **Creation is gated by the target calendar's supported component set** — events only on VEVENT-capable calendars, tasks/subtasks only on VTODO-capable lists; a calendar created with "both" supports either. The type must be *known* (declared via the server's `supported-calendar-component-set`, captured on sync, or set explicitly when created in-app): an unconfirmed type blocks creation until a sync settles it (rather than guessing from contents) — with a manual override, `i!`… (e.g. `i!e`), for when the user knows better than the missing metadata. The override applies **only** to the unknown-type case: read-only calendars and a *known* wrong type are never forced. The Calendars overview marks each calendar `[events]`/`[tasks]`/`[both]` (or `[?]` when unknown). Each has a **quick-add** form (a one-line smart-parsed input) and a **full form**; the two are reached by distinct keys (quick vs full) — interim keys now, folded into the chorded keymap in step 10.

Quick-add tokens parsed from the text: dates ("fri", "jul 20", "tomorrow", "7/20", "2026-07-20"), times ("5pm", "3:30pm", "15:00" — a bare number is never a time), `!high`/`!med`/`!low` or `!1`–`!9` priority, `#tag`. Everything unparsed becomes the title. The full form (`e` on an existing item, or the full-create key) edits every field. Parsing rules must be predictable and documented in `:help` — when in doubt, leave text in the title rather than guess.

### Colors, completed tasks, sorting, undo

- **Colors**: draw with the terminal's standard 16-color palette so LazyPlanner inherits the terminal theme and renders correctly everywhere (including a bare Pi console/TTY) — lazygit's approach. Server calendar colors are mapped to the nearest palette color. The **background is the terminal's default** everywhere (`tview.Styles.PrimitiveBackgroundColor = ColorDefault`), so text never sits in a shaded box on color schemes whose background isn't pure black; only deliberate fills (event blocks, selection highlights) use an explicit background.
- **Window chrome**: panes and dialogs use **rounded (soft) corners**; the focused pane is shown by border *color* (yellow), not a heavier line. Popups (edit/create forms, quick-add line, confirm prompt) share one look: the terminal's **default (unified) background** with **high-contrast default text** and an **accent rounded border/title**, so they read as part of the terminal theme rather than a jarring card. Because tview applies a single field style to every form field, the **focused field is marked by a `▸` caret** in the label gutter (and the focused button is reversed) rather than a per-field color.
- **Completed tasks**: hidden from the tree by default (keeps deep trees clean); `.` toggles showing them in place with a **filled checkbox** (`[■]` vs `[ ]`) — the dotfiles gesture, fitting the file-explorer metaphor. Completion state always remains in the data and on the server. **Checking off a task while completed are hidden keeps it visible** (shown done) until you leave the list — switching to another list or to the calendar/agenda — so you can see what you just did; opening/closing a popup does *not* trigger the hide.
- **Sibling task order**: smart sort — due date (soonest first), then priority, then title. Predictable and zero-maintenance; the sort key can become configurable later. Manual ordering rejected: iCal has no standard order field, so hand-arranged order wouldn't reliably survive other clients.
- **Undo**: session-scoped undo stack on the `u` key — every local mutation (edit, delete, complete, re-parent) pushes the prior `.ics` version onto an in-memory stack. Cheap on this storage model, and the safety net that makes single-key actions trustworthy. Persistent trash deferred unless it proves needed.

### Pane sizing

Layout proportions adapt automatically to terminal size (tview reflows the `Flex` tree on every resize). On top of that, two interactive controls are planned for build step 10:

- **Accordion expand** (`+` / `-`): collapse the side panels and Detail so the focused Main view (calendar grid or task tree) fills the screen, then restore them — the lazygit `+`/`_` idiom.
- **Keyboard resize** (`Ctrl-←` / `Ctrl-→`): grow/shrink the left-column and Detail widths in steps, clamped to sane minimums, via tview's `Flex.ResizeItem`.

Chosen sizes are remembered across launches in the state file under the data dir (never the config file). Mouse drag-to-resize is intentionally out of scope — LazyPlanner is keyboard-first.

### Keybindings (vim-flavored; hardcoded for now, config `[keys]` section possible later)

The keyboard interface feels like **vim, not lazygit**: single keys for panel focus and toggles, and short **chords under a prefix** for grouped actions — a which-key popup lists the continuations after a prefix, and `?` shows the full cheat sheet. Panels are on mnemonic letters (`c`/`t`/`a`), which frees the number row for vim **counts**. (History: the interim `a`/`s` create keys and `1`/`2`/`3` panel keys were replaced by this scheme; the `a` create-prefix was later moved to `i` so `a` could focus Agenda and `n`/`N` could keep their search meaning.)

| Key | Action |
|---|---|
| `↑↓←→` / `hjkl` | Move within pane / expand-collapse tree nodes |
| `<count>` + motion | Repeat a motion (`3j`, `5k`) |
| `gg` / `G` | Go to top / bottom of the list or tree (`<count>G` → nth item) |
| `c` `t` `a` | Focus Calendars / Tasks / Agenda panel |
| `Tab` / `Shift-Tab` | Cycle pane focus |
| `+` / `-` | Expand / restore the Main pane (accordion) |
| `Ctrl-←` / `Ctrl-→` | Widen / narrow the focused side pane (keyboard resize) |
| `Enter` | Select / open in Main (drill into a day and cycle its events) |
| `i` prefix | Create: `it`/`iT` task, `ie`/`iE` event, `is`/`iS` subtask (Shift = full form), `ic` calendar, `il` list. `i!`… (e.g. `i!e`) forces creation on an unknown-type (`[?]`) calendar — read-only and known-wrong-type are never forced |
| `e` | Edit selected (full form) |
| `s` prefix | Quick-set a task field: `sp` priority, `sd` due date (one-line inputs; blank clears) |
| `Space` | Toggle task done — or, in Calendar view, hide/show the highlighted calendar (remembered in the state file) |
| `d` | Delete selected (item, or calendar/list when its panel is focused; recursive confirm for a non-empty folder) |
| `>` / `<` | Zoom into / out of task subtree |
| `H` / `L` | Outdent / indent task (re-parent) |
| `z` prefix | Fold: `zR` expand-all, `zM` collapse-all, `za` toggle current node |
| `u` | Undo last local change (session stack) |
| `r` | Sync now (alias for `:sync`) |
| `.` | Show/hide completed tasks |
| `v` | Cycle calendar view: month → week → day |
| `[` / `]` | Cycle the highlighted calendar (any mode; works from the grid too) |
| `{` / `}` | Cycle the highlighted task list (any mode) |
| `f` / `b` | Forward / back one period (month/week/day) |
| `g` prefix | Go: `gg` top, `gt` today, `gd` go-to-date (smart-parsed) |
| `/`, `n` / `N` | Search current view; next / previous match |
| `y` / `p` | Yank / paste a task (move between lists/parents) |
| `:` | Command mode (`:sync`, etc.) |
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
8. **Editing** — create / edit / complete / delete todos and events from the UI; writes go to the local vdir only. Separate **create-task** (top level) and **create-subtask** (under the selection) actions; **quick-add smart parser** with a toggle to a **full form** (both offered as distinct keys for now); tasks with incomplete children behave as **folders** (see Data model); session **undo**; indent/outdent (re-parent). Cosmetic pass: rounded borders, black/white dialogs, outline-box selection in the agenda (matching the calendar).
9. **Two-way sync** — ETag-based diff, push local changes, pull remote changes, conflict handling, manual sync trigger. **This completes the must-have feature.** Also: **in-app calendar / todo-list creation and deletion** (CalDAV `MKCALENDAR` / `DELETE` via `internal/caldav`) — created offline-first (a local collection made now, the server round-trip on push) — and wiring the **sync-status indicator** in the status bar. Also **namespace the local cache by account**: the vdir root becomes `<dataDir>/<account-id>/calendars/…`, where `<account-id>` is a stable id derived from the server URL + username, so changing the server connection automatically uses a separate cache and two accounts' data can never bleed together. This is essential *before* sync exists as anything more than a pull, because the sidecar's ETags/hrefs are meaningful only against the server that issued them — mixing two accounts in one cache would corrupt conflict detection. See the **Account model** decision below.
10. **Command mode & keybinding polish** — `:` command line (opens an input line near the top; the status bar's middle "command view" echoes the most recently executed action, lazygit-style), a **vim-style chorded keymap** (group related actions under a prefix — e.g. `a` → `at` task, `as` subtask, `ac` calendar, `al` list; map as many actions as possible to short sequences), single-key shortcut coverage, help screen, mouse support pass, and interactive pane sizing (accordion expand + keyboard resize; chosen sizes remembered in the state file).
11. **Recurrence editing semantics** — "this occurrence / this and future / all" editing flows.
12. **Background sync + polish** — periodic sync; **incremental sync** via the CalDAV `sync-collection` REPORT and the collection CTag (use the stored sync token / short-circuit on "nothing changed" instead of a full calendar-query re-download every sync — matters for the Pi target and larger calendars; the sidecar already carries a `sync_token` field for this); sync status indicator; error surfacing in the UI.
13. **Raspberry Pi target** — ARM cross-compile, performance check on hardware, dedicated-terminal (kiosk) setup notes.

---

## Settled Decisions

- **Language**: Go — best fit for the four driving requirements (lazygit-style TUI ecosystem, long-term stability via the Go 1 compatibility promise, workable CalDAV libraries, fast + trivial ARM cross-compilation). Rust was the runner-up; Python ruled out on robustness/speed despite having the most mature CalDAV library.
- **Sync model**: Offline-first with a local cache; NextCloud CalDAV server is the source of truth for sync.
- **TUI library**: tview — years of backwards-compatible stability (vs Bubble Tea's breaking v2 + module-path move to a vanity domain in July 2026), a widget set (Table/Grid/Flex/InputField/Pages) that maps naturally onto calendar and task views, and k9s as proof that the target UX (`:` command mode, single-key shortcuts, mouse, full-screen panes) works on it. gocui ruled out: effectively a lazygit-internal library now.
- **Config file**: TOML (via `BurntSushi/toml`), **moderate scope** — server connection, appearance and behavior options (first day of week, default view, date/time formats, sync interval), and *optional per-calendar local overrides* (hide locally, override color locally). Keybindings hardcoded for now but the schema is structured so a `[keys]` section can be added later without breaking existing configs. TOML chosen over INI (no standard spec), YAML (footgunny spec, heavy dependency), and JSON (no comments, hand-edit hostile).
- **Default values match the owner's workflow** — every moderate-scope option remains fully configurable in the config file, but the *default* each option takes when unset is the owner's preference, so a working config needs nothing beyond the `[server]` section: week starts **Monday**, **12-hour** times (2:30pm), **month view** on open, **US dates** (07/04/2026), sync all calendars with server names/colors. The first-run generated config lists every option set to its default (commented), ready to change. The one unavoidable edit is filling in the server connection.
- **Config editing model**: the config file is hand-edited; the app reads it at startup and **never writes it**. Two conveniences are planned: (1) on first run with no config, generate a fully-commented default `config.toml` documenting every option; (2) a `:config` command that opens the file in `$EDITOR` and reloads on exit. Auto-reload via file-watching was considered and **rejected** (extra dependency + mid-operation edge cases for marginal benefit). Anything the app must remember on its own (e.g., last-used view) goes in a small state file under the data directory, never the config.
- **Calendar metadata is server-owned**: calendar identity, display name, and color are CalDAV properties on the NextCloud server, cached locally in the vdir (sidecar convention) — they are data, not config. Renaming/recoloring a calendar in-app updates the server via sync (propagating to NextCloud web and other clients); conversely, the color is **pulled** on sync (a Depth-1 `calendar-color` PROPFIND, since go-webdav's `FindCalendars` doesn't surface it) and mapped to the nearest terminal-palette color for display — so colors stay consistent both ways, with the server authoritative except when a local color edit is still pending a push (that edit wins until pushed, never silently clobbered). creating a calendar in-app issues a CalDAV **MKCALENDAR** request and deleting one issues **DELETE**. (go-webdav's client does not expose calendar creation, so LazyPlanner sends these over its own authenticated HTTP client, held in `internal/caldav`; verified working against NextCloud. Creating in NextCloud web remains a fallback but is not needed.) Default behavior with no calendar config sections: sync all calendars using server names/colors.
- **Credentials**: always a NextCloud **app password** (Settings → Security), never the real account password — revocable per-app. Stored in `config.toml`, which must be `0600` (the app warns on looser permissions). Escape hatch: an optional `password_command` whose stdout is the secret — with the owner's Vaultwarden server, that's `password_command = "bw get password lazyplanner"` (Vaultwarden speaks the Bitwarden API, so the standard `bw` CLI works). OS keyring rejected: daemon requirement breaks headless Pi, extra dependency, extra failure modes.
- **Conflict resolution**: ETag-based detection (conditional writes) — the app **never silently overwrites** in either direction. On a true conflict (same item edited locally and remotely between syncs), keep both versions, mark the item conflicted, and show a UI indicator; the owner resolves at leisure (pick a winner or keep both as separate items). Sync never blocks waiting for resolution. "Newest wins" and "server wins" rejected as silent data-loss paths.
- **Sync triggers**: manual `:sync` always available, plus all three automatic triggers — background sync on startup (UI opens instantly from cache, refreshes when sync lands), periodic while open (default 15 min, configurable, 0 = off), and debounced push a few seconds after local edits (other devices see changes fast; shrinks the conflict window).
- **Data model — surfaced fields**: tasks show title, due date, status, **priority** (iCal 1–9), **tags** (CATEGORIES), **notes**, and **subtasks**; events show title, start/end, all-day flag, recurrence, **location**, **notes**, and a **reminder indicator** (shows that alarms exist; LazyPlanner does not fire notifications itself — phone/NextCloud handle that). Everything else in the `.ics` round-trips untouched.
- **Subtask hierarchy**: arbitrary-depth nesting via `RELATED-TO` (RELTYPE=PARENT) — the same mechanism NextCloud Tasks uses, so existing nested tasks import as-is. The owner's most-used feature: the UI treats the task tree like a file explorer (collapsible nodes, indent/outdent, drill-in), and "folders" are just tasks with children — no new storage concept needed.
- **Property preservation (iron rule)**: LazyPlanner never drops or mangles iCal properties it doesn't understand (X- properties, VALARMs, other clients' metadata). Editing a known field preserves everything else byte-for-byte-equivalent. This is what keeps LazyPlanner a well-behaved CalDAV citizen.
- **Timezones**: store what the server has; always display in the system's local timezone; create new items in the local timezone; all-day items stay date-only with no timezone math. **Robustness**: the IANA tz database is embedded in the binary (`import _ "time/tzdata"`) so zones resolve on any OS (minimal Pi image, Windows). A TZID that Go can't load — an Outlook/Windows zone name (e.g. `Eastern Standard Time`) or a custom `VTIMEZONE` label — is mapped via the CLDR windowsZones table, and if still unresolved the value is kept as floating/local time rather than dropping the item. LazyPlanner never silently loses an event/todo over an unfamiliar timezone.
- **Recurrence editing**: all three scopes — "only this occurrence" (RECURRENCE-ID override), "this and future" (series split), "all occurrences" (edit master) — so LazyPlanner never forces a reach for another client.
- **Local cache storage**: vdir-style raw `.ics` files (one file per event/todo, one directory per calendar — the vdirsyncer/khal convention), with a JSON sidecar for sync state (ETags, sync tokens) and an in-memory index built at startup. Chosen for the 1:1 mapping onto CalDAV resources (simplest possible sync logic), zero extra dependencies, and human-readable/debuggable storage. SQLite rejected (cgo vs huge pure-Go dep; query speed unneeded at personal-calendar scale); custom JSON rejected (lossy translation away from the data's native iCalendar format).
- **Account model (single account, server-keyed cache)**: LazyPlanner is **single-account** — one `[server]` section, one user's data at a time; there is no in-app account switcher and no multi-account UI. Account switching is expected to be **rare, but must be safe**: the local vdir cache is namespaced by a stable `<account-id>` derived from the server URL + username (`<dataDir>/<account-id>/calendars/…`), so changing the server connection automatically maps to a *separate* cache and two accounts' data can never bleed into one directory. This matters because the sidecar's ETags and hrefs are meaningful only against the server that issued them — mixing two accounts in one cache would corrupt two-way sync's conflict detection and risk data loss. This is a deliberately **cheap safeguard, not a feature**: switching accounts still means editing the `[server]` connection and reopening; the app simply guarantees the caches stay isolated. **Full multi-account profiles** (multiple `[[account]]` blocks + an in-app `:account` switcher, each with its own namespaced cache and credentials) are a possible **future enhancement, explicitly out of initial scope**. The account-keyed cache path is wired in with two-way sync (Build Plan step 9), since that is when a stale/mismatched cache first becomes dangerous.

- **Read-only calendars**: some server calendars grant no write privilege — notably NextCloud's generated **Contact Birthdays** calendar (`contact_birthdays`), and read-only shares/subscriptions. LazyPlanner **detects** these via a `current-user-privilege-set` PROPFIND during discovery (a calendar lacking `write`/`write-content`/`bind`/`all` is read-only; issued over `internal/caldav`'s own HTTP client since go-webdav's client doesn't expose privileges), caches the flag in the sidecar, and **never writes to them**: the UI blocks create/edit/complete/delete/re-parent (marking the calendar `[ro]`), and sync treats them **pull-only** — mirroring the server one-way and discarding any local change that can't be pushed (matching how the NextCloud web UI itself forbids edits there). A write refused with HTTP **403** is a reactive fallback that flags the calendar read-only even if privilege discovery missed it. This keeps LazyPlanner a well-behaved CalDAV citizen (no futile writes, no silent data loss from the earlier "push → server drops → reconcile deletes locally" cycle).

## Open Decisions

**None — the spec is code-ready.** All major decisions are settled (see Settled Decisions); the UI Design section is a v1 draft expected to be refined against real screens during build steps 6–8. The one flagged build-time verification is now **resolved**: `go-webdav`'s client does not support calendar creation, so LazyPlanner issues `MKCALENDAR`/`DELETE` itself (see the calendar-metadata decision above), verified working against NextCloud.
