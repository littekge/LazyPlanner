# LazyPlanner

A terminal-based todo-list and calendar manager with offline-first CalDAV sync — a full-screen interactive TUI in the style of [lazygit](https://github.com/jesseduffield/lazygit), written in Go.

> **Status: pre-build.** The spec is complete (see [`main.md`](main.md)); implementation has not started. This README is kept current as features land — sections marked *not yet built* fill in as the build progresses.

## What it does

- **Syncs with a CalDAV server** (built for NextCloud): offline-first, so the app opens instantly and works without network; changes sync both ways and stay visible from NextCloud web and your phone.
- **Todo management** with deep subtask hierarchies — arbitrary nesting, navigated like a file explorer.
- **Calendar views** — month, week, and day grids for events and dated tasks.
- **Recurring events and tasks**, including per-occurrence editing.
- Keyboard-first (single-key shortcuts + a `:` command mode), with full mouse support.

## Usage

*Not yet built.* Keybindings and commands will be documented here (and in the in-app `?` help) as they land.

## Build & Install

*Not yet built.* Planned:

- **Linux** (primary): `go build ./cmd/lazyplanner` — a single static binary, no runtime dependencies.
- **Windows** (secondary): `GOOS=windows go build ./cmd/lazyplanner`.
- Config lives at `~/.config/lazyplanner/config.toml` (Linux) / `%APPDATA%\lazyplanner\` (Windows); a commented default is generated on first run.

## Development

- [`main.md`](main.md) — the build specification (single source of truth)
- [`CLAUDE.md`](CLAUDE.md) — project rules and coding standards
- [`log.md`](log.md) — the change log; every change gets an entry

## License

[MIT](LICENSE)
