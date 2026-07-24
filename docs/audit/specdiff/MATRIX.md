# Key×Context Consistency Matrix

> Purpose: an exhaustive key×context ledger for the LazyPlanner TUI (`internal/ui`), built to
> phase-2's execution method in
> [`docs/superpowers/specs/2026-07-24-v1.5.0-phase2-matrix-execution.md`](../../superpowers/specs/2026-07-24-v1.5.0-phase2-matrix-execution.md).
> Every cell below is a `(key/chord, mode×surface)` pair; a blank/`unverified` cell is the
> exhaustiveness guarantee — nothing can be silently dropped. Later tasks fill in `Actual behavior`,
> `Help bar`, `:help`, `README`, and `Verdict` per row, then triage divergences with the owner.
>
> **Row count: 517 scaffold rows.** Five verification slices (task tree; calendar grids
> NORMAL/DRILL; calendar grids SELECT/GRAB + agenda board; the three overview panels;
> forms/modals/RESIZE) have now filled all 517 — **0 rows remain `Verdict = unverified`**. Raw,
> not-yet-triaged divergences the slices surfaced are collected verbatim in §4 for the owner to
> adjudicate; this is the number that must reach zero before phase 2 closes.

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
| Tab (app.go:815) | NORMAL · task tree | `a.setMode((a.mode+1)%3)` — cycles Calendar→Tasks→Agenda→Calendar; from the tree this leaves Tasks mode for Agenda (app.go:815-817, 708-743) | — (fixed NORMAL hint string, render.go:735, omits Tab/Shift-Tab — curated subset) | "Tab / Shift-Tab — cycle panels" (help.go:20) | "Cycle those three" (README.md:116) | holds |
| Tab (app.go:815) | NORMAL · month grid | `a.setMode((a.mode+1)%3)` — leaves Calendar mode for Tasks; `setMode`'s Calendar case (not entered here) would otherwise call `buildCenterCalendar()`/reset drill (app.go:815-817, 708-743) | — (curated subset, render.go:735) | "Tab / Shift-Tab — cycle panels" (help.go:20) | "Cycle those three" (README.md:116) | holds |
| Tab (app.go:815) | NORMAL · week-day grid | Same — mode-agnostic, doesn't touch `a.timegrid`'s state directly | — | help.go:20 | README.md:116 | holds |
| Tab (app.go:815) | NORMAL · agenda board | `setMode((mode+1)%3)` — Agenda(2)→Calendar(0) (app.go:815-817) | — (curated NORMAL hint, render.go:735, omits Tab) | "Tab / Shift-Tab — cycle panels" (help.go:20) | "Cycle those three" (README.md:116) | holds |
| Tab (app.go:815) | NORMAL · Calendars overview | `setMode((mode+1)%3)` (app.go:815-817) — moves focus + center to **Tasks overview** (a.tasklists), rebuilding the tree. | — (not in the curated hints line) | help.go:20 "Tab / Shift-Tab — cycle panels" | README.md:116 "Cycle those three" | holds |
| Tab (app.go:815) | NORMAL · Tasks overview | Same mechanism — cycles to **Agenda overview** (a.agendaList). | — | help.go:20 | README.md:116 | holds |
| Tab (app.go:815) | NORMAL · Agenda overview | Same mechanism — wraps back to **Calendars overview** (a.calendars). | — | help.go:20 | README.md:116 | holds |
| Shift-Tab (app.go:818) | NORMAL · task tree | `a.setMode((a.mode+2)%3)` — cycles the other direction (app.go:818-820) | — | help.go:20 (same row as Tab) | README.md:116 (same row) | holds |
| Shift-Tab (app.go:818) | NORMAL · month grid | `a.setMode((a.mode+2)%3)` — cycles the other direction (app.go:818-820) | — | help.go:20 | README.md:116 | holds |
| Shift-Tab (app.go:818) | NORMAL · week-day grid | Same | — | help.go:20 | README.md:116 | holds |
| Shift-Tab (app.go:818) | NORMAL · agenda board | `setMode((mode+2)%3)` — Agenda(2)→Tasks(1) (app.go:818-820) | — | help.go:20 | README.md:116 | holds |
| Shift-Tab (app.go:818) | NORMAL · Calendars overview | `setMode((mode+2)%3)` (app.go:818-820) — wraps backward to **Agenda overview**. | — | help.go:20 | README.md:116 | holds |
| Shift-Tab (app.go:818) | NORMAL · Tasks overview | Cycles backward to **Calendars overview**. | — | help.go:20 | README.md:116 | holds |
| Shift-Tab (app.go:818) | NORMAL · Agenda overview | Cycles backward to **Tasks overview**. | — | help.go:20 | README.md:116 | holds |
| Ctrl-W (app.go:821) | NORMAL · task tree | `a.enterResizeMode()` — enters the RESIZE sub-mode (app.go:821-823, keys.go:343-352) | — | "Ctrl-W — resize sub-mode: ←/→ overview · H/L Detail · Enter keep · Esc/q cancel" (help.go:95) | "Ctrl-W \| resize sub-mode (overview + Detail)" (README.md:139) | holds |
| Ctrl-W (app.go:821) | NORMAL · month grid | `a.enterResizeMode()` — enters RESIZE sub-mode; un-collapses accordion first if needed (app.go:821-823, keys.go:343-352) | — | "Ctrl-W — resize sub-mode: ←/→ overview · H/L Detail · Enter keep · Esc/q cancel" (help.go:95) | "Ctrl-W \| resize sub-mode (overview + Detail)" (README.md:139) | holds |
| Ctrl-W (app.go:821) | NORMAL · week-day grid | Same | — | help.go:95 | README.md:139 | holds |
| Ctrl-W (app.go:821) | NORMAL · agenda board | `enterResizeMode()`, mode-independent (app.go:821-823) | — | help.go:95 | README.md:139 | holds |
| Ctrl-W (app.go:821) | NORMAL · Calendars overview | `enterResizeMode()` (keys.go:343-352) — enters the modal RESIZE sub-mode; identical regardless of which overview/pane was focused (`handleResizeKey`, keys.go:388-413, only ever touches `leftCol`/`detail` widths). | "RESIZE · ←/→ overview · H/L detail · Enter keep · Esc/q cancel" (flash on entry, keys.go:351) then `a.hints` shows the same via a dedicated RESIZE hint — actually the generic line stays until the next `updateStatus`; the flash is the immediate feedback. | help.go:95 "Ctrl-W — resize sub-mode: ←/→ overview · H/L Detail · Enter keep · Esc/q cancel" | README.md:139, 144 (RESIZE keys listed under handleResizeKey's own bindings, not this table row directly — see `README.md:139`) | holds |
| Ctrl-W (app.go:821) | NORMAL · Tasks overview | Same — RESIZE is context-independent by design (MATRIX.md §2.2 "RESIZE × any specific surface" dropped combination). | same | help.go:95 | README.md:139 | holds |
| Ctrl-W (app.go:821) | NORMAL · Agenda overview | Same. | same | help.go:95 | README.md:139 | holds |
| Ctrl-Left (app.go:824) | NORMAL · task tree | `a.resizeLeft(-leftWidthStep)` — narrows the overview column (app.go:824-828, keys.go:311-322) | — | "Ctrl-← / Ctrl-→ — narrow / widen the overview column" (help.go:94) | "Ctrl-← / Ctrl-→ \| Narrow / widen the overview column" (README.md:139) | holds |
| Ctrl-Left (app.go:824) | NORMAL · month grid | `a.resizeLeft(-leftWidthStep)` — narrows the overview column (app.go:824-828, keys.go:311-322) | — | "Ctrl-← / Ctrl-→ — narrow / widen the overview column" (help.go:94) | "Ctrl-← / Ctrl-→ \| Narrow / widen the overview column" (README.md:139) | holds |
| Ctrl-Left (app.go:824) | NORMAL · week-day grid | Same | — | help.go:94 | README.md:139 | holds |
| Ctrl-Left (app.go:824) | NORMAL · agenda board | `resizeLeft(-step)`, intercepted before focus dispatch, mode-independent (app.go:824-828) | — | help.go:94 | README.md:139 | holds |
| Ctrl-Left (app.go:824-828) | NORMAL · Calendars overview | `resizeLeft(-leftWidthStep)` (keys.go:311-322) — narrows the overview column directly, no modal, no focus dependency; no-op if `a.accordion` is on. | — | help.go:94 "Ctrl-← / Ctrl-→ — narrow/widen overview" | README.md:139 | holds |
| Ctrl-Left (app.go:824-828) | NORMAL · Tasks overview | Same. | — | help.go:94 | README.md:139 | holds |
| Ctrl-Left (app.go:824-828) | NORMAL · Agenda overview | Same. | — | help.go:94 | README.md:139 | holds |
| Ctrl-Right (app.go:829) | NORMAL · task tree | `a.resizeLeft(+leftWidthStep)` — widens the overview column (app.go:829-833) | — | help.go:94 (same row) | README.md:139 (same row) | holds |
| Ctrl-Right (app.go:829) | NORMAL · month grid | `a.resizeLeft(+leftWidthStep)` — widens the overview column (app.go:829-833) | — | help.go:94 | README.md:139 | holds |
| Ctrl-Right (app.go:829) | NORMAL · week-day grid | Same | — | help.go:94 | README.md:139 | holds |
| Ctrl-Right (app.go:829) | NORMAL · agenda board | `resizeLeft(+step)` (app.go:829-833) | — | help.go:94 | README.md:139 | holds |
| Ctrl-Right (app.go:829-833) | NORMAL · Calendars overview | `resizeLeft(+leftWidthStep)` — widens the overview column. | — | help.go:94 | README.md:139 | holds |
| Ctrl-Right (app.go:829-833) | NORMAL · Tasks overview | Same. | — | help.go:94 | README.md:139 | holds |
| Ctrl-Right (app.go:829-833) | NORMAL · Agenda overview | Same. | — | help.go:94 | README.md:139 | holds |
| 1-9 / 0 (count prefix) (app.go:787-794) | NORMAL · task tree | Accumulates `a.pendingCount` (capped at `maxCount`=999); applies to the next motion (app.go:787-799, keys.go:139) — surface-agnostic | — | "3j 5k … — count prefix — repeat a motion" (help.go:24) | "`<count>` + motion \| Repeat a motion — `3j`, `5k`" (README.md:118) | holds |
| 1-9 / 0 (count prefix) (app.go:787-794) | NORMAL · month grid | Accumulates `a.pendingCount` (capped 999); a bare `0` only extends an already-nonzero count. Applies to the next motion (hjkl/arrows via `repeatKey`) or to `G` (`gotoBottom`); an *undrilled* grid ignores the count for `G` and just lands on the last day (`gotoBottom`'s `count>0` branch only fires for a `calGrid` that's already `active`-drilled, `keys.go:238-270,260-267`) | — | "3j 5k … — count prefix — repeat a motion" (help.go:24) | "`<count>` + motion \| Repeat a motion — `3j`, `5k`" (README.md:118) | holds |
| 1-9 / 0 (count prefix) (app.go:787-794) | NORMAL · week-day grid | Same mechanics — count feeds `repeatKey`/`gotoBottom` identically via the shared `calGrid` interface | — | help.go:24 | README.md:118 | holds |
| 1-9 / 0 (count prefix) (app.go:787-794) | NORMAL · agenda board | Accumulates `pendingCount`, applies to next motion fed to `agendaList` (app.go:787-799) | — | "3j 5k …" (help.go:24) | README.md:118 | holds |
| 1-9 / 0 (count prefix) (app.go:787-794) | NORMAL · Calendars overview | Accumulates into `a.pendingCount`, shown in `a.statusLeft` (not the hints bar) as `count N` (app.go:793); consumed by the next motion key against whichever list is focused. | — (shown via statusLeft, not hints) | help.go:24 "3j 5k … — count prefix" | README.md:118 "`<count>` + motion — 3j, 5k" | holds |
| 1-9 / 0 (count prefix) (app.go:787-794) | NORMAL · Tasks overview | Same mechanism, applies to a.tasklists. | — | help.go:24 | README.md:118 | holds |
| 1-9 / 0 (count prefix) (app.go:787-794) | NORMAL · Agenda overview | Same mechanism, applies to a.agendaList. | — | help.go:24 | README.md:118 | holds |
| h j k l / arrows (motion) (keys.go:147-164) | NORMAL · task tree | `motionArrow` translates hjkl to arrow keys; `repeatKey` feeds the translated arrow to `a.tree`'s own `InputHandler` `count` times (app.go:803-813, keys.go:147-182). Note: tview's `TreeView.InputHandler` treats `KeyLeft`/`KeyRight` as vertical move (step ±1, treeview.go:821-826) same as Up/Down — so in the tree, `h`/`l` are functionally synonyms for `k`/`j` (no horizontal fold behavior) | "hjkl move" (render.go:735) | "move the highlight (Enter expands/collapses a tree node)" (help.go:23) | "Move the highlight in the focused pane" (README.md:117) | holds — docs only promise "move the highlight", not distinct left/right semantics, so the h/l≡k/j collapse in the tree isn't a broken promise |
| h j k l / arrows (motion) (keys.go:147-164) | NORMAL · month grid | `motionArrow` translates hjkl→arrow, `repeatKey` feeds it to `a.month`'s `InputHandler` n times; `calendarView.handleDayMode` moves the selected day ±1 (h/l) or ±7 (j/k, i.e. a week) via `onSelectDay` (calendarview.go:95-118). The widget's own rune cases for h/j/k/l (calendarview.go:130-138) never actually fire — raw runes never reach it (see Method notes / observation) | "hjkl move" (render.go:735) | "move the highlight (Enter expands/collapses a tree node)" (help.go:23) | "Move the highlight in the focused pane" (README.md:117) | holds |
| h j k l / arrows (motion) (keys.go:147-164) | NORMAL · week-day grid | Same translation; `timeGridView.handleDayMode` only has `KeyLeft`/`KeyRight` cases (±1 day) — `KeyUp`/`KeyDown` do nothing un-drilled by design (comment: "days are navigated horizontally … you drill in with Enter", timegridview.go:427-428) | "hjkl move" (render.go:735) | help.go:23 | README.md:117 | holds |
| h j k l / arrows (motion) (keys.go:147-164) | NORMAL · agenda board | `j`/`k` (→Down/Up) move `agendaList.currentItem`, driving the board's highlight box. `h`/`l` (→Left/Right) only shift `horizontalOffset` (vendor list.go:628-631) — **no visible highlight movement** | "hjkl move" (render.go:735) | "move the highlight" (help.go:23) | "Move the highlight in the focused pane" (README.md:117) | **code-diverges** (Divergence #2) |
| h j k l / arrows (motion) (keys.go:147-164) | NORMAL · Calendars overview | `motionArrow` translates hjkl to arrows, fed via `repeatKey` into `a.calendars.InputHandler()` (keys.go:169-182). On this `tview.List`, `j`/`k` (→Down/Up) move `currentItem`; `h`/`l` (→Left/Right) only shift `horizontalOffset` (vendor list.go:628-631) — **the highlight itself never moves for h/l**. See Divergence 1. | — | help.go:23 "h j k l / arrows — move the highlight" | README.md:117 "Move the highlight in the focused pane" | doc-stale (h/l don't move the highlight on a flat List — README/help overclaim uniformity) |
| h j k l / arrows (motion) (keys.go:147-164) | NORMAL · Tasks overview | Same List mechanism against `a.tasklists`; j/k move it (rebuilding the tree via `SetChangedFunc`, app.go:602-621), h/l scroll only. | — | help.go:23 | README.md:117 | doc-stale (same reason) |
| h j k l / arrows (motion) (keys.go:147-164) | NORMAL · Agenda overview | Same against `a.agendaList`; j/k move it (updating the board's highlight via `SetChangedFunc`, app.go:624-628), h/l scroll only. | — | help.go:23 | README.md:117 | doc-stale (same reason) |
| gg (keys.go:42,187-195) | NORMAL · task tree | `gotoTop()` — `a.tree` branch selects `visibleTreeNodes(...)[0]` (keys.go:187-195) | — (curated hint omits gg, only shows "hjkl move") | "g g / G — go to top / bottom" (help.go:25) | "Go to top / bottom of the list, tree, or calendar grid" (README.md:119) | holds |
| gg (keys.go:42,187-195) | NORMAL · month grid | `gotoTop()` — not a `List`/`TreeView`, so `repeatKey(KeyHome,1)` → `calendarView.handleDayMode`'s `KeyHome` case jumps to the first day of the first displayed week (calendarview.go:110-113,184-194) | — (curated hint omits gg) | "g g / G — go to top / bottom" (help.go:25) | "Go to top / bottom of the list, tree, or calendar grid" (README.md:119) | holds |
| gg (keys.go:42,187-195) | NORMAL · week-day grid | Same path; `timeGridView.handleDayMode`'s `KeyHome` jumps to the first displayed day (day view: itself; week view: the week's first day) (timegridview.go:429-432) | — | help.go:25 | README.md:119 | holds |
| gg (keys.go:42,187-195) | NORMAL · agenda board | `gotoTop()` — not a TreeView, falls to `repeatKey(Home,1)` → `agendaList` jumps to item 0 (keys.go:187-195) | — | "go to top / bottom" (help.go:25) | README.md:119 | holds |
| gg (keys.go:42,187-195) | NORMAL · Calendars overview | `gotoTop()` — not a TreeView, so falls to `repeatKey(Home)` on the focused List (`a.calendars`), moving to item 0. | — | help.go:25 "g g / G — go to top/bottom" | README.md:119 | holds |
| gg (keys.go:42,187-195) | NORMAL · Tasks overview | Same, targets `a.tasklists`. | — | help.go:25 | README.md:119 | holds |
| gg (keys.go:42,187-195) | NORMAL · Agenda overview | Same, targets `a.agendaList`. | — | help.go:25 | README.md:119 | holds |
| gt (keys.go:43,222-231) | NORMAL · task tree | `gotoToday()` — sets `a.anchor` to today and, since `a.mode != modeCalendar`, calls `a.setMode(modeCalendar)` and returns — i.e. `gt` from the tree **switches out of Tasks mode** into Calendar mode (keys.go:220-231) | — | "Calendar" section: "g t — jump to today" (help.go:89), no mode-switch caveat | "`gt` \| jump to today" (README.md:137), no mode-switch caveat | holds — the code comment (keys.go:220-221) frames the mode switch as the intended meaning of "today" ("implies the calendar"); no doc surface contradicts it, they just don't spell out the side effect |
| gt (keys.go:43,222-231) | NORMAL · month grid | `gotoToday()` — sets `a.anchor` to today; already in Calendar mode, so calls `buildCenterCalendar()` (resets drill — moot, already undrilled) + `refocusCalendar()` (keeps grid focused) (keys.go:220-231) | — | "Calendar" section: "g t — jump to today" (help.go:89) | "`f` / `b` · `gt` \| Forward / back one period · jump to today" (README.md:137) | holds |
| gt (keys.go:43,222-231) | NORMAL · week-day grid | Same, rebuilds the week/day grid centered on today | — | help.go:89 | README.md:137 | holds |
| gt (keys.go:43,222-231) | NORMAL · agenda board | `gotoToday()` — sets `a.anchor`=today; since `mode != modeCalendar`, calls `setMode(modeCalendar)` — **switches out of Agenda** into Calendar mode (keys.go:222-231) | — | "jump to today" (help.go:89, Calendar section) | "jump to today" (README.md:137) | holds (mode-switch side effect isn't spelled out, but "go to today" implying the calendar is the code's own stated rationale, keys.go:220-221, and no doc contradicts it) |
| gt (keys.go:43,222-231) | NORMAL · Calendars overview | `gotoToday()` (keys.go:222-231) — already `modeCalendar`, so it re-anchors on today, rebuilds the grid, and `refocusCalendar()` is a no-op (grid wasn't focused) — focus stays on `a.calendars`. | — | help.go:89 "g t — jump to today" (listed under the "Calendar" section heading) | README.md:137 "`f`/`b` · `gt` — … jump to today" | holds |
| gt (keys.go:43,222-231) | NORMAL · Tasks overview | `a.mode != modeCalendar` → `setMode(modeCalendar)` and returns immediately (keys.go:224-227) — **switches to Calendars overview**, confirming `gt` is the one key that truly works from any pane (per the task brief's own note). | — | help.go:89 (placed under "Calendar" — arguably reads as calendar-only, though the mechanism auto-switches mode; not a factual error, just a mild clarity gap) | README.md:137 | holds |
| gt (keys.go:43,222-231) | NORMAL · Agenda overview | Same — switches to Calendars overview. | — | help.go:89 | README.md:137 | holds |
| gd (keys.go:44) | NORMAL · task tree | `a.openCommandLine("goto ")` — opens the command line prefilled, mode-agnostic (keys.go:44) | — | "g d — go to date" (help.go:90) | "`gd` \| go to date (smart-parsed)" (README.md:141) | holds |
| gd (keys.go:44) | NORMAL · month grid | `a.openCommandLine("goto ")` — opens the `:` command line prefilled; mode-agnostic, no immediate navigation until Enter (keys.go:44, command.go:266-286) | — | "g d — go to date (smart-parsed)" (help.go:90) | "`:` · `gd` · `?` \| Command line · go to date · help" (README.md:141) | holds |
| gd (keys.go:44) | NORMAL · week-day grid | Same | — | help.go:90 | README.md:141 | holds |
| gd (keys.go:44) | NORMAL · agenda board | `openCommandLine("goto ")`, mode-agnostic modal (keys.go:44); `cmdGoto` itself would switch to Calendar mode on submit (command.go:266-286) | — | "go to date (smart-parsed)" (help.go:90) | README.md:141 | holds |
| gd (keys.go:44) | NORMAL · Calendars overview | `openCommandLine("goto ")` (keys.go:44, command.go:18-42) — opens the `:` modal prefilled, mode/focus-independent. | — | help.go:90 "g d — go to date (smart-parsed)" | README.md:141 "`:` · gd · ? — … go to date …" | holds |
| gd (keys.go:44) | NORMAL · Tasks overview | Same. | — | help.go:90 | README.md:141 | holds |
| gd (keys.go:44) | NORMAL · Agenda overview | Same. | — | help.go:90 | README.md:141 | holds |
| G (app.go:859; keys.go:238-270) | NORMAL · task tree | `gotoBottom(count)` — tree branch selects the last (or count-th) visible node (keys.go:238,248-259) | — | help.go:25 (same row as gg) | README.md:119 (same row, incl. `<count>G` → nth item) | holds |
| G (app.go:859; keys.go:238-270) | NORMAL · month grid | `gotoBottom(count)` — grid isn't drilled, so falls to `repeatKey(KeyEnd,1)` → `calendarView.handleDayMode`'s `KeyEnd` jumps to the last day of the last displayed week, ignoring any count (keys.go:238,260-269, calendarview.go:114-118) | — | help.go:25 (same row as gg) | "`<count>G` → nth item of a list, the tree, or a drilled day" (README.md:119) — correctly scopes count-honoring to a *drilled* day, matching the undrilled no-count behavior here | README.md:119 | holds |
| G (app.go:859; keys.go:238-270) | NORMAL · week-day grid | Same — lands on the last displayed day, count ignored | — | help.go:25 | README.md:119 | holds |
| G (app.go:859; keys.go:238-270) | NORMAL · agenda board | `gotoBottom(count)` — `agendaList` is a `*tview.List` branch: jumps to the last item, or the count-th with a count (keys.go:238-246) | — | help.go:25 (same row as gg) | "`<count>G` → nth item of a list…" (README.md:119) | holds |
| G (app.go:859; keys.go:238-270) | NORMAL · Calendars overview | `gotoBottom(count)` — `a.calendars` is a `*tview.List`, so it sets `currentItem` to the last (or `count`-th) item directly (keys.go:239-245). | — | help.go:25 | README.md:119 "`<count>G` → nth item of a list…" | holds |
| G (app.go:859; keys.go:238-270) | NORMAL · Tasks overview | Same against `a.tasklists`. | — | help.go:25 | README.md:119 | holds |
| G (app.go:859; keys.go:238-270) | NORMAL · Agenda overview | Same against `a.agendaList`. | — | help.go:25 | README.md:119 | holds |
| it (keys.go:32) | NORMAL · task tree | `addTaskQuick()` — quick-add a top-level task into the **highlighted Tasks-overview list** (`taskCreateContext`, edit.go:110-116,146-156), not necessarily related to the tree cursor | which-key popup only (keys.go:547-572), not the persistent hint bar | "i t / i T — add task — quick / full form" (help.go:42) | "`i` … \| Create prefix — t/T task ..." (README.md:122) | holds |
| it (keys.go:32) | NORMAL · month grid | `addTaskQuick()` — targets the **highlighted Tasks-overview list** (`taskCreateContext`→`selectedTasklistID()`, edit.go:110-116,146-156), independent of the calendar grid's own state; flashes "Select a task list first (press t)" if none is highlighted | which-key popup only (keys.go:547-572), not the persistent hint bar | "i t / i T — add task — quick / full form" (help.go:42) | "`i` … \| Create prefix — t/T task ..." (README.md:122) | holds |
| it (keys.go:32) | NORMAL · week-day grid | Same | which-key popup | help.go:42 | README.md:122 | holds |
| it (keys.go:32) | NORMAL · agenda board | `addTaskQuick()` — targets the **highlighted Tasks-overview list** (`selectedTasklistID`, render.go:86-92), independent of the agenda selection | which-key popup only | "i t / i T" (help.go:42) | README.md:122 | holds |
| it (keys.go:32) | NORMAL · Calendars overview | `addTaskQuick` (edit.go:110-116) via `taskCreateContext` — targets whatever tasklist is highlighted in `a.tasklists` (visible, idle-bordered, above/below) regardless of current focus; quick-add prompt opens. | "i… new" (generic, render.go:735) | help.go:42 "i t / i T — add task" | README.md:122 | holds |
| it (keys.go:32) | NORMAL · Tasks overview | Same, and the target list is now also the focused one. | "i… new" | help.go:42 | README.md:122 | holds |
| it (keys.go:32) | NORMAL · Agenda overview | Same — still resolves via `a.tasklists`' highlight, not the agenda. | "i… new" | help.go:42 | README.md:122 | holds |
| iT (keys.go:33) | NORMAL · task tree | `addTaskFull()` — full create form, same target-list resolution as `it` (edit.go:118-125) | which-key popup | help.go:42 | README.md:122 | holds |
| iT (keys.go:33) | NORMAL · month grid | `addTaskFull()` — full form, same target resolution as `it` (edit.go:118-125) | which-key popup | help.go:42 | README.md:122 | holds |
| iT (keys.go:33) | NORMAL · week-day grid | Same | which-key popup | help.go:42 | README.md:122 | holds |
| iT (keys.go:33) | NORMAL · agenda board | `addTaskFull()` — same target resolution | which-key popup | help.go:42 | README.md:122 | holds |
| iT (keys.go:33) | NORMAL · Calendars overview | `addTaskFull` — full create form, same target resolution as `it`. | "i… new" | help.go:42 | README.md:122 | holds |
| iT (keys.go:33) | NORMAL · Tasks overview | Same. | "i… new" | help.go:42 | README.md:122 | holds |
| iT (keys.go:33) | NORMAL · Agenda overview | Same. | "i… new" | help.go:42 | README.md:122 | holds |
| ie (keys.go:34) | NORMAL · task tree | `addEventQuick()` — targets the **highlighted Calendars-overview** calendar and `a.anchor` day (`eventCreateContext`, edit.go:128-134,176-191); reachable from the tree even though it creates a calendar event | which-key popup | "i e / i E — add event — quick / full form" (help.go:43) | README.md:122 | holds |
| ie (keys.go:34) | NORMAL · month grid | `addEventQuick()` — targets the **highlighted Calendars-overview** calendar and `a.anchor` (the grid's currently-selected day) via `eventCreateContext` (edit.go:128-134,177-191) | which-key popup | "i e / i E — add event — quick / full form" (help.go:43) | README.md:122 | holds |
| ie (keys.go:34) | NORMAL · week-day grid | Same; `a.anchor` tracks whichever day is highlighted in the week/day grid | which-key popup | help.go:43 | README.md:122 | holds |
| ie (keys.go:34) | NORMAL · agenda board | `addEventQuick()` — targets the **highlighted Calendars-overview** calendar; base day defaults to **today** in Agenda mode (`eventCreateContext`, edit.go:177-191) | which-key popup | "i e / i E" (help.go:43) | README.md:122 | holds |
| ie (keys.go:34) | NORMAL · Calendars overview | `addEventQuick` (edit.go:128-134) via `eventCreateContext` — targets whatever calendar is highlighted in `a.calendars`; `base` day = `a.anchor`. | "i… new" | help.go:43 "i e / i E — add event" | README.md:122 | holds |
| ie (keys.go:34) | NORMAL · Tasks overview | Same target resolution (still `a.calendars`' highlight, not `a.tasklists`); `base` = `a.anchor` (mode isn't Agenda). | "i… new" | help.go:43 | README.md:122 | holds |
| ie (keys.go:34) | NORMAL · Agenda overview | Same target resolution; `base` = `model.DayStart(a.now)` (today) since `a.mode==modeAgenda` (edit.go:187-189). | "i… new" | help.go:43 | README.md:122 | holds |
| iE (keys.go:35) | NORMAL · task tree | `addEventFull()` — full create form, same context resolution as `ie` (edit.go:136-142) | which-key popup | help.go:43 | README.md:122 | holds |
| iE (keys.go:35) | NORMAL · month grid | `addEventFull()` — full form, same context resolution as `ie` (edit.go:136-142) | which-key popup | help.go:43 | README.md:122 | holds |
| iE (keys.go:35) | NORMAL · week-day grid | Same | which-key popup | help.go:43 | README.md:122 | holds |
| iE (keys.go:35) | NORMAL · agenda board | `addEventFull()` — same context resolution | which-key popup | help.go:43 | README.md:122 | holds |
| iE (keys.go:35) | NORMAL · Calendars overview | `addEventFull` — full form, same target/base rules as `ie`. | "i… new" | help.go:43 | README.md:122 | holds |
| iE (keys.go:35) | NORMAL · Tasks overview | Same. | "i… new" | help.go:43 | README.md:122 | holds |
| iE (keys.go:35) | NORMAL · Agenda overview | Same (today as base). | "i… new" | help.go:43 | README.md:122 | holds |
| is (keys.go:36) | NORMAL · task tree | `addSubtaskQuick()` — subtask under the tree's current node via `subtaskContext`→`currentTarget` (edit.go:158-165,193-215) | which-key popup | "i s / i S — add subtask — quick / full form" (help.go:44) | README.md:122 | holds |
| is (keys.go:36) | NORMAL · month grid | `addSubtaskQuick()` — `subtaskContext()`→`currentTarget()` requires a drilled *task* (edit.go:159-165,197-214); undrilled here, so `currentTarget` returns `ok=false` and it flashes "Select a task to add a subtask under" | which-key popup | "i s / i S — add subtask — quick / full form" (help.go:44) | README.md:122 | holds |
| is (keys.go:36) | NORMAL · week-day grid | Same — undrilled, same flash | which-key popup | help.go:44 | README.md:122 | holds |
| is (keys.go:36) | NORMAL · agenda board | `addSubtaskQuick()` — parent resolved via `currentTarget()`, which for `modeAgenda` reads the highlighted agenda row (`subtaskContext`, edit.go:93-95,197-215); flashes if the highlighted item is an event, not a task | which-key popup | "i s / i S" (help.go:44) | README.md:122 | holds |
| is (keys.go:36) | NORMAL · Calendars overview | `addSubtaskQuick` via `subtaskContext` (edit.go:197-215) → `currentTarget()`; `modeCalendar` case needs a **drilled** item, which by NORMAL's own definition doesn't exist here → `ok=false` → flash "Select a task to add a subtask under" (edit.go:200). No-op. | "i… new" | help.go:44 "i s / i S — add subtask" | README.md:122 | holds (correctly a no-op; matches "select a task" precondition) |
| is (keys.go:36) | NORMAL · Tasks overview | `currentTarget()`'s `modeTasks` case reads `a.tree.GetCurrentNode()` directly (no focus check) — succeeds if the (visible, center-pane) tree has a task selected; creates the subtask in that task's own list. | "i… new" | help.go:44 | README.md:122 | holds |
| is (keys.go:36) | NORMAL · Agenda overview | `currentTarget()`'s `modeAgenda` case reads `a.agendaList.GetCurrentItem()` — succeeds only if the highlighted row is a task (`t.isTodo`); an event row flashes the same "Select a task…" message. | "i… new" | help.go:44 | README.md:122 | holds |
| iS (keys.go:37) | NORMAL · task tree | `addSubtaskFull()` — full create form, same target resolution as `is` (edit.go:167-174) | which-key popup | help.go:44 | README.md:122 | holds |
| iS (keys.go:37) | NORMAL · month grid | `addSubtaskFull()` — same `subtaskContext` gate as `is`; same flash when undrilled (edit.go:167-174) | which-key popup | help.go:44 | README.md:122 | holds |
| iS (keys.go:37) | NORMAL · week-day grid | Same | which-key popup | help.go:44 | README.md:122 | holds |
| iS (keys.go:37) | NORMAL · agenda board | `addSubtaskFull()` — same target resolution | which-key popup | help.go:44 | README.md:122 | holds |
| iS (keys.go:37) | NORMAL · Calendars overview | `addSubtaskFull` — same `subtaskContext` gate as `is`; no-op (nothing drilled). | "i… new" | help.go:44 | README.md:122 | holds |
| iS (keys.go:37) | NORMAL · Tasks overview | Same as `is`, opens the full form. | "i… new" | help.go:44 | README.md:122 | holds |
| iS (keys.go:37) | NORMAL · Agenda overview | Same as `is`, opens the full form (task rows only). | "i… new" | help.go:44 | README.md:122 | holds |
| ic (keys.go:38) | NORMAL · task tree | `showCalendarForm("", 0)` — new calendar form, mode-agnostic (keys.go:38) | which-key popup | "i c / i l — new calendar / task list" (help.go:45) | README.md:122 | holds |
| ic (keys.go:38) | NORMAL · month grid | `showCalendarForm("", 0)` — new-calendar form, mode/grid-agnostic (keys.go:38) | which-key popup | "i c / i l — new calendar / task list" (help.go:45) | README.md:122 | holds |
| ic (keys.go:38) | NORMAL · week-day grid | Same | which-key popup | help.go:45 | README.md:122 | holds |
| ic (keys.go:38) | NORMAL · agenda board | `showCalendarForm("",0)`, mode-agnostic | which-key popup | "i c / i l" (help.go:45) | README.md:122 | holds |
| ic (keys.go:38) | NORMAL · Calendars overview | `showCalendarForm("", 0)` (calendar.go:97) — opens the create-calendar form unconditionally; no target resolution at all. | "i… new" | help.go:45 "i c / i l — new calendar/list" | README.md:122, 96 (Managing Calendars) | holds |
| ic (keys.go:38) | NORMAL · Tasks overview | Same, unconditional. | "i… new" | help.go:45 | README.md:122, 96 | holds |
| ic (keys.go:38) | NORMAL · Agenda overview | Same, unconditional. | "i… new" | help.go:45 | README.md:122, 96 | holds |
| il (keys.go:39) | NORMAL · task tree | `showCalendarForm("", 1)` — new task-list form, mode-agnostic (keys.go:39) | which-key popup | help.go:45 | README.md:122 | holds |
| il (keys.go:39) | NORMAL · month grid | `showCalendarForm("", 1)` — new-list form, mode/grid-agnostic (keys.go:39) | which-key popup | help.go:45 | README.md:122 | holds |
| il (keys.go:39) | NORMAL · week-day grid | Same | which-key popup | help.go:45 | README.md:122 | holds |
| il (keys.go:39) | NORMAL · agenda board | `showCalendarForm("",1)`, mode-agnostic | which-key popup | help.go:45 | README.md:122 | holds |
| il (keys.go:39) | NORMAL · Calendars overview | `showCalendarForm("", 1)` — create-tasklist form, unconditional. | "i… new" | help.go:45 | README.md:122, 96 | holds |
| il (keys.go:39) | NORMAL · Tasks overview | Same. | "i… new" | help.go:45 | README.md:122, 96 | holds |
| il (keys.go:39) | NORMAL · Agenda overview | Same. | "i… new" | help.go:45 | README.md:122, 96 | holds |
| i! (keys.go:87-91) | NORMAL · task tree | Arms `a.pendingForce` for the next `i`-chord continuation, re-renders the which-key hint flagged "(force)" (keys.go:87-91,550-552) — bypasses the unknown-type `[?]` calendar block on the next create | which-key popup shows "(force)" title | "i ! e / i ! t — force-create on an unknown-type [?] calendar" (help.go:46) | prose: "unless you force it with `i!` (e.g. `i!e`)" (README.md:71) — not in the table | holds |
| i! (keys.go:87-91) | NORMAL · month grid | Arms `a.pendingForce` for the next `i`-chord continuation, re-renders the which-key hint flagged "(force)" (keys.go:87-91,550-552); grid/drill-agnostic | which-key popup shows "(force)" | "i ! e / i ! t — force-create on an unknown-type [?] calendar" (help.go:46) | prose only: "unless you force it with `i!`" (README.md:71) — not in the table | holds |
| i! (keys.go:87-91) | NORMAL · week-day grid | Same | which-key popup | help.go:46 | README.md:71 | holds |
| i! (keys.go:87-91) | NORMAL · agenda board | Arms `pendingForce`, re-renders the which-key hint (keys.go:87-91) | which-key popup shows "(force)" | "i ! e / i ! t" (help.go:46) | prose only, not in the table (README.md:71) | **doc-stale** (Divergence #7) |
| i! (keys.go:87-91) | NORMAL · Calendars overview | Arms `a.pendingForce` for the still-pending `i` chord (re-renders the which-key hint with "(force)"); doesn't itself create anything — combines with the object key that follows, bypassing `guardComponent`'s unknown-`[?]`-type block (calendar.go:70-76). | "i… new" (generic; force isn't separately hinted here) | help.go:46 "i ! e / i ! t — force-create on unknown-type" | README.md:71 "force it with `i!`" (table row 122 doesn't spell out `i!` itself) | holds |
| i! (keys.go:87-91) | NORMAL · Tasks overview | Same. | same | help.go:46 | README.md:71 | holds |
| i! (keys.go:87-91) | NORMAL · Agenda overview | Same. | same | help.go:46 | README.md:71 | holds |
| sp (keys.go:52) | NORMAL · task tree | `setPriorityPrompt()` — one-line prompt on `currentTarget()`'s task (quickfield.go:99-118) | — | "s p / s d — quick-set task priority / due date" (help.go:59) | "`s` … \| Quick-set a task field — p priority, d due date" (README.md:124) | holds |
| sp (keys.go:52) | NORMAL · month grid | `setPriorityPrompt()`→`quickTaskTarget()` requires `currentTarget().isTodo`; undrilled here, so flashes "Select a task first" (quickfield.go:81-118) | — | "s p / s d — quick-set task priority / due date" (help.go:59) | "`s` … \| Quick-set a task field — p priority, d due date" (README.md:124) | holds |
| sp (keys.go:52) | NORMAL · week-day grid | Same | — | help.go:59 | README.md:124 | holds |
| sp (keys.go:52) | NORMAL · agenda board | `setPriorityPrompt()` on `currentTarget()`'s task (the highlighted agenda row, if a task) | — | "s p / s d" (help.go:59) | README.md:124 | holds |
| sp (keys.go:52) | NORMAL · Calendars overview | `setPriorityPrompt` (quickfield.go:99-118) → `quickTaskTarget` → `currentTarget()` finds nothing drilled → flash "Select a task first". No-op. | — (not in curated hints) | help.go:59 "s p / s d — quick-set priority/due" | README.md:124 | holds |
| sp (keys.go:52) | NORMAL · Tasks overview | Resolves the tree's current task (focus-independent read of `a.tree`); opens the priority prompt. | — | help.go:59 | README.md:124 | holds |
| sp (keys.go:52) | NORMAL · Agenda overview | Resolves the agenda list's current row if it's a task; opens the prompt (else "Select a task first" for an event row). | — | help.go:59 | README.md:124 | holds |
| sd (keys.go:53) | NORMAL · task tree | `setDuePrompt()` — smart-parsed due prompt, blank clears (quickfield.go:122-151) | — | help.go:59 | README.md:124 | holds |
| sd (keys.go:53) | NORMAL · month grid | `setDuePrompt()` — same `quickTaskTarget` gate; same flash when undrilled (quickfield.go:81-96,122-151) | — | help.go:59 | README.md:124 | holds |
| sd (keys.go:53) | NORMAL · week-day grid | Same | — | help.go:59 | README.md:124 | holds |
| sd (keys.go:53) | NORMAL · agenda board | `setDuePrompt()`, same target | — | help.go:59 | README.md:124 | holds |
| sd (keys.go:53) | NORMAL · Calendars overview | `setDuePrompt` (quickfield.go:122-…) — same gate as `sp`; no-op here. | — | help.go:59 | README.md:124 | holds |
| sd (keys.go:53) | NORMAL · Tasks overview | Resolves the tree's current task; opens the due-date prompt. | — | help.go:59 | README.md:124 | holds |
| sd (keys.go:53) | NORMAL · Agenda overview | Resolves the agenda list's current row if a task. | — | help.go:59 | README.md:124 | holds |
| y (app.go:876) | NORMAL · task tree | `yankTask()`→`setClip(true)` — cuts `currentTarget()`'s task onto the clipboard (yankpaste.go:29-53) | — | "y / Y — cut / copy a task (with its subtree) to the clipboard" (help.go:67) | table row 129 covers `y` (see divergence #4 for the `Y`/`P` gap) | holds |
| y (app.go:876) | NORMAL · month grid | `yankTask()`→`setClip(true)` — requires `currentTarget().isTodo`; undrilled here, so flashes "Select a task to cut (y)" (yankpaste.go:29-53) | — | "y / Y — cut / copy a task (with its subtree) to the clipboard" (help.go:67) | table row covers `y` only (README.md:129) | holds |
| y (app.go:876) | NORMAL · week-day grid | Same | — | help.go:67 | README.md:129 | holds |
| y (app.go:876) | NORMAL · agenda board | `yankTask()`→`setClip(true)` — cuts `currentTarget()`'s task (agenda row) onto the clipboard; flashes if the highlighted item is an event (yankpaste.go:29-53) | — | "y / Y" (help.go:67) | README.md:129 (`y`/`p` combined row) | holds |
| y (app.go:876) | NORMAL · Calendars overview | `yankTask`→`setClip(true)` (yankpaste.go:29-50) → `currentTarget()` finds nothing drilled → flash "Select a task to cut (y)". No-op. | — | help.go:67 "y / Y — cut/copy a task" | README.md:129 (table: `y`/`p` only — see Divergence 3 for `Y`) | holds (for `y` itself) |
| y (app.go:876) | NORMAL · Tasks overview | Cuts the tree's current task onto the clipboard (`a.yankUIDs`), flashes confirmation with its summary. | — | help.go:67 | README.md:129, 77 | holds |
| y (app.go:876) | NORMAL · Agenda overview | Cuts the agenda list's current row if it's a task (else "Select a task to cut (y)" for an event row). | — | help.go:67 | README.md:129, 77 | holds |
| Y (app.go:879) | NORMAL · task tree | `copyTask()`→`setClip(false)` — copies instead of cuts (yankpaste.go:29-53) | — | help.go:67 | **not in the keybindings table** (only in prose, README.md:77) — divergence #4 | doc-stale |
| Y (app.go:879) | NORMAL · month grid | `copyTask()`→`setClip(false)` — same gate; flashes "Select a task to copy (Y)" when undrilled (yankpaste.go:29-53) | — | help.go:67 | **not in the keybindings table** (only prose, README.md:77) — divergence #3 | doc-stale |
| Y (app.go:879) | NORMAL · week-day grid | Same | — | help.go:67 | README.md:77 (prose only) — divergence #3 | doc-stale |
| Y (app.go:879) | NORMAL · agenda board | `copyTask()`→`setClip(false)` | — | help.go:67 | **not in the keybindings table** (only prose, README.md:77) | **doc-stale** (Divergence #8) |
| Y (app.go:879) | NORMAL · Calendars overview | `copyTask`→`setClip(false)` — same gate as `y`; no-op here ("Select a task to copy (Y)"). | — | help.go:67 | README.md: **missing from table**, prose only (README.md:77) | doc-stale (see Divergence 3) |
| Y (app.go:879) | NORMAL · Tasks overview | Copies the tree's current task onto the clipboard (non-destructive). | — | help.go:67 | README.md: missing from table, prose only | doc-stale |
| Y (app.go:879) | NORMAL · Agenda overview | Copies the agenda list's current row if a task. | — | help.go:67 | README.md: missing from table, prose only | doc-stale |
| m (app.go:882) | NORMAL · task tree | `startGrab()` — enters single-item GRAB on `currentTarget()`'s task (grab.go:26-61) | — | "m — grab: move an event ... or nudge a task's due date ... Enter keep, Esc cancel" (help.go:69) | "`m` \| Grab mode: ... nudge a task's due date (`j`/`k` day, `h`/`l` week)" (README.md:130) | holds |
| m (app.go:882) | NORMAL · month grid | `startGrab()`→`currentTarget()`; undrilled here (`ok=false`), so flashes "Nothing selected to grab (m)" — never reaches `beginGrab` (grab.go:26-61) | — | "m — grab: move an event … Enter keep, Esc cancel" (help.go:69) | "`m` \| Grab mode: move an event in time …" (README.md:130) | holds |
| m (app.go:882) | NORMAL · week-day grid | Same | — | help.go:69 | README.md:130 | holds |
| m (app.go:882) | NORMAL · agenda board | `startGrab()` on `currentTarget()`'s agenda row — an event or dated task; enters GRAB (see the `GRAB · agenda board` table below) | — | "m — grab: move an event … or nudge a task's due date …" (help.go:69) | README.md:130 | holds |
| m (app.go:882) | NORMAL · Calendars overview | `startGrab` (grab.go:26-61) → `currentTarget()` finds nothing drilled → flash "Nothing selected to grab (m)". Genuinely can't enter GRAB from here (nothing to resolve). | — | help.go:69 "m — grab…" | README.md:130 | holds |
| m (app.go:882) | NORMAL · Tasks overview | `currentTarget()` resolves the tree's current task (focus-independent) — if it has a due date, **enters GRAB mode** (`beginGrab`, grab.go:64-78) right from the overview list; `a.grabbing=true` and focus stays on `a.tasklists`, but `interactionMode()` now reports GRAB and every subsequent key routes through `handleGrabKey`. See the "Additional finding" note above re: MATRIX §2.2. | Switches to the grab status line (`a.grabStatus()`, render.go:716) once entered. | help.go:69 | README.md:130 | holds (intentional per the "quick-set works wherever a task is selected" design; not flagged as a doc issue since neither doc claims `m` is Tasks-focus-only) |
| m (app.go:882) | NORMAL · Agenda overview | `currentTarget()` resolves the agenda list's current row — a due-dated task or a (non-recurring or scope-picked) event both enter GRAB the same way. | Same grab-status hint once entered. | help.go:69 | README.md:130 | holds |
| p (app.go:885) | NORMAL · task tree | `pasteUnderSelection()` — pastes clipboard under `currentTarget()`'s task (yankpaste.go:57-65) | — | "p / P — paste under the selected task / at the list top level" (help.go:68) | table row 129 covers `p` | holds |
| p (app.go:885) | NORMAL · month grid | `pasteUnderSelection()`→`paste()`: `a.mode == modeCalendar != modeTasks` → flashes "Switch to a task list (t) to paste" before ever checking the clipboard or the grid's drill state (yankpaste.go:57-84) | — | "p / P — paste under the selected task / at the list top level" (help.go:68) | table row covers `p` only (README.md:129) | holds |
| p (app.go:885) | NORMAL · week-day grid | Same | — | help.go:68 | README.md:129 | holds |
| p (app.go:885) | NORMAL · agenda board | `pasteUnderSelection()` → `paste()` gates `a.mode != modeTasks` and flashes "Switch to a task list (t) to paste" — **no-op in Agenda mode** (yankpaste.go:57-84) | — | "p / P — paste under the selected task…" (help.go:68), no mode restriction noted | README.md:129, no mode restriction noted | **doc-stale** (Divergence #9) |
| p (app.go:885) | NORMAL · Calendars overview | `pasteUnderSelection`→`paste` (yankpaste.go:57-93) — gate `a.mode != modeTasks` (yankpaste.go:81-84) fires immediately → flash "Switch to a task list (t) to paste", regardless of clipboard contents. | — | help.go:68 "p / P — paste under/top" | README.md:129 | holds |
| p (app.go:885) | NORMAL · Tasks overview | Mode gate passes; pastes the clipboard under the tree's current task (or at top if clipboard empty triggers the "Nothing on the clipboard" flash instead). | — | help.go:68 | README.md:129 | holds |
| p (app.go:885) | NORMAL · Agenda overview | Same mode gate as Calendars overview — always flashes "Switch to a task list (t) to paste". | — | help.go:68 | README.md:129 | holds |
| P (app.go:888) | NORMAL · task tree | `pasteAtTop()` — pastes at the list's top level (yankpaste.go:68) | — | help.go:68 | **not in the keybindings table** — divergence #4 | doc-stale |
| P (app.go:888) | NORMAL · month grid | `pasteAtTop()`→`paste("")` — same `a.mode != modeTasks` block fires first; identical flash (yankpaste.go:68,81-84) | — | help.go:68 | **not in the keybindings table** (only prose, README.md:77) — divergence #3 | doc-stale |
| P (app.go:888) | NORMAL · week-day grid | Same | — | help.go:68 | README.md:77 (prose only) — divergence #3 | doc-stale |
| P (app.go:888) | NORMAL · agenda board | `pasteAtTop()` → same `paste()` gate — no-op in Agenda mode | — | help.go:68, no restriction noted | **not in the table at all** (Divergence #9) | **doc-stale** (Divergence #9) |
| P (app.go:888) | NORMAL · Calendars overview | `pasteAtTop`→`paste("")` — same mode gate; flash "Switch to a task list (t) to paste". | — | help.go:68 | README.md: missing from table, prose only | doc-stale (see Divergence 3) |
| P (app.go:888) | NORMAL · Tasks overview | Pastes the clipboard at the tasklist's top level. | — | help.go:68 | README.md: missing from table, prose only | doc-stale |
| P (app.go:888) | NORMAL · Agenda overview | Same mode gate; flash "Switch to a task list (t) to paste". | — | help.go:68 | README.md: missing from table, prose only | doc-stale |
| / (app.go:891) | NORMAL · task tree | `openSearch()` — incremental search over the tree's task labels (search.go:22-63,104-113) | "/ find" (render.go:735) | "/ then n / N — search; next / prev match" (help.go:26) | "`/` · `n` / `N` \| Search the current view · next / prev match" (README.md:127) | holds |
| / (app.go:891) | NORMAL · month grid | `openSearch()` — Calendar mode's `default:` branch searches the **Calendars-overview list's names** (`searchWidget`/`searchItems`, search.go:104-113,141-148), not the grid's events/tasks — same for both grid types (see Additional observations) | "/ find" (render.go:735) | "/ then n / N — search; next / prev match" (help.go:26) | "`/` · `n` / `N` \| Search the current view · next / prev match" (README.md:127) | holds |
| / (app.go:891) | NORMAL · week-day grid | Same | "/ find" | help.go:26 | README.md:127 | holds |
| / (app.go:891) | NORMAL · agenda board | `openSearch()` — incremental search over `agendaList`'s item labels (`searchItems`, search.go:104-113,134-140) | "/ find" (render.go:735) | "/ then n / N" (help.go:26) | README.md:127 | holds |
| / (app.go:891) | NORMAL · Calendars overview | `openSearch` (search.go:22-63) opens the `/` input; incremental matches select within `a.calendars` (calendar display names). | "/ find" (render.go:735) | help.go:26 "/ then n / N — search" | README.md:127 | holds |
| / (app.go:891) | NORMAL · Tasks overview | Matches select within the **tree** (`a.tree`, task summaries) — the visible center pane, not the (focused) `a.tasklists` overview list itself. | "/ find" | help.go:26 | README.md:127 | holds |
| / (app.go:891) | NORMAL · Agenda overview | Matches select within `a.agendaList` (its own item labels). | "/ find" | help.go:26 | README.md:127 | holds |
| n (app.go:894) | NORMAL · task tree | `searchNext(1)` — next match (search.go:86-101) | — | help.go:26 | README.md:127 | holds |
| n (app.go:894) | NORMAL · month grid | `searchNext(1)` — advances through calendar-name matches (search.go:86-101) | — | help.go:26 | README.md:127 | holds |
| n (app.go:894) | NORMAL · week-day grid | Same | — | help.go:26 | README.md:127 | holds |
| n (app.go:894) | NORMAL · agenda board | `searchNext(1)` — cycles matches, re-focuses `agendaList` (search.go:86-101) | — | help.go:26 | README.md:127 | holds |
| n (app.go:894) | NORMAL · Calendars overview | `searchNext(1)` (search.go:86-101) — cycles forward through the same calendar-name matches; flashes "no active search" if none yet. | — | help.go:26 | README.md:127 | holds |
| n (app.go:894) | NORMAL · Tasks overview | Cycles forward through tree matches. | — | help.go:26 | README.md:127 | holds |
| n (app.go:894) | NORMAL · Agenda overview | Cycles forward through agenda-list matches. | — | help.go:26 | README.md:127 | holds |
| N (app.go:897) | NORMAL · task tree | `searchNext(-1)` — previous match (search.go:86-101) | — | help.go:26 | README.md:127 | holds |
| N (app.go:897) | NORMAL · month grid | `searchNext(-1)` (search.go:86-101) | — | help.go:26 | README.md:127 | holds |
| N (app.go:897) | NORMAL · week-day grid | Same | — | help.go:26 | README.md:127 | holds |
| N (app.go:897) | NORMAL · agenda board | `searchNext(-1)` | — | help.go:26 | README.md:127 | holds |
| N (app.go:897) | NORMAL · Calendars overview | `searchNext(-1)` — same, backward. | — | help.go:26 | README.md:127 | holds |
| N (app.go:897) | NORMAL · Tasks overview | Same, backward, tree. | — | help.go:26 | README.md:127 | holds |
| N (app.go:897) | NORMAL · Agenda overview | Same, backward, agenda list. | — | help.go:26 | README.md:127 | holds |
| e (app.go:900) | NORMAL · task tree | `editSelected()` — opens the full edit form for `currentTarget()`'s task (recurring todos go through the this/all scope picker) (edit.go:576-626) | "e edit" (render.go:735) | "e — edit selected (full form) ..." (help.go:58) | "`e` \| Edit selected (full form)" (README.md:123) | holds |
| e (app.go:900) | NORMAL · month grid | `editSelected()` — focus isn't `a.calendars`/`a.tasklists`, `currentTarget()` fails undrilled, so it falls to the Calendar-mode fallback: edits the **highlighted Calendars-overview calendar's** name+color via `currentCalendarID()` (edit.go:576-626, esp. 595-607) — a documented "convenience shortcut from the grid" | "e edit" (render.go:735) | "e — edit selected (full form) …" (help.go:58) | "`e` \| Edit selected (full form)" (README.md:123) | holds |
| e (app.go:900) | NORMAL · week-day grid | Same fallback | "e edit" | help.go:58 | README.md:123 | holds |
| e (app.go:900) | NORMAL · agenda board | `editSelected()` — no overview-list special case applies (focus is `agendaList`, not `a.calendars`/`a.tasklists`), falls to `currentTarget()` and opens the full edit form for the highlighted item (edit.go:576-600) | "e edit" (render.go:735) | "e — edit selected…" (help.go:58) | README.md:123 | holds |
| e (app.go:900) | NORMAL · Calendars overview | `editSelected` (edit.go:576-626) — `a.tv.GetFocus()==a.calendars` special-cases first (edit.go:583-588): opens `showCalendarForm` on the **highlighted calendar** (name+color), not any drilled item. | "e edit" (render.go:735) | help.go:58 "e — edit selected…on the Calendars/Tasks pane, edit the calendar/list" | README.md:123, 72 | holds |
| e (app.go:900) | NORMAL · Tasks overview | `a.tv.GetFocus()==a.tasklists` special-cases (edit.go:589-593): opens `showCalendarForm` on the **highlighted tasklist**. | "e edit" | help.go:58 | README.md:123, 72 | holds |
| e (app.go:900) | NORMAL · Agenda overview | Neither special-case matches (`a.tv.GetFocus()==a.agendaList`) → falls to `currentTarget()` (edit.go:595-625) → opens the item form for the highlighted agenda row (task or event; recurring items get the scope picker first). | "e edit" | help.go:58 | README.md:123, 72 | holds |
| d (app.go:903) | NORMAL · task tree | `deleteContextual()` — focus is `a.tree` (not `a.calendars`/`a.tasklists`), so it calls `deleteSelected()` on `currentTarget()`'s task, with subtree confirm (keys.go:125-131, edit.go:422-478) | "d del" (render.go:735) | "d — delete (item; calendar/list when its panel is focused ...)" (help.go:60) | "`d` \| Delete selected item — or the calendar/list when its panel is focused" (README.md:125) | holds |
| d (app.go:903) | NORMAL · month grid | `deleteContextual()` — focus is the grid (not `a.calendars`/`a.tasklists`), so `deleteSelected()`→`currentTarget()` fails undrilled → flashes "Nothing selected to delete" (keys.go:125-132, edit.go:422-427). Unlike `e`, there is **no** calendar-panel fallback for `d` in the grid | "d del" (render.go:735) | "d — delete (item; calendar/list when its panel is focused …)" (help.go:60) | "`d` \| Delete selected item — or the calendar/list when its panel is focused" (README.md:125) | holds |
| d (app.go:903) | NORMAL · week-day grid | Same | "d del" | help.go:60 | README.md:125 | holds |
| d (app.go:903) | NORMAL · agenda board | `deleteContextual()` → focus isn't `a.calendars`/`a.tasklists`, so `deleteSelected()` runs on `currentTarget()` (keys.go:125-131, edit.go:422-442) | "d del" (render.go:735) | "d — delete…" (help.go:60) | README.md:125 | holds |
| d (keys.go:125) | NORMAL · Calendars overview | `deleteContextual` (keys.go:125-132) matches `a.calendars` → `deleteCollection` (calendar.go:277-303) → `promptDeleteCollection`'s type-to-confirm dialog on the **highlighted calendar**. | "d del" (render.go:735) | help.go:60 "d — delete…calendar/list when its panel is focused" | README.md:125, 72, 96 | holds |
| d (keys.go:125) | NORMAL · Tasks overview | Matches `a.tasklists` → deletes the **highlighted tasklist** (type-to-confirm). | "d del" | help.go:60 | README.md:125, 72, 96 | holds |
| d (keys.go:125) | NORMAL · Agenda overview | Focus is `a.agendaList`, matching neither case → falls to `deleteSelected` (edit.go:422-442) → deletes the highlighted **agenda item** (with its subtree if a task-folder), via the ordinary confirm (not the type-to-confirm collection dialog). Never reaches `deleteCollection`'s dead `"Switch to Calendars (1) or Tasks (2)…"` branch — see the code-hygiene note above. | "d del" | help.go:60 | README.md:125, 72, 96 | holds |
| Space (app.go:906-924) | NORMAL · task tree | `a.mode != modeCalendar`, so it calls `toggleComplete()` directly — completes the task, or advances a recurring one (edit.go:289-360) | "Space done/hide" (render.go:735) | "Space — toggle task done ..." (help.go:64) | "`Space` \| Toggle the selected/drilled task done" (README.md:126) | holds |
| Space (app.go:906-924) | NORMAL · month grid | Calendar-mode special case in `globalKeys` itself: `currentTarget()` fails (undrilled) → the `default:` arm fires → `toggleCalendarVisibility()` hides/shows the **highlighted Calendars-overview calendar** (app.go:906-924, keys.go:432-462) | "Space done/hide" (render.go:735) | "Space — toggle task done … or hide/show the highlighted calendar" (help.go:64,87) | "`Space` \| Toggle the selected/drilled task done — or hide/show the highlighted calendar (Calendar view, no task drilled)" (README.md:126) | holds |
| Space (app.go:906-924) | NORMAL · week-day grid | Same | "Space done/hide" | help.go:64,87 | README.md:126 | holds |
| Space (app.go:906-924) | NORMAL · agenda board | `mode != modeCalendar` → straight to `toggleComplete()`: completes the highlighted task, or flashes "Can't complete an event" for an event, "Select a task first" if nothing resolves (edit.go:289-330) | "Space done/hide" (render.go:735) | "Space — toggle task done…" (help.go:64) | README.md:126 | holds |
| Space (app.go:906-924) | NORMAL · Calendars overview | `a.mode==modeCalendar` branch (app.go:912-920): `currentTarget()` finds nothing drilled → `toggleCalendarVisibility()` (keys.go:435-446) toggles the **highlighted calendar**'s hidden flag and persists it. | "Space done/hide" (render.go:735) | help.go:64, 87 "Space — toggle task done (hide/show calendar in Calendar view)" | README.md:126, 75 | holds |
| Space (app.go:906-924) | NORMAL · Tasks overview | Falls to the `else` branch → `toggleComplete()` (edit.go:289-360) → `currentTarget()` reads the **tree's** current node directly (focus-independent) → toggles that task's completion (or advances it, if recurring). | "Space done/hide" | help.go:64, 70 | README.md:126, 75, 83 | holds |
| Space (app.go:906-924) | NORMAL · Agenda overview | Same `else` branch → `toggleComplete()` → `currentTarget()` reads `a.agendaList`'s current row → toggles it if a task, flashes "Can't complete an event" if not. | "Space done/hide" | help.go:64 | README.md:126, 75 | holds |
| u (app.go:925) | NORMAL · task tree | `undoLast()` — reverts the last undo step, re-selects `step.selUID` (edit.go:698-726) | "u undo" (render.go:735) | "u — undo last local change" (help.go:72) | "`u` \| Undo last local change (this session)" (README.md:133) | holds |
| u (app.go:925) | NORMAL · month grid | `undoLast()` — reverts the last undo step; grid already undrilled, so `refresh()`'s drill-preserve branch is moot (edit.go:698-726,746-766) | "u undo" (render.go:735) | "u — undo last local change" (help.go:72) | "`u` \| Undo last local change (this session)" (README.md:133) | holds |
| u (app.go:925) | NORMAL · week-day grid | Same | "u undo" | help.go:72 | README.md:133 | holds |
| u (app.go:925) | NORMAL · agenda board | `undoLast()`, mode-agnostic (edit.go:698) | "u undo" (render.go:735) | "u — undo last local change" (help.go:72) | README.md:133 | holds |
| u (app.go:925) | NORMAL · Calendars overview | `undoLast` (edit.go:698-726) — pops `a.undo` unconditionally, mode/focus-independent. | "u undo" (render.go:735) | help.go:72 "u — undo last local change" | README.md:133 | holds |
| u (app.go:925) | NORMAL · Tasks overview | Same. | "u undo" | help.go:72 | README.md:133 | holds |
| u (app.go:925) | NORMAL · Agenda overview | Same. | "u undo" | help.go:72 | README.md:133 | holds |
| r (app.go:928) | NORMAL · task tree | `triggerSync()` — background two-way sync, alias for `:sync` (sync.go:14-25) | "r sync" (render.go:735) | "r — sync now (= :sync)" (help.go:98) | "`r` \| Sync now (= `:sync`)" (README.md:140) | holds |
| r (app.go:928) | NORMAL · month grid | `triggerSync()` — background two-way sync, alias for `:sync`; grid-agnostic | "r sync" (render.go:735) | "r — sync now (= :sync)" (help.go:98) | "`r` \| Sync now (= `:sync`)" (README.md:140) | holds |
| r (app.go:928) | NORMAL · week-day grid | Same | "r sync" | help.go:98 | README.md:140 | holds |
| r (app.go:928) | NORMAL · agenda board | `triggerSync()` (sync.go:14), alias for `:sync` | "r sync" (render.go:735) | "r — sync now (= :sync)" (help.go:98) | README.md:140 | holds |
| r (app.go:928) | NORMAL · Calendars overview | `triggerSync` (sync.go:14-30) — global, no context dependency; flashes "Sync not configured" if no server. | "r sync" (render.go:735) | help.go:98 "r — sync now (= :sync)" | README.md:140 | holds |
| r (app.go:928) | NORMAL · Tasks overview | Same. | "r sync" | help.go:98 | README.md:140 | holds |
| r (app.go:928) | NORMAL · Agenda overview | Same. | "r sync" | help.go:98 | README.md:140 | holds |
| : (app.go:932) | NORMAL · task tree | `openCommandLine("")` — opens the command line | ": cmd" (render.go:735) | "`: ` — cmd — :sync :view :goto ..." (help.go:99) | "`:` · `gd` · `?` \| Command line · go to date · help" (README.md:141) | holds |
| : (app.go:932) | NORMAL · month grid | `openCommandLine("")` — opens the `:` command line | ": cmd" (render.go:735) | "`: ` — cmd — :sync :view :goto …" (help.go:99) | "`:` · `gd` · `?` \| Command line · go to date · help" (README.md:141) | holds |
| : (app.go:932) | NORMAL · week-day grid | Same | ": cmd" | help.go:99 | README.md:141 | holds |
| : (app.go:932) | NORMAL · agenda board | `openCommandLine("")`, mode-agnostic | ": cmd" (render.go:735) | "` : ` — cmd — :sync :view …" (help.go:99) | README.md:141 | holds |
| : (app.go:932) | NORMAL · Calendars overview | `openCommandLine("")` (command.go:18-42) — opens the `:` modal, context-independent. | ": cmd" (render.go:735) | help.go:99 | README.md:141 | holds |
| : (app.go:932) | NORMAL · Tasks overview | Same. | ": cmd" | help.go:99 | README.md:141 | holds |
| : (app.go:932) | NORMAL · Agenda overview | Same. | ": cmd" | help.go:99 | README.md:141 | holds |
| ? (app.go:935) | NORMAL · task tree | `showHelp()` — opens the `:help` overlay (help.go:110-135) | "? help" (render.go:735) | self-referential (this is the overlay) | README.md:141 | holds |
| ? (app.go:935) | NORMAL · month grid | `showHelp()` — opens the `:help` overlay (help.go:110-135) | "? help" (render.go:735) | self-referential | README.md:141 | holds |
| ? (app.go:935) | NORMAL · week-day grid | Same | "? help" | — | README.md:141 | holds |
| ? (app.go:935) | NORMAL · agenda board | `showHelp()`, mode-agnostic | "? help" (render.go:735) | "? — this help" (help.go:105) | README.md:141 | holds |
| ? (app.go:935) | NORMAL · Calendars overview | `showHelp` (help.go:110-135) — opens the cheat-sheet overlay, context-independent. | "? help" (render.go:735) | help.go:105 (self-referential "this help") | README.md:141, 92 | holds |
| ? (app.go:935) | NORMAL · Tasks overview | Same. | "? help" | help.go:105 | README.md:141, 92 | holds |
| ? (app.go:935) | NORMAL · Agenda overview | Same. | "? help" | help.go:105 | README.md:141, 92 | holds |
| + (app.go:938-944) | NORMAL · task tree | Not `timeGridActive()` (mode is Tasks) → `setAccordion(true)` — collapses the overview + Detail panes, focuses `a.tree` (keys.go:511-533) | — | "week/day: zoom hour height in/out · 0 = auto-fit ... else: +/- collapse / restore the overview and Detail (accordion)" (help.go:93) | "`+` / `-` / `0` \| Accordion collapse / restore overview + Detail · in week/day: zoom hour height" (README.md:138) | holds |
| + (app.go:938-944) | NORMAL · month grid | `timeGridActive()` is false (viewMonth) → `setAccordion(true)` — collapses overview + Detail, focuses `a.month` (app.go:938-944, keys.go:464-468,506-533) | — (curated hint omits +/-/0 entirely) | "week/day: zoom hour height … else: +/- collapse / restore the overview and Detail (accordion)" (help.go:93) | "`+` / `-` / `0` \| Accordion collapse / restore overview + Detail · in week/day: zoom hour height" (README.md:138) | holds |
| + (app.go:938-944) | NORMAL · week-day grid | `timeGridActive()` is true → `zoomHour(1)` — grows the hour-row height, remembered in `a.hourRows` (keys.go:470-491) | — | help.go:93 | README.md:138 | holds |
| + (app.go:938-944) | NORMAL · agenda board | `timeGridActive()` is false (mode≠Calendar) → `setAccordion(true)`, which explicitly refuses in Agenda mode: flashes "Expand isn't available in Agenda", no layout change (keys.go:511-519) | not in the persistent hint | "+ / - / 0 … +/- collapse / restore" (help.go:93), no Agenda exception noted | "Accordion collapse / restore" (README.md:138), no Agenda exception noted | **doc-stale** (Divergence #10) |
| + (app.go:938-944) | NORMAL · Calendars overview | If `viewMode` is week/day: `zoomHour(1)` (keys.go:475-491) grows the (currently unfocused but visible) time-grid's hour rows. If `viewMode` is month (the default): `setAccordion(true)` (keys.go:511-533) collapses the overview+Detail and moves focus into the grid. | — (not in curated hints; Layout keys aren't listed there) | help.go:93 "+ / - / 0 — week/day: zoom hour height…else: collapse/restore" | README.md:138 | holds |
| + (app.go:938-944) | NORMAL · Tasks overview | `timeGridActive()` is false unconditionally (`a.mode!=modeCalendar`) → always `setAccordion(true)`, moving focus to `a.tree` (`mainPrimitive`, keys.go:537-542). | — | help.go:93 | README.md:138 | holds |
| + (app.go:938-944) | NORMAL · Agenda overview | `setAccordion(true)` is blocked in Agenda mode (keys.go:515-518, flash "Expand isn't available in Agenda") — no-op. | — | help.go:93 | README.md:138 | holds |
| - (app.go:945-951) | NORMAL · task tree | `setAccordion(false)` — restores the overview + Detail (keys.go:511-533) | — | help.go:93 | README.md:138 | holds |
| - (app.go:945-951) | NORMAL · month grid | `setAccordion(false)` — restores overview + Detail (keys.go:506-533) | — | help.go:93 | README.md:138 | holds |
| - (app.go:945-951) | NORMAL · week-day grid | `zoomHour(-1)` — shrinks the hour-row height (keys.go:470-491) | — | help.go:93 | README.md:138 | holds |
| - (app.go:945-951) | NORMAL · agenda board | `setAccordion(false)` — unconditional, but Agenda always starts un-collapsed (`setMode` restores it on every mode entry, app.go:716-724), so this is a harmless no-op that just re-focuses `a.focusForMode()` | not in the persistent hint | help.go:93 (same row) | README.md:138 (same row) | holds |
| - (app.go:945-951) | NORMAL · Calendars overview | Mirrors `+`: `zoomHour(-1)` if week/day, else `setAccordion(false)` (restores the panes if already collapsed, otherwise a no-op). | — | help.go:93 | README.md:138 | holds |
| - (app.go:945-951) | NORMAL · Tasks overview | `setAccordion(false)` — no-op unless already collapsed. | — | help.go:93 | README.md:138 | holds |
| - (app.go:945-951) | NORMAL · Agenda overview | `setAccordion(false)` — same (the `on && modeAgenda` guard only blocks collapsing, not restoring). | — | help.go:93 | README.md:138 | holds |
| 0 (bare) (app.go:952-958) | NORMAL · task tree | Not `timeGridActive()` → falls through the `case '0'` with no return, then `return ev` at the end of `globalKeys` (app.go:952-958,1018); reaches `a.tree`'s `InputHandler`, which has no `'0'` case (treeview.go:839-857) — true no-op | — | help.go:93 (only describes `0` in the week/day zoom context) | README.md:138 (same) | holds — silent no-op in a context where no doc claims it does anything |
| 0 (bare) (app.go:952-958) | NORMAL · month grid | Not `timeGridActive()` → the `case '0'` has no `return` inside its `if`, falls out of the switch, `globalKeys` returns `ev` unhandled (app.go:952-958,1018); `calendarView.handleDayMode` has no `'0'` case — true no-op | — | help.go:93 (only describes `0` in the week/day zoom context) | README.md:138 (same) | holds — silent no-op in a context no doc claims otherwise for |
| 0 (bare) (app.go:952-958) | NORMAL · week-day grid | `timeGridActive()` true → `resetHourZoom()` — returns to auto-fit hour height, remembered (keys.go:493-504) | — | help.go:93 | README.md:138 | holds |
| 0 (bare) (app.go:952-958) | NORMAL · agenda board | `timeGridActive()` false → no case matches, falls to `return ev` (app.go:1018); `agendaList`'s `InputHandler` sees rune `'0'`, checks item shortcuts (none set — `AddItem(..., 0, nil)`, render.go:78,82, rune-zero not `'0'`) → no visible effect | not mentioned (0's only documented use is week/day hour-zoom) | "0 = auto-fit" scoped to week/day (help.go:93) | "0 = auto-fit" scoped to week/day (README.md:138) | holds |
| 0 (bare) (app.go:952-958) | NORMAL · Calendars overview | If `viewMode` week/day: `resetHourZoom()` (keys.go:495-504) — resets to auto-fit, visible in the (unfocused) grid. If month: falls out of the switch unhandled, forwarded to `a.calendars`' `InputHandler`; rune `'0'` matches no item `Shortcut` (all `AddItem` calls pass shortcut `0`, vendor list.go:658-673) → true no-op. | — | help.go:93 ("0 = auto-fit" scoped to week/day; silent elsewhere, matching code) | README.md:138 | holds |
| 0 (bare) (app.go:952-958) | NORMAL · Tasks overview | `timeGridActive()` always false here → forwarded to `a.tasklists`, no shortcut match → no-op. | — | help.go:93 | README.md:138 | holds |
| 0 (bare) (app.go:952-958) | NORMAL · Agenda overview | Same — forwarded to `a.agendaList`, no-op. | — | help.go:93 | README.md:138 | holds |
| [ (app.go:994) | NORMAL · task tree | `cycleCalendar(-1)` — moves the Calendars-overview highlight, usable from any pane (app.go:994-996,1068-1075) | "[ ] cal" (render.go:735) | (Panels & navigation doesn't list it; not found as its own help.go row) — | "`[` / `]` \| Cycle the highlighted calendar (any mode)" (README.md:135) | holds |
| [ (app.go:994) | NORMAL · month grid | `cycleCalendar(-1)` — moves the Calendars-overview highlight (wrapping); doesn't touch the grid at all, so has no visible effect on grid content itself (only which calendar is highlighted for the next `ic`/Space/etc.) (app.go:994-996,1068-1075) | "[ ] cal" (render.go:735) | not its own help.go row | "`[` / `]` \| Cycle the highlighted calendar (any mode)" (README.md:135) | holds |
| [ (app.go:994) | NORMAL · week-day grid | Same | "[ ] cal" | — | README.md:135 | holds |
| [ (app.go:994) | NORMAL · agenda board | `cycleCalendar(-1)` — moves the Calendars-overview highlight, mode-agnostic (app.go:1063-1075) | "[ ] cal" (render.go:735) | "[ / ] — cycle highlighted calendar (any mode)" (help.go:21) | "Cycle the highlighted calendar (any mode)" (README.md:135) | holds |
| [ (app.go:994) | NORMAL · Calendars overview | `cycleCalendar(-1)` (app.go:1068-1075) — moves `a.calendars`' highlight back one (wrapping); this is the focused, on-screen list. | "[ ] cal" (render.go:735) | help.go:21 "[ / ] — cycle highlighted calendar (any mode)" | README.md:135 "(any mode)" | holds |
| [ (app.go:994) | NORMAL · Tasks overview | Same call, moving the (visible, idle-bordered) Calendars box's highlight while Tasks overview holds focus — no `SetChangedFunc` side effect on `a.calendars` (none registered), so this is a pure highlight move. | "[ ] cal" | help.go:21 | README.md:135 | holds |
| [ (app.go:994) | NORMAL · Agenda overview | Same. | "[ ] cal" | help.go:21 | README.md:135 | holds |
| ] (app.go:997) | NORMAL · task tree | `cycleCalendar(1)` (app.go:997-999) | "[ ] cal" (render.go:735) | — | README.md:135 | holds |
| ] (app.go:997) | NORMAL · month grid | `cycleCalendar(1)` (app.go:997-999) | "[ ] cal" | — | README.md:135 | holds |
| ] (app.go:997) | NORMAL · week-day grid | Same | "[ ] cal" | — | README.md:135 | holds |
| ] (app.go:997) | NORMAL · agenda board | `cycleCalendar(1)` | "[ ] cal" (render.go:735) | help.go:21 | README.md:135 | holds |
| ] (app.go:997) | NORMAL · Calendars overview | `cycleCalendar(1)` — forward. | "[ ] cal" | help.go:21 | README.md:135 | holds |
| ] (app.go:997) | NORMAL · Tasks overview | Same, moves the visible-but-idle Calendars box. | "[ ] cal" | help.go:21 | README.md:135 | holds |
| ] (app.go:997) | NORMAL · Agenda overview | Same. | "[ ] cal" | help.go:21 | README.md:135 | holds |
| { (app.go:1000) | NORMAL · task tree | `cycleTasklist(-1)` — moves the Tasks-overview highlight; its changed-callback rebuilds the tree for the new list (app.go:1000-1002,1081-1088, app.go:605-622) | "{ } list" (render.go:735) | — | "`{` / `}` \| Cycle the highlighted task list (any mode)" (README.md:136) | holds |
| { (app.go:1000) | NORMAL · month grid | `cycleTasklist(-1)` — moves the Tasks-overview highlight; irrelevant to Calendar mode's own content but still executes (app.go:1000-1002,1081-1088) | "{ } list" (render.go:735) | — | "`{` / `}` \| Cycle the highlighted task list (any mode)" (README.md:136) | holds |
| { (app.go:1000) | NORMAL · week-day grid | Same | "{ } list" | — | README.md:136 | holds |
| { (app.go:1000) | NORMAL · agenda board | `cycleTasklist(-1)`, mode-agnostic (app.go:1077-1088) | "{ } list" (render.go:735) | "{ / } — cycle highlighted task list (any mode)" (help.go:22) | README.md:136 | holds |
| { (app.go:1000) | NORMAL · Calendars overview | `cycleTasklist(-1)` (app.go:1081-1088) — moves `a.tasklists`' highlight; **its `SetChangedFunc` (app.go:602-621) unconditionally rebuilds `a.tree` for the new list ID**, regardless of `a.mode` — so this also silently updates the (currently not-displayed-in-center) task tree's data, visible once the user switches to Tasks mode. | "{ } list" (render.go:735) | help.go:22 "{ / } — cycle highlighted task list (any mode)" | README.md:136 "(any mode)" | holds |
| { (app.go:1000) | NORMAL · Tasks overview | Same call; here the rebuilt tree is also the visible center pane, so the effect is fully on-screen immediately. | "{ } list" | help.go:22 | README.md:136 | holds |
| { (app.go:1000) | NORMAL · Agenda overview | Same as Calendars overview — moves the visible-but-idle Tasks box and rebuilds the (off-screen-center) tree in the background. | "{ } list" | help.go:22 | README.md:136 | holds |
| } (app.go:1003) | NORMAL · task tree | `cycleTasklist(1)` (app.go:1003-1005) | "{ } list" (render.go:735) | — | README.md:136 | holds |
| } (app.go:1003) | NORMAL · month grid | `cycleTasklist(1)` (app.go:1003-1005) | "{ } list" | — | README.md:136 | holds |
| } (app.go:1003) | NORMAL · week-day grid | Same | "{ } list" | — | README.md:136 | holds |
| } (app.go:1003) | NORMAL · agenda board | `cycleTasklist(1)` | "{ } list" (render.go:735) | help.go:22 | README.md:136 | holds |
| } (app.go:1003) | NORMAL · Calendars overview | `cycleTasklist(1)` — forward, same rebuild side effect. | "{ } list" | help.go:22 | README.md:136 | holds |
| } (app.go:1003) | NORMAL · Tasks overview | Same, on-screen. | "{ } list" | help.go:22 | README.md:136 | holds |
| } (app.go:1003) | NORMAL · Agenda overview | Same, background rebuild. | "{ } list" | help.go:22 | README.md:136 | holds |
| . (app.go:969) | NORMAL · task tree | Toggles `a.showCompleted` and `reloadCurrent()` — rebuilds the tree with/without completed tasks (app.go:969-972,1090-1099) | ". comp:%s" (render.go:735, shows current on/off state) | ". — show/hide completed tasks" (help.go:73) | "`.` \| Show/hide completed tasks" (README.md:142) | holds |
| . (app.go:969) | NORMAL · month grid | Toggles `a.showCompleted`, `reloadCurrent()`→`buildCenterCalendar()` rebuilds the grid with/without completed dated tasks (app.go:969-972,1090-1104) — already undrilled, so the reset-drill side effect of `buildCenterCalendar` is moot here | ". comp:%s" (render.go:735) | ". — show/hide completed tasks" (help.go:73) | "`.` \| Show/hide completed tasks" (README.md:142) | holds |
| . (app.go:969) | NORMAL · week-day grid | Same | ". comp:%s" | help.go:73 | README.md:142 | holds |
| . (app.go:969) | NORMAL · agenda board | `showCompleted = !showCompleted; reloadCurrent()` — rebuilds both `agendaList` and `a.agenda` (render.go:1091-1099) | ". comp:%s" (render.go:735) | ". — show/hide completed tasks" (help.go:73) | README.md:142 | holds |
| . (app.go:969) | NORMAL · Calendars overview | Toggles `a.showCompleted`, calls `reloadCurrent()` (app.go:1091-1104) — for `modeCalendar` this rebuilds only `buildCenterCalendar()` (the grid); the Calendars overview list itself is unaffected (it doesn't list tasks). | ". comp:on/off" (render.go:735, dynamic) | help.go:73 "." — show/hide completed tasks" | README.md:142 | holds |
| . (app.go:969) | NORMAL · Tasks overview | Rebuilds `buildTree()` only — the visible center tree updates; the Tasks overview list of tasklists is unaffected. | ". comp:on/off" | help.go:73 | README.md:142 | holds |
| . (app.go:969) | NORMAL · Agenda overview | Rebuilds **both** `buildAgendaLeft()` (the overview list itself) and `buildAgendaCenter()` (the board) — per the code comment (app.go:1098-1101) Agenda is the one mode where the toggle must refresh both halves together. | ". comp:on/off" | help.go:73 | README.md:142 | holds |
| V (app.go:981; selection.go:51-99) | NORMAL · task tree | `enterSelect()` — `selContext()==selTree`, `a.tv.GetFocus()==a.tree`, anchors on `currentTreeUID()` (selection.go:30-33,51-64) | — | "V — enter SELECT — anchors at the cursor (task tree, ...); needs that pane itself focused" (help.go:76) | "`V` \| SELECT mode: extend a contiguous selection ..." (README.md:131) | holds |
| V (app.go:981; selection.go:51-99) | NORMAL · month grid | `enterSelect()` — `selContext()` returns `selDays` (grid undrilled, `selection.go:34-38`); requires `a.tv.GetFocus()==a.calendarPrimitive()`, anchors `a.selAnchorDay` at the currently-selected day (`selection.go:80-92`) | — | "V — enter SELECT — anchors at the cursor (task tree, calendar days, or a drilled day's items); needs that pane itself focused" (help.go:76) | "`V` \| SELECT mode: extend a contiguous selection … (task tree, calendar days, or a drilled day's items) …" (README.md:131) | holds |
| V (app.go:981; selection.go:51-99) | NORMAL · week-day grid | Same — anchors a day range over the week/day grid | — | help.go:76 | README.md:131 | holds |
| V (app.go:981; selection.go:51-99) | NORMAL · agenda board | `enterSelect()` → `selContext()` switches only on `modeTasks`/`modeCalendar`; `modeAgenda` falls to the `default` case, which flashes "Nothing to select here (SELECT works in the task tree and calendar)" without entering SELECT (selection.go:29-41,93-96) | not in the persistent hint | enumerates only "task tree, calendar days, or a drilled day's items" (help.go:76), implicitly excluding Agenda | same enumeration (README.md:131) | holds (implicit exclusion via exhaustive enumeration matches the actual flash) |
| V (app.go:981; selection.go:51-99) | NORMAL · Calendars overview | `selContext()`→`selDays` (grid not drilled); `enterSelect`'s focus check `a.tv.GetFocus()!=a.calendarPrimitive()` fails (focus is `a.calendars`) → flash "Nothing to select here" (selection.go:81-84). No-op. | — | help.go:76 "V — … needs that pane itself focused, not the Calendars/Tasks overview list" (explicitly documents this!) | README.md:79, 131 (doesn't mention the overview-focus exception explicitly, but doesn't claim it works from there either) | holds |
| V (app.go:981; selection.go:51-99) | NORMAL · Tasks overview | `selContext()`→`selTree`; focus check `a.tv.GetFocus()!=a.tree` fails (focus is `a.tasklists`) → same "Nothing to select here" flash (selection.go:54-57). | — | help.go:76 (same explicit note) | README.md:79, 131 | holds |
| V (app.go:981; selection.go:51-99) | NORMAL · Agenda overview | `selContext()` returns `selNone` (no case for Agenda) → `default` branch's **longer** flash: "Nothing to select here (SELECT works in the task tree and calendar)" (selection.go:93-95). | — | help.go:76 (doesn't call out Agenda specifically, but README/help never claim V works there either — matches MATRIX.md §2.2's "SELECT × agenda board" dropped-combination note) | README.md:79 (silently omits Agenda from V's scope) | holds |
| Esc (app.go:834-838) | NORMAL · task tree | `a.mode == modeTasks` → `a.setFocus(a.tasklists)` — moves focus back to the Tasks overview list regardless of what was focused (app.go:834-838) | "Esc back" (render.go:735) | "Esc — back out (a form/dialog too)" (help.go:28) | "`Esc` \| Back out to the overview · cancel a form/dialog/chord" (README.md:121) | holds |
| Esc (app.go:834-838) | NORMAL · month grid | `a.mode == modeCalendar`, not `modeTasks`, so `globalKeys`'s own `KeyEscape` case doesn't fire (app.go:834-838); falls to `a.month`'s `InputHandler` → `handleDayMode`'s `KeyEscape` → `cv.onExit()` → `a.setFocus(a.calendars)` (calendarview.go:125-128, app.go:580) | "Esc back" (render.go:735) | "Esc — back out (a form/dialog too)" (help.go:28) | "`Esc` \| Back out to the overview · cancel a form/dialog/chord" (README.md:121) | holds |
| Esc (app.go:834-838) | NORMAL · week-day grid | Same path via `tg.onExit()` → `a.setFocus(a.calendars)` (timegridview.go:439-441, app.go:588) | "Esc back" | help.go:28 | README.md:121 | holds |
| Esc (app.go:834-838) | NORMAL · agenda board | Only handles `mode == modeTasks`; Agenda falls through to `return ev` (app.go:1018) → reaches `agendaList`'s `InputHandler`, whose `KeyEscape` case calls `l.done` if non-nil (it's nil — no `SetDoneFunc` on `agendaList`) and returns — a true no-op | "Esc back" (render.go:735) | "Esc — back out (a form/dialog too)" (help.go:28) | "Back out to the overview" (README.md:121) | holds — Agenda's board *is* the overview level (no deeper focus state to leave, unlike the tree/grid), so there is nothing to back out of; this mirrors Esc's equally-inert behavior when focus is already on `a.calendars`/`a.tasklists` in the other modes |
| Esc (app.go:834-838) | NORMAL · Calendars overview | `a.mode!=modeTasks` → the explicit case body does nothing, falls out of the switch, `return ev` at the end of `globalKeys` (app.go:1018) forwards it to `a.calendars`' `InputHandler`; `List`'s Escape case calls `l.done()` only if set — none is (vendor list.go:612-616) → true no-op. | — (not in curated hints) | help.go:28 "Esc — back out (a form/dialog too)" | README.md:121 "Back out to the overview · cancel a form/dialog/chord" | holds (already "at" the overview — nothing further to back out to; doc's "back out to the overview" framing is satisfied vacuously) |
| Esc (app.go:834-838) | NORMAL · Tasks overview | `a.mode==modeTasks` → explicit `a.setFocus(a.tasklists)` (app.go:836-837) — a no-op re-focus since focus is already there (only re-paints border colors). | — | help.go:28 | README.md:121 | holds |
| Esc (app.go:834-838) | NORMAL · Agenda overview | Same pass-through as Calendars overview → forwarded to `a.agendaList`, no `done` func set → no-op. | — | help.go:28 | README.md:121 | holds |
| Enter (widget-dependent (calendarview.go:119; timegridview.go:437; list/tree default)) | NORMAL · task tree | No `case tcell.KeyEnter` in `globalKeys` (app.go:814-1016), so the event reaches `a.tree`'s default `InputHandler`, which toggles `node.SetExpanded` via `SetSelectedFunc` and refreshes the ▸/▾ caret (app.go:640-646, treeview.go:858-859 `selectNode()`) | "Enter open" (render.go:735) | "Enter — dive in / open (a drilled day's events are then cycled with j/k/arrows, not Enter)" (help.go:27) | "`Enter` \| Dive into the center; cycle a day's events; open a list / expand a task" (README.md:120) | holds |
| Enter (widget-dependent (calendarview.go:119; timegridview.go:437; list/tree default)) | NORMAL · month grid | No `KeyEnter` case in `globalKeys`; reaches `calendarView.handleDayMode`'s `KeyEnter` case: drills in (sets `eventMode=true`, `eventIndex=0`, emits the first item) only if the selected day has ≥1 item — else silent no-op (calendarview.go:119-124) | "Enter open" (render.go:735) | "Enter — dive in / open (a drilled day's events are then cycled with j/k/arrows, not Enter)" (help.go:27) | "`Enter` \| Dive into the center; cycle a day's events; open a list / expand a task" (README.md:120) | holds |
| Enter (widget-dependent (calendarview.go:119; timegridview.go:437; list/tree default)) | NORMAL · week-day grid | `timeGridView.handleDayMode`'s `KeyEnter` → `enterEventMode()`, same has-items gate (timegridview.go:437-438,152-162) | "Enter open" | help.go:27 | README.md:120 | holds |
| Enter (widget-dependent (calendarview.go:119; timegridview.go:437; list/tree default)) | NORMAL · agenda board | `agendaList` has no `SetSelectedFunc` and item `Selected` callbacks are `nil` (render.go:78,82) — tview's default `KeyEnter` handling (vendor list.go:648-657) does nothing; Enter is a complete no-op | "Enter open" (render.go:735) | "Enter — dive in / open" (help.go:27), no Agenda exception | "Dive into the center; cycle a day's events; open a list / expand a task" (README.md:120), no Agenda exception | **doc-stale** (Divergence #6) |
| Enter (widget-dependent (calendarview.go:119; timegridview.go:437; list/tree default)) | NORMAL · Calendars overview | `a.calendars.SetSelectedFunc` (app.go:601) fires on Enter (vendor list.go:648-657): `a.setFocus(a.calendarPrimitive())` — dives focus into the month/week/day grid (still NORMAL, not DRILL, until the grid itself is drilled into a day). | "Enter open" (render.go:735) | help.go:27 "Enter — dive in / open" | README.md:120 "Dive into the center…" | holds |
| Enter (widget-dependent (calendarview.go:119; timegridview.go:437; list/tree default)) | NORMAL · Tasks overview | `a.tasklists.SetSelectedFunc` (app.go:623) fires: `a.setFocus(a.tree)` — dives focus into the task tree. | "Enter open" | help.go:27 | README.md:120 | holds |
| Enter (widget-dependent (calendarview.go:119; timegridview.go:437; list/tree default)) | NORMAL · Agenda overview | `a.agendaList` has **no** `SetSelectedFunc` and its items carry no `Selected` callback (render.go:74-84) — tview's default List Enter handler (vendor list.go:648-657) finds `item.Selected==nil` and `l.selected==nil` → does nothing. Focus stays on `a.agendaList`; there is no keyboard drill-in to the agenda board (MATRIX.md §2.2). | "Enter open" | help.go:27 | README.md:120 | **inconsistent-across-contexts** (see Divergence 4) |
| v (app.go:973-980) [Calendar-mode gated] | NORMAL · month grid | `a.mode==modeCalendar` → `a.viewMode` cycles month→week→day, `buildCenterCalendar()` rebuilds (setting the *new* active grid's data, which resets `eventMode=false` — moot, already undrilled), `refocusCalendar()` keeps the grid focused (app.go:973-980) | "v view" (render.go:735) | "Calendar" section: "v — cycle month / week / day" (help.go:86) | "`v` \| Cycle calendar view: month → week → day" (README.md:134) | holds |
| v (app.go:973-980) [Calendar-mode gated] | NORMAL · week-day grid | Same, cycling onward (week→day→month or day→month→week depending on entry point) | "v view" | help.go:86 | README.md:134 | holds |
| v (app.go:973-980) [Calendar-mode gated] | NORMAL · Calendars overview | `a.mode==modeCalendar` → cycles `viewMode` month→week→day→month, rebuilds the grid, `refocusCalendar()` no-ops (grid not focused) — focus stays on `a.calendars`. (Outside Calendar mode the `if` fails, falls out of the switch, and the rune is forwarded to whichever overview list is focused, matching no shortcut — a silent no-op there, per the task brief's "no-op elsewhere" pattern; not a row I own but confirmed while reading app.go:973-980.) | "v view" (render.go:735 — shown unconditionally in every mode, including Tasks/Agenda where `v` is a no-op; see the additional note below) | help.go:86 "v — cycle month/week/day" (listed under "Calendar" section) | README.md:134, 58 | holds |
| f (app.go:984-988) [Calendar-mode gated] | NORMAL · month grid | `shiftAnchor(1)` — advances the anchor by one month (view-appropriate unit); grid already undrilled so the explicit re-drill branch (`wasDrilled`) is inert here (app.go:984-988,1022-1047) | "f/b prev/next" (render.go:735) | "Calendar" section: "f / b — forward / back one period" (help.go:88) | "`f` / `b` · `gt` \| Forward / back one period · jump to today" (README.md:137) | holds |
| f (app.go:984-988) [Calendar-mode gated] | NORMAL · week-day grid | `shiftAnchor(1)` advances by one week (week view) or one day (day view) | "f/b prev/next" | help.go:88 | README.md:137 | holds |
| f (app.go:984-988) [Calendar-mode gated] | NORMAL · Calendars overview | `a.mode==modeCalendar` → `shiftAnchor(1)` (app.go:1022-1047) advances one view-period, preserving grid drill state if any (there is none here); focus stays on `a.calendars`. | "f/b prev/next" (render.go:735 — same unconditional-display caveat) | help.go:88 "f / b — forward/back one period" | README.md:137 | holds |
| b (app.go:989-993) [Calendar-mode gated] | NORMAL · month grid | `shiftAnchor(-1)` — back one month (app.go:989-993) | "f/b prev/next" | help.go:88 | README.md:137 | holds |
| b (app.go:989-993) [Calendar-mode gated] | NORMAL · week-day grid | `shiftAnchor(-1)` — back one week/day | "f/b prev/next" | help.go:88 | README.md:137 | holds |
| b (app.go:989-993) [Calendar-mode gated] | NORMAL · Calendars overview | `shiftAnchor(-1)` — same, backward. | "f/b prev/next" | help.go:88 | README.md:137 | holds |
| H (app.go:959-963) [Tasks-mode gated] | NORMAL · task tree | `reparentSelected(outdent)` — re-parents to the grandparent (or root) using the tree node actually on screen (edit.go:482-521) | — | "H / L — outdent / indent task (re-parent)" (help.go:65) | "`H` / `L` \| Outdent / indent task (re-parent)" (README.md:128) | holds |
| H (app.go:959-963) [Tasks-mode gated] | NORMAL · Tasks overview | `reparentSelected(outdent)` — reads `a.tree.GetCurrentNode()` directly; outdents it to its grandparent (or flashes "Already at the top level"). | — (not in curated hints) | help.go:65 "H / L — outdent/indent task" | README.md:128 | holds |
| L (app.go:964-968) [Tasks-mode gated] | NORMAL · task tree | `reparentSelected(indent)` — nests under the previous sibling (edit.go:482,500-511) | — | help.go:65 | README.md:128 | holds |
| L (app.go:964-968) [Tasks-mode gated] | NORMAL · Tasks overview | `reparentSelected(indent)` — nests it under the previous sibling (or flashes "Can't indent: no task above at this level"). | — | help.go:65 | README.md:128 | holds |
| > (app.go:1006-1010) [Tasks-mode gated] | NORMAL · task tree | `zoomInTree()` — re-roots the tree at the selected task, builds a "List / ancestor / task" breadcrumb (render.go:261-274,315-334,360-370) | — | "> / < — zoom into / out of the selected task's subtree" (help.go:66) | **not in the keybindings table** — only in Usage prose (README.md:59) — divergence #5 | doc-stale |
| > (app.go:1006-1010) [Tasks-mode gated] | NORMAL · Tasks overview | `zoomInTree` (render.go:261-274) — re-roots the tree at the current task (`a.zoomUID`), rebuilds `buildTree()`; flashes "Select a task to zoom into (>)" if nothing is selected. | — | help.go:66 "> / < — zoom into/out of subtree" | README.md: **missing from the Keybindings table entirely** — prose only, README.md:59 | doc-stale (see Divergence 2) |
| < (app.go:1011-1015) [Tasks-mode gated] | NORMAL · task tree | `zoomOutTree()` — pops one level toward the list root (render.go:276-289) | — | help.go:66 | **not in the keybindings table** — divergence #5 | doc-stale |
| < (app.go:1011-1015) [Tasks-mode gated] | NORMAL · Tasks overview | `zoomOutTree` (render.go:278-289) — pops one level toward the zoomed task's parent, or the list root; flashes "Already at the list root" if not zoomed. | — | help.go:66 | README.md: missing from the table, prose only | doc-stale |
| z (app.go:862-868) [Tasks-mode gated] | NORMAL · task tree | `a.mode == modeTasks` → `startPrefix('z')` — opens the fold which-key hint (app.go:862-868, keys.go:46-50) | — | (implicit — the `z …` row documents the continuations, same pattern as `i`/`g`/`s`) | "`z` … \| Fold the tree — zR expand-all, zM collapse-all, za toggle" (README.md:132) | holds |
| z (app.go:862-868) [Tasks-mode gated] | NORMAL · Tasks overview | `startPrefix('z')` — opens the which-key hint for the fold chord (zR/zM/za); outside Tasks mode this flashes "fold: Tasks view only" instead (app.go:862-868). | — | help.go:71 "z R / z M / z a — fold" | README.md:132 "z … — Fold the tree" | holds |
| zR (keys.go:47) [Tasks-mode gated] | NORMAL · task tree | `setFoldAll(true)` — expands every folder recursively (keys.go:274-282) | — | "z R / z M / z a — fold — expand all / collapse all / toggle" (help.go:71) | README.md:132 | holds |
| zR (keys.go:47) [Tasks-mode gated] | NORMAL · Tasks overview | `setFoldAll(true)` (keys.go:274-282) — expands every folder in the tree, updates each node's `▸`/`▾` caret. | — | help.go:71 | README.md:132 | holds |
| zM (keys.go:48) [Tasks-mode gated] | NORMAL · task tree | `setFoldAll(false)` — collapses every folder recursively (keys.go:274-282) | — | help.go:71 | README.md:132 | holds |
| zM (keys.go:48) [Tasks-mode gated] | NORMAL · Tasks overview | `setFoldAll(false)` — collapses every folder. | — | help.go:71 | README.md:132 | holds |
| za (keys.go:49) [Tasks-mode gated] | NORMAL · task tree | `toggleFold()` — flips the current node's expand state (keys.go:297-307) | — | help.go:71 | README.md:132 | holds |
| za (keys.go:49) [Tasks-mode gated] | NORMAL · Tasks overview | `toggleFold` (keys.go:297-307) — flips the current node's own fold state (no-op if it has no children). | — | help.go:71 | README.md:132 | holds |
| h j k l / arrows (item cycle / spatial nav) (calendarview.go:143-186; timegridview.go:453-477 (arrows only — see report note)) | DRILL · month grid | Global `motionArrow` always pre-translates hjkl to arrow-key events (keys.go:147-164, `app.go:803-813`) before any widget sees them, so `calendarView.handleEventMode` only ever receives arrow `Key()` values: `Up`/`Down` step `eventIndex` ±1 through the day's item list, `Left`/`Right` are unhandled (no case — a drilled day is a 1D list, calendarview.go:151-160,171-172). The widget's *own* rune cases for `j`/`k` (calendarview.go:173-185) are dead code — a raw rune never arrives (see Method notes / Additional observations, resolving the MATRIX.md open question at §1.3) | "hjkl move" (render.go:735) | "h j k l / arrows — move the highlight" (help.go:23); Select section separately notes drilled-day cycling is via "j/k/arrows, not Enter" (help.go:27) | "Move the highlight …" (README.md:117); "`<count>G` → nth item … of a drilled day" implies the same j/k-style stepping (README.md:119) | holds |
| h j k l / arrows (item cycle / spatial nav) (calendarview.go:143-186; timegridview.go:453-477 (arrows only — see report note)) | DRILL · week-day grid | Same pre-translation; `timeGridView.handleEventMode` has genuine `Up`/`Down`/`Left`/`Right` cases doing full 2D `spatialMove` — vertical by time (`nearestLevel`), horizontal between overlapping-lane events at the same time (`spatialTarget`, timegridview.go:290-339,458-465). Unlike the month grid, `timeGridView` never had rune cases to begin with, so nothing here is dead code — the global translation is what makes hjkl work at all | "hjkl move" | help.go:23,27 | README.md:117,119 | holds |
| gg (Home) (calendarview.go:161-165; timegridview.go:466-470) | DRILL · month grid | `KeyHome` → `eventIndex=0`, jumps to the day's first item (calendarview.go:161-165); no count support (`gotoTop` doesn't take one, keys.go:184-195) | — | help.go:25 (gg/G row) | README.md:119 | holds |
| gg (Home) (calendarview.go:161-165; timegridview.go:466-470) | DRILL · week-day grid | Same — first item of the drilled day (timegridview.go:466-470) | — | help.go:25 | README.md:119 | holds |
| G (End) (calendarview.go:166-170; timegridview.go:471-475) | DRILL · month grid | `gotoBottom(count)`: grid *is* `active`-drilled, so `count>0` calls `g.reDrill(day, count-1)` — jumps to the count-th item (1-indexed) of the day, clamped (keys.go:260-267, calendarview.go:202-209); with no count, `KeyEnd` → last item (calendarview.go:166-170) | — | help.go:25 | "`<count>G` → nth item of a list, the tree, or **a drilled day**" (README.md:119) — this is the one row that explicitly documents the count-honoring behavior verified here | holds |
| G (End) (calendarview.go:166-170; timegridview.go:471-475) | DRILL · week-day grid | Same mechanism via `timeGridView.reDrill` (timegridview.go:189-196,471-475) | — | help.go:25 | README.md:119 | holds |
| Enter (no case in handleEventMode — calendarview.go:143-187; timegridview.go:453-477) | DRILL · month grid | No `KeyEnter` case anywhere (`globalKeys`, `calendarView.handleEventMode`) — genuinely absorbed, no-op, no flash (calendarview.go:143-187) | "Enter open" (render.go:735) | "…(a drilled day's events are then cycled with j/k/arrows, not Enter)" (help.go:27) — accurate | "Dive into the center; **cycle a day's events**; open a list …" (README.md:120) — reads as if Enter cycles events; it doesn't — divergence #1 | doc-stale |
| Enter (no case in handleEventMode — calendarview.go:143-187; timegridview.go:453-477) | DRILL · week-day grid | Same — `timeGridView.handleEventMode` has no `KeyEnter` case either (timegridview.go:453-477) | "Enter open" | help.go:27 (accurate) | README.md:120 — divergence #1 | doc-stale |
| Esc (calendarview.go:146-150; timegridview.go:456-457) | DRILL · month grid | `eventMode=false`, re-emits the day via `onSelectDay(cv.selected)` (refreshes Detail) — undrills but **keeps focus in the grid** (no `onExit`, unlike the NORMAL-context Esc); a second Esc then exits to the Calendars overview (calendarview.go:146-150) | "Esc back" (render.go:735) | "Esc — back out (a form/dialog too)" (help.go:28) | "`Esc` \| Back out to the overview · cancel a form/dialog/chord" (README.md:121) | holds |
| Esc (calendarview.go:146-150; timegridview.go:456-457) | DRILL · week-day grid | Same two-step pattern: `tg.eventMode=false` (timegridview.go:456-457), stays grid-focused; a second Esc exits via `onExit` | "Esc back" | help.go:28 | README.md:121 | holds |
| Space (app.go:906-924) | DRILL · month grid | `currentTarget()` succeeds (drilled item exists): if it's a task, `toggleComplete()`; if it's an event, flashes "Can't complete an event" — never falls to `toggleCalendarVisibility()` here, unlike the undrilled NORMAL rows (app.go:906-924, edit.go:75-98,289-360) | "Space done/hide" (render.go:735) — "done" half applies here | "Space — toggle task done …" (help.go:64) | "`Space` \| Toggle the selected/drilled task done — or hide/show the highlighted calendar (Calendar view, no task drilled)" (README.md:126) — explicitly documents this exact NORMAL/DRILL split | holds |
| Space (app.go:906-924) | DRILL · week-day grid | Same | "Space done/hide" | help.go:64 | README.md:126 | holds |
| e (app.go:900) | DRILL · month grid | `currentTarget()` succeeds → edits the **drilled item** directly (event or task; a recurring one gets the this/future/all scope picker) via `editSelected()` — no calendar-panel fallback here, unlike the undrilled NORMAL rows (edit.go:576-626,595-607,617-620) | "e edit" (render.go:735) | "e — edit selected …"; "e / d on recurring event/task — prompts scope …" (help.go:58,61-62) | "`e` \| Edit selected (full form)" (README.md:123) | holds |
| e (app.go:900) | DRILL · week-day grid | Same | "e edit" | help.go:58,61-62 | README.md:123 | holds |
| d (app.go:903) | DRILL · month grid | `deleteContextual()` → `deleteSelected()`: `currentTarget()` succeeds → deletes the **drilled item** (confirm dialog; recurring gets the scope picker; a task's subtree goes with it) — contrast with the undrilled NORMAL rows' "Nothing selected to delete" (keys.go:125-132, edit.go:422-478) | "d del" (render.go:735) | "d — delete …"; recurring-scope row (help.go:60-62) | "`d` \| Delete selected item …" (README.md:125) | holds |
| d (app.go:903) | DRILL · week-day grid | Same | "d del" | help.go:60-62 | README.md:125 | holds |
| m (app.go:882 (enters GRAB on the drilled item)) | DRILL · month grid | `startGrab()`→`currentTarget()` succeeds → enters single-item GRAB on the drilled event/task (a recurring event prompts scope first) (grab.go:26-61) | — | "m — grab: move an event … or nudge a task's due date … Enter keep, Esc cancel" (help.go:69) | "`m` \| Grab mode: move an event in time … Enter keeps, Esc reverts" (README.md:130) | holds |
| m (app.go:882 (enters GRAB on the drilled item)) | DRILL · week-day grid | Same; `grabStatus()` additionally offers hour-level nudging + `J`/`K` resize since `a.viewMode != viewMonth` (grab.go:141-150) | — | help.go:69 | README.md:130 | holds |
| V (app.go:981; selection.go:65-79 (selDrill)) | DRILL · month grid | `enterSelect()` — `selContext()` returns `selDrill` (grid is drilled); anchors on the drilled item's UID/occurrence via `g.selectedItem()` rather than the day (selection.go:34-38,65-79) | — | help.go:76 (same row) | README.md:131 (same row, explicitly covers "a drilled day's items") | holds |
| V (app.go:981; selection.go:65-79 (selDrill)) | DRILL · week-day grid | Same | — | help.go:76 | README.md:131 | holds |
| f (app.go:984-988) | DRILL · month grid | `shiftAnchor(1)` — captures `wasDrilled=true` up front, rebuilds, then **explicitly re-drills** via `g.reDrill(a.anchor, 0)` — stays drilled on the first item of the new period's day, unlike `v`/`gt`/`gd`/`.` which drop drill silently (app.go:984-988,1022-1047) — see Additional observations | "f/b prev/next" (render.go:735) | "f / b — forward / back one period" (help.go:88) | "`f` / `b` · `gt` \| Forward / back one period · jump to today" (README.md:137) | holds |
| f (app.go:984-988) | DRILL · week-day grid | Same re-drill behavior | "f/b prev/next" | help.go:88 | README.md:137 | holds |
| b (app.go:989-993) | DRILL · month grid | `shiftAnchor(-1)` — same explicit re-drill (app.go:989-993,1022-1047) | "f/b prev/next" | help.go:88 | README.md:137 | holds |
| b (app.go:989-993) | DRILL · week-day grid | Same | "f/b prev/next" | help.go:88 | README.md:137 | holds |
| v (app.go:973-980) | DRILL · month grid | View cycles month→week→day; `buildCenterCalendar()` calls `setData()` on the *new* active grid, which unconditionally resets `eventMode=false` — **no re-drill call anywhere in this path**, so `v` silently drops DRILL back to NORMAL day-navigation on the new view (app.go:973-980, render.go:108-134) — see Additional observations (contrast with `f`/`b`) | "v view" (render.go:735) | "v — cycle month / week / day" (help.go:86) | "`v` \| Cycle calendar view: month → week → day" (README.md:134) | holds — no doc promises drill survives a view switch |
| v (app.go:973-980) | DRILL · week-day grid | Same undrill-on-cycle behavior | "v view" | help.go:86 | README.md:134 | holds |
| + / - (app.go:938-951) | DRILL · month grid | `timeGridActive()` false → `setAccordion(true/false)`; doesn't touch `eventMode` at all, so **drill state is preserved** across the accordion collapse/restore (unlike `v`) (keys.go:506-533) | — | help.go:93 | README.md:138 | holds |
| + / - (app.go:938-951) | DRILL · week-day grid | `timeGridActive()` true → `zoomHour(±1)`; likewise doesn't touch `eventMode` — drill preserved (keys.go:470-491) | — | help.go:93 | README.md:138 | holds |
| 0 (bare) (app.go:952-958) | DRILL · month grid | Not `timeGridActive()` → falls through unhandled, same true no-op as the NORMAL·month row; drill state (already whatever it was) is untouched since nothing runs (app.go:952-958) | — | help.go:93 | README.md:138 | holds |
| 0 (bare) (app.go:952-958) | DRILL · week-day grid | `timeGridActive()` true → `resetHourZoom()`; doesn't touch `eventMode` — drill preserved (keys.go:493-504) | — | help.go:93 | README.md:138 | holds |
| Enter (grab.go:167) | GRAB · task tree | `commitGrab()` — keeps the nudged due date as one undo step, ends grab (grab.go:167-168,404-415) | "Enter keep" (`grabStatus`, grab.go:141-150, surfaced via `updateStatus`, render.go:712-717) | "m — ... Enter keep, Esc cancel" (help.go:69) | "`m` \| ... — `Enter` keeps, `Esc` reverts" (README.md:130) | holds |
| Enter (grab.go:167) | GRAB · month grid | `commitGrab()` — pushes one undo step, `focusGrabbed()` re-drills onto the moved item, ends grab, flashes "Rescheduled (u to undo)" (grab.go:404-415) | `grabStatus()` default case: "GRAB event · h/l ±day · Enter keep · Esc cancel" (grab.go:141-150) — or "GRAB due · j/k ±day · h/l ±week · Enter keep · Esc cancel" for a todo | "m — … Enter keep, Esc cancel" (help.go:69) | "`Enter` keeps" (README.md:130) | holds |
| Enter (grab.go:167) | GRAB · week-day grid | Same `commitGrab()` | grabStatus week/day case: "GRAB event · j/k ±hour · h/l ±day · J/K resize · Enter keep · Esc cancel" (grab.go:146) | help.go:69 | README.md:130 | holds |
| Enter (grab.go:167) | GRAB · agenda board | `commitGrab()` — same universal commit; `focusGrabbed()` falls to its `else` branch (`a.refresh(a.grabUID)`, grab.go:356-377) since `mode != modeCalendar` | grabStatus default: "GRAB event · h/l ±day · Enter keep · Esc cancel" (event) or "GRAB due · j/k ±day · h/l ±week · Enter keep · Esc cancel" (todo) | help.go:69 | README.md:130 | holds |
| Esc (grab.go:169) | GRAB · task tree | `cancelGrab()` — reverts to the pre-grab snapshot, surfaces a write error if the revert itself fails (grab.go:169,422-451) | "Esc cancel" (grabStatus) | help.go:69 | README.md:130 | holds |
| Esc (grab.go:169) | GRAB · month grid | `cancelGrab()` — restores the pre-grab snapshot (`Restore`), surfaces a revert failure rather than reporting a clean cancel (grab.go:422-451) | same grabStatus text ("Esc cancel") | help.go:69 | "`Esc` reverts" (README.md:130) | holds |
| Esc (grab.go:169) | GRAB · week-day grid | Same `cancelGrab()` | same grabStatus text | help.go:69 | README.md:130 | holds |
| Esc (grab.go:169) | GRAB · agenda board | `cancelGrab()` — same universal revert | same grabStatus text | help.go:69 | README.md:130 | holds |
| h l / Left Right (grab.go:171-174,181-183,246-267) | GRAB · task tree | Not an event (`!a.grabIsEvent`) → `grabNudge`'s `map[rune]int{'j':1,'k':-1,'l':7,'h':-7}` — **h/l = ∓1 week** (grab.go:171-186,199-221) | "h/l ±week" (`grabStatus`'s `!a.grabIsEvent` branch: "GRAB due · j/k ±day · h/l ±week · Enter keep · Esc cancel", grab.go:143-144) | help.go:69 (doesn't spell out the per-key day/week split for tasks) | "nudge a task's due date (`j`/`k` day, `h`/`l` week)" (README.md:130) | inconsistent-across-contexts (see divergence #1 — matches its own doc but conflicts with bulk grab's opposite mapping on the same object type) |
| h l / Left Right (grab.go:171-174,181-183,246-267) | GRAB · month grid | **Event**: ±1 day (`d.Start/.End.AddDate(0,0,±1)`, grab.go:246-267); a whole-series day-move re-anchors any day-pinning `BY*` rule via `model.ReanchoredRecurrence`, or blocks with a flash if the rule can't be reasoned about. **Todo**: h/l = ∓/±1 **week** (map `{'h':-7,'l':7}`, grab.go:214) | month case: "h/l ±day" (event) or "h/l ±week" (todo) (grab.go:144-148) | "hjkl day/hour" generic, doesn't split todo's day/week axis (help.go:69) | precisely splits it: "h`/`l` day" (event) vs "`h`/`l` week" (todo) (README.md:130) | holds |
| h l / Left Right (grab.go:171-174,181-183,246-267) | GRAB · week-day grid | Same as month grid: event ±1 day; todo ±1 week | grabStatus lists "h/l ±day" (event context) | help.go:69 | README.md:130 | holds |
| h l / Left Right (grab.go:171-174,181-183,246-267) | GRAB · agenda board | Event: ±1 day. Todo: ±1 week (same map as calendar-grid GRAB, grab.go:214,246-267) — mode-independent | grabStatus (event: "h/l ±day"; todo: "h/l ±week") | help.go:69 | README.md:130 | holds |
| j k / Up Down (grab.go:175-178,181-183,268-280) | GRAB · task tree | Same map — **j/k = ±1 day** (grab.go:214-221) | "j/k ±day" (grabStatus) | help.go:69 | README.md:130 | inconsistent-across-contexts (divergence #1) |
| j k / Up Down (grab.go:175-178,181-183,268-280) | GRAB · month grid | **Event**: `timed` is always false in month view (`a.viewMode==viewMonth`, grab.go:244) — always blocked, flashes `grabTimeHint("change the time")` = "switch to week/day view (v) to change the time" (**correct** wording here, since the user really is in month view). **Todo**: j/k = ±1 day always (map `{'j':1,'k':-1}`) | month grabStatus omits j/k for events; todo case shows "j/k ±day" | "hjkl day/hour" (help.go:69) doesn't scope hour-nudge to week/day view | README.md:130 doesn't scope it either | **doc-stale** (Divergence #5) |
| j k / Up Down (grab.go:175-178,181-183,268-280) | GRAB · week-day grid | **Timed event**: ±1 hour (`grabHourStep`, grab.go:268-280). **All-day event, even though already in week/day view**: `timed` is false (`!base.AllDay`, grab.go:244) → blocked, but `grabTimeHint` returns "switch to week/day view (v) to change the time" — **wrong**, the user is already there (grab.go:156-161). **Todo**: ±1 day always, unaffected by view | grabStatus advertises "j/k ±hour" unconditionally for the event case, regardless of AllDay | help.go:69 doesn't distinguish AllDay | README.md:130 doesn't distinguish AllDay | **code-diverges** (Divergence #3) |
| j k / Up Down (grab.go:175-178,181-183,268-280) | GRAB · agenda board | Event: `timed` requires `a.mode==modeCalendar`, always false in Agenda → blocked, flashes `grabTimeHint`'s else-branch: **"open the week/day calendar view to change the time"** (grab.go:157-161) — accurate here (Agenda genuinely isn't the calendar view). Todo: ±1 day always | grabStatus omits j/k for the event case in Agenda; shows "j/k ±day" for todos | help.go:69 (generic) | README.md:130 | holds |
| J K (grab.go:181-183,281-298) | GRAB · task tree | Dispatched to `grabNudge('J')`/`grabNudge('K')`, but the todo-path map has no `'J'`/`'K'` entry → `days==0` → silent `return`, no flash, nothing happens (grab.go:181-183,214-217) | not mentioned in the `!a.grabIsEvent` `grabStatus` string (grab.go:143-144) | not mentioned for tasks (help.go:69 only ties J/K to events: "hjkl day/hour, J/K resize") | not mentioned for tasks (README.md:130 only ties J/K to events) | holds — undocumented no-op is fine (obscure key, no doc claims otherwise) |
| J K (grab.go:181-183,281-298) | GRAB · month grid | **Event**: same month-view block as j/k, correct hint. **Todo**: no `J`/`K` entry in the nudge map → `days==0` → silent `return`, **zero feedback** (grab.go:214-217) | month grabStatus never lists J/K | help.go:69 mentions "J/K resize" without noting it's event-only / view-scoped | README.md:130 mentions "J/K resize" for events only (accurate) | **inconsistent-across-contexts** (Divergence #4: silent for a todo vs. bulk grab's explicit flash) |
| J K (grab.go:181-183,281-298) | GRAB · week-day grid | **Timed event**: resizes the end ±1 hour, refuses if it would invert (`d.End` ≤ `d.Start`, "Event can't be that short", grab.go:281-298). **All-day event**: same wrong "switch to week/day view" hint as j/k. **Todo**: silent no-op (no `J`/`K` in the nudge map) | grabStatus advertises "J/K resize" unconditionally | help.go:69 | README.md:130 (event-only, correct) | **code-diverges** (Divergence #3, plus the silent-todo issue of Divergence #4) |
| J K (grab.go:181-183,281-298) | GRAB · agenda board | Event: same Agenda-appropriate block/hint as j/k ("open the week/day calendar view to resize"). Todo: silent no-op — no `J`/`K` in the nudge map, `days==0` returns with zero feedback (grab.go:214-217) | grabStatus omits J/K for events in Agenda; never shown for todos either | help.go:69 mentions "J/K resize" without noting it's event-only | README.md:130 (event-only, accurate) | **inconsistent-across-contexts** (Divergence #4 — silent for a todo, vs. bulk grab's explicit flash) |
| Enter (bulkgrab.go:93) | GRAB (bulk, via SELECT) · task tree | `commitBulkGrab()` — keeps all shifts as one undo step, exits GRAB **and** SELECT (bulkgrab.go:93-94,214-232) | "Enter keep" (`bulkGrabStatus`, bulkgrab.go:85-87, surfaced via updateStatus render.go:712-714) | "m — bulk grab — one uniform date-shift over the whole range" (help.go:81) | "`m` grab all (±day/±week). `Esc` cancels" (README.md:131) | holds |
| Enter (bulkgrab.go:93) | GRAB (bulk, via SELECT) · month grid | `commitBulkGrab()` — one undo step covering every shifted item, ends bulk-grab **and** SELECT, flashes "Rescheduled N item(s)" or "Nothing moved" (bulkgrab.go:214-232) | `bulkGrabStatus()`: "GRAB ×N · h/l ±day · j/k ±week · Enter keep · Esc cancel" (bulkgrab.go:85-87) | "m — bulk grab — one uniform date-shift over the whole range" (help.go:81) | "m grab all (±day/±week)" (README.md:131) | holds |
| Enter (bulkgrab.go:93) | GRAB (bulk, via SELECT) · week-day grid | Same `commitBulkGrab()` | bulkgrab.go:85-87 | help.go:81 | README.md:131 | holds |
| Esc (bulkgrab.go:95) | GRAB (bulk, via SELECT) · task tree | `cancelBulkGrab()` — reverts every item, returns to SELECT (not fully out) with the range intact (bulkgrab.go:95-96,237-271) | "Esc cancel" (bulkGrabStatus) | "Esc — cancel (from a nested grab, back to SELECT; from SELECT, exits to the underlying view)" (help.go:83) | README.md:131 ("Esc cancels", no nesting detail) | holds |
| Esc (bulkgrab.go:95) | GRAB (bulk, via SELECT) · month grid | `cancelBulkGrab()` — restores every pre-grab snapshot newest-first, returns to **SELECT** (not fully exits) with the range intact, flashes "Grab cancelled — still selecting" (bulkgrab.go:237-271) | same bulkGrabStatus text | "Esc — cancel (from a nested grab, back to SELECT…)" (help.go:83) — matches exactly | README.md:131 (Esc cancels) | holds |
| Esc (bulkgrab.go:95) | GRAB (bulk, via SELECT) · week-day grid | Same `cancelBulkGrab()`, returns to SELECT | same | help.go:83 | README.md:131 | holds |
| h l / Left Right (bulkgrab.go:97-100,107-110) | GRAB (bulk, via SELECT) · task tree | `bulkGrabShift(-1)`/`bulkGrabShift(1)` — **h/l = ∓1 day** for every selected task (bulkgrab.go:97-100,107-110,127-190) | "h/l ±day" (bulkGrabStatus, bulkgrab.go:86) | help.go:81 (no per-key breakdown) | README.md:131 ("±day/±week", no per-key breakdown) | inconsistent-across-contexts (divergence #1) |
| h l / Left Right (bulkgrab.go:97-100,107-110) | GRAB (bulk, via SELECT) · month grid | `bulkGrabShift(∓1/±1)` — every grabbed item's start (and end, if any) shifts by 1 day, each write version-checked (`PutIfUnchanged`); a stale item ends the grab keeping earlier nudges (bulkgrab.go:127-190,195-210) | "h/l ±day" (bulkGrabStatus) | help.go:81 | README.md:131 | holds |
| h l / Left Right (bulkgrab.go:97-100,107-110) | GRAB (bulk, via SELECT) · week-day grid | Same ±1 day shift | same | help.go:81 | README.md:131 | holds |
| j k / Up Down (bulkgrab.go:101-104,111-114) | GRAB (bulk, via SELECT) · task tree | `bulkGrabShift(7)`/`bulkGrabShift(-7)` — **j/k = ±1 week** (bulkgrab.go:101-104,111-114) | "j/k ±week" (bulkGrabStatus) | help.go:81 | README.md:131 | inconsistent-across-contexts (divergence #1) |
| j k / Up Down (bulkgrab.go:101-104,111-114) | GRAB (bulk, via SELECT) · month grid | `bulkGrabShift(±7/∓7)` — a whole **week** shift (deliberately different granularity from single-grab's per-target-type axis) | "j/k ±week" (bulkGrabStatus) | help.go:81 | README.md:131 | holds |
| j k / Up Down (bulkgrab.go:101-104,111-114) | GRAB (bulk, via SELECT) · week-day grid | Same ±1 week shift (no hour-level option even though initiated from the time-grid) | same | help.go:81 | README.md:131 | holds |
| J K (inert) (bulkgrab.go:115-117) | GRAB (bulk, via SELECT) · task tree | `a.flash("Resize doesn't apply to a multi-selection")` — explicit rejection, not silent (bulkgrab.go:115-117) | not mentioned in bulkGrabStatus | not mentioned | not mentioned | holds — undocumented but self-explanatory via its own flash message |
| J K (inert) (bulkgrab.go:115-117) | GRAB (bulk, via SELECT) · month grid | Always flashes `"Resize doesn't apply to a multi-selection"`, no state change (bulkgrab.go:115-117) | bulkGrabStatus never lists J/K (correctly omitted) | not mentioned in help.go's Select section (help.go:75-84) | not mentioned in README | holds (the flash itself is the only documentation, but it's accurate and doesn't mislead) |
| J K (inert) (bulkgrab.go:115-117) | GRAB (bulk, via SELECT) · week-day grid | Same inert flash | same | not mentioned | not mentioned | holds |
| Esc (selection.go:332-335) | SELECT · task tree | `exitSelect()` + flash "Select cancelled" (selection.go:104-110,332-335) | "Esc/V cancel" (SELECT hint, render.go:722) | "Esc — cancel (from a nested grab, back to SELECT; from SELECT, exits to the underlying view)" (help.go:83) | "`V` \| ... `Esc` cancels" (README.md:131) | holds |
| Esc (selection.go:332-335) | SELECT · month grid | Swallowed: `a.exitSelect()` clears the anchor and `a.selecting`, flashes "Select cancelled"; returns `nil` | "SELECT · hjkl extend · gg/G ends · Space done · d delete · y/Y yank · m grab · Esc/V cancel" (render.go:722) | "Esc — cancel (from a nested grab, back to SELECT; from SELECT, exits to the underlying view)" (help.go:83) | "…`Esc` cancels" (README.md:131) | holds |
| Esc (selection.go:332-335) | SELECT · week-day grid | Same as month grid: `a.exitSelect()` + flash, swallowed | render.go:722 | help.go:83 | README.md:131 | holds |
| h j k l / arrows (extend, unmodified) (selection.go:336-339,345-346) | SELECT · task tree | Motion keys pass through (`return ev`), fall to `globalKeys`' hjkl→arrow translation and `a.tree`'s own selection movement; `a.tree.SetChangedFunc` then calls `syncSelectionVisuals()` since `a.selecting` is true, restyling the in-range rows (selection.go:336-346, app.go:634-638, selection.go:129-147) | "hjkl extend" (SELECT hint, render.go:722) | "h j k l / f b / gg / G — extend the range" (help.go:77) | "extend a contiguous selection with the movement keys" (README.md:131) | holds |
| h j k l / arrows (extend, unmodified) (selection.go:336-339,345-346) | SELECT · month grid | Unmodified arrows / hjkl runes return the event unhandled, so `calendarPrimitive()`'s own motion (day-nav or, if drilled, item-cycle) runs as normal; the range is re-derived anchor→new-cursor via `daysRange()`/`drillRange()` (selection.go:260-313) on next redraw. A modified arrow (e.g. Ctrl-Left/Right) is swallowed instead (comment, selection.go:340-342) so it can't leak a layout resize | "hjkl extend" (render.go:722) | "h j k l / f b / gg / G — extend the range (motion incl. period shift…)" (help.go:77) | "extend a contiguous selection with the movement keys (task tree, calendar days, or a drilled day's items)" (README.md:131) | holds |
| h j k l / arrows (extend, unmodified) (selection.go:336-339,345-346) | SELECT · week-day grid | Same passthrough; motion reaches `timegrid`'s own day-nav or (drilled) 2D item-cycle (`timegridview.go:416-477`); range re-derived via `daysRange()`/`drillRange()` | render.go:722 | help.go:77 | README.md:131 | holds |
| gg (selection.go:358-359 (passthrough; resolvePrefix gates to gg only)) | SELECT · task tree | `'g'` passes through, starts the prefix; `resolvePrefix` special-cases `p=='g' && ev.Rune()=='g'` to still run while selecting → `gotoTop()` → extends the range to the first node (keys.go:98-121, selection.go:358-359) | "gg/G ends" (SELECT hint, render.go:722) | help.go:77 (gg listed) | README.md:131 (implied by "movement keys") | holds |
| gg (selection.go:358-359 (passthrough; resolvePrefix gates to gg only)) | SELECT · month grid | First `g` passes through → `startPrefix('g')`; `resolvePrefix` (keys.go:82-121) explicitly permits only `p=='g' && ev.Rune()=='g'` while selecting (keys.go:103-106, blocking `gt`/`gd` with a flash) — the second `g` runs `gotoTop()`, moving the cursor to the first day/item and extending the range | "gg/G **ends**" (render.go:722) — see Divergence #1 | "extend the range" (help.go:77) | implies extend via generic movement-key wording (README.md:131) | **code-diverges** (Divergence #1: hint bar says "ends", code/`:help` say "extend") |
| gg (selection.go:358-359 (passthrough; resolvePrefix gates to gg only)) | SELECT · week-day grid | Same as month grid — `gotoTop()` extends the range to the top | "gg/G **ends**" (render.go:722) | "extend the range" (help.go:77) | README.md:131 | **code-diverges** (Divergence #1) |
| G (selection.go:345) | SELECT · task tree | Passes through → `gotoBottom(count)` selects the last (or count-th) node, extending the range (selection.go:345, keys.go:238-259) | "gg/G ends" (SELECT hint) | help.go:77 | README.md:131 | holds |
| G (selection.go:345) | SELECT · month grid | Passthrough → `gotoBottom(count)`: with no count, `repeatKey(End,1)`; with a count and the grid drilled, `g.reDrill(day, count-1)` (keys.go:238-269) — extends the range to the last day/item (or the count-th) | "gg/G **ends**" (render.go:722) | "extend the range" (help.go:77) | README.md:131 (generic) | **code-diverges** (same root cause as #1) |
| G (selection.go:345) | SELECT · week-day grid | Same as month grid — `gotoBottom(count)` extends to the last day/item | "gg/G **ends**" (render.go:722) | help.go:77 | README.md:131 | **code-diverges** (Divergence #1) |
| f (selection.go:345) | SELECT · task tree | Passes through as "motion", but `globalKeys`' `case 'f'` only acts when `a.mode == modeCalendar` (app.go:984-988); in Tasks mode it falls through to `return ev`, reaching `a.tree`'s `InputHandler` which has no `'f'` case — **true no-op**, range does not extend | "hjkl extend" (SELECT hint doesn't literally list f/b) | "h j k l / **f b** / gg / G — extend the range (... period shift ...)" (help.go:77) — lists f/b unconditionally | "extend ... with the movement keys" (README.md:131, doesn't name f/b specifically) | code-diverges (divergence #2) |
| f (selection.go:345) | SELECT · month grid | Passthrough → `a.shiftAnchor(1)` (`a.mode == modeCalendar` is always true in this context, app.go:984-988) shifts the visible period forward one step, extending the day-range; capped in practice by `maxSelectDays`=366 in `daysRange()` (selection.go:198-200,270-272) | not in the live hint (render.go:722 omits f/b) | "h j k l / f b / gg / G — extend the range (motion incl. period shift…)" (help.go:77) | generic "movement keys" (README.md:131) | holds |
| f (selection.go:345) | SELECT · week-day grid | Same — `shiftAnchor(1)`, one view-period forward (a week or a day depending on `viewMode`) | not in the live hint | help.go:77 | README.md:131 | holds |
| b (selection.go:345) | SELECT · task tree | Same as `f` — no-op in Tasks mode (app.go:989-993 gates to `modeCalendar`) | same | help.go:77 | README.md:131 | code-diverges (divergence #2) |
| b (selection.go:345) | SELECT · month grid | Passthrough → `a.shiftAnchor(-1)`, backward period shift, same cap | not in the live hint | help.go:77 (same row) | README.md:131 (generic) | holds |
| b (selection.go:345) | SELECT · week-day grid | Same — `shiftAnchor(-1)` | not in the live hint | help.go:77 | README.md:131 | holds |
| 1-9 / 0 (count, conditional) (selection.go:347-357) | SELECT · task tree | Digits pass through and accumulate normally; a bare `0` with no pending count is swallowed (mirrors `globalKeys`' own reset-hour-zoom guard) (selection.go:347-357) | — | "3j 5k … count prefix" applies generally (help.go:24) | "`<count>` + motion" (README.md:118) | holds |
| 1-9 / 0 (count, conditional) (selection.go:347-357) | SELECT · month grid | Digits `1-9` always pass through (accumulate `pendingCount` for the next motion); bare `0` passes through only if a count is already pending (continuing e.g. "10j"), otherwise swallowed — blocks `resetHourZoom`'s layout mutation from leaking past SELECT (selection.go:349-357) | not explicitly named (implied by "hjkl extend") | generic "3j 5k … count prefix" (help.go:24, not Select-specific) | not Select-specific | holds |
| 1-9 / 0 (count, conditional) (selection.go:347-357) | SELECT · week-day grid | Same digit-accumulation / bare-0-swallow rule | implied | help.go:24 | — | holds |
| V (selection.go:360-363) | SELECT · task tree | `exitSelect()` + flash "Select cancelled" — a second way out of SELECT, same effect as Esc (selection.go:360-363) | "Esc/V cancel" (SELECT hint explicitly lists V, render.go:722) | not mentioned (help.go:76 only documents V entering SELECT; the cancel row, help.go:83, only names Esc) | not mentioned (README.md:131 only says "Esc cancels") | doc-stale (divergence #3) |
| V (selection.go:360-363) | SELECT · month grid | `a.exitSelect()` + flash "Select cancelled" — toggles SELECT off exactly like Esc | "Esc/V cancel" (render.go:722) | only documents V as the *entry* key (help.go:76) — no mention of toggle-off | only documents V as the entry key + Esc as exit (README.md:131) | **doc-stale** (Divergence #11) |
| V (selection.go:360-363) | SELECT · week-day grid | Same toggle-off as month grid | render.go:722 | help.go:76 (entry only) | README.md:131 (entry+Esc only) | **doc-stale** (Divergence #11) |
| Space (selection.go:364-366) | SELECT · task tree | `bulkComplete()` — completes every eligible task in the range, reverse order so a folder can complete alongside its last child (selection.go:364-366, bulkops.go:57-160) | "Space done" (SELECT hint, render.go:722) | "Space — bulk complete the range" (help.go:78) | "`Space` complete all" (README.md:131) | holds |
| Space (selection.go:364-366) | SELECT · month grid | `a.bulkComplete()` — completes every todo in the range; non-todo (event) targets are silently skipped and counted (`bulkops.go:69-... ,90-95`) | "Space done" (render.go:722) | "Space — bulk complete the range" (help.go:78); "(skips)" row omits "event(s)" (help.go:82) | "Space complete all" (README.md:131) | **doc-stale** (Divergence #12) |
| Space (selection.go:364-366) | SELECT · week-day grid | Same `bulkComplete()`, same event-skip | render.go:722 | help.go:78,82 | README.md:131 | **doc-stale** (Divergence #12) |
| d (selection.go:367-369) | SELECT · task tree | `bulkDelete()` — one confirm naming the full count, whole subtrees, all-or-nothing (selection.go:367-369, bulkops.go:268-332) | "d delete" (SELECT hint) | "d — bulk delete the range (one confirm, whole subtrees)" (help.go:79) | "`d` delete all" (README.md:131) | holds |
| d (selection.go:367-369) | SELECT · month grid | `a.bulkDelete()` — one confirm; deletes every non-recurring survivor (tasks + non-recurring events), skipping recurring events/read-only/missing (`bulkops.go:172-225,261-...`) | "d delete" (render.go:722) | "d — bulk delete the range (one confirm, whole subtrees)" (help.go:79); skip reasons (help.go:82) match | "d delete all" (README.md:131) | holds |
| d (selection.go:367-369) | SELECT · week-day grid | Same `bulkDelete()` | render.go:722 | help.go:79,82 | README.md:131 | holds |
| y (selection.go:370-372) | SELECT · task tree | `bulkYank(true)` — cuts the ancestor-deduped roots onto the clipboard (selection.go:370-372, bulkops.go:228-259) | "y/Y yank" (SELECT hint) | "y / Y — bulk cut / copy the range to the clipboard (task tree only)" (help.go:80) | "`y`/`Y` cut/copy all (tree)" (README.md:131) | holds |
| y (selection.go:370-372) | SELECT · month grid | `a.bulkYank(true)` — gated: `if a.selContext() != selTree { flash "Yank works in the task tree (t)"; return }` (bulkops.go:232-236) — in a calendar SELECT this is **always** just a flash; SELECT stays active | "y/Y yank" (render.go:722, no tree-only caveat, but it's a terse persistent line) | "y / Y — bulk cut / copy the range to the clipboard (task tree only)" (help.go:80) | "y/Y cut/copy all (tree)" (README.md:131) | holds (restriction accurately documented in both static surfaces) |
| y (selection.go:370-372) | SELECT · week-day grid | Same tree-only gate | render.go:722 | help.go:80 | README.md:131 | holds |
| Y (selection.go:373-375) | SELECT · task tree | `bulkYank(false)` — copies instead of cuts (selection.go:373-375) | "y/Y yank" (SELECT hint) | help.go:80 | README.md:131 | holds |
| Y (selection.go:373-375) | SELECT · month grid | `a.bulkYank(false)` — same task-tree-only gate as `y` | render.go:722 (same) | help.go:80 (same) | README.md:131 (same) | holds |
| Y (selection.go:373-375) | SELECT · week-day grid | Same tree-only gate | render.go:722 | help.go:80 | README.md:131 | holds |
| m (selection.go:376-378) | SELECT · task tree | `startBulkGrab()` — filters the range to shiftable items (skips recurring events, undated tasks, read-only), enters bulk GRAB (selection.go:376-378, bulkgrab.go:34-81) | "m grab" (SELECT hint) | "m — bulk grab — one uniform date-shift over the whole range" (help.go:81) | "`m` grab all (±day/±week)" (README.md:131) | holds |
| m (selection.go:376-378) | SELECT · month grid | `a.startBulkGrab()` — filters the range (skips recurring events, undated tasks, missing, read-only via `bulkgrab.go:34-64`); enters bulk GRAB if ≥1 item survives, else flashes a skip summary | "m grab" (render.go:722) | "m — bulk grab — one uniform date-shift over the whole range" (help.go:81); "(skips) … grab also skips undated" (help.go:82) | "m grab all (±day/±week)" (README.md:131) | holds |
| m (selection.go:376-378) | SELECT · week-day grid | Same `startBulkGrab()` | render.go:722 | help.go:81,82 | README.md:131 | holds |
| Enter (keys.go:390) | RESIZE (global sub-mode) | `handleResizeKey` (`keys.go:388-413`) case `KeyEnter`/`KeyCtrlW` → `exitResizeMode(false)` (`keys.go:359-384`): keeps the adjusted widths, clears `a.resizing`, calls `updateStatus()`, flashes "Resize kept". | Not updated (F1); `updateStatus()` at exit resets `a.hints` back to the generic default line (no resize-specific content ever appears there). | "Layout" section: "Ctrl-W — resize sub-mode: ←/→ overview · H/L Detail · Enter keep · Esc/q cancel" (help.go:95). Matches. | `README.md:139`: "`Ctrl-W` … resize sub-mode (overview + Detail)" — doesn't spell out Enter=keep vs Esc/q=cancel; that detail lives only in `:help`. | holds (behavior matches `:help`; README is intentionally terser per its own style rules, not a contradiction) |
| Ctrl-W (keys.go:390) | RESIZE (global sub-mode) | Same case as Enter above — `KeyCtrlW` is a second way to "keep" (`keys.go:390`), not a toggle-exit; pressing it again while still resizing just re-keeps (no-op difference). | Not updated (F1). | Not named as an *exit* key in `:help`'s Layout row (only "Enter keep") — Ctrl-W's dual role (enter *and* keep) is undocumented as an exit key. | Not documented as an exit key. | doc-stale (minor): `:help`/README describe Ctrl-W only as the *entry* key; the code's exit-via-Ctrl-W (identical to Enter) isn't mentioned anywhere. |
| Esc (keys.go:392) | RESIZE (global sub-mode) | `exitResizeMode(true)` (`keys.go:359-384`): reverts `leftWidth`/`detailWidth` to their pre-resize values, restores a collapsed accordion if resize had un-collapsed it, flashes "Resize cancelled". | Not updated (F1). | Matches ("Esc/q cancel", help.go:95). | Matches (`README.md:139`, generic). | holds |
| Left / Right (arrows) (keys.go:394-397) | RESIZE (global sub-mode) | `resizeLeft(-leftWidthStep)` / `resizeLeft(+leftWidthStep)` (`keys.go:311-322`), clamped, persisted via `persistState()` — no-op while the accordion is collapsed (already undone on entry, so this never actually fires as a no-op in practice). | Not updated (F1); left-status flash from entry persists unchanged (resizeLeft doesn't call `flash`/`updateStatus`). | Matches ("←/→ overview", help.go:95). | Matches ("←/→" resizes overview, `README.md:139`). | holds |
| h (keys.go:400-401) | RESIZE (global sub-mode) | `resizeLeft(-leftWidthStep)` — identical effect to Left arrow. | Not updated (F1). | `:help`'s Layout row only names "←/→", not `h`/`l`, as the RESIZE overview-size keys — though the vim-style h/l-as-arrow-alias convention is documented generally elsewhere (Panels & navigation: "hjkl / arrows"). | README's `Ctrl-W` row (139) likewise only names "←/→", not h/l. | doc-stale (minor): the h/l aliases work (confirmed in code) but aren't spelled out in the RESIZE-specific doc rows, only implied by the app-wide hjkl=arrows convention. |
| l (keys.go:402-403) | RESIZE (global sub-mode) | `resizeLeft(+leftWidthStep)` — identical to Right arrow. | Not updated (F1). | Same as `h` row above. | Same as `h` row above. | doc-stale (minor, same as `h`) |
| H (keys.go:404-405) | RESIZE (global sub-mode) | `resizeDetail(-detailWidthStep)` (`keys.go:324-337`), clamped, persisted; no-op if Detail is hidden (Agenda mode). | Not updated (F1). | Matches ("H/L Detail", help.go:95). | Matches ("H/L Detail" via `Ctrl-W` row, `README.md:139`, plus the general `Ctrl-W` note at line 91). | holds |
| L (keys.go:406-407) | RESIZE (global sub-mode) | `resizeDetail(+detailWidthStep)` — grows Detail. | Not updated (F1). | Matches. | Matches. | holds |
| q (keys.go:408-409) | RESIZE (global sub-mode) | `exitResizeMode(true)` — identical to Esc (revert). | Not updated (F1). | Matches ("Esc/q cancel", help.go:95). | Matches (`README.md:139`, generic "resize sub-mode" note; the Esc/q pairing itself is only in `:help`, not spelled out in the table, but not contradicted). | holds |
| j / Down (forms.go:219-221,237-238) | forms (NORMAL) | `navKey`→`normalKey` (`forms.go:216-253`): `KeyDown`/rune `j` → `moveFocus(cur+1, false)` (`forms.go:175-188`) — steps to the next field/button, landing in NORMAL (no auto-drill). | Not updated (F1) — see divergence table (Tab row) for the closest related gap; j/k themselves are fully documented so no gap here. | Matches: "j / k / ↑ / ↓ — NORMAL: step between fields and the Save/Cancel buttons" (help.go:33). | Matches (`README.md:73`: "j/k/arrows … step between fields and the Save/Cancel buttons"). | holds |
| k / Up (forms.go:222-224,239-240) | forms (NORMAL) | `KeyUp`/rune `k` → `moveFocus(cur-1, false)` — steps to the previous field/button. | Not updated (F1). | Matches (help.go:33). | Matches (README.md:73). | holds |
| Tab (forms.go:219-221) | forms (NORMAL) | Grouped in the same `case tcell.KeyDown, tcell.KeyTab:` as `j`/Down — identical effect, undocumented as a named synonym (see Divergences). | Not updated (F1). | Not named (only j/k/↑/↓ listed, help.go:33). | Not named; README's global Tab row (116) says "cycle panels" with no form-context caveat. | doc-stale — see Divergences table |
| Shift-Tab (forms.go:222-224) | forms (NORMAL) | Grouped with `k`/Up (`KeyUp, KeyBacktab` case) — identical effect, undocumented as a named synonym. | Not updated (F1). | Not named. | Not named. | doc-stale — see Divergences table |
| l / Right (button move) (forms.go:225-227,241-242,278-294) | forms (NORMAL) | `KeyRight`/rune `l` → `moveButton(cur, +1)` (`forms.go:278-294`): inert while a field has focus (`cur < items` → return), moves within the button row otherwise, clamped at the last button; also calls `setDrilled(false)`. | Not updated (F1). | Matches: "h / l (or ←/→) — NORMAL: move between the buttons" (help.go:34). | Matches (`README.md:73`: "h/l/←/→ between the buttons"). | holds |
| h / Left (button move) (forms.go:228-230,243-244,278-294) | forms (NORMAL) | `KeyLeft`/rune `h` → `moveButton(cur, -1)` — same inert-on-field / clamp behavior, moving left. | Not updated (F1). | Matches (help.go:34). | Matches (README.md:73). | holds |
| Enter (forms.go:231-232,257-276) | forms (NORMAL) | `actNormal` (`forms.go:257-276`) dispatches by focused element type: `InputField`→drills (`setDrilled(true)`), `DropDown`→passes `ev` through so tview opens its native list, `Checkbox`→toggles + auto-advances, `weekdayStrip`→drills; on a button, returns `ev` so the button's own activation runs. | Not updated (F1). | Matches: "Enter — NORMAL: drill a text field, open a dropdown, toggle a checkbox, or press a button" (help.go:36). | Matches (`README.md:73`, same enumeration). | holds |
| Esc (forms.go:233-234) | forms (NORMAL) | `case tcell.KeyEscape: return ev` — passed to the focused item's own handler, which (per `forms.go:233-234`'s own comment) lets the item's finished-handler fire the Form's `SetCancelFunc` (wired per-form, e.g. `itemforms.go:162,189,367,393`, `calendar.go:178,339` → `a.closeModal(pageForm)`), i.e. a *second* Esc (the first already having returned DRILL→NORMAL) cancels/closes the form. Verified: `tview.Modal`/`Form`'s cancel wiring confirmed via each caller's explicit `f.SetCancelFunc(...)`. | Not updated (F1). | Matches: "Esc — DRILL → NORMAL (keeps the value); a second Esc cancels the form" (help.go:38). | Matches (`README.md:73`: "`Esc` steps back out to NORMAL … a second `Esc` cancels the form"). | holds |
| g (forms.go:245-246) | forms (NORMAL) | rune `g` → `moveFocus(0, false)` — jumps to the first field. | Not updated (F1). | Matches: "g / G — NORMAL: jump to the first field / last element" (help.go:35). | Matches (`README.md:73`, "steps between fields" context implies this but doesn't literally restate g/G — see note). | holds (README doesn't literally list g/G for forms, but per its own style rule the keybindings table + this line is meant to be the canonical reference and g/G's *global* meaning — "go to top/bottom" — already generalizes here; not a contradiction) |
| G (forms.go:247-248) | forms (NORMAL) | rune `G` → `moveFocus(f.count()-1, false)` — jumps to the last element (a button). | Not updated (F1). | Matches (help.go:35). | Same note as `g` above. | holds |
| Up / Down (forms.go:196-199 (delegated to tview DropDown list)) | forms (NORMAL, dropdown open) | `navKey` (`forms.go:193-204`): `focusedDropDown()` + `dd.IsOpen()` check (`forms.go:196-199`) returns `ev` unconsumed the instant a dropdown is open, handing all navigation to tview's own `DropDown` list until it closes. | Not updated (F1). | Not documented (only "opens a dropdown" is mentioned, help.go:36; no further detail on navigating the open list). | Not documented (README.md:73, same "opening a dropdown" phrase, no detail on subsequent keys). | holds (no doc claim to contradict; the omission is expected — a native list's Up/Down is assumed obvious) |
| Enter (forms.go:196-199) | forms (NORMAL, dropdown open) | Same delegation — `ev` passed through; tview's `DropDown` closes the list and commits the highlighted option on Enter (its own default, not app code). | Not updated (F1). | Not documented (as above). | Not documented (as above). | holds |
| Esc (forms.go:196-199) | forms (NORMAL, dropdown open) | Same delegation — tview's `DropDown` closes the list without changing the selection on Esc (its own default). | Not updated (F1). | Not documented (as above). | Not documented (as above). | holds |
| Esc (forms.go:299-301) | forms (DRILL) | `drillKey` (`forms.go:296-310`): `case tcell.KeyEscape: f.setDrilled(false); return nil` — back to NORMAL, keeping the field's current (possibly-edited) value. | Not updated (F1). | Matches: "Esc — DRILL → NORMAL (keeps the value) …" (help.go:38). | Matches (`README.md:73`). | holds |
| Enter / Tab (commit + advance) (forms.go:302-304) | forms (DRILL) | `case tcell.KeyEnter, tcell.KeyTab: f.moveFocus(cur+1, true)` — commits the field and auto-drills into the next text field (per `moveFocus`'s `autoDrill` param, `forms.go:175-188`). `Tab` is grouped with Enter, undocumented as a synonym. | Not updated (F1). | "Enter (in DRILL)" documented; `Tab` not named as a synonym (help.go:37). | Not documented as a Tab synonym (README.md:73 only mentions Enter). | doc-stale — see Divergences table |
| Shift-Tab (commit + back) (forms.go:305-307) | forms (DRILL) | `case tcell.KeyBacktab: f.moveFocus(cur-1, true)` — commits and auto-drills into the *previous* text field. Note: no plain `Enter`-reverse exists; only Shift-Tab goes backward in DRILL. | Not updated (F1). | Not documented at all — `:help`'s DRILL row only covers forward Enter/Tab commit-advance and the Esc-back behavior; going backward within DRILL via Shift-Tab is undocumented in both `:help` and README. | Not documented. | doc-stale: a real, working key (Shift-Tab reverse-commit in DRILL) has no doc mention anywhere. |
| typed chars / cursor keys / Backspace-Delete (forms.go:309) | forms (DRILL) | `default: return ev` (fall-through at the end of `drillKey`) — every other key (letters, digits, cursor motion, Backspace/Delete) reaches the focused `InputField`'s native `tview` editing, unmodified. | Not updated (F1). | Implied but not literally itemized (help.go's DRILL rows describe Enter/Esc only; typing itself is assumed). | Implied (`README.md:73`: "In DRILL the keys reach the field"). | holds |
| Left / Right / h / l (weekday-strip cursor) (weekdaystrip.go:137-158) | forms (DRILL) | `weekdayStrip.InputHandler` (`weekdaystrip.go:137-158`): `KeyLeft`/rune `h` and `KeyRight`/rune `l` → `moveCursor(∓1)` (`weekdaystrip.go:161-169`), clamped to the 7 day cells. Reached only once the strip is drilled into (`caretForm.actNormal`'s `weekdayStrip` case sets `drilled=true`, then `drillKey`'s `default: return ev` hands the key straight to the strip's own handler). | Not updated (F1). | Matches: "Weekday strip (Custom repeat) — drill in (Enter), then ←/→ or h/l move, Space toggles a day" (help.go:39). | Matches (`README.md:73`: "on the weekday strip, `Space` toggles the highlighted day" — the ←/→/h/l move itself is implied by "the keys reach the field" rather than spelled out, but not contradicted). | holds |
| Space (weekday-strip toggle) (weekdaystrip.go:149) | forms (DRILL) | rune `' '` → `w.selected[w.cursor] = !w.selected[w.cursor]` — toggles the day under the cursor; does not advance the cursor or leave DRILL. | Not updated (F1). | Matches (help.go:39, "Space toggles a day"). | Matches (README.md:73, "`Space` toggles the highlighted day"). | holds |
| Esc (help.go:124-129) | modals (help) | `SetInputCapture` (`help.go:124-130`): `ev.Key()==KeyEscape` (combined with the `q`/`?` runes in one condition) → `a.closeModal(pageHelp)` (`edit.go:856-868`), which pops the focus stack and clears `a.formDrill`. | Title bar reads " Help — keys & commands (Esc to close) " (help.go:123) — its own inline hint, not `a.hints` (F1: `a.hints` unaffected). | N/A (this *is* the `:help` overlay). | N/A. | holds |
| q (help.go:125) | modals (help) | Same condition/line as Esc — `ev.Rune()=='q'` also closes it. | Same title (mentions only "Esc to close" — see Divergences: minor under-advertising). | The generic Panels row documents `q` as closing "a non-form dialog" (help.go:29) — Help is exactly that. | `README.md:143`: same generic claim, applies correctly here. | holds (behavior matches the *generic* doc claim; only the in-modal title text under-advertises it — noted in Divergences as cosmetic) |
| ? (help.go:125) | modals (help) | Same condition/line — `ev.Rune()=='?'` also closes the modal that `?` opened it with, a natural toggle. | Not mentioned in the title. | Not explicitly stated that `?` also *closes* Help (only that it *opens* it — help.go:105, ":help section" `?` row: "this help"). | Not stated (`README.md:92`: "`?` opens the full cheat sheet" — opening only). | doc-stale (minor): `?`-as-toggle-to-close isn't documented anywhere, only `?`-to-open. |
| j k / arrows / PgUp PgDn (scroll, tview TextView default) (help.go:129; vendor tview/textview.go:1341-1352) | modals (help) | Any key that isn't Esc/q/? falls to `return ev` (help.go:129), reaching `tview.TextView`'s own scrollable `InputHandler`. Verified against `vendor/github.com/rivo/tview/textview.go:1340-1381`: this natively binds not just `j`/`k`/arrows/PgUp/PgDn/Ctrl-F/Ctrl-B, but **also `g`/`G` (jump to top/bottom of the text) and `h`/`l` (horizontal scroll)** — a broader key set than the matrix row's own label states. None of this conflicts with app-level bindings since the modal owns all input while open. | Not applicable (title only says "Esc to close", no scroll hint). | Not documented (`:help` doesn't mention that the Help overlay itself is scrollable/its scroll keys). | Not documented. | holds, with a correction: the row's own citation (`textview.go:1341-1352`) undershoots — the full native binding set (verified at lines 1340-1381) is g/G/h/l/j/k/arrows/Home/End/PgUp/PgDn/Ctrl-F/Ctrl-B, not just "j k / arrows / PgUp PgDn". |
| Esc (conflicts.go:35-36) | modals (conflicts list) | `SetInputCapture` (`conflicts.go:34-51`): `ev.Key()==KeyEscape` (combined with `q`) → `a.closeModal(pageConflicts)`. | Title: " Conflicts — Enter to resolve · Esc to close " (conflicts.go:33) — inline hint; `a.hints` unaffected (F1). | Not itemized beyond the generic `q` row + the `:conflicts` command-row (help.go:104: "`:conflicts` — resolve items that changed on both sides"). | `README.md:156`: "resolve in-app with `:conflicts` (keep local / keep server)" — no key-level detail. | holds |
| q (conflicts.go:35-36) | modals (conflicts list) | Same line/condition as Esc. | Title omits `q` (see Divergences: minor under-advertising, same class as Help). | Covered by the generic `q` row (help.go:29). | Covered by the generic `q` row (`README.md:143`). | holds (same cosmetic caveat as Help's `q`) |
| j / k (conflicts.go:42-49) | modals (conflicts list) | `SetInputCapture` locally translates rune `j`→`KeyDown`, `k`→`KeyUp` (`conflicts.go:42-49`) before handing off, since the list is modal and `globalKeys`' app-wide hjkl-alias (`motionArrow`, `keys.go:147-164`) never reaches it. | Not shown anywhere (F1) — no on-screen j/k hint, though vim users will assume it. | Not documented — `:help`'s Panels row documents `hjkl` generally but doesn't call out that the Conflicts modal specifically re-implements the alias locally (an implementation detail, not user-visible difference — the *effect* is identical to elsewhere in the app). | Not documented (same reasoning). | holds — behavior is unsurprising/consistent with the rest of the app even though the doc doesn't name this modal specifically; no contradiction. |
| Enter (conflicts.go:64) | modals (conflicts list) | `list.AddItem(..., func() { a.chooseResolution(list, cc) })` (`conflicts.go:64`) — tview's `List` fires the item's selected-func on Enter by default, opening the resolve sub-dialog for the highlighted conflict. | Title says "Enter to resolve" (conflicts.go:33). | Matches conceptually (`:conflicts` row, help.go:104). | Matches conceptually (`README.md:156`). | holds |
| arrows (native List nav) (vendor tview/list.go default) | modals (conflicts list) | Confirmed via `vendor/github.com/rivo/tview/list.go:610-644`: `List`'s own `InputHandler` binds `Tab`/`Down`, `Backtab`/`Up`, `Home`, `End`, `PgDn`, `PgUp` — no rune cases at all (hence the local j/k shim above is necessary). | N/A. | N/A (arrows are the universally-assumed default, undocumented anywhere as this is standard). | N/A. | holds |
| Left / Right / Tab (button nav) (conflicts.go:70-98 (tview.Modal default)) | modals (conflict resolve dialog) | `chooseResolution` (`conflicts.go:69-102`) builds a plain `tview.NewModal()` with 3 buttons; button focus-nav (`Left`/`Right`/`Tab`/`Backtab`) is `tview.Modal`'s own default (`vendor/github.com/rivo/tview/modal.go:110-131`: each button's `SetInputCapture` remaps Down/Right→Tab, Up/Left→Backtab). | No inline hint text beyond the button labels themselves ("Keep local"/"Keep server"/"Cancel") and the modal's title " Resolve conflict " (conflicts.go:98, via `styleModal`). `a.hints` unaffected (F1). | Not documented at the key level (only the concept, "resolve each — keep the local edit or take the server version", is implied by `:conflicts`' description). | Not documented at the key level (`README.md:156`, concept only). | holds |
| Enter (activate button) (conflicts.go:70-98) | modals (conflict resolve dialog) | Enter (or Space, tview's Button default) activates the focused button, running `SetDoneFunc`'s handler (`conflicts.go:73-97`), which unconditionally removes the sub-dialog and restores focus to the list, then applies `ResolveKeepLocal`/`ResolveKeepServer` only for the matching label (default/`"Cancel"`/Escape's synthetic `""` label all just return, doing nothing further — confirmed Esc also closes this dialog via `tview.Modal`'s built-in `Form.SetCancelFunc`, `vendor/.../modal.go:43-47`, even though no Esc row exists in this Context bucket in the scaffold). | Same as above row. | Not documented at key level. | Not documented at key level. | holds |
| Esc (command.go:172 (list.SetDoneFunc)) | modals (account picker) | `accountPickerList` (`command.go:146-188`) wires `list.SetDoneFunc(func() { a.closeModal(pageAccount) })` (`command.go:172`) — `tview.List`'s own default `InputHandler` calls this on `KeyEscape` (`vendor/.../list.go:610-612`). | Title: " account " only (command.go:155) — **no key hint at all**, unlike Help/Conflicts. `a.hints` unaffected (F1). | `:help`'s generic `q` row claims `q` closes "a non-form dialog" — **false here** (see Divergences/F2). Otherwise not itemized. | `README.md:87` describes `:account`'s *effect* (switches account) but not its picker's close key. | **inconsistent-across-contexts** — see Divergences table. |
| j / k (command.go:173-186) | modals (account picker) | Same local rune-translation pattern as the Conflicts list (`command.go:173-186`): `j`→`KeyDown`, `k`→`KeyUp`, needed because `globalKeys` never reaches this modal's list. | Not shown (F1). | Not documented at this modal specifically (generic hjkl convention applies). | Not documented specifically. | holds |
| Enter (command.go:164-167) | modals (account picker) | `list.AddItem(label, "", 0, func() { a.closeModal(pageAccount); a.switchAccount(n) })` (`command.go:164-167`) — Enter (List's default select) closes the picker and calls `switchAccount`, which (if the target differs from `a.activeAccount`) calls `requestSwitch` → stops the event loop so `main`'s rebuild loop reopens on the new account. | No inline hint (title is bare, see Esc row above). | `:help`'s `:account` row: "switch account — `:account <name>`, or bare to pick from a list (multi-account)" (help.go:103) — matches at the concept level. | `README.md:87` matches at the concept level. | holds |
| Esc (colorpicker.go:137-140) | modals (color picker) | `colorPicker.InputHandler` (`colorpicker.go:132-167`): `case ev.Key()==KeyEscape: if p.onCancel != nil { p.onCancel() }` — wired at the call site to `a.closeModal(pageColor)` (`calendar.go:230`). | Title is caller-supplied and content-only, e.g. " Color · <name> " (`calendar.go:245`) — **no key hint at all**, same gap as the account picker. `a.hints` unaffected (F1). | `:calendar` row mentions the picker exists ("`color` with no hex opens the swatch picker", help.go:102) but no keys. Generic `q` row's blanket claim is **false here** too (see Divergences/F2). | `README.md:64` mentions the swatch picker exists, no keys. | **inconsistent-across-contexts** — see Divergences table (same root cause as the account picker). |
| Enter (colorpicker.go:141-142,169-179) | modals (color picker) | `case ev.Key()==KeyEnter: p.choose()` (`colorpicker.go:141-142`) → `choose()` (`colorpicker.go:169-179`): on the "Custom hex…" entry calls `p.onCustom()` (opens a text-input prompt, `calendar.go:216-229`), otherwise `p.onSelect(calendarPalette[p.cursor])` (`calendar.go:212-215`) applies the swatch and closes. | No inline hint. | Not documented at key level. | Not documented at key level. | holds |
| Left / Right / h / l (column move) (colorpicker.go:143-150) | modals (color picker) | `case ev.Key()==KeyLeft \|\| ev.Rune()=='h'`: cursor left one column if not already at the row start and not past the palette bound; mirrored for Right/`l` (`colorpicker.go:143-150`), clamped so it never overruns `colorPickerCols` (5) per row or the palette length. | No inline hint. | Not documented at key level (only that a swatch picker exists). | Not documented. | holds |
| Up / Down / j / k (row move) (colorpicker.go:151-165) | modals (color picker) | `case ev.Key()==KeyUp \|\| ev.Rune()=='k'`: moves up a row, or from the "Custom…" entry back to the last swatch row; mirrored for Down/`j`, dropping to "Custom…" once past the last swatch row (`colorpicker.go:151-165`). | No inline hint. | Not documented at key level. | Not documented. | holds |
| Enter (command.go:28-37) | modals (command line input) | `openCommandLine` (`command.go:18-42`): `in.SetDoneFunc` `case tcell.KeyEnter`: reads the typed line, `a.closeModal(pageCommand)`, then `a.runCommand(line)` (`command.go:45-88`) parses and dispatches. | Title: " command " (command.go:27) — bare, but the leading `:` label prefix (`in.SetLabel(":")`, command.go:19) itself is the affordance. `a.hints` unaffected (F1). | Matches conceptually: "cmd — :sync :view :goto …" (help.go:99). | Matches (`README.md:85`, lists all subcommands + aliases). | holds |
| Esc (command.go:28-37) | modals (command line input) | `case tcell.KeyEscape: a.closeModal(pageCommand)` — discards the typed line, no command runs. | Same as above. | Covered by the generic Esc row ("back out (a form/dialog too)", help.go:28). | Covered by the generic Esc row (`README.md:121`). | holds |
| Enter (search.go:36-58) | modals (search input) | `openSearch` (`search.go:22-63`): `in.SetDoneFunc` `case tcell.KeyEnter`: removes the search page; if a match is active, pops the focus stack (bypassing `restoreFocus`'s normal pop-then-refocus so the *matched* item keeps focus) and calls `a.setFocus(a.searchWidget())`; if no match, falls back to `a.restoreFocus()`. | Title: " search " (search.go:31) — bare; the leading `/` label is the affordance. `a.hints` unaffected (F1) — though note `runSearch` (`search.go:68-82`) itself calls `a.flash(...)` on every keystroke to show "/query (n/total)" in the left status area, which *does* give live incremental feedback (just not via `a.hints`). | Matches conceptually ("`/` then `n`/`N` — search; next/prev match", help.go:26). | Matches (`README.md:127`: "`/` · `n`/`N` — Search the current view · next/prev match"). | holds |
| Esc (search.go:36-58) | modals (search input) | `case tcell.KeyEscape`: clears `a.searchQuery`, removes the page directly (**not** via `a.closeModal`, unlike every other modal in this slice — it calls `a.root.RemovePage` + `a.searchRestore()` + `a.restoreFocus()` manually), restoring the pre-search selection. Functionally equivalent to `closeModal` for this case (no `a.formDrill` reset needed since search can't nest under a form, and no pending-sync re-arm is lost since opening search never defers a sync the way opening a form does) but is the one modal in this slice that doesn't route through the shared helper. | Same as Enter row. | Covered by the generic Esc row + the doc-comment-level description at `search.go:15-19` ("Esc cancels and restores the prior selection"), which is itself not surfaced in `:help`/README (only the generic Esc row applies). | Covered by the generic Esc row. | holds (behaviorally correct; noted only as an internal-consistency curiosity that it bypasses `closeModal` where every sibling modal in this slice uses it — not a doc-facing issue) |
| Left / Right / Tab (button nav) (generic tview.Modal default (delete confirm, recurrence-scope picker, config-reload notice, etc.)) | modals (generic confirm/choice dialog) | Two concrete call sites confirmed: `confirmOK` (`edit.go:1019-1033`, e.g. delete confirms) and `pickRecurrenceScope` (`recur_edit.go:30-63`, the recurrence-scope picker) both build a bare `tview.NewModal()` with no custom `SetInputCapture` — button nav is entirely `tview.Modal`'s own default (`vendor/.../modal.go:110-131`, remapping Down/Right→Tab and Up/Left→Backtab per button). No separate "config-reload notice" modal exists in the code (`applyConfigReload`, `command.go:209-244`, only calls `a.flash(...)`, never opens a modal) — the scaffold's example list overstates what's actually implemented as a modal, though the *class* (bare `tview.Modal` confirms) is real and correctly described. | No inline hint beyond the button labels + caller-supplied title (e.g. " Delete task ", " Recurring event "). `a.hints` unaffected (F1). | Not documented at key level (only the concept — e.g. "prompts scope: this occurrence / this & future / all", help.go:61-62). | Not documented at key level (`README.md:83`, concept only). | holds, with a scaffold-accuracy note: "config-reload notice" in the Context's own parenthetical is not an actual modal in the current code (it's a `flash()`), so it should not be relied on as a third example instance. |
| Enter (activate button) (generic tview.Modal default) | modals (generic confirm/choice dialog) | Enter activates the focused button via `tview.Modal`'s embedded `Form`, invoking `SetDoneFunc` (`edit.go:1023-1028` / `recur_edit.go:50-58`), which always calls `a.closeModal(pageConfirm)` then runs the matched action (or nothing, for Cancel). Esc also reaches this (via `Form.SetCancelFunc` → `done(-1,"")`, `vendor/.../modal.go:43-47`) even though no Esc row exists in this Context bucket in the scaffold — confirmed functionally equivalent to clicking Cancel. | Same as above row. | Not documented at key level. | Not documented at key level. | holds |

---

## 4. Divergences (raw — pre-verification)

> This section is **raw input for the next task**, not a triaged findings list: every entry below
> is copied verbatim from the five phase-2 verification slices' own "Divergences found" write-ups
> (`.superpowers/sdd/2026-07-24-v1.5.0-phase2-matrix/slice-{1..5}-*.md`). Nothing here has been
> deduplicated, re-verified, or adjudicated by the owner — several entries across slices describe
> the same root cause from different Context rows (e.g. the single-vs-bulk GRAB h/l/j/k swap
> appears in both Slice 1 and Slice 3; the `Y`/`P` README-table gap appears in Slices 1, 2, and 4).
> That overlap is intentional at this stage — collapsing it is the next task's job.

### Slice 1 — task tree

1. **Context · Key: GRAB · task tree `h l` / `j k`  vs.  GRAB (bulk, via SELECT) · task tree `h l` / `j k`** — `inconsistent-across-contexts`.
   Single-item task grab (`grab.go:214`, map `{'j':1,'k':-1,'l':7,'h':-7}`) uses **h/l = ±1 week, j/k = ±1 day**, exactly as README documents ("nudge a task's due date (`j`/`k` day, `h`/`l` week)", README.md:130). Bulk grab (`bulkgrab.go:107-114`) uses the opposite convention for the *same object type* — **h/l = ±1 day, j/k = ±1 week** (`bulkGrabStatus`, `bulkgrab.go:86`). Neither doc surface states the two modes intentionally swap the axes; README's `V` row (README.md:131) simply says "grab all (±day/±week)" directly under the row that just established the opposite mapping for single-item task grab. A user who learned single-grab's h/l=week convention gets the reverse in bulk on the very same tree. Recommended fix: **code** (pick one convention and make bulk match single-item grab) — or, if the swap is intentional (bulk mixes event/todo items so it standardizes on the event-grab h/l=day convention), **doc** (call out the exception explicitly in help.go's and README's bulk-grab entries).

2. **Context · Key: SELECT · task tree `f`, `b`** — `code-diverges`.
   help.go's Select section (`help.go:77`) lists `f`/`b` alongside `hjkl`/`gg`/`G` as range-extension keys ("`h j k l / f b / gg / G` — extend the range (motion incl. period shift...)") with no context qualifier. In the task tree, `f`/`b` are gated to `a.mode == modeCalendar` only (`app.go:984-993`); `handleSelectKey` passes them through unhandled (`selection.go:345`) but `globalKeys`'s `case 'f'`/`case 'b'` then no-ops for `modeTasks`, so they do **nothing** — they neither extend the range nor shift any period. help.go's phrasing implies they always work as a "period shift" motion during SELECT. Recommended fix: **doc** — scope the help.go row (e.g. "`f b` (calendar only)").

3. **Context · Key: SELECT · task tree `V`** — `doc-stale` (documentation gap, not a code bug).
   `V` pressed while already in SELECT calls `exitSelect()` + flashes "Select cancelled" (`selection.go:360-363`), i.e. it's a second, undocumented way out (mirroring `Esc`). None of the three doc surfaces (hint bar, `:help`, README) mention that `V` toggles off an active SELECT — help.go's `V` row (`help.go:76`) only describes entering SELECT, and the Select section's cancel row (`help.go:83`) only names `Esc`. Recommended fix: **doc** (add "`V` also cancels" to help.go's Select-section Esc row or the SELECT hint-bar string, `render.go:722`).

4. **Context · Key: NORMAL · task tree `Y`, `P`** — `doc-stale` (README table incomplete).
   README's keybindings table (the doc CLAUDE.md designates canonical) has only one combined row for `y`/`p` (README.md:129, "Yank / paste a task — move it..."), omitting `Y` (copy) and `P` (paste at top) as table rows entirely. Both keys are real, distinct, working bindings (`app.go:879`, `app.go:888`) and are correctly described in prose one section up (README.md:77, "`y`/`Y` cut/copy a task ... `p`/`P` paste") and in help.go (`help.go:67-68`). This is exactly the table/prose drift CLAUDE.md's README rules warn against — the table under-documents while prose (elsewhere) has it right. Recommended fix: **doc** (add `Y` and `P` rows to the README table, or fold them into the existing `y`/`p` row).

5. **Context · Key: NORMAL · task tree `>`, `<`** — `doc-stale` (README table incomplete).
   Same pattern as #4: `zoomInTree`/`zoomOutTree` (`render.go:261-289`, Tasks-mode gated, `app.go:1006-1015`) are correctly described in README's Usage prose (README.md:59, "`>` zooms into a subtree (`cd`-style, with a breadcrumb), `<` zooms back out") and in help.go (`help.go:66`), but have **no row at all** in the README keybindings table. Recommended fix: **doc** (add a `>` / `<` row to the table).

Everything else in this slice — 72 of 79 rows — holds: actual behavior matches every doc surface that mentions it, and silence in the hint bar (which is an intentionally curated subset, per its own code comment at `render.go:734`) is not counted against a row when `:help`/README cover it.

### Slice 2 — calendar grids (month + week/day), NORMAL and DRILL

1. **Enter — DRILL · month grid, DRILL · week-day grid** — `doc-stale`.
   `calendarView.handleEventMode` (`calendarview.go:143-187`) and `timeGridView.handleEventMode`
   (`timegridview.go:453-477`) have **no `tcell.KeyEnter` case** — Enter is a true, silent no-op
   once drilled (confirmed: `globalKeys` has no `KeyEnter` case either, `app.go:753-1019`, so the
   key reaches the grid's own `InputHandler` unhandled). `:help`'s own row gets this right —
   "dive in / open (a drilled day's events are then cycled with j/k/arrows, not Enter)"
   (`help.go:27`) — but two other surfaces don't: (a) the persistent hint bar still shows
   `"Enter open"` (`render.go:735`) with no adaptation for DRILL (see Method notes), and (b)
   README's Enter row — "Dive into the center; **cycle a day's events**; open a list / expand a
   task" (`README.md:120`) — reads as if Enter itself cycles a day's events once drilled, which is
   backwards: cycling is exclusively `j`/`k`/arrows (`calendarview.go:151-185`,
   `timegridview.go:458-465`). This is the exact question the MATRIX.md scaffold left open at
   §1.3 ("Whether this is reachable divergence or dead code … is left for the next task to
   verify", `MATRIX.md:56-58`) — resolved here: it's a real, permanent, structural no-op, not dead
   code and not a reachability question. Recommended fix: **doc** — reword the README Enter row so
   "cycle a day's events" isn't attached to Enter (e.g. split it: "Enter dives in; once drilled,
   `j`/`k`/arrows cycle the day's events"), and give the hint-bar string a DRILL variant the same
   way `updateStatus` already special-cases GRAB/SELECT.

2. **u — DRILL · month grid, DRILL · week-day grid** — `inconsistent-across-contexts`.
   `undoLast()` calls `a.refresh(step.selUID)` with the undone step's **non-empty** `selUID`
   (`edit.go:698-726`). `refresh()`'s calendar-mode branch only preserves the grid's drill state
   when `selUID == ""` (`edit.go:746-766`, comment: "preserve an active day-drill across the
   rebuild … a mutation that passes selUID uses `refreshKeepingDrill`, which captures the drill and
   also restores focus"). `undoLast` does neither: it doesn't route through
   `refreshKeepingDrill` (unlike `toggleComplete`, `edit.go:354`) and doesn't pass `""` (unlike
   `deleteWholeObject`'s confirm callback, `edit.go:475`, or a background sync's `refresh("")`).
   Net effect: pressing `u` while drilled (e.g. undoing a Space-complete that was itself performed
   drilled) silently kicks the grid back to day-navigation (NORMAL) — the one mutation-adjacent
   path in the whole app that doesn't honor the "don't kick the user out of a drilled day"
   guardrail every sibling path (`refreshKeepingDrill`, the delete confirm, and the form-save
   `captureFocus`/`restoreFocus` cycle, `edit.go:659-676`, `edit.go:879-924`) was written to
   enforce. `u` in NORMAL (this slice's other two rows) is unaffected — there's no drill to lose —
   so the divergence is specifically the NORMAL vs. DRILL difference for the same key. No doc
   surface promises undo preserves drill either way, so this is an internal-consistency finding,
   not a doc contradiction. Recommended fix: **code** — pass `""` from `undoLast` when
   `a.calendarPrimitive()` is currently drilled (mirroring the background-sync-refresh convention),
   or route it through `refreshKeepingDrill` the way `toggleComplete` does.

3. **Y — NORMAL · month grid, NORMAL · week-day grid** and **P — NORMAL · month grid,
   NORMAL · week-day grid** — `doc-stale` (README table incomplete).
   README's keybindings table has only one combined row, "`y` / `p` | Yank / paste a task — move it
   (and its subtree) to another parent or list" (`README.md:129`) — `Y` (copy, `app.go:879`→
   `yankpaste.go:53`) and `P` (paste-at-top, `app.go:888`→`yankpaste.go:68`) have **no row of their
   own**. Both are real, working, calendar-reachable bindings (via `currentTarget()`'s drilled-task
   case) and are correctly described in prose one section up ("`y`/`Y` cut/copy a task ... `p`/`P`
   paste", `README.md:77`) and in `:help` (`help.go:67-68`) — this is exactly the table/prose drift
   CLAUDE.md's own README rules warn against ("the keybindings table is the canonical key
   reference; prose must not re-narrate it"). Same finding independently surfaced in slice-1
   (task tree) — it's a doc-structure gap, not calendar-grid-specific, but this slice's rows are
   affected the same way. Recommended fix: **doc** — add `Y`/`P` rows (or fold into the existing
   `y`/`p` row) in README's table.

Everything else in this slice — 124 of 132 rows — holds: actual behavior matches every doc surface
that addresses it, and hint-bar silence on a key it never claimed to cover (see Method notes) is
not counted against a row.

### Additional observations (not counted as divergences — no doc contradicts either reading)

- **Search in Calendar mode targets the Calendars-overview list (calendar names), never the grid's
  events/tasks**, in both NORMAL and DRILL (`search.go:104-113,141-148`, the `default:` branch of
  `searchWidget`/`searchItems` — Calendar mode falls to it since it isn't `modeTasks`/`modeAgenda`).
  README's `/ · n / N` row ("Search the current view", `README.md:127`) is vague enough not to be
  contradicted, but a user could plausibly expect `/` to search event/task titles on a populated
  grid. Worth a documentation clarification, not a fix.
- **`v` (view cycle) and `.` (show-completed toggle) silently drop DRILL, while `f`/`b` (period
  shift) explicitly preserve it.** `v`'s handler calls `buildCenterCalendar()` with no re-drill
  (`app.go:973-980`, `render.go:108-134` — `setData` unconditionally resets `eventMode=false`);
  `.`'s handler calls `reloadCurrent()`→`buildCenterCalendar()` the same way (`app.go:969-972,
  1090-1104`). `shiftAnchor` (`f`/`b`), by contrast, captures `wasDrilled` up front and explicitly
  calls `g.reDrill(...)` after rebuilding (`app.go:1022-1047`). No doc promises drill persists
  across any of these, so each key's own row below is `holds` — but the three period/view-changing
  keys aren't internally consistent with each other, which is worth a design pass.
- **`calendarView`'s per-rune `h`/`j`/`k`/`l` cases are dead code.** `globalKeys`'s `motionArrow`
  block (`keys.go:147-164`, invoked at `app.go:803-813`) unconditionally translates every `h`/`j`/
  `k`/`l` keypress to the matching arrow key *before* the switch that would otherwise let a raw rune
  fall through to the focused widget, and does so regardless of NORMAL/DRILL/month/week-day (the
  only gates are `modalOpen`/`grabbing`/`resizing`, none of which apply here). Since
  `a.tv.SetInputCapture(a.globalKeys)` is registered at the `Application` level (`app.go:435`), a
  raw `KeyRune` `'h'`/`'j'`/`'k'`/`'l'` can never reach `calendarView.handleDayMode`/
  `handleEventMode`'s own rune cases (`calendarview.go:129-139,173-185`) — they're unreachable via
  any keyboard path. `timeGridView` has no such rune cases at all (`timegridview.go:443-477`) and
  loses nothing, because the global translation already guarantees an arrow-key event arrives
  either way. This resolves the MATRIX.md scaffold's open question (§1.3, `MATRIX.md:54-58`) about
  reachability. Not a behavior bug (observable behavior is correct in both grids) — a code-cleanup
  candidate (remove the dead cases in `calendarView`), not a doc/behavior divergence.

### Slice 3 — calendar grids in SELECT/GRAB/bulk-GRAB, plus the agenda board

1. **SELECT · month grid / week-day grid — `gg`, `G`** — `code-diverges`.
   The persistent SELECT hint bar (`render.go:722`) reads: `"SELECT · hjkl extend · gg/G ends ·
   Space done · d delete · y/Y yank · m grab · Esc/V cancel"` — it literally says **"gg/G ends"**.
   But `gg`/`G` do not end SELECT: `handleSelectKey` passes both through unhandled
   (`selection.go:345,358-359`) precisely so they **extend the range** by moving the cursor to the
   top/bottom (`gotoTop`/`gotoBottom`, keys.go:184-195,233-270) — the same "extend" contract as
   `hjkl`/`f`/`b`. `:help`'s own Select-section row (`help.go:77`) confirms this: `"h j k l / f b /
   gg / G — extend the range"`. Only `Esc`/`V` actually end SELECT (`selection.go:332-335,360-363`).
   The live hint bar's wording contradicts both the actual behavior and the app's own `:help` text.
   Recommended fix: **code** — change `render.go:722`'s `"gg/G ends"` to `"gg/G extend"` (or drop
   the clause; `hjkl extend` already covers it).

2. **NORMAL · agenda board — `h`/`l` (as part of the `h j k l / arrows` motion row)** —
   `code-diverges`.
   The agenda board is driven entirely by `a.agendaList` (a `tview.List`; the board itself "takes
   no focus", `agendaboard.go:15-16,34-37`). `motionArrow` translates `h`→Left, `l`→Right
   (keys.go:153-160) and `repeatKey` feeds that straight to the focused primitive's own
   `InputHandler` (app.go:803-813). But `tview.List.InputHandler`'s `KeyLeft`/`KeyRight` cases
   shift `horizontalOffset` by ∓2/±2 (vendor `tview/list.go:628-631`) — they do **not** move
   `currentItem`. Only `j`/`k` (Down/Up) actually move the highlighted agenda item
   (`list.go:624-627`). Every doc surface (persistent hint "hjkl move", `:help`'s "move the
   highlight", README's "Move the highlight … `h` `l`") states `hjkl` uniformly moves the highlight;
   in the agenda board, `h`/`l` silently do nothing visible instead. Recommended fix: **code** — add
   an explicit `h`/`l` no-op (or an intentional binding) for `agendaList`, or document the exception.

3. **GRAB · week-day grid — `j`/`k` (hour nudge) and `J`/`K` (resize)** — `code-diverges`.
   `grabTimeHint` (grab.go:156-161) assumes that "not timed" always means "wrong view": `if
   a.mode == modeCalendar { return "switch to week/day view (v) to " + action }`. But `timed` is
   `a.mode == modeCalendar && a.viewMode != viewMonth && !base.AllDay` (grab.go:244) — it can also be
   false because the grabbed item is an **all-day event**, even while already sitting in the
   week/day time-grid. Grabbing an all-day event there and pressing `j`/`k`/`J`/`K` flashes
   "switch to week/day view (v) to change the time" / "…to resize" — a factually wrong message,
   since the user is already in that view. Recommended fix: **code** — `grabTimeHint` (or its
   caller) needs a distinct message for the AllDay-in-week/day-view case (e.g. "all-day events have
   no time to nudge here").

4. **GRAB · month grid / week-day grid / agenda board — `J`/`K` on a grabbed *task*** —
   `inconsistent-across-contexts`.
   For an event, `J`/`K` always produce feedback: either the resize (if timed) or an explanatory
   flash via `grabTimeHint` (grab.go:281-298). For a **todo**, the nudge map has no `J`/`K` entries
   (`map[rune]int{'j':1,'k':-1,'l':7,'h':-7}`, grab.go:214) — `days == 0` and the function just
   `return`s (grab.go:215-217): **zero feedback, in every grid/agenda context**. Contrast this with
   bulk grab's parallel case, which explicitly flashes `"Resize doesn't apply to a multi-selection"`
   for `J`/`K` (`bulkgrab.go:115-117`) regardless of item type. Single-item grab on a task is
   silently inert where bulk grab on the same key is helpfully vocal. Recommended fix: **code** —
   add a `J`/`K`-on-todo flash mirroring bulk grab's, in `grabNudge`'s todo branch.

5. **GRAB · month grid — `j`/`k` (hour nudge), `J`/`K` (resize)** — `doc-stale`.
   Both are unconditionally blocked in month view (`timed` is always false when `viewMode ==
   viewMonth`, grab.go:244) — this is intentional and correctly reflected in `grabStatus()`'s month
   branch, which omits `j/k`/`J/K` from its hint text entirely (grab.go:145-149, default case).
   Neither `:help` (`help.go:69`, one blanket "move an event (hjkl day/hour, J/K resize)" line) nor
   README (`README.md:130`, same blanket phrasing) mentions that hour-nudge/resize require the
   week/day grid — a reader could reasonably expect `j`/`k`/`J`/`K` to work from the month grid too.
   Recommended fix: **doc** — note the week/day-only restriction next to the `m` row in both
   surfaces (README already does this correctly for the *todo* h/l-vs-j/k axis; it just doesn't
   scope the event hour/resize keys to a specific grid).

6. **NORMAL · agenda board — `Enter`** — `doc-stale`.
   `a.agendaList` has no `SetSelectedFunc` and every item's `Selected` callback is `nil`
   (`render.go:78,82`), so `tview.List`'s default `KeyEnter` handling (`list.go:648-657`) does
   nothing — Enter is a complete no-op in the agenda board (selection already lives on the
   highlight; there's no deeper level to dive into, unlike the tree's fold-toggle or the calendar's
   day-drill). README's Enter row (`README.md:120`) enumerates "Dive into the center; cycle a day's
   events; open a list / expand a task" — a per-context list that covers task tree and calendar but
   never mentions Agenda, so a reader has no way to learn Enter is inert there. `:help`'s Enter row
   (`help.go:27`) has the same gap. Recommended fix: **doc** — add "(no effect in Agenda — the
   highlight already drives the board)" to both rows.

7. **NORMAL · agenda board — `i!`** — `doc-stale`.
   `i!` (arm-force-create, `keys.go:87-91`) is a real, working chord continuation, documented in
   `:help` (`help.go:46`, "i ! e / i ! t") and in README's Usage prose (`README.md:71`, "unless you
   force it with `i!`") — but it has **no row in the README keybindings table**
   (`README.md:113-144`), which CLAUDE.md's own README rules designate the canonical key reference.
   Recommended fix: **doc** — add `i!` to the table's `i …` row or its own row.

8. **NORMAL · agenda board — `Y`** — `doc-stale`.
   README's keybindings table has only one combined row, `` `y` / `p` `` (`README.md:129`,
   "Yank / paste a task — move it…"). `Y` (copy, `app.go:879`→`copyTask`) is a distinct, real key —
   correctly described in `:help` (`help.go:67`, "y / Y") and in Usage prose (`README.md:77`,
   "`y`/`Y` cut/copy") — but is entirely absent from the table itself. (Functionally `Y` works
   identically everywhere `y` does; this is a documentation-completeness gap, not a behavior bug.)
   Recommended fix: **doc** — add `Y` to the table row (and see #9 for `P`).

9. **NORMAL · agenda board — `p`, `P`** — `doc-stale`.
   `paste()` (yankpaste.go:76-84) gates on `a.mode != modeTasks` and flashes `"Switch to a task list
   (t) to paste"` otherwise — so **both `p` and `P` are no-ops (with a flash) from the agenda
   board**, and from the calendar grids too. Neither `:help` (`help.go:68`, "p / P — paste under the
   selected task / at the list top level") nor README documents this Tasks-mode-only restriction —
   both read as if paste works wherever a task is targeted, mirroring `y`/`Y`/`m`/`s…`, which do
   not have this restriction. `P` additionally has the same table-omission gap as `Y` (#8): README's
   table row 129 lists only `y`/`p`, never `P`. Recommended fix: **doc** — state the Tasks-mode
   restriction next to the `p`/`P` entries in `:help` and README, and add `P` to the README table.

10. **NORMAL · agenda board — `+`** — `doc-stale`.
    `setAccordion(true)` explicitly refuses in Agenda mode: `if on && a.mode == modeAgenda { a.flash
    ("Expand isn't available in Agenda"); return }` (keys.go:515-518) — so `+` is a pure no-op
    (with a flash) in the agenda board; only `-` (restore, unconditional) has any effect, and even
    that is a no-op in practice since Agenda always starts un-collapsed (`setMode` restores the
    accordion on every mode switch, app.go:716-724). Neither `:help` (`help.go:93`, "+ / - / 0 …
    else: +/- collapse / restore the overview and Detail") nor README (`README.md:138`, same
    phrasing) mentions the Agenda-mode exception, even though the code comment right next to the
    gate spells out the reason ("Agenda's center navigation is driven by the (left) agenda list",
    keys.go:509-510). Recommended fix: **doc** — note the Agenda exception in both surfaces.

11. **SELECT · month grid / week-day grid — `V`** — `doc-stale`.
    Pressing `V` again while already in SELECT calls `exitSelect()` + flashes "Select cancelled"
    (`selection.go:360-363`) — an undocumented second way out, mirroring `Esc`. The live hint bar
    correctly shows `"Esc/V cancel"` (`render.go:722`), but `:help`'s `V` row (`help.go:76`) only
    describes *entering* SELECT, and its cancel row (`help.go:83`) names only `Esc`; README's `V`
    row (`README.md:131`) likewise only says `Esc` cancels. Recommended fix: **doc** — add "`V`
    also cancels" to `:help`'s Esc row (or the `V` row itself) and to README.

12. **SELECT · month grid / week-day grid — `Space`** — `doc-stale`.
    `bulkComplete()` silently skips every non-todo (event) target, counting it under
    `skips.add("event(s)")` (`bulkops.go:90-95`) — a calendar day-range selection very plausibly
    contains events. `:help`'s generic `"(skips)"` row (`help.go:82`) enumerates "recurring,
    read-only, missing, already-done, or open-subtask folders" as skip reasons but never mentions
    events — a reader of the cheat sheet has no way to learn that Space over a mixed day-range
    silently drops every event from the completion (only the runtime flash reveals it).
    Recommended fix: **doc** — add "event(s) (Space only completes tasks)" to the `(skips)` row.

Everything else in this slice — 86 of 98 rows — holds: actual behavior matches every doc surface
that mentions it, and silence in the persistent hint bar (an intentionally curated subset per its
own code comments, `render.go:731-734`) is not counted against a row when `:help`/README cover it.

### Slice 4 — the three overview panels

1. **Context: all three overviews · Key: `h`/`l` (part of "h j k l / arrows")** — README.md:117
   ("Move the highlight in the focused pane") and help.go:23 ("move the highlight") both claim
   `hjkl`/arrows move the highlight uniformly. On the three flat `tview.List` overview panels only
   `j`/`k` (→ `KeyDown`/`KeyUp`) move the highlighted row; `h`/`l` (→ `KeyLeft`/`KeyRight`) shift the
   `List`'s internal horizontal-scroll offset (`vendor/github.com/rivo/tview/list.go:628-631`) and
   never move the current item. Visually inert in practice (row text rarely overflows), but it
   contradicts the "move the highlight" claim literally. **Recommended fix (doc)**: add a footnote
   to the README table row / help.go entry that `h`/`l` scroll rather than move the highlight on
   the three flat overview lists (only `j`/`k`/arrows-vertical do there).

2. **Context: NORMAL · Tasks overview · Key: `>` / `<`** — README's Keybindings table (README.md
   §Keybindings, lines 111-144) has **no row at all** for `>`/`<` (subtree zoom); it exists only in
   prose (README.md:59, "`>` zooms into a subtree… `<` zooms back out"). This is the CLAUDE.md
   house rule inverted: "the keybindings table is the canonical key reference; prose must not
   re-narrate it" implies every key belongs in the table, with prose reserved for concepts — here
   the key is prose-only. help.go:66 documents it correctly (`"> / <"`). **Recommended fix (doc)**:
   add a `>` / `<` row to the README table (e.g. next to `H`/`L`).

3. **Context: all three overviews · Key: `Y` / `P`** — same class of gap: README's table row 129
   lists only `y` / `p` ("Yank / paste a task…"); `Y` (copy) and `P` (paste-at-top) appear only in
   prose (README.md:77, "`y`/`Y` cut/copy… `p`/`P` paste"). help.go:67-68 documents both correctly.
   **Recommended fix (doc)**: expand the README table row's Key column to `` `y` `Y` / `p` `P` ``.

4. **Context: NORMAL · Agenda overview · Key: `Enter`** — Enter dives focus into the center pane
   from Calendars overview (`a.calendars.SetSelectedFunc`, app.go:601) and Tasks overview
   (`a.tasklists.SetSelectedFunc`, app.go:623), but is a **true no-op** on the Agenda overview:
   `a.agendaList` has no `SetSelectedFunc` and its items carry no `Selected` callback
   (`buildAgendaLeft`, render.go:74-84, `AddItem(..., nil)`), so tview's default List Enter handler
   (`vendor/.../list.go:648-657`) fires nothing. This matches the deliberate "no keyboard drill for
   the agenda board" design already noted in MATRIX.md §2.2, but README.md:120 and help.go:27 both
   state the generic "Enter: dive in / open" without carving out the Agenda exception, so a reader
   would reasonably expect Enter to do *something* from the Agenda overview. **Recommended fix
   (doc)**: note in README/help.go that Enter has no effect from the Agenda overview/board
   (selection there is `j`/`k` + mouse only, per the existing gap-closer-A design).

### Additional finding (not a scaffolded row — flagged for the record)

- **MATRIX.md §2.2's "GRAB × Calendars/Tasks/Agenda overview" dropped-combination reasoning is only
  half right.** Its stated reason is that `startGrab`'s target resolver `currentTarget()`
  (edit.go:75-98) "has… none [of its cases] for the collection-picker overview lists themselves."
  True for **Calendars overview**: `currentTarget()`'s `modeCalendar` case reads the calendar grid's
  `selectedItem()`, which is `nil` whenever nothing is drilled — and NORMAL-by-definition means not
  drilled, so `m` genuinely can't resolve a target there (verified below, row `m`). But for
  **Tasks overview** and **Agenda overview**, the `modeTasks`/`modeAgenda` cases read
  `a.tree.GetCurrentNode()` / `a.agendaList.GetCurrentItem()` directly — **neither checks
  `a.tv.GetFocus()`** — so `m` pressed while the *overview list* holds focus resolves the tree's/
  agenda list's own current selection and **does** enter GRAB (`startGrab`→`beginGrab`,
  grab.go:26-78) whenever that selection is a grabbable item. This is intentional (same
  "quick-set works wherever a task is selected" design already commented at app.go:869-874), not a
  bug — see row `m` below for the confirmed behavior — but the dropped-combination note in
  MATRIX.md §2.2 should be corrected to scope the "dropped" claim to Calendars overview only. Not
  counted in the divergence count above since it targets the matrix's own background reasoning,
  not one of my four doc columns.

- **Dead/stale string, not reachable from any of my rows**: `deleteCollection`'s `default` branch
  (calendar.go:284-286) flashes `"Switch to Calendars (1) or Tasks (2) to delete a list"` —
  referencing keys `1`/`2` from an evidently older mode-switch scheme (today it's `c`/`t`/`a`).
  `deleteCollection` is only ever called from `deleteContextual` (keys.go:123-132) when
  `a.tv.GetFocus()` is `a.calendars` or `a.tasklists`, and focus can only be on those two when
  `a.mode` is already `modeCalendar`/`modeTasks` respectively (`setMode`, app.go:708-742; the mouse
  path enforces the same invariant, mouse.go:38-50) — so this branch is unreachable, and `d` from
  Agenda overview never touches it (it falls to `deleteSelected` instead — see row `d`). Flagged as
  a code-hygiene note, not a matrix-row divergence.

**Divergence count (scaffolded rows only): 4 findings above, spanning 6 rows** (`h`/`l` in 3 rows
counted under "h j k l / arrows"'s 3 context rows is one finding but affects all 3 rows; `>`/`<` is
1 row; `Y`/`P` is one finding across 3 rows each = 3+3 rows; `Enter`/Agenda is 1 row). Concretely:
**`code-diverges`/`doc-stale`/`inconsistent-across-contexts` rows: 3 (h/l) + 1 (`>`) + 1 (`<`) + 3
(`Y`) + 3 (`P`) + 1 (`Enter`/Agenda) = 12 of 155 rows.**

### Slice 5 — forms, modals, RESIZE sub-mode

| Context | Key | Mismatch | Recommended fix |
|---|---|---|---|
| modals (account picker) | Esc (command.go:172) | `:help`/README's blanket "`q` … closes a non-form dialog" is false here — no `q` case exists in the account-picker's list (`SetInputCapture`, `command.go:173-186`) or in `tview.List`'s own default handler. Title also carries no hint (unlike Help/Conflicts). | Code: add a `q` case to `accountPickerList`'s `SetInputCapture` (mirror the Conflicts list) — OR — Doc: caveat the `q` row in `help.go`'s "Panels & navigation" section and `README.md:143` to name the account/color pickers as Esc-only exceptions. |
| modals (color picker) | Esc (colorpicker.go:137-140) | Same root cause as above: `colorPicker.InputHandler` (`colorpicker.go:132-167`) has no `'q'` case, contradicting the same blanket `:help`/README claim. Title carries no hint either. | Same two-option fix as the account-picker row above; apply to both together since they share the root cause (a `q` case is missing from both non-`tview.Modal`, non-`List`-with-`SetDoneFunc` custom widgets). |
| modals (help) | q (help.go:125) | Minor: the modal's own title ("Esc to close") only advertises `Esc`, not `q`/`?`, even though both also close it per code and per `:help`'s own generic `q` row. Not a hard divergence (the behavior is documented elsewhere), but the in-modal chrome under-advertises. | Doc/cosmetic only: extend the title to " Help — keys & commands (Esc/q/? to close) " for parity with how thoroughly the Forms/Layout `:help` rows are written. |
| modals (conflicts list) | q (conflicts.go:35) | Same minor under-advertising as Help: title says "Esc to close" only, omitting the also-functional `q`. | Same cosmetic fix: extend the title text. |
| forms (NORMAL) | Tab (forms.go:219-221) | `:help`'s "Forms (full dialogs)" section and `README.md:73` document NORMAL field-stepping only as "j / k / ↑ / ↓" — `Tab` is never named as a synonym, even though `normalKey` (`forms.go:219`) treats `KeyTab` identically to `KeyDown`. A reader who only has the keybindings table (`README.md:116`: "Tab / Shift-Tab — Cycle those three [panels]") would reasonably assume Tab always cycles the overview panels; inside a form it is silently repurposed to field-nav instead (the form's own `SetInputCapture` intercepts it before `globalKeys`' Tab-cycle ever runs, since `modalOpen()` gates it out at `app.go:761-763`). | Doc: add "Tab / Shift-Tab" as an explicit synonym in `:help`'s Forms section's first row (and a one-clause note in `README.md`'s form paragraph), matching what the code already does. |
| forms (NORMAL) | Shift-Tab (forms.go:222-224) | Same root cause/fix as the Tab row above (`KeyBacktab` aliased to `KeyUp` in `normalKey`, undocumented as a Tab-family key inside forms). | Same fix as above. |
| forms (DRILL) | Enter / Tab (forms.go:302-304) | `:help`'s Forms section documents only "Enter (in DRILL)" → "commit the field and advance to the next"; `Tab` is not named as a synonym even though `drillKey` (`forms.go:302`) treats `KeyTab` identically to `KeyEnter`. | Same class of fix: name Tab alongside Enter in the DRILL row. |
| RESIZE (global sub-mode) | (all 9 rows — Enter, Ctrl-W, Esc, Left/Right, h, l, H, L, q) | The bottom Help bar (`a.hints`) is never updated for RESIZE (see F1); `updateStatus()`'s hint logic (`render.go:706-736`) has no `a.resizing` branch, unlike GRAB and SELECT which do get bespoke `a.hints` text. The correct, accurate hint string *does* get shown to the user — but via `a.flash()` (`keys.go:351`), which writes to `a.statusLeft` (the left "context" section, e.g. "Calendar · Jul 24 · …"), not `a.hints`. This is inconsistent with the sibling sub-modes GRAB/SELECT, whose per-keystroke hints live in the bottom bar as designed. | Code: give RESIZE the same treatment as GRAB/SELECT — add a case in `updateStatus()`'s hint block so `a.hints` (not just the one-shot `flash()`) shows the RESIZE hint for the duration of the sub-mode, or explicitly document (code comment + this guardrail file) that RESIZE deliberately uses the left/status-context slot instead of the hint bar. |

### Cross-cutting / non-row findings

These aren't tied to one scaffolded key×context cell — they're patterns the slices noticed while
filling adjacent rows. Raw, uncollapsed, same as above.

- **(a) The bottom Help bar / `a.hints` never updates for any form/modal/RESIZE context.**
  `updateStatus()` (`render.go:706-736`) only branches its `a.hints` text for `a.grabbing`/
  `a.selecting`, with one fixed fallback line otherwise — there is no branch for `a.resizing`,
  `a.modalOpen()`, or `a.formDrill` anywhere in the file (`grep -rn "hints.SetText"` = exactly those
  four sites). Confirmed independently by Slice 5 (its "F1", `render.go:706-736`), Slice 1 (its
  closing note on the curated NORMAL hint string, `render.go:734-735`), Slice 2 (Method notes,
  `render.go:686-736`), Slice 3 (`render.go:731-734`), and Slice 4 (`render.go:706-736,731`).
- **(b) The account picker and color picker lack the `q`-close that `:help`/README claim for every
  non-form dialog.** `:help` (`help.go:29`) and `README.md:143` both state `q` closes "a non-form
  dialog" as a blanket rule; Help (`help.go:125`) and Conflicts (`conflicts.go:35`) honor it, but
  the account picker's list (`command.go:172-186`, `tview.List.SetDoneFunc`, no rune case) and the
  color picker (`colorpicker.go:132-167`, `InputHandler` has no `'q'` case) don't — confirmed
  against `tview.List`'s own default handler too (`vendor/github.com/rivo/tview/list.go:610-644`,
  no rune cases at all). Slice 5's "F2".
- **(c) Single-item task GRAB uses h/l=±week, j/k=±day, while bulk GRAB uses the opposite mapping
  for the same object type.** Single-item: `grab.go:214` map `{'j':1,'k':-1,'l':7,'h':-7}` — h/l
  = ∓/±1 week, j/k = ±1 day (matches README.md:130). Bulk: `bulkgrab.go:97-114` — h/l = ∓/±1 day,
  j/k = ±1 week (the opposite axis assignment), per `bulkGrabStatus` at `bulkgrab.go:86`. Neither
  doc surface states the swap is intentional. Slice 1 divergence #1; corroborated in Slice 3's
  `GRAB · month grid` / `GRAB (bulk, via SELECT)` rows.
- **(d) The NORMAL hint bar shows "f/b prev/next · v view" in Tasks mode, where both are no-ops.**
  The fixed hint string (`render.go:735`) is identical across all modes; `f`/`b`/`v` are gated to
  `a.mode == modeCalendar` (`app.go:973-993`) and silently no-op in Tasks mode's task tree/overview.
  Slice 1's closing cross-cutting note; also flagged in Slice 4 ("Note on the hint-bar cells above").
- **(e) `u` (undo) while drilled drops grid drill state, unlike sibling mutations.** `undoLast()`
  calls `a.refresh(step.selUID)` with a non-empty `selUID` (`edit.go:698-726`), but `refresh()`'s
  drill-preserving branch only fires when `selUID == ""` (`edit.go:746-766`) — unlike
  `toggleComplete` (routes through `refreshKeepingDrill`) or the delete confirm (passes `""`). `u`
  while drilled silently kicks the grid back to NORMAL day-navigation. Slice 2 divergence #2
  (`inconsistent-across-contexts`).
- **(f) `calendarView`'s per-rune h/j/k/l cases are unreachable dead code.** `globalKeys`'s
  `motionArrow` (`keys.go:147-164`, invoked at `app.go:803-813`) unconditionally translates every
  raw `h`/`j`/`k`/`l` keypress to the matching arrow key before any widget-level rune case can ever
  see it — `calendarView.handleDayMode`/`handleEventMode`'s own rune cases
  (`calendarview.go:129-139,173-185`) can never fire via any keyboard path. Resolves the open
  question MATRIX.md §1.3 left for verification. Slice 2's "Additional observations" (third bullet).
- **(g) MATRIX.md §2.2's "GRAB × Calendars/Tasks/Agenda overview" drop is only half right — `m`
  DOES enter GRAB from Tasks overview and Agenda overview.** `currentTarget()`'s `modeTasks`/
  `modeAgenda` cases (`edit.go:75-98`) read `a.tree.GetCurrentNode()`/`a.agendaList.GetCurrentItem()`
  directly and **never check `a.tv.GetFocus()`** — so `m` pressed while the overview list itself
  holds focus resolves the tree's/agenda list's current selection and enters GRAB. Only
  **Calendars overview** is correctly dropped (the grid's `selectedItem()` is genuinely `nil` when
  undrilled). MATRIX.md §2.2's dropped-combination note needs to be re-scoped to Calendars overview
  only, and the "GRAB · Tasks overview" / "GRAB · Agenda overview" cells (dropped from the scaffold)
  need restoring as real rows. Slice 4's "Additional finding" (confirmed live in its `m` rows under
  the Yank/copy/paste table, `edit.go:75-98`).
- **(h) The SELECT hint bar (`render.go:722`) says "gg/G ends" but gg/G actually EXTEND the range.**
  `handleSelectKey` passes both through unhandled specifically so they extend the range via
  `gotoTop()`/`gotoBottom()` (`selection.go:345,358-359`, `keys.go:184-195,233-270`) — the same
  "extend" contract as hjkl/f/b. Only `Esc`/`V` actually end SELECT. `:help`'s own row
  (`help.go:77`) gets this right; the hint bar's wording contradicts both the code and `:help`.
  Slice 3 divergence #1 (`code-diverges`); corroborated in Slice 1's `gg`/`G` SELECT rows.
- **(i) Grabbing an all-day event in week/day view shows a misleading "switch to week/day view"
  error.** `grabTimeHint` (`grab.go:156-161`) assumes "not timed" always means "wrong view", but
  `timed` (`grab.go:244`) is also false for an all-day event even while already in the week/day
  time-grid — so `j`/`k`/`J`/`K` on a grabbed all-day event flashes a factually wrong message
  telling the user to switch to a view they're already in. Slice 3 divergence #3
  (`code-diverges`).
- **(j) A dead/stale flash string in `deleteCollection` references obsolete 1/2 mode keys.**
  Its `default` branch (`calendar.go:284-286`) flashes `"Switch to Calendars (1) or Tasks (2) to
  delete a list"` — an evidently older mode-switch scheme (today it's `c`/`t`/`a`). The branch is
  unreachable from any current input path (`deleteContextual`, `keys.go:123-132`, only calls
  `deleteCollection` when focus is already `a.calendars`/`a.tasklists`), so this is stale/dead code
  rather than a live user-facing bug. Slice 4's code-hygiene note.
- **(k) Help-modal scroll: the scaffold's own citation undershoots native tview's bindings (also
  g/G/h/l).** MATRIX.md's scaffold row cites `vendor/.../textview.go:1341-1352` for the Help
  overlay's scroll keys ("j k / arrows / PgUp PgDn"); re-reading the vendor code
  (`textview.go:1340-1381`) shows `TextView`'s native `InputHandler` also binds `g`/`G` (jump to
  top/bottom) and `h`/`l` (horizontal scroll) — a broader key set than either the scaffold or any
  doc surface states. Slice 5's filled `j k / arrows / PgUp PgDn` row for `modals (help)`.
