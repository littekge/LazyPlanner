# LazyPlanner

A terminal-based todo-list and calendar manager with offline-first CalDAV sync — a full-screen interactive TUI in the style of [lazygit](https://github.com/jesseduffield/lazygit), written in Go.

> **Status: early build.** The spec is complete (see [`main.md`](main.md)). Done so far: build steps 1–5 — the Go module, package skeleton, vendored dependencies, CI, and a placeholder TUI window (step 1); the core `model` layer parsing events and todos from iCalendar data (step 2); timezone-aware recurrence expansion (step 3); the local vdir cache with an in-memory index and atomic writes (step 4); and one-way CalDAV import — discovering NextCloud calendars and downloading them into the cache (step 5). The interactive UI is *not yet built*, but the `import` command below already works against a real server.

## What it does

- **Syncs with a CalDAV server** (built for NextCloud): offline-first, so the app opens instantly and works without network; changes sync both ways and stay visible from NextCloud web and your phone.
- **Todo management** with deep subtask hierarchies — arbitrary nesting, navigated like a file explorer.
- **Calendar views** — month, week, and day grids for events and dated tasks.
- **Recurring events and tasks**, including per-occurrence editing.
- Keyboard-first (single-key shortcuts + a `:` command mode), with full mouse support.

## Usage

The interactive TUI is *not yet built* — keybindings and commands will be documented here (and in the in-app `?` help) as they land.

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
