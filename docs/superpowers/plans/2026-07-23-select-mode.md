# v1.4.0 SELECT Mode Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** A vim-style contiguous multi-select layer (`V`) over the task tree, calendar days, and drilled-day items, with bulk complete/delete/yank/grab — one compound undo step per op, all-or-nothing rollback.

**Architecture:** Derived-range, anchor-only state: the app stores `selecting bool` plus one anchor (tree UID / date / item identity); the selected set is always computed anchor→cursor in visible order, never stored — so refreshes can't drift it. SELECT is one more derived branch on the existing `interactionMode()` seam (like GRAB/RESIZE), never a parallel mode enum. Bulk ops follow the `moveSubtree` rollback template (`internal/ui/yankpaste.go:138`); bulk grab is a uniform date-shift reusing grab's per-nudge `PutIfUnchanged` commit model.

**Tech Stack:** Go, tview/tcell (UI only in `internal/ui`), standard `testing` package (no assertion libs), table-driven tests.

**Spec:** `docs/superpowers/specs/2026-07-23-select-mode-design.md` (owner-approved). Read it before starting.

## Global Constraints

- Full gate after every task: `go test ./... && go vet ./... && staticcheck ./... && go build ./...` — all clean before commit.
- Every task's commit also appends a `log.md` entry (newest at top, `## YYYY-MM-DD — Title` heading, per CLAUDE.md format; verify `##` heading count == entry count after editing).
- Branch: `ai-workspace` only. Never `main`.
- Comments explain *why*, never *what/how* (CLAUDE.md Comment Rules). Doc comments on all exported identifiers; nothing here needs exporting outside `internal/ui`.
- **Draw-lock rule:** draw paths (`Draw`, `SetDrawFunc`) read plain struct fields only — never `a.tv.GetFocus()` or anything taking the tview app lock.
- **Write rule:** every write to an existing resource goes through `store.PutIfUnchanged` with the `Locate`d `Prev`; a stale result must never be overwritten.
- **No parallel mode enum:** SELECT state is `a.selecting bool` + anchor fields; `interactionMode()` stays a pure derivation.
- Multi-write ops: accumulate `rollback []func()` (run newest-first on failure) + `ops []undoOp` (one `pushUndo` on full success) — the `moveSubtree` pattern.
- Tests: both sides of every boundary window; mirror guards onto sibling paths (audit guardrail).
- gofmt/goimports before every commit.

---

### Task 1: SELECT core — state, enter/exit, badge, key layer

**Files:**
- Create: `internal/ui/selection.go`
- Create: `internal/ui/selection_test.go`
- Modify: `internal/ui/app.go` (state fields ~line 150; `globalKeys` branch ~line 725; `V` binding in the rune switch ~line 913)
- Modify: `internal/ui/render.go` (mode consts ~594; `interactionMode` ~610; `drawModeIndicator` ~650; `updateStatus` hints ~703)
- Modify: `internal/ui/keys.go` (`resolvePrefix` gate ~line 94)
- Modify: `internal/ui/mouse.go` (`mouseCapture` gate, line 13)
- Modify: `internal/ui/mode_test.go` (badge cases)

**Interfaces:**
- Consumes: `a.currentTreeUID()` (`edit.go:787`), `a.calendarPrimitive().(calGrid)` + `drillState()/selectedItem()` (`app.go:262`), `targetFromItem` (`edit.go:100`), `a.flash`, `a.updateStatus`.
- Produces: app fields `selecting bool`, `selAnchorUID string`, `selAnchorOcc time.Time`, `selAnchorDay time.Time`; funcs `enterSelect()`, `exitSelect()`, `handleSelectKey(*tcell.EventKey) *tcell.EventKey`, `selContext() int` with consts `selNone/selTree/selDays/selDrill`, `syncSelectionVisuals()` (stub for now), const `modeSelect = "SELECT"`, const `selectHint`. Later tasks replace the op-key stubs in `handleSelectKey` and extend `syncSelectionVisuals`.

- [ ] **Step 1: Write the failing tests** (`internal/ui/selection_test.go`)

```go
package ui

import (
	"testing"
	"time"

	"github.com/gdamore/tcell/v2"
)

// key feeds one key event through the app's global input capture, mimicking the
// event loop's dispatch (capture first, then the focused primitive).
func key(a *app, k tcell.Key, r rune) {
	ev := tcell.NewEventKey(k, r, tcell.ModNone)
	if out := a.globalKeys(ev); out != nil {
		if p := a.tv.GetFocus(); p != nil {
			if h := p.InputHandler(); h != nil {
				h(out, func(x tview.Primitive) { a.setFocus(x) })
			}
		}
	}
}

// TestSelectEnterPerContext covers V's entry gate: tree and calendar (drilled
// and un-drilled) accept with the right anchor; the agenda pane refuses.
func TestSelectEnterPerContext(t *testing.T) {
	now := time.Date(2026, 7, 5, 9, 0, 0, 0, time.UTC)
	a := newRootedTestApp(t, now)

	// Tree context: anchor = current task UID.
	a.setMode(modeTasks)
	uid := putTodo(t, a, testCalID(a), "", "task A", now, true)
	a.refresh(uid)
	a.setFocus(a.tree)
	a.enterSelect()
	if !a.selecting || a.selAnchorUID != uid {
		t.Fatalf("tree enterSelect: selecting=%v anchor=%q, want true/%q", a.selecting, a.selAnchorUID, uid)
	}
	a.exitSelect()
	if a.selecting || a.selAnchorUID != "" || !a.selAnchorDay.IsZero() {
		t.Fatal("exitSelect must clear all anchor state")
	}

	// Un-drilled calendar: anchor = the selected day.
	a.setMode(modeCalendar)
	a.enterSelect()
	if !a.selecting || a.selAnchorDay.IsZero() {
		t.Fatalf("calendar enterSelect: selecting=%v dayAnchor=%v", a.selecting, a.selAnchorDay)
	}
	if got := a.selContext(); got != selDays {
		t.Fatalf("selContext = %d, want selDays", got)
	}
	a.exitSelect()

	// Agenda: refused.
	a.setMode(modeAgenda)
	a.enterSelect()
	if a.selecting {
		t.Fatal("agenda must refuse SELECT")
	}
}

// TestSelectBadge: SELECT surfaces in interactionMode, and an inner GRAB wins.
func TestSelectBadge(t *testing.T) {
	a := newRootedTestApp(t, time.Date(2026, 7, 5, 9, 0, 0, 0, time.UTC))
	a.setMode(modeCalendar)
	a.enterSelect()
	if m := a.interactionMode(); m != modeSelect {
		t.Fatalf("selecting badge = %q, want SELECT", m)
	}
	a.grabbing = true
	if m := a.interactionMode(); m != modeGrab {
		t.Fatalf("nested grab badge = %q, want GRAB", m)
	}
	a.grabbing = false
	a.exitSelect()
	if m := a.interactionMode(); m != modeNormal {
		t.Fatalf("after exit badge = %q, want NORMAL", m)
	}
}

// TestSelectKeyLayer: motion passes through (the cursor move IS the range
// extension), mutating/context keys are inert, Esc and V exit exactly one level.
func TestSelectKeyLayer(t *testing.T) {
	now := time.Date(2026, 7, 5, 9, 0, 0, 0, time.UTC)
	a := newRootedTestApp(t, now)
	a.setMode(modeTasks)
	u1 := putTodo(t, a, testCalID(a), "", "task A", now, true)
	putTodo(t, a, testCalID(a), "", "task B", now.AddDate(0, 0, 1), true)
	a.refresh(u1)
	a.setFocus(a.tree)
	a.enterSelect()

	// Motion passes through: j moves the tree cursor.
	before := a.currentTreeUID()
	key(a, tcell.KeyRune, 'j')
	if a.currentTreeUID() == before {
		t.Fatal("j must still move the cursor while selecting")
	}

	// Context/data keys are inert: t (pane), . (show-completed), e (edit form).
	key(a, tcell.KeyRune, 't')
	if a.mode != modeTasks {
		t.Fatal("pane switch must be inert while selecting")
	}
	sc := a.showCompleted
	key(a, tcell.KeyRune, '.')
	if a.showCompleted != sc {
		t.Fatal(". must be inert while selecting")
	}
	key(a, tcell.KeyRune, 'e')
	if a.modalOpen() {
		t.Fatal("e must not open the edit form while selecting")
	}

	// Esc exits SELECT only (still in Tasks, cursor kept).
	key(a, tcell.KeyEscape, 0)
	if a.selecting {
		t.Fatal("Esc must exit SELECT")
	}

	// V toggles: enter then V exits.
	a.enterSelect()
	key(a, tcell.KeyRune, 'V')
	if a.selecting {
		t.Fatal("V while selecting must exit SELECT")
	}
}

// TestSelectPrefixGate: while selecting, gg still jumps (pure motion) but gt/gd
// (context jumps / modals) are blocked — gotoToday switches to Calendar mode,
// which would yank the context out from under the range.
func TestSelectPrefixGate(t *testing.T) {
	now := time.Date(2026, 7, 5, 9, 0, 0, 0, time.UTC)
	a := newRootedTestApp(t, now)
	a.setMode(modeTasks)
	u1 := putTodo(t, a, testCalID(a), "", "task A", now, true)
	putTodo(t, a, testCalID(a), "", "task B", now.AddDate(0, 0, 1), true)
	a.refresh(u1)
	a.setFocus(a.tree)
	key(a, tcell.KeyRune, 'j') // move off the top
	a.enterSelect()

	key(a, tcell.KeyRune, 'g')
	key(a, tcell.KeyRune, 't') // gt = gotoToday: must be blocked
	if a.mode != modeTasks || !a.selecting {
		t.Fatal("gt must be blocked while selecting")
	}

	key(a, tcell.KeyRune, 'g')
	key(a, tcell.KeyRune, 'g') // gg = top: pure motion, allowed
	nodes := visibleTreeNodes(a.tree.GetRoot())
	if len(nodes) == 0 || a.tree.GetCurrentNode() != nodes[0] {
		t.Fatal("gg must still jump to the top while selecting")
	}
}

// TestSelectEscKeepsDrill: SELECT entered from a drilled day unwinds to DRILL,
// not to day navigation — Esc backs out exactly one mode level.
func TestSelectEscKeepsDrill(t *testing.T) {
	now := time.Date(2026, 7, 5, 9, 0, 0, 0, time.UTC)
	a := newRootedTestApp(t, now)
	putEvent(t, a, testCalID(a), "meeting", now, false)
	a.setMode(modeCalendar)
	a.refresh("")
	a.month.reDrill(model.DayStart(now), 0)
	if !a.gridDrilled() {
		t.Fatal("precondition: day must be drilled")
	}
	a.enterSelect()
	if a.selContext() != selDrill || a.selAnchorUID == "" {
		t.Fatalf("drilled enterSelect: ctx=%d anchor=%q", a.selContext(), a.selAnchorUID)
	}
	key(a, tcell.KeyEscape, 0)
	if a.selecting {
		t.Fatal("Esc must exit SELECT")
	}
	if !a.gridDrilled() {
		t.Fatal("Esc from SELECT must land back in DRILL, not day navigation")
	}
}
```

Notes for the implementer: `newRootedTestApp` is in `keys_test.go:21`, `putTodo`/`putEvent` in `displaystress_test.go:69/83`. If no `testCalID(a)` helper exists, check how existing tests obtain the seeded calendar id (grep `putTodo(t, a,` for call sites) and reuse that idiom; add a tiny local helper only if none exists. Add imports as needed (`tview`, `model`).

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./internal/ui/ -run 'TestSelect' -v`
Expected: FAIL — `undefined: a.enterSelect`, `undefined: modeSelect`, etc.

- [ ] **Step 3: Implement**

`internal/ui/app.go` — add fields to the `app` struct after `formDrill` (~line 150):

```go
	// SELECT mode (V): a contiguous multi-select range anchored where the mode
	// was entered. The selected set is always DERIVED anchor→cursor in visible
	// order (see selRange), never stored — a stored set would need re-mapping on
	// every refresh, the exact drift class the v1.0.1 cursor fix closed. Exactly
	// one anchor is meaningful per context: tree and drilled-day anchor on an
	// item (selAnchorUID + selAnchorOcc for the occurrence), the un-drilled
	// calendar on a day (selAnchorDay).
	selecting    bool
	selAnchorUID string
	selAnchorOcc time.Time
	selAnchorDay time.Time
```

`internal/ui/selection.go` (new):

```go
package ui

