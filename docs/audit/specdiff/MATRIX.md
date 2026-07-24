# Key×Context Consistency Matrix

> Purpose: an exhaustive key×context ledger for the LazyPlanner TUI (`internal/ui`), built to
> phase-2's execution method in
> [`docs/superpowers/specs/2026-07-24-v1.5.0-phase2-matrix-execution.md`](../../superpowers/specs/2026-07-24-v1.5.0-phase2-matrix-execution.md).
> Every cell below is a `(key/chord, mode×surface)` pair; a blank/`unverified` cell is the
> exhaustiveness guarantee — nothing can be silently dropped. Later tasks fill in `Actual behavior`,
> `Help bar`, `:help`, `README`, and `Verdict` per row, then triage divergences with the owner.
>
> **Row count: 517 scaffold rows, all `Verdict = unverified`.** This is the number that must reach
> zero (via `holds` / `code-diverges` / `doc-stale` / `inconsistent-across-contexts`) before phase 2
> closes.

---

## 1. Key axis

Built per the task brief: grep every key-dispatch site under `internal/ui/` (`SetInputCapture`,
`InputHandler`, `ev.Rune()`, `ev.Key()`, `case '...'`, `KeyRune`, `tcell.Key`), then union with the
documented axis (`main.md` §Keybindings, `README.md` §Keybindings, `internal/ui/help.go`'s
`helpSections`). Every key below carries at least one `file:line` site; a few carry more than one
because the same rune is dispatched from multiple places (global alias vs. a modal's local
translation).

### 1.1 Global keys (reachable from every NORMAL surface via `globalKeys`, `internal/ui/app.go:753`)

`Tab` `Shift-Tab` `Ctrl-W` `Ctrl-Left` `Ctrl-Right` · digit count-prefix `1-9`/`0` · motion
`h j k l` / arrows · `gg` `gt` `gd` `G` · create-prefix `i` and its continuations `it iT ie iE is iS
ic il i!` · quick-set prefix `s` (`sp sd`) · `y` `Y` `m` `p` `P` · `/` `n` `N` · `e` `d` `Space` · `u`
`r` · `:` `?` · `+` `-` `0` (bare) · `[` `]` `{` `}` · `.` · `V` · `Esc` · `Enter` (widget-dependent).

All sites: `app.go:753-1013`, `keys.go:31-58,141-164,184-270`.

### 1.2 Mode-gated global keys

- **Calendar-mode only** (still dispatched via the same `globalKeys` switch, gated on `a.mode ==
  modeCalendar`, not on which pane is focused): `v` (`app.go:973-980`), `f` (`app.go:984-988`), `b`
  (`app.go:989-993`).
- **Tasks-mode only** (gated on `a.mode == modeTasks`): `H` (`app.go:959-963`), `L`
  (`app.go:964-968`), `>` (`app.go:1006-1010`), `<` (`app.go:1011-1015`), fold-prefix `z` and its
  continuations `zR zM za` (`app.go:862-868`, `keys.go:46-50`).

### 1.3 Modal-family keys (each modal owns its own `SetInputCapture`, layered under `globalKeys`'
`modalOpen()` pass-through, `app.go:761-764`)

- **Resize sub-mode** (`Ctrl-W`, its own modal state): `Enter` `Ctrl-W` `Esc` `Left`/`Right` `h` `l`
  `H` `L` `q` — `keys.go:388-413`.
- **Grab** (single item, `m`): `Enter` `Esc` `h` `l` `j` `k` `J` `K` (+ mirrored arrows) —
  `grab.go:165-186`.
- **Bulk grab** (`m` inside SELECT): same key set, distinct hint/behavior — `bulkgrab.go:91-120`.
- **Select** (`V`): `Esc`, motion pass-through (`h j k l` / arrows / `gg` / `G` / `f` / `b` / count
  digits), `V` (exit), `Space` `d` `y` `Y` `m` — `selection.go:326-382`.
- **Calendar grid drill** (Enter into a day): arrows/`gg`/`G` (item-cycle), `Esc` (exit) —
  `calendarview.go:95-187`, `timegridview.go:416-477`. Note: `timeGridView.handleEventMode`
  (`timegridview.go:453-477`) dispatches spatial nav on **arrow keys only** — it has no `h`/`j`/`k`/`l`
  rune case, unlike `calendarView.handleEventMode` (`calendarview.go:143-187`), which does. Whether
  this is reachable divergence or dead code (global `hjkl`→arrow translation intercepts first; see
  report) is left for the next task to verify.
- **Forms** (`caretForm`): NORMAL nav `j k Tab Shift-Tab h l Enter Esc g G` — `forms.go:216-253`;
  an **open dropdown** hands off entirely to tview's own list (`Up`/`Down`/`Enter`/`Esc`) —
  `forms.go:196-199`; DRILL `Esc Enter Tab Shift-Tab` + raw typing/cursor/Backspace pass-through —
  `forms.go:296-310`; the **weekday-strip** field (reached via DRILL) adds `Left`/`Right`/`h`/`l`
  (move) and `Space` (toggle) — `weekdaystrip.go:137-158`.
- **Help overlay**: `Esc` `q` `?` close it; everything else passes through to tview's `TextView`
  default scroll, which — confirmed by reading `vendor/github.com/rivo/tview/textview.go:1341-1352`
  — natively binds `j`/`k` (not just arrows/PgUp/PgDn) — `help.go:109-135`.
