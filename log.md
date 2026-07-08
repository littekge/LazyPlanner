# LazyPlanner — Change Log

> Append a new entry every time a change is made. Newest entries at the top.

---

## 2026-07-08 — Lock item creation to a calendar's type (events vs tasks)

- Owner request: sharpen the fuzzy calendar/task-list split — events only on event calendars, tasks/subtasks only on todo lists, either on a "both" calendar — using the component set now tracked per calendar.
- **Gate** (`internal/ui/calendar.go` `guardComponent`): event creation requires **VEVENT**, task/subtask requires **VTODO**; a wrong-type attempt is refused with an explanatory flash (e.g. *"Errands is a task list — can't add events"*). Wired into `eventCreateContext` (VEVENT) and `taskCreateContext`/`subtaskContext` (VTODO), alongside the existing read-only `guardWrite`. Owner's policy for an **unknown/unconfirmed** component set: **block until known** (declared set required) rather than guessing from contents — so it's refused with "unknown type — sync it first" until a sync settles it.
- **"Both" made explicit** (`componentsForType`): the in-app create form's "Both" now records `["VEVENT","VTODO"]` instead of an empty set, so a both-calendar's type is *known* immediately (empty = unknown under the block-until-known rule). MKCALENDAR still gets both components.
- **Type marker** (`render.go` `calTypeMarker`): the Calendars overview tags each row `[events]`/`[tasks]`/`[both]`, or `[?]` when the component set is unknown — making the gate self-explanatory (and why `[?]` blocks). Sits with the existing `[ro]`/`(hidden)` markers.
- Fixture: gave `personal`'s sidecar `"components": ["VEVENT","VTODO"]` (it holds a standup event + a grocery todo), matching what a real sync would record — so the chord create-task test isn't blocked by an unknown type.
- Tests: `guardComponent` table (event/task/both allowed correctly, unknown blocked both ways), `calTypeMarker` table, `componentsForType("Both")` = explicit set, and the Calendars panel renders `[both]` for personal / `[?]` for work. Full gate + `-race` pass.
- Note: interactive pty capture of this was flaky (tview + pty in this env); verified headlessly instead. Also observed (pre-existing, out of scope) that `j`/`k` don't move the overview *lists* — tview's List binds only arrows — while they do in the task tree.

## 2026-07-08 — Fixes: overview pane titles, empty task lists, visibility-toggle selection

- Three owner-reported bugs from exercising the step-10 finale keymap.
- **Pane titles still showed `1`/`2`/`3`** (`internal/ui/app.go` `build`): the overview boxes were decorated `1 Calendars`/`2 Tasks`/`3 Agenda` from before the remap. Now `c Calendars`/`t Tasks`/`a Agenda`, matching the actual focus keys.
- **Empty task lists were invisible** (`internal/ui/render.go` `buildTasklists`): the panel only listed calendars with `todos > 0`, so a freshly-created (or emptied) task list never appeared — you couldn't add tasks to it. New `supportsTodos` predicate: list a calendar when its supported component set includes **VTODO** (so an empty list shows), falling back to "has todos" when the component set is unknown. To make imported empty lists recognizable, sync now records the server's `supported-calendar-component-set` (already surfaced by go-webdav) via new `store.SetCalendarComponents` (no-op when unchanged), called per calendar in `Sync`.
- **Hiding a calendar jumped the selection to the top** (`internal/ui/keys.go` `afterVisibilityChange`): rebuilding the Calendars list (`Clear`/`AddItem`) parks the cursor at index 0. Now the current row is captured and restored around the rebuild — since hiding marks a calendar rather than removing it, its index is stable, so the cursor stays on the calendar you just toggled.
- Docs: `main.md` task-list description updated (task lists = VTODO-supporting calendars incl. empty).
- Tests: ui — pane titles are `c/t/a`, `supportsTodos` table, an empty in-app VTODO list appears in the Tasks panel, hiding keeps the calendar selection (`bugfix_test.go`); sync — an imported empty VTODO calendar records its component set. Full gate + `-race` pass. Pty: launch renders `c Calendars`/`t Tasks`/`a Agenda`, exit 0, no panic.

## 2026-07-08 — Build step 10 finale (part 12): `:calendar rename|color|hide|show` — step 10 finale complete

