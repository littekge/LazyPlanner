# LazyPlanner — Change Log

> Append a new entry every time a change is made. Newest entries at the top.

---

## 2026-07-05 — UI polish pass (2/3): create task vs subtask, folders, sticky-complete

- **Create keys** (`edit.go`, `app.go`): split creation into distinct actions — `a` quick-add top-level task (or event in calendar/agenda), `s` quick-add subtask under the highlighted task, `A`/`S` the same via the full form. Refactored the forms into reusable builders (`newTodoForm`/`newEventForm`) + readers (`readTodoDraft`/`readEventDraft`) shared by edit and create; `commitMutation` is the shared write/undo/refresh tail.
- **Folders**: a task with ≥1 incomplete child renders `▸`/`▾` (in place of `[ ]`/`[x]`), the marker flips on expand/collapse; folders can't be completed until their children are (guarded in `toggleComplete`), and revert to a normal task when empty/all-done (`folderSet` recomputed each build). Deleting a task now takes its whole subtree — `descendants` gathers them, the confirm counts them ("Delete X and its N subtask(s)?"), and undo restores them all.
- **Undo** generalized to compound steps (`undoStep.ops []undoOp`) so a recursive delete undoes in one `u`; `pushUndo` helper; all sites migrated.
- **Sticky-complete**: checking off a task while completed are hidden pins it visible (`stickyDone`) until the list is left (switching list or pane), not on a popup/refresh. Fixed a subtle bug where the panel-rebuild's transient empty selection cleared the pins.
- Tests (`edit_test.go`): folder blocks completion until children done then completes; sticky keeps a completed task visible then hides it after leaving the list; `descendants` depth. All pass incl. `-race`.

## 2026-07-05 — UI polish pass (1/3): status bar, cosmetics, tz + Space bugs

- First of a multi-part UI refinement batch (owner feedback). Spec/plan updates + localized fixes + chrome; the behavioral pieces (create-task-vs-subtask, folders, agenda outline widget, week/day drill-in, modal focus restore) land in follow-up commits.
- **Spec/plan** (`main.md`, `CLAUDE.md`): deferred to their proper steps with documentation — in-app **calendar/list creation → step 9** (server MKCALENDAR), **command view + `:` line + full vim-style chorded keymap → step 10**, **sync-status indicator → step 9/12**. Documented (for build now): two-line status bar, create-task vs create-subtask, quick-add/full-form toggle via distinct keys, folder semantics, rounded borders, B/W dialogs, agenda outline box, week/day drill-in, "keep completed visible until leaving list", UTC-store/local-display.
- **Status bar** (`app.go`, `render.go`): the bottom is now two lines — a 3-section bar (left general/results, middle command-view stub → step 10, right sync stub → step 9) above an always-visible controls line. `flash()` writes the left section; it persists until the next `updateStatus`.
- **Cosmetics**: rounded (soft) corners on all borders (`tview.Borders` + custom `drawBox`); monochrome confirm modal and edit forms (white card, black labels, black input boxes). Note: tview applies one field style to every form field per frame, so per-field "white when focused" isn't possible without a custom form; the black boxes on a white card read clearly.
- **Bugs**: timed values are stored UTC but were rendered without converting to local (a created `12:00am` showed as 4am on a UTC-4 box) — all event/occurrence render sites now convert to local. `Space` no longer flashes "select a task" in views where nothing is toggleable (silent no-op). Edit-form fields use `fieldWidth 0` so they fit the dialog instead of overflowing. Shortened the controls line so it doesn't truncate.
- Tests: existing model/store/ui suites pass; pty check confirms rounded corners render, the two-line status bar + sync stub show, and the B/W confirm opens.

## 2026-07-05 — Build step 8: editing (create/edit/complete/delete + undo + re-parent)

- Editing from the UI; writes go to the local vdir only (server push is step 9). Scope confirmed with owner: core create/edit/complete/delete plus session undo (`u`) and indent/outdent re-parent (`H`/`L`).
- **model** (`internal/model/edit.go`, `quickadd.go`): field-mutation + construction helpers honoring the property-preservation iron rule — `EditTodo`/`EditEvent`/`SetTodoCompleted`/`SetTodoParent` clone via encode→decode and mutate the raw component (unknown props, VALARMs survive); `NewTodoObject`/`NewEventObject` build fresh objects (`NewUID`, VERSION/PRODID, DTSTAMP). Timed values stored UTC (Z), all-day date-only. Int props (PRIORITY/PERCENT-COMPLETE/SEQUENCE) written without a VALUE=TEXT tag so they round-trip. Quick-add parser: conservative/documented tokens (dates, times requiring am/pm-or-colon, `!priority`, `#tag`); ambiguity stays in the title; `QuickAdd.At` combines the parsed date/time onto a context day.
- **store** (`internal/store/mutate.go`): `Locate(uid)` finds the resource holding an event/todo; `Restore` writes a prior snapshot back exactly (ETag/Href/Dirty) — the undo primitive. (`Put`/`Delete` already existed from step 4.)
- **ui** (`internal/ui/edit.go`, `app.go`): keys `a` quick-add (contextual), `e` full form (tview.Form modal), `Space` complete-toggle, `d` delete-with-confirm, `u` multi-level session undo (memento of the pre-change snapshot; create→delete, else Restore), `H`/`L` re-parent via RELATED-TO. Top-level `Pages` root hosts centered modal overlays; `globalKeys` yields all keys while a modal is open. New events target the highlighted calendar, new tasks the selected list; a new task nests under the selected tree node. `refresh` rebuilds panels preserving selection and reselects the touched item by UID.
- Tests: model — iron-rule preservation, clone independence, completion round-trip, re-parent preserving other relations, quick-add table. store — Locate, Restore-undoes-edit. ui — create+undo, complete-toggle+undo, indent+undo (headless app harness over a temp copy of the fixture). Full gate + `-race` pass.
- Verified end-to-end via pty against a seeded writable cache: quick-add task modal wrote a file with DUE/PRIORITY:1/NEEDS-ACTION, edit form opened and cancelled, quick-add event wrote DTSTART; exit 0, no panic.

## 2026-07-05 — UI: legible agenda selection + task-tree rooted at list name

