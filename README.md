# LazyPlanner

A terminal-based todo-list and calendar manager with offline-first CalDAV sync — a full-screen interactive TUI in the style of [lazygit](https://github.com/jesseduffield/lazygit), written in Go.

> **Status: early build.** The spec is complete (see [`main.md`](main.md)). Done so far: build steps 1–7 — the Go module, packages, vendored deps, and CI (step 1); the `model` layer parsing iCalendar events and todos (step 2); timezone-aware recurrence expansion (step 3); the local vdir cache with an in-memory index and atomic writes (step 4); one-way CalDAV import from NextCloud (step 5); a read-only TUI with a calendar subtask tree and today's agenda (step 6); **month/week/day calendar views** with movement keys (step 7); and **editing** — create/edit/complete/delete tasks and events, indent/outdent subtasks, and a session undo (step 8). Edits are written to the local cache only; **two-way sync is *not yet built*** (changes don't reach the server until step 9).

## What it does

- **Syncs with a CalDAV server** (built for NextCloud): offline-first, so the app opens instantly and works without network; changes sync both ways and stay visible from NextCloud web and your phone.
- **Todo management** with deep subtask hierarchies — arbitrary nesting, navigated like a file explorer.
- **Calendar views** — month, week, and day grids for events and dated tasks.
- **Recurring events and tasks**, including per-occurrence editing.
- Keyboard-first (single-key shortcuts + a `:` command mode), with full mouse support.

## Usage

Run `lazyplanner` with no arguments to open the TUI. It reads the local cache (populate it with `import` first — see below). A left "overview" column holds **Calendars**, **Tasks** (your task lists), and **Agenda**. `1`/`2`/`3` **focus the matching overview panel** (the highlight lives there); the **center** pane shows the corresponding view, and `Enter` dives in / `Esc` backs out:

- **`1` Calendars** → focus the calendar list on the left (arrows highlight each calendar). The center shows a month grid (each day cell lists its events/tasks) or a week/day **hourly time-grid**. `Enter` dives into the grid — arrows move days, `Enter` cycles the selected day's events (the Detail pane shows the highlighted one), `Esc` returns to the list. `[`/`]` cycle the highlighted calendar from anywhere; `v` cycles the view; `n`/`p` move by period; `t` jumps to today.
- **`2` Tasks** → pick a list on the left; its full subtask tree opens in the center (with inline priority/due/status). The Detail pane shows the highlighted task's full description and fields.
- **`3` Agenda** → focus the agenda list on the left; moving its highlight highlights the matching block in the center (which auto-scrolls). The center shows the day's events and tasks with full descriptions, at full width (the Detail pane hides).

**Creating and editing** (writes to the local cache only until two-way sync lands):

- **`a`** — quick-add. A top-level task (Tasks) or an event on the selected/current day (Calendar/Agenda). One line; it parses smart tokens and leaves anything ambiguous in the title: dates (`today`, `tomorrow`, `fri`, `jul 20`, `7/20`, `2026-07-20`), times (`3pm`, `3:30pm`, `15:00` — a bare number stays a number), `!1`–`!9` / `!high` / `!med` / `!low` priority, and `#tag`.
- **`s`** — quick-add a **subtask** under the highlighted task.
- **`A` / `S`** — the same as `a` / `s` but opening the **full form** (all fields) instead of the quick line.
- **`e`** — full edit form for the selected item. `Esc` or Cancel to back out.
- **`Space`** — toggle a task complete/incomplete. A task with unfinished subtasks is a **folder** (shown with `▸`/`▾`) and can't be completed until they are.
- **`d`** — delete the selected item (with a confirm; deleting a folder removes its whole subtree).
- **`H` / `L`** — outdent / indent the selected task (re-parent in the subtask tree).
- **`u`** — undo the last create/edit/complete/delete this session (multi-level).

Full key list:

| Key | Action |
|---|---|
| `1` `2` `3` | Focus the Calendars / Tasks / Agenda overview panel |
| `Tab` / `Shift-Tab` | Cycle those three |
| `↑` `↓` `←` `→` / `j` `k` `h` `l` | Move the highlight in the focused pane (overview rows, days in the grid, nodes in the tree) |
| `v` | Cycle calendar view: month → week → day |
| `[` / `]` | Cycle the highlighted calendar (calendar mode; works from the grid too) |
| `n` / `p` | Next / previous month·week·day |
| `t` | Jump to today |
| `Enter` | Dive into the center; on a day (month **or** week/day grid) cycle its events; open a list / expand a task |
| `Esc` | Back out to the overview (event cycling, grid, task tree) · cancel a form/dialog |
| `a` / `A` | Add task/event — quick line / full form |
| `s` / `S` | Add subtask — quick line / full form |
| `e` `d` | Edit / delete selected |
| `Space` | Toggle task done (folders can't complete until their subtasks do) |
| `H` / `L` | Outdent / indent task (re-parent) |
| `u` | Undo last local change (this session) |
| `PageUp` / `PageDown` | Scroll the week/day time-grid or the agenda |
| `.` | Show/hide completed tasks |
| `q` / `Ctrl-C` | Quit |

Navigation is still being refined — the full keymap and a `:` command mode arrive with later build steps.

### Importing your calendars (early, one-way)

You can already pull your NextCloud calendars into the local cache. Use a NextCloud **app password** (Settings → Security → Devices & sessions), never your account password:

```sh
lazyplanner import \
  --url https://cloud.example.com/remote.php/dav \
  --username you \
  --password <app-password>
```

Credentials can also come from the `LAZYPLANNER_CALDAV_URL`, `LAZYPLANNER_CALDAV_USERNAME`, and `LAZYPLANNER_CALDAV_PASSWORD` environment variables. Data is written to the OS data directory (`~/.local/share/lazyplanner/` on Linux), overridable with `--data`.

This is a one-way download (server → local) for validating against real data; it does not yet push local changes or delete anything — two-way sync comes later.

### Managing calendars (early)

LazyPlanner can create and delete calendars/task lists directly on the server (via CalDAV `MKCALENDAR`), so you never have to use the NextCloud web UI. Connection flags/env vars are the same as `import`.

```sh
lazyplanner calendar list                          # show calendars + their server paths
lazyplanner calendar create --name "Projects"      # an event calendar
lazyplanner calendar create --name "Errands" --tasks   # a task list (VTODO)
lazyplanner calendar create --name "Home" --both --color "#3366cc"
lazyplanner calendar delete --path "/remote.php/dav/calendars/you/errands/"
```

After creating a calendar, run `lazyplanner import` to pull it into the local cache.

## Build & Install

Requires [Go](https://go.dev/dl/) (the stable release current at scaffold time or newer; see the `go` directive in `go.mod`). Dependencies are vendored, so no network is needed to build.

- **Linux** (primary): `go build -o lazyplanner ./cmd/lazyplanner` — a single static binary, no runtime dependencies. Run `./lazyplanner`.
- **Windows** (secondary): `GOOS=windows go build -o lazyplanner.exe ./cmd/lazyplanner`.

Today the program opens a placeholder window; press `q` or `Ctrl-C` to quit. Real functionality lands over the build steps in [`main.md`](main.md).

Config will live at `~/.config/lazyplanner/config.toml` (Linux) / `%APPDATA%\lazyplanner\` (Windows), with a commented default generated on first run — *not yet built*.

## Development

- [`main.md`](main.md) — the build specification (single source of truth)
- [`CLAUDE.md`](CLAUDE.md) — project rules and coding standards
- [`log.md`](log.md) — the change log; every change gets an entry

## License

[MIT](LICENSE)
