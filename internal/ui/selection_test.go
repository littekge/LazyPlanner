package ui

import (
	"testing"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"

	"github.com/littekge/LazyPlanner/internal/model"
)

// testCalID returns the calendar id to seed test fixtures into. The fixture's
// "personal" calendar (selected by default in both overview lists) accepts
// both VEVENT and VTODO, so one id works for putTodo and putEvent alike.
func testCalID(a *app) string { return a.selectedCalendarID() }

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

// TestSelectSwallowsModifiedArrows: a modified arrow (Ctrl-Left/Right resizes
// the left column outside SELECT) is not motion — it must be swallowed like
// every other non-motion key, not leaked through to the pre-existing Ctrl-gated
// resize handlers in globalKeys.
func TestSelectSwallowsModifiedArrows(t *testing.T) {
	now := time.Date(2026, 7, 5, 9, 0, 0, 0, time.UTC)
	a := newRootedTestApp(t, now)
	a.setMode(modeTasks)
	a.setFocus(a.tree)
	a.enterSelect()

	before := a.leftWidth
	a.globalKeys(tcell.NewEventKey(tcell.KeyLeft, 0, tcell.ModCtrl))
	if a.leftWidth != before {
		t.Fatalf("Ctrl-Left while selecting must not resize: leftWidth %d -> %d", before, a.leftWidth)
	}
	a.globalKeys(tcell.NewEventKey(tcell.KeyRight, 0, tcell.ModCtrl))
	if a.leftWidth != before {
		t.Fatalf("Ctrl-Right while selecting must not resize: leftWidth %d -> %d", before, a.leftWidth)
	}
	if !a.selecting {
		t.Fatal("SELECT must still be active after the swallowed modified arrows")
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