- Two owner-noticed polish items.
- **Agenda highlight legibility**: the selected agenda block's title was illegible under tview's region highlight. Root cause: tview derives highlight contrast from a color's *nominal* RGB, but the terminal's 16-color palette remaps those colors, so e.g. a green title became low-contrast under the auto-picked highlight. Fix: stop using tview's region highlight for the agenda; render the selected block ourselves as an explicit **black-on-white** bar (`agendaItemBlock(it, plain)` emits no color tags for the selected block so the uniform wrap wins), and scroll it into view manually (`scrollAgendaTo` — keeps the block visible like a list cursor instead of jumping to top). Non-selected blocks keep their green/aqua colors.
- **Task tree root**: top-level tasks previously dangled from an empty, invisible root node (stems connecting to nothing). The tree root now shows the **list's own name** (teal), so the top-level tasks attach to it like a file tree rooted at its directory.
- Refactor: extracted `newApp(store, title, now)` from `Run` so the UI can be built + loaded headlessly with a fixed clock for tests.
- Files: `internal/ui/{render.go (renderAgenda/scrollAgendaTo/currentAgendaIndex, agendaItemBlock plain mode, buildTree root name), app.go (newApp, drop SetRegions + syncAgendaHighlight, SetChangedFunc→renderAgenda)}`; spec note in `main.md` (task tree rooted at list name).
- Tests: new `internal/ui/app_test.go` — `TestAgendaSelectedBlockLegible` asserts the selected title renders `fg=black,bg=white` on a `SimulationScreen` (guards the contrast fix), `TestTaskTreeRootIsListName` asserts the root text equals the list's display name with children attached. Full gate + `-race` pass; pty smoke test (agenda up/down, tasks) exits 0, no panic.

## 2026-07-05 — UI: focus lives in the overview (calendar + agenda)