import (
	"time"

	"github.com/gdamore/tcell/v2"

	"github.com/littekge/LazyPlanner/internal/model"
)

// SELECT mode (V) is the multi-select layer: a contiguous range anchored where
// the mode was entered and extended by ordinary cursor motion, then acted on as
// one bulk operation (complete / delete / yank / grab). The range is derived —
// anchor → cursor in visible order — so it can never drift from what's on
// screen. Modes nest: DRILL → SELECT → GRAB.

// The three contexts a SELECT range can span. Derived from the view state on
// demand (selContext), not stored — context-switch keys are inert while
// selecting, so the view state can't change under an active range.
const (
	selNone = iota
	selTree
	selDays
	selDrill
)

const selectHint = "SELECT · move to extend · Space done · d delete · y/Y yank · m grab · Esc cancel"

// selContext identifies which kind of range the current view would give SELECT.
func (a *app) selContext() int {
	switch a.mode {
	case modeTasks:
		return selTree
	case modeCalendar:
		if a.gridDrilled() {
			return selDrill
		}
		return selDays
	}
	return selNone
}

// enterSelect (V) starts SELECT anchored at the current cursor. Only contexts
// with a meaningful contiguous range accept it: the task tree, the un-drilled
// calendar (a day range), and a drilled day (its item list).
func (a *app) enterSelect() {
	switch a.selContext() {
	case selTree:
		uid := a.currentTreeUID()
		if uid == "" {
			a.flash("Nothing to select here")
			return
		}
		a.selecting = true
		a.selAnchorUID = uid
	case selDrill:
		g, _ := a.calendarPrimitive().(calGrid)
		it := g.selectedItem()
		if it == nil {
			a.flash("Nothing to select here")
			return
		}
		t := targetFromItem(*it)
		a.selecting = true
		a.selAnchorUID = t.uid
		a.selAnchorOcc = t.occStart
	case selDays:
		g, ok := a.calendarPrimitive().(calGrid)
		if !ok {
			a.flash("Nothing to select here")
			return
		}
		day, _, _ := g.drillState()
		a.selecting = true
		a.selAnchorDay = model.DayStart(day)
	default:
		a.flash("Nothing to select here (SELECT works in the task tree and calendar)")
		return
	}
	a.syncSelectionVisuals()
	a.flash(selectHint)
}

// exitSelect leaves SELECT, clearing the anchors. The underlying context (a
// drilled day, the tree cursor) is untouched, so Esc backs out exactly one
// mode level.
func (a *app) exitSelect() {
	a.selecting = false
	a.selAnchorUID = ""
	a.selAnchorOcc = time.Time{}
	a.selAnchorDay = time.Time{}
	a.syncSelectionVisuals()
}

// syncSelectionVisuals refreshes everything that displays the selection.
// Extended by later build steps (range validation, view range fields); it is
// always event-driven — never called from a draw path.
func (a *app) syncSelectionVisuals() {
	a.updateStatus()
}

// handleSelectKey routes keys while SELECT is active. Motion returns the event
// unhandled — moving the cursor is how the range extends — the bulk-op keys
// act on the range, and everything else is swallowed so a context switch or
// edit can't happen under an active selection (Esc first, then act).
func (a *app) handleSelectKey(ev *tcell.EventKey) *tcell.EventKey {
	switch ev.Key() {
	case tcell.KeyEscape:
		a.exitSelect()
		a.flash("Select cancelled")
		return nil
	case tcell.KeyLeft, tcell.KeyRight, tcell.KeyUp, tcell.KeyDown, tcell.KeyHome, tcell.KeyEnd:
		return ev
	case tcell.KeyRune:
		switch r := ev.Rune(); {
		case r == 'h' || r == 'j' || r == 'k' || r == 'l' || r == 'G' || r == 'f' || r == 'b':
			return ev // motion / period shift: extends the range
		case r >= '0' && r <= '9':
			return ev // vim counts still apply to motion
		case r == 'g':
			return ev // the g prefix; resolvePrefix gates it to gg while selecting
		case r == 'V':
			a.exitSelect()
			a.flash("Select cancelled")
			return nil
		case r == ' ':
			a.flash("Bulk complete lands in a later build step")
			return nil
		case r == 'd':
			a.flash("Bulk delete lands in a later build step")
			return nil
		case r == 'y' || r == 'Y':
			a.flash("Bulk yank lands in a later build step")
			return nil
		case r == 'm':
			a.flash("Bulk grab lands in a later build step")
			return nil
		}
	}
	return nil // everything else is inert while selecting
}
```

`internal/ui/app.go` — wire the branch into `globalKeys` directly after the `a.resizing` branch (~line 724):

```go
	// SELECT is semi-modal: motion still reaches the views (extending the
	// range); the bulk-op keys and Esc are handled; the rest is swallowed.
	if a.selecting {
		if a.handleSelectKey(ev) == nil {
			return nil
		}
		// fall through: a motion key extends the range via the normal handlers
	}
```

and add the `V` binding in the rune switch (next to `case 'v':`):

```go
		case 'V':
			a.enterSelect()
			return nil
```

`internal/ui/render.go` — add the mode const and badge branch:

```go
const (
	modeNormal = "NORMAL"
	modeDrill  = "DRILL"
	modeGrab   = "GRAB"
	modeResize = "RESIZE"
	modeSelect = "SELECT"
)
```

In `interactionMode()` insert between the `modalOpen` and `gridDrilled` cases (innermost mode wins: a nested grab shows GRAB, a confirm modal shows the form badge, and SELECT outranks the drill it may sit on):

```go
	case a.selecting:
		return modeSelect
```

In `drawModeIndicator` add a chip case (reads only the derived string — draw-lock safe):

```go
	case modeSelect:
		style = tcell.StyleDefault.Foreground(tcell.ColorBlack).Background(tcell.ColorLime).Bold(true)
```

In `updateStatus`, override the hints line while selecting (place just before `a.hints.SetText(...)`, wrapping it in an else, or early-return after setting the select hints — keep the statusLeft/sync sections running either way):

```go
	if a.selecting {
		a.hints.SetText("SELECT · hjkl extend · gg/G ends · Space done · d delete · y/Y yank · m grab · Esc/V cancel")
		return
	}
```

`internal/ui/keys.go` — gate `resolvePrefix` (insert right after the `clearPrefix()` call and the non-rune early return, ~line 97):

```go
	// While SELECT is active only the pure-motion chord (gg) may run — every
	// other continuation edits data or jumps context under the active range
	// (gt switches to Calendar mode; gd/i/s/z open modals or mutate).
	if a.selecting && !(p == 'g' && ev.Rune() == 'g') {
		a.flash("Not available while selecting (Esc to cancel)")
		return
	}
```

`internal/ui/mouse.go` — gate `mouseCapture` (top of the function):

```go
	// SELECT is keyboard-modal like grab: a click could switch panes or move
	// the context under the active range, so mouse input is inert until Esc.
	if a.selecting {
		return nil, action
	}
```

`internal/ui/mode_test.go` — extend `TestInteractionMode` with SELECT cases (after the grab block):

```go
	a.selecting = true
	if m := a.interactionMode(); m != modeSelect {
		t.Errorf("selecting mode = %q, want SELECT", m)
	}
	a.grabbing = true
	if m := a.interactionMode(); m != modeGrab {
		t.Errorf("selecting+grabbing mode = %q, want GRAB (innermost wins)", m)
	}
	a.grabbing, a.selecting = false, false
```

and extend `TestModeIndicatorRenders`:

```go
	a.grabbing = false
	a.selecting = true
	if s := dump(); !strings.Contains(s, "SELECT") {
		t.Error("status bar should show the SELECT badge while selecting")
	}
	a.selecting = false
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./internal/ui/ -run 'TestSelect|TestInteractionMode|TestModeIndicator' -v`
Expected: PASS (all). Also run `go test ./internal/ui/` to confirm nothing else broke (notably `modedeadlock_test.go` and `displaystress_test.go`).

- [ ] **Step 5: Full gate, log entry, commit**

Run: `go test ./... && go vet ./... && staticcheck ./... && go build ./...`
Append a `log.md` entry (`## 2026-07-23 — v1.4.0: SELECT mode core (state, badge, key layer)`), then:

```bash
git add -A && git commit -m "feat: SELECT mode core — V enter/exit, badge, semi-modal key layer"
```

---

### Task 2: Range derivation — selRange per context

**Files:**
- Modify: `internal/ui/selection.go`
- Modify: `internal/ui/selection_test.go`

**Interfaces:**
- Consumes: `visibleTreeNodes` (`keys.go:191`), `a.dayItems(day)` (`render.go:708`), `targetFromItem`, `editTarget` (`edit.go:62`), `calGrid.drillState()`.
- Produces: `a.selRange() []editTarget` (nil = anchor unresolvable → caller exits SELECT), `a.treeRange()`, `a.daysRange()`, `a.drillRange()`, `itemIndex(items []model.AgendaItem, uid string, occStart time.Time) int`, const `maxSelectDays = 366`. `syncSelectionVisuals` gains the anchor-validation guard.

- [ ] **Step 1: Write the failing tests** (append to `selection_test.go`)

```go
// TestTreeRange: anchor→cursor over visible rows, inclusive, either direction;
// nil when the anchor vanished (e.g. a background sync deleted it).
func TestTreeRange(t *testing.T) {
	now := time.Date(2026, 7, 5, 9, 0, 0, 0, time.UTC)
	a := newRootedTestApp(t, now)
	a.setMode(modeTasks)
	var uids []string
	for _, s := range []string{"A", "B", "C", "D", "E"} {
		uids = append(uids, putTodo(t, a, testCalID(a), "", "task "+s, now, true))
	}
	a.refresh(uids[0])
	a.setFocus(a.tree)

	// Visible order is the smart sort (same due date → title order A..E).
	a.selectTreeByUID(uids[1]) // anchor at B
	a.enterSelect()
	a.selectTreeByUID(uids[3]) // cursor at D
	got := a.selRange()
	if len(got) != 3 || got[0].uid != uids[1] || got[2].uid != uids[3] {
		t.Fatalf("forward range = %+v, want B..D", got)
	}

	// Reversed: cursor above the anchor selects the same rows.
	a.selectTreeByUID(uids[0])
	got = a.selRange()
	if len(got) != 2 || got[0].uid != uids[0] || got[1].uid != uids[1] {
		t.Fatalf("reversed range = %+v, want A..B", got)
	}

	// Single-item range: cursor on the anchor.
	a.selectTreeByUID(uids[1])
	if got = a.selRange(); len(got) != 1 || got[0].uid != uids[1] {
		t.Fatalf("single range = %+v, want just B", got)
	}
	a.exitSelect()
}

// TestTreeRangeAnchorVanished: after the anchor's task is gone, selRange is nil
// and syncSelectionVisuals (run by refresh) exits SELECT with a flash.
func TestTreeRangeAnchorVanished(t *testing.T) {
	now := time.Date(2026, 7, 5, 9, 0, 0, 0, time.UTC)
	a := newRootedTestApp(t, now)
	a.setMode(modeTasks)
	u1 := putTodo(t, a, testCalID(a), "", "task A", now, true)
	u2 := putTodo(t, a, testCalID(a), "", "task B", now, true)
	a.refresh(u1)
	a.setFocus(a.tree)
	a.selectTreeByUID(u1)
	a.enterSelect()

	// Simulate a remote deletion landing via sync: delete + refresh.
	loc, _ := a.store.Locate(u1)
	if err := a.store.Delete(context.Background(), loc.CalID, loc.Name); err != nil {
		t.Fatal(err)
	}
	a.refresh(u2)
	if a.selecting {
		t.Fatal("SELECT must exit when the anchor vanishes")
	}
}

// TestDaysRange: the date interval materializes every visible item once — a
// multi-day event spanning several selected days is deduped to one target.
func TestDaysRange(t *testing.T) {
	now := time.Date(2026, 7, 6, 9, 0, 0, 0, time.UTC) // Monday
	a := newRootedTestApp(t, now)
	a.setMode(modeCalendar)
	putEvent(t, a, testCalID(a), "day1", now, false)
	putEvent(t, a, testCalID(a), "day3", now.AddDate(0, 0, 2), false)
	// A two-day timed event covering day1→day2 (spans midnight).
	putSpanningEvent(t, a, testCalID(a), "span", now, now.AddDate(0, 0, 1).Add(2*time.Hour))
	a.refresh("")

	a.month.selected = model.DayStart(now)
	a.enterSelect()
	a.month.selected = model.DayStart(now.AddDate(0, 0, 2)) // extend to day3
	got := a.selRange()
	count := map[string]int{}
	for _, tg := range got {
		count[tg.uid]++
	}
	if len(got) != 3 {
		t.Fatalf("range = %d targets, want 3 (day1, span, day3)", len(got))
	}
	for uid, n := range count {
		if n != 1 {
			t.Fatalf("uid %s appears %d times, want 1 (multi-day dedupe)", uid, n)
		}
	}
	a.exitSelect()
}

// TestSelectRangeSyncRace: derive the range continuously while a background
// goroutine mutates the store (the sync scenario) — run under -race. The store
// is internally locked; this asserts derivation never panics or returns
// targets for items that are mid-deletion, in the TestConcurrentSyncAndEditsRace
// mold (internal/sync/sync_test.go:372).
func TestSelectRangeSyncRace(t *testing.T) {
	now := time.Date(2026, 7, 5, 9, 0, 0, 0, time.UTC)
	a := newRootedTestApp(t, now)
	a.setMode(modeTasks)
	anchor := putTodo(t, a, testCalID(a), "", "anchor", now, true)
	for i := 0; i < 20; i++ {
		putTodo(t, a, testCalID(a), "", fmt.Sprintf("task %02d", i), now, true)
	}
	a.refresh(anchor)
	a.setFocus(a.tree)
	a.selectTreeByUID(anchor)
	a.enterSelect()

	// Snapshot one resource to churn: the goroutine may not touch t (t.Fatal is
	// test-goroutine-only), so it uses raw store calls exclusively.
	churn := putTodo(t, a, testCalID(a), "", "churn", now, true)
	churnLoc, ok := a.store.Locate(churn)
	if !ok {
		t.Fatal("churn task missing")
	}
	stop := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(1)
	go func() { // the "sync" goroutine: delete/recreate a resource repeatedly
		defer wg.Done()
		ctx := context.Background()
		for {
			select {
			case <-stop:
				return
			default:
			}
			_ = a.store.Delete(ctx, churnLoc.CalID, churnLoc.Name)
			_, _ = a.store.Put(ctx, churnLoc.CalID, churnLoc.Name, churnLoc.Object)
		}
	}()
	for i := 0; i < 500; i++ {
		_ = a.selRange() // must never panic; content raced deliberately
	}
	close(stop)
	wg.Wait()
	a.exitSelect()
}

// TestDrillRange: within a drilled day, the range covers the item list between
// anchor and cursor in drill-cycling order.
func TestDrillRange(t *testing.T) {
	now := time.Date(2026, 7, 5, 9, 0, 0, 0, time.UTC)
	a := newRootedTestApp(t, now)
	a.setMode(modeCalendar)
	putEvent(t, a, testCalID(a), "e1", now, false)
	putEvent(t, a, testCalID(a), "e2", now.Add(time.Hour), false)
	putEvent(t, a, testCalID(a), "e3", now.Add(2*time.Hour), false)
	a.refresh("")
	a.month.reDrill(model.DayStart(now), 0)
	a.enterSelect()
	a.month.eventIndex = 2 // cursor on the third item
	got := a.selRange()
	if len(got) != 3 {
		t.Fatalf("drill range = %d targets, want 3", len(got))
	}
}
```

