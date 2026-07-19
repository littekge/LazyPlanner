# LazyPlanner

A terminal-based todo-list and calendar manager with offline-first CalDAV sync — a full-screen interactive TUI in the style of [lazygit](https://github.com/jesseduffield/lazygit), written in Go.

## What it does

- **Syncs with a CalDAV server** (built for NextCloud): offline-first, so the app opens instantly and works without network; changes sync both ways and stay visible from NextCloud web and your phone.
- **Todo management** with deep subtask hierarchies — arbitrary nesting, navigated like a file explorer.
- **Calendar views** — month, week, and day grids for events and dated tasks.
- **Recurring events and tasks**, including per-occurrence editing.
- Keyboard-first (single-key shortcuts + a `:` command mode), with full mouse support.

## Build & Install

Requires [Go](https://go.dev/dl/) (see the `go` directive in `go.mod` for the minimum version). Dependencies are vendored, so no network is needed to build.

- **Linux** (primary): `go build -o lazyplanner ./cmd/lazyplanner` — a single static binary, no runtime dependencies. Run `./lazyplanner`.
- **Windows** (secondary): `GOOS=windows go build -o lazyplanner.exe ./cmd/lazyplanner`.

On first launch LazyPlanner writes a starter `config.toml` (see [Configuration](#configuration) below) and exits; fill in `[server]` and run it again to open the TUI. Press `q` or `Ctrl-C` to quit.

A `Makefile` wraps the common tasks: `make build` (native binary), `make run`, and `make cross` (the Raspberry Pi binaries — see [Raspberry Pi](#raspberry-pi--dedicated-terminal)). `make build` and `make cross` **stamp the version** from the current git tag (so `lazyplanner version` reports e.g. `v1.0.0`); a plain `go build` leaves it as `dev`.

## Configuration

On first run (no config file), LazyPlanner writes a fully-commented `config.toml` to `~/.config/lazyplanner/` (Linux) / `%APPDATA%\lazyplanner\` (Windows) and exits so you can fill in the connection. The only required section is `[server]`; every other option is shown at its default, commented out. The app **reads this file once at startup and never writes it**.

```toml
[server]
url = "https://cloud.example.com/remote.php/dav"
username = "you"
# password = "your-app-password"          # inline (keep the file chmod 600)
password_command = "bw get password lazyplanner"   # or fetch it from a command
```

Authentication is always a NextCloud **app password** (Settings → Security → Devices & sessions), never your account password. `password_command` (its stdout is used as the secret) keeps the password out of the file — e.g. `bw get password …` with Bitwarden/Vaultwarden. If the file is group/other-readable, LazyPlanner warns you to `chmod 600` it.

The `[appearance]` section tunes display (all optional): `first_day_of_week`, `default_view`, `time_format`, `date_format`, and **`color_mode`** — how calendar colors render. `color_mode` defaults to `auto` (exact 24-bit truecolor, which your terminal downsamples to 256 or 16 colors as needed); set it to `truecolor` to force 24-bit on a terminal that underreports, `16` to use the nearest themed ANSI color (inherits your terminal theme — good for a light terminal or bare console), or `off` for no calendar colors.

The local cache is **namespaced by account** (a stable id derived from the server URL + username), so changing the server connection uses a separate cache and two accounts' data never mix. Data lives under the OS data directory (`~/.local/share/lazyplanner/<account-id>/` on Linux).

## Usage

Run `lazyplanner` with no arguments to open the TUI. It reads the local cache (populate it with `import` first — see [Syncing](#syncing)). A left "overview" column holds **Calendars**, **Tasks** (your task lists), and **Agenda**. `c`/`t`/`a` **focus the matching overview panel** (the highlight lives there); the **center** pane shows the corresponding view, and `Enter` dives in / `Esc` backs out. Movement is vim-style — `hjkl` or arrows, a **count** prefix repeats a motion (`3j`), and `gg`/`G` jump to the top/bottom of a list or tree:

- **`c` Calendars** → focus the calendar list on the left (arrows highlight each calendar; **`Space`** hides/shows the highlighted calendar's items on the calendar and agenda — remembered across launches). Each row shows a **color dot** (the calendar's exact server color in truecolor — automatically downsampled to 256/16 colors on terminals that can't do 24-bit, and configurable via `color_mode`; a hidden calendar drops the dot) and what the calendar can hold — **`[events]`**, **`[tasks]`**, or **`[both]`** (and **`[?]`** when the type isn't known until the next sync). That color also tints the calendar's events and tasks across the month grid, time-grid, and agenda, so each calendar is recognizable at a glance and matches NextCloud. The center shows a month grid (each day cell lists its events/tasks) or a week/day **hourly time-grid** (which also shows due tasks — a `[ ]`/`[■]` task line at the due time, or in the top all-day band for all-day-due tasks — in the list's color, matching the month grid). `Enter` dives into the grid. Un-drilled, `←`/`→` move between days (up/down do nothing). Once drilled, navigation is **2D over the day's layout**: `↑`/`↓` move by time and `←`/`→` move between overlapping side-by-side events (so two 12–1pm events are reachable left/right); `f`/`b` change the day/week (staying drilled), `Esc` returns to the list. `[`/`]` cycle the highlighted calendar and `{`/`}` cycle the highlighted task list — both from any pane; `v` cycles the view; `f`/`b` move forward/back by period; `gt` jumps to today. In week/day view the hours are evenly spaced (each hour a uniform height), and `+`/`-` zoom that height in/out — scrolling the day when it's taller than the pane and remembering the zoom across launches.
- **`t` Tasks** → pick a list on the left; its full subtask tree opens in the center (with inline priority/due/status). The Detail pane shows the highlighted task's full description and fields. `z` folds the tree: `zR` expand-all, `zM` collapse-all, `za` toggle. **`>`** zooms into the selected task's subtree (re-roots the tree, with a `List / Task` breadcrumb, like `cd`-ing into a folder); **`<`** zooms back out one level.
- **`a` Agenda** → focus the agenda list on the left; moving its highlight highlights the matching block in the center (which auto-scrolls). The center shows the day's events and tasks with full descriptions, at full width (the Detail pane hides).

**Creating and editing** — create actions are grouped under the **`i` prefix** (as in "insert") pressed as a short chord; a **which-key** hint pops up after `i` so you don't have to memorize them. Capitalize the object for the full form.

- **`i` then `t` / `T`** — add a top-level **task** (quick line / full form) to the selected list.
- **`i` then `e` / `E`** — add an **event** (quick / full) on the selected/current day.
- **`i` then `s` / `S`** — add a **subtask** (quick / full) under the **selected task** — the tree node in Tasks, or a task you've drilled into in a calendar/agenda view. The subtask is created in the parent task's own list, whatever pane you're in.
- **`i` then `c` / `l`** — create a **calendar** / **task list**, offline-first (it appears immediately; the server `MKCALENDAR` happens on the next sync). The form has a **Color** field with a **Pick color…** button that opens a grid of preset swatches (`hjkl`/arrows, `Enter` to pick) plus a **Custom hex…** entry — so a new calendar is colored from the start (the field is pre-filled with a default blue, so a created calendar/list always has a color instead of being left the app default until you fix it in NextCloud). **`e`** on the Calendars pane opens the same form to edit an existing calendar's name and color — and `e` on the Tasks pane edits the highlighted list's, symmetric with `d` (which deletes the focused pane's collection).
- Creation is **locked to the calendar's type**: events can only be added to `[events]` or `[both]` calendars, and tasks/subtasks only to `[tasks]` or `[both]` lists — a wrong-type attempt is refused with a message. A calendar whose type isn't yet confirmed (`[?]`, e.g. imported by another tool and not yet synced) blocks creation until a sync settles it — unless you **force** it with `i!` (e.g. **`i!e`** to add an event, **`i!t`** a task) when you know the calendar is fine. The force only covers the unknown-type case: read-only calendars and a *known* wrong type are never forced.
- Quick-add parses smart tokens and leaves anything ambiguous in the title: dates (`today`, `tomorrow`, `fri`, `jul 20`, `7/20`, `2026-07-20`), times (`3pm`, `3:30pm`, `15:00` — a bare number stays a number), `!1`–`!9` / `!high` / `!med` / `!low` priority, and `#tag`.
- **`e`** — full edit form for the selected item. **`s`** quick-sets one task field without the full form: **`sp`** priority (`1`–`9` / `high`/`med`/`low`), **`sd`** due date (smart-parsed; blank clears) — on the selected task in **any** view (tree, agenda, or a task drilled into in the calendar), like `Space`/`e`/`d`. **`d`** — delete: the selected item, or the calendar/list when its overview panel is focused (with a confirm; deleting a folder removes its whole subtree).
- **`Space`** — toggle a task complete/incomplete. This works on the selected task in **any** view: the tree, the agenda, or a task you've drilled into in the month/week/day calendar (drill with `Enter`, then `Space`). A task with unfinished subtasks is a **folder** — shown with a `▸` caret (instead of a checkbox) in the tree **and** the calendar/agenda — and can't be completed until they are. A folder keeps its own due date, so it still appears on the calendar; adding a subtask to a dated task just swaps its `[ ]` for `▸`. (In a calendar view with no task drilled, `Space` instead hides/shows the highlighted calendar; drilled into an *event*, `Space` just flashes a reminder that events can't be completed, rather than flipping visibility.)
- **`H` / `L`** — outdent / indent the selected task (re-parent). **`y`** cuts (move) a task and its subtree, **`Y`** copies it; **`p`** pastes under the selected task and **`P`** at the list's top level. A cut moves it (same list = re-parent, other list = move to that calendar); a copy duplicates it with fresh UIDs, leaving the original. The clipboard **persists after a paste**, so you can paste the same item multiple times. **`m`** — **grab mode**: temporally manipulate the selected item. On an **event** (week/day view) `j`/`k` move it ±an hour, `h`/`l` ±a day, and `J`/`K` resize its end; on a **task** `j`/`k` nudge its due date ±a day and `h`/`l` ±a week. `Enter` keeps the change, `Esc` reverts. (Undated tasks are skipped; a recurring event prompts for scope first — this occurrence / this & future / all.) **`u`** — undo the last change this session (multi-level).

**Recurring items.** Editing (`e`), deleting (`d`), or grabbing (`m`) a recurring **event** opens a scope picker — **This occurrence** (writes a `RECURRENCE-ID` override / `EXDATE`), **This & future** (splits the series at that point, preserving a bounded count), or **All** (edits the master). A recurring **task** shows as a single live instance at its current due; completing it (`Space`) advances it to the next occurrence (the way NextCloud rolls a repeating task forward) — the flash confirms it advanced rather than being checked off, and it's marked done only when the series runs out. Editing "this occurrence" of a task detaches that instance as a separate one-off task (after a confirmation) and advances the rest. All of it is undo-able and syncs like any other change.

**Commands, help & layout:**

- **`:`** opens a command line: `:sync`, `:view month|week|day`, `:goto <date>`, `:search <text>`, `:config`, `:conflicts`, `:help`, `:q`. The status bar's middle section echoes the last action in command form. **`gd`** opens `:goto` prefilled.
- **`:config`** opens `config.toml` in your `$EDITOR` (the TUI suspends) and reloads it on exit; server/credential edits and `color_mode` changes take effect immediately (switching `auto`↔`truecolor` still needs a restart, since 24-bit output is negotiated at startup; changing to a different account also needs a restart, since the cache is per-account).
- **`:calendar new`** opens the create-calendar form (the command form of `ic`). **`:calendar rename <name>`** / **`:calendar color <#rrggbb>`** change the highlighted calendar's server-owned display name / color (offline-first: applied locally now, pushed to the server via a CalDAV `PROPPATCH` on the next sync, so it propagates to NextCloud web and other clients). **`:calendar color` with no hex** opens the swatch **color picker** directly (a quick recolor); **`e`** while the Calendars (or Tasks) pane is focused opens the full edit form (name + color) for that calendar/list. Colors sync **both ways** — a color set from NextCloud web (or another client) is pulled in on the next sync and applied, and a local edit you haven't pushed yet is never overwritten by the pull. **`:calendar hide`** / **`:calendar show`** are the command form of the `Space` visibility toggle.
- **`?`** opens the full help cheat sheet.
- **Mode indicator**: the status bar shows a vim-style **mode badge** at its far left — `NORMAL` at rest, `DRILL` when you've drilled into a calendar day (to cycle its events), and `GRAB` in grab mode. Merely focusing the task tree or the calendar grid is ordinary navigation and stays `NORMAL`. It tells you what the movement keys (`hjkl`) act on right now, so a context-sensitive key is never a surprise.
- **`:conflicts`** resolves items that changed on both sides (keep local / keep server); the status bar shows the live conflict count.
- **`+` / `-`** collapse / restore the overview so the calendar or tree fills the width (in week/day view they zoom the hour height instead, and **`0`** resets it to auto-fit); **`Ctrl-←` / `Ctrl-→`** narrow / widen the overview column. **`Ctrl-W`** opens a resize sub-mode: `←`/`→` size the overview, `H`/`L` the Detail pane, `Enter` keeps and `Esc` cancels. All widths are remembered across launches.
- **`r`** — sync now (alias for `:sync`). LazyPlanner also syncs in the background on startup and **periodically while open** (every `sync_interval_minutes`, default 15, `0` = off); the status bar's right section shows the state (`syncing…`, `synced HH:MM`, `! N conflict(s)`, `offline`, or `not configured`).
- **Mouse**: click a panel to switch to it, click to select, double-click the tree/agenda to edit, wheel to scroll.

### Keybindings

| Key | Action |
|---|---|
| `c` `t` `a` | Focus the Calendars / Tasks / Agenda overview panel |
| `Tab` / `Shift-Tab` | Cycle those three |
| `↑` `↓` `←` `→` / `j` `k` `h` `l` | Move the highlight in the focused pane |
| `<count>` + motion | Repeat a motion — `3j`, `5k` |
| `gg` / `G` | Go to top / bottom of the list, tree, or calendar grid (`<count>G` → nth item of a list, the tree, or a drilled day) |
| `Enter` | Dive into the center; cycle a day's events; open a list / expand a task |
| `Esc` | Back out to the overview · cancel a form/dialog/chord |
| `i` … | Create prefix — `t`/`T` task, `e`/`E` event, `s`/`S` subtask, `c` calendar, `l` list (Shift = full form) |
| `e` | Edit selected (full form) |
| `s` … | Quick-set a task field — `p` priority, `d` due date (blank clears) |
| `d` | Delete selected item — or the calendar/list when its panel is focused |
| `Space` | Toggle the selected/drilled task done — or hide/show the highlighted calendar (Calendar view, no task drilled) |
| `/` · `n` / `N` | Search the current view · next / prev match |
| `H` / `L` | Outdent / indent task (re-parent) |
| `y` / `p` | Yank / paste a task — move it (and its subtree) to another parent or list |
| `z` … | Fold the tree — `zR` expand-all, `zM` collapse-all, `za` toggle |
| `u` | Undo last local change (this session) |
| `v` | Cycle calendar view: month → week → day |
| `[` / `]` | Cycle the highlighted calendar (any mode) |
| `{` / `}` | Cycle the highlighted task list (any mode) |
| `f` / `b` · `gt` | Forward / back one period · jump to today |
| `+` / `-` / `0` | Accordion collapse / restore · in week/day: zoom hour height, `0` = auto-fit |
| `Ctrl-←` / `Ctrl-→` · `Ctrl-W` | Narrow / widen the overview column · resize sub-mode (overview + Detail) |
| `r` | Sync now (= `:sync`) |
| `:` · `gd` · `?` | Command line · go to date · help |
| `.` | Show/hide completed tasks |
| `q` / `Ctrl-C` | Quit / back out (best-effort syncs pending changes on the way out) |

## Syncing

Once `[server]` is set, LazyPlanner syncs **both ways** on startup, **periodically** while open (`sync_interval_minutes`, default 15, `0` = off), a few seconds after any local edit (a **debounced** background push, so other devices see changes fast), on **quit** (a best-effort push of anything still pending, so an edit made right before you quit isn't stranded until the next launch — it's skipped instantly when nothing's pending or you're offline, and is time-bounded so a slow network can't delay exit), and whenever you press `r` (or run the `sync` command below). Sync is ETag-based and **never silently overwrites**: it pushes local creates/edits/deletes, pulls remote changes, and when the same item changed on both sides it keeps both versions and flags the conflict — resolve them in-app with `:conflicts` (keep local / keep server). Sync is **incremental**: each calendar's server CTag is checked first, and one whose contents haven't changed (and has nothing local to push) is skipped without re-downloading — so a routine sync of an idle account is cheap, which matters on a Raspberry Pi or with large calendars.

**Read-only calendars** (like NextCloud's generated "Contact Birthdays" calendar, or read-only shares) are detected automatically and marked `[ro]` in the overview. LazyPlanner never writes to them — creating/editing/deleting there is blocked with a hint, and sync mirrors them one-way — exactly as the NextCloud web UI treats them.

```sh
lazyplanner sync      # two-way sync of the local cache against the server
lazyplanner import    # one-way pull only (server → local), e.g. for a first seed
lazyplanner version   # print the version
lazyplanner help      # list the subcommands
```

(An unrecognized subcommand is reported with a non-zero exit and the usage, rather than silently opening the TUI.)

Both take the same connection flags as below (or the `LAZYPLANNER_CALDAV_URL` / `LAZYPLANNER_CALDAV_USERNAME` / `LAZYPLANNER_CALDAV_PASSWORD` environment variables), and honor `--data` to override the data directory:

```sh
lazyplanner sync \
  --url https://cloud.example.com/remote.php/dav \
  --username you \
  --password <app-password>
```

## Managing calendars

You can create and delete calendars/task lists in-app (`ic` / `il` to create a calendar / list, `d` to delete the focused pane's collection — all offline-first), so you never need the NextCloud web UI. These CLI subcommands do the same directly on the server (via CalDAV `MKCALENDAR` / `DELETE`); connection flags/env vars are the same as `import`.

```sh
lazyplanner calendar list                          # show calendars + their server paths
lazyplanner calendar create --name "Projects"      # an event calendar
lazyplanner calendar create --name "Errands" --tasks   # a task list (VTODO)
lazyplanner calendar create --name "Home" --both --color "#3366cc"
lazyplanner calendar delete --path "/remote.php/dav/calendars/you/errands/"
```

After creating a calendar, run `lazyplanner import` to pull it into the local cache.

## Raspberry Pi / dedicated terminal

LazyPlanner is a single static binary with no runtime dependencies, so it's a natural fit for a low-power Raspberry Pi used as an always-on wall calendar. Because it's pure Go (no cgo), you **cross-compile from any machine** — no ARM toolchain needed:

```sh
make cross      # → dist/lazyplanner-linux-{arm64,armv7,armv6}, stripped (~8.6 MB)
```

Pick the binary for your Pi and OS: **arm64** for 64-bit Raspberry Pi OS (Pi 3/4/5, Zero 2 W), **armv7** for 32-bit Pi OS (Pi 2/3/4, Zero 2 W), **armv6** for the original Pi / Pi Zero / Zero W. Copy it over and drop it on the `PATH`:

```sh
scp dist/lazyplanner-linux-arm64 pi@raspberrypi:/tmp/lazyplanner
ssh pi@raspberrypi 'sudo install -m0755 /tmp/lazyplanner /usr/local/bin/lazyplanner'
```

Run `lazyplanner` once to write the starter config, fill in `[server]` (see [Configuration](#configuration)), and set `sync_interval_minutes` to how often the display should refresh from the server.

**Kiosk (launch full-screen on boot).** LazyPlanner is a terminal program, so the simplest dedicated-terminal setup is a console **autologin** on `tty1` that execs it — no X server needed. Enable console autologin with `sudo raspi-config` (*System Options → Boot / Auto Login → Console Autologin*), then have the login shell launch LazyPlanner on the main console only:

```sh
# ~/.bash_profile on the Pi — replace the login shell on tty1 with LazyPlanner,
# and drop back to a shell when you quit (q). Other ttys/SSH stay normal shells.
if [ "$(tty)" = "/dev/tty1" ]; then
  exec lazyplanner
fi
```

`raspi-config`'s autologin drops in a systemd getty override equivalent to:

```ini
# /etc/systemd/system/getty@tty1.service.d/autologin.conf
[Service]
ExecStart=
ExecStart=-/sbin/agetty --autologin pi --noclear %I $TERM
```

Set `color_mode = "16"` in the config if the Pi console is a bare framebuffer TTY (no truecolor); on a desktop terminal emulator leave it `auto`. The periodic background sync keeps the display current without any interaction.

**Performance.** The binary starts from the local cache instantly and syncs in the background, and the incremental CTag short-circuit keeps routine syncs cheap — both designed for modest hardware. The core hot paths (bulk import, tree building, recurrence expansion) scale **linearly** with calendar/list size, so large calendars stay responsive. Measure `time lazyplanner sync` and startup on your Pi and tune `sync_interval_minutes` to taste.

## Development

The project's full specification and development history live in [`main.md`](main.md) (the master spec, including the versioned build plan), [`log.md`](log.md) (the change log), and [`docs/audit/`](docs/audit/) (hardening-audit records); contributor/agent rules are in [`CLAUDE.md`](CLAUDE.md).

## License

[MIT](LICENSE)
