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