`putSpanningEvent` won't exist: add it to `selection_test.go`, modeled on `putEvent` (`displaystress_test.go:83`) but taking an explicit end time — copy that helper's store-write idiom and set the event's DTEND to the given end.

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./internal/ui/ -run 'TestTreeRange|TestDaysRange|TestDrillRange' -v`
Expected: FAIL — `undefined: a.selRange` (and helpers).

- [ ] **Step 3: Implement** (append to `internal/ui/selection.go`)

```go
// maxSelectDays caps a calendar day-range so f/b can't build a multi-year
// span whose materialization (dayItems per day) would stall the UI.
const maxSelectDays = 366

// selRange materializes the selection into targets in visible order. nil means
// the anchor can no longer be resolved (deleted remotely, day items changed) —
// callers exit SELECT rather than guess.
func (a *app) selRange() []editTarget {
	if !a.selecting {
		return nil
	}
	switch a.selContext() {
	case selTree:
		return a.treeRange()
	case selDays:
		return a.daysRange()
	case selDrill:
		return a.drillRange()
	}
	return nil
}

// treeRange walks the visible tree rows (display order, collapsed subtrees
// excluded — fold keys are inert while selecting) and slices anchor→cursor.
func (a *app) treeRange() []editTarget {
	var rows []*model.Todo
	ai, ci := -1, -1
	cur := a.tree.GetCurrentNode()
	for _, n := range visibleTreeNodes(a.tree.GetRoot()) {
		td, ok := n.GetReference().(*model.Todo)
		if !ok {
			continue
		}
		if td.UID == a.selAnchorUID {
			ai = len(rows)
		}
		if n == cur {
			ci = len(rows)
		}
		rows = append(rows, td)
	}
	if ai < 0 || ci < 0 {
		return nil
	}
	if ai > ci {
		ai, ci = ci, ai
	}
	out := make([]editTarget, 0, ci-ai+1)
	for _, td := range rows[ai : ci+1] {
		out = append(out, editTarget{isTodo: true, uid: td.UID, occStart: td.Due, allDay: td.DueAllDay, recurring: td.Recurring})
	}
	return out
}

// daysRange materializes every visible item on the selected date interval.
// Hidden calendars are excluded (dayItems already filters them); a multi-day
// event spanning several selected days is deduped to one target.
func (a *app) daysRange() []editTarget {
	g, ok := a.calendarPrimitive().(calGrid)
	if !ok || a.selAnchorDay.IsZero() {
		return nil
	}
	day, _, _ := g.drillState()
	from, to := a.selAnchorDay, model.DayStart(day)
	if from.After(to) {
		from, to = to, from
	}
	if to.Sub(from) > maxSelectDays*24*time.Hour {
		to = from.AddDate(0, 0, maxSelectDays)
	}
	var out []editTarget
	seen := map[string]bool{}
	for d := from; !d.After(to); d = d.AddDate(0, 0, 1) {
		for _, it := range a.dayItems(d) {
			t := targetFromItem(it)
			if seen[t.uid] {
				continue
			}
			seen[t.uid] = true
			out = append(out, t)
		}
	}
	return out
}

// drillRange slices the drilled day's item list anchor→cursor in the same
// linear order Enter cycles (both grids drill the model.DayAgenda order).
func (a *app) drillRange() []editTarget {
	g, ok := a.calendarPrimitive().(calGrid)
	if !ok {
		return nil
	}
	day, drilled, idx := g.drillState()
	if !drilled {
		return nil
	}
	items := a.dayItems(day)
	ai := itemIndex(items, a.selAnchorUID, a.selAnchorOcc)
	if ai < 0 || idx < 0 || idx >= len(items) {
		return nil
	}
	ci := idx
	if ai > ci {
		ai, ci = ci, ai
	}
	out := make([]editTarget, 0, ci-ai+1)
	for _, it := range items[ai : ci+1] {
		out = append(out, targetFromItem(it))
	}
	return out
}

// itemIndex finds the item with uid at occStart in a day's agenda order, or -1.
func itemIndex(items []model.AgendaItem, uid string, occStart time.Time) int {
	for i, it := range items {
		t := targetFromItem(it)
		if t.uid == uid && t.occStart.Equal(occStart) {
			return i
		}
	}
	return -1
}
```

Replace `syncSelectionVisuals` with the validation-bearing version:

```go
// syncSelectionVisuals refreshes everything that displays the selection and
// validates the anchor: if the range can no longer be derived (the anchor was
// deleted remotely, the drilled day's items changed), SELECT exits with a
// flash rather than acting on a guess. Event-driven only — never a draw path.
func (a *app) syncSelectionVisuals() {
	if a.selecting && a.selRange() == nil {
		a.selecting = false
		a.selAnchorUID = ""
		a.selAnchorOcc = time.Time{}
		a.selAnchorDay = time.Time{}
		a.updateStatus()
		a.flash("Selection cleared — the items changed")
		return
	}
	a.updateStatus()
}
```

Add `a.syncSelectionVisuals()` as the last line of `refresh(selUID)` (`edit.go:733`, after `a.updateStatus()`).

Also surface the count: in `updateStatus` (`render.go:675`), after `left` is assembled and before `a.statusLeft.SetText(left)`:

```go
	if a.selecting {
		left = fmt.Sprintf("%d selected", len(a.selRange())) + " · " + left
	}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./internal/ui/ -run 'TestSelect|TestTreeRange|TestDaysRange|TestDrillRange' -v` and `go test -race ./internal/ui/ -run 'TestSelectRangeSyncRace' -v`
Expected: PASS, race detector clean.

- [ ] **Step 5: Full gate, log entry, commit**

Full gate; `log.md` entry (`v1.4.0: SELECT range derivation`); commit:

```bash
git add -A && git commit -m "feat: SELECT range derivation — tree/days/drill anchor→cursor materialization"
```

---

### Task 3: Selection visuals — tree, month grid, time grid, display-stress

**Files:**
- Modify: `internal/ui/selection.go` (`syncSelectionVisuals`, `syncTreeSelection`, shared `dayInRange`)
- Modify: `internal/ui/calendarview.go` (range fields + `drawCell`)
- Modify: `internal/ui/timegridview.go` (range fields + `Draw` header/blocks)
- Modify: `internal/ui/app.go` (tree `SetChangedFunc` wrapper ~line 595)
- Create: `internal/ui/selectionvisuals_test.go`
- Modify: `internal/ui/displaystress_test.go` (draw with an active range)

**Interfaces:**
- Consumes: Task 2's `selRange`/`itemIndex`; `selectionStyle` (`app.go:37`); `drawBox`/`printStyled` (`calendarview.go`); `visibleTreeNodes`.
- Produces: view fields `selDayAnchor time.Time`, `selAnchorUID string`, `selAnchorOcc time.Time` on both `calendarView` and `timeGridView`; `dayInRange(anchor, cursor, day time.Time) bool`; `a.syncTreeSelection()`. All read at draw time as plain fields (draw-lock rule).

- [ ] **Step 1: Write the failing tests** (`internal/ui/selectionvisuals_test.go`)

```go
package ui

import (
	"strings"
	"testing"
	"time"

	"github.com/gdamore/tcell/v2"

	"github.com/littekge/LazyPlanner/internal/model"
)

// TestTreeSelectionReverseVideo: rows inside the range carry the theme-adaptive
// selectionStyle (the legibility guardrail class); rows outside stay default;
// exiting SELECT restores every row.
func TestTreeSelectionReverseVideo(t *testing.T) {
	now := time.Date(2026, 7, 5, 9, 0, 0, 0, time.UTC)
	a := newRootedTestApp(t, now)
	a.setMode(modeTasks)
	var uids []string
	for _, s := range []string{"A", "B", "C"} {
		uids = append(uids, putTodo(t, a, testCalID(a), "", "task "+s, now, true))
	}
	a.refresh(uids[0])
	a.setFocus(a.tree)
	a.selectTreeByUID(uids[0])
	a.enterSelect()
	a.selectTreeByUID(uids[1])
	a.syncSelectionVisuals()

	styles := map[string]tcell.Style{}
	for _, n := range visibleTreeNodes(a.tree.GetRoot()) {
		if td, ok := n.GetReference().(*model.Todo); ok {
			styles[td.UID] = n.GetTextStyle()
		}
	}
	if styles[uids[0]] != selectionStyle || styles[uids[1]] != selectionStyle {
		t.Fatal("in-range rows must carry selectionStyle (reverse video)")
	}
	if styles[uids[2]] == selectionStyle {
		t.Fatal("out-of-range rows must stay default")
	}
	a.exitSelect()
	for _, n := range visibleTreeNodes(a.tree.GetRoot()) {
		if td, ok := n.GetReference().(*model.Todo); ok && n.GetTextStyle() == selectionStyle {
			t.Fatalf("row %s must be restored on exit", td.UID)
		}
	}
}

