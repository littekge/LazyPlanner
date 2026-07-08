# LazyPlanner

A terminal-based todo-list and calendar manager with offline-first CalDAV sync — a full-screen interactive TUI in the style of [lazygit](https://github.com/jesseduffield/lazygit), written in Go.

> **Status: early build.** The spec is complete (see [`main.md`](main.md)). Done so far: build steps 1–8 — the Go module, packages, vendored deps, and CI (step 1); the `model` layer parsing iCalendar events and todos (step 2); timezone-aware recurrence expansion (step 3); the local vdir cache with an in-memory index and atomic writes (step 4); one-way CalDAV import from NextCloud (step 5); a read-only TUI with a calendar subtask tree and today's agenda (step 6); **month/week/day calendar views** with movement keys (step 7); and **editing** — create/edit/complete/delete tasks and events, indent/outdent subtasks, and a session undo (step 8). **Step 9 (two-way sync) is complete**: a `config.toml` (generated on first run), an ETag-based **two-way sync engine** that never silently overwrites, the account-namespaced local cache, an in-app sync trigger with a sync-status indicator, and offline-first calendar/task-list creation & deletion. **Step 10 (command mode & keybinding polish) is complete**: a **vim-style chorded keymap** with a which-key popup, a `:` **command line** with a status-bar command view, a `?` **help** overlay, interactive **conflict resolution** (`:conflicts`), **pane sizing** (accordion + keyboard resize, remembered), and a mouse pass. Read-only calendars (e.g. NextCloud birthdays) are detected and respected.

## What it does

- **Syncs with a CalDAV server** (built for NextCloud): offline-first, so the app opens instantly and works without network; changes sync both ways and stay visible from NextCloud web and your phone.
- **Todo management** with deep subtask hierarchies — arbitrary nesting, navigated like a file explorer.
- **Calendar views** — month, week, and day grids for events and dated tasks.
- **Recurring events and tasks**, including per-occurrence editing.
- Keyboard-first (single-key shortcuts + a `:` command mode), with full mouse support.

## Usage

Run `lazyplanner` with no arguments to open the TUI. It reads the local cache (populate it with `import` first — see below). A left "overview" column holds **Calendars**, **Tasks** (your task lists), and **Agenda**. `c`/`t`/`a` **focus the matching overview panel** (the highlight lives there); the **center** pane shows the corresponding view, and `Enter` dives in / `Esc` backs out. Movement is vim-style — `hjkl` or arrows, a **count** prefix repeats a motion (`3j`), and `gg`/`G` jump to the top/bottom of a list or tree:

- **`c` Calendars** → focus the calendar list on the left (arrows highlight each calendar; **`Space`** hides/shows the highlighted calendar's items on the calendar and agenda — remembered across launches). The center shows a month grid (each day cell lists its events/tasks) or a week/day **hourly time-grid**. `Enter` dives into the grid — arrows move days, `Enter` cycles the selected day's events (the Detail pane shows the highlighted one), `Esc` returns to the list. `[`/`]` cycle the highlighted calendar from anywhere; `v` cycles the view; `f`/`b` move forward/back by period; `gt` jumps to today.
- **`t` Tasks** → pick a list on the left; its full subtask tree opens in the center (with inline priority/due/status). The Detail pane shows the highlighted task's full description and fields. `z` folds the tree: `zR` expand-all, `zM` collapse-all, `za` toggle.
- **`a` Agenda** → focus the agenda list on the left; moving its highlight highlights the matching block in the center (which auto-scrolls). The center shows the day's events and tasks with full descriptions, at full width (the Detail pane hides).

**Creating and editing** — create actions are grouped under the **`i` prefix** (as in "insert") pressed as a short chord; a **which-key** hint pops up after `i` so you don't have to memorize them. Capitalize the object for the full form.

- **`i` then `t` / `T`** — add a top-level **task** (quick line / full form) to the selected list.
- **`i` then `e` / `E`** — add an **event** (quick / full) on the selected/current day.
- **`i` then `s` / `S`** — add a **subtask** (quick / full) under the highlighted task.
- **`i` then `c` / `l`** — create a **calendar** / **task list**, offline-first (it appears immediately; the server `MKCALENDAR` happens on the next sync).
- Quick-add parses smart tokens and leaves anything ambiguous in the title: dates (`today`, `tomorrow`, `fri`, `jul 20`, `7/20`, `2026-07-20`), times (`3pm`, `3:30pm`, `15:00` — a bare number stays a number), `!1`–`!9` / `!high` / `!med` / `!low` priority, and `#tag`.
- **`e`** — full edit form for the selected item. **`s`** quick-sets one task field without the full form: **`sp`** priority (`1`–`9` / `high`/`med`/`low`), **`sd`** due date (smart-parsed; blank clears). **`d`** — delete: the selected item, or the calendar/list when its overview panel is focused (with a confirm; deleting a folder removes its whole subtree).
- **`Space`** — toggle a task complete/incomplete. A task with unfinished subtasks is a **folder** (`▸`/`▾`) and can't be completed until they are.
- **`H` / `L`** — outdent / indent the selected task (re-parent). **`u`** — undo the last change this session (multi-level).

**Commands, help & layout:**

- **`:`** opens a command line: `:sync`, `:view month|week|day`, `:goto <date>`, `:conflicts`, `:help`, `:q`. The status bar's middle section echoes the last action in command form. **`gd`** opens `:goto` prefilled.
- **`?`** opens the full help cheat sheet.
- **`:conflicts`** resolves items that changed on both sides (keep local / keep server); the status bar shows the live conflict count.
- **`+` / `-`** collapse / restore the overview so the calendar or tree fills the width; **`Ctrl-←` / `Ctrl-→`** narrow / widen the overview column (remembered across launches).
- **`r`** — sync now (alias for `:sync`). LazyPlanner also syncs in the background on startup; the status bar's right section shows the state (`syncing…`, `synced HH:MM`, `! N conflict(s)`, `offline`, or `not configured`).
- **Mouse**: click a panel to switch to it, click to select, double-click the tree/agenda to edit, wheel to scroll.

Full key list:

| Key | Action |
|---|---|
| `c` `t` `a` | Focus the Calendars / Tasks / Agenda overview panel |
| `Tab` / `Shift-Tab` | Cycle those three |
| `↑` `↓` `←` `→` / `j` `k` `h` `l` | Move the highlight in the focused pane |
| `<count>` + motion | Repeat a motion — `3j`, `5k` |
| `gg` / `G` | Go to top / bottom of the list or tree (`<count>G` → nth item) |
| `Enter` | Dive into the center; cycle a day's events; open a list / expand a task |
| `Esc` | Back out to the overview · cancel a form/dialog/chord |
| `i` … | Create prefix — `t`/`T` task, `e`/`E` event, `s`/`S` subtask, `c` calendar, `l` list (Shift = full form) |
| `e` | Edit selected (full form) |
| `s` … | Quick-set a task field — `p` priority, `d` due date (blank clears) |
| `d` | Delete selected item — or the calendar/list when its panel is focused |
| `Space` | Toggle task done — or hide/show the highlighted calendar (Calendar view) |
| `/` · `n` / `N` | Search the current view · next / prev match |
| `H` / `L` | Outdent / indent task (re-parent) |
| `z` … | Fold the tree — `zR` expand-all, `zM` collapse-all, `za` toggle |
| `u` | Undo last local change (this session) |
| `v` | Cycle calendar view: month → week → day |
| `[` / `]` | Cycle the highlighted calendar |
| `f` / `b` · `gt` | Forward / back one period · jump to today |
| `+` / `-` | Collapse / restore the overview (accordion) |
| `Ctrl-←` / `Ctrl-→` | Narrow / widen the overview column (remembered) |
| `r` | Sync now (= `:sync`) |
| `:` · `gd` · `?` | Command line · go to date · help |
| `.` | Show/hide completed tasks |
| `q` / `Ctrl-C` | Quit / back out |

### Configuration

On first run (no config file), LazyPlanner writes a fully-commented `config.toml` to `~/.config/lazyplanner/` (Linux) / `%APPDATA%\lazyplanner\` (Windows) and exits so you can fill in the connection. The only required section is `[server]`; every other option is shown at its default, commented out. The app **reads this file once at startup and never writes it**.

```toml
[server]
url = "https://cloud.example.com/remote.php/dav"
username = "you"
# password = "your-app-password"          # inline (keep the file chmod 600)
password_command = "bw get password lazyplanner"   # or fetch it from a command
```

Authentication is always a NextCloud **app password** (Settings → Security → Devices & sessions), never your account password. `password_command` (its stdout is used as the secret) keeps the password out of the file — e.g. `bw get password …` with Bitwarden/Vaultwarden. If the file is group/other-readable, LazyPlanner warns you to `chmod 600` it.

The local cache is **namespaced by account** (a stable id derived from the server URL + username), so changing the server connection uses a separate cache and two accounts' data never mix. Data lives under the OS data directory (`~/.local/share/lazyplanner/<account-id>/` on Linux).

### Syncing

Once `[server]` is set, LazyPlanner syncs **both ways** on startup and whenever you press `r` (or run the `sync` command below). Sync is ETag-based and **never silently overwrites**: it pushes local creates/edits/deletes, pulls remote changes, and when the same item changed on both sides it keeps both versions and flags the conflict — resolve them in-app with `:conflicts` (keep local / keep server).

**Read-only calendars** (like NextCloud's generated "Contact Birthdays" calendar, or read-only shares) are detected automatically and marked `[ro]` in the overview. LazyPlanner never writes to them — creating/editing/deleting there is blocked with a hint, and sync mirrors them one-way — exactly as the NextCloud web UI treats them.

```sh
lazyplanner sync      # two-way sync of the local cache against the server
lazyplanner import    # one-way pull only (server → local), e.g. for a first seed
```

Both take the same connection flags as below (or the `LAZYPLANNER_CALDAV_URL` / `LAZYPLANNER_CALDAV_USERNAME` / `LAZYPLANNER_CALDAV_PASSWORD` environment variables), and honor `--data` to override the data directory:

```sh
lazyplanner sync \
  --url https://cloud.example.com/remote.php/dav \
  --username you \
  --password <app-password>
```

### Managing calendars (early)

You can create and delete calendars/task lists in-app (the `c` / `D` keys — offline-first), so you never need the NextCloud web UI. These CLI subcommands do the same directly on the server (via CalDAV `MKCALENDAR` / `DELETE`); connection flags/env vars are the same as `import`.

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

On first launch LazyPlanner writes a starter `config.toml` (see [Configuration](#configuration)) and exits; fill in `[server]` and run it again to open the TUI. Press `q` or `Ctrl-C` to quit. Remaining functionality lands over the build steps in [`main.md`](main.md).

## Development

- [`main.md`](main.md) — the build specification (single source of truth)
- [`CLAUDE.md`](CLAUDE.md) — project rules and coding standards
- [`log.md`](log.md) — the change log; every change gets an entry

## License

[MIT](LICENSE)
