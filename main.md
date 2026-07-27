# LazyPlanner — Build Specification

> **Purpose**: This document is the single source of truth for **what** LazyPlanner is: the project identity, the complete design, all settled decisions, and the versioned Build Plan — including a compressed history of everything done, so an agent gets full project context without sifting the change log. *How* to work on the project (coding rules, architecture guardrails, workflows) lives in `CLAUDE.md`; the detailed change history lives in `log.md`; the user guide is `README.md`.

---

## Project Identity

- **Name**: LazyPlanner
- **Releases**: versioned on GitHub (Releases + git tags) — the single source of truth for release version numbers. main.md tracks the **design** and the **Build Plan** (planning milestones), never a maintained release version.
- **Module**: `github.com/littekge/LazyPlanner` (matches the GitHub repo)
- **Language**: Go (chosen for the Go 1 compatibility promise, single static binaries, easy ARM cross-compilation, and the lazygit-style TUI ecosystem). Minimum version: the stable release current at scaffold time, bumped only deliberately thereafter.
- **Framework/Libraries**:
  - **TUI**: tview (`rivo/tview`, on top of `gdamore/tcell`)
  - **CalDAV/iCalendar**: `emersion/go-webdav` (CalDAV client), `emersion/go-ical` (iCalendar parsing), `teambition/rrule-go` (recurrence rules)
  - **Config**: `BurntSushi/toml` (pure Go, API-stable for a decade)
- **Platform**: Terminal. **Linux is the primary target** (incl. a Raspberry Pi dedicated terminal); Windows is a secondary compatibility build — features are tailored to Linux, and OS-specific paths go through one resolver (`os.UserConfigDir` + a data-dir helper) so the Windows build comes nearly free.
- **License**: MIT (see `LICENSE`)
- **Docs**: `README.md` — the end-user guide (what it does, build/install, usage, keybindings), kept current with user-visible behavior
- **Change Log**: `log.md`

---

## What This Program Is

LazyPlanner is a terminal-based todo-list and calendar management program. It is a full-screen interactive TUI in the style of **lazygit** — panes and views navigated entirely with the keyboard, designed to make managing tasks and a schedule fast and low-friction.

**Core Features:**

- **CalDAV sync** — the must-have feature. Offline-first: a local cache is the working copy; syncs with a NextCloud CalDAV server in the background or on demand, so the app opens instantly and works without network. Existing calendars and todo lists on the server are imported, and changes remain accessible from the web via NextCloud.
- **Todo management** — create, edit, complete, and organize tasks. **Deep subtask hierarchy is the centerpiece feature**: arbitrary-depth nesting rendered as a collapsible tree and navigated like a file explorer, where a "folder" is simply a task with children. Fields surfaced: title, due date, status, priority, tags, notes, subtasks.
- **Calendar views** — day/week/month views showing tasks and events on a timeline
- **Recurring tasks/events** — repeat rules (daily, weekly, custom) for tasks and calendar entries

**Design Goals:**

- Lazygit-inspired UI: multi-pane layout, single-keystroke actions, discoverable keybindings, mouse support, and a `:` command mode for in-program commands
- Fast to open, fast to use — managing your day should take seconds, not minutes
- Robust and long-lasting: a single static binary with no interpreter or runtime dependencies, so OS and dependency updates don't break the program
- Fast on modest hardware — a dedicated Raspberry Pi terminal running LazyPlanner is a design target
- A well-behaved CalDAV citizen: never corrupts or drops data it doesn't understand; other clients (phone, NextCloud web) keep working alongside it

---

## Current State

**v1.0.0 through v1.4.0 are released.** **v1.5.0 — final polishing & auditing — is the current phase**, and the last planned release for the foreseeable future. Each version is scoped as a Build Plan subsection before work begins.

Running alongside every version is a continuous **hardening & audit phase** (patch-level bug-hunting, resilience, and consistency work). Coverage and residual risk are tracked in `docs/audit/COVERAGE.md`, the full per-pass reports in `docs/audit/passes/`, and the Build Plan's Hardening & audit ledger carries a short summary of every pass. Sync findings are verified headlessly; the opt-in live CalDAV suite (run against a throwaway test account) is available on demand.

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
  docs/audit/          Hardening-audit protocol, coverage ledger, pass reports
  main.md  log.md  notes.md  CLAUDE.md  README.md
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
| Calendar data (vdir cache + sync-state sidecar) | `~/.local/share/lazyplanner/<account-id>/calendars/<calendar>/<uid>.ics` (`$XDG_DATA_HOME`) | `%LOCALAPPDATA%\lazyplanner\<account-id>\calendars\...` |

The vdir data lives under *data* paths, **not** `~/.cache` — it can hold offline edits not yet synced to the server, so it must never be treated as disposable.

---

## UI Design

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
 a:add  e:edit  space:done  ::cmd  ?:help         synced 3:04pm
```

**The overview column** (left) holds three small panels: **Calendars** (the calendar list, with visibility toggles), **Tasks** (the **task lists** — calendars that support todos, incl. empty ones, *not* the full tree), and **Agenda** (today's events + due tasks). `c`/`t`/`a` **focus the corresponding overview panel** — the highlight lives there and moves through its rows — while the Main pane shows the matching view. `Enter` dives from the overview into the Main pane where that makes sense; `Esc` backs out to the overview. Each Calendars row shows a `●` bullet in the calendar's server color (a **hidden** calendar drops the bullet, so its off-the-views state reads at a glance next to the `(hidden)` marker), an item-count suffix like `(5e 3t)` (events/tasks currently cached for that calendar), and a component-set tag — `[events]`/`[tasks]`/`[both]`, `[?]` when unknown, `[ro]` when read-only.

**The Main pane** follows the active overview panel:

- **Calendars** → focus lands on the Calendars list (arrow keys highlight each calendar; `Space` hides/shows the highlighted calendar on the calendar+agenda views, remembered in the state file). The Main pane shows the calendar view: month grid (default) or the week/day hourly time-grid. `Enter` dives into the grid (arrows move days, `Enter` drills into the selected day's items — `j`/`k` cycle them — and `Esc` returns to the list). `[` / `]` cycle the highlighted calendar and `{` / `}` cycle the highlighted task list from any pane (independent overview selectors). Of the view/forward/back/today keys, only `gt` truly works from any pane (it switches the Main pane to the calendar itself); `v`/`f`/`b` act on the calendar view and no-op silently from the Tasks or Agenda overview.
- **Tasks** → focus lands on the Tasks list; selecting a list opens its full collapsible subtask tree in the Main pane, with inline priority / due date / completion status.
- **Agenda** → focus lands on the Agenda list; moving its highlight draws an **outline box** around the matching block in the Main pane (the same cursor style as the calendar's selected day), auto-scrolling to it. The Main pane shows the day's events and tasks with full descriptions, at **full width** (the Detail pane is hidden), scrollable when a day overflows.

**The Detail pane** (right) shows the highlighted item's full fields — event: time/location/reminders/notes; task: due/priority/tags/status/notes. Before a day is drilled into, it instead shows a day-summary (not a single item's fields) for the highlighted day. It is hidden in Agenda mode so the center gets the whole width.

**The bottom two lines** are the status bar and the help bar.

The **status bar** (upper of the two, outlined like the primary panes) has four sections:

- **Interaction-mode indicator** (leftmost) — a vim-style mode badge drawn as a filled high-contrast chip for the active modes, so a mode-sensitive key like `hjkl` is never a surprise: `NORMAL` at rest, `DRILL` when drilled into a calendar day to cycle its events (merely focusing the task tree or the calendar grid stays `NORMAL`, so the two agree), `GRAB` in grab mode. It shows the *interaction* mode — what the movement keys act on now — distinct from the *view* context shown in the next section.
- **General status line** — results of the last action ("task created", "saved", errors), or the current view context when idle. A red `!N load error(s)` is appended here when cached resources failed to parse — the only surfacing of ingest errors.
- **Command view** — echoes the most recently executed action in command form (lazygit-style).
- **Sync status** — color-coded words, not glyphs: `synced 3:04pm` (an absolute clock time, not a relative "ago" duration), `syncing...`, or `offline`. A red `! N conflict(s)` is appended after the synced text when there are unresolved conflicts.

The `:` command line opens as a separate input near the top of the screen.

The **help bar** (very bottom) shows the key hints, always visible so the basic controls never scroll away, and is **mode-adaptive**: it curates the pane-specific clause to the active mode (Calendar's `f`/`b`/`v` vs. Tasks' `>`/`<`/`H`/`L`/`z`, both silent no-ops outside their own mode), and swaps in dedicated hints for `RESIZE` and an open form rather than showing the resting NORMAL line in every context.

### Calendar views

The calendar (active when Calendars is selected) has three views, cycled with `v`.

**Month** is a custom-drawn grid that fills the pane: one cell per day listing that day's events/tasks (with a `+N more` overflow line), today emphasized and adjacent-month days dimmed.

- The selected day is marked with an **outline box** (a cursor), never a solid fill, so event text stays readable. Selecting a day lets you cycle through *that day's* events; the Detail pane then shows the highlighted event/task's full info.
- When a day holds more items than its cell can show, drilling **scrolls the visible window** so the highlighted item stays on screen. A `+N more` line at the bottom counts items still *below* the window; a matching `+N more` at the top counts items *above* it (appearing once the window has scrolled past the first item). Each shrinks and disappears as you drill toward that edge.
- A **multi-day timed event** spans each day it covers, but its time label reflects the day: the **start day** shows the start time, the **final day** shows the end time (prefixed `→`), and the days it merely continues through show the **title alone** — so the start time is not repeated on every cell.

**Week and day** are an **hourly time-grid** like a conventional calendar: an hour axis down the side with events drawn as blocks sized by their duration; all-day items sit in a band across the top; overlapping events are placed side-by-side (overlap lanes come from `model.LayoutDay`). Day view is one column, week view seven. A **multi-day timed event** is drawn on every day it covers as a per-day block clipped to that day's column: the start day runs from its start time to the bottom (midnight), the days it spans through fill the whole column, and the final day runs from the top (midnight) to its end time.

Every hour gets a **uniform height** (the largest whole number of rows per hour that fits), so the hour axis is evenly spaced rather than jittering by a row; a small blank margin can sit below the last hour when the pane height isn't a whole number of hours. When the pane is too short to show all 24 hours at even one row each, the grid **scrolls** — following the drilled item, otherwise the current time — instead of squashing the hours together. `+`/`-` **zoom the hour-row height** (taller hours scroll more of the day off-screen; the chosen zoom is remembered across launches, `0` = auto-fit).

Tasks with a due date also appear on the time-grid: a timed-due task draws a `[ ]`/`[■]` task line at its due time, an all-day-due task sits in the top all-day band, both tinted by their list's color and using the same checkbox convention as the month grid.

**Un-drilled**, `←`/`→`/`h`/`l` move between days and `↑`/`↓`/`j`/`k` do nothing (days are horizontal); `Enter` drills in. **Drilled**, navigation is **2D and spatial over that day's layout**: `↑`/`↓` move by time, `←`/`→` move between concurrent (side-by-side) events — e.g. from an 11–12 event down to the leftmost of two 12–1 events, then right/left between the pair. The all-day band is the top row (`←`/`→` between its items, `↓` enters the timed grid); due-task markers are single-lane rows. Movement stops at the day's edges; `f`/`b` changes the period (staying drilled), `Esc` backs out.

All timed values are **displayed in the app's own zone** (`config.LocalZone()`, which the forms also parse and write in — not necessarily `time.Local`). A **recurring** item's anchor (`DTSTART`, or `DUE` for a VTODO) is written as **local time with a `TZID`**, with a matching `VTIMEZONE` in the object — a recurrence rule's `BY*` parts are evaluated in the anchor's own zone, so a UTC anchor makes a locally-authored rule fire on the wrong day and drift across DST. Non-recurring timed values are still written in **UTC**, and a value imported from the server is preserved as-is per the iron rule. All-day items stay date-only. The anchor is re-serialized only when LazyPlanner is authoring the rule in the same edit; an edit that leaves the rule alone never re-anchors, because re-interpreting `BY*` against a different zone would move the series. See Timezones & recurrence for the full design and known residuals.

### Task tree: lists in the overview, tree in Main

The left **Tasks** panel lists the task lists (calendars whose supported component set includes VTODO — an empty list still appears so you can add to it; when the component set is unknown, a calendar shows once it holds a todo). Selecting a list opens its full collapsible subtask tree in the **Main** pane, **rooted at the list's own name** so the top-level tasks attach to it (`Enter` or `Space` expand/collapse a node; arrows/`hjkl` only move the highlight up and down the flattened tree), with inline priority/due/status; the Detail pane shows the highlighted task's full fields. `>` **zooms** — re-roots the Main tree at the selected task like `cd`-ing into a directory (breadcrumb shows `School / ECE384`); `<` zooms back out.

### Folders

A task with at least one *incomplete* child is a "folder" and behaves like one. It renders with a `▸`/`▾` disclosure marker in place of the `[ ]`/`[■]` checkbox (in the tree it doubles as the expand/collapse indicator), and the **same `▸` caret is used in the calendar and agenda views** so a folder reads as a folder everywhere — one global folder set drives all three views.

A folder **keeps its own due date**. Folder-ness is orthogonal to the due date: a folder with a due date still renders that date and still appears on the calendar on its due day (a task shows on the calendar iff it has a due date — folder or not). Adding a subtask to a dated task therefore never makes it vanish from the calendar; it just gains the `▸` caret. (Hiding folder due dates was considered and rejected: it discards user-set data and causes that disappearance.)

A folder **cannot be completed** while it still has incomplete children — finish or remove them first (enforced in every view, including `Space` on a folder in the calendar) — and **reverts to an ordinary task** (checkbox, completable) once it has no children or all its children are complete.

**Deleting a task that has subtasks** removes the whole subtree (the task and all descendants) after a confirmation whose message names the subtask count — including a folder with incomplete children. A leaf task (no descendants) deletes with a plain confirm and never cascades. (The confirm is one dialog with an explicit subtree count, not a second separate gate.)

### Creation: quick-add with smart date parsing

Creation targets are keyed to the object, not the focused pane: **create task** makes a top-level task in the **selected task list** (Tasks overview), **create event** makes an event in the **selected calendar** (Calendars overview) on the selected/current day, and **create subtask** makes one under the **currently selected task** — the tree node in Tasks, or a task drilled into in a calendar/agenda view — created in that parent task's own list (RELATED-TO must share a collection). All three work from any pane (a "both" calendar appears in both overviews, so select it in each to target it).

**Creation is gated by the target calendar's supported component set** — events only on VEVENT-capable calendars, tasks/subtasks only on VTODO-capable lists; a calendar created with "both" supports either. The type must be *known* (declared via the server's `supported-calendar-component-set`, captured on sync, or set explicitly when created in-app): an unconfirmed type blocks creation until a sync settles it (rather than guessing from contents) — with a manual override, `i!`… (e.g. `i!e`), for when the user knows better than the missing metadata. The override applies **only** to the unknown-type case: read-only calendars and a *known* wrong type are never forced. The Calendars overview marks each calendar `[events]`/`[tasks]`/`[both]` (or `[?]` when unknown).

Each create has a **quick-add** form (a one-line smart-parsed input) and a **full form**, reached by distinct keys folded into the chorded keymap (the `i` prefix; Shift = the full form). The quick-add parser is deliberately conservative — a token is claimed only when it clearly matches a documented form, and everything unmatched becomes the title. Tokens are **whitespace-delimited** (a sigil matches only at a token's start, so `bob@example.com` and `task!5` are inert title text) and each slot is **first-match-wins** — except **tag**, which is multi-valued: every `#tag` token accumulates rather than only the first being kept.