- **Conflicts list**: `Esc` `q` close; `j`/`k` locally translated to Down/Up (the list is modal, so
  `globalKeys`' hjkl-alias never reaches it) — `conflicts.go:22-57`; the resolve sub-dialog is a
  plain `tview.Modal` (`Left`/`Right`/`Tab` button nav, `Enter` activate) — `conflicts.go:69-102`.
- **Account picker**: `Esc` closes (via `list.SetDoneFunc`, `command.go:172`); `j`/`k` locally
  translated; `Enter` switches — `command.go:139-188`. Note: unlike the Help and Conflicts modals,
  this list's `SetInputCapture` has **no explicit `q` case** — a candidate divergence, not yet
  verified (see report).
- **Color picker**: `Esc` `Enter` `Left`/`Right`/`h`/`l` (column) `Up`/`Down`/`j`/`k` (row) —
  `colorpicker.go:132-167`.
- **Command line** (`:`) and **search** (`/`) inputs: `Enter` `Esc` (typed text itself is not a
  "key" in this matrix's sense) — `command.go:16-42`, `search.go:21-63`.
- **Generic confirm/choice dialogs** (delete confirm, recurrence-scope picker, config-reload
  notice, …): plain `tview.Modal` defaults, `Left`/`Right`/`Tab` button nav + `Enter` — no bespoke
  `SetInputCapture`, so one row set covers all of them rather than one per dialog instance.
- **Which-key popup** (`i`/`g`/`z`/`s` prefix hint): draws no input capture of its own — the
  continuation key is consumed by `resolvePrefix` (`keys.go:82-121`) inside `globalKeys`, *before*
  `modalOpen()` is even checked (`app.go:754-758`). It is therefore **not** a separate modal context
  in this matrix; its keys are the same `i`/`g`/`z`/`s` chord rows already listed under §1.1/§1.2.

### 1.4 Doc-described, code-implicit (not app-dispatched — noted, not a matrix row)

- **`Ctrl-C`** — documented in `README.md` ("Force-quits immediately … same best-effort sync
  flush"). No `KeyCtrlC` case exists anywhere under `internal/ui/`. Reading
  `vendor/github.com/rivo/tview/application.go:432-436` shows tview's own event loop calls `a.Stop()`
  by default when `Ctrl-C` reaches it unhandled — the same `Stop()` the `q` key calls
  (`app.go:841`) — so the documented behavior is plausible via a library default rather than app
  code. Left out of the cell table (it is not a key the app itself dispatches on) but flagged for
  the next task to confirm the flush actually still runs on that path.

---

## 2. Context axis

Modes: **NORMAL** / **DRILL** / **GRAB** / **SELECT** / **RESIZE** (`internal/ui/render.go:601-605`,
surfaced via `interactionMode()`, `render.go:617-637`).

Surfaces (per the brief): task tree, month grid, week-day grid, agenda board, Calendars overview,
Tasks overview, Agenda overview, forms (NORMAL), forms (DRILL), modals.

### 2.1 Non-dropped combinations (used below)

- **NORMAL** × {task tree, month grid, week-day grid, agenda board, Calendars overview, Tasks
  overview, Agenda overview} — 7 contexts. (Calendars/Tasks/Agenda overview only exist inside their
  own mode, so e.g. "Calendars overview" implies `a.mode == modeCalendar`.)
- **DRILL** × {month grid, week-day grid} — 2 contexts, plus **forms (DRILL)** as its own row-set
  (§1.3).
- **GRAB** × {task tree, month grid, week-day grid, agenda board} — 4 contexts (single-item), plus
  **GRAB (bulk, via SELECT)** × {task tree, month grid, week-day grid} — 3 contexts.
- **SELECT** × {task tree, month grid, week-day grid} — 3 contexts.
- **RESIZE** — 1 context, no per-surface split (see drop reason below).
- **forms (NORMAL)** and **modals** — each its own context bucket, internally broken into
  sub-rows by modal/field type (§1.3) since "modals" spans several unrelated widgets.

### 2.2 Dropped combinations and why

| Dropped combination | Reason |
|---|---|
| RESIZE × any specific surface | `handleResizeKey` (`keys.go:388-413`) processes every key identically regardless of what was focused before `Ctrl-W`; it only ever touches `leftCol`/`detail` widths, never the surface's own content. Splitting it per-surface would be 10 identical rows for one behavior. |
| DRILL × task tree | The task tree has no deeper level to drill into — `render.go:613-616` states this explicitly ("the tree has no deeper level, so DRILL never shows in Tasks"). Tree navigation is NORMAL end to end. |
| DRILL × agenda board | The agenda board keeps no drill state (no `drillState`/`eventMode` fields); item selection there is via the `agendaList` row (keyboard) or a direct click on the board (mouse, gap-closer A) — there is no keyboard drill-in. |
| DRILL × Calendars/Tasks/Agenda overview | These are flat `tview.List`s with no drill concept. |
| DRILL × modals (non-form) | `interactionMode()` shows DRILL only when `a.formDrill` is true (`render.go:623-629`), and `a.formDrill` is force-reset to `false` whenever the item form opens/closes (`edit.go:848,860`) and wired only to the item `caretForm`'s `onDrill` callback. No other modal (help, conflicts, account picker, color picker, command/search input) ever sets it. |
| GRAB × Calendars/Tasks/Agenda overview | `startGrab` resolves its target via `currentTarget()` (`edit.go:75-98`), which has cases for `modeTasks` (tree), `modeCalendar` (drilled grid item), and `modeAgenda` (agenda list row) — none for the collection-picker overview lists themselves. |
| GRAB × forms/modals | `globalKeys` returns the event unhandled whenever `a.modalOpen()` (`app.go:761-764`), so `m` never reaches `startGrab` while a form or modal is open; no modal offers its own grab entry point. |
| SELECT × agenda board | `selContext()` (`selection.go:29-41`) switches only on `modeTasks`/`modeCalendar`; `modeAgenda` falls through to `selNone`. |
| SELECT × Calendars/Tasks/Agenda overview | `enterSelect` (`selection.go:51-99`) explicitly requires `a.tv.GetFocus()` to be `a.tree` or `a.calendarPrimitive()`; an overview list focused fails that check and flashes "Nothing to select here". |
| SELECT × forms/modals | Same `modalOpen()` gate as GRAB — `V` never reaches `enterSelect` while a form/modal is open. |

---

## 3. Cell table

Schema (fixed, per the execution spec):

| Key/chord | Context | Actual behavior (file:line) | Help bar | `:help` | README | Verdict |
|---|---|---|---|---|---|---|
| Tab (app.go:815) | NORMAL · task tree | | | | | unverified |
| Tab (app.go:815) | NORMAL · month grid | | | | | unverified |
| Tab (app.go:815) | NORMAL · week-day grid | | | | | unverified |
| Tab (app.go:815) | NORMAL · agenda board | | | | | unverified |
| Tab (app.go:815) | NORMAL · Calendars overview | | | | | unverified |
| Tab (app.go:815) | NORMAL · Tasks overview | | | | | unverified |
| Tab (app.go:815) | NORMAL · Agenda overview | | | | | unverified |
| Shift-Tab (app.go:818) | NORMAL · task tree | | | | | unverified |
| Shift-Tab (app.go:818) | NORMAL · month grid | | | | | unverified |
| Shift-Tab (app.go:818) | NORMAL · week-day grid | | | | | unverified |
| Shift-Tab (app.go:818) | NORMAL · agenda board | | | | | unverified |
| Shift-Tab (app.go:818) | NORMAL · Calendars overview | | | | | unverified |
| Shift-Tab (app.go:818) | NORMAL · Tasks overview | | | | | unverified |
| Shift-Tab (app.go:818) | NORMAL · Agenda overview | | | | | unverified |
| Ctrl-W (app.go:821) | NORMAL · task tree | | | | | unverified |
| Ctrl-W (app.go:821) | NORMAL · month grid | | | | | unverified |
| Ctrl-W (app.go:821) | NORMAL · week-day grid | | | | | unverified |
| Ctrl-W (app.go:821) | NORMAL · agenda board | | | | | unverified |
| Ctrl-W (app.go:821) | NORMAL · Calendars overview | | | | | unverified |
| Ctrl-W (app.go:821) | NORMAL · Tasks overview | | | | | unverified |
| Ctrl-W (app.go:821) | NORMAL · Agenda overview | | | | | unverified |
| Ctrl-Left (app.go:824) | NORMAL · task tree | | | | | unverified |
| Ctrl-Left (app.go:824) | NORMAL · month grid | | | | | unverified |
| Ctrl-Left (app.go:824) | NORMAL · week-day grid | | | | | unverified |
| Ctrl-Left (app.go:824) | NORMAL · agenda board | | | | | unverified |
| Ctrl-Left (app.go:824) | NORMAL · Calendars overview | | | | | unverified |
| Ctrl-Left (app.go:824) | NORMAL · Tasks overview | | | | | unverified |
| Ctrl-Left (app.go:824) | NORMAL · Agenda overview | | | | | unverified |
| Ctrl-Right (app.go:829) | NORMAL · task tree | | | | | unverified |
| Ctrl-Right (app.go:829) | NORMAL · month grid | | | | | unverified |
| Ctrl-Right (app.go:829) | NORMAL · week-day grid | | | | | unverified |
| Ctrl-Right (app.go:829) | NORMAL · agenda board | | | | | unverified |
| Ctrl-Right (app.go:829) | NORMAL · Calendars overview | | | | | unverified |
| Ctrl-Right (app.go:829) | NORMAL · Tasks overview | | | | | unverified |
| Ctrl-Right (app.go:829) | NORMAL · Agenda overview | | | | | unverified |
| 1-9 / 0 (count prefix) (app.go:787-794) | NORMAL · task tree | | | | | unverified |
| 1-9 / 0 (count prefix) (app.go:787-794) | NORMAL · month grid | | | | | unverified |
| 1-9 / 0 (count prefix) (app.go:787-794) | NORMAL · week-day grid | | | | | unverified |
| 1-9 / 0 (count prefix) (app.go:787-794) | NORMAL · agenda board | | | | | unverified |
| 1-9 / 0 (count prefix) (app.go:787-794) | NORMAL · Calendars overview | | | | | unverified |
| 1-9 / 0 (count prefix) (app.go:787-794) | NORMAL · Tasks overview | | | | | unverified |
| 1-9 / 0 (count prefix) (app.go:787-794) | NORMAL · Agenda overview | | | | | unverified |
| h j k l / arrows (motion) (keys.go:147-164) | NORMAL · task tree | | | | | unverified |
| h j k l / arrows (motion) (keys.go:147-164) | NORMAL · month grid | | | | | unverified |
| h j k l / arrows (motion) (keys.go:147-164) | NORMAL · week-day grid | | | | | unverified |
| h j k l / arrows (motion) (keys.go:147-164) | NORMAL · agenda board | | | | | unverified |
| h j k l / arrows (motion) (keys.go:147-164) | NORMAL · Calendars overview | | | | | unverified |
| h j k l / arrows (motion) (keys.go:147-164) | NORMAL · Tasks overview | | | | | unverified |
| h j k l / arrows (motion) (keys.go:147-164) | NORMAL · Agenda overview | | | | | unverified |
| gg (keys.go:42,187-195) | NORMAL · task tree | | | | | unverified |
| gg (keys.go:42,187-195) | NORMAL · month grid | | | | | unverified |
| gg (keys.go:42,187-195) | NORMAL · week-day grid | | | | | unverified |
| gg (keys.go:42,187-195) | NORMAL · agenda board | | | | | unverified |
| gg (keys.go:42,187-195) | NORMAL · Calendars overview | | | | | unverified |
| gg (keys.go:42,187-195) | NORMAL · Tasks overview | | | | | unverified |
| gg (keys.go:42,187-195) | NORMAL · Agenda overview | | | | | unverified |
| gt (keys.go:43,222-231) | NORMAL · task tree | | | | | unverified |
| gt (keys.go:43,222-231) | NORMAL · month grid | | | | | unverified |
| gt (keys.go:43,222-231) | NORMAL · week-day grid | | | | | unverified |
| gt (keys.go:43,222-231) | NORMAL · agenda board | | | | | unverified |
| gt (keys.go:43,222-231) | NORMAL · Calendars overview | | | | | unverified |
| gt (keys.go:43,222-231) | NORMAL · Tasks overview | | | | | unverified |
| gt (keys.go:43,222-231) | NORMAL · Agenda overview | | | | | unverified |
| gd (keys.go:44) | NORMAL · task tree | | | | | unverified |
| gd (keys.go:44) | NORMAL · month grid | | | | | unverified |
| gd (keys.go:44) | NORMAL · week-day grid | | | | | unverified |
| gd (keys.go:44) | NORMAL · agenda board | | | | | unverified |
| gd (keys.go:44) | NORMAL · Calendars overview | | | | | unverified |
| gd (keys.go:44) | NORMAL · Tasks overview | | | | | unverified |
| gd (keys.go:44) | NORMAL · Agenda overview | | | | | unverified |
| G (app.go:859; keys.go:238-270) | NORMAL · task tree | | | | | unverified |
| G (app.go:859; keys.go:238-270) | NORMAL · month grid | | | | | unverified |
| G (app.go:859; keys.go:238-270) | NORMAL · week-day grid | | | | | unverified |
| G (app.go:859; keys.go:238-270) | NORMAL · agenda board | | | | | unverified |
| G (app.go:859; keys.go:238-270) | NORMAL · Calendars overview | | | | | unverified |
| G (app.go:859; keys.go:238-270) | NORMAL · Tasks overview | | | | | unverified |
| G (app.go:859; keys.go:238-270) | NORMAL · Agenda overview | | | | | unverified |
| it (keys.go:32) | NORMAL · task tree | | | | | unverified |
| it (keys.go:32) | NORMAL · month grid | | | | | unverified |
| it (keys.go:32) | NORMAL · week-day grid | | | | | unverified |
| it (keys.go:32) | NORMAL · agenda board | | | | | unverified |
| it (keys.go:32) | NORMAL · Calendars overview | | | | | unverified |
| it (keys.go:32) | NORMAL · Tasks overview | | | | | unverified |
| it (keys.go:32) | NORMAL · Agenda overview | | | | | unverified |
| iT (keys.go:33) | NORMAL · task tree | | | | | unverified |
| iT (keys.go:33) | NORMAL · month grid | | | | | unverified |
| iT (keys.go:33) | NORMAL · week-day grid | | | | | unverified |
| iT (keys.go:33) | NORMAL · agenda board | | | | | unverified |
| iT (keys.go:33) | NORMAL · Calendars overview | | | | | unverified |
| iT (keys.go:33) | NORMAL · Tasks overview | | | | | unverified |
| iT (keys.go:33) | NORMAL · Agenda overview | | | | | unverified |
| ie (keys.go:34) | NORMAL · task tree | | | | | unverified |
| ie (keys.go:34) | NORMAL · month grid | | | | | unverified |
| ie (keys.go:34) | NORMAL · week-day grid | | | | | unverified |
| ie (keys.go:34) | NORMAL · agenda board | | | | | unverified |
| ie (keys.go:34) | NORMAL · Calendars overview | | | | | unverified |
| ie (keys.go:34) | NORMAL · Tasks overview | | | | | unverified |
| ie (keys.go:34) | NORMAL · Agenda overview | | | | | unverified |
| iE (keys.go:35) | NORMAL · task tree | | | | | unverified |
| iE (keys.go:35) | NORMAL · month grid | | | | | unverified |
| iE (keys.go:35) | NORMAL · week-day grid | | | | | unverified |
| iE (keys.go:35) | NORMAL · agenda board | | | | | unverified |
| iE (keys.go:35) | NORMAL · Calendars overview | | | | | unverified |
| iE (keys.go:35) | NORMAL · Tasks overview | | | | | unverified |
| iE (keys.go:35) | NORMAL · Agenda overview | | | | | unverified |
| is (keys.go:36) | NORMAL · task tree | | | | | unverified |
| is (keys.go:36) | NORMAL · month grid | | | | | unverified |
| is (keys.go:36) | NORMAL · week-day grid | | | | | unverified |
| is (keys.go:36) | NORMAL · agenda board | | | | | unverified |
| is (keys.go:36) | NORMAL · Calendars overview | | | | | unverified |
| is (keys.go:36) | NORMAL · Tasks overview | | | | | unverified |
| is (keys.go:36) | NORMAL · Agenda overview | | | | | unverified |
| iS (keys.go:37) | NORMAL · task tree | | | | | unverified |
| iS (keys.go:37) | NORMAL · month grid | | | | | unverified |
| iS (keys.go:37) | NORMAL · week-day grid | | | | | unverified |
| iS (keys.go:37) | NORMAL · agenda board | | | | | unverified |
| iS (keys.go:37) | NORMAL · Calendars overview | | | | | unverified |
| iS (keys.go:37) | NORMAL · Tasks overview | | | | | unverified |
| iS (keys.go:37) | NORMAL · Agenda overview | | | | | unverified |
| ic (keys.go:38) | NORMAL · task tree | | | | | unverified |
| ic (keys.go:38) | NORMAL · month grid | | | | | unverified |
| ic (keys.go:38) | NORMAL · week-day grid | | | | | unverified |
| ic (keys.go:38) | NORMAL · agenda board | | | | | unverified |
| ic (keys.go:38) | NORMAL · Calendars overview | | | | | unverified |
| ic (keys.go:38) | NORMAL · Tasks overview | | | | | unverified |
| ic (keys.go:38) | NORMAL · Agenda overview | | | | | unverified |
| il (keys.go:39) | NORMAL · task tree | | | | | unverified |
| il (keys.go:39) | NORMAL · month grid | | | | | unverified |
| il (keys.go:39) | NORMAL · week-day grid | | | | | unverified |
| il (keys.go:39) | NORMAL · agenda board | | | | | unverified |
| il (keys.go:39) | NORMAL · Calendars overview | | | | | unverified |
| il (keys.go:39) | NORMAL · Tasks overview | | | | | unverified |
| il (keys.go:39) | NORMAL · Agenda overview | | | | | unverified |
| i! (keys.go:87-91) | NORMAL · task tree | | | | | unverified |
| i! (keys.go:87-91) | NORMAL · month grid | | | | | unverified |
| i! (keys.go:87-91) | NORMAL · week-day grid | | | | | unverified |
| i! (keys.go:87-91) | NORMAL · agenda board | | | | | unverified |
| i! (keys.go:87-91) | NORMAL · Calendars overview | | | | | unverified |
| i! (keys.go:87-91) | NORMAL · Tasks overview | | | | | unverified |
| i! (keys.go:87-91) | NORMAL · Agenda overview | | | | | unverified |
| sp (keys.go:52) | NORMAL · task tree | | | | | unverified |
| sp (keys.go:52) | NORMAL · month grid | | | | | unverified |
| sp (keys.go:52) | NORMAL · week-day grid | | | | | unverified |
| sp (keys.go:52) | NORMAL · agenda board | | | | | unverified |
| sp (keys.go:52) | NORMAL · Calendars overview | | | | | unverified |
| sp (keys.go:52) | NORMAL · Tasks overview | | | | | unverified |
| sp (keys.go:52) | NORMAL · Agenda overview | | | | | unverified |
| sd (keys.go:53) | NORMAL · task tree | | | | | unverified |
| sd (keys.go:53) | NORMAL · month grid | | | | | unverified |
| sd (keys.go:53) | NORMAL · week-day grid | | | | | unverified |
| sd (keys.go:53) | NORMAL · agenda board | | | | | unverified |
| sd (keys.go:53) | NORMAL · Calendars overview | | | | | unverified |
| sd (keys.go:53) | NORMAL · Tasks overview | | | | | unverified |
| sd (keys.go:53) | NORMAL · Agenda overview | | | | | unverified |
| y (app.go:876) | NORMAL · task tree | | | | | unverified |
| y (app.go:876) | NORMAL · month grid | | | | | unverified |
| y (app.go:876) | NORMAL · week-day grid | | | | | unverified |
| y (app.go:876) | NORMAL · agenda board | | | | | unverified |
| y (app.go:876) | NORMAL · Calendars overview | | | | | unverified |
| y (app.go:876) | NORMAL · Tasks overview | | | | | unverified |
| y (app.go:876) | NORMAL · Agenda overview | | | | | unverified |
| Y (app.go:879) | NORMAL · task tree | | | | | unverified |
| Y (app.go:879) | NORMAL · month grid | | | | | unverified |
| Y (app.go:879) | NORMAL · week-day grid | | | | | unverified |
| Y (app.go:879) | NORMAL · agenda board | | | | | unverified |
| Y (app.go:879) | NORMAL · Calendars overview | | | | | unverified |
| Y (app.go:879) | NORMAL · Tasks overview | | | | | unverified |
| Y (app.go:879) | NORMAL · Agenda overview | | | | | unverified |
| m (app.go:882) | NORMAL · task tree | | | | | unverified |
| m (app.go:882) | NORMAL · month grid | | | | | unverified |
| m (app.go:882) | NORMAL · week-day grid | | | | | unverified |
| m (app.go:882) | NORMAL · agenda board | | | | | unverified |
| m (app.go:882) | NORMAL · Calendars overview | | | | | unverified |
| m (app.go:882) | NORMAL · Tasks overview | | | | | unverified |
| m (app.go:882) | NORMAL · Agenda overview | | | | | unverified |
| p (app.go:885) | NORMAL · task tree | | | | | unverified |
| p (app.go:885) | NORMAL · month grid | | | | | unverified |
| p (app.go:885) | NORMAL · week-day grid | | | | | unverified |
| p (app.go:885) | NORMAL · agenda board | | | | | unverified |
| p (app.go:885) | NORMAL · Calendars overview | | | | | unverified |
| p (app.go:885) | NORMAL · Tasks overview | | | | | unverified |
| p (app.go:885) | NORMAL · Agenda overview | | | | | unverified |
| P (app.go:888) | NORMAL · task tree | | | | | unverified |
| P (app.go:888) | NORMAL · month grid | | | | | unverified |
| P (app.go:888) | NORMAL · week-day grid | | | | | unverified |
| P (app.go:888) | NORMAL · agenda board | | | | | unverified |
| P (app.go:888) | NORMAL · Calendars overview | | | | | unverified |
| P (app.go:888) | NORMAL · Tasks overview | | | | | unverified |
| P (app.go:888) | NORMAL · Agenda overview | | | | | unverified |
| / (app.go:891) | NORMAL · task tree | | | | | unverified |
| / (app.go:891) | NORMAL · month grid | | | | | unverified |
| / (app.go:891) | NORMAL · week-day grid | | | | | unverified |
| / (app.go:891) | NORMAL · agenda board | | | | | unverified |
| / (app.go:891) | NORMAL · Calendars overview | | | | | unverified |
| / (app.go:891) | NORMAL · Tasks overview | | | | | unverified |
| / (app.go:891) | NORMAL · Agenda overview | | | | | unverified |
| n (app.go:894) | NORMAL · task tree | | | | | unverified |
| n (app.go:894) | NORMAL · month grid | | | | | unverified |
| n (app.go:894) | NORMAL · week-day grid | | | | | unverified |
| n (app.go:894) | NORMAL · agenda board | | | | | unverified |
| n (app.go:894) | NORMAL · Calendars overview | | | | | unverified |
| n (app.go:894) | NORMAL · Tasks overview | | | | | unverified |
| n (app.go:894) | NORMAL · Agenda overview | | | | | unverified |
| N (app.go:897) | NORMAL · task tree | | | | | unverified |
| N (app.go:897) | NORMAL · month grid | | | | | unverified |
| N (app.go:897) | NORMAL · week-day grid | | | | | unverified |
| N (app.go:897) | NORMAL · agenda board | | | | | unverified |
| N (app.go:897) | NORMAL · Calendars overview | | | | | unverified |
| N (app.go:897) | NORMAL · Tasks overview | | | | | unverified |
| N (app.go:897) | NORMAL · Agenda overview | | | | | unverified |
| e (app.go:900) | NORMAL · task tree | | | | | unverified |
| e (app.go:900) | NORMAL · month grid | | | | | unverified |
| e (app.go:900) | NORMAL · week-day grid | | | | | unverified |
| e (app.go:900) | NORMAL · agenda board | | | | | unverified |
| e (app.go:900) | NORMAL · Calendars overview | | | | | unverified |
| e (app.go:900) | NORMAL · Tasks overview | | | | | unverified |
| e (app.go:900) | NORMAL · Agenda overview | | | | | unverified |
| d (app.go:903) | NORMAL · task tree | | | | | unverified |
| d (app.go:903) | NORMAL · month grid | | | | | unverified |
| d (app.go:903) | NORMAL · week-day grid | | | | | unverified |
| d (app.go:903) | NORMAL · agenda board | | | | | unverified |
| d (app.go:903) | NORMAL · Calendars overview | | | | | unverified |
| d (app.go:903) | NORMAL · Tasks overview | | | | | unverified |
| d (app.go:903) | NORMAL · Agenda overview | | | | | unverified |
| Space (app.go:906-924) | NORMAL · task tree | | | | | unverified |
| Space (app.go:906-924) | NORMAL · month grid | | | | | unverified |
| Space (app.go:906-924) | NORMAL · week-day grid | | | | | unverified |
| Space (app.go:906-924) | NORMAL · agenda board | | | | | unverified |
| Space (app.go:906-924) | NORMAL · Calendars overview | | | | | unverified |
| Space (app.go:906-924) | NORMAL · Tasks overview | | | | | unverified |
| Space (app.go:906-924) | NORMAL · Agenda overview | | | | | unverified |
| u (app.go:925) | NORMAL · task tree | | | | | unverified |
| u (app.go:925) | NORMAL · month grid | | | | | unverified |
| u (app.go:925) | NORMAL · week-day grid | | | | | unverified |
| u (app.go:925) | NORMAL · agenda board | | | | | unverified |
| u (app.go:925) | NORMAL · Calendars overview | | | | | unverified |
| u (app.go:925) | NORMAL · Tasks overview | | | | | unverified |
| u (app.go:925) | NORMAL · Agenda overview | | | | | unverified |
| r (app.go:928) | NORMAL · task tree | | | | | unverified |
| r (app.go:928) | NORMAL · month grid | | | | | unverified |
| r (app.go:928) | NORMAL · week-day grid | | | | | unverified |
| r (app.go:928) | NORMAL · agenda board | | | | | unverified |
| r (app.go:928) | NORMAL · Calendars overview | | | | | unverified |
| r (app.go:928) | NORMAL · Tasks overview | | | | | unverified |
| r (app.go:928) | NORMAL · Agenda overview | | | | | unverified |
| : (app.go:932) | NORMAL · task tree | | | | | unverified |
| : (app.go:932) | NORMAL · month grid | | | | | unverified |
| : (app.go:932) | NORMAL · week-day grid | | | | | unverified |
| : (app.go:932) | NORMAL · agenda board | | | | | unverified |
| : (app.go:932) | NORMAL · Calendars overview | | | | | unverified |
| : (app.go:932) | NORMAL · Tasks overview | | | | | unverified |
| : (app.go:932) | NORMAL · Agenda overview | | | | | unverified |
| ? (app.go:935) | NORMAL · task tree | | | | | unverified |
| ? (app.go:935) | NORMAL · month grid | | | | | unverified |
| ? (app.go:935) | NORMAL · week-day grid | | | | | unverified |
| ? (app.go:935) | NORMAL · agenda board | | | | | unverified |
| ? (app.go:935) | NORMAL · Calendars overview | | | | | unverified |
| ? (app.go:935) | NORMAL · Tasks overview | | | | | unverified |
| ? (app.go:935) | NORMAL · Agenda overview | | | | | unverified |
| + (app.go:938-944) | NORMAL · task tree | | | | | unverified |
| + (app.go:938-944) | NORMAL · month grid | | | | | unverified |
| + (app.go:938-944) | NORMAL · week-day grid | | | | | unverified |
| + (app.go:938-944) | NORMAL · agenda board | | | | | unverified |
| + (app.go:938-944) | NORMAL · Calendars overview | | | | | unverified |
| + (app.go:938-944) | NORMAL · Tasks overview | | | | | unverified |
| + (app.go:938-944) | NORMAL · Agenda overview | | | | | unverified |
| - (app.go:945-951) | NORMAL · task tree | | | | | unverified |
| - (app.go:945-951) | NORMAL · month grid | | | | | unverified |
| - (app.go:945-951) | NORMAL · week-day grid | | | | | unverified |
| - (app.go:945-951) | NORMAL · agenda board | | | | | unverified |
| - (app.go:945-951) | NORMAL · Calendars overview | | | | | unverified |
| - (app.go:945-951) | NORMAL · Tasks overview | | | | | unverified |
| - (app.go:945-951) | NORMAL · Agenda overview | | | | | unverified |
| 0 (bare) (app.go:952-958) | NORMAL · task tree | | | | | unverified |
| 0 (bare) (app.go:952-958) | NORMAL · month grid | | | | | unverified |
| 0 (bare) (app.go:952-958) | NORMAL · week-day grid | | | | | unverified |
| 0 (bare) (app.go:952-958) | NORMAL · agenda board | | | | | unverified |
| 0 (bare) (app.go:952-958) | NORMAL · Calendars overview | | | | | unverified |
| 0 (bare) (app.go:952-958) | NORMAL · Tasks overview | | | | | unverified |
| 0 (bare) (app.go:952-958) | NORMAL · Agenda overview | | | | | unverified |
| [ (app.go:994) | NORMAL · task tree | | | | | unverified |
| [ (app.go:994) | NORMAL · month grid | | | | | unverified |
| [ (app.go:994) | NORMAL · week-day grid | | | | | unverified |
| [ (app.go:994) | NORMAL · agenda board | | | | | unverified |
| [ (app.go:994) | NORMAL · Calendars overview | | | | | unverified |
| [ (app.go:994) | NORMAL · Tasks overview | | | | | unverified |
| [ (app.go:994) | NORMAL · Agenda overview | | | | | unverified |
| ] (app.go:997) | NORMAL · task tree | | | | | unverified |
| ] (app.go:997) | NORMAL · month grid | | | | | unverified |
| ] (app.go:997) | NORMAL · week-day grid | | | | | unverified |
| ] (app.go:997) | NORMAL · agenda board | | | | | unverified |
| ] (app.go:997) | NORMAL · Calendars overview | | | | | unverified |
| ] (app.go:997) | NORMAL · Tasks overview | | | | | unverified |
| ] (app.go:997) | NORMAL · Agenda overview | | | | | unverified |
| { (app.go:1000) | NORMAL · task tree | | | | | unverified |
| { (app.go:1000) | NORMAL · month grid | | | | | unverified |
| { (app.go:1000) | NORMAL · week-day grid | | | | | unverified |
| { (app.go:1000) | NORMAL · agenda board | | | | | unverified |
| { (app.go:1000) | NORMAL · Calendars overview | | | | | unverified |
| { (app.go:1000) | NORMAL · Tasks overview | | | | | unverified |
| { (app.go:1000) | NORMAL · Agenda overview | | | | | unverified |
| } (app.go:1003) | NORMAL · task tree | | | | | unverified |
| } (app.go:1003) | NORMAL · month grid | | | | | unverified |
| } (app.go:1003) | NORMAL · week-day grid | | | | | unverified |
| } (app.go:1003) | NORMAL · agenda board | | | | | unverified |
| } (app.go:1003) | NORMAL · Calendars overview | | | | | unverified |
| } (app.go:1003) | NORMAL · Tasks overview | | | | | unverified |
| } (app.go:1003) | NORMAL · Agenda overview | | | | | unverified |
| . (app.go:969) | NORMAL · task tree | | | | | unverified |
| . (app.go:969) | NORMAL · month grid | | | | | unverified |
| . (app.go:969) | NORMAL · week-day grid | | | | | unverified |
| . (app.go:969) | NORMAL · agenda board | | | | | unverified |
| . (app.go:969) | NORMAL · Calendars overview | | | | | unverified |
| . (app.go:969) | NORMAL · Tasks overview | | | | | unverified |
| . (app.go:969) | NORMAL · Agenda overview | | | | | unverified |
| V (app.go:981; selection.go:51-99) | NORMAL · task tree | | | | | unverified |
| V (app.go:981; selection.go:51-99) | NORMAL · month grid | | | | | unverified |
| V (app.go:981; selection.go:51-99) | NORMAL · week-day grid | | | | | unverified |
| V (app.go:981; selection.go:51-99) | NORMAL · agenda board | | | | | unverified |
| V (app.go:981; selection.go:51-99) | NORMAL · Calendars overview | | | | | unverified |
| V (app.go:981; selection.go:51-99) | NORMAL · Tasks overview | | | | | unverified |
| V (app.go:981; selection.go:51-99) | NORMAL · Agenda overview | | | | | unverified |
| Esc (app.go:834-838) | NORMAL · task tree | | | | | unverified |
| Esc (app.go:834-838) | NORMAL · month grid | | | | | unverified |
| Esc (app.go:834-838) | NORMAL · week-day grid | | | | | unverified |
| Esc (app.go:834-838) | NORMAL · agenda board | | | | | unverified |
| Esc (app.go:834-838) | NORMAL · Calendars overview | | | | | unverified |
| Esc (app.go:834-838) | NORMAL · Tasks overview | | | | | unverified |
| Esc (app.go:834-838) | NORMAL · Agenda overview | | | | | unverified |
| Enter (widget-dependent (calendarview.go:119; timegridview.go:437; list/tree default)) | NORMAL · task tree | | | | | unverified |
| Enter (widget-dependent (calendarview.go:119; timegridview.go:437; list/tree default)) | NORMAL · month grid | | | | | unverified |
| Enter (widget-dependent (calendarview.go:119; timegridview.go:437; list/tree default)) | NORMAL · week-day grid | | | | | unverified |
| Enter (widget-dependent (calendarview.go:119; timegridview.go:437; list/tree default)) | NORMAL · agenda board | | | | | unverified |
| Enter (widget-dependent (calendarview.go:119; timegridview.go:437; list/tree default)) | NORMAL · Calendars overview | | | | | unverified |
| Enter (widget-dependent (calendarview.go:119; timegridview.go:437; list/tree default)) | NORMAL · Tasks overview | | | | | unverified |
| Enter (widget-dependent (calendarview.go:119; timegridview.go:437; list/tree default)) | NORMAL · Agenda overview | | | | | unverified |
| v (app.go:973-980) [Calendar-mode gated] | NORMAL · month grid | | | | | unverified |
| v (app.go:973-980) [Calendar-mode gated] | NORMAL · week-day grid | | | | | unverified |
| v (app.go:973-980) [Calendar-mode gated] | NORMAL · Calendars overview | | | | | unverified |
| f (app.go:984-988) [Calendar-mode gated] | NORMAL · month grid | | | | | unverified |
| f (app.go:984-988) [Calendar-mode gated] | NORMAL · week-day grid | | | | | unverified |
| f (app.go:984-988) [Calendar-mode gated] | NORMAL · Calendars overview | | | | | unverified |
| b (app.go:989-993) [Calendar-mode gated] | NORMAL · month grid | | | | | unverified |
| b (app.go:989-993) [Calendar-mode gated] | NORMAL · week-day grid | | | | | unverified |
| b (app.go:989-993) [Calendar-mode gated] | NORMAL · Calendars overview | | | | | unverified |
| H (app.go:959-963) [Tasks-mode gated] | NORMAL · task tree | | | | | unverified |
| H (app.go:959-963) [Tasks-mode gated] | NORMAL · Tasks overview | | | | | unverified |
| L (app.go:964-968) [Tasks-mode gated] | NORMAL · task tree | | | | | unverified |
| L (app.go:964-968) [Tasks-mode gated] | NORMAL · Tasks overview | | | | | unverified |
| > (app.go:1006-1010) [Tasks-mode gated] | NORMAL · task tree | | | | | unverified |
| > (app.go:1006-1010) [Tasks-mode gated] | NORMAL · Tasks overview | | | | | unverified |
| < (app.go:1011-1015) [Tasks-mode gated] | NORMAL · task tree | | | | | unverified |
| < (app.go:1011-1015) [Tasks-mode gated] | NORMAL · Tasks overview | | | | | unverified |
| z (app.go:862-868) [Tasks-mode gated] | NORMAL · task tree | | | | | unverified |
| z (app.go:862-868) [Tasks-mode gated] | NORMAL · Tasks overview | | | | | unverified |
| zR (keys.go:47) [Tasks-mode gated] | NORMAL · task tree | | | | | unverified |
| zR (keys.go:47) [Tasks-mode gated] | NORMAL · Tasks overview | | | | | unverified |
| zM (keys.go:48) [Tasks-mode gated] | NORMAL · task tree | | | | | unverified |
| zM (keys.go:48) [Tasks-mode gated] | NORMAL · Tasks overview | | | | | unverified |
| za (keys.go:49) [Tasks-mode gated] | NORMAL · task tree | | | | | unverified |
| za (keys.go:49) [Tasks-mode gated] | NORMAL · Tasks overview | | | | | unverified |
| h j k l / arrows (item cycle / spatial nav) (calendarview.go:143-186; timegridview.go:453-477 (arrows only — see report note)) | DRILL · month grid | | | | | unverified |
| h j k l / arrows (item cycle / spatial nav) (calendarview.go:143-186; timegridview.go:453-477 (arrows only — see report note)) | DRILL · week-day grid | | | | | unverified |
| gg (Home) (calendarview.go:161-165; timegridview.go:466-470) | DRILL · month grid | | | | | unverified |
| gg (Home) (calendarview.go:161-165; timegridview.go:466-470) | DRILL · week-day grid | | | | | unverified |
| G (End) (calendarview.go:166-170; timegridview.go:471-475) | DRILL · month grid | | | | | unverified |
| G (End) (calendarview.go:166-170; timegridview.go:471-475) | DRILL · week-day grid | | | | | unverified |
| Enter (no case in handleEventMode — calendarview.go:143-187; timegridview.go:453-477) | DRILL · month grid | | | | | unverified |
| Enter (no case in handleEventMode — calendarview.go:143-187; timegridview.go:453-477) | DRILL · week-day grid | | | | | unverified |
| Esc (calendarview.go:146-150; timegridview.go:456-457) | DRILL · month grid | | | | | unverified |
| Esc (calendarview.go:146-150; timegridview.go:456-457) | DRILL · week-day grid | | | | | unverified |
| Space (app.go:906-924) | DRILL · month grid | | | | | unverified |
| Space (app.go:906-924) | DRILL · week-day grid | | | | | unverified |
| e (app.go:900) | DRILL · month grid | | | | | unverified |
| e (app.go:900) | DRILL · week-day grid | | | | | unverified |
| d (app.go:903) | DRILL · month grid | | | | | unverified |
| d (app.go:903) | DRILL · week-day grid | | | | | unverified |
| m (app.go:882 (enters GRAB on the drilled item)) | DRILL · month grid | | | | | unverified |
| m (app.go:882 (enters GRAB on the drilled item)) | DRILL · week-day grid | | | | | unverified |
| V (app.go:981; selection.go:65-79 (selDrill)) | DRILL · month grid | | | | | unverified |
| V (app.go:981; selection.go:65-79 (selDrill)) | DRILL · week-day grid | | | | | unverified |
| f (app.go:984-988) | DRILL · month grid | | | | | unverified |
| f (app.go:984-988) | DRILL · week-day grid | | | | | unverified |
| b (app.go:989-993) | DRILL · month grid | | | | | unverified |
| b (app.go:989-993) | DRILL · week-day grid | | | | | unverified |
| v (app.go:973-980) | DRILL · month grid | | | | | unverified |
| v (app.go:973-980) | DRILL · week-day grid | | | | | unverified |
| + / - (app.go:938-951) | DRILL · month grid | | | | | unverified |
| + / - (app.go:938-951) | DRILL · week-day grid | | | | | unverified |
| 0 (bare) (app.go:952-958) | DRILL · month grid | | | | | unverified |
| 0 (bare) (app.go:952-958) | DRILL · week-day grid | | | | | unverified |
| Enter (grab.go:167) | GRAB · task tree | | | | | unverified |
| Enter (grab.go:167) | GRAB · month grid | | | | | unverified |
| Enter (grab.go:167) | GRAB · week-day grid | | | | | unverified |
| Enter (grab.go:167) | GRAB · agenda board | | | | | unverified |
| Esc (grab.go:169) | GRAB · task tree | | | | | unverified |
| Esc (grab.go:169) | GRAB · month grid | | | | | unverified |
| Esc (grab.go:169) | GRAB · week-day grid | | | | | unverified |
| Esc (grab.go:169) | GRAB · agenda board | | | | | unverified |
| h l / Left Right (grab.go:171-174,181-183,246-267) | GRAB · task tree | | | | | unverified |
| h l / Left Right (grab.go:171-174,181-183,246-267) | GRAB · month grid | | | | | unverified |
| h l / Left Right (grab.go:171-174,181-183,246-267) | GRAB · week-day grid | | | | | unverified |
| h l / Left Right (grab.go:171-174,181-183,246-267) | GRAB · agenda board | | | | | unverified |
| j k / Up Down (grab.go:175-178,181-183,268-280) | GRAB · task tree | | | | | unverified |
| j k / Up Down (grab.go:175-178,181-183,268-280) | GRAB · month grid | | | | | unverified |
| j k / Up Down (grab.go:175-178,181-183,268-280) | GRAB · week-day grid | | | | | unverified |
| j k / Up Down (grab.go:175-178,181-183,268-280) | GRAB · agenda board | | | | | unverified |
| J K (grab.go:181-183,281-298) | GRAB · task tree | | | | | unverified |
| J K (grab.go:181-183,281-298) | GRAB · month grid | | | | | unverified |
| J K (grab.go:181-183,281-298) | GRAB · week-day grid | | | | | unverified |
| J K (grab.go:181-183,281-298) | GRAB · agenda board | | | | | unverified |
| Enter (bulkgrab.go:93) | GRAB (bulk, via SELECT) · task tree | | | | | unverified |
| Enter (bulkgrab.go:93) | GRAB (bulk, via SELECT) · month grid | | | | | unverified |
| Enter (bulkgrab.go:93) | GRAB (bulk, via SELECT) · week-day grid | | | | | unverified |
| Esc (bulkgrab.go:95) | GRAB (bulk, via SELECT) · task tree | | | | | unverified |
| Esc (bulkgrab.go:95) | GRAB (bulk, via SELECT) · month grid | | | | | unverified |
| Esc (bulkgrab.go:95) | GRAB (bulk, via SELECT) · week-day grid | | | | | unverified |
| h l / Left Right (bulkgrab.go:97-100,107-110) | GRAB (bulk, via SELECT) · task tree | | | | | unverified |
| h l / Left Right (bulkgrab.go:97-100,107-110) | GRAB (bulk, via SELECT) · month grid | | | | | unverified |
| h l / Left Right (bulkgrab.go:97-100,107-110) | GRAB (bulk, via SELECT) · week-day grid | | | | | unverified |
| j k / Up Down (bulkgrab.go:101-104,111-114) | GRAB (bulk, via SELECT) · task tree | | | | | unverified |
| j k / Up Down (bulkgrab.go:101-104,111-114) | GRAB (bulk, via SELECT) · month grid | | | | | unverified |
| j k / Up Down (bulkgrab.go:101-104,111-114) | GRAB (bulk, via SELECT) · week-day grid | | | | | unverified |
| J K (inert) (bulkgrab.go:115-117) | GRAB (bulk, via SELECT) · task tree | | | | | unverified |
| J K (inert) (bulkgrab.go:115-117) | GRAB (bulk, via SELECT) · month grid | | | | | unverified |
| J K (inert) (bulkgrab.go:115-117) | GRAB (bulk, via SELECT) · week-day grid | | | | | unverified |
| Esc (selection.go:332-335) | SELECT · task tree | | | | | unverified |
| Esc (selection.go:332-335) | SELECT · month grid | | | | | unverified |
| Esc (selection.go:332-335) | SELECT · week-day grid | | | | | unverified |
| h j k l / arrows (extend, unmodified) (selection.go:336-339,345-346) | SELECT · task tree | | | | | unverified |
| h j k l / arrows (extend, unmodified) (selection.go:336-339,345-346) | SELECT · month grid | | | | | unverified |
| h j k l / arrows (extend, unmodified) (selection.go:336-339,345-346) | SELECT · week-day grid | | | | | unverified |
| gg (selection.go:358-359 (passthrough; resolvePrefix gates to gg only)) | SELECT · task tree | | | | | unverified |
| gg (selection.go:358-359 (passthrough; resolvePrefix gates to gg only)) | SELECT · month grid | | | | | unverified |
| gg (selection.go:358-359 (passthrough; resolvePrefix gates to gg only)) | SELECT · week-day grid | | | | | unverified |
| G (selection.go:345) | SELECT · task tree | | | | | unverified |
| G (selection.go:345) | SELECT · month grid | | | | | unverified |
| G (selection.go:345) | SELECT · week-day grid | | | | | unverified |
| f (selection.go:345) | SELECT · task tree | | | | | unverified |
| f (selection.go:345) | SELECT · month grid | | | | | unverified |
| f (selection.go:345) | SELECT · week-day grid | | | | | unverified |
| b (selection.go:345) | SELECT · task tree | | | | | unverified |
| b (selection.go:345) | SELECT · month grid | | | | | unverified |
| b (selection.go:345) | SELECT · week-day grid | | | | | unverified |
| 1-9 / 0 (count, conditional) (selection.go:347-357) | SELECT · task tree | | | | | unverified |
| 1-9 / 0 (count, conditional) (selection.go:347-357) | SELECT · month grid | | | | | unverified |
| 1-9 / 0 (count, conditional) (selection.go:347-357) | SELECT · week-day grid | | | | | unverified |
| V (selection.go:360-363) | SELECT · task tree | | | | | unverified |
| V (selection.go:360-363) | SELECT · month grid | | | | | unverified |
| V (selection.go:360-363) | SELECT · week-day grid | | | | | unverified |
| Space (selection.go:364-366) | SELECT · task tree | | | | | unverified |
| Space (selection.go:364-366) | SELECT · month grid | | | | | unverified |
| Space (selection.go:364-366) | SELECT · week-day grid | | | | | unverified |
| d (selection.go:367-369) | SELECT · task tree | | | | | unverified |
| d (selection.go:367-369) | SELECT · month grid | | | | | unverified |
| d (selection.go:367-369) | SELECT · week-day grid | | | | | unverified |
| y (selection.go:370-372) | SELECT · task tree | | | | | unverified |
| y (selection.go:370-372) | SELECT · month grid | | | | | unverified |
| y (selection.go:370-372) | SELECT · week-day grid | | | | | unverified |
| Y (selection.go:373-375) | SELECT · task tree | | | | | unverified |
| Y (selection.go:373-375) | SELECT · month grid | | | | | unverified |
| Y (selection.go:373-375) | SELECT · week-day grid | | | | | unverified |
| m (selection.go:376-378) | SELECT · task tree | | | | | unverified |
| m (selection.go:376-378) | SELECT · month grid | | | | | unverified |
| m (selection.go:376-378) | SELECT · week-day grid | | | | | unverified |
| Enter (keys.go:390) | RESIZE (global sub-mode) | | | | | unverified |
| Ctrl-W (keys.go:390) | RESIZE (global sub-mode) | | | | | unverified |
| Esc (keys.go:392) | RESIZE (global sub-mode) | | | | | unverified |
| Left / Right (arrows) (keys.go:394-397) | RESIZE (global sub-mode) | | | | | unverified |
| h (keys.go:400-401) | RESIZE (global sub-mode) | | | | | unverified |
| l (keys.go:402-403) | RESIZE (global sub-mode) | | | | | unverified |
| H (keys.go:404-405) | RESIZE (global sub-mode) | | | | | unverified |
| L (keys.go:406-407) | RESIZE (global sub-mode) | | | | | unverified |
| q (keys.go:408-409) | RESIZE (global sub-mode) | | | | | unverified |
| j / Down (forms.go:219-221,237-238) | forms (NORMAL) | | | | | unverified |
| k / Up (forms.go:222-224,239-240) | forms (NORMAL) | | | | | unverified |
| Tab (forms.go:219-221) | forms (NORMAL) | | | | | unverified |
| Shift-Tab (forms.go:222-224) | forms (NORMAL) | | | | | unverified |
| l / Right (button move) (forms.go:225-227,241-242,278-294) | forms (NORMAL) | | | | | unverified |
| h / Left (button move) (forms.go:228-230,243-244,278-294) | forms (NORMAL) | | | | | unverified |
| Enter (forms.go:231-232,257-276) | forms (NORMAL) | | | | | unverified |
| Esc (forms.go:233-234) | forms (NORMAL) | | | | | unverified |
| g (forms.go:245-246) | forms (NORMAL) | | | | | unverified |
| G (forms.go:247-248) | forms (NORMAL) | | | | | unverified |
| Up / Down (forms.go:196-199 (delegated to tview DropDown list)) | forms (NORMAL, dropdown open) | | | | | unverified |
| Enter (forms.go:196-199) | forms (NORMAL, dropdown open) | | | | | unverified |
| Esc (forms.go:196-199) | forms (NORMAL, dropdown open) | | | | | unverified |
| Esc (forms.go:299-301) | forms (DRILL) | | | | | unverified |
| Enter / Tab (commit + advance) (forms.go:302-304) | forms (DRILL) | | | | | unverified |
| Shift-Tab (commit + back) (forms.go:305-307) | forms (DRILL) | | | | | unverified |
| typed chars / cursor keys / Backspace-Delete (forms.go:309) | forms (DRILL) | | | | | unverified |
| Left / Right / h / l (weekday-strip cursor) (weekdaystrip.go:137-158) | forms (DRILL) | | | | | unverified |
| Space (weekday-strip toggle) (weekdaystrip.go:149) | forms (DRILL) | | | | | unverified |
| Esc (help.go:124-129) | modals (help) | | | | | unverified |
| q (help.go:125) | modals (help) | | | | | unverified |
| ? (help.go:125) | modals (help) | | | | | unverified |
| j k / arrows / PgUp PgDn (scroll, tview TextView default) (help.go:129; vendor tview/textview.go:1341-1352) | modals (help) | | | | | unverified |
| Esc (conflicts.go:35-36) | modals (conflicts list) | | | | | unverified |
| q (conflicts.go:35-36) | modals (conflicts list) | | | | | unverified |
| j / k (conflicts.go:42-49) | modals (conflicts list) | | | | | unverified |
| Enter (conflicts.go:64) | modals (conflicts list) | | | | | unverified |
| arrows (native List nav) (vendor tview/list.go default) | modals (conflicts list) | | | | | unverified |
| Left / Right / Tab (button nav) (conflicts.go:70-98 (tview.Modal default)) | modals (conflict resolve dialog) | | | | | unverified |
| Enter (activate button) (conflicts.go:70-98) | modals (conflict resolve dialog) | | | | | unverified |
| Esc (command.go:172 (list.SetDoneFunc)) | modals (account picker) | | | | | unverified |
| j / k (command.go:173-186) | modals (account picker) | | | | | unverified |
| Enter (command.go:164-167) | modals (account picker) | | | | | unverified |
| Esc (colorpicker.go:137-140) | modals (color picker) | | | | | unverified |
| Enter (colorpicker.go:141-142,169-179) | modals (color picker) | | | | | unverified |
| Left / Right / h / l (column move) (colorpicker.go:143-150) | modals (color picker) | | | | | unverified |
| Up / Down / j / k (row move) (colorpicker.go:151-165) | modals (color picker) | | | | | unverified |
| Enter (command.go:28-37) | modals (command line input) | | | | | unverified |
| Esc (command.go:28-37) | modals (command line input) | | | | | unverified |
| Enter (search.go:36-58) | modals (search input) | | | | | unverified |
| Esc (search.go:36-58) | modals (search input) | | | | | unverified |
| Left / Right / Tab (button nav) (generic tview.Modal default (delete confirm, recurrence-scope picker, config-reload notice, etc.)) | modals (generic confirm/choice dialog) | | | | | unverified |
| Enter (activate button) (generic tview.Modal default) | modals (generic confirm/choice dialog) | | | | | unverified |