// TestMonthGridDayRangeBoxes: every day in the range draws an outline box (the
// cursor day keeps the focused style), verified by corner glyphs in the cells.
func TestMonthGridDayRangeBoxes(t *testing.T) {
	now := time.Date(2026, 7, 6, 9, 0, 0, 0, time.UTC)
	a := newRootedTestApp(t, now)
	a.setMode(modeCalendar)
	a.refresh("")
	a.month.selected = model.DayStart(now)
	a.enterSelect()
	a.month.selected = model.DayStart(now.AddDate(0, 0, 2))
	a.syncSelectionVisuals()

	cells, cw, ch := drawCells(t, a.month, 84, 30)
	var b strings.Builder
	for row := 0; row < ch; row++ {
		for col := 0; col < cw; col++ {
			if c := cells[row*cw+col]; len(c.Runes) > 0 {
				b.WriteRune(c.Runes[0])
			} else {
				b.WriteByte(' ')
			}
		}
	}
	// Three boxed days → at least three top-left rounded corners on the grid.
	if n := strings.Count(b.String(), "╭"); n < 3 {
		t.Fatalf("expected ≥3 day boxes for a 3-day range, found %d corners", n)
	}
	a.exitSelect()
}

// TestDrilledRangeMarksItems: in a drilled month cell, items between anchor and
// cursor draw reverse-video.
func TestDrilledRangeMarksItems(t *testing.T) {
	now := time.Date(2026, 7, 5, 9, 0, 0, 0, time.UTC)
	a := newRootedTestApp(t, now)
	a.setMode(modeCalendar)
	putEvent(t, a, testCalID(a), "eventone", now, false)
	putEvent(t, a, testCalID(a), "eventtwo", now.Add(time.Hour), false)
	a.refresh("")
	a.month.reDrill(model.DayStart(now), 0)
	a.enterSelect()
	a.month.eventIndex = 1
	a.syncSelectionVisuals()

	cells, cw, ch := drawCells(t, a.month, 84, 30)
	reversed := 0
	for row := 0; row < ch; row++ {
		for col := 0; col < cw; col++ {
			c := cells[row*cw+col]
			if _, _, attrs := c.Style.Decompose(); attrs&tcell.AttrReverse != 0 {
				reversed++
			}
		}
	}
	// Two item lines reversed: clearly more reversed cells than a single-item
	// drill (the pre-SELECT baseline is one line's worth).
	if reversed < len("eventone")+len("eventtwo") {
		t.Fatalf("expected both range items reversed, got %d reversed cells", reversed)
	}
	a.exitSelect()
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./internal/ui/ -run 'TestTreeSelectionReverse|TestMonthGridDayRange|TestDrilledRangeMarks' -v`
Expected: FAIL (no styling logic yet; `GetTextStyle` returns default, corner/reverse counts too low).

- [ ] **Step 3: Implement**

Shared helper in `selection.go`:

```go
// dayInRange reports whether day falls inside [anchor, cursor] (either order).
// A zero anchor means no active day-range.
func dayInRange(anchor, cursor, day time.Time) bool {
	if anchor.IsZero() {
		return false
	}
	from, to := anchor, cursor
	if from.After(to) {
		from, to = to, from
	}
	d := model.DayStart(day)
	return !d.Before(model.DayStart(from)) && !d.After(model.DayStart(to))
}
```

Tree styling in `selection.go` (event-driven — wired below, never from Draw):

```go
// syncTreeSelection re-styles the visible tree rows to mark the range. In-range
// rows carry the theme-adaptive selectionStyle (reverse video — the legibility
// guardrail: never a hardcoded color pair on the terminal-default background).
func (a *app) syncTreeSelection() {
	inRange := map[string]bool{}
	if a.selecting && a.selContext() == selTree {
		for _, t := range a.treeRange() {
			inRange[t.uid] = true
		}
	}
	for _, n := range visibleTreeNodes(a.tree.GetRoot()) {
		td, ok := n.GetReference().(*model.Todo)
		if !ok {
			continue
		}
		if inRange[td.UID] {
			n.SetTextStyle(selectionStyle)
		} else {
			n.SetTextStyle(tcell.StyleDefault)
		}
	}
}
```

Extend `syncSelectionVisuals` — full replacement (pushes plain fields into the grids; they derive the other end from their own `selected`/`eventIndex` at draw time, so per-move sync is only needed for the tree and the count):

```go
func (a *app) syncSelectionVisuals() {
	if a.selecting && a.selRange() == nil {
		a.selecting = false
		a.selAnchorUID = ""
		a.selAnchorOcc = time.Time{}
		a.selAnchorDay = time.Time{}
		a.flash("Selection cleared — the items changed")
	}
	a.month.selDayAnchor, a.timegrid.selDayAnchor = time.Time{}, time.Time{}
	a.month.selAnchorUID, a.timegrid.selAnchorUID = "", ""
	a.month.selAnchorOcc, a.timegrid.selAnchorOcc = time.Time{}, time.Time{}
	if a.selecting {
		switch a.selContext() {
		case selDays:
			a.month.selDayAnchor, a.timegrid.selDayAnchor = a.selAnchorDay, a.selAnchorDay
		case selDrill:
			a.month.selAnchorUID, a.timegrid.selAnchorUID = a.selAnchorUID, a.selAnchorUID
			a.month.selAnchorOcc, a.timegrid.selAnchorOcc = a.selAnchorOcc, a.selAnchorOcc
		}
	}
	a.syncTreeSelection()
	a.updateStatus()
}
```

(The flash lands after `updateStatus` inside `flash` itself persists until the next `updateStatus`; keep the call order above — validation flash after clearing state is acceptable because `updateStatus` runs once more at the end and `flash` writes `statusLeft` directly. If the cleared-flash proves invisible in the test, move `a.flash(...)` after `a.updateStatus()`.)

`internal/ui/calendarview.go` — add fields to `calendarView` after `eventIndex`:

```go
	// SELECT-mode range, set by the app event-side and read at draw time (plain
	// fields only — the draw-lock rule). selDayAnchor is the day-range anchor
	// (zero = none; the other end is cv.selected); selAnchorUID/selAnchorOcc
	// identify the drilled-item anchor ("" = none; the other end is eventIndex).
	selDayAnchor time.Time
	selAnchorUID string
	selAnchorOcc time.Time
```

In `drawCell` (line ~245), extend the box logic — replace the `if selected { ... }` block:

```go
	if selected {
		boxColor := borderIdle
		if cv.HasFocus() {
			boxColor = borderFocused
		}
		drawBox(screen, cellX, cellY, cellW, cellH, tcell.StyleDefault.Foreground(boxColor))
		cx, cy, cw, ch = cellX+1, cellY+1, cellW-2, cellH-2
	} else if dayInRange(cv.selDayAnchor, cv.selected, day) {
		// A day inside the SELECT range gets the accent outline; the cursor day
		// keeps the focused style above so the two ends stay distinguishable.
		drawBox(screen, cellX, cellY, cellW, cellH, tcell.StyleDefault.Foreground(accentColor))
		cx, cy, cw, ch = cellX+1, cellY+1, cellW-2, cellH-2
	}
```

Still in `drawCell`, before the `drawItem` closure, compute the drilled range once:

```go
	selFrom, selTo := -1, -1
	if selected && cv.eventMode && cv.selAnchorUID != "" {
		if ai := itemIndex(items, cv.selAnchorUID, cv.selAnchorOcc); ai >= 0 {
			selFrom, selTo = ai, cv.eventIndex
			if selFrom > selTo {
				selFrom, selTo = selTo, selFrom
			}
		}
	}
```

and replace the highlight line inside `drawItem` (`if selected && cv.eventMode && i == cv.eventIndex { style = style.Reverse(true) }`):

```go
		if selected && cv.eventMode {
			switch {
			case i == cv.eventIndex:
				style = style.Reverse(true)
				if selFrom >= 0 {
					// Range active: the cursor item stays distinguishable from
					// the reversed range rows.
					style = style.Bold(true).Underline(true)
				}
			case selFrom >= 0 && i >= selFrom && i <= selTo:
				style = style.Reverse(true)
			}
		}
```

`internal/ui/timegridview.go` — add the same three fields to `timeGridView` (after `eventIndex`, same comment). In `Draw`'s header loop (~line 540), replace the style logic:

```go
	for di, day := range tg.days {
		style := tcell.StyleDefault.Foreground(accentColor).Bold(true)
		cur := model.SameDay(day, tg.selected)
		switch {
		case cur && !tg.selDayAnchor.IsZero():
			style = style.Reverse(true).Underline(true) // cursor end of a day-range
		case cur, dayInRange(tg.selDayAnchor, tg.selected, day):
			style = style.Reverse(true)
		}
		printStyled(screen, colStart+di*colW+1, y, colW-1, day.Format("Mon 2"), style)
	}
```

For the drilled item-range, compute the window once near the top of `Draw` (after `sel := tg.selectedItem()`):

```go
	selFrom, selTo := -1, -1
	if tg.eventMode && tg.selAnchorUID != "" {
		if ai := itemIndex(tg.daySelectables(), tg.selAnchorUID, tg.selAnchorOcc); ai >= 0 {
			selFrom, selTo = ai, tg.eventIndex
			if selFrom > selTo {
				selFrom, selTo = selTo, selFrom
			}
		}
	}
	inSelRange := func(uid string, start time.Time) bool {
		if selFrom < 0 {
			return false
		}
		i := itemIndex(tg.daySelectables(), uid, start)
		return i >= selFrom && i <= selTo
	}
```

then extend the two per-item `selected := ...` computations in the block/task-marker loops (~lines 632 and 640) to also highlight range members:

```go
			selected := sel != nil && sel.Event != nil && !sel.Event.AllDay && model.SameDay(day, tg.selected) &&
				p.Occ.Event == sel.Event && p.Occ.Start.Equal(sel.Start)
			if !selected && model.SameDay(day, tg.selected) && p.Occ.Event != nil {
				selected = inSelRange(p.Occ.Event.UID, p.Occ.Start)
			}
```

```go
			selected := sel != nil && sel.Todo != nil && sel.Todo.UID == t.UID && model.SameDay(day, tg.selected)
			if !selected && model.SameDay(day, tg.selected) {
				selected = inSelRange(t.UID, t.Due)
			}
```

(The collapsed all-day band keeps its existing cursor-only highlight — the `+N` collapse already hides individual membership; documented limitation.)

`internal/ui/app.go` line ~595 — wrap the tree changed-func so cursor moves restyle the range and refresh the count:

```go
	a.tree.SetChangedFunc(func(node *tview.TreeNode) {
		a.showTreeNode(node)
		if a.selecting {
			a.syncSelectionVisuals()
		}
	})
```

Locate the grid callbacks wired in `app.go` (`onSelectDay` / `onSelectEvent` closures for `a.month` and `a.timegrid`, in the build wiring around lines 540–600) and append the same guarded call inside each closure body:

```go
		if a.selecting {
			a.syncSelectionVisuals()
		}
```

`internal/ui/displaystress_test.go` — extend the harness: find the sub-test that draws `a.month` / `a.timegrid` / the tree across `stressGeoms` with `nastyStrings` content, and add a variant (same loop shape) that first activates a range:

```go
	// SELECT-mode range active: the range visuals must survive the same hostile
	// content and geometries (new draw branches = new freeze/panic surface).
	a.setMode(modeCalendar)
	a.month.selDayAnchor = model.DayStart(now.AddDate(0, 0, -40)) // spans off-view
	a.timegrid.selDayAnchor = a.month.selDayAnchor
	// ...redraw all geometries exactly as the existing loop does...
	a.month.selDayAnchor = time.Time{}
	a.timegrid.selDayAnchor = time.Time{}
```

and a drilled variant setting `selAnchorUID`/`selAnchorOcc` to the first drilled item plus `eventIndex` at the last, then the same geometry sweep. Follow the file's existing watchdog/panic-recover pattern (`drawGeom`).

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./internal/ui/ -run 'TestTreeSelectionReverse|TestMonthGridDayRange|TestDrilledRangeMarks|TestDisplayStress' -v` then the whole package `go test ./internal/ui/`.
Expected: PASS.

- [ ] **Step 5: Full gate, log entry, commit**

```bash
git add -A && git commit -m "feat: SELECT range visuals — tree reverse-video, day-range boxes, drilled marks"
```

---

### Task 4: Bulk complete (Space)

**Files:**
- Create: `internal/ui/bulkops.go`
- Create: `internal/ui/bulkops_test.go`
- Modify: `internal/ui/selection.go` (wire the `' '` case)

**Interfaces:**
- Consumes: `selRange`, `store.Locate/PutIfUnchanged/Restore`, `model.SetTodoCompleted` (`model/edit.go:115`), `model.AdvanceRecurringTodo` (`model/recur_edit.go:661`), `findTodo`, `a.hasIncompleteChildren` (`edit.go:390`), `a.store.Calendar(calID)` for the silent read-only check (`guardWrite`'s test, `calendar.go:33` — without the flash), `pushUndo`, `refreshKeepingDrill`, `stickyDone`.
- Produces: `a.bulkComplete()`, `bulkSkip` counter type + `bulkSummary(verb string, n int, skips bulkSkip) string`, `a.calReadOnly(calID string) bool` — reused by Tasks 5–7.

- [ ] **Step 1: Write the failing tests** (`internal/ui/bulkops_test.go`)

```go
package ui

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/littekge/LazyPlanner/internal/model"
)

// selectTreeRangeAll is a test helper: enter SELECT anchored on the first tree
// row and extend the cursor to the last.
func selectTreeRangeAll(t *testing.T, a *app) {
	t.Helper()
	nodes := visibleTreeNodes(a.tree.GetRoot())
	if len(nodes) == 0 {
		t.Fatal("empty tree")
	}
	a.tree.SetCurrentNode(nodes[0])
	a.enterSelect()
	a.tree.SetCurrentNode(nodes[len(nodes)-1])
}

// TestBulkCompleteRange: every incomplete task in the range completes; the op
// is one undo step; SELECT exits.
func TestBulkCompleteRange(t *testing.T) {
	now := time.Date(2026, 7, 5, 9, 0, 0, 0, time.UTC)
	a := newRootedTestApp(t, now)
	a.setMode(modeTasks)
	var uids []string
	for _, s := range []string{"A", "B", "C"} {
		uids = append(uids, putTodo(t, a, testCalID(a), "", "task "+s, now, true))
	}
	a.refresh(uids[0])
	a.setFocus(a.tree)
	undoBefore := len(a.undo)
	selectTreeRangeAll(t, a)
	a.bulkComplete()

	if a.selecting {
		t.Fatal("bulk complete must exit SELECT")
	}
	for _, u := range uids {
		loc, _ := a.store.Locate(u)
		if td := findTodo(loc.Object, u); td == nil || !td.Completed() {
			t.Fatalf("task %s not completed", u)
		}
	}
	if len(a.undo) != undoBefore+1 {
		t.Fatalf("bulk complete must push exactly one undo step, got %d", len(a.undo)-undoBefore)
	}
	a.undoLast()
	for _, u := range uids {
		loc, _ := a.store.Locate(u)
		if td := findTodo(loc.Object, u); td == nil || td.Completed() {
			t.Fatalf("undo must reopen task %s", u)
		}
	}
}

// TestBulkCompleteFolderChildFirst: processing runs children-first (reverse
// visible order), so selecting a folder together with its only incomplete
// child completes both — the child's completion un-folders the parent.
func TestBulkCompleteFolderChildFirst(t *testing.T) {
	now := time.Date(2026, 7, 5, 9, 0, 0, 0, time.UTC)
	a := newRootedTestApp(t, now)
	a.setMode(modeTasks)
	parent := putTodo(t, a, testCalID(a), "", "folder", now, true)
	child := putTodo(t, a, testCalID(a), parent, "leaf", now, true)
	a.refresh(parent)
	a.setFocus(a.tree)
	selectTreeRangeAll(t, a)
	a.bulkComplete()

	for _, u := range []string{parent, child} {
		loc, _ := a.store.Locate(u)
		if td := findTodo(loc.Object, u); td == nil || !td.Completed() {
			t.Fatalf("%s not completed (children-first ordering broken)", u)
		}
	}
}

// TestBulkCompleteSkips: events and already-done tasks are skipped and counted;
// a recurring todo advances instead of completing (single-item semantics).
func TestBulkCompleteSkips(t *testing.T) {
	now := time.Date(2026, 7, 6, 9, 0, 0, 0, time.UTC)
	a := newRootedTestApp(t, now)
	a.setMode(modeCalendar)
	putEvent(t, a, testCalID(a), "meeting", now, false)
	due := putTodo(t, a, testCalID(a), "", "due today", now, true)
	recurring := putRecurringTodo(t, a, testCalID(a), "standup", now) // FREQ=DAILY
	a.refresh("")
	a.month.reDrill(model.DayStart(now), 0)
	a.enterSelect()
	items := a.dayItems(model.DayStart(now))
	a.month.eventIndex = len(items) - 1 // extend over the whole day
	a.bulkComplete()

	loc, _ := a.store.Locate(due)
	if td := findTodo(loc.Object, due); td == nil || !td.Completed() {
		t.Fatal("plain task must complete")
	}
	loc, _ = a.store.Locate(recurring)
	if td := findTodo(loc.Object, recurring); td == nil || td.Completed() {
		t.Fatal("recurring todo must advance, not complete")
	} else if !td.Due.After(now) {
		t.Fatalf("recurring todo due must advance past %v, got %v", now, td.Due)
	}
	if s := a.statusLeft.GetText(true); !strings.Contains(s, "skipped") {
		t.Fatalf("summary must report the skipped event, got %q", s)
	}
}

// TestBulkCompleteStaleRollsBack: a stale item mid-batch rolls back the items
// already written — all-or-nothing, no partial completion and no undo step.
func TestBulkCompleteStaleRollsBack(t *testing.T) {
	now := time.Date(2026, 7, 5, 9, 0, 0, 0, time.UTC)
	a := newRootedTestApp(t, now)
	a.setMode(modeTasks)
	u1 := putTodo(t, a, testCalID(a), "", "task A", now, true)
	u2 := putTodo(t, a, testCalID(a), "", "task B", now, true)
	a.refresh(u1)
	a.setFocus(a.tree)
	undoBefore := len(a.undo)
	selectTreeRangeAll(t, a)

	// Make u1 stale underneath the selection: mutate it directly (as a sync
	// pull would) after the range is anchored. Reverse visible order means B
	// (u2) writes first, then A (u1) detects stale → B must roll back.
	loc, _ := a.store.Locate(u1)
	obj, err := model.SetTodoCompleted(loc.Object, u1, false, now, time.UTC)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := a.store.Put(context.Background(), loc.CalID, loc.Name, obj); err != nil {
		t.Fatal(err)
	}

	a.bulkComplete()
	loc2, _ := a.store.Locate(u2)
	if td := findTodo(loc2.Object, u2); td == nil || td.Completed() {
		t.Fatal("stale abort must roll back the already-completed sibling")
	}
	if len(a.undo) != undoBefore {
		t.Fatal("a rolled-back bulk op must not push an undo step")
	}
}
```

Add a `putRecurringTodo` helper next to `putSpanningEvent` (mirror `putTodo` but set an RRULE `FREQ=DAILY` on the draft — grep `TodoDraft{` in existing tests for the draft-based creation idiom, e.g. in `internal/ui` tests that build recurring todos: see `recurbugfix_test.go`/`grab_test.go` for the existing helper if one exists, and reuse it instead of writing a new one).

Note on the stale test: `bulkComplete` captures `selRange()` (with `Locate` per item) at execution, so the direct `Put` above changes the resource *after* anchoring but *before* the op's own `Locate` — to force staleness the test must instead interleave: if the simple version passes trivially (no stale detected because `Locate` re-reads fresh), wrap the second write between the op's materialize and execute by making u1's *stored Prev* diverge: perform the mutation, then call `a.bulkComplete()` with a doctored `loc.Prev`. If that's impractical, follow the existing stale-write test idiom — grep `PutIfUnchanged` in `internal/ui/*_test.go` (e.g. the grab stale tests) and copy how they fabricate staleness.

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/ui/ -run 'TestBulkComplete' -v`
Expected: FAIL — `undefined: a.bulkComplete`.

- [ ] **Step 3: Implement** (`internal/ui/bulkops.go`)

```go
package ui

import (
	"context"
	"fmt"
	"sort"

	"github.com/littekge/LazyPlanner/internal/model"
)

// Bulk operations run over the SELECT range with one shared shape:
// materialize → filter (counting skips) → execute with rollback → summarize.
// Execution follows the moveSubtree template: every write is version-checked,
// any failure or stale result rolls back the writes already made (newest
// first), and full success pushes ONE compound undo step.

// bulkSkip counts filtered-out items per reason for the summary flash.
type bulkSkip map[string]int

func (s bulkSkip) add(reason string) { s[reason]++ }

// summary renders the counts deterministically (sorted by reason).
func (s bulkSkip) summary() string {
	if len(s) == 0 {
		return ""
	}
	reasons := make([]string, 0, len(s))
	for r := range s {
		reasons = append(reasons, r)
	}
	sort.Strings(reasons)
	out := ""
	for i, r := range reasons {
		if i > 0 {
			out += " · "
		}
		out += fmt.Sprintf("%d %s", s[r], r)
	}
	return out
}

func bulkSummary(verb string, n int, skips bulkSkip) string {
	s := fmt.Sprintf("%d %s", n, verb)
	if sk := skips.summary(); sk != "" {
		s += " · skipped: " + sk
	}
	return s
}

// calReadOnly is guardWrite's test without the flash, for bulk filters that
// count read-only items instead of aborting on the first one.
func (a *app) calReadOnly(calID string) bool {
	cal, ok := a.store.Calendar(calID)
	return ok && cal.ReadOnly
}

// bulkComplete (Space in SELECT) completes every incomplete task in the range:
// plain tasks are marked done, recurring todos advance one occurrence — the
// single-item Space semantics applied per item. Events, folders with
// incomplete children, already-done tasks, and read-only items are skipped and
// counted. Processing runs in REVERSE visible order (children before parents),
// so completing a folder together with its last incomplete child works in one
// pass. All-or-nothing: a failed or stale write rolls back this op's writes.
func (a *app) bulkComplete() {
	targets := a.selRange()
	if targets == nil {
		a.exitSelect()
		a.flash("Selection no longer valid")
		return
	}
	ctx := context.Background()
	skips := bulkSkip{}
	var ops []undoOp
	var rollback []func()
	fail := func(msg string) {
		for i := len(rollback) - 1; i >= 0; i-- {
			rollback[i]()
		}
		a.exitSelect()
		a.refreshKeepingDrill("")
		a.flash(msg)
	}
	done := 0
	var sticky []string
	for i := len(targets) - 1; i >= 0; i-- {
		t := targets[i]
		if !t.isTodo {
			skips.add("event(s)")
			continue
		}
		loc, ok := a.store.Locate(t.uid)
		if !ok {
			skips.add("missing")
			continue
		}
		if a.calReadOnly(loc.CalID) {
			skips.add("read-only")
			continue
		}
		td := findTodo(loc.Object, t.uid)
		if td == nil || td.Completed() {
			skips.add("already done")
			continue
		}
		if a.hasIncompleteChildren(t.uid) {
			skips.add("folder(s) with open subtasks")
			continue
		}
		var newObj *model.Parsed
		var err error
		if td.Recurring {
			newObj, _, err = model.AdvanceRecurringTodo(loc.Object, t.uid, a.now, a.loc)
		} else {
			newObj, err = model.SetTodoCompleted(loc.Object, t.uid, true, a.now, a.loc)
		}
		if err != nil {
			fail("Complete failed: " + err.Error())
			return
		}
		applied, err := a.store.PutIfUnchanged(ctx, loc.CalID, loc.Name, newObj, loc.Prev)
		if err != nil {
			fail("Complete failed: " + err.Error())
			return
		}
		if !applied {
			fail("An item changed on the server — nothing completed; retry")
			return
		}
		calID, name, prev := loc.CalID, loc.Name, loc.Prev
		rollback = append(rollback, func() { _, _ = a.store.Restore(ctx, calID, name, prev) })
		ops = append(ops, undoOp{calID: calID, name: name, prev: prev})
		if !a.showCompleted {
			sticky = append(sticky, t.uid)
		}
		done++
	}
	if done == 0 {
		// Nothing changed — keep the selection so the user can adjust it.
		a.flash(bulkSummary("completed", 0, skips))
		return
	}
	for _, uid := range sticky {
		a.stickyDone[uid] = true
	}
	a.pushUndo("bulk complete", "", ops...)
	a.exitSelect()
	a.refreshKeepingDrill("")
	a.flash(bulkSummary("completed", done, skips) + undoHint)
}
```

Wire it in `selection.go` — replace the `' '` stub line in `handleSelectKey`:

```go
		case r == ' ':
			a.bulkComplete()
			return nil
```

Check `undoHint`'s exact name/content (grep `undoHint` in `edit.go`) and match the existing suffix idiom.

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/ui/ -run 'TestBulkComplete|TestSelect' -v` then `go test ./internal/ui/`
Expected: PASS.

- [ ] **Step 5: Full gate, log entry, commit**

```bash
git add -A && git commit -m "feat: SELECT bulk complete — children-first, skip counts, rollback, one undo step"
```

---

### Task 5: Bulk delete (d)

**Files:**
- Modify: `internal/ui/bulkops.go`
- Modify: `internal/ui/bulkops_test.go`
- Modify: `internal/ui/selection.go` (wire `'d'`)

**Interfaces:**
- Consumes: `a.descendants(uid)` (`edit.go:396`), `a.confirm(title, text, onYes)` (`edit.go:1012`), `store.Delete/Restore`, `calReadOnly`, `bulkSkip`/`bulkSummary`.
- Produces: `a.bulkDelete()`, `a.bulkDeleteRoots(targets []editTarget) (roots []editTarget, skips bulkSkip)` (ancestor-dedupe + filters, reused by Task 6's yank).

- [ ] **Step 1: Write the failing tests** (append to `bulkops_test.go`)

```go
// TestBulkDeleteDedupeAndUndo: parent+child both selected → the child is
// absorbed into the parent's subtree (deleted once); the confirm names the
// full resource count; one undo step restores everything.
func TestBulkDeleteDedupeAndUndo(t *testing.T) {
	now := time.Date(2026, 7, 5, 9, 0, 0, 0, time.UTC)
	a := newRootedTestApp(t, now)
	a.setMode(modeTasks)
	parent := putTodo(t, a, testCalID(a), "", "folder", now, true)
	child := putTodo(t, a, testCalID(a), parent, "leaf", now, true)
	solo := putTodo(t, a, testCalID(a), "", "solo", now, true)
	a.refresh(parent)
	a.setFocus(a.tree)
	undoBefore := len(a.undo)
	selectTreeRangeAll(t, a) // parent, leaf, solo all in range
	a.bulkDelete()
	confirmYes(t, a) // accept the confirm dialog

	for _, u := range []string{parent, child, solo} {
		if _, ok := a.store.Locate(u); ok {
			t.Fatalf("%s must be deleted", u)
		}
	}
	if len(a.undo) != undoBefore+1 {
		t.Fatalf("bulk delete must be one undo step, got %d", len(a.undo)-undoBefore)
	}
	a.undoLast()
	for _, u := range []string{parent, child, solo} {
		if _, ok := a.store.Locate(u); !ok {
			t.Fatalf("undo must restore %s", u)
		}
	}
}

// TestBulkDeleteSkipsRecurringEvent: a recurring event in a drilled-day range
// is skipped with a count; the non-recurring siblings delete.
func TestBulkDeleteSkipsRecurringEvent(t *testing.T) {
	now := time.Date(2026, 7, 6, 9, 0, 0, 0, time.UTC)
	a := newRootedTestApp(t, now)
	a.setMode(modeCalendar)
	putEvent(t, a, testCalID(a), "oneoff", now, false)
	rec := putRecurringEvent(t, a, testCalID(a), "weekly", now) // FREQ=WEEKLY
	a.refresh("")
	a.month.reDrill(model.DayStart(now), 0)
	a.enterSelect()
	a.month.eventIndex = len(a.dayItems(model.DayStart(now))) - 1
	a.bulkDelete()
	confirmYes(t, a)

	if _, ok := a.store.Locate(rec); !ok {
		t.Fatal("recurring event must be skipped, not deleted")
	}
	if s := a.statusLeft.GetText(true); !strings.Contains(s, "recurring") {
		t.Fatalf("summary must count the recurring skip, got %q", s)
	}
}
```

`confirmYes` / `putRecurringEvent`: grep the test suite for how existing tests press a confirm dialog's Yes button (e.g. the delete tests around `deleteWholeObject` — `calendar_test.go` / `edit_test.go` use a pattern for driving `a.confirm`); reuse that idiom, adding a small local helper if none is shared. `putRecurringEvent` mirrors `putEvent` with an RRULE — reuse the existing recurring-event fixture idiom from `grab_recur_reanchor_test.go`.

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/ui/ -run 'TestBulkDelete' -v`
Expected: FAIL — `undefined: a.bulkDelete`.

- [ ] **Step 3: Implement** (append to `bulkops.go`)

```go
// bulkDeleteRoots dedupes the range for subtree-shaped ops: a selected row
// whose ancestor is also selected is absorbed (its subtree travels with the
// ancestor), and recurring events / read-only / missing items are filtered out
// with counts. Also used by bulk yank.
func (a *app) bulkDeleteRoots(targets []editTarget) ([]editTarget, bulkSkip) {
	skips := bulkSkip{}
	selected := map[string]bool{}
	for _, t := range targets {
		if t.isTodo {
			selected[t.uid] = true
		}
	}
	parentOf := map[string]string{}
	for _, td := range a.store.Todos() {
		parentOf[td.UID] = td.ParentUID
	}
	var roots []editTarget
	for _, t := range targets {
		if !t.isTodo && t.recurring {
			skips.add("recurring")
			continue
		}
		loc, ok := a.store.Locate(t.uid)
		if !ok {
			skips.add("missing")
			continue
		}
		if a.calReadOnly(loc.CalID) {
			skips.add("read-only")
			continue
		}
		if t.isTodo {
			absorbed := false
			for p := parentOf[t.uid]; p != ""; p = parentOf[p] {
				if selected[p] {
					absorbed = true
					break
				}
			}
			if absorbed {
				continue // travels with its selected ancestor's subtree
			}
		}
		roots = append(roots, t)
	}
	return roots, skips
}

// bulkDelete (d in SELECT) deletes every selected item — tasks with their whole
// subtrees — after one confirm naming the full count. All-or-nothing with
// rollback; one undo step restores everything (item deletes stay undoable, so
// the ordinary confirm is correct — unlike collection deletes).
func (a *app) bulkDelete() {
	targets := a.selRange()
	if targets == nil {
		a.exitSelect()
		a.flash("Selection no longer valid")
		return
	}
	roots, skips := a.bulkDeleteRoots(targets)
	if len(roots) == 0 {
		a.flash(bulkSummary("deleted", 0, skips))
		return
	}
	// Expand each task root with its descendants, deduped across roots.
	var uids []string
	seen := map[string]bool{}
	for _, r := range roots {
		for _, u := range append([]string{r.uid}, a.descendants(r.uid)...) {
			if !seen[u] {
				seen[u] = true
				uids = append(uids, u)
			}
		}
	}
	prompt := fmt.Sprintf("Delete %d item(s)?", len(roots))
	if len(uids) > len(roots) {
		prompt = fmt.Sprintf("Delete %d item(s) (%d with subtasks)?", len(roots), len(uids))
	}
	a.confirm(" Delete selection ", prompt, func() {
		ctx := context.Background()
		var ops []undoOp
		var rollback []func()
		deleted := 0
		for _, u := range uids {
			loc, ok := a.store.Locate(u)
			if !ok {
				continue
			}
			if err := a.store.Delete(ctx, loc.CalID, loc.Name); err != nil {
				for i := len(rollback) - 1; i >= 0; i-- {
					rollback[i]()
				}
				a.exitSelect()
				a.refresh("")
				a.flash("Delete failed: " + err.Error())
				return
			}
			calID, name, prev := loc.CalID, loc.Name, loc.Prev
			rollback = append(rollback, func() { _, _ = a.store.Restore(ctx, calID, name, prev) })
			ops = append(ops, undoOp{calID: calID, name: name, prev: prev})
			deleted++
		}
		a.pushUndo("bulk delete", "", ops...)
		a.exitSelect()
		a.refresh("")
		a.flash(bulkSummary("deleted", deleted, skips) + " (u to undo)")
	})
}
```

Wire in `selection.go` (replace the `'d'` stub):

```go
		case r == 'd':
			a.bulkDelete()
			return nil
```

Note: like the single-item `deleteWholeObject`, this deletes whole resources per UID (a recurring *todo*'s resource is its series — the "natural meaning" per the spec). Mirror that path's semantics exactly; do not add scope pickers.

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/ui/ -run 'TestBulkDelete|TestBulk' -v` then `go test ./internal/ui/`.
Expected: PASS.

- [ ] **Step 5: Full gate, log entry, commit**

```bash
git add -A && git commit -m "feat: SELECT bulk delete — ancestor dedupe, one confirm with counts, rollback"
```

---

### Task 6: Multi-item yank/paste (y/Y, p/P)

**Files:**
- Modify: `internal/ui/yankpaste.go` (clipboard → `yankUIDs []string`; shared-rollback cores)
- Modify: `internal/ui/app.go` (field rename ~line 111)
- Modify: `internal/ui/bulkops.go` (`bulkYank`)
- Modify: `internal/ui/selection.go` (wire `'y'`/`'Y'`)
- Modify: `internal/ui/yankpaste_test.go` (adapt to the slice clipboard)
- Modify: `internal/ui/bulkops_test.go`

**Interfaces:**
- Consumes: `bulkDeleteRoots` (Task 5) for the tree-only root set; existing `paste`/`reparentTo`/`moveSubtree`/`copySubtree`; `model.SetTodoParent/IsolateComponent/RemoveComponent/CopyTodo/NewUID`.
- Produces: app field `yankUIDs []string` (replaces `yankUID string`; `yankCut bool` unchanged); `a.bulkYank(cut bool)`; refactored `moveSubtreeOps(uid, targetParent, srcCal, dstCal string, ops *[]undoOp, rollback *[]func()) error` and `copySubtreeOps(rootUID, targetParent, dstCal string, ops *[]undoOp, rollback *[]func()) error` — the existing single-item wrappers and the multi-paste both call these cores.

- [ ] **Step 1: Write the failing tests** (append to `bulkops_test.go`)

```go
// TestBulkYankPasteUnder: cut a range of siblings, paste under another task —
// all roots move (order preserved), one undo step, clipboard persists.
func TestBulkYankPasteUnder(t *testing.T) {
	now := time.Date(2026, 7, 5, 9, 0, 0, 0, time.UTC)
	a := newRootedTestApp(t, now)
	a.setMode(modeTasks)
	target := putTodo(t, a, testCalID(a), "", "a target", now, true)
	u1 := putTodo(t, a, testCalID(a), "", "b move1", now, true)
	u2 := putTodo(t, a, testCalID(a), "", "c move2", now, true)
	a.refresh(target)
	a.setFocus(a.tree)

	a.selectTreeByUID(u1)
	a.enterSelect()
	a.selectTreeByUID(u2)
	a.bulkYank(true) // cut
	if a.selecting {
		t.Fatal("yank must exit SELECT")
	}
	if len(a.yankUIDs) != 2 {
		t.Fatalf("clipboard = %v, want two roots", a.yankUIDs)
	}

	a.selectTreeByUID(target)
	undoBefore := len(a.undo)
	a.pasteUnderSelection()
	for _, u := range []string{u1, u2} {
		loc, _ := a.store.Locate(u)
		if td := findTodo(loc.Object, u); td == nil || td.ParentUID != target {
			t.Fatalf("%s must be re-parented under the target", u)
		}
	}
	if len(a.undo) != undoBefore+1 {
		t.Fatal("multi-paste must be one undo step")
	}
	if len(a.yankUIDs) != 2 {
		t.Fatal("clipboard must persist after paste")
	}
}

// TestBulkYankDedupesSubtree: yanking a parent and its child cuts one root —
// the child travels inside the parent's subtree, not as a second root.
func TestBulkYankDedupesSubtree(t *testing.T) {
	now := time.Date(2026, 7, 5, 9, 0, 0, 0, time.UTC)
	a := newRootedTestApp(t, now)
	a.setMode(modeTasks)
	parent := putTodo(t, a, testCalID(a), "", "folder", now, true)
	putTodo(t, a, testCalID(a), parent, "leaf", now, true)
	a.refresh(parent)
	a.setFocus(a.tree)
	selectTreeRangeAll(t, a)
	a.bulkYank(false) // copy
	if len(a.yankUIDs) != 1 || a.yankUIDs[0] != parent {
		t.Fatalf("clipboard roots = %v, want just the parent", a.yankUIDs)
	}
}

// TestBulkYankTreeOnly: y in a calendar-days range flashes and keeps SELECT.
func TestBulkYankTreeOnly(t *testing.T) {
	now := time.Date(2026, 7, 5, 9, 0, 0, 0, time.UTC)
	a := newRootedTestApp(t, now)
	a.setMode(modeCalendar)
	a.refresh("")
	a.enterSelect()
	a.bulkYank(true)
	if !a.selecting || len(a.yankUIDs) != 0 {
		t.Fatal("yank outside the tree must flash and keep SELECT with an empty clipboard")
	}
}

// TestBulkPasteCycleGuard: pasting a cut range onto one of its own roots (or
// into a root's subtree) is refused before any write.
func TestBulkPasteCycleGuard(t *testing.T) {
	now := time.Date(2026, 7, 5, 9, 0, 0, 0, time.UTC)
	a := newRootedTestApp(t, now)
	a.setMode(modeTasks)
	u1 := putTodo(t, a, testCalID(a), "", "a root1", now, true)
	u2 := putTodo(t, a, testCalID(a), "", "b root2", now, true)
	a.refresh(u1)
	a.setFocus(a.tree)
	a.selectTreeByUID(u1)
	a.enterSelect()
	a.selectTreeByUID(u2)
	a.bulkYank(true)

	a.selectTreeByUID(u2) // paste target = one of the cut roots
	a.pasteUnderSelection()
	loc, _ := a.store.Locate(u1)
	if td := findTodo(loc.Object, u1); td == nil || td.ParentUID != "" {
		t.Fatal("cycle-guarded paste must not move anything")
	}
}
```

Existing tests in `yankpaste_test.go` referencing `a.yankUID` must be updated to `a.yankUIDs` (single-item cases become one-element slices) — update assertions mechanically, do not weaken them.

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/ui/ -run 'TestBulkYank|TestBulkPaste' -v`
Expected: FAIL — `undefined: a.bulkYank`, `a.yankUIDs`.

- [ ] **Step 3: Implement**

`app.go`: replace `yankUID string` with `yankUIDs []string` (update the field comment: "task roots on the yank/copy clipboard, in visible order; nil when empty").

`yankpaste.go` mechanical updates:
- `setClip` sets `a.yankUIDs = []string{t.uid}`.
- `paste`: `if len(a.yankUIDs) == 0 { flash; return }`; the src-Locate existence check and the cycle guards run **per root before any write**:

```go
	// Validate every root up front — a cycle or vanished root refuses the whole
	// paste before any write, so a multi-root paste can't half-apply.
	for _, root := range a.yankUIDs {
		if _, ok := a.store.Locate(root); !ok {
			a.flash("A clipboard task no longer exists")
			a.yankUIDs = nil
			return
		}
		if a.yankCut {
			if targetParent == root {
				a.flash("Can't paste a task onto itself")
				return
			}
			for _, d := range a.descendants(root) {
				if d == targetParent {
					a.flash("Can't paste a task into its own subtree")
					return
				}
			}
		}
	}
```

- Extract the loop bodies of `moveSubtree` and `copySubtree` into `moveSubtreeOps` / `copySubtreeOps` taking `ops *[]undoOp, rollback *[]func()` and returning `error` (the existing `fail` closure moves to the callers). The single-item public behavior is unchanged; the multi-root paste calls the cores for each root sharing ONE ops/rollback pair, so a failure on root 3 rolls back roots 1–2 too, and success pushes one `pushUndo("move tasks", ...)`/`("copy tasks", ...)` step.
- Same-list cut paste (the `reparentTo` path) becomes a multi-root loop with `PutIfUnchanged` per root (the Locate'd `Prev` — bulk writes follow the version-check guardrail even though the legacy single-item path used `Put`), shared ops/rollback, one undo step.

`bulkops.go`:

```go
// bulkYank (y/Y in SELECT) puts the selected task roots on the clipboard —
// tree context only: the clipboard is a task-subtree concept, and paste needs
// a tree target. Roots are the ancestor-deduped range in visible order; each
// root's subtree travels with it on paste, exactly like a single-item yank.
func (a *app) bulkYank(cut bool) {
	if a.selContext() != selTree {
		a.flash("Yank works in the task tree (t)")
		return
	}
	targets := a.selRange()
	if targets == nil {
		a.exitSelect()
		a.flash("Selection no longer valid")
		return
	}
	roots, skips := a.bulkDeleteRoots(targets)
	if len(roots) == 0 {
		a.flash(bulkSummary("on clipboard", 0, skips))
		return
	}
	a.yankUIDs = a.yankUIDs[:0]
	for _, r := range roots {
		a.yankUIDs = append(a.yankUIDs, r.uid)
	}
	a.yankCut = cut
	verb := "Copied"
	if cut {
		verb = "Cut"
	}
	a.exitSelect()
	a.flash(fmt.Sprintf("%s %d task(s) — p paste under · P paste at top", verb, len(a.yankUIDs)))
}
```

Wire in `selection.go` (replace the `'y'`/`'Y'` stub):

```go
		case r == 'y':
			a.bulkYank(true)
			return nil
		case r == 'Y':
			a.bulkYank(false)
			return nil
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/ui/ -run 'TestBulkYank|TestBulkPaste|TestYank|TestPaste|TestMove|TestCopy' -v` then `go test ./internal/ui/`.
Expected: PASS, including the adapted legacy yank/paste suite.

- [ ] **Step 5: Full gate, log entry, commit**

```bash
git add -A && git commit -m "feat: multi-item clipboard — SELECT bulk yank, all-or-nothing multi-root paste"
```

---

### Task 7: Bulk grab (m) — uniform date-shift

**Files:**
- Create: `internal/ui/bulkgrab.go`
- Create: `internal/ui/bulkgrab_test.go`
- Modify: `internal/ui/app.go` (state fields; grab routing in `globalKeys` ~line 717)
- Modify: `internal/ui/selection.go` (wire `'m'`)

**Interfaces:**
- Consumes: `selRange`, `draftFromTodo` (`quickfield.go:18`), `draftFromEvent` (`grab.go:466`), `model.EditTodo/EditEvent`, `store.Locate/PutIfUnchanged/Restore`, `calReadOnly`, `pushUndo`, `refreshKeepingDrill`.
- Produces: app fields `bulkGrab []bulkGrabItem`, `bulkGrabMoved bool`; `a.startBulkGrab()`, `a.handleBulkGrabKey(*tcell.EventKey) *tcell.EventKey`, `a.bulkGrabShift(days int)`, `a.commitBulkGrab()`, `a.cancelBulkGrab()`, `a.endBulkGrab()`.

- [ ] **Step 1: Write the failing tests** (`internal/ui/bulkgrab_test.go`)

```go
package ui

import (
	"testing"
	"time"

	"github.com/gdamore/tcell/v2"

	"github.com/littekge/LazyPlanner/internal/model"
)

// TestBulkGrabShiftsMixed: a drilled-day range holding an event and a due task
// shifts both by whole days (h/l) and weeks (j/k) — times of day untouched.
func TestBulkGrabShiftsMixed(t *testing.T) {
	now := time.Date(2026, 7, 6, 9, 30, 0, 0, time.UTC)
	a := newRootedTestApp(t, now)
	a.setMode(modeCalendar)
	putEvent(t, a, testCalID(a), "meeting", now, false)
	task := putTodo(t, a, testCalID(a), "", "due today", now, true)
	a.refresh("")
	a.month.reDrill(model.DayStart(now), 0)
	a.enterSelect()
	a.month.eventIndex = len(a.dayItems(model.DayStart(now))) - 1
	a.startBulkGrab()
	if !a.grabbing || len(a.bulkGrab) != 2 {
		t.Fatalf("grabbing=%v n=%d, want true/2", a.grabbing, len(a.bulkGrab))
	}
	if m := a.interactionMode(); m != modeGrab {
		t.Fatalf("badge = %q, want GRAB (innermost)", m)
	}

	a.handleBulkGrabKey(tcell.NewEventKey(tcell.KeyRune, 'l', tcell.ModNone)) // +1 day
	a.handleBulkGrabKey(tcell.NewEventKey(tcell.KeyRune, 'j', tcell.ModNone)) // +1 week
	a.handleBulkGrabKey(tcell.NewEventKey(tcell.KeyEnter, 0, tcell.ModNone))  // keep

	if a.grabbing || a.selecting {
		t.Fatal("Enter must exit both GRAB and SELECT")
	}
	loc, _ := a.store.Locate(task)
	td := findTodo(loc.Object, task)
	want := now.AddDate(0, 0, 8)
	if td == nil || !td.Due.Equal(want) {
		t.Fatalf("task due = %v, want %v (+8 days, time preserved)", td.Due, want)
	}
}

// TestBulkGrabEscRevertsToSelect: Esc restores every pre-grab snapshot and
// returns to SELECT with the range intact (retry-friendly), no undo step.
func TestBulkGrabEscRevertsToSelect(t *testing.T) {
	now := time.Date(2026, 7, 6, 9, 0, 0, 0, time.UTC)
	a := newRootedTestApp(t, now)
	a.setMode(modeCalendar)
	putEvent(t, a, testCalID(a), "meeting", now, false)
	task := putTodo(t, a, testCalID(a), "", "due today", now, true)
	a.refresh("")
	a.month.reDrill(model.DayStart(now), 0)
	a.enterSelect()
	a.month.eventIndex = len(a.dayItems(model.DayStart(now))) - 1
	undoBefore := len(a.undo)
	a.startBulkGrab()
	a.handleBulkGrabKey(tcell.NewEventKey(tcell.KeyRune, 'l', tcell.ModNone))
	a.handleBulkGrabKey(tcell.NewEventKey(tcell.KeyEscape, 0, tcell.ModNone))

	if a.grabbing {
		t.Fatal("Esc must end the grab")
	}
	if !a.selecting {
		t.Fatal("Esc must return to SELECT (one nesting level)")
	}
	loc, _ := a.store.Locate(task)
	if td := findTodo(loc.Object, task); td == nil || !td.Due.Equal(now) {
		t.Fatal("Esc must restore the pre-grab due date")
	}
	if len(a.undo) != undoBefore {
		t.Fatal("a cancelled grab pushes no undo step")
	}
}

// TestBulkGrabFilters: recurring events and undated tasks never enter the
// grab set; if nothing is grabbable SELECT stays active with a flash.
func TestBulkGrabFilters(t *testing.T) {
	now := time.Date(2026, 7, 6, 9, 0, 0, 0, time.UTC)
	a := newRootedTestApp(t, now)
	a.setMode(modeCalendar)
	putRecurringEvent(t, a, testCalID(a), "weekly", now)
	a.refresh("")
	a.month.reDrill(model.DayStart(now), 0)
	a.enterSelect()
	a.startBulkGrab()
	if a.grabbing {
		t.Fatal("a range of only-recurring events must not start a grab")
	}
	if !a.selecting {
		t.Fatal("SELECT must stay active when nothing is grabbable")
	}
}
```

Also assert (in `TestBulkGrabShiftsMixed` or a dedicated case) that the committed grab is exactly one undo step and `undoLast` restores both items.

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/ui/ -run 'TestBulkGrab' -v`
Expected: FAIL — `undefined: a.startBulkGrab`.

- [ ] **Step 3: Implement**

`app.go` fields (after the grab block ~line 132):

```go
	// Bulk grab (m in SELECT): a uniform date-shift over the selected items —
	// single-item grab's per-type keys (events ±hour, tasks ±day) are incoherent
	// over a mixed set, so bulk h/l = ±1 day and j/k = ±1 week for everything,
	// times of day untouched. Non-empty bulkGrab routes grab keys to the bulk
	// handler; bulkGrabMoved gates the undo push (Enter with no nudge = no-op).
	bulkGrab      []bulkGrabItem
	bulkGrabMoved bool
```

Route in `globalKeys` — replace the grab branch:

```go
	if a.grabbing {
		if len(a.bulkGrab) > 0 {
			return a.handleBulkGrabKey(ev)
		}
		return a.handleGrabKey(ev)
	}
```

`internal/ui/bulkgrab.go` (new):

```go
package ui

import (
	"context"
	"errors"
	"fmt"

	"github.com/gdamore/tcell/v2"

	"github.com/littekge/LazyPlanner/internal/model"
	"github.com/littekge/LazyPlanner/internal/store"
)

// Bulk grab is SELECT's temporal layer: one date-shift applied to every
// selected item. It reuses grab's contract — modal keys, per-nudge
// version-checked commits, Enter keeps (one undo step), Esc restores the
// pre-grab snapshots — but returns to SELECT on Esc so the range can retry.

// bulkGrabItem is one grabbed item's identity plus its pre-grab snapshot.
type bulkGrabItem struct {
	uid    string
	calID  string
	name   string
	prev   *store.Resource
	isTodo bool
}

// startBulkGrab (m in SELECT) filters the range to date-shiftable items:
// recurring events are skipped (scope ambiguity — the settled SELECT rule),
// undated tasks have nothing to shift, read-only calendars never take writes.
// Recurring todos participate: shifting the due moves the series anchor,
// exactly like a single-item grab of a recurring todo.
func (a *app) startBulkGrab() {
	targets := a.selRange()
	if targets == nil {
		a.exitSelect()
		a.flash("Selection no longer valid")
		return
	}
	skips := bulkSkip{}
	var items []bulkGrabItem
	for _, t := range targets {
		if !t.isTodo && t.recurring {
			skips.add("recurring")
			continue
		}
		loc, ok := a.store.Locate(t.uid)
		if !ok {
			skips.add("missing")
			continue
		}
		if a.calReadOnly(loc.CalID) {
			skips.add("read-only")
			continue
		}
		if t.isTodo {
			if td := findTodo(loc.Object, t.uid); td == nil || !td.HasDue {
				skips.add("undated")
				continue
			}
		}
		items = append(items, bulkGrabItem{uid: t.uid, calID: loc.CalID, name: loc.Name, prev: loc.Prev, isTodo: t.isTodo})
	}
	if len(items) == 0 {
		a.flash(bulkSummary("grabbable", 0, skips))
		return
	}
	a.bulkGrab = items
	a.bulkGrabMoved = false
	a.grabbing = true
	a.flash(fmt.Sprintf("GRAB ×%d · h/l ±day · j/k ±week · Enter keep · Esc cancel", len(items)))
}

// handleBulkGrabKey consumes every key while a bulk grab is active, mirroring
// handleGrabKey's modality.
func (a *app) handleBulkGrabKey(ev *tcell.EventKey) *tcell.EventKey {
	switch ev.Key() {
	case tcell.KeyEnter:
		a.commitBulkGrab()
	case tcell.KeyEscape:
		a.cancelBulkGrab()
	case tcell.KeyLeft:
		a.bulkGrabShift(-1)
	case tcell.KeyRight:
		a.bulkGrabShift(1)
	case tcell.KeyDown:
		a.bulkGrabShift(7)
	case tcell.KeyUp:
		a.bulkGrabShift(-7)
	case tcell.KeyRune:
		switch ev.Rune() {
		case 'h':
			a.bulkGrabShift(-1)
		case 'l':
			a.bulkGrabShift(1)
		case 'j':
			a.bulkGrabShift(7)
		case 'k':
			a.bulkGrabShift(-7)
		case 'J', 'K':
			a.flash("Resize doesn't apply to a multi-selection")
		}
	}
	return nil
}

// bulkGrabShift applies one ±days nudge to every grabbed item, committing each
// version-checked. A mid-nudge failure or stale item reverts THIS nudge's
// partial writes (our own writes an instant ago) and — for stale — ends the
// grab keeping earlier completed nudges, mirroring abortGrabStale's rule of
// never force-restoring over a server change.
func (a *app) bulkGrabShift(days int) {
	ctx := context.Background()
	var nudged []func()
	revertNudge := func() {
		for i := len(nudged) - 1; i >= 0; i-- {
			nudged[i]()
		}
	}
	for _, it := range a.bulkGrab {
		loc, ok := a.store.Locate(it.uid)
		if !ok {
			revertNudge()
			a.abortBulkGrabStale(it.uid)
			return
		}
		var newObj *model.Parsed
		var err error
		if it.isTodo {
			td := findTodo(loc.Object, it.uid)
			if td == nil || !td.HasDue {
				revertNudge()
				a.abortBulkGrabStale(it.uid)
				return
			}
			d := draftFromTodo(td)
			d.Due = d.Due.AddDate(0, 0, days)
			newObj, err = model.EditTodo(loc.Object, it.uid, d, a.now, a.loc)
		} else {
			ev := findEvent(loc.Object, it.uid)
			if ev == nil {
				revertNudge()
				a.abortBulkGrabStale(it.uid)
				return
			}
			d := draftFromEvent(ev)
			d.Start = d.Start.AddDate(0, 0, days)
			if !d.End.IsZero() {
				d.End = d.End.AddDate(0, 0, days)
			}
			newObj, err = model.EditEvent(loc.Object, it.uid, d, a.now, a.loc)
		}
		if err != nil {
			revertNudge()
			a.flashErr("Grab", err)
			return
		}
		applied, err := a.store.PutIfUnchanged(ctx, it.calID, it.name, newObj, loc.Prev)
		if err != nil {
			revertNudge()
			a.flash("Grab failed: " + err.Error())
			return
		}
		if !applied {
			revertNudge()
			a.abortBulkGrabStale(it.uid)
			return
		}
		calID, name, prev := it.calID, it.name, loc.Prev
		nudged = append(nudged, func() { _, _ = a.store.Restore(ctx, calID, name, prev) })
	}
	a.bulkGrabMoved = true
	a.refreshKeepingDrill("")
	a.flash(fmt.Sprintf("Shifted %d item(s) %+d day(s) · Enter keep · Esc cancel", len(a.bulkGrab), days))
}

// abortBulkGrabStale ends the grab when an item changed underneath (a sync
// pull): earlier completed nudges are kept — undoable as one step — and the
// stale item is left at the server's version (never force-restored).
func (a *app) abortBulkGrabStale(staleUID string) {
	if a.bulkGrabMoved {
		var ops []undoOp
		for _, it := range a.bulkGrab {
			if it.uid == staleUID {
				continue
			}
			ops = append(ops, undoOp{calID: it.calID, name: it.name, prev: it.prev})
		}
		a.pushUndo("bulk grab", "", ops...)
	}
	a.endBulkGrab()
	a.exitSelect()
	a.refresh("")
	a.flash("An item changed on the server — grab ended (moves so far kept)")
}

// commitBulkGrab keeps the shifts as one undo step and exits both GRAB and
// SELECT (the action completes the selection, vim-style).
func (a *app) commitBulkGrab() {
	if a.bulkGrabMoved {
		var ops []undoOp
		for _, it := range a.bulkGrab {
			ops = append(ops, undoOp{calID: it.calID, name: it.name, prev: it.prev})
		}
		a.pushUndo("bulk grab", "", ops...)
	}
	n := len(a.bulkGrab)
	moved := a.bulkGrabMoved
	a.endBulkGrab()
	a.exitSelect()
	a.refreshKeepingDrill("")
	if moved {
		a.flash(fmt.Sprintf("Rescheduled %d item(s) (u to undo)", n))
	} else {
		a.flash("Nothing moved")
	}
}

// cancelBulkGrab restores every pre-grab snapshot (newest-first) and returns
// to SELECT with the range intact so the user can retry. Revert failures are
// surfaced, never reported as a clean cancel (the cancelGrab rule).
func (a *app) cancelBulkGrab() {
	ctx := context.Background()
	var revertErr error
	for i := len(a.bulkGrab) - 1; i >= 0; i-- {
		it := a.bulkGrab[i]
		if _, err := a.store.Restore(ctx, it.calID, it.name, it.prev); err != nil {
			revertErr = errors.Join(revertErr, err)
		}
	}
	a.endBulkGrab()
	a.refreshKeepingDrill("")
	a.syncSelectionVisuals()
	if revertErr != nil {
		a.flashErr("Grab cancel", revertErr)
		return
	}
	a.flash("Grab cancelled — still selecting")
}

func (a *app) endBulkGrab() {
	a.grabbing = false
	a.bulkGrab = nil
	a.bulkGrabMoved = false
}
```

Wire in `selection.go` (replace the `'m'` stub):

```go
		case r == 'm':
			a.startBulkGrab()
			return nil
```

Note: `cancelBulkGrab` restores to *pre-grab* snapshots even after several nudges (each item's `prev` is captured once at `startBulkGrab`) — that is the intended semantics (Esc = revert the whole grab). Only non-recurring events are in the set, so no `ReanchoredRecurrence` handling is needed; recurring todos mirror the single-grab due-shift exactly.

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/ui/ -run 'TestBulkGrab|TestGrab' -v` then `go test ./internal/ui/` and `go test -race ./internal/ui/`.
Expected: PASS (the existing single-grab suite must be untouched).

- [ ] **Step 5: Full gate, log entry, commit**

```bash
git add -A && git commit -m "feat: SELECT bulk grab — uniform date-shift, Esc back to SELECT, one undo step"
```

---

### Task 8: Docs ripple — README, :help, main.md, coverage ledger

**Files:**
- Modify: `README.md` (keybindings table: `V` row; delete/space/y rows gain a "in SELECT" clause only if the table idiom fits — prefer one new `V` row + one short Usage sentence)
- Modify: `internal/ui/help.go` (`?` overlay: SELECT section)
- Modify: `main.md` (keybinding table `V` row; a short "SELECT mode" design paragraph in the UI Design section near Grab mode; the `### v1.4.0` Build Plan subsection rewritten as the build record — per the owner: log-summary style, not a plan)
- Modify: `docs/audit/COVERAGE.md` (new `ui/selection + bulk ops` surface row, marked never-audited)
- Modify: `log.md`

**Interfaces:** none — documentation only. Verify against the shipped behavior (read `selection.go`/`bulkops.go`/`bulkgrab.go` as built, not this plan).

- [ ] **Step 1: README** — add to the keybindings table after the `m` row:

```markdown
| `V` | SELECT mode: extend a contiguous selection with the movement keys (task tree, calendar days, or a drilled day's items), then `Space` complete all, `d` delete all, `y`/`Y` cut/copy all (tree), `m` grab all (±day/±week). `Esc` cancels |
```

and one concept sentence in Usage prose (the pane model paragraph area): SELECT is a mode like GRAB — the badge shows `SELECT`, movement extends the range, one action applies to everything selected and is one `u` undo step. Follow the README rules: the table row carries the keys; prose carries only the concept.

- [ ] **Step 2: `:help`** — add a "Select (multi-select)" section to `internal/ui/help.go` mirroring the existing Grab section's format: `V` enter · move to extend · `gg`/`G` ends · `Space`/`d`/`y`/`Y`/`m` bulk actions · recurring events are skipped · `Esc` cancels (from a grab, back to SELECT).

- [ ] **Step 3: main.md** — three edits, all in place:
  1. Keybinding table: the same `V` row (condensed to the table's voice).
  2. UI Design: a short **SELECT mode** paragraph after the Grab mode section — contiguous anchor→cursor range, the three contexts, derived-range architecture (one sentence), bulk-op semantics (skip counts, one undo step, all-or-nothing), bulk grab = uniform date-shift, mode nesting DRILL→SELECT→GRAB.
  3. `### v1.4.0 — SELECT mode` Build Plan subsection: flip from "(planned)" to the build record — status line (implemented, gates green), the settled decisions in two or three sentences, and a build-steps list summarizing what each task delivered (log-summary style per the owner's direction; the detailed record stays in `log.md`, the full spec in `docs/superpowers/specs/2026-07-23-select-mode-design.md`).

- [ ] **Step 4: COVERAGE.md** — add the new surface row (`internal/ui` selection/bulk-ops/bulk-grab) to the ledger marked never-audited, so the next hardening pass targets it.

- [ ] **Step 5: Verify docs against behavior** — run the app headlessly-built (`go build ./...`) and cross-check every claim: key list matches `handleSelectKey`, skip reasons match `bulkops.go`, the grab keys match `handleBulkGrabKey`. Fix any drift in the docs, not the code.

- [ ] **Step 6: Full gate, log entry, commit**

```bash
git add -A && git commit -m "docs: v1.4.0 SELECT mode — README/:help/main.md build record, coverage ledger row"
```

---

## Self-Review (run after writing, fixed inline)

1. **Spec coverage:** enter/exit/badge/nesting → T1; key layer incl. prefix/mouse gates → T1; derived range + anchor-vanish + maxSelectDays → T2; visuals + displaystress + legibility → T3; bulk complete (children-first, recurring-advance, skips, rollback, one undo) → T4; bulk delete (dedupe, confirm counts, rollback) → T5; multi-clipboard yank/paste (tree-only, cycle guard, all-or-nothing) → T6; bulk grab (uniform shift, Esc→SELECT, stale rule) → T7; docs + `N selected` status + hints (T1/T2) + coverage ledger → T8. `-race` run → T7 step 4.
2. **Type consistency:** `editTarget` fields (`uid/isTodo/occStart/allDay/recurring`) used consistently; `bulkSkip`/`bulkSummary` defined T4, consumed T5–T7; `bulkDeleteRoots` defined T5, consumed T6; view fields `selDayAnchor/selAnchorUID/selAnchorOcc` identical on both grids (T3); `yankUIDs []string` renamed once (T6).
3. **Known judgment calls encoded:** reverse-order processing in bulkComplete; whole-resource delete mirroring `deleteWholeObject`; stale-abort keeps landed nudges with an undo step; `PutIfUnchanged` for the multi-reparent (stricter than the legacy single-item `Put`).