- **Date** — `today`/`tomorrow`, a weekday (`fri`), `jul 20`, `7/20`, `7/20/2026`, `2026-07-20`; and relative forms `next <weekday>` (the next one, +7 days), `next week` (today +7), `next month`, and `in N days/weeks/months` (singular units too, e.g. `in 1 day`).
- **Time** — `3pm`/`3:30pm`/`15:00`, or a **range** `5-6pm` / `14:00-15:30` (at least one half carries am/pm or a colon; a right-side am/pm distributes to a bare left half; an end at or before the start crosses midnight). A bare number is never a time. An event takes the range's end (else a 1-hour default); a task's due is the range's start.
- **Recurrence** — `daily`/`weekly`/`monthly`/`yearly`, `every day/week/month/year`, `every <weekday>` (weekly on that day), `every <month> <day>` (yearly on that date). With no explicit date typed the recurrence anchors the start/due itself (`every mon` → the next Monday); an explicit date wins and anchors the series. **This is the first in-app way to create a recurring item.**
- **Priority** `!1`–`!9` / `!high`/`!med`/`!low`, **tag** `#tag`, and **location** `@cafeteria` or `@"room 204"` (set on events and tasks; both full forms carry a Location field, so a task's location survives an edit).

An **obvious typo** — a `!`+letters that isn't a priority (or a duplicate priority), an unclosed `@"…`, a fuzzy follower (`next tuedsay`, `in 3 dayz`), or an impossible time/date (`25:00`, `5-6xm`, `2026-07-40`, `2/30/2026`) — produces a warning: the quick-add input **stays open** showing it and creates nothing, and resubmitting the *identical* text accepts it as-is (the failed token stays in the title). A warning fires **only on an unmistakable intent anchor**, never on plausible prose (`My Event!!!!!`, `email bob@example.com`, `24/7`, `http://x.com` stay silent). The single-field quick-sets (`sp`/`sd`) flash the warning instead of re-prompting.

The **full form** (`e` on an existing item, or the full-create key) edits **every** field, including the recurrence rule via a **Repeat dropdown** (v1.3.0). Its options are built from the item's start/due date: `None`, the anchor-derived presets (`Daily`, `Weekly on <weekday>`, `Monthly on day <n>`, `Yearly on <mon day>`), and `Custom…`.

- **Custom…** opens a nested sub-form that shows only the fields relevant to the current selection — Every, Unit and Ends always, re-laid-out live as Unit or Ends changes (values preserved): a weekly rule adds a compact weekday **toggle strip**; a monthly rule adds a "Monthly by" dropdown (day-of-month **or** nth/last weekday, derived from the start date, so the rule can't contradict its anchor); the matching `Ends` choice (`On date` / `After N times`) adds its date or count input.
- A rule the app can't represent shows as **"Custom rule (kept)"** and is preserved byte-for-byte unless the user explicitly overwrites it; an **unchanged** rule is never rewritten.
- Picking a rule on a plain item makes it recurring. On a recurring item, scope **All** rewrites the master (keeping EXDATEs, dropping only orphaned per-occurrence edits); scope **this & future** gives the split-off series the new rule; scope **this occurrence** hides the field. `Repeat → None` drops the rule and its exceptions/overrides.

Parsing rules are predictable and documented in `:help` — when in doubt, text stays in the title rather than being guessed.

**Calendar color** is part of the **create/edit calendar form** — one form for both.

- **The Color field** carries a **"Pick color…"** button opening a **swatch-grid picker**: a popup of preset color cells (a NextCloud-like palette) navigated with `hjkl`/arrows, `Enter` to pick, plus a "Custom hex…" entry for any other color. The pick is written back into the Color field, which also accepts a typed hex. The picker can nest over the form because modal focus save/restore is a stack.
- **Color is set at creation.** A new calendar is colored from the start and carries the color in its MKCALENDAR, not left default until manually recolored. The field is **pre-seeded with a default palette color** (NextCloud blue) and a blank on create falls back to it, so **every created calendar/list always has a color**.
- **The same form edits an existing collection's name + color** — `e` on the Calendars pane for a calendar, `e` on the Tasks pane for a task list. This is symmetric with `d`, which deletes the focused pane's collection behind a type-to-confirm dialog (a collection delete can't be undone).
- **`:calendar color`** with no hex opens the swatch picker directly (a quick recolor); `:calendar color #rrggbb` sets one directly.
- **All changes are offline-first**, pushed on the next sync — MKCALENDAR for a new calendar, `PROPPATCH` for an existing one.

### Completing, editing, and quick field-set

`Space` toggles the selected/drilled task complete in **any** view — the tree, the agenda, or a task drilled into in the month/week/day calendar — and `e`/`d` likewise edit/delete from the calendar and agenda views (drill in first). In a calendar view with **no** task drilled, `Space` instead hides/shows the highlighted calendar; drilled into an *event*, `Space` flashes a reminder that events can't be completed rather than flipping visibility. The `s` prefix **quick-sets a single task field** without the full form — `sp` priority, `sd` due date — one-line inputs that change that one field and preserve everything else.

### Moving tasks: yank & paste

Structural moves go through the clipboard: `y` **cuts** (move) and `Y` **copies** (duplicate) a task together with its subtree; `p` pastes under the selected task, `P` at the list's top level. A copy gets fresh UIDs with the subtree's RELATED-TO links remapped onto the copies (`model.CopyTodo`), leaving the original untouched. A cut pastes as a **move**: within the same list it re-parents; across lists it recreates the subtree in the target and deletes the original — both **all-or-nothing with rollback**, so a failure can't leave half a subtree moved. The clipboard **persists after paste** (multi-paste), and every paste is undo-able. (Temporal moves are grab mode's job, not the clipboard's.)

### Grab mode: temporal manipulation

`m` enters **grab mode**, the temporal-manipulation layer, unified across the tree, calendar, and agenda views. It moves an **event** in time (`j`/`k` ±hour in week/day, `h`/`l` ±day, `J`/`K` resize the end) or nudges a **task's** due date (`j`/`k` ±day, `h`/`l` ±week).

- **Modality.** Grab swallows other keys and the mode badge shows `GRAB`. Each nudge commits to the store so every view updates live; `Enter` keeps the result, `Esc` reverts to the pre-grab snapshot as one undo step. Undated tasks are skipped.
- **Recurring events** prompt for scope first (see Recurring items). A *this & future* grab splits the series when the grab starts and retargets onto the new series; `Esc` then reverts both resources (deletes the new series, restores the master).
- **A whole-series day-move** (`h`/`l` at scope *all* or *this & future*) **re-anchors the rule to the moved day** so the series follows the move: a weekly weekday set shifts as a whole (Mon,Thu → Tue,Fri), a monthly nth-weekday re-derives from the new date. Moving `DTSTART` alone would leave a day-pinning `BY*` firing on the old day with the moved instance vanished.
- **A recurring todo's due-grab** (single or bulk) re-anchors its day-pinning rule the same way — a todo's `DUE` *is* its recurrence anchor, so shifting the due moves the whole series and its `BY*` with it (`model.ReanchoredRecurrenceTodo`).
- **A rule the app can't represent** (a *Custom rule (kept)*) blocks the day-move with a hint rather than risk corrupting it or leaving the anchor contradicting the rule (`model.ReanchoredRecurrence`).

### SELECT mode: multi-select and bulk operations

`V` enters **SELECT**, a vim-style multi-select layer that turns the task tree, the calendar, or a drilled day's items into a contiguous range for a single bulk action.

- **Contexts and entry**: SELECT is reachable from the task tree, the un-drilled calendar (a day range), or a drilled day's item list — but only when that surface itself is focused, not an overview list (`c`/`t` alone leave focus on the Calendars/Tasks overview, so a `V` from there just flashes a hint). The range is a **contiguous anchor→cursor span** — anchored where `V` was pressed and extended by ordinary cursor motion in whichever context is active — never a scattered multi-pick — and it is **derived, not stored**: `selRange()` re-slices the anchor and the view's live cursor position on demand, so it can never drift from what is actually on screen, and it revalidates on every refresh. A day-range selection (the un-drilled calendar) is capped at 366 days from the anchor — a deliberate bound so `f`/`b` can't build a multi-year span.
- **Motion and the swallow contract**: SELECT is modal like GRAB — motion (`hjkl`, `gg`/`G`, `f`/`b`, and a count) extends the range and passes through, but everything else is swallowed: context-switch keys, edit keys, a **modified** motion key (Ctrl-arrows can't sneak a pane resize in mid-select), and a **bare `0`** (which would otherwise leak `globalKeys`' week/day hour-zoom reset — a `0` continuing a count, e.g. the one in `10j`, still reaches motion). The badge shows `SELECT`.
- **Bulk operations**: one bulk action — `Space` complete, `d` delete, `y`/`Y` yank/copy (tree only), or `m` grab — applies to the whole range. Complete and delete are all-or-nothing with **one compound `u` undo step**; bulk grab is the exception — if a mid-session item goes stale, the grab ends keeping the moves already committed by earlier nudges rather than reverting the whole session. Yank/copy pushes **no** undo step at all (the clipboard changes nothing; undo happens at paste, same as a single-item yank). Each first filters out items it can't act on (recurring events, read-only, missing, already-done, folders with open subtasks, undated); delete/yank's subtree-absorption dedupe runs only against what survives that filter, so the confirm/summary count always matches what's actually acted on; the skip counts are reported in the flash.
- **Bulk grab**: reuses grab's per-nudge mechanics but applies **one uniform date-shift** to every selected item at once, returning to SELECT (not exiting it) on `Esc` so a grab can be retried.
- **Exit and nesting**: modes nest the same way GRAB does under DRILL — `DRILL → SELECT → GRAB`, each layer's `Esc` unwinding exactly one level. A **lost** anchor (a remote delete, an emptied drilled day) exits SELECT with a flash rather than acting on a guess — but a day-range anchor can't itself vanish, so an anchored range spanning only empty days is still a **valid** selection, extendable (`f`/`b`) toward a day that has items.

### Recurring items

Editing (`e`), deleting (`d`), or grabbing (`m`) a recurring **event** opens a **scope picker**: *this occurrence* (a `RECURRENCE-ID` override / `EXDATE`), *this and future* (the series is split into two, with a bounded `COUNT` preserved across the split), or *all* (edit the master). All three scopes are supported for all three actions.

A recurring **todo** shows as a **single live instance** at its current due date; completing it with `Space` **advances** it one occurrence (NextCloud-style), and it is marked done only when the series is exhausted. Editing "this occurrence" of a recurring todo detaches that instance as a standalone one-off task (confirmed first) and advances the series. (See Settled Decisions for why todos don't expand into per-occurrence calendar entries.)

### Colors and window chrome

Server calendar colors render as **exact truecolor (24-bit RGB)** by default, so a calendar looks like the same color it is in NextCloud web/phone. tcell **downsamples automatically** to whatever the terminal supports — 256 colors, or the 16-color palette on a bare Pi console/TTY — so one code path degrades gracefully everywhere. The `[appearance] color_mode` option (`auto` default · `truecolor` to force 24-bit on terminals that underreport · `16` for the themed nearest-ANSI mapping that inherits the terminal theme · `off` for no calendar colors) lets a user override the default; `16` is the escape hatch for a light terminal or a bare TTY where truecolor isn't wanted.

The **background is the terminal's default** everywhere (`tview.Styles.PrimitiveBackgroundColor = ColorDefault`), so text never sits in a shaded box; only deliberate fills (event blocks, selection highlights) use an explicit background. Because item colors are drawn as **foreground text on that unknown default background**, a very dark server color is **lightened to a luminance floor** for legibility (assuming the common dark terminal; the exact color is still used for filled event blocks, which supply their own contrasting text). Non-color UI chrome stays on the terminal's named colors so it still inherits the theme.

Panes and dialogs use **rounded (soft) corners**; the focused pane is shown by border *color* (yellow), not a heavier line. Popups (edit/create forms, quick-add line, confirm prompt) share one look: the terminal's **default (unified) background** with **high-contrast default text** and an **accent rounded border/title**, so they read as part of the terminal theme rather than a jarring card. Because tview applies a single field style to every form field, the **focused field is marked by a `▸` caret** in the label gutter (and the focused button is reversed) rather than a per-field color.

### Completed tasks, sort order, undo

**Completed tasks** are hidden from the tree by default (keeps deep trees clean); `.` toggles showing them in place with a **filled checkbox** (`[■]` vs `[ ]`) — the dotfiles gesture, fitting the file-explorer metaphor. Completion state always remains in the data and on the server. **Checking off a task while completed are hidden keeps it visible** (shown done) until you leave the list — switching to another list or to the calendar/agenda — so you can see what you just did; opening/closing a popup does *not* trigger the hide.

**Sibling task order** is a smart sort — due date (soonest first), then priority, then title. Predictable and zero-maintenance; the sort key can become configurable later. Manual ordering rejected: iCal has no standard order field, so hand-arranged order wouldn't reliably survive other clients.

**Undo** is a session-scoped stack on the `u` key — every local mutation (edit, delete, complete, re-parent) pushes the prior `.ics` version onto an in-memory stack. Cheap on this storage model, and the safety net that makes single-key actions trustworthy. Persistent trash deferred unless it proves needed.

### Search

`/` starts an incremental search over the current view — the task tree, the agenda, or the calendar names — with `n`/`N` for next/previous match; `:search <text>` is the command form.

### Pane sizing

Layout proportions adapt automatically to terminal size (tview reflows the `Flex` tree on every resize). On top of that, two interactive controls:

- **Accordion expand** (`+` / `-`): collapse the side panels and Detail so the focused Main view (calendar grid or task tree) fills the screen, then restore them — the lazygit `+`/`_` idiom. (In the week/day time-grid these keys instead zoom the hour-row height; the accordion applies in the other views. It is **not available in Agenda mode** — whose center navigation is driven by the Agenda list, not the Main pane — and flashes a hint instead.)
- **Keyboard resize**: `Ctrl-←` / `Ctrl-→` quick-resize the overview column in steps. `Ctrl-W` enters a modal **resize sub-mode** (badge: `RESIZE`) where `←`/`→` (or `h`/`l`) size the overview and `H`/`L` size the Detail pane, `Enter` keeps and `Esc` **or `q`** cancels (reverting to the pre-resize widths) — a keyboard- and terminal-robust way to size either side pane (no exotic modifier chords, so it works on a bare Pi console). Both widths are clamped to sane minimums by the app's own clamp helpers before the width is applied via tview's `Flex.ResizeItem`.

Chosen sizes (overview + Detail widths) are remembered across launches in the state file under the data dir (never the config file). Mouse drag-to-resize is intentionally out of scope — LazyPlanner is keyboard-first.

### Keybindings (vim-flavored; hardcoded for now, config `[keys]` section possible later)

The keyboard interface feels like **vim, not lazygit**: single keys for panel focus and toggles, and short **chords under a prefix** for grouped actions — a which-key popup lists the continuations after a prefix, and `?` shows the full cheat sheet. Panels are on mnemonic letters (`c`/`t`/`a`), which frees the number row for vim **counts**.

| Key | Action |
|---|---|
| `↑↓←→` / `hjkl` | Move the highlight; in a drilled week/day, 2D spatial event nav (`↑↓` time, `←→` concurrent events) |
| `<count>` + motion | Repeat a motion (`3j`, `5k`) |
| `gg` / `G` | Go to top / bottom of the list, tree, or calendar grid (`<count>G` → nth item of a list, the tree, or a drilled day; an undrilled 2D grid lands on the last day) |
| `c` `t` `a` | Focus Calendars / Tasks / Agenda panel |
| `Tab` / `Shift-Tab` | Cycle pane focus |
| `+` / `-` / `0` | In week/day view: zoom the hour-row height in/out; `0` resets to auto-fit (remembered across launches). Elsewhere: `+`/`-` expand / restore the Main pane (accordion) |
| `Ctrl-←` / `Ctrl-→` | Narrow / widen the overview column (quick keyboard resize) |
| `Ctrl-W` | Resize sub-mode: `←`/`→` overview, `H`/`L` Detail, `Enter` keep, `Esc`/`q` cancel |
| `Enter` | Select / open in Main (drill into a day; `j`/`k` then cycle its items) |
| `i` prefix | Create: `it`/`iT` task, `ie`/`iE` event, `is`/`iS` subtask (Shift = full form), `ic` calendar, `il` list. `i!`… (e.g. `i!e`) forces creation on an unknown-type (`[?]`) calendar — read-only and known-wrong-type are never forced |
| `e` | Edit selected (full form); with the Calendars **or** Tasks overview panel focused, open the calendar/list **edit form** (name + color) — symmetric with `d` |
| `s` prefix | Quick-set a task field: `sp` priority, `sd` due date (one-line inputs; blank clears) |
| `Space` | Toggle the selected/drilled task done (any view) — or, in Calendar view with nothing drilled, hide/show the highlighted calendar (remembered in the state file); drilled into an **event**, flashes "can't complete an event" instead |
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
| `y` / `Y` | Cut / copy a task (and its subtree) to the clipboard |
| `p` / `P` | Paste under the selected task / at the list top level (clipboard persists → paste repeatedly) |
| `m` | Grab mode: temporally manipulate the selected item — move an event (`j`/`k` ±hour in week/day, `h`/`l` ±day, `J`/`K` resize) or nudge a task's due date (`j`/`k` ±day, `h`/`l` ±week). `Enter` keeps, `Esc` reverts. Undated tasks are skipped; a recurring event first prompts scope (this occurrence / this & future / all) |
| `V` | SELECT mode: extend a contiguous selection (task tree, calendar days, or a drilled day's items) with the movement keys, then bulk `Space` complete, `d` delete, `y`/`Y` cut/copy (tree), or `m` grab the whole range as one date-shift. `Esc` cancels |
| `:` | Command mode (`:sync`, etc.) |
| `?` | Help overlay |
| `q` / `Esc` | Quit / back out (zoom, dialogs) |

### `:` commands

`:sync` · `:config` (open in `$EDITOR`, reload on exit) · `:view month|week|day` · `:goto <date>` (smart-parsed) · `:search <text>` · `:account` (switch the active account, when more than one is configured) · `:calendar new|rename|color|hide|show` (server-side via sync where applicable; `color` with no hex opens the swatch picker; `rename`/`color`/`hide`/`show` target the focused task list when issued from the Tasks pane, symmetric with `e`/`d`) · `:conflicts` (list/resolve conflicted items) · `:help` · `:q`

Several commands accept a shorter alias: `:q`/`:quit`, `:search`/`:find`, `:account`/`:acct`, `:calendar`/`:cal`, `:conflicts`/`:conflict`, `:help`/`:h`.

### Mouse

Click focuses panes and selects items, including the row under the cursor on the center agenda board; clicking a folder in the task tree expands/collapses it; double-click opens the edit form for the item under the cursor (tree or agenda board); the scroll wheel scrolls panes/lists. (Wheel-paging the calendar month/week/day grid was considered and dropped — the keyboard `f`/`b` pages them; the custom-drawn center views, the calendar grid and the agenda board, take no wheel handler.)

The mouse is **fully inert** during **SELECT**, **GRAB**, and the **Ctrl-W resize sub-mode** — every mouse action is swallowed, wheel included, so a stray click or scroll can't desync the modal state.

---

## Build Plan

The Build Plan is versioned: each version gets its own subsection, planned out with an agent **before** implementation begins, and the completed subsections stay in place as the compressed history of everything done to the project (the detailed record is `log.md`). Every step ends with passing tests (`go test ./...`, vet, staticcheck) and a buildable program, logged in `log.md`.

### v1.0.0 — complete (2026-07-12)

#### Build steps

1. **Scaffold** — `go mod init github.com/littekge/LazyPlanner`, directory skeleton, vendor setup, `.gitignore`, CI (GitHub Actions running test/vet/staticcheck on push), and a hello-world tview window that opens and quits cleanly (`q` / `Ctrl-C`). Proves the toolchain end to end.
2. **Core model** — `internal/model` types (Event, Todo, Calendar) parsed from `.ics` via go-ical; tests against fixture files covering the basics (all-day vs timed events, todos with due dates, timezones).
3. **Recurrence** — RRULE expansion via rrule-go wrapped behind a model API ("occurrences of X between dates A–B"); timezone-aware; heavily tested (recurrence is a classic bug farm).
4. **vdir store** — `internal/store`: read a vdir tree into the in-memory index, atomic writes back to disk (write-temp-then-rename), sync-state sidecar read/write; tests against fixture vdir trees.
5. **CalDAV one-way import** — `internal/caldav` + a first `:import`-style path: connect to NextCloud, discover calendars/todo lists, download everything into the vdir. Doing this *before* building real UI validates the model against real-world NextCloud data early, when fixing parsing assumptions is cheap.
6. **UI shell (read-only)** — pane layout, navigation between panes, a todo-list view and a simple agenda view over real imported data.
7. **Calendar views** — month / week / day grids with movement keys.
8. **Editing** — create / edit / complete / delete todos and events from the UI; writes go to the local vdir only. Separate **create-task** (top level) and **create-subtask** (under the selection) actions; **quick-add smart parser** with a toggle to a **full form** (both offered as distinct keys for now); tasks with incomplete children behave as **folders** (see Data model); session **undo**; indent/outdent (re-parent). Cosmetic pass: rounded borders, black/white dialogs, outline-box selection in the agenda (matching the calendar).
9. **Two-way sync** — ETag-based diff, push local changes, pull remote changes, conflict handling, manual sync trigger. **This completes the must-have feature.** Also: **in-app calendar / todo-list creation and deletion** (CalDAV `MKCALENDAR` / `DELETE` via `internal/caldav`) — created offline-first (a local collection made now, the server round-trip on push) — and wiring the **sync-status indicator** in the status bar. Also **namespace the local cache by account** (`<dataDir>/<account-id>/calendars/…`) — done here rather than later because the sidecar's ETags/hrefs are meaningful only against the server that issued them; see the **Account model** decision below.
10. **Command mode & keybinding polish** — `:` command line (opens an input line near the top; the status bar's middle "command view" echoes the most recently executed action, lazygit-style), a **vim-style chorded keymap** (group related actions under a prefix — e.g. `a` → `at` task, `as` subtask, `ac` calendar, `al` list; map as many actions as possible to short sequences), single-key shortcut coverage, help screen, mouse support pass, and interactive pane sizing (accordion expand + keyboard resize; chosen sizes remembered in the state file). A finale moved the keymap to its final shape: panels on mnemonic `c`/`t`/`a`, create on the `i` prefix, `g`-prefix go / `z`-prefix fold / vim counts, incremental search, calendar visibility toggle, quick field-set (`s` prefix), and yank/paste for tasks.
11. **Recurrence editing semantics** — "this occurrence / this and future / all" editing flows for events (write-side primitives in `internal/model/recur_edit.go`), the single-live-instance advance model for recurring todos, and **grab mode** (`m`) as the unified temporal-manipulation layer. Also `:calendar rename`/`color` (offline-first `PROPPATCH`), the unified create/edit calendar form with the swatch color picker, both-ways name/color sync, and read-only calendar detection.
12. **Background sync + polish** — periodic sync (`sync_interval_minutes`); **incremental sync** short-circuit via the collection CTag (an unchanged CTag with no local changes skips the full re-download); sync status indicator; error surfacing in the UI. (Full `sync-collection` REPORT delta sync is a deliberate deferral — see Settled Decisions.)
13. **Raspberry Pi target** — ARM cross-compile (`make cross`: stripped arm64/armv7/armv6 binaries), performance check, dedicated-terminal (kiosk) setup notes in the README.

### Hardening & audit ledger (all versions, ongoing)

Post-1.0 the project entered a continuous hardening phase: deep, multi-angle audits — gaps vs. spec, cross-program consistency, adversarially-verified bug hunts across concurrency / hostile input / sync data-loss / UI state — with every finding fixed repro-first, carrying a regression test and its own full-gate commit. From pass 10 onward, audits run through the reusable `hardening-audit` workflow (coverage-first surface selection, mutation canaries that test the test suite, bounded residual-risk reporting); the coverage ledger and full per-pass reports live in `docs/audit/`, and every individual fix is in `log.md`.

- **Passes 1–3** (2026-07-12) — spec-gap, consistency, and deep-debugging audits: 9 confirmed defects fixed, several sync-core data-loss/TOCTOU races among them; added the concurrent sync-vs-edits `-race` stress test and the opt-in live CalDAV suite (`//go:build live`).
- **Pass 4** (2026-07-13) — fuzz pass over the iCalendar ingest boundary: contained go-ical/rrule-go **panics** on malformed input behind recover guards, and added ingest **healing** so decode-but-unencodable objects stay editable; the fuzz targets + seed corpus now run on the normal gate.
- **Pass 5** (2026-07-13) — scale-performance pass: de-quadratified three hot paths (bounded recurrence expansion, memoized linear `BuildTree`, batched bulk pull via `store.PullRemoteBatch`), each a UI-freeze/memory-blowup at scale; guarded by benchmarks.
- **Pass 6** (2026-07-13) — terminal/display robustness stress over the six custom-drawn widgets (hostile content × 1×1→400×150 geometries): **no defect found**; the stress tests stay as permanent guards (`internal/ui/displaystress_test.go`).
- **Pass 7** (2026-07-13) — network fault-injection at the CalDAV trust boundary: capped every response body (64 MiB `cappingTransport`); oversized/malformed/truncated/wrong-status responses all degrade cleanly.
- **Pass 8** (2026-07-13) — exhaustive timezone/DST recurrence sweep: **no bug found**; the sweep stays as regression guards (`internal/model/tzsweep_test.go`).
- **Pass 9** (2026-07-13) — pre-1.0 audit of the remaining unhardened areas: 16 findings fixed, incl. a path-traversal calendar id that could delete the whole account dir and three recurrence-edit data-loss/iron-rule bugs; added the `FuzzRecurrenceMutations` target; the UI input surface and the sidecar-corruption path were audited and found already sound.
- **Pass 10** (2026-07-13) — first run of the `hardening-audit` **workflow**: 9 findings (5 HIGH — decode-but-unencodable go-ical shapes each poisoning a whole resource, and yank/paste corrupting foreign *bundled* multi-item `.ics` files) + 3 canary coverage holes, all fixed.
- **Pass 11** (2026-07-15) — stale/never-surface sweep (grab mode, recurrence-edit UI, sync concurrency): 7 findings rooted in **multi-write-without-rollback** ops and the first **`Locate→Put` version-check clobber**; introduced `store.PutIfUnchanged`.
- **Pass 12** (2026-07-15) — systemic sweep of the `Locate→Put` class + session undo + calendar PROPFIND decode: 7 findings; introduced `store.RestoreDirty` (undo of a *synced* edit/delete now survives sync), fixed an iron-rule STATUS/PERCENT-COMPLETE flatten and a read-only-detection fail-open (`caldav.hrefKey`).
- **Pass 13** (2026-07-16) — whole-app spec-diff re-run: 5 findings — a degraded download no longer reads as a remote **deletion** (HIGH), the `Locate→Put` class closed *exhaustively* after the spec-diff disproved pass 12's closure claim (the edit form's `applyMutation` + `reparentSelected` were still unguarded), and `MKCALENDAR`/`DELETE` made **idempotent** — + 4 canary holes closed.
- **Pass 14** (2026-07-18) — coverage-first sweep of the top stale/never surfaces (reconcile matrix, tz resolver fuzz, quick-add semantics, recurrence write-side spec-diff): 6 findings (HIGH 0 · MED 5 · LOW 1) + 1 escaped canary, all fixed. Three shared an **RDATE/EXDATE-semantics** root cause — multi-valued lines collapsing a series, EXDATE'd instances miscounted against COUNT on split, a trailing RDATE surviving UNTIL to duplicate across a split — codified as a new Hard-won guardrail. Also: `pushDelete`'s 412 branch no longer drops a delete-vs-server-change conflict, keep-local of a server-deletion converges, quick-add rejects an impossible day-of-month. First HIGH-free pass; total ticked 5→6.
- **Pass 15** (2026-07-18) — gap-closing pass on the stale/never matrix cells (CalDAV response-parse fault-injection, store disk-fault atomicity + the first direct `-race` of the store write primitives, reconcile keep-both/Forget/read-only-twin, import ingest fuzz): 3 findings (HIGH 2 · MED 1) + 1 escaped canary. Both HIGHs were data-loss: a CalDAV **write silently reported success on an HTTP redirect** (method-aware `CheckRedirect`), and the load-time **stale-temp sweep deleted a legitimate `.ics`** whose name matched the temp heuristic. The MED — import drops a valid sibling when a resource mixes a UID-bearing with a UID-less component — is an owner-accepted residual (every fix crosses a hard invariant; reachable only from a malformed foreign `.ics`). **HIGH resurged 0→2**, confirming the stale cells still hid real bugs.
- **Pass 16** (2026-07-18) — closed the last stale headless cells (mouse input-edge, `:config`/`$EDITOR` fault-injection, adjacent go-ical encoder + CLI wiring): 6 findings (HIGH 2 · MED 2 · LOW 2) + 2 escaped canaries, all fixed/guarded. Both HIGHs **reopened the pass-10 decode-but-unencodable heal class** — a VJOURNAL/VFREEBUSY missing DTSTAMP or carrying a duplicate prop, and a VTIMEZONE missing TZID/required STANDARD-DAYLIGHT props, each still bricking a whole resource; healed (DTSTAMP + dedupe extended to VJOURNAL/VFREEBUSY) and stripped (`dropUnusableTimezones`, owner-approved), and **codified as a Hard-won guardrail: the heal set must mirror go-ical's full encoder validation**. Also `:config` reload surfaces Load's warning, subcommand `-h/--help` exits cleanly, and double-click edits the row under the cursor.
- **Pass 17** (2026-07-18) — first re-sweep pass (model-side parsers + one write path; first audits of `color.go` and the Windows→IANA zone table, both clean): 2 findings (HIGH 0 · MED 2) + **4 of 4 escaped canaries**. Both MEDs had the *missing-guard-a-sibling-path-already-has* shape — the Import loop lacked `reconcileCalendar`'s empty-href skip (a hostile server's empty-`<href/>` objects silently overwrite each other while reporting success), and `resolveDateTime` had a Windows-name TZID recovery branch but no IANA one (an `RDATE;VALUE=PERIOD` silently mis-zoned to floating). The heavier signal was the canary sweep: all four escaped, two of them twins of already-known patterns, all closed with boundary tests.
- **Pass 18** (2026-07-21) — first audit of the never-covered v1.1.0 multi-account surfaces (config-parse fuzz, `global.json` + `runTUILoop` fault-injection, `:account` input-edge, v1.1.0 promise spec-diff) plus the long-deferred **deep sync-core TOCTOU re-sweep** (first since pass 11): 3 findings (HIGH 2 · MED 1) + **4 of 4 escaped canaries** (second consecutive 4/4), all fixed/closed. The HIGHs: a nested-inline-table config decodes in **O(depth²)**, hanging startup for minutes-to-hours while staying *inside* the 4 MiB read cap — the cap bounds bytes, not decode CPU (`checkNestingDepth` now rejects nesting past 64 levels before `toml.Decode`); and `store.CommitPush` **resurrected a resource deleted mid-push**, silently losing the delete (`cur==nil` now honors the deletion and advances the tombstone ETag so the pending conditional DELETE still matches). The MED: `:config` reload discarded the re-parsed account list, so a `:config`-added account stayed unswitchable until restart. HIGH resurged 0→2 — expected in direction (a brand-new feature surface plus the oldest heavy surface on its first re-visit), but the no-HIGH streak resets.
- **Pass 19** (2026-07-24) — first audit of the never-covered v1.4.0 SELECT/bulk-ops surface, the v1.3.0 recurrence-rewrite primitives, the v1.2.0 quick-add grammar additions, and the reconcile-vs-concurrent-pull matrix beyond the `CommitPush` window: 8 findings (HIGH 4 · MED 2 · LOW 2), all fixed repro-first, plus both escaped canaries closed. The whole-arc review came back **READY TO MERGE** (0 Critical/Important, 7/7 cross-cutting checks) but recommended `more_passes_recommended`: severity ran **up** (HIGH 2→4, total 3→8), as expected on a first look at three untouched feature surfaces.
- **Pass 20** (2026-07-25) — followed pass 19's flagged next targets (the reconcile matrix beyond the CommitPush/pushDelete-412 windows, `moveSubtreeOps`'s cross-collection write, bulk grab, and a whole-app promise spec-diff): 5 findings (HIGH 1 · MED 2 · LOW 2) + 2 escaped canaries, all fixed/closed. The HIGH — reconcile step-(A)'s `Forget` clobbering a concurrent local edit — was the **third reopening of the "resource-is-gone" reconcile class**; guarded by `store.ForgetIfUnchanged` and codified as its own Hard-won guardrail. Raw severity fell (HIGH 4→1, total 8→5) but the class reopening plus 2/4 canary escapes keep it `more_passes_recommended`; the named residual is the cross-package resource-is-gone sweep (`internal/model` / `internal/caldav` peer writes).
- **Pass 21** (2026-07-25) — the least-covered ledger rows (recurrence read-path re-fuzz, the go-ical heal-set spec-diff, CalDAV request-construction fault-injection, and the read-only/peer twins of the pass-20 classes): 3 findings (HIGH 1 · MED 2), all fixed repro-first, plus the 1 genuine canary escape closed. The HIGH was the **third reopening of the heal-set class**, this time at *recursion depth* — a phantom component nested under a VALARM/STANDARD/DAYLIGHT bricked the whole resource on re-encode; fixed by completing `model.allowedChildren` so `stripForbiddenNesting` strips it before go-ical's recursive `checkComponent` sees it, with the guardrail extended to demand that depth. The MEDs: recurrence expansion had no *aggregate* step budget (N far-anchored high-frequency events froze each redraw — fixed with a shared `model.StepBudget`), and PROPPATCH treated any 207 as success without inspecting the per-property status (a server-rejected rename/recolor silently dropped — fixed by parsing the propstat status). Severity held (HIGH 1→1, total 5→3); the cross-package resource-is-gone sweep remained the standing residual.
- **Pass 22** (2026-07-25) — took direct aim at that carried residual (`internal/model` / `internal/caldav` peer write paths) and largely **retired** it: `internal/model` is pure/headless so the concurrent-delete class cannot manifest there by construction, and the `internal/caldav` conditional-write paths held under fault injection. The one confirmed finding was elsewhere — 1 MED: the v1.3.0 Custom repeat "Ends on date" dropped the selected end day for *timed* items, because UNTIL was stored at midnight and made inclusive only for all-day anchors; fixed by anchoring UNTIL at the selected date plus the anchor's own time-of-day. Findings converged (total 3→1, HIGH 1→0) but the mutation-canary net regressed sharply — **3 of 4 escaped** (caldav `normalizeETag`'s `W/` strip, `drillRange` bounds, `reconcileReadOnly` dirty-discard), all closed with boundary tests verified to kill their mutations. Residual: the remaining `internal/caldav` write-path sweep.
- **Pass 23** (2026-07-25) — spent five of its six slots on ledger rows marked `never`, and they were high-yield: **15 findings (HIGH 6 · MED 6 · LOW 3)**, all fixed repro-first, plus the 1 escaped canary and 2 coverage holes closed. The HIGHs: a sidecar tombstone key that let sync **write outside the cache root**; a corrupt sidecar **discarding all sync metadata** (clobbering unsynced edits and resurrecting deletions); a **failed** conflict-resolve that still resolved in memory; the **fourth** reopening of the heal-set class; `FREQ=YEARLY;BYMONTHDAY=n` decoded as plain yearly (it actually fires 12×/year, and a grab made the event vanish); and a Θ(n²) `LayoutDay`. Two systemic lessons became Hard-won guardrails. First, the heal-set guardrail's "enumerate every container" wording was **unsatisfiable** — go-ical recurses into containers `checkComponent` has no case for, which is *why* the class kept reopening; the strip is now deny-by-default over encoder-validated types, and `internal/model/encoderdrift_test.go` makes the gate enforce the table re-diff that had failed manually in three of the four reopenings. Second, **a fix can land, pass its test, and never reach production** — pass 21's `StepBudget` and pass 22's timed UNTIL were real fixes whose regression tests called the helper directly while the UI wiring never delivered the fixed value. The arc also surfaced **four defects the audit missed**, the largest being that all-day "Ends on date D" *also* dropped D — two bugs whose signs flip with the UTC offset, **falsifying the pass-22 close-out** that recorded all-day as correct.

**Convergence is measured as a severity-weighted trend, not a raw finding count** (the explicit criteria live in `docs/audit/PROTOCOL.md` — "Convergence — the stop rule"). Status after pass 23:

- **HIGH by pass**: 5 (10) → 1 (13) → 0 (14) → 2 (15) → 2 (16) → 0 (17) → 2 (18) → 4 (19) → 1 (20) → 1 (21) → 0 (22) → **6 (23)**.
- **Criterion 1 — the headless matrix covered once — remains met.**
- **Criterion 2 — two consecutive no-HIGH passes — is at zero.** It has reset four times now (passes 15, 18, 19, 23), each time because a never-audited or long-unvisited surface was opened; the streak restarts from the next no-HIGH pass.
- **Pass 23's spike is a measurement effect, not a regression.** Pass 22 re-examined thrice-hardened surfaces; pass 23 opened never-audited ones. The lesson recorded in the ledger is that `never` rows are *not* low-yield — the two the plan flagged as highest-value both returned HIGHs immediately, one of them an out-of-cache-root write.
- **The standing weak spot is the test net, not the code.** Canary escapes ran 4/4 (17), 4/4 (18), 2/3 (19), 2/4 (20), 1/4 (21), 3/4 (22), 1/4 (23) — every one closed with a boundary test. The boundary-class rule in `docs/audit/PROTOCOL.md` (test both sides of every half-open window; mirror every sibling path's guard) continues to apply.

**Next-pass (24) targets**, carried from `docs/audit/passes/PASS-23.md`: `internal/caldav` write paths (unswept for the bare-write class two passes running); race and fault-injection (not exercised in pass 23); direct iCalendar decode/ingest fuzz (last direct pass since 4); the pass-11/12-era UI data-loss surfaces and the pass-18-era multi-account surfaces; the write-side `safeAfter` aggregate-budget gap; and the decomposer asymmetry that accepts `FREQ=WEEKLY;BYDAY=TU,TH` on a Monday anchor where monthly/yearly reject the equivalent.

**Permanently-accepted gaps**: the **Raspberry Pi on real hardware** (unauditable headlessly); a missing **UID** on any component is not fabricated (it would churn sync identity), so a UID-less component stays display-only and a resource mixing it with a valid sibling drops the whole resource on import (pass-15 MED, owner-accepted residual); and the empty-VCALENDAR encoder constraint is not auto-healed.

### v1.0.1 — bug fixes

Targeted patch-level bug fixes found outside the audit cadence (distinct from v1.0.0's hardening-pass ledger above). Every fix is repro-first with a regression test and full-gate commit; the detailed record is `log.md`.

- **Sync no longer resets the highlight** (2026-07-20) — `refresh("")` after a sync fell back to the first task and reset the calendar's day-drill, so periodic/debounced sync snapped the tree cursor to the top and kicked the user out of event-cycling. `refresh` now preserves the current position when given no explicit `selUID` — the tree UID via `currentTreeUID`, the calendar drill via `drillState`/`reDrill` — with no focus change, so a background sync can't steal focus from an open modal. The agenda view was checked and is unaffected. Guards: `internal/ui/synchighlight_test.go`.

### v1.0.2 — bug fixes

Same charter as v1.0.1: targeted patch-level fixes outside the audit cadence, each repro-first with a regression test and a full-gate commit; the detailed record is `log.md`.

- **Month grid: multi-day timed event no longer repeats its start time** (2026-07-20) — `DayAgenda` copied the occurrence start verbatim onto every covered cell. `AgendaItem` gained an `End` field and `itemLabel` now branches on the cell's day (behavior described under Calendar views). Guards: `internal/ui/multiday_test.go`.
- **Week/day time grid: multi-day timed event renders on every day of its span** (2026-07-20) — `splitOccs` bucketed a timed occurrence onto its start day only (the all-day branch already fanned out) and block geometry was time-of-day-blind, so the event vanished after day one. It now fans across every covered day (new `Occurrence.OverlapsDay`) and draws per-day clipped blocks (behavior described under Calendar views). Guards: `internal/ui/multiday_test.go`, `internal/model/calendar_test.go`.
- **Debounced/periodic sync deferred while a create/edit form is open** (2026-07-20) — the timer-driven push could fire mid-edit; pushing the just-edited resource stores a new pointer, so the open form's version-checked Save read as stale and `commitMutation` tore the form down, discarding everything typed. The version-check CAS itself is untouched (it still guards the genuine concurrent-pull clobber); see the Sync-triggers decision for the shipped behavior and the in-flight residual. Guards: `internal/ui/sync_modal_test.go`.

### v1.1.0 — account switching (released 2026-07-22)

Full multi-account profiles: several configured CalDAV accounts, one **active** at a time, switchable in-app without a restart. No merged multi-account view — explicitly out of scope for this version. All decisions below are owner-settled (2026-07-21). The settled behavior now also lives in the Account model / Config decisions below (updated in place); this subsection stays as the build record.

**Status**: build steps 1–5 all implemented, each repro-first with a green full gate (per-step record in `log.md`). **Released 2026-07-22** (owner merged and tagged; CI-built binaries, `linux_amd64` checksum-verified and smoke-tested). All automated verification is headless — config parse/migration, resolver logic, the cmd rebuild loop, the UI command/picker/status, and flush-on-switch mirroring the quit-flush tests — plus a binary smoke of the migration error and first-run template; **live two-account end-to-end sync was verified manually against the real CalDAV server** (owner, 2026-07-22).

Behavior settled here that is **not** restated in the Account model / Config decisions below:

- **Config schema.** Duplicate `name`s or a nameless `[[account]]` block are a load error — config ambiguity stays fatal by design. The first-run template generates one commented `[[account]]` block.
- **Active-account memory.** The global state file lives at the data-dir root (per-account state files stay where they are) and is written on each successful switch; corrupt or missing global state is never fatal — it falls back to the first block.
- **Switch mechanics.** `ui.Run` returns either quit or "switch to account X". On `:account X` the UI validates the name, then winds down exactly like quit — cancel the sync context, stop timers, run the time-bounded best-effort flush — and returns; the `cmd` loop (still thin wiring: load config → resolve account → run UI → repeat on switch) reopens the new account's store, state, and sync closure. The undo stack and all view state die with the old App, same as quit. Unpushed changes that miss the flush stay in that account's cache and sync when it is next active.
- **UX.** Bare `:account` opens a picker modal (the existing form-modal pattern) listing accounts with the active one marked; the status bar shows the active account name whenever more than one is configured. Switching to the current account is a no-op flash, an unknown name an error flash, and `:account` with no accounts configured errors cleanly.
- **`:config` interplay.** Reload re-parses the account list so the picker and status bar update live; a reload never yanks a live store.
- **Failure modes.** A failing `password_command` on the switch target behaves like startup — warning, and the account opens offline over its cache. A failed store-open on switch reports and falls back to reopening the previous account; if even that fails, the app exits with the error.

**Build steps** (each with tests, its own commit, full gate):

1. **Config**: `[[account]]` parse + validation + `[server]` migration error + template rewrite.
2. **Global state file**: last-active-id read/write, corrupt-tolerant.
3. **cmd loop**: account resolution (stored-id → first-block fallback), `ui.Run` switch-result plumbing, rebuild loop, previous-account fallback.
4. **UI**: `:account` command + picker modal + status-bar segment + wind-down path reusing the quit flush; display-stress coverage for the new modal.
5. **Docs ripple**: README (Configuration/Usage/keybindings), main.md Settled Decisions rewritten in place (Account model → multi-account one-active; Config → `[[account]]` schema + migration note).

### v1.2.0 — quick-add parser improvements (released 2026-07-23)

Extend the quick-add smart parser with four grammar additions — time ranges, simple recurrence, relative dates, and a location token — plus obvious-error warnings with a fix-it re-prompt. All decisions owner-settled 2026-07-22. The existing contract holds throughout: **space-delimited tokens** (a sigil matches only at the start of a whitespace-delimited token — `Title@house`, `task!5`, `bob@example.com` are inert title text), first-match-wins slots, unmatched tokens stay in the title, rules predictable and documented in `:help`. The shipped grammar now lives in the `Creation: quick-add` design section above (rewritten in place at the docs-ripple step); this subsection remains as the build record.

**Status**: all six build steps implemented, each repro-first with a green full gate (per-step record in `log.md`). **Released 2026-07-23** (owner merged and tagged; all 8 CI-built assets present, `linux_amd64` checksum-verified and smoke-tested reporting v1.2.0).

Grammar precision settled here and only summarized in the design section above:

- **Time ranges.** A right-side am/pm distributes to a bare left half (`5-6pm` = 5pm–6pm); an end at or before the start rolls to the next day (`11pm-1am`). Two bare numbers (`3-4`) are never a time. The range fills the existing first-time-wins slot; an event's 1-hour default applies only when no end is given, and a task takes the range's start with the end ignored (documented behavior, not a warning).
- **Recurrence.** `every <weekday>` → `FREQ=WEEKLY;BYDAY=…`; `every <month> <day>` → yearly on that date; a recurring task uses the shipped single-live-instance model. Accepted trade-off: in `daily standup 9am` the word `daily` is a token (title "standup", recurs daily) — the same class as bare `fri` being a date, and documented as such.
- **Relative dates.** `next <weekday>` is the bare-weekday result **+7 days** (one rule, no week-start dependence); `next month` is the same day-of-month next month clamped to that month's last day; in `in N …`, N is 1–3 digits and months clamp the same way. Free riders: `:goto` and `sd` gain the whole family automatically.
- **Location.** A pre-lexer keeps a token-leading `@"…"` quoted span together across spaces. LOCATION is legal on a VTODO and NextCloud Tasks shows it, so tasks get it too; the task Detail pane gains a Location row when non-empty. A bare `@` is title text ("lunch @ noon").

**Obvious-error warnings.** The parser returns `Warnings []string` alongside the normal result and never blocks parsing (a failed token falls to the title). A warning fires **only on an unmistakable intent anchor**:

1. `!` followed by 1+ purely alphanumeric chars that fail the priority parse (`!t`, `!0`, `!hgh`); a duplicate priority token also warns. Punctuation runs (`!!!!`, bare `!`, `!?`) are silent title text.
2. An unclosed `@"…` quote (bare `@` is silent).
3. Anchor word + fuzzy follower — `next X` / `every X` / `in N X` where X is within Damerau–Levenshtein distance ≤ 2 of a weekday, month, or unit name but not exact (`next tuedsay`, `in 3 dayz`). `next steps` and `in 3 acts` are nowhere near and stay silent.
4. Shape triggers — impossible colon-times (`25:00`, `12:99`; `http://…` is safe, its halves aren't bare numbers), failed time-range shapes (`5-6xm`, `5pm-`), impossible ISO dates (`2026-07-40`), and the three-part `m/d/y` form with an impossible date. **Slashed two-part near-misses stay silent** (`24/7`, `7/45` are plausible titles).

A lone `#` is silent. Warning text names the token and what it resembled. On submit with warnings nothing is created and the warned text is remembered; any edit re-parses fresh and `Esc` cancels.

**Architecture** (extends the existing single-pass loop; no rewrite): a pre-lexer replacing `strings.Fields` (identical output except quoted `@"…"` spans); new recognizers in the existing loop — `parseTimeRange` (before `parseClock`), `parseRecur` (multi-token via the existing `consumed` mechanism), extended `parseDate`, `parseLocation`. `QuickAdd` gains `HasEnd`/`EndHour`/`EndMinute`, `Recur *RecurSpec` (small struct — Freq/Weekday/Month/Day — not a raw RRULE string), `Location`, `Warnings`. `TodoDraft`/`EventDraft` gain recurrence + (todo) location; `NewEventObject`/`NewTodoObject` serialize the RRULE; existing recurrence machinery (scope pickers, single-live-instance todos) keys off the RRULE and needs no changes. Model stays pure — no I/O, no UI.

**Testing.** Boundary-class tables per recognizer (both sides of every window, per the audit guardrail): `12:00am/pm`, end==start rollover, `next fri` on a Friday, `in 1 day` / `in 999 months`, Feb-29 clamps, fuzzy distance 2 vs 3. An **adversarial zero-warning title table** is asserted verbatim in `internal/model` (plausible prose that must stay silent). `FuzzParseQuickAdd` gained new-grammar seeds plus the invariant that a warning only ever fires alongside an intent anchor (~1M execs clean). UI tests cover the keep-open re-prompt flow (warn → identical resubmit creates; edited resubmit re-parses).

**Build steps** (each with tests, its own commit, full gate):

1. **Relative dates** — extend `parseDate` with the `next` family and `in N …`; `:goto`/`sd` ride along.
2. **Time ranges** — `parseTimeRange`, the `HasEnd` fields, `createEvent`/`createTask` end handling.
3. **Recurrence** — `RecurSpec`, `parseRecur`, the anchoring rule, draft RRULE serialization for events and todos.
4. **Location** — the quoted-span pre-lexer, `parseLocation`, draft plumbing, task Detail-pane Location row.
5. **Warnings + re-prompt UX** — `Warnings`, all trigger classes, the adversarial table, the keep-open submit flow, the fuzz invariant.
6. **Docs ripple** — `:help`, README quick-add documentation, and the main.md `Creation: quick-add` section rewritten in place.

### v1.3.0 — recurrence-rule UI (released 2026-07-24)

**Status**: all six build steps implemented repro-first with green full gates (2026-07-23), followed by post-build dialog polish (see Post-Build Incremental Changes) — every UI item scoped before release shipped. Verified headlessly (see Testing below). **Released 2026-07-24** (owner merged and tagged; all 8 CI-built assets present, `linux_amd64` checksum-verified against `sha256sums.txt` and smoke-tested reporting v1.3.0).

Close the recurrence-creation gap in the full forms (acknowledged 2026-07-22: quick-add v1.2.0 is otherwise the only in-app way to create a recurring item, and an existing rule can't be rewritten in-app at all). A **Repeat field** in both full forms plus a **Custom… sub-form**, with Google-Calendar-style expressiveness — the owner's benchmark use case is "every week on Tuesday and Thursday until an end date". All decisions owner-settled 2026-07-23.

**The editable set (Google-custom parity).** Frequency daily/weekly/monthly/yearly · interval "every N units" · weekly on a *set* of weekdays · monthly by day-of-month **or** by nth weekday (1st–4th or last, derived from the start date, like Google — no free-floating nth/weekday picker, so the rule and its anchor can't contradict) · yearly on the start date · end condition never / until a date / after N times. Anything beyond this vocabulary is out of editing scope but fully preserved (below).

**Model: `RecurSpec` extension (zero-value compatible — quick-add behavior unchanged).** New fields: `Interval int` (0/1 = every), `Weekdays []time.Weekday` (the existing single `Weekday`/`HasWeekday` pair migrates into a one-element slice), `MonthlyNth int` (1–4, −1 = last) + `MonthlyWeekday`, `Until *time.Time` / `Count int` (at most one set). `ROption()` extends accordingly (all-day UNTIL serialized date-only, reusing the `dateOnlyUntil` pathway). Two new pure functions: a **humanizer** (spec → "every 2 weeks on Tue, Thu until Dec 12, 2026") for the dropdown and Detail pane, and a **decomposer** `RecurSpecFromRule` (RRULE → spec, ok) that is deliberately conservative — any feature outside the vocabulary (BYSETPOS, HOURLY, multi-BYMONTH, nth-ordinals on weekly, WKST, …) returns ok=false, and so does a rule that **contradicts its own anchor** (an explicit BYMONTHDAY or nth-weekday that doesn't match the DTSTART/DUE date — the editable set derives those from the start date, so a disagreeing rule can't be seeded faithfully and stays Custom (kept)). Serialize→decompose round-trips to identity for every representable shape.

**Rewrite only when changed.** The form seeds the Repeat state from the decomposer; on save, if the read-back spec equals the seeded spec the RRULE is **not touched** — the original bytes survive (iron rule: a semantically-equal rewrite could still drop oddities like WKST). Drafts gain `RecurRemove bool` alongside the existing `Recur *RecurSpec`: nil + !remove = keep untouched, nil + remove = delete the rule, non-nil = rewrite.

**Scope interactions.** Create and edit-of-a-non-recurring item show the field (picking a rule makes the item recurring — no scope picker involved). Scope **All**: field shown, seeded from the master; a change rewrites the master's RRULE. Scope **this occurrence** (and the todo detach): field **hidden** — an override/detached one-off has no rule. Scope **this & future**: field shown, seeded; a change gives the split-off future series the new rule while the capped past keeps the old one — untouched keeps the existing COUNT-rebalancing split math, an explicitly-set count is taken as the future series' own end.

**Baggage rules (owner-settled).** Repeat → **None**: RRULE, EXDATEs, RDATEs, and override components are all removed — one plain item remains (deliberate, user-directed, undo-able; the flash says what happened). Rule **changed**: EXDATEs are always kept (excluding a time the rule no longer generates is a harmless no-op); overrides are kept when their occurrence still exists under the new rule and **dropped when orphaned** (an override of a nonexistent occurrence describes nothing and renders as a phantom in some clients) — the flash reports "N edited occurrence(s) removed", undo restores everything.

**UI: Repeat dropdown** (both forms). Options built at form-open from the seeded start/due date: `None · Daily · Weekly on Tue · Monthly on day 25 · Yearly on Aug 25 · Custom…`. Editing an item whose rule is representable but matches no preset adds its humanized text as the selected entry (leaving it = untouched; Custom… seeds from it). An unrepresentable rule adds a selected **"Custom rule (kept)"** entry — leaving it preserves the rule byte-for-byte; picking anything else overwrites (explicit user intent, no extra confirm — undo covers mistakes). Preset labels derive from the seeded date, but on save the weekday/month-day is **re-derived from the final start date**, so editing the date after picking a preset can't disagree (Google-parity quirk).

**UI: Custom… sub-form.** A nested modal over the form (the color-picker focus-stack precedent) carrying **Every** `[N]` `[days/weeks/months/years]`, a weekday selector (weekly only; nothing selected falls back to the start date's weekday), a **Monthly by** dropdown (`on day 25` / `on the 4th Tuesday` / `on the last Tuesday`, derived from the start date; monthly only), and an **Ends** dropdown (`Never` / `On date` / `After N times`) with its date and count inputs. OK validates (N ≥ 1, date parses, count ≥ 1) and writes the humanized result back as the Repeat dropdown's selected entry; Cancel restores the prior selection. Its field layout and weekday control were reshaped after the build — see the Custom repeat sub-form redesign below, which is the shipped form.

**Detail pane.** Recurring events and tasks gain a **Repeats** row: the humanized rule, or `custom (FREQ=…)` raw for a kept-custom rule.

**Recurring todos.** `AdvanceRecurringTodo` walks the recurrence set, so intervals/COUNT/UNTIL work unchanged — table-verified, no code change expected.

**Testing.** Model tables: spec↔RRULE round-trip identity, an unrepresentable-rule catalogue (ok=false + bytes untouched), and a humanizer table. Boundary-class per the audit guardrail (both sides of every window): interval 0/1/2, COUNT 0/1, UNTIL exactly-on vs just-before an occurrence, nth 4 vs last vs a nonexistent 5th, leap-day yearly, all-day UNTIL date-only. Rewrite primitives: unrelated props preserved (iron rule), EXDATE kept, orphan overrides dropped and still-valid ones kept, Repeat→None full cleanup, undo restores, split-with-new-rule — with **`FuzzRecurrenceMutations` extended over the new primitives** (extend, don't fork). UI: seed tables per rule shape, per-scope save wiring, sub-form read/validate, re-derive-on-date-change, this-occurrence hides the row, plus display-stress and focus-stack tests for the new modal.

**Build steps** (each with tests, its own commit, full gate):

1. **Model: spec extension** — new fields, `Weekdays` migration, `ROption()`, humanizer.
2. **Model: decomposer** — `RecurSpecFromRule` + the unrepresentable catalogue.
3. **Model: rewrite primitives** — `Recur`/`RecurRemove` in the edit paths, orphan pruning, split-with-new-rule, all-day UNTIL; fuzz extension.
4. **UI: Repeat dropdown** — presets, seeding, scope wiring, Detail-pane Repeats row (Custom… entry present but stubbed).
5. **UI: Custom… sub-form** — nested modal, validation, display-stress + focus-stack tests.
6. **Docs ripple** — README, `:help`, main.md (the Creation section's "except the recurrence rule" exception comes out).

#### Post-Build Incremental Changes

Behavior refinements after the six-step build (per-change detail lives in `log.md`). **Shipped** items each carry tests and a green gate:

- **Unified dialog chrome.** Every dialog shares one look — the multi-field forms and the confirmation/picker modals alike: an accent rounded border, a contextual title, and the unified terminal-default background with no contrast band. A shared `styleModal` gives the `tview.Modal` confirms/pickers the treatment the forms get from `stylePopup`; each title names the action (` Delete task `, ` Recurring event `, ` Resolve conflict `, …). Selection highlighting is theme-adaptive everywhere, dropdown lists included.

- **DRILL-mode form navigation** (mirroring the app-wide NORMAL/DRILL seam). The form dialogs use a vim-style modal input layer rather than Tab-only field movement, implemented once in the shared `caretForm` (a form-level input capture + a `drilled` flag surfaced through `interactionMode`) so all four forms inherit it:
  - **NORMAL** (forms open here, caret on the first field): `j`/`k`/arrows step through the fields and the Save/Cancel buttons; `h`/`l` (or `←`/`→`) move between the buttons; `g`/`G` jump to the first field / last item. `Enter` drills a text field, opens a dropdown, toggles a checkbox (then advances), or activates a button. `Esc` closes the form. Other keys are inert.
  - **DRILL**: keys reach the focused field — a text field types normally (so `hjkl` are letters and `←`/`→` move the cursor), `Enter` commits and **advances** (auto-drilling the next text field, but stopping in NORMAL on a dropdown/checkbox/button), and `Esc` returns to NORMAL keeping the value. Opening a dropdown (`Enter` in NORMAL) hands off to tview's native list — navigated with `↑`/`↓` (plus type-ahead), `Enter` selects → NORMAL, `Esc` aborts, no auto-advance. The open list is arrow-only rather than `j`/`k` because tview reinstalls its own key capture on the list each time it opens; `j`/`k` remain the field/button navigators in NORMAL.
  - `Tab`/`Shift-Tab` remain aliases for advance / previous. The NORMAL/DRILL state surfaces through the existing `interactionMode` badge.

- **Rigorous confirm for collection deletes.** Deleting a calendar or task list (`d` on the focused Calendars/Tasks pane) is not undoable, so it no longer uses the one-button confirm: a type-to-confirm dialog requires typing the collection's exact name (trim + case-sensitive) before **Delete** fires — a mismatch flashes and keeps the dialog open. Item deletes stay on the ordinary undoable confirm.

- **Custom repeat sub-form redesign.** The Custom… repeat sub-form now shows only
  the fields relevant to the current selection — Every, Unit and Ends always, plus
  the weekday strip only for a weekly rule, "Monthly by" only for monthly, and
  Until / Count only for the matching "Ends" choice — re-laid-out live as Unit or
  Ends changes (values preserved). Weekday selection is a single compact toggle
  strip (a `tview.FormItem` drilled into like any field: `←`/`→` move, Space
  toggles) in place of seven checkboxes.

### v1.4.0 — SELECT mode

**Status**: **released 2026-07-24** (tag `v1.4.0` at `5877c39`). All seven build steps landed repro-first with a green full gate each (see `log.md` for the per-step record), followed by a whole-branch review (two Important fixes — SELECT entry requires the selection surface focused; a bare `0` is swallowed — plus an N>1 paste test gap and a docs restructure) and a grab-discoverability fix (the persistent help bar now shows the active grab's controls/granularity instead of stale SELECT hints). Verified headlessly: the range-derivation boundary tables (reversed/single/capped), a display-stress sweep over five new SELECT draw states, and a `-race` run over the bulk-op and bulk-grab suites. Full design detail lives in `docs/superpowers/specs/2026-07-23-select-mode-design.md`; this subsection is the build record.

A vim-style multi-select layer built on **mode composition**, not a parallel mode enum (the hard-won guardrail this version was scoped around): SELECT nests under DRILL and hosts GRAB the same way GRAB already nests under DRILL, and the range is *derived* (anchor + the view's live cursor) rather than a stored selected-items set, so it can never desync from the screen. Every bulk action shares one shape — materialize the range, filter with counted skips, execute with rollback, one compound undo step — deliberately reusing the single-item `moveSubtree`/grab templates rather than inventing a new commit pattern; see "SELECT mode: multi-select and bulk operations" above for the full behavior.

**Build steps** (each with tests, its own commit, full gate):

1. **SELECT core** — state, badge, and the semi-modal key layer: per-context entry/exit, motion passthrough, everything else swallowed.
2. **Range derivation** — `selRange()`/`treeRange`/`daysRange`/`drillRange`, the `maxSelectDays` cap, and anchor-vanish revalidation on refresh.
3. **Selection visuals** — theme-adaptive range highlighting in the tree, month grid, and time grid; the `N selected` status prefix and hint line.
4. **Bulk complete** (`Space`) — reverse-order walk (children before parents so a folder-plus-last-child completes in one pass), recurring-todo advance, skip counting, all-or-nothing rollback.
5. **Bulk delete** (`d`) — ancestor-deduped roots (`bulkDeleteRoots`, shared with yank), one confirm naming the full count, whole-subtree expansion; a same-day bugfix cycle-guarded the ancestor walk against malformed `RELATED-TO` loops (see below).
6. **Multi-root yank/paste** (`y`/`Y`) — the clipboard becomes `[]string`; `pasteMultiRoot` moves/copies every root under one shared ops/rollback pair, via `store.PutIfUnchanged` (stricter than the legacy single-item path).
7. **Bulk grab** (`m`) — one uniform date-shift over the range, GRAB nested inside SELECT; `Esc` returns to SELECT (not out of it), and a stale mid-nudge keeps already-landed moves as one undo step.
8. **Docs ripple** — this step: README, `:help`, this section, and the coverage ledger.

**Build-time finding.** `bulkDeleteRoots`'s ancestor-absorption walk trusted untrusted `RELATED-TO` parent data with no visited guard — a reciprocal parent cycle (hand-edited or foreign `.ics`) would spin the walk forever, freezing the single-threaded UI event loop (the same "malformed iCalendar must never be fatal" class as the sibling `descendants()` walk, which already carried the guard this one lacked). Fixed same-day with a `seen` map, repro-first (`internal/ui/bulkops_test.go`).

### v1.5.0 — polishing & auditing (in progress)

The last planned release: a consolidation phase — systemic bug-finding and polish, no new features beyond two owner-approved gap-closers. Full design: `docs/superpowers/specs/2026-07-24-v1.5.0-polish-audit-design.md` (owner-settled 2026-07-24).

**Scope**, in priority order:

0. **Step 0 — `moveSubtreeOps` version-check fix**: the source-side rewrite of a cross-list move routes through `store.PutIfUnchanged` (was a bare `Put` — the `COVERAGE.md`-flagged clobber gap), fixed repro-first before the sweeps begin.
1. **Exhaustive spec↔program diff, both directions** — every behavioral claim in main.md / README / `:help` verified against the code (claim inventory: `docs/audit/specdiff/CLAIMS.md`), plus a reverse sweep for shipped behavior the docs don't describe. Spec-vs-code disagreements are triaged per finding with the owner (recommended resolution: fix code vs fix doc); code fixes land repro-first, one commit each.
2. **UI/keymap consistency sweep** — a key×context matrix (every key/chord × NORMAL/DRILL/GRAB/SELECT/RESIZE × view/form/modal) reconciling actual behavior with the help bar, `:help`, and README; findings triaged like phase 1. Plus the two gap-closers: **agenda-board click-to-select** (single click selects, double-click edits the item under the cursor — closes the pass-16 accepted mouse gap) and the **Detail-pane accordion** (`+`/`-` collapses/restores Detail together with the overview column, making the Pane-sizing section's wording true).
3. **Deep audit — minimum one pass**, then best-effort: `/audit` over the never-audited v1.2.0–v1.4.0 surfaces (SELECT/bulk ops, recurrence-rewrite primitives + Repeat UI, quick-add grammar) and the sync-core reconcile matrix beyond the `CommitPush` window; further passes toward the two-consecutive-no-HIGH streak as time allows. The hardening cadence continues post-release as v1.5.x patches regardless.

**Status (2026-07-26)**

- **Release readiness**: all three scope phases are complete and the TZID arc's own two regressions are
  fixed (below), so nothing scoped for v1.5.0 remains open. The two `never`-state coverage rows are both
  permanently-accepted gaps (full `sync-collection` delta sync, deferred by design; Raspberry Pi hardware,
  unauditable headlessly). What remains is the owner's: review, merge to `main`, tag `v1.5.0`. Everything
  still on the coverage board is discretionary hardening, continuing post-release as v1.5.x patches.
- **Step 0 and both gap-closers — shipped.** Subagent-driven build off the plan `docs/superpowers/plans/2026-07-24-v1.5.0-step0-and-gap-closers.md`; whole-branch review clean.
- **Phase 1 — complete.** 962 claims verified against the code (`docs/audit/specdiff/CLAIMS.md`, the living inventory); 21 adversarially-confirmed divergences owner-triaged and fixed (all fix-doc); ~20 reverse-sweep doc gaps documented. Plus an owner-approved four-fix polish bundle: the `sp` warning relay, stale-rect double-click guards on the agenda board and tree, `j`/`k` in the Conflicts list and the account picker, and Ctrl-W cancel restoring the accordion. Every code fix repro-first with a regression test.
- **Phase 2 — complete.** An exhaustive key×context matrix (`docs/audit/specdiff/MATRIX.md`) found 20 divergences in its first 529 verified cells — 2 UI-text bugs, bulk-grab axis alignment, undo-drill preservation, the `J`/`K` todo flash, `q`-close on the account/color pickers, a mode-adaptive hint bar, and ~10 README/`:help` corrections — plus a `j`/`k`→`motionArrow` refactor folding duplicate key-mapping onto the existing idiom; whole-branch review clean. The matrix was then extended with the `:` command surface (35 rows) and the mouse contract (29 rows), reaching 593 verified cells and 4 further divergences, all triaged and fixed.
- **Phase 3 — the required minimum one pass is done** (pass 19), and four more have followed. Passes 19–23 are recorded in the Hardening & audit ledger above. Pass 19's own whole-arc review returned `more_passes_recommended` (its severity trend ran up, not down), so hardening continues as v1.5.x passes post-release; this release-gate item is satisfied by the one completed, fully-resolved pass.
- **v1.5.x fix arc — TZID-anchored recurrence** (2026-07-26, plan `docs/superpowers/plans/2026-07-26-tzid-anchored-recurrence.md`). Closed pass 23's carried unverified product-bug lead — recovered by reading the killed agent's transcript rather than by re-auditing. It was **three linked recurrence-anchor defects, not one**: the untouched-Repeat-dropdown rule rewrite (the lead itself), a New York weekly-Tuesday event that actually fired every Monday, and DST drift on the resulting series — all sharing one root cause (`BY*` derived from the local anchor while `DTSTART`/`DUE` were serialized in UTC). Fixed by writing a recurring item's anchor as local-time-with-`TZID` plus a generated `VTIMEZONE`, gated narrowly so an edit that leaves the rule alone never re-anchors (commits `1bbd338`..`680f10a`). Full design and residuals: Timezones & recurrence.
- **v1.5.x fix arc — the TZID arc's own two regressions** (2026-07-26, commits `47dab30`, `7fca8a2`). Both were recorded as open at the previous session's close and both turned out wider than recorded. **HIGH**: a recurring item lost the day its zone's DST change starts on — rooted in rrule-go's day enumeration rather than our arithmetic, hitting **11 zones** (not 3), at every frequency, for any anchor time of day, on the calendar read path as well as the todo advance. Expansion now runs in wall-clock space and resolves back into the real zone, and DATE / zone-less DATE-TIME decoding is gap-safe for the same reason. **MED**: `store` and the UI disagreed about the local zone — sizing found **nine** divergent decode sites, not the one recorded, so one resolved zone is installed as the process's `time.Local` instead of threading a location through four packages, and `LocalZone()` now honours every `$TZ` form Go itself accepts (the three it rejected had been silently overriding an explicit setting). Full design and residuals: Timezones & recurrence.

### Future versions

With the planned feature line closing at v1.4.0 (v1.5.0 is polish & audit), no further versions are committed. Should feature work resume, it gets planned here first: talk the version through with an agent, write its scope as a new `### v1.x.0` subsection (feature versions minor-bump; hardening stays patch-level), and only then implement step by step. Deferred ideas, in no particular order:

- **Configurable keybindings** — a `[keys]` config section (the schema deliberately left room; see Configuration & credentials).
- **Persistent trash** — undo today is session-scoped; deferred unless it proves needed.
- **Conflict "keep both as separate items"** — a third resolution besides keep-local/keep-server; needs a new-UID clone.
- **Full-cell click mapping** in the calendar grids — a click anywhere in a day/hour cell, not just on a drawn event block, selects it.
- **Full `sync-collection` delta sync** — **indefinitely deferred** (see the Incremental sync decision for the measured rationale); revisit only if a real pain point appears.

---

## Settled Decisions

### Language & libraries

**Go** — best fit for the four driving requirements (lazygit-style TUI ecosystem, long-term stability via the Go 1 compatibility promise, workable CalDAV libraries, fast + trivial ARM cross-compilation). Rust was the runner-up; Python ruled out on robustness/speed despite having the most mature CalDAV library.

**tview** for the TUI — years of backwards-compatible stability (vs Bubble Tea's breaking v2 + module-path move to a vanity domain in July 2026), a widget set (Table/Grid/Flex/InputField/Pages) that maps naturally onto calendar and task views, and k9s as proof that the target UX (`:` command mode, single-key shortcuts, mouse, full-screen panes) works on it. gocui ruled out: effectively a lazygit-internal library now.

### Sync & conflicts

**Offline-first with a local cache**; the NextCloud CalDAV server is the source of truth for sync.

**Conflict resolution**: ETag-based detection (conditional writes) — the app **never silently overwrites** in either direction. On a true conflict (same item edited locally and remotely between syncs), keep both versions, mark the item conflicted, and show a UI indicator; the owner resolves at leisure (pick a winner or keep both as separate items). Sync never blocks waiting for resolution. "Newest wins" and "server wins" rejected as silent data-loss paths.

**Sync triggers**: manual `:sync` always available, plus the automatic triggers — background sync on startup (UI opens instantly from cache, refreshes when sync lands), periodic while open (default 15 min, configurable, 0 = off), debounced push a few seconds after local edits (other devices see changes fast; shrinks the conflict window), and a **best-effort push on quit** (`flushOnQuit`): as the app exits it pushes anything still pending so an edit made inside the debounce window — or while briefly offline — isn't stranded until the next launch. It runs after the TUI stops (prints a plain notice), is a no-op when offline or nothing is pending (quit stays instant), and is time-bounded (`defaultQuitFlushTimeout`) so a hung network can never trap the user; nothing is lost either way (unpushed edits persist in the cache and sync next launch). Gated by `store.HasPendingChanges`.

The two **timer-driven** triggers (the debounced push and the periodic tick) are **deferred while a create/edit form is open** — a sync landing then would replace the store pointer the open form captured, so its version-checked Save would read as stale and silently discard the user's typed input. The debounced fire re-arms (retries) while a modal is open, the periodic tick skips that interval, and `closeModal` re-arms the deferred push once the form closes so a still-pending edit syncs promptly. Manual `:sync` is unaffected (it is unreachable while a form holds focus). (Residual: a sync already *in flight* when the form opens can still land — the version-checked write then protects the data, the edit is just skipped.)

**Incremental sync**: each calendar's `getctag` CTag is compared to the last-synced value; an unchanged CTag with no local changes skips the full re-download, so a routine sync of an idle account is cheap (a Pi/large-calendar concern). The full `sync-collection` REPORT (per-resource delta by sync token; the sidecar already carries a `sync_token` field) is **indefinitely deferred** (owner decision 2026-07-21). The 2026-07-21 estimate that settled it: a worst-case decade-scale calendar (~10,000 items at ~850 B each) costs roughly **0.5 s of background client CPU on a Pi 5 and ~12–15 MB of transfer** per full re-download (measured: 7.5 µs/event go-ical decode, 7.6 ms reconcile+store at n=10k on the dev machine, derated ~3× for the Pi; sync never blocks the UI) — paid on the sync after each local edit (the stale-CTag rule above) and on remote changes. That is acceptable, so delta sync's second server-trust surface (truncated reports, token expiry — it would need its own hardening pass) isn't worth buying now. Revisit if a real pain point appears: a metered/slow link, a slow server generating full REPORTs, or calendars growing toward the **~40k-item ceiling** where a full download approaches the 64 MiB response cap.

**Read-only calendars**: some server calendars grant no write privilege — notably NextCloud's generated **Contact Birthdays** calendar (`contact_birthdays`), and read-only shares/subscriptions. LazyPlanner **detects** these via a `current-user-privilege-set` PROPFIND during discovery (a calendar lacking `write`/`write-content`/`bind`/`all` is read-only; issued over `internal/caldav`'s own HTTP client since go-webdav's client doesn't expose privileges), caches the flag in the sidecar, and **never writes to them**: the UI blocks create/edit/complete/delete/re-parent (marking the calendar `[ro]`), and sync treats them **pull-only** — mirroring the server one-way and discarding any local change that can't be pushed (matching how the NextCloud web UI itself forbids edits there). A write refused with HTTP **403** is a reactive fallback that flags the calendar read-only even if privilege discovery missed it. This keeps LazyPlanner a well-behaved CalDAV citizen (no futile writes, no silent data loss from the earlier "push → server drops → reconcile deletes locally" cycle).

### Configuration & credentials

**Config file**: TOML (via `BurntSushi/toml`), **moderate scope** — one or more account connections (each a `[[account]]` block with a unique `name` + `url`/`username`/`password`/`password_command`) plus appearance and behavior options (first day of week, default view, date/time formats, color mode, sync interval). One account is active at a time; the earlier single `[server]` section was replaced by `[[account]]` blocks and is rejected with a migration message (see the Account model below). Per-calendar *local* preferences: hiding a calendar from the views is remembered in the state file (toggled with `Space`), not the config. A local *color* override was considered and dropped — calendar colors are server-owned and edited in-app via `:calendar color` (which syncs everywhere), so a display-only local override would fight that model. Keybindings hardcoded for now but the schema is structured so a `[keys]` section can be added later without breaking existing configs. TOML chosen over INI (no standard spec), YAML (footgunny spec, heavy dependency), and JSON (no comments, hand-edit hostile).

**Defaults match the owner's workflow** — every moderate-scope option remains fully configurable in the config file, but the *default* each option takes when unset is the owner's preference, so a working config needs nothing beyond one `[[account]]` block: week starts **Monday**, **12-hour** times (2:30pm), **month view** on open, **US dates** (07/04/2026), sync all calendars with server names/colors. The first-run generated config lists every option set to its default (commented), ready to change. The one unavoidable edit is filling in an account's connection.

**Config editing model**: the config file is hand-edited; the app reads it at startup and **never writes it**. Two conveniences: (1) on first run with no config, generate a fully-commented default `config.toml` documenting every option; (2) a `:config` command that opens the file in `$EDITOR` and reloads on exit (applying the active account's `color_mode` and credential/`password_command` changes live; an `auto`↔`truecolor` switch still needs a restart, and editing the active account's *connection* — or removing it — can't be hot-swapped onto the open cache, so it flashes "use `:account` or restart" rather than reloading). Switching to a *different* configured account is `:account`'s job, not `:config`'s. Auto-reload via file-watching was considered and **rejected** (extra dependency + mid-operation edge cases for marginal benefit). Anything the app must remember on its own (pane/Detail widths, hidden calendars, the week/day hour-row zoom) goes in a small state file under the data directory, never the config. The opening view is the configured `default_view` (not a remembered last-used view — the config knob is the single source).

**Credentials**: always a NextCloud **app password** (Settings → Security), never the real account password — revocable per-app. Stored in `config.toml`, which must be `0600` (the app warns on looser permissions). Escape hatch: an optional `password_command` whose stdout is the secret — with the owner's Vaultwarden server, that's `password_command = "bw get password lazyplanner"` (Vaultwarden speaks the Bitwarden API, so the standard `bw` CLI works). OS keyring rejected: daemon requirement breaks headless Pi, extra dependency, extra failure modes.

### Data model & storage

**Surfaced fields**: tasks show title, due date, status, **priority** (iCal 1–9), **tags** (CATEGORIES), **notes**, and **subtasks**; events show title, start/end, all-day flag, recurrence, **location**, **notes**, and a **reminder indicator** (shows that alarms exist; LazyPlanner does not fire notifications itself — phone/NextCloud handle that). Everything else in the `.ics` round-trips untouched.

**Subtask hierarchy**: arbitrary-depth nesting via `RELATED-TO` (RELTYPE=PARENT) — the same mechanism NextCloud Tasks uses, so existing nested tasks import as-is. The owner's most-used feature: the UI treats the task tree like a file explorer (collapsible nodes, indent/outdent, drill-in), and "folders" are just tasks with children — no new storage concept needed.

**Property preservation (iron rule)**: LazyPlanner never drops or mangles iCal properties it doesn't understand (X- properties, VALARMs, other clients' metadata). Editing a known field preserves everything else byte-for-byte-equivalent. This is what keeps LazyPlanner a well-behaved CalDAV citizen.

**Local cache storage**: vdir-style raw `.ics` files (one file per event/todo, one directory per calendar — the vdirsyncer/khal convention), with a JSON sidecar for sync state (ETags, sync tokens) and an in-memory index built at startup. The `.ics` files are the **local source of truth** — there is never a second store that can drift from them. Chosen for the 1:1 mapping onto CalDAV resources (simplest possible sync logic), zero extra dependencies, and human-readable/debuggable storage. SQLite rejected (cgo vs huge pure-Go dep; query speed unneeded at personal-calendar scale); custom JSON rejected (lossy translation away from the data's native iCalendar format).

**A damaged sidecar is salvaged, never discarded.** The sidecar is derived data, but throwing it away is not free — its ETags, hrefs, dirty flags and tombstones are what stop the next sync clobbering unsynced edits and resurrecting deleted items. So a sidecar that fails to parse is recovered **field by field**: one bad value costs one field, not the calendar's whole sync state. Whatever cannot be recovered is treated as **unknown, never as empty** — a partially-recovered resource loads *dirty* (it may hold an unsynced edit), while a fully-recovered one keeps its exact recorded state so a healthy cache is never force-re-pushed. The original bytes are copied to `<calendar>/.lazyplanner.json.corrupt` before anything overwrites them, and the parse failure is surfaced through the status bar's `!N load error(s)` badge. An unreadable sidecar does **not** make its calendar read-only — only the server's recorded privilege does that; locking it would block editing offline, which is precisely when the user is most likely to be working from the cache.

**The conflict stash is byte-lossless.** When sync keeps both versions, the server's raw iCalendar is stored in the sidecar **base64-encoded** (`server_data_b64`), not as a JSON string — a JSON string rewrites non-UTF-8 bytes to U+FFFD, so the "server version" the user is offered would not be what the server sent. Sidecars written before this carry a plain `server_data` field, which is still read (and re-emitted as base64 on the next write) so an in-flight conflict survives the upgrade.

**Account model (multiple accounts, one active at a time, server-keyed caches)**: LazyPlanner supports **several configured accounts** (one `[[account]]` block each) with **one active at a time** — there is **no merged multi-account view** (calendars from different accounts are never shown together). The active account is switched in-app with **`:account`** (`:account <name>`, or bare `:account` for a picker); switching **tears the app down and rebuilds it** on the new account (flush pending pushes, cancel in-flight sync, reopen the new cache), which is why no state can leak across accounts. A live hot-swap of the store was rejected: every captured pointer (timers, undo, open modals) would become a cross-account leak risk, and the rebuild's cost is imperceptible.

Each account's cache is namespaced by a stable `<account-id>` derived from the server URL + username (`<dataDir>/<account-id>/calendars/…`), so two accounts' data can never bleed into one directory. This matters because the sidecar's ETags and hrefs are meaningful only against the server that issued them — mixing two accounts in one cache would corrupt two-way sync's conflict detection and risk data loss. The **last-active account** (by id) is remembered in a small `global.json` at the data-dir root and reopened next launch; a stored id that no longer matches any configured account falls back to the first block, so a removed or renamed account can't strand the user. If a switch target fails to open, the app falls back to the previously-working account rather than exiting.

The old **single `[server]`** section is gone: a config still carrying it is rejected at load with a migration message, and it must be rewritten as a named `[[account]]` (the cache id is unchanged, so the existing cache is reused). Zero configured accounts is a valid fully-offline run over the cache.

### Calendar metadata

**Calendar metadata is server-owned**: calendar identity, display name, and color are CalDAV properties on the NextCloud server, cached locally in the vdir (sidecar convention) — they are data, not config.

- **Both ways on sync.** Renaming or recoloring in-app updates the server (propagating to NextCloud web and other clients), and both the display name and the color are **pulled** on sync, so a change made elsewhere shows up locally. The server is authoritative except when a local edit is still pending a push — that edit wins until pushed, never silently clobbered.
- **Color needs its own request**: a Depth-1 `calendar-color` PROPFIND, since go-webdav's `FindCalendars` doesn't surface it. It renders as exact truecolor by default, or the nearest terminal-palette color under `color_mode = "16"` (see the Colors design note).
- **Create and delete go over our own client.** Creating a calendar issues CalDAV **MKCALENDAR**, deleting one issues **DELETE**; go-webdav's client exposes neither, so LazyPlanner sends them over its own authenticated HTTP client in `internal/caldav`. Verified against NextCloud — creating in NextCloud web remains a fallback but is not needed.
- **Default with no calendar config sections**: sync all calendars using server names and colors.

### Timezones & recurrence

**Timezones**: preserve whatever the server sends untouched (iron rule). A **non-recurring** timed value LazyPlanner creates or edits is still written in **UTC (Z form)** — entered as local wall-clock time and serialized to the equivalent UTC instant. All-day items stay date-only with no timezone math.

**One display zone, everywhere.** Every value is displayed in the app's own zone (`a.loc`, from `config.LocalZone()`), which the forms also parse and write in. Every render path reads `a.loc` — the month grid, the agenda lines and the time grid each carry a `loc` field refreshed on every redraw — so an item entered at 20:00 always renders at 20:00. A render site left on `time.Local` shows the wrong hour on exactly the hosts where the two would disagree.

**One zone for the whole process.** `run()` installs `config.LocalZone()` as the process's `time.Local` before any dispatch (`installProcessZone`, `cmd/lazyplanner/main.go`), so the display zone and the decode zone are the same object by construction.

- **Why it is needed**: nine sites across `internal/store`, `internal/sync` and `internal/model` resolve floating and date-only values against `time.Local`, while the UI renders against `config.LocalZone()`. When the two differed, an all-day event could render on two days, a floating time land an offset out, and "today" resolve to the wrong day.
- **Why not thread a location through instead**: `Store.Open`, the sync engine, import and the model's object builders would all need one, and nine call sites are nine chances to drift apart again. One zone cannot disagree with itself.
- **`LocalZone()` mirrors Go's own `$TZ` resolution**, not just `time.LoadLocation`: an empty-but-set `TZ` means UTC, a leading colon is stripped (`TZ=:Asia/Tokyo`), and an absolute zoneinfo path is reduced to its IANA name. Each of those forms previously fell through to `/etc/timezone` and silently overrode an explicit user setting. A value naming no loadable zone still falls through, because an unnameable zone cannot be written as a TZID.
- It runs in `run()` rather than `main()` because every subcommand (`import`, `sync`, `calendar`) decodes cached or server data too, not just the TUI.

**A wall clock is not an instant.** A recurrence rule generates *wall clocks* (RFC 5545 §3.8.5.3 evaluates the set in the anchor's local time); zone resolution is the last step, not the first. rrule-go conflates the two — it derives each occurrence's date with `firstyday.AddDate(0, 0, i)` on local Jan-1 **midnight** — so in a zone whose DST transition lands exactly on 00:00, that midnight does not exist, `AddDate` normalizes backwards into the previous day, and the generated instant duplicates the previous occurrence and is dropped as a duplicate. The day vanished from the series entirely, for **11 zones** (Havana, Santiago, Coyhaique, Punta_Arenas, Palmer, Azores, Scoresbysund, Sao_Paulo, Asuncion, Campo_Grande, Cuiaba), at every frequency and for any anchor time of day.

- **Expansion therefore runs in wall-clock space** (`internal/model/wallclock.go`): the anchor, RDATEs, EXDATEs and the RRULE's `UNTIL` bound are all restamped into UTC — a zone with no gaps — and each generated wall clock is resolved back into the real zone as `safeBetween`/`safeAfter` yield it. Keeping the rule's own comparisons in the same space as the instants it generates is what makes the translation total rather than piecemeal.
- **`resolveWallClock` changes as little as possible**: it reproduces `time.Date`'s reading, backwards normalization included, whenever that keeps the occurrence on its own calendar day, and snaps forward to the day's first existing instant *only* when normalization would move it to a different day. A DST gap may shift an occurrence's time, as it always has; it must never move it to another day. Verified across all 485 IANA zones: the recovered gap days are the only day-level difference and there are no instant-level differences elsewhere.
- **Decoding is gap-safe too**, or the fix would be invisible: a DATE value names a day, and reading `DUE;VALUE=DATE:20260308` in Havana normalized it to 03-07, so the app wrote the right day and immediately misread it. `resolveDateTime` resolves DATE and zone-less DATE-TIME values through the same helper, taking the zone from whatever go-ical already resolved so no zone-selection logic is duplicated.
- A `COUNT=10` series crossing a gap day now yields 10 occurrences rather than 9 — the dropped duplicate had consumed one.
- Eight further zones (Cairo, Beirut, Amman, Damascus, Gaza, Hebron, Tehran, Casey) have a nonexistent local midnight that normalizes *forward* to 01:00 of the same day; they never lost a day and are deliberately left alone.

**Recurring-item anchors are written differently.** A recurring item's anchor (`DTSTART`, or `DUE` for a VTODO) is written as **local time with a `TZID`**, with a matching generated `VTIMEZONE` component in the object. This exists because RFC 5545 evaluates a rule's `BY*` parts in the anchor's own zone: a UTC anchor derived from a local weekday disagrees with that weekday whenever the local and UTC dates differ, making the rule fire on the wrong day and drift an hour across DST. Three defects shared this one root cause, all reproduced end-to-end before the fix:

- Opening the edit form on a recurring item and pressing Save **without touching the Repeat dropdown** silently rewrote the rule (`BYDAY=MO`→`BYDAY=TU`), shifting the series a day and dropping orphaned per-occurrence overrides — in both the event and todo forms.
- A New York user creating a Tue 20:00 weekly meeting via the dropdown labelled "Weekly on Tue" got a series firing every **Monday**, whose first occurrence was not the event's own start.
- That series drifted from 20:00 EDT to 19:00 EST after the November DST transition.

**The re-anchor gate is deliberately narrow.** The TZID form is written only when (a) the anchor already carries a resolvable TZID — keep writing in that zone, or (b) the same call is authoring the rule (creating or rewriting it) and the draft's own zone is nameable. An edit that leaves the rule alone never re-anchors: re-interpreting `BY*` against a different zone would silently move an existing series — a correct legacy Berlin event must not start firing Mondays because someone renamed it.

**Every writer that emits a TZID also defines it.** An undefined TZID is rejected by Sabre/NextCloud validation and read as floating by Apple/Thunderbird — the series lands on the wrong hour in every other client — so `ensureVTimezone` runs on all of `editComponent`, `NewEventObject`, `NewTodoObject`, `AddException`, `AddOccurrenceOverride`, `RewriteEventRule`, `NewSeriesFrom` and `DetachTodoOccurrence`. The last three are the fresh-calendar / primary-rule-change shapes; they shipped without it once. It scans `DTSTART`/`DTEND`/`DUE` **and** `EXDATE`/`RDATE`/`RECURRENCE-ID`, per value, because a recurrence date is written in the anchor's *resolved IANA* zone while the anchor itself may keep a Windows spelling. It is additive only: an existing VTIMEZONE, ours or a server's, is never replaced.

**A generated observance's rule must generate its own DTSTART.** `BuildVTimezone` renders an observance `DTSTART` as the transition's wall clock in the offset being switched *from* (RFC 5545 §3.6.5), so its `BYMONTH`/`BYDAY` are derived from that same rendering. Reading them from the zone-local (TO-offset) rendering instead puts them on a different calendar day for a transition near local midnight — `Africa/Cairo`, `Asia/Beirut`, `America/Santiago` — leaving the rule contradicting its own anchor, the project's codified `DTSTART`-vs-`BY*` invariant reappearing inside the generator. The observance's onset is also never dated after the anchor: a transition-less zone uses the conventional far-past `19700101T000000` unless the anchor itself predates 1970, in which case the onset falls back to the start of the anchor's year.

**Known residuals**:

- An item already stored with a UTC anchor keeps its existing (possibly wrong-day) recurrence behavior until its rule is next authored or rewritten — a blanket migration would have to re-derive every `BY*` and was judged riskier than the defect.
- A Windows host without `$TZ` set cannot resolve an IANA zone name (`LocalZone()` falls back to `/etc/timezone`/`/etc/localtime`, both Linux-only), so it keeps writing UTC anchors.
- An absolute `$TZ` path outside every known zoneinfo root (`TZ=/opt/custom/zone`) is not honoured. Go would load it, but it yields no IANA name and an unnameable zone cannot be written as a TZID, so resolution falls through to the system files — the process stays internally consistent, just not on that exotic zone.
- **Day *bucketing* is still not gap-safe.** `model.DayStart` and quick-add's relative dates build local midnight with `time.Date`, so in the 11 gap zones a day window is shifted an hour at its edges and "tomorrow" can resolve to today. Pre-existing and independent of recurrence expansion; fixing it means reworking day-window semantics (`DayStart` + `AddDate(0,0,1)`) across the calendar views, which was judged too wide for a pre-release fix.
- `resolveWallClock` falls back to `time.Date`'s reading when it cannot bracket a single forward jump around the wall clock (a zone whose offset moved more than once within a day of it) — no worse than not having the path, but not resolved either.
- Editing an Outlook-authored recurring series now rewrites its Windows TZID (e.g. `Eastern Standard Time`) to the resolved IANA spelling (`America/New_York`) — the instant and wall clock are unchanged, but the TZID text on the wire changes.
- `zoneInfoRoots` (`internal/config/zone.go`) is a package-level var slice; both reviewers judged the immutable-lookup-table use conventional despite CLAUDE.md's mutable-package-var guardrail, and left it as-is.
- `BuildVTimezone`: negative-DST zones (e.g. `Europe/Dublin`) invert the generated STANDARD/DAYLIGHT observance labels (offsets stay correct); the transition-bisection's minute truncation is unguarded (empirically safe across the 486 zones tested).
- The compact one-observance-per-offset VTIMEZONE cannot express a zone whose real rule is not a single yearly rule — DST abolished mid-window (`America/Vancouver`, `America/Asuncion`) or lunar-political (`Africa/Casablanca`, `Asia/Gaza`, `Asia/Jerusalem`) — so roughly a third of sampled zones diverge on **forward projection**. That is inherent to the shape NextCloud, Google and Apple all emit; what is guaranteed instead is self-consistency (the emitted rule generates the emitted `DTSTART`).
- `icalNthWeekday` resolves the last-week ordinal ambiguity from earlier years of the same observance, but when *every* sampled year is both the nth and the last, `-1` is kept — the reading every real zone rule in that shape means.
- `SetTodoCompleted`/`SetTodoParent`/`CopyTodo` — ordinary, UI-reachable actions — now add a VTIMEZONE to a foreign object on these flip-one-field paths; iron-rule-safe, but a contract change, costing ~69µs/object in bulk loops.
- `ensureVTimezone` resolves a TZID with `time.LoadLocation`, so a foreign anchor keeping a Windows spelling (`TZID=Eastern Standard Time`) still gets no definition of its own. Defining it would mean emitting a VTIMEZONE under a non-IANA label; the app's own writes always normalize to the IANA spelling, so only untouched foreign props are affected.

**Robustness**: the IANA tz database is embedded in the binary (`import _ "time/tzdata"`) so zones resolve on any OS (minimal Pi image, Windows). A TZID that Go can't load — an Outlook/Windows zone name (e.g. `Eastern Standard Time`) or a custom `VTIMEZONE` label — is mapped via the CLDR windowsZones table, and if still unresolved the value is kept as floating/local time rather than dropping the item. LazyPlanner never silently loses an event/todo over an unfamiliar timezone.

**Recurrence editing (events)**: all three scopes — "only this occurrence" (RECURRENCE-ID override), "this and future" (series split, with a bounded `COUNT` preserved across the split so the total occurrence count is unchanged), "all occurrences" (edit master) — so LazyPlanner never forces a reach for another client.

**Recurring todos — single live instance**: a recurring task shows **one** live instance at its current due date; completing it advances the series one occurrence (NextCloud-style), and editing "this occurrence" detaches it as a standalone task and advances the rest. Expanding every occurrence onto the calendar was **tried and reverted** as unneeded complexity — and a recurring task-with-subtasks is *not* a supported "checklist" concept (the independent handling is data-safe but does not regenerate subtasks per occurrence); don't reintroduce occurrence-expansion for todos.

## Open Decisions

**None — the spec is code-ready.** All major decisions are settled (see Settled Decisions).