- Owner-requested tweak before step 8 (spec updated in `main.md` UI Design + `CLAUDE.md` + `README.md`). Previously `1` and `3` jumped focus straight into the center pane; now all three modes focus their **left overview panel** first (matching how Tasks already worked), so the highlight lives in the overview.
- **Calendars (`1`)** → focus the left Calendars list; arrows highlight each calendar (per-calendar visibility toggles land in step 10). `Enter` dives into the grid (arrows→days, `Enter`→cycle the day's events, `Esc`→back to the list). Added `[` / `]` to cycle the highlighted calendar from anywhere in calendar mode — including while diving in the grid (fast-nav, owner's request). `v`/`n`/`p`/`t` no longer yank focus back to the grid: new `refocusCalendar` keeps focus on the list unless the grid itself was focused (then it follows the swapped month/time primitive).
- **Agenda (`3`)** → focus the left Agenda list; moving its highlight highlights the matching block in the center pane and auto-scrolls to it. Center agenda blocks are now wrapped in tview text regions (`["item-N"]`, `SetRegions(true)`), driven by `syncAgendaHighlight` via the list's `SetChangedFunc`. Detail pane stays hidden (full-width center) as before.
- `Enter`/`Esc` wiring: `calendarView` and `timeGridView` gained an `onExit` callback (Esc in day-mode / time-grid hands focus back to the Calendars list); the month grid's existing two-level Esc (event-mode → day-mode) still works, then a further Esc returns to the list.
- Files: `internal/ui/{app.go (focus model, refocusCalendar/gridFocused/cycleCalendar/syncAgendaHighlight, `[`/`]` keys, agendaCount), render.go (agenda region tags, agendaCount, status hints), calendarview.go (onExit + Esc), timegridview.go (onExit + Esc)}`
- Tests: existing `model` + UI `SimulationScreen` suites pass, incl. `-race`. Smoke-tested the compiled binary through a pty against seeded data (today's `Project meeting` + `Buy groceries` todo): drove `1`→cal nav→`[`/`]`→`Enter`/`Esc` dive→`v v t`→`3` agenda highlight→`2`→`Tab`→`q`; exits 0, no panic, expected labels render.

## 2026-07-04 — UI refinement: center-follows-focus, time-grid, highlight

- Owner-driven UI refinement before step 8 (spec updated in `main.md` UI Design + `CLAUDE.md`). Implements the spec's "Main pane follows focus" properly and adds the requested behaviors. All in one pass.
- **Center follows the active overview panel** (`1`/`2`/`3`), rebuilt around a `mode` + a center `Pages`:
  - **Calendars** → the calendar view (month grid / week·day time-grid). Left Calendars panel lists calendars.
  - **Tasks** → left Tasks panel now lists **only the task lists** (calendars with todos); selecting one opens that list's full subtask tree in the center (inline `[ ]`/`[x]`, `!priority`, `due`), and the Detail pane shows the highlighted task's full fields/description.
  - **Agenda** → center shows the day's events/tasks with **full descriptions at full width**; the Detail pane is **hidden** (`Flex.ResizeItem` to 0) and the view scrolls (PageUp/PageDown).
- **Week/Day = hourly time-grid** (`internal/ui/timegridview.go`, new custom primitive): hour axis, all-day band at top, events drawn as duration-sized blocks, overlapping events placed side-by-side. Overlap layout is a pure, tested `model.LayoutDay` (greedy lane assignment + per-cluster lane count). v1: one row/hour, `scrollHour` window (PgUp/PgDn/arrows scroll), simple overlap — proportional/refined overlap can follow.
- **Highlight fix**: the selected calendar day is now an **outline box** (`drawBox`), not a solid teal fill, so event text stays readable (fixes the "events invisible" complaint).
- **Day → event cycling** (point 5): in the month grid, `Enter` on a selected day enters "event mode" — up/down cycle that day's events and the Detail pane shows the highlighted event/task; `Esc`/left/right exits. (Time-grid event cursor deferred; blocks already show details.)
- Focus/navigation kept interim (finalized in step 10): `1`/`2`/`3` select mode, `Tab` cycles, `Enter`/`Esc` dive into/out of the tree and day-events.
- Files: `internal/ui/{app.go (rewritten: modes, center Pages, detail hide, focus/borders), render.go (rewritten: per-mode build + detail), calendarview.go (outline + event cursor), timegridview.go (new)}`; `internal/model/timegrid.go` (+ test)
- Tests: `model.LayoutDay` (non-overlap, 2-way, 3-way peak) + empty; UI `SimulationScreen` tests — month render, week render, month arrow-select, **time-grid day render** (headers/all-day/hour labels/event block), time-grid arrow. All pass, incl. `-race`
- Verified end-to-end via pty against seeded data: mode 1 shows the calendar + today's event + day detail; mode 2 the Work list's tree ("Write report" → "Draft section") with full task detail; mode 3 the full-width agenda (event location + task description), detail hidden; cycle/nav stress exits 0
- Verification: `gofmt`/`go build`/`go vet`/`staticcheck`/`go test ./...` (+`-race`) all pass
- Deferred/notes: proportional time-grid overlap columns and a time-grid event cursor; the calendar Calendars/Agenda left panels are informational for now (drive nothing) pending the step-10 navigation pass
- Issues: none

---

## 2026-07-04 — Calendar grid: custom spacious primitive (replaces the Table)

- Refinement to step 7's calendar (owner chose the "spacious grid" option). The `tview.Table`-based grid rendered content-width, single-line cells that didn't fill the pane; replaced it with a custom-drawn primitive that fills width and height and lists each day's events/tasks in the cell
- New `internal/ui/calendarview.go` — `calendarView` embeds `*tview.Box` and implements `Draw` + `InputHandler`:
  - Draws weekday headers, a header rule, vertical column separators, and one cell per day; each cell shows the day number then event/task lines (`3pm Title`, `[] Task`), with a `+N more` overflow note and a `N (count)` fallback when the cell is only one line tall
  - Today highlighted (bold), adjacent-month days dimmed, selected day background-filled (brighter when focused); colors from the 16-color palette
  - Arrow / `hjkl` move the selection by ±1 / ±7 days via an `onSelect` callback; the app re-anchors the grid only when the day leaves the visible range (period stays put while navigating within it)
  - `printStyled` helper draws background-aware, width-clipped text (tview's `Print` only sets a foreground color); uses `mattn/go-runewidth` (promoted to a direct dep — already vendored via tcell) for correct truncation
- `app.go`/`render.go`: swapped the Table for `calendarView`; `buildCalendar` now computes each visible day's agenda once (`calItems`) and calls `setData`; removed the Table-era `renderGrid`/`countsFor`/`dayCellLabel`/`onDaySelected`. Left column narrowed to 26 and the calendar given proportion 3 (was 2) so it has more room by default
- Files: `internal/ui/calendarview.go` (+ `calendarview_test.go`), `internal/ui/{app,render}.go`
- Tests added (**headless via tcell `SimulationScreen`** — first real UI unit tests): month render (weekday headers, a day number, an event title all present at 140 cols), week render, and arrow-key selection movement — all pass, including `-race`
- Verification: `gofmt`/`go build`/`go vet`/`staticcheck`/`go test ./...` (+`-race`) pass; `go mod verify` clean; pty smoke exits 0
- Note: at ~80 columns the cells are narrow and titles truncate to day numbers; the grid shows full detail on a wide terminal, and step-10 pane resizing (accordion/keyboard) will let the calendar take the whole screen
- Issues: none

---

## 2026-07-04 — Spec: interactive pane sizing added to step 10

- Owner requested interactive pane resizing; agreed to build it in **step 10** (keybinding polish). Recorded in the spec:
  - `main.md`: new **Pane sizing** subsection under UI Design — (A) **accordion expand** (`+`/`-`, lazygit idiom) collapses side panels/Detail so the Main view fills the screen; (B) **keyboard resize** (`Ctrl-←`/`Ctrl-→`) grows/shrinks left-column & Detail widths via `Flex.ResizeItem`, clamped. Sizes remembered in the state file (not config). Mouse drag-to-resize declared out of scope (keyboard-first). Two keymap rows added; Build Plan step 10 updated
  - `CLAUDE.md`: UI Project Context line notes pane sizing lands in step 10
- Feasibility confirmed in tview: `Flex.ResizeItem` (runtime resize), `Application.SetMouseCapture` + `Box.GetRect` (would enable mouse drag, but that's out of scope). No code yet — spec change only
- Also: terminal-resize reflow already works automatically (tview redraws the Flex tree on resize)

---

## 2026-07-04 — Build step 7: calendar views (month/week/day)

- **Build Plan step 7 complete.** Added the center "Main" pane with month/week/day calendar grids and movement keys, moving to the spec's four-region layout (left panels · calendar · detail · status).
- `model` additions (pure, tested): `MonthGrid(anchor, mondayFirst)` (6×7 days, padded with adjacent-month days, DST-safe midnight arithmetic), `Week`, `StartOfWeek`, `DayStart`, `SameDay`, `OccurrencesOn` (occurrences overlapping a day, multi-day aware)
- `internal/ui`:
  - Layout reworked: left column now stacks **Calendars** / **Tasks tree** / **Agenda**; center **Main** is a `Pages` holding the month/week grid (`tview.Table`) and the day view (`TextView`); **Detail** on the right; status bar
  - Month grid: weekday headers (Monday-first default per spec), one selectable cell per day showing the day number + `*N` event/due-task marker, today highlighted, adjacent-month days dimmed; arrow keys move between days and update Detail with that day's agenda
  - Week view = one-week grid; Day view = that day's agenda text
  - Keys: `v` cycles month→week→day, `n`/`p` prev/next period, `t` today (all global — they only affect the calendar); `1`/`2`/`3` focus left panels, `Tab` cycles all four regions including the calendar; focused pane border highlights
  - Event/due-task counts bucketed across the visible grid range (multi-day events mark every covered day; DTEND treated as exclusive; zero-length events counted on their start day)
- Files: `internal/model/{calendar.go,calendar_test.go}`; `internal/ui/{app.go,render.go}` reworked
- Tests added: `StartOfWeek`, `MonthGrid` (6×7, contiguity, correct padding, covers the month), `OccurrencesOn` (incl. a multi-day span) — all pass. UI is thin glue (no unit tests) but **pty-verified**: month grid renders with Monday-first headers + `*1` marker on today + today's agenda in Detail; week view ("Week of Jun 29, 2026") and day view ("Saturday, Jul 4 2026", "2:00pm Today Meeting") render; a cycle/navigate/tab stress run exits 0 (no crash)
- Verification: `gofmt`/`go build`/`go vet`/`staticcheck`/`go test ./...` all pass
- Note: the task tree moved from center (step 6) back to the left column to give the calendar the Main space; deep-tree ergonomics improve with `>` zoom in a later step. UI remains a v1 draft to refine against real screens
- Issues: none

---

## 2026-07-04 — Calendar creation (MKCALENDAR); resolves the go-webdav gap

- **Resolved the spec's flagged verification** ("verify go-webdav calendar creation"). Finding: go-webdav v0.7.0's CalDAV *client* has no calendar-creation method (only its server code handles MKCOL; `webdav.Client.Mkdir` sends a plain MKCOL = generic collection, not a calendar; the low-level request helpers are in go-webdav's unimportable `internal/` package). Owner opted to verify a fix before proceeding to step 7
- **Fix (no NextCloud web UI needed)**: `caldav.Client` now retains the authenticated HTTP client + parsed endpoint, so it can issue verbs go-webdav doesn't expose. Added:
  - `CreateCalendar(ctx, path, CalendarSpec)` — RFC 4791 **MKCALENDAR** with displayname, description, Apple `calendar-color`, and `supported-calendar-component-set` (VEVENT / VTODO / both). Generated XML eyeball-checked for correct namespaces; success = 201, errors surface the server's response body
  - `DeleteCalendar(ctx, path)` — DELETE on the collection (so calendars can be removed in-app too)
  - `CalendarHomeSet(ctx)` — extracted from DiscoverCalendars (principal → home set), reused by create
- **CLI**: new `lazyplanner calendar <list|create|delete>` subcommand (`create` flags: `--name`, `--tasks`, `--both`, `--color`, `--desc`, `--path`; slugifies the name into the collection path under the home set). `main.go` dispatch tidied (`exitOnError`); shared `connFlags` helper extracted and `import` refactored onto it
- Files: `internal/caldav/{client.go (endpoint/http fields, CalendarHomeSet),mkcalendar.go}`; `cmd/lazyplanner/{calendar.go,conn.go}`, `import.go`/`main.go` updated; tests `internal/caldav/mkcalendar_test.go`; README documents the new commands
- Tests added: `CreateCalendar` (method=MKCALENDAR, path, body contains displayname/color/comp set), default-components (VEVENT+VTODO), error surfacing (non-201 includes server hint), `DeleteCalendar` (method=DELETE) — all pass via httptest
- Verification: `gofmt`/`go build`/`go vet`/`staticcheck`/`go test ./...` all pass. **Real-server MKCALENDAR acceptance to be confirmed by the owner** against their NextCloud (`lazyplanner calendar create`) before relying on it
- Memory: recorded the decision + plan ([[calendar-creation-mkcalendar]])

---

## 2026-07-04 — Build step 6: read-only UI shell

- **Build Plan step 6 complete.** First real tview UI: a read-only shell over the imported cache, showing a subtask tree and a day agenda. `lazyplanner` (no args) now opens it.
- **Decomposition**: the testable logic lives in `model` (pure, unit-tested); `internal/ui` is thin tview glue verified by launch. Only `ui` imports tview/tcell (architecture rule holds)
- `model` additions (tested):
  - `BuildTree(todos, includeCompleted)` → `[]*TodoNode` — assembles the subtask forest from `ParentUID`, smart-sorts siblings, hides completed by default (their incomplete descendants surface as roots), and **breaks cycles** in malformed data (guarded against infinite recursion)
  - `SortTodos` — smart sort: due (soonest, undated last) → priority (1 highest, 0/undefined last) → title
  - `DayAgenda(occs, todos, dayStart, dayEnd)` → `[]AgendaItem` — merges event occurrences with todos due that day, all-day first then by time
- `internal/ui` (`app.go` + `render.go`): three-pane read-only shell —
  - **Left column**: Calendars list + Agenda list; **center**: the Tasks tree (centerpiece, given the width) with each calendar as a top-level folder; **right**: Detail pane; **bottom**: status bar with key hints + live counts + load-error indicator
  - Focus with `1`/`2`/`3` and `Tab`/`Shift-Tab` (focused pane border turns yellow); Detail tracks the focused pane's selection; `Enter`/`Space` expand/collapse tasks; `.` toggles completed; `q`/`Ctrl-C` quit; mouse enabled
  - Colors use the terminal 16-color palette (per spec); labels use ASCII markers (`[ ]`/`[x]`) to render on a bare TTY
- Wiring: `ui.Run` now takes `*store.Store`; `cmd/lazyplanner` `runTUI` opens the cache at `config.DataDir()` and hands it to the UI (UI reads only through the store)
- Files: `internal/model/{tree,agenda}.go` + tests; `internal/ui/{app,render}.go` (replaced the placeholder); `cmd/lazyplanner/main.go` updated; README Usage documents the TUI + current keymap
- Tests added: `BuildTree` (hierarchy, hide/show completed, cycle-breaking), `SortTodos`, `DayAgenda` — all pass. UI is thin glue (no unit tests) but **launch-verified** via pty: renders panes + calendar/tree/agenda from a populated cache and quits 0; empty cache shows the welcome/"nothing today" and quits 0; no-TTY still errors gracefully (exit 1, no panic)
- Verification: `gofmt`/`go build`/`go vet`/`staticcheck`/`go test ./...` all pass
- Scope note: the big "Main" pane + month/week/day calendar grids are step 7; this step is the shell + task tree + agenda
- Issues: none

---

## 2026-07-04 — Build step 5: CalDAV one-way import

- **Build Plan step 5 complete.** LazyPlanner can now connect to NextCloud, discover calendars, and download everything into the local vdir — a one-way pull, done before the UI so the model is validated against real server data early. First code that talks to a real server.
- Added and vendored `github.com/emersion/go-webdav` v0.7.0 (go-vcard correctly pruned as unused; MVS keeps the newer go-ical across the module)
- `internal/caldav` — the only package that speaks HTTP. `Client` wraps go-webdav:
  - `NewClient(Config)` — basic-auth (app password) over `webdav.HTTPClientWithBasicAuth`; injectable `*http.Client` for tests; default 30s timeout
  - `DiscoverCalendars(ctx)` — walks current-user-principal → calendar-home-set → calendars
  - `DownloadAll(ctx, path)` — one calendar-query REPORT returning full data + ETags for every resource
  - Types `Calendar` and `Object{Path, ETag, Data *ical.Calendar}`; go-ical stays confined to model/caldav
- `internal/sync` — seeded with the orchestration layer (imports caldav + store + model, the higher tier):
  - `Import(ctx, Source, *store.Store)` — discovers calendars, sets metadata, downloads and upserts every resource as clean/synced. **Pull-only** (no local-change push, no deletion of server-absent locals — that's the two-way sync step). Unparseable/unwritable resources are **skipped and collected**; only discovery/listing failures abort
  - `Source` interface (satisfied by `*caldav.Client`) makes the import unit-testable with a fake
- `internal/store` additions: `PutRemote` (writes a resource clean — not dirty, with server ETag/href), `SetCalendarMeta`, `SetSyncToken`; refactored `Put`/`PutRemote` onto a shared `writeResource`; exported `SafeName`
- `internal/model`: added `Parse(*ical.Calendar, loc)` (Decode now = decode-bytes + Parse) so the sync layer can consume go-webdav's already-decoded calendars
- `internal/config`: added the OS-aware path helpers `DataDir()` / `ConfigDir()` (XDG on Linux, `%LOCALAPPDATA%`/`%APPDATA%` on Windows, `Application Support` on macOS) — needed for a default data location
- **Runnable now**: `lazyplanner import` subcommand (thin wiring in `cmd/lazyplanner`) reads `--url/--username/--password` or `LAZYPLANNER_CALDAV_*` env vars, uses a NextCloud app password, cancels cleanly on Ctrl-C, and prints a summary. README documents it. The owner can validate against real NextCloud immediately
- Files: `internal/caldav/client.go`, `internal/sync/import.go`, `internal/store/remote.go`, `internal/config/paths.go`, `cmd/lazyplanner/import.go`; tests `internal/caldav/client_test.go` (httptest canned multistatus), `internal/sync/import_test.go` (fake source), `internal/store/remote_test.go`
- Tests added: `DownloadAll` against a canned CalDAV REPORT (validated the real query→parse path — and surfaced that go-webdav unquotes ETags via `strconv.Unquote`, so the store holds unquoted etags); `Import` (2 calendars, skips a bad resource, clean state persisted across reload); import discovery-error; `PutRemote`/`SetCalendarMeta` round-trip. All pass, including `-race`
- Verification: `gofmt`/`go build`/`go vet`/`staticcheck`/`go test ./...` (+`-race`) all pass; `go mod verify` clean; `lazyplanner import` with no creds returns a clean error (exit 1)
- Not yet handled (noted for later steps): calendar color (go-webdav's FindCalendars doesn't surface it), pruning of server-absent local resources, and pushing local edits — all part of two-way sync (step 9)
- Issues: none

---

## 2026-07-04 — Build step 4: vdir store

- **Build Plan step 4 complete.** `internal/store` is the local vdir cache — the first package with filesystem I/O. Reads a vdir tree into an in-memory index, writes resources back atomically, and persists sync state in a per-calendar JSON sidecar. Concurrency-safe (RWMutex; passes `go test -race`) since background sync will mutate it while the UI reads.
- Layout: `<dataDir>/calendars/<calendar-id>/<name>.ics` (one file per event/todo object, the local source of truth) + `.lazyplanner.json` sidecar per calendar (server-owned display name/color, sync token, href, and per-resource ETag/href/dirty). The `.ics` files win: sidecar is derived data, rebuilt on sync if lost/corrupt
- Types (all snapshots immutable; resources replaced copy-on-write, never mutated in place): `Store`, `Calendar` (metadata + `[]*Resource`), `Resource` (Name, ETag, Href, Dirty, parsed `*model.Parsed`), `LoadError`
- API:
  - `Open(ctx, dataDir)` — scans calendars, parses each `.ics`, merges sidecar sync state; missing dir → empty store (first run); unparseable/unreadable files are **skipped and recorded** in `LoadErrors()` (a corrupt file never blocks startup)
  - `Calendars()`, `Calendar(id)` — sorted snapshots; DisplayName falls back to id
  - `Todos()`, `EventOccurrences(from, to)` — the in-memory index backing task and calendar-view queries (occurrences via the step-3 recurrence engine, sorted)
  - `Put(ctx, calID, name, obj)` — atomic write-temp-fsync-rename (+ dir fsync), creates the calendar dir on first write, marks the resource `Dirty`, **preserves server identity (ETag/Href) on overwrite** so sync can detect local edits; updates index + sidecar
  - `Delete(ctx, calID, name)` — local delete (server propagation is the sync layer's job)
  - `ResourceName(uid)` — filesystem-safe `.ics` name for new resources
- `model`: added `(*Parsed).Encode()` (symmetric with `Decode`), keeping go-ical confined to `model`; the store round-trips resources through it, so unknown properties are preserved on write (verified by test — an `X-` property survives Put)
- Design decisions: I/O entry points take `context.Context` (checked for cancellation) per the no-uninterruptible-blocking rule; data files `0600` / dirs `0700` (private by default); filename keyed by sanitized UID for new resources, but existing resources keep their on-disk name so they map back to the same server resource
- Files: `internal/store/{store,mutate,sidecar}.go`; tests `internal/store/store_test.go` with a committed fixture vdir tree under `testdata/vdir/` (two calendars, a todo, an untracked file, a corrupt file) plus `t.TempDir()` round-trip tests
- Tests added: load tree (metadata, tracked/untracked ETags, sidecar fallback, load-error surfacing), queries, missing-dir, Put+reload+preservation, server-identity preservation on overwrite, Delete (+reload), cancelled-context — all pass, including `-race`
- Verification: `gofmt`/`go build`/`go vet`/`staticcheck`/`go test ./...` (and `-race`) all pass; `go mod verify` clean (no new deps)
- Issues: none

---

## 2026-07-04 — Build step 3: recurrence expansion

- **Build Plan step 3 complete.** `internal/model` now expands recurring events into concrete occurrences over a date window, wrapping `teambition/rrule-go` behind a small model API. Timezone-aware and heavily tested (recurrence is a classic bug farm).
- `rrule-go` promoted from indirect to a direct dependency; re-vendored (`go mod verify` clean)
- API (`internal/model/recurrence.go`):
  - `Occurrence{Start, End, Event}` — one materialized instance; `Event` points to the underlying component so the UI can show details and route edits
  - `(*Event).Occurrences(from, to)` — expands a single component's RRULE + RDATE − EXDATE within the half-open window `[from, to)`, anchored at DTSTART; non-recurring events yield at most one instance. Queries start one duration early so an instance that begins before the window but overlaps it is still returned
  - `(*Parsed).EventOccurrences(from, to)` — object-level expansion that applies **RECURRENCE-ID overrides**: an override replaces the instance it targets (moved time / changed details) and the original slot is suppressed; orphan overrides stand alone. Results sorted by start
  - `(*Event).Duration()` helper
- Correctness decisions, grounded by probing rrule-go's actual behavior first (then the probe was removed):
  - **DST**: instances keep wall-clock time across transitions (weekly 09:00 stays 09:00 local; the UTC instant shifts an hour). Verified explicitly in a spring-forward test
  - **DTSTART inclusion**: rrule-go emits DTSTART only via an RRULE, so for RDATE-only events DTSTART is added explicitly (it belongs to the recurrence set per RFC 5545)
  - **Must set `ROption.Dtstart`** before building the rule — rrule-go otherwise defaults it to "now"
  - **UNTIL** boundary is inclusive; **EXDATE** must match the instance instant (incl. TZID); all handled
  - **Not yet handled**: `RANGE=THISANDFUTURE` on an override (affects only its own instance for now — noted for the recurrence-editing step); recurring *todos* (deferred — their occurrence semantics tie to completion)
- Files: `internal/model/recurrence.go`; tests `internal/model/recurrence_test.go` with five new fixtures (`recur_weekly_dst`, `recur_exdate`, `recur_allday`, `recur_rdate`, `recur_override`)
- Tests added: weekly-DST, EXDATE, all-day recurring, RDATE-only, windowing (narrow / empty), non-recurring multi-day overlap, and RECURRENCE-ID override — all pass
- Verification: `gofmt`/`go build`/`go vet`/`staticcheck`/`go test ./...` all pass; `go mod verify` clean
- Issues: none

---

## 2026-07-04 — Build step 2: core model (iCalendar parsing)

- **Build Plan step 2 complete.** `internal/model` now parses events and todos from iCalendar data via `emersion/go-ical`. Pure logic, no filesystem/network I/O — fully headless.
- Added and vendored `github.com/emersion/go-ical` (pulls `teambition/rrule-go` as an indirect dep, ready for the recurrence step)
- Types: `Event` (UID, Summary, Start/End, AllDay, Location, Description, HasAlarm, Recurring) and `Todo` (UID, Summary, Due/HasDue/DueAllDay, Status, Priority, Categories, Description, ParentUID, Recurring, `Completed()`); `TodoStatus` enum + `PriorityUndefined`
- Parsers: `ParseEvent`/`ParseTodo` (per-component units) and `Decode` (whole-stream convenience). Design choices honoring the spec:
  - **Property-preservation iron rule**: each type keeps its source `*ical.Component` as `Raw`, and `Parsed.Calendar` retains the whole decoded calendar, so unknown properties/components survive a future re-encode. A decode→encode round-trip test proves an `X-` property and a `VALARM` are preserved
  - **All-day detection** via `VALUE=DATE` (with a bare-`YYYYMMDD` fallback); all-day/date-only values interpreted at local midnight
  - **Timezones**: TZID and UTC (`Z`) instants parsed correctly; floating times interpreted in a caller-supplied `loc` (defaults to `time.Local` per the local-timezone rule)
  - **Subtask hierarchy**: `ParentUID` from `RELATED-TO`, treating absent-or-`RELTYPE=PARENT` as the parent per RFC 5545 (matches NextCloud Tasks)
  - **Graceful degradation**: a malformed *required* field (event DTSTART) errors; malformed *optional* fields degrade to zero values rather than discarding the item. `Decode` fails on the first bad component; per-item resilience is left to the store layer (step 4)
  - **Reminder indicator**: `HasAlarm` reflects VALARM presence only — LazyPlanner never fires notifications
- Files: `internal/model/{decode,event,todo}.go`; tests `internal/model/model_test.go` (table-driven, `package model_test` against the public API) with six `testdata/*.ics` fixtures (timed/all-day/UTC-recurring events; timed-due/completed-subtask/minimal todos)
- Tests added: `TestParseEvents`, `TestParseTodos`, `TestRoundTripPreservesUnknownData`, `TestDecodeMalformedStreamErrors` — all pass (8 subtests, no skips; timezone DB present)
- Verification: `gofmt -l` clean; `go build`/`go vet`/`staticcheck`/`go test ./...` all pass
- Issues: none

---

## 2026-07-04 — Build step 1: project scaffold

- **Build Plan step 1 complete.** First code in the repo; toolchain proven end to end (build, vet, staticcheck, test, and a launchable tview window).
- `go mod init github.com/littekge/LazyPlanner` (Go 1.26.4). Added and **vendored** the UI deps: `rivo/tview` v0.42.0 + `gdamore/tcell/v2` v2.13.10 (with transitive deps); `go mod tidy && go mod vendor`, `vendor/` committed
- Package skeleton created per the `main.md` architecture: `cmd/lazyplanner/main.go` (thin wiring — app identity + hand-off to UI) and `internal/{config,model,store,caldav,sync,ui}`. The not-yet-implemented packages carry a `doc.go` with the package doc comment stating each one's responsibility and separation rule
- `internal/ui/app.go`: `Run(title)` builds a tview Application showing a centered placeholder window; quits cleanly on `q` (explicit) and `Ctrl-C` (tview default); mouse enabled. Only `ui` imports tview/tcell, per the architecture rules
- `.gitignore` (build output, go.work, coverage, editor/OS cruft; `vendor/` deliberately **not** ignored)
- CI: `.github/workflows/ci.yml` — GitHub Actions running `go build`/`go test`/`go vet` + `dominikh/staticcheck-action` on push and PR, using `go-version-file: go.mod`
- Files created: `go.mod`, `go.sum`, `vendor/**`, `cmd/lazyplanner/main.go`, `internal/{config,model,store,caldav,sync}/doc.go`, `internal/ui/app.go`, `.gitignore`, `.github/workflows/ci.yml`. `README.md` updated (status → scaffolding; real build/run instructions)
- Verification: `gofmt -l` clean; `go build ./...`, `go vet ./...`, `staticcheck ./...` all pass; `go test ./...` passes (no test files yet — thin UI glue and empty stubs; real tests begin at step 2). Manually confirmed the TUI: renders `LazyPlanner 0.0.1` and exits 0 on `q` in a pty; exits 1 with a wrapped error (no panic) when no TTY is available
- Issues: none. `go get` auto-upgraded tcell to v2.13.10 (from an initial v2.8.1 resolution) and pulled newer `golang.org/x` deps — expected, all vendored

---

## 2026-07-04 — Log repair: restored per-entry headings; format rule hardened

- Fixed `log.md`: 14 of 15 entries had lost their `## YYYY-MM-DD — Title` headings (an insert-at-top editing mistake repeatedly consumed the previous entry's heading), leaving anonymous `---`-separated blocks. All headings restored from session history; content unchanged
- `CLAUDE.md` Log Format section hardened: every entry gets its own heading (even same-day/same-session), never append under an existing heading, inserts must leave the previous entry byte-identical, and verify heading-count == entry-count after editing

---

## 2026-07-04 — Git branching rules for the build

- `CLAUDE.md`: new Git Branching Rules section — all Claude work happens on **`ai-workspace`** (or branches off it, merged back into it); **never merge or commit to `main`** (owner-only, after review); **`ai-init`** is a frozen branch preserving the pre-build-step-1 state (spec complete, no code) as a permanent reference/reset point
- Workflow commit step updated to name the branch
- Branches created: `ai-init` (frozen at this commit) and `ai-workspace` (checked out, ready for build step 1)

---

## 2026-07-04 — Final pre-build pass: handoff readiness audit

- Audited all spec files with fresh eyes ahead of a new build session; fixed staleness that would mislead a fresh reader:
  - `main.md` header status ("early skeleton" → "spec complete and code-ready, begin at Build Plan step 1"), Current State updated, leftover "TBD — more goals" design-goal bullet replaced with the well-behaved-CalDAV-citizen goal
  - `CLAUDE.md`: removed stale "will be expanded once language decided" note, fixed run command for the cmd/ layout (`go run ./cmd/lazyplanner`), added staticcheck install command (dev tool, not vendored), "config format TBD" → TOML
- Final decisions closed:
  - **License: MIT confirmed** — `LICENSE` (MIT, Gabriel Litteken) already existed from the initial commit and matches
  - **examples/ committed** — reference specs kept in the repo
  - **README.md is a living document**: what the program does, usage docs, build/install for Linux + Windows; updated in the same increment as any user-visible change. Rule added to CLAUDE.md workflow (step 6); starter README written (pre-build status, planned sections stubbed)
  - **CI: deferred to scaffold** — GitHub Actions (test/vet/staticcheck) added to Build Plan step 1, alongside `.gitignore`
- Spec is handoff-ready for the build session

---

## 2026-07-04 — UI follow-up decisions: colors, completed tasks, sorting, undo

- **Colors**: terminal 16-color palette (inherits terminal theme, works on TTY/Pi); server calendar colors mapped to nearest palette color. Truecolor theme rejected
- **Completed tasks**: hidden by default, `.` toggles struck-through display (dotfiles gesture)
- **Sibling task sort**: smart sort — due date, then priority, then title; manual ordering rejected (no standard iCal order field; wouldn't survive other clients)
- **Undo**: session-scoped undo stack (`u`) — every local mutation pushes the prior .ics version onto an in-memory stack; persistent trash deferred
- `main.md`: new subsection under UI Design; `u` and `.` added to keymap. `CLAUDE.md` UI line updated
- Remaining UI details (pane proportions, cell rendering, truncation) deliberately deferred to build steps 6–8 against real screens

---

## 2026-07-04 — UI design v1 draft: spec is code-ready

- **Layout**: lazygit-style three regions — left column of focusable panels (1 Calendars, 2 Tasks tree, 3 Agenda), Main pane whose content follows focus (calendar grid / zoomed task tree / day agenda), always-visible right Detail pane (owner requested event detail next to calendar), status bar with contextual hints + sync state. Chosen over "two workspaces" and "dashboard" alternatives
- **Task tree navigation**: full collapsible tree + `>`/`<` zoom (re-root at selected task, breadcrumb, like cd-ing into a directory). Chosen over ranger-style drill-in and plain tree
- **Creation UX**: `a` quick-add one-liner with smart parsing (dates, times, `!priority`, `#tag`; unparsed text → title; predictable rules documented in `:help`), `e` full form. Chosen over title-only quick-add and form-always
- Draft keybinding table and `:` command set written into `main.md` (hardcoded v1; `[keys]` config section deferred)
- Open Decisions section now empty — **spec is code-ready**; UI marked as v1 draft to refine against real screens during build steps 6–8. Non-blocking loose ends: confirm MIT, verify go-webdav calendar creation
- `CLAUDE.md`: UI summary line added to Project Context

---

## 2026-07-04 — Data model: fields, subtask hierarchy, preservation rule, recurrence scopes

- **Task fields surfaced**: title, due, status, priority (iCal 1–9), tags (CATEGORIES), notes, subtasks. **Subtasks are the owner's centerpiece feature** — arbitrary-depth nesting via RELATED-TO (RELTYPE=PARENT, same as NextCloud Tasks so existing data imports as-is), UI presents the tree like a file explorer; "folders" are just tasks with children
- **Event fields surfaced**: title, start/end, all-day, recurrence, location, notes, reminder indicator (LazyPlanner shows alarms exist but never fires notifications — phone/NextCloud handle that)
- **Property preservation iron rule**: never drop/mangle unrecognized iCal properties; edits to known fields preserve everything else. Added to CLAUDE.md as a hard rule
- **Timezones**: store server's data, display/create in system local timezone, all-day items date-only
- **Recurrence editing**: all three scopes (only-this via RECURRENCE-ID, this-and-future via series split, all via master edit)
- `main.md`: core features bullet rewritten around the subtask tree; six data-model entries added to Settled Decisions; Open Decisions down to UI design only
- Also: committed the spec files (d9cc198) — examples/ left untracked pending owner preference

---

## 2026-07-04 — Sync design: credentials, conflict resolution, triggers

- **Credentials**: NextCloud app password only (never the real password), stored in `config.toml` with enforced-0600 warning; optional `password_command` escape hatch (owner runs Vaultwarden, so `bw get password lazyplanner` works — Vaultwarden speaks the Bitwarden API). OS keyring rejected (daemon breaks headless Pi, extra dep + failure modes)
- **Conflicts**: ETag-based detection with conditional writes — never silently overwrite either direction; true conflicts keep both versions, flag the item, and surface a UI indicator for resolution at leisure (pick winner or keep both). "Newest wins" / "server wins" rejected as silent data-loss paths
- **Triggers**: manual `:sync` + all three automatic: background sync on startup (open instantly from cache), periodic while open (default 15 min, 0 = off), debounced push a few seconds after local edits
- `main.md`: three sync decisions added to Settled Decisions; Open Decisions down to data model details + UI design
- `CLAUDE.md`: sync summary line added to Project Context

---

## 2026-07-04 — Default config values set to owner's preferences

- Principle recorded in `main.md`: all moderate-scope options stay configurable in the config file; the *default value* of each option (when unset) is the owner's preference, so a working config needs only the `[server]` section (the one unavoidable first-run edit). Initially phrased as "hardcoded defaults" — corrected after owner feedback: reducing needed edits must not reduce config capability
- Defaults chosen: week starts Monday, 12-hour time display (2:30pm), month view on open, US date format (07/04/2026), sync all calendars with server names/colors

---

## 2026-07-04 — Config editing model; calendar metadata is server-owned

- **Config editing**: hand-edit + read-once-at-startup; the app never writes the config file. Planned conveniences: first-run generation of a fully-commented default config, and a `:config` command (open in `$EDITOR`, reload on exit). Auto-reload/file-watching explicitly rejected. App-remembered state (e.g., last view) goes in a state file under the data dir, not config.
- **Calendar metadata**: resolved the apparent conflict between "app never writes config" and "rename/recolor/create calendars in-app" — calendar identity, display name, and color are CalDAV properties owned by the server (cached in the vdir via sidecar convention), so in-app changes go through sync, not the config file, and propagate to NextCloud web/other clients. Config `[[calendars]]` sections demoted to optional local overrides (hide locally, override color locally); default is sync-everything with server names/colors. New calendars: CalDAV make-calendar call, with create-in-NextCloud-web as fallback if go-webdav lacks client support (verify at build time).
- `main.md`: config settled-decision entry updated (overrides, not definitions); two new settled decisions added (config editing model, server-owned calendar metadata)
- `CLAUDE.md`: config context line updated with the never-writes-config rule

---

## 2026-07-04 — Config decision: TOML, moderate scope; runtime paths; Windows as secondary target

- **Config file**: TOML via `BurntSushi/toml`, moderate scope — server connection, calendar selection/colors/visibility, appearance/behavior options (first day of week, default view, date/time formats, sync interval). Keybindings hardcoded for now; schema leaves room for a future `[keys]` section. Rejected: INI (no standard spec), YAML (footgun spec + heavy dep), JSON (no comments)
- **Platform scope**: Linux is primary (features tailored to it); Windows is a secondary compatibility build. All path resolution through one OS-aware helper (`os.UserConfigDir` + data-dir helper)
- **Runtime file locations** documented in `main.md`: config at `~/.config/lazyplanner/config.toml`, calendar data at `~/.local/share/lazyplanner/calendars/` (XDG data, NOT ~/.cache — offline edits live there, never disposable); Windows equivalents `%APPDATA%` / `%LOCALAPPDATA%`
- `main.md`: platform line updated, BurntSushi/toml added to libraries, config decision in Settled Decisions, Runtime File Locations section added under Architecture, Open Decisions now: sync details → data model → UI design
- `CLAUDE.md`: config + runtime paths line added to Project Context

---

## 2026-07-04 — Drafted: architecture, build plan, housekeeping

- `main.md` Architecture section: idiomatic Go layout (`cmd/lazyplanner/` entry point, `internal/{config,model,store,caldav,sync,ui}`, committed `vendor/`, tests beside code with `testdata/` fixtures) with separation rules — only `ui` imports tview; `model` does no I/O; `store` owns the cache dir; `caldav` owns HTTP. Note added explaining why Go doesn't use src/lib/include/test dirs.
- `main.md` Build Plan: 13 incremental steps — scaffold → model → recurrence → vdir store → CalDAV one-way import (early, to validate against real NextCloud data) → read-only UI shell → calendar views → editing → two-way sync (completes the must-have) → command mode → recurrence editing → background sync → Raspberry Pi target
- `main.md` housekeeping: module path `github.com/littekge/LazyPlanner` (matches GitHub remote), Go minimum = stable at scaffold time bumped only deliberately, license MIT (proposed, pending confirmation)
- `CLAUDE.md` Architecture Rules section filled in with the hard separation rules + "code is hand-edited by the user; keep it conventional and boring"
- Open Decisions reordered: config file (in discussion) → sync details → data model → UI design

---

## 2026-07-04 — Cache storage decision: vdir-style raw .ics files

- Chose **vdir-style raw `.ics` files** for the offline-first local cache: one file per event/todo, one directory per calendar (vdirsyncer/khal convention), JSON sidecar for sync state (ETags, sync tokens), in-memory index built at startup
- Reasons: 1:1 mapping onto CalDAV resources keeps sync logic simple, zero extra dependencies (pure Go, easy Pi cross-compile), human-readable/greppable when debugging sync
- Rejected: SQLite (cgo driver breaks cross-compile, pure-Go driver is a huge vendored dep, indexed queries unneeded at personal scale, binary format not inspectable); custom JSON (lossy translation away from native iCalendar format, breaks file-per-resource sync correspondence)
- `main.md`: decision added to Settled Decisions; Open Decisions rewritten as an ordered list of next decisions (architecture/package layout, build plan, sync design details, data model details, UI design, config file, housekeeping)
- `CLAUDE.md`: local cache rule added to Project Context (.ics files are the local source of truth)

---

## 2026-07-04 — TUI library decision: tview

- Chose **tview** (`rivo/tview` on `tcell`) over Bubble Tea and gocui:
  - tview: years of backwards compatibility, widgets (Table/Grid/Flex/InputField/Pages) that fit calendar/task views, k9s proves the target UX (`:` command mode, single-key shortcuts, mouse, panes)
  - Bubble Tea rejected: v2.0.0 (released 2026-07-03) is a breaking major version that also moved the module path to the vanity domain `charm.land` — churn profile conflicts with the robustness requirement
  - gocui rejected: original unmaintained; the active fork is tailored to lazygit and was recently absorbed into lazygit's own repo
- `main.md`: framework line filled in, decision + reasoning added to Settled Decisions, TUI item removed from Open Decisions
- `CLAUDE.md`: platform line updated with tview

---

## 2026-07-04 — Coding standards: Go conventions filled in

- `CLAUDE.md` "Other Conventions" section written: gofmt/goimports, `go vet` + `staticcheck` as the only linters, **vendored dependencies** (`vendor/` committed; `go mod tidy && go mod vendor` after dep changes; stdlib preferred), error wrapping with `%w` and no-panic policy, no global mutable state, Go naming + godoc comments on exports, `context.Context` on all I/O (UI must never block on network), table-driven tests with stdlib `testing` only, named constants over magic numbers
- `CLAUDE.md` workflow step 4 updated to include vet + staticcheck alongside tests
- Decisions made: vendoring chosen for build-forever robustness; staticcheck chosen over golangci-lint (less tooling churn) and over vet-only (better bug-finding)

---

## 2026-07-04 — Language decision: Go; offline-first CalDAV sync model

- Chose **Go** as the implementation language, driven by four requirements: lazygit-style interactive TUI, long-term robustness (static binary, Go 1 compatibility promise), CalDAV sync with an existing NextCloud server (the must-have feature), and speed on modest hardware (future Raspberry Pi terminal). Rust was runner-up; Python ruled out on robustness/speed.
- Chose **offline-first sync**: local cache is the working copy, NextCloud CalDAV server syncs in background/on demand.
- `main.md`: filled in language and libraries (`emersion/go-webdav`, `emersion/go-ical`, `teambition/rrule-go`; TUI lib TBD — Bubble Tea vs tview), added CalDAV sync as the top core feature, expanded design goals (`:` command mode, mouse, static-binary robustness, Pi target), added Settled Decisions section
- `CLAUDE.md`: platform line updated to Go, workflow test/run commands filled in (`go test ./...`, `go build ./...`), comment example converted to Go
- No code yet — next open decisions: TUI library, local cache storage format

---

## 2026-07-04 — Initial project structure: spec, log, and project rules

- `main.md` (new): minimal starting spec — project identity (language/libraries/license TBD), lazygit-inspired TUI description, initial core features (todo management, calendar views, recurring tasks/events), open decisions list
- `log.md` (new): change log initialized with this format
- `CLAUDE.md` (new): project context, iterative build workflow (test/run commands TBD), coding standards with Comment Rules (rest TBD), log entry format, architecture rules placeholder
- No code or tests yet — spec development is the next step

---