- Final finale increment: edit a calendar's server-owned metadata in-app. **Step 10 finale complete** (all seven owner-requested keybinds/commands landed).
- **CalDAV PROPPATCH** (`internal/caldav/proppatch.go`): `SetCalendarProps(path, displayName, color)` issues an RFC 4918 PROPPATCH (DAV:displayname + Apple calendar-color) over the authenticated HTTP client — the same gap-filling approach as MKCALENDAR (go-webdav's client doesn't expose it). Empty values are skipped; success = 207 (or 200).
- **Store** (`internal/store/calendar.go`, sidecar): `UpdateCalendarMeta(id, displayName, color)` edits the local name/color and flags `pending_props` for a server push (offline-first); a still-pending-create calendar just carries the new values into its MKCALENDAR. `PendingCalendarProps` (only calendars already on the server) + `MarkCalendarPropsSynced` drive/clear the push. New `pending_props` sidecar field.
- **Sync** (`internal/sync/sync.go`): `pushCalendarProps` runs before discovery (so a routine pull doesn't race the change) — PROPPATCH each pending calendar, then clear the flag. `Syncer` gained `SetCalendarProps`; `SyncResult.CalendarsUpdated` counts them.
- **UI** (`internal/ui/command.go`): `:calendar rename <name>` / `:calendar color <#rrggbb>` act on the highlighted calendar (guarded read-only), update it locally, and rebuild the lists; `:calendar hide`/`show` mirror the `Space` visibility toggle (shared `afterVisibilityChange`). `normalizeColor` validates the hex. `currentCalendarID` resolves the active panel's calendar.
- Docs: help overlay, `README.md` (command list + PROPPATCH/offline-first note), `CLAUDE.md`. (`main.md`'s `:` section already listed `:calendar`.)
- Tests: caldav — PROPPATCH method/body/error/no-op (`proppatch_test.go`); sync — local rename+recolor pushes one PROPPATCH with the right name/color and doesn't re-push once synced (fake server gained `SetCalendarProps`); ui — `normalizeColor` table, `:calendar rename` updates the local name, `:calendar hide`/`show` toggle visibility. Full gate + `-race` pass. Pty: `:calendar rename Home`, `:calendar color #ff8800`, `:calendar hide` → exit 0, no panic; the sidecar records the new name, color, and `pending_props:true`.

## 2026-07-08 — Build step 10 finale (part 11): `:config` (edit in $EDITOR, reload on exit)

- Sixth finale increment; delivers the settled `:config` convenience (open in `$EDITOR`, reload on exit) deferred out of step 10.
- **UI** (`internal/ui/command.go`): `:config` calls `a.tv.Suspend` (releases the terminal for the editor), runs the `EditConfig` callback, then `applyConfigReload` swaps in a fresh sync closure and flashes — all inside the suspend so it's applied before the redraw on resume. `applyConfigReload` is split out so it's unit-testable without a running app. A nil callback flashes "unavailable".
- **Wiring** keeps the architecture rule intact — the UI runs no editor and parses no config itself. `ui.Options.EditConfig` is provided by `main`: `editConfigFn` (`cmd/lazyplanner/main.go`) launches `$EDITOR` (default `vi`) on the config path, re-reads via `config.Load`, and returns a rebuilt sync closure. It **refuses an account change** (`config.AccountID` differs) with a "restart to switch caches" error, since the vdir cache is account-keyed — a mid-session account swap would point sync at the wrong cache. Added `config.ConfigPath()`.
- Docs: help overlay (`:config` row), `README.md` command list + a note on the account-change caveat, `CLAUDE.md` command list. (`main.md`'s `:` commands section already listed `:config`.)
- Tests (`configreload_test.go`): `:config` with no callback flashes "unavailable"; `applyConfigReload` swaps a non-nil sync closure and flashes "reloaded"; a reload error is surfaced and the sync closure left untouched. Full gate + `-race` pass. Pty (`EDITOR=true`): `:config⏎` suspends, runs the editor, reloads (status shows "reloaded"), exit 0, no panic.

## 2026-07-08 — Build step 10 finale (part 10): yank/paste to move tasks (`y`/`p`)

- Fifth finale increment: move a task (and its subtree) between parents and lists — the reorganize flow that `H`/`L` (one level, in-list) couldn't do.
- **Yank/paste** (`internal/ui/yankpaste.go`): `y` records the selected task (`a.yankUID`); `p` moves it under the current tree selection (or the list's top level when the root is selected). Target list = the selected task list; target parent = the highlighted task. Guards against pasting a task onto itself or into its own subtree (cycle).
  - **Same list** → `reparentTo`: just `SetTodoParent` (RELATED-TO); children follow because their links are UID-based (unchanged).
  - **Different list** → `moveSubtree`: recreate the root + every descendant in the target calendar (`Put` under `ResourceName(uid)`) and delete each from the source, as **one compound undo step** (per resource: delete-the-copy + restore-the-original). The moved root adopts the paste target as its parent; descendants keep their UID links, so the subtree stays intact across the move. Read-only source or target is refused via `guardWrite`.
- Keys `y`/`p` added to `globalKeys` (both freed earlier — `y` was unused, `p` was the retired prev-period). The clipboard clears after a successful move.
- Docs: help overlay, `README.md` (edit prose + key table), `CLAUDE.md` UI line (`main.md` already listed `y`/`p` from the keymap rewrite).
- Tests (`yankpaste_test.go`): same-list paste re-parents Mover under Parent and `u` restores top-level; cross-list `moveSubtree` moves a Mover+Child subtree to another calendar (links intact, root becomes top-level, clipboard cleared) and `u` restores both to the source. Full gate + `-race` pass. Pty: `t y j p u q`, exit 0, no panic.

## 2026-07-08 — Build step 10 finale (part 9): quick field-set (`sp` priority, `sd` due)

- Fourth finale increment: change one field of the selected task without the full edit form.
- **`s` ("set") chord** (`internal/ui/quickfield.go`): `sp` sets priority, `sd` sets/clears the due date — each a one-line `promptInput`. Tasks view only (events have no priority/due); the `s` prefix flashes elsewhere.
- **Field application** honors the property-preservation iron rule: `draftFromTodo` clones the task's current fields into a `TodoDraft`, a mutator changes just the one field, and `EditTodo` re-encodes (so unknown iCal props, VALARMs, RELATED-TO, etc. survive). `applyTodoField` relocates the task fresh, guards read-only calendars, writes, pushes an **undo** step, and refreshes.
- **Parsing** reuses the quick-add rules: `parseSetPriority` accepts `1`-`9` / `high`/`med`/`low` (leading `!` tolerated; blank/`0`/`none` clears); the due prompt runs `ParseQuickAdd` (`fri`, `jul 20`, `3pm`, …; blank clears). Consistent with `it`/`is` quick-add.
- Docs: help overlay, `main.md` keymap (`s` row), `README.md` (edit prose + key table), `CLAUDE.md` UI line.
- Tests (`quickfield_test.go`): `parseSetPriority` table (digits, aliases, clear tokens, out-of-range/garbage rejected); `applyTodoField` sets priority then `u` restores 0; due set (date round-trips) then cleared. Full gate + `-race` pass. Pty: `t sp 3⏎ sd fri⏎ q`, exit 0, no panic.

## 2026-07-08 — Build step 10 finale (part 8): calendar visibility toggle (remembered)

- Third finale increment; closes the visibility toggle promised for the Calendars panel in step 10 but never built.
- **Store** (`internal/store/store.go`): added `EventOccurrencesVisible(from, to, hidden)` and `TodosVisible(hidden)` — the same queries filtered by a set of hidden calendar ids (keyed by id, which the store already has but the old flatten-all queries discarded). `EventOccurrences`/`Todos` now delegate with a nil set, so existing callers are unchanged.
- **State** (`internal/state`): `State` gained `HiddenCalendars []string` (`hidden_calendars`). The `SaveState` callback signature changed to `func(leftWidth int, hidden []string)` and now rewrites the whole state file, so pane width and hidden calendars persist together; `ui.Options` gained `Hidden`. `main.go` loads/saves both.
- **UI**: `a.hidden` (map, seeded from `Options.Hidden`); `persistState` gathers both prefs (sorted for stable output) and calls the save callback — `resizeLeft` now routes through it too. **`Space` in Calendar mode** toggles the highlighted calendar via `toggleCalendarVisibility` (rebuilds the calendar+agenda and persists); in other modes `Space` still toggles task done. The month grid, time-grid, and agenda queries pass `a.hidden`, so a hidden calendar's events **and** due tasks drop out. The Calendars list shows a `(hidden)` marker.
- Docs: help overlay, controls line (adds `Space done/hide` + `/ find`), `main.md` (Space keymap row + Calendars description), `README.md`, and the `CLAUDE.md` UI line updated; the stale "visibility toggles land in step 10" note replaced.
- Tests: `state` round-trip covers `HiddenCalendars`; ui — `toggleCalendarVisibility` hides/shows, persists the id set, and renders the `(hidden)` marker; hiding every calendar yields zero occurrences from `EventOccurrencesVisible`; the sizing test's save stub updated to the new signature. Full gate + `-race` pass. Pty: Calendar mode → `Space` hides the highlighted calendar → exit 0; `state.json` records `hidden_calendars:["personal"]`.

## 2026-07-08 — Build step 10 finale (part 7): incremental search (`/` `n`/`N`)

- Second finale increment: search across the current view.
- **Search** (`internal/ui/search.go`): `/` opens a top-line input; the selection follows the first match **as you type** (incremental — `SetChangedFunc` runs the search on each keystroke, changing only the selection so the input keeps focus). Enter keeps the match (focus lands on the view); Esc cancels and restores the pre-search selection. `n`/`N` cycle matches afterward (matches recomputed each press, so a cycle survives edits). Case-insensitive substring match.
- **Per-mode targets**: Tasks → the task tree (walks every `*model.Todo` node in display order and **expands ancestor folders** to reveal a match inside a collapsed subtree); Agenda → the agenda list; Calendar → the calendars list (search by name). `searchWidget`/`searchItems` centralize the per-mode collection + selection.
- **`:search <text>`** wired into the command dispatcher (also `:find`), matching the `main.md` command list; echoes to the command view.
- Keys: `/` opens search, `n`/`N` next/prev (added to `globalKeys`; `n`/`N` were freed by moving period-nav to `f`/`b`). Help overlay + `:` command hint updated.
- Tests (`search_test.go`): tasks search jumps to the first match and `n` cycles with wrap-around; no-match flashes; `n` with no active query flashes; calendar-name search selects the matching calendar. Full gate + `-race` pass. Pty: drive `t / meet⏎ n N a /g⎋ q`, exit 0, no panic.

## 2026-07-08 — Build step 10 finale (part 6): keymap overhaul + counts / gg-G + fold-all

- Start of a "step 10 finale" (owner-requested extra keybinds, treated as the last UI-polish step before step 11). First increment: a keymap remap that frees the number row, plus vim counts, `gg`/`G`, and tree fold-all.
- **Keymap remap** (owner's mnemonic scheme): panel focus moved off `1`/`2`/`3` to **`c`/`t`/`a`** (Calendars/Tasks/Agenda); the create prefix moved off `a` to **`i`** ("insert" — `it`/`iT`/`ie`/`iE`/`is`/`iS`/`ic`/`il`, Shift = full form), freeing `a` for Agenda and keeping `n`/`N` for search; calendar period nav moved off `n`/`p` to **`f`/`b`** (forward/back). Freeing the digits is what makes counts possible.
- **Vim counts** (`internal/ui/app.go` `globalKeys`): `1`-`9` start a count and `0` extends one (`a.pendingCount`, capped at 999, shown in the status-bar left section); the next motion (`hjkl`/arrows) repeats via `repeatKey`, which feeds the event to the focused primitive N times — reusing tview's own List/TreeView navigation so counted movement matches a single keypress. A non-motion key drops the count.
- **`gg` / `G`** (`internal/ui/keys.go`): `g` is now a which-key prefix — `gg` top, `gt` today, `gd` go-to-date (the old standalone `g`=goto). `gg`/`G` feed `Home`/`End` to the focused list/tree (both handled natively by tview); `<count>G` jumps to the nth item of a list. `G` bottom is a standalone key.
- **Fold-all** (`z` prefix, Tasks view only): `zR` expand-all, `zM` collapse-all, `za` toggle the current node — walks the tree nodes, sets expansion, and keeps each folder's `▸`/`▾` marker in sync.
- **which-key**: the popup footer now varies by prefix (the "Shift = full form" note only shows for the `i` create prefix); `prefixLabel` gains `i`/`g`/`z`.
- Docs: help overlay (`help.go`), controls line (`render.go`), `main.md` keybinding table (rewritten from the stale "draft/future step 10" framing to the real vim-flavored scheme), `README.md` usage + key table, and the `CLAUDE.md` UI line all updated to the new keys.
- Tests (`keys_test.go`): count prefix repeats a motion (`3` then Down moves 3 and resets), `gg`/`G`/`<count>G` land on first/last/nth, fold-all collapses+expands a folder and `za` toggles it; existing chord tests migrated `a`→`i`. Full gate (build/vet/staticcheck) + `go test -race ./...` pass. Pty smoke: TUI drives `t zR zM za gg G 3j c f b [ ] gt a i⎋ q` against a seeded cache, exit 0, no panic.
- Remaining finale increments: search (`/` `n`/`N`), calendar visibility toggle, quick field-set keys, yank/paste (`y`/`p`), `:config`, `:calendar rename|color`.

## 2026-07-07 — Build step 10 (part 5): mouse pass + docs — step 10 complete

- **Mouse** (`internal/ui/mouse.go`): app-level `SetMouseCapture` makes the mouse coherent with the mode model on top of tview's built-in click-to-select/scroll — clicking a left overview panel switches to that mode (so the center follows), and a double-click on the task tree or agenda opens the edit form. Skipped while a modal/overlay is up.
- **Docs**: README rewritten for the chorded keymap (a-prefix create with which-key, contextual `d`, `:` commands, `g`/`?`, `+`/`-`/`Ctrl-arrows`, `:conflicts`, mouse) and the status blockquote marks step 10 complete; CLAUDE.md UI line updated. (Full-cell click mapping for the custom calendar grids and detail-pane accordion collapse noted as future niceties.)
- Test: `TestMouseClickSwitchesMode` draws the layout to a simulation screen so panels have rects, then simulates clicks that switch mode. Full gate + `-race` pass. Pty end-to-end: which-key on `a`, `:view week` echoes to the command view, `?` help opens, `+`/`-` accordion, clean exit.
- **Build step 10 complete.** Next: step 11 (recurrence editing semantics).

## 2026-07-07 — Build step 10 (part 4): interactive pane sizing + state file

- **State file** (`internal/state`): a new package persisting small UI prefs in `<dataDir>/<account-id>/state.json` (0600, atomic rename) — separate from config (never app-written) and the vdir cache. `Load` is best-effort (missing/corrupt → zero, never blocks startup). Wired in `main.go`; `ui` stays disk-free — it receives the remembered width and a `SaveState` callback via a new `ui.Options` (Run's signature is now `Run(Options)`).
- **Keyboard resize** (`Ctrl-←`/`Ctrl-→`): grow/shrink the left overview column by a step, clamped to [16, 50], persisted on each change (`resizeLeft`). Uses `Flex.ResizeItem`.
- **Accordion** (`+`/`-`): `+` collapses the left overview so the Main view fills the width and moves focus into the center; `-` restores it. Switching panels (`1`/`2`/`3`) also restores it. Gated out of Agenda mode (its center navigation is driven by the left agenda list). (Detail-pane collapse left as a future extension; the overview collapse delivers the main width win.)
- Help overlay gained a Layout section.
- Tests: `state` round-trip + bad-file-is-zero; `resizeLeft` clamps at both bounds and calls `SaveState`; accordion is restored on mode switch and blocked in Agenda. Full gate + `-race` pass.

## 2026-07-07 — Build step 10 (part 3): interactive conflict resolution (`:conflicts`)

- Closes the piece deferred from step 9 (sync detects conflicts and keeps both; now they're resolvable in-app).
- **Store** (`internal/store/conflict.go`): `ResolveKeepLocal` clears the conflict and adopts the server's current ETag so the next sync's conditional PUT overwrites the server with the local edit (local .ics untouched). `ResolveKeepServer` decodes the stashed server version and writes it clean via `PutRemote`, so the next sync is a no-op. `writeResource` now also clears a name's conflict stash (any deliberate write supersedes a conflict). Keep-both (preserve both as separate items) noted as a future refinement — needs a new-UID clone; keep-local/keep-server cover winner-picking.
- **UI** (`internal/ui/conflicts.go`): `:conflicts` opens a list of conflicted items (calendar — title); Enter opens a Keep local / Keep server / Cancel chooser; resolving refreshes the views (and the sync-status conflict count) and rebuilds the list, auto-closing when none remain. Added to `:help` and the command dispatcher. The status bar already shows the live conflict count.
- Tests: store — `ResolveKeepLocal` (dirty, adopts server etag, keeps local content, clears conflict, survives reload) and `ResolveKeepServer` (clean, server content adopted). ui — `:conflicts` flashes when none, opens and lists when a conflict is present. Full gate + `-race` pass.

## 2026-07-07 — Build step 10 (part 2): `:` command mode + `?` help overlay

- **Command line** (`internal/ui/command.go`): `:` opens an input near the top; Enter runs, Esc cancels. `runCommand` dispatches `:sync`, `:view month|week|day`, `:goto <date>` (smart-parsed via `ParseQuickAdd`), `:help`, `:q`. Each echoes its command form to the status-bar middle "command view". `g` opens the command line prefilled `goto `. (`:search`/`:config`/`:calendar`/`:conflicts` land in later step-10 increments.)
- **Help overlay** (`internal/ui/help.go`): `?` (and `:help`) open a scrollable cheat sheet grouped by area (panels/nav, create chords, edit, calendar, sync/commands); Esc/`q`/`?` closes, `j`/`k`/arrows scroll. Controls line now advertises `: cmd · ? help`.
- Tests (`command_test.go`): `:view week` switches to calendar/week and echoes; invalid arg flashes without changing state; `:goto 2026-12-25` moves the anchor + switches to calendar; unparseable goto and unknown commands flash; help overlay opens and closes.
- Full gate + `-race` pass.

## 2026-07-07 — Build step 10 (part 1): chorded keymap + which-key popup

- Start of step 10 (command mode & keybinding polish). First piece: the vim-style chord scheme, replacing the interim standalone create keys.
- **Chord dispatcher** (`internal/ui/keys.go`): `a` is now a prefix — `at`/`aT` task, `ae`/`aE` event, `as`/`aS` subtask (Shift = full form), `ac` calendar, `al` list. `globalKeys` claims the next key when a prefix is pending (before the modal/single-key handling); Esc or an unknown continuation cancels. Bindings live in a `chords` table (data) so the which-key popup and, later, the help screen render from the same source.
- **which-key popup**: after a prefix, a bottom overlay lists the continuations (non-focused — the next keystroke is intercepted by `globalKeys`, so it needs no focus). Chosen per the owner's "shift the object letter" convention.
- **Contextual delete**: `d` deletes the calendar/list when an overview list is focused, else the selected item — folding in the old `D`.
- **Command-view echo**: executing a chord writes its command form to the status bar's middle section (`echo`), the lazygit-style "what you just did" line (fleshed out with `:` command mode next).
- Retired interim keys `A`/`s`/`S`/`c`/`D`; split `addQuick`/`addFull` into typed `addTaskQuick`/`addTaskFull`/`addEventQuick`/`addEventFull`; `createCollection` takes a default-type arg (`ac`→calendar, `al`→list). `r` kept as a `:sync` alias.
- Tests (`keys_test.go`): prefix shows which-key then Esc cancels; `at` completes the chord, opens the quick-add prompt, and echoes the command view; an unknown `az` clears the prefix and flashes. Full gate + `-race` pass.

## 2026-07-07 — Timezone robustness: embed tzdata + Windows-zone map + floating fallback (no more dropped events)

- Follow-up to the read-only fix: another silent-data-loss quirk. go-ical's date parser calls `time.LoadLocation(TZID)` and **errors** on any non-IANA zone (`vendor/.../ical.go:150`); our `ParseEvent`/`ParseTodo` treated that as fatal, so a timed event/todo with an Outlook/Windows TZID (e.g. `Eastern Standard Time`), a custom `VTIMEZONE` label, or *any* TZID on a build without system zoneinfo was rejected and skipped — it silently vanished. Recorded in `main.md` (Timezones decision + step 12).
- **Embed tzdata** (`cmd/lazyplanner/main.go`): blank `import _ "time/tzdata"` bakes the IANA database into the binary, so zones resolve on a minimal Pi image or Windows — fits the "robust single static binary" goal. Verified the binary resolves zones with `ZONEINFO=/nonexistent`.
- **Windows→IANA map** (`internal/model/windowszones.go`): the CLDR windowsZones "001" defaults (~140 entries) map Outlook zone names to IANA.
- **Graceful resolution** (`internal/model/tz.go`, `resolveDateTime`): try go-ical first; on failure map a Windows TZID→IANA; if still unresolved, interpret the value as **floating/local** so the item is kept (at worst offset for an exotic unmapped zone) instead of dropped. Wired into DTSTART, DTEND (recovers an explicit DTEND with a bad TZID; DURATION still handled by go-ical), and DUE.
- Tests: `TestParseEventTimezones` (Windows name → correct IANA offset; real IANA still works; unknown TZID → kept as floating, not dropped); `windowsToIANA` lookups; and a guard that every mapped IANA name actually loads with the embedded db (catches table typos). Full gate + `-race` pass.
- **Owner action**: none required unless you have Outlook-authored events — if any were missing before, they should appear after the next sync.

## 2026-07-07 — Read-only calendars (NextCloud birthdays etc.): detect + block + pull-only

- Owner report: events added to NextCloud's generated "Contact Birthdays" calendar (read-only, no writes allowed in the web UI) were silently discarded during sync. Root cause: LazyPlanner pushed them, the server rejected/dropped them, and reconcile then treated the missing server copy as a remote deletion and `Forget`-ed them. Fix: **know a calendar is read-only and never write to it** (mirrors NextCloud web). Decision recorded in `main.md`. Owner approved discarding the already-stuck test events.
- **Detection** (`internal/caldav/privileges.go`): a Depth-1 `PROPFIND current-user-privilege-set` (RFC 3744) on the calendar home set, issued over our own authed HTTP client (go-webdav's client neither requests nor exposes privileges — same gap as MKCALENDAR). A calendar granting read but not write/write-content/bind/all is read-only. `caldav.Calendar` gained `ReadOnly`, set during `DiscoverCalendars`; a failed privilege query degrades gracefully (fail-open). Plus a **reactive safety net**: `PutObject`/`DeleteObject` map HTTP **403 → `ErrReadOnly`**.
- **Store** (`internal/store`): `Calendar.ReadOnly` + sidecar `read_only` (persists so the UI knows offline) + `SetCalendarReadOnly` (no-op when unchanged).
- **Sync** (`internal/sync/sync.go`): each sync persists the server's read-only status. A read-only calendar is reconciled **pull-only** (`reconcileReadOnly`): local dirty/never-synced resources are discarded, local deletions (tombstones) are reverted by re-pulling, and the server state is mirrored in. If a write ever returns `ErrReadOnly` (privilege detection missed it), the calendar is flagged read-only and the change discarded. New `SyncResult.Discarded` counter.
- **UI** (`internal/ui`): `guardWrite` blocks create/edit/complete/delete/re-parent (and delete-collection) on a read-only calendar with a "read-only" flash — at the source, before opening any form. Read-only calendars/task lists show a `[ro]` marker in the overview lists.
- Tests: caldav — `discoverWritable` parses privilege multistatus (writable vs read-only), 403→`ErrReadOnly`. store — read-only persists across reload. sync — read-only calendar discards a stuck local event and mirrors the server (no writes), reactive-403 marks read-only + discards. ui — `guardWrite` blocks + flashes, `[ro]` marker renders. Full gate + `-race` pass. Pty: read-only calendar blocks `a` (add) with a read-only flash. (Fixed a test-hygiene bug: the read-only UI tests must use the writable temp-copy app harness, not the shared in-place fixture.)
- **Owner action**: confirm against real NextCloud that the birthday calendar is now detected read-only (shows `[ro]`, refuses edits, mirrors birthdays in).

## 2026-07-07 — Build step 9 (part 5): in-app calendar / list creation + deletion (offline-first) — step 9 complete

- Final step-9 piece: create/delete calendars and task lists in-app, offline-first (local now, server round-trip on next sync). **Build step 9 is complete.**
- **Store calendar management** (`internal/store/calendar.go` + sidecar/store): per-calendar pending state in the sidecar (`pending_create`, `pending_delete`, `components`). `CreateCalendarLocal(id, meta, components)` makes the collection locally, flagged for MKCALENDAR. `MarkCalendarDeleted` hides the calendar from `Calendars()` immediately and flags it for a server DELETE (a never-pushed calendar is removed outright, no round-trip). `MarkCalendarSynced`/`RemoveCalendarLocal`/`PendingCalendarDeletes` support the sync push. `Calendars()` skips pending-deletes; the `Calendar` snapshot gained `PendingCreate`/`Components`.
- **Sync** (`internal/sync/sync.go`): before discovery, `pushCalendarDeletes` issues server `DELETE` for calendars marked deleted (then removes them locally; a failed delete stays pending and is not re-imported), and `pushCalendarCreates` issues `MKCALENDAR` (under the calendar-home-set) for locally-created calendars, then records the href so the following reconcile pushes their resources. `Syncer` extended with `CalendarHomeSet`/`CreateCalendar`/`DeleteCalendar` (all already on `*caldav.Client`). New result counters `CalendarsCreated`/`CalendarsDeleted`.
- **UI** (`internal/ui/calendar.go`, `app.go`): **`c`** opens a create form (Name + Type: Event calendar / Task list / Both — defaults to a task list in Tasks mode); **`D`** deletes the highlighted calendar (Calendars) or list (Tasks) with a confirm. Both offline-first. Interim keys (fold into the `a`-prefix `ac`/`al` in step 10); added to the controls line.
- Tests: store calendar API exercised via sync tests; sync — create-local-calendar-then-push-its-resources (MKCALENDAR spec + resources pushed), delete-local-calendar-on-server (DELETE issued, not re-imported), delete-never-pushed-skips-server. UI — `componentsForType`, delete-needs-collection-pane guard. Full gate + `-race` pass. Pty: `c` → typed name → Create writes `<account-id>/calendars/Groceries/` with `pending_create:true` + `components:["VEVENT"]`; exit 0.
- **Owner action**: real-NextCloud MKCALENDAR/DELETE-on-sync acceptance to be confirmed by the owner alongside the sync validation.

## 2026-07-07 — Build step 9 (part 4): in-app sync trigger + sync-status indicator

- Wired the sync engine into the TUI and the status bar.
- **UI sync** (`internal/ui/sync.go`, `app.go`): `Run` now takes a `syncFn` closure (nil = no server → app runs fully offline). `triggerSync` runs the sync on a background goroutine (UI never blocks on the network), coalesces overlapping requests, and on completion `QueueUpdateDraw`s a view refresh + status repaint. **Background sync on startup** fires from `Run` (offline-first: opens instantly from cache, refreshes when sync lands). Interim manual trigger on **`r`** (the real `:sync` command lands with command mode in step 10).
- **Sync-status indicator** (`render.go` `updateStatus` → `renderSyncStatus`): the status bar's right section now shows real state with color+words (TTY-safe, no glyphs): `not configured` (gray) · `syncing...` (yellow) · `synced HH:MM` (green) · `! N conflict(s)` (red, from `store.Conflicts()`) · `offline` (red, on error). Replaced the step-9 placeholder. Controls line gained `r sync`.
- **Wiring** (`cmd/lazyplanner/main.go`): `buildSyncFn` builds a caldav client from `[server]` (resolving `password_command`) and returns the closure; a failing password command or client build is a warning, not fatal — the app opens offline.
- Tests (`internal/ui/sync_test.go`): `renderSyncStatus` across all five states; `syncSummary` (quiet sync → empty, else up/down/conflict); `triggerSync` no-op + hint when unconfigured. Full gate + `-race` pass. Pty smoke: TUI launches, background startup sync against an unreachable server resolves to `offline`, `r` re-triggers, `q` exits 0 — no panic.

## 2026-07-07 — Build step 9 (part 3): two-way sync engine + `lazyplanner sync` CLI

- The must-have feature: ETag-based two-way reconciliation that never silently overwrites.
- **Sync engine** (`internal/sync/sync.go`): `Sync(ctx, Syncer, *store.Store)` reconciles the cache against the server resource by resource, keyed by href + ETag + the local dirty flag. Per-resource decisions: local create (no href) → `PUT If-None-Match:*`; local edit + server unchanged → `PUT If-Match:etag`; server edit + local clean → pull; **both edited → conflict (keep both, flag, no overwrite)**; server-new → pull; server-deleted + local clean → drop locally (`store.Forget`, no tombstone); server-deleted + local edited → conflict (keep local); tombstone → `DELETE If-Match:etag`; tombstone vs server edit (412) → resurrect the server copy + flag. A conflicted resource is skipped on later syncs until resolved. New server calendars are created + pulled; calendars only present locally are left untouched (in-app calendar management handles those). Discovery/listing errors abort; per-resource errors collect in `SyncResult.Skipped`. `Syncer` interface (DiscoverCalendars/DownloadAll/PutObject/DeleteObject) keeps go-ical out of `sync` — pushes go through `model.Parsed.Encode()` → `[]byte`.
- **Store conflict support** (`internal/store/{conflict,sidecar,store,mutate}.go`): `MarkConflict` stashes the server's diverging version losslessly in the sidecar and flags the local resource `Conflicted`; `Conflicts()` lists them (drives the status count → part 4); `Forget` deletes a resource locally without leaving a tombstone (server already lacks it); `remove` also clears any conflict on delete.
- **caldav.PutObject → `[]byte`**: takes the encoded body instead of `*ical.Calendar` so `sync` needs no ical import (architecture rule: go-ical confined to model/caldav).
- **CLI** (`cmd/lazyplanner/sync.go`): `lazyplanner sync` runs a two-way sync against the account-namespaced cache (flags/env creds), printing pushed/pulled/deleted/conflict counts — the runnable path to validate against real NextCloud before the UI drives it.
- Tests (`internal/sync/sync_test.go`): in-memory fake server; one test per branch — push-create (+ idempotent second sync), push-edit, pull-server-edit, pull-new-server-object, conflict-keeps-both (+ skipped next sync), server-delete-drops-clean (no tombstone), server-delete-vs-edit conflict, tombstone push, tombstone-vs-server-edit conflict (resurrect), new-server-calendar. Full gate + `-race` pass.
- **Owner action**: real-NextCloud sync acceptance to be confirmed by the owner (`lazyplanner sync`) — the engine is fake-tested, like the MKCALENDAR work.

## 2026-07-07 — Build step 9 (part 2): sync primitives — delete tombstones + conditional PUT/DELETE

- The store and caldav pieces the two-way engine needs; no sync logic yet.
- **Store tombstones** (`internal/store/{sidecar,store,mutate,tombstone}.go`): deleting a resource that was previously synced (has an `Href`) now records a **tombstone** (href + last ETag) in the sidecar, so sync can push the deletion instead of the resource silently reappearing as "new on server" on the next pull. A never-synced local delete (no Href) leaves no tombstone. `writeResource` clears a name's tombstone whenever it writes that name — so undo's `Restore` resurrecting a just-deleted resource cancels the pending delete for free. New store API: `Tombstones()` (sorted, cross-calendar) and `ClearTombstone` (after a successful server DELETE). Tombstones persist across reload.
- **caldav conditional writes** (`internal/caldav/object.go`): `PutObject(href, cal, ifMatch, create)` — issues the PUT over the authenticated HTTP client (go-webdav's `PutCalendarObject` can't set conditional headers) with `If-Match: <etag>` on update or `If-None-Match: *` on create, so the app never blindly overwrites; returns the new bare ETag. `DeleteObject(href, ifMatch)` — conditional DELETE; 404 is idempotent success. Both map HTTP **412 → `ErrPreconditionFailed`** so sync can turn a lost race into a conflict. ETag representation pinned: the store keeps **bare** ETags (matching go-webdav's unquoting download path) and the header layer quotes/unquotes at the boundary (`normalizeETag`/`httpETag`), so ETags from every code path compare equal.
- Tests: store — synced-delete leaves a tombstone that survives reload and clears; never-synced delete leaves none; `Restore` clears a tombstone. caldav — create sends `If-None-Match: *`; update sends a **quoted** `If-Match` from a bare stored etag and returns the bare new etag; 412 → `ErrPreconditionFailed` (PUT + DELETE); 404 delete is success. Full gate passes.

## 2026-07-07 — Build step 9 (part 1): config module + account-keyed cache path

- Start of two-way sync (step 9). First two sub-parts: the config file and the account-namespaced cache path (both prerequisites — sync needs credentials, and a mismatched cache would corrupt conflict detection).
- **Config module** (`internal/config/config.go`, `template.go`): added `BurntSushi/toml` (vendored). `Config` = `[server]` (url/username/password/**password_command**) + `[appearance]` (first_day_of_week, default_view, time_format, date_format) + `[behavior]` (sync_interval_minutes). `Load()` overlays the file on owner-preferred `Default()`s (a working config needs only `[server]`); a missing file returns `configured=false`. `GenerateDefault()` writes a **fully-commented starter config.toml** (every option at its default, commented) `0600`, never overwriting an existing file. Loose-permission (`&0o077`) files get a non-fatal chmod-600 warning (POSIX-only). `Server.ResolvePassword()` runs `password_command` via `sh -c` (owner's `bw get password lazyplanner`), else inline password — resolved at connect time, not load.
- **Account-keyed cache** (`config.AccountID`, `AccountDataDir`): opaque 12-hex-char sha256 of normalized `url\x00username` (trailing-slash/case-insensitive). Cache root is now `<dataDir>/<account-id>/calendars/…`. Wired into `runTUI` (loads config; on first run writes the starter config and exits so the user fills in `[server]`) and the `import` CLI (same id so import and TUI agree). **No auto-migration** of the old un-namespaced `<dataDir>/calendars/` — the server is source of truth, so a re-import repopulates the new path.
- Tests (`internal/config/config_test.go`): missing→defaults, file-overlay-keeps-omitted-defaults, loose-permission warning, `ResolvePassword` (command precedence + trim, inline fallback), `AccountID` (normalization + distinctness), `GenerateDefault` (parses, 0600, no-overwrite). Full gate (build/vet/staticcheck/test) passes.

## 2026-07-07 — Spec: account model (single-account, server-keyed cache) folded into step 9

- Owner asked to record the account-switching plan before starting step 9. Decision: LazyPlanner stays **single-account** (one `[server]`, no in-app switcher), but account switching — expected rare — **must be safe**, so the local vdir cache is namespaced by a stable `<account-id>` derived from server URL + username (`<dataDir>/<account-id>/calendars/…`). Changing the server connection then maps to a separate cache; two accounts can never share one directory. Rationale: sidecar ETags/hrefs are server-specific, so a mixed cache would corrupt two-way-sync conflict detection.
- Scoped as a **cheap safeguard, not a feature**: full multi-account profiles (`[[account]]` blocks + `:account` switcher) are noted as a future enhancement, explicitly out of initial scope.
- `main.md`: new **Account model** entry in Settled Decisions; **Build Plan step 9** folds in the account-keyed cache path (wired with two-way sync, when a mismatched cache first becomes dangerous).
- Spec-only change (no code). Verified `main.md` reads cleanly and `log.md` heading count matches entry count.

## 2026-07-06 — UI: all-day drill, filled-box completed glyph, full-day time-grid

- Three owner-requested UI changes before step 9.
- **All-day events in the week/day drill cycle** (`timegridview.go`): `dayOccs` now returns the selected day's all-day items first, then timed ones, so `Enter`-to-cycle covers all-day events too. The cycled all-day event is shown highlighted (reverse) in the top band; timed events highlight their block as before. Detail pane follows the selection.
- **Completed-task glyph**: the checkbox now fills with `[■]` when done (was `[x]`) — in the task tree (`nodeLabel`), the month-grid day cells (`itemLabel`), and the agenda list (`agendaLeftLabel`). Hide-by-default behavior is unchanged (glyph only).
- **Week/day fills the height**: the time-grid now scales the full 24-hour day across the pane body (`row = bodyY + hour*bodyH/24`) instead of one fixed row per hour with a scroll window — the day always fills the screen, hour rows grow with the window, and event blocks are sized proportionally. Removed `scrollHour` and the scroll keys (nothing to scroll).
- Tests: `TestTimeGridDrillsAllDayFirst` (all-day cycles before timed), `TestNodeLabelCompletedGlyph` (`[■]`), and `TestTimeGridDrawsDay` now asserts the whole day renders (12am..11pm). Full gate + `-race` pass; pty confirms the day view fills top-to-bottom with the all-day band and a timed block, no panic.

## 2026-07-05 — Fix: legible selection highlight on any theme (reverse video)

- Report: the terminal-background fix made highlighted (selected) text illegible on every tested terminal — a latent bug the black background had masked.
- Cause: tview's default selected style (List `selectedStyle`, TreeNode `selectedTextStyle`) is `Foreground(Styles.PrimitiveBackgroundColor).Background(Styles.PrimaryTextColor)`. With `PrimitiveBackgroundColor` now `ColorDefault`, the selected *foreground* became the terminal's default text color (usually light) on a light bar → unreadable. Previously it was black (the old default), which happened to be legible.
- Fix: select with **reverse video** (`tcell.StyleDefault.Reverse(true)`) for the overview lists (`SetSelectedStyle`) and every task-tree node (`SetSelectedTextStyle`). Reverse is the inverse of the already-legible normal text, so it stays readable on any light or dark scheme and doesn't depend on the palette. The calendar/agenda/time-grid selections were already independent of the primitive background (outline box / explicit fill / reverse) and were unaffected.
- Test: `TestSelectionIsLegible` asserts the highlighted list row renders with the reverse attribute. Full gate + `-race` pass.

## 2026-07-05 — Fix: inherit the terminal background (no more shaded text boxes)

- Report: on some terminal color schemes, text sat in a shaded box (text background ≠ overall background).
- Cause: tview's default `Styles.PrimitiveBackgroundColor` is solid **black**, so every pane/box filled black, while our custom-drawn text (calendar/agenda/time-grid via `printStyled` with `tcell.StyleDefault`) uses the **terminal default** background. On any scheme whose background isn't pure black, the black fill vs. default-bg text cells showed as boxes behind the text.
- Fix: set `tview.Styles.PrimitiveBackgroundColor = tcell.ColorDefault` (in `useTerminalTheme`, folded in with the rounded-border setup and run before any widget is created), so panes, boxes, and text all inherit the terminal's background. Deliberate fills (time-grid event blocks, selection highlights, list selection) still use explicit colors.
- Test: `TestTextInheritsPaneBackground` renders the agenda and asserts a text cell shares the surrounding pane's background (both terminal default). Full gate + `-race` pass.

## 2026-07-05 — Fix: H/L re-parent now reads the on-screen tree (WYSIWYG)

- Bug: after the folders/sticky-complete work, H/L (indent/outdent) misbehaved — often "can't indent"/"already top level" or nesting under the wrong task.
- Cause: `reparentSelected` recomputed sibling/parent structure with `model.BuildTree(todos, showCompleted)`, but `buildTree` now renders a *different* forest (`BuildTree(visible = incomplete + sticky, true)`). Before folders/sticky both used the same call, so they matched; now they diverge (e.g. the row visually above you is a sticky-completed task the reparent forest omits), so H/L computed the wrong sibling/parent or a no-op.
- Fix (owner chose WYSIWYG): compute parent + previous-sibling directly from the displayed tview tree via `treeNodeContext` (walks `a.tree` for the node's parent + index) — indent nests under the row shown directly above at the same level; outdent moves to the parent's parent (or top level when the parent node is the list root). No second forest, so H/L can't drift from the rendering again. Removed the now-unused `findInForest`.
- Test: `TestReparentUsesOnScreenSibling` indents below a sticky-completed row and asserts it nests under that on-screen row (would fail under the old forest-mismatch logic). Existing reparent test still passes; full gate + `-race` pass.

## 2026-07-05 — Popup restyle: terminal-themed forms with a ▸ focus caret

- Owner review of the popups. Reworked the edit/create forms, quick-add line, and confirm dialog to a single look: the terminal's **default (unified) background**, **high-contrast default text**, and an **accent (teal) rounded border/title** — no more white card.
- **Focus caret**: tview reapplies one field style to every form field each frame (and `FormItem` has no `SetLabel`), so a per-field "white when focused" isn't possible. New `caretForm` (`internal/ui/forms.go`) wraps `tview.Form` and, in `Draw`, marks the focused field (`GetFocusedItemIndex`) with a `▸` in a fixed two-column label gutter; the focused button is reversed. Forms now hold explicit field references (`todoFields`/`eventFields`) and read values from them, since the moving caret changes labels and label-lookup would break.
- Removed the old `styleBWForm`/`formText`.
- Tests: `TestCaretFormGutter` exercises the Draw override + gutter (the ▸-on-focus placement needs the live app, verified via pty). Full gate + `-race` pass; pty confirms the edit form shows the caret, labels, title, and Save on a terminal-themed background.

## 2026-07-05 — Fix: sticky-complete worked only on the first task list

- Bug: checking off a task while completed are hidden only kept it visible on the first list; later lists reverted to the old (immediate-hide) behavior.
- Cause: the list-change detection (which drops the sticky pins) lived in `buildTree` and compared `selectedTasklistID()` to `treeListID`. During a panel rebuild, `List.Clear`/`AddItem` park the selection at index 0, and — critically — `List.SetCurrentItem` fires its changed callback *before* updating the current item, so `GetCurrentItem()` was stale (returned the first list). Both made `buildTree` see the first list's id mid-refresh and wipe `stickyDone` for any other list.
- Fix: moved the sticky-clear out of `buildTree` into the tasklist changed-callback, keyed on the callback's **index argument** (reliable) rather than `GetCurrentItem`; suppressed the callback during panel rebuilds (`suspendTree`); and sync `treeListID` when entering tasks mode so restore events aren't misread as a list switch.
- Test: `TestStickyWorksOnNonFirstList` completes a task on a second list and asserts it stays visible. Full gate + `-race` pass.

## 2026-07-05 — UI polish pass (3/3): week/day drill-in, agenda outline box, modal focus

- **Week/day drill-in** (`timegridview.go`): the time-grid now mirrors the month grid — `Enter` on the selected day enters event mode, `↑`/`↓` (`k`/`j`) cycle its timed events with the current one boxed/highlighted and shown in the Detail pane, `Esc`/`←`/`→` back out. New `onSelectEvent` callback + `eventMode`.
- **Agenda outline box** (`agendaboard.go`, new): replaced the agenda center's tview.TextView with a custom-drawn widget that draws a **rounded outline box** around the selected item (matching the calendar's day cursor) instead of a filled bar; items keep their green/aqua colors. It manages its own scroll to keep the selection visible; selection is driven by the left Agenda list.
- **Modal return-focus** (`edit.go`): closing a dialog returns focus to where you were — including a drilled-into calendar day — via a `calGrid` interface (`drillState`/`reDrill`) implemented by both the month and time-grid views, captured on open and restored on close (create/edit refresh first, then restore so the grid can re-drill).
- Tests: time-grid drill-in (Enter → event mode + emit, Esc → exit); agenda selection is outlined (rounded corner drawn, title keeps its color, not inverted). Full gate + `-race` pass; pty smoke test confirms folder arrows, rounded corners, the agenda box, and week drill-in all render with no panics.

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
