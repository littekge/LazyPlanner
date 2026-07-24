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

// syncSelectionVisuals refreshes everything that displays the selection and
// validates the anchor: if the range can no longer be derived (the anchor was
// deleted remotely, the drilled day's items changed), SELECT exits with a
// flash rather than acting on a guess. Event-driven only — never a draw path;
// the grids only need the plain anchor fields pushed in (they derive the other
// end of the range from their own selected/eventIndex at draw time), so
// per-move sync work here is limited to the tree restyle and the status count.
//
// The clearing flash is deliberately the LAST write to statusLeft: flash and
// updateStatus both write that same TextView synchronously, so flashing before
// the trailing updateStatus() would have the ordinary status text overwrite it
// in the same call — the user would never see it.
func (a *app) syncSelectionVisuals() {
	cleared := false
	if a.selecting && a.selRange() == nil {
		a.selecting = false
		a.selAnchorUID = ""
		a.selAnchorOcc = time.Time{}
		a.selAnchorDay = time.Time{}
		cleared = true
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
	if cleared {
		a.flash("Selection cleared — the items changed")
	}
}

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
// event spanning several selected days is deduped to one target. Unlike a tree
// UID or a drilled item, a date anchor can never "vanish", so an interval that
// simply holds no items is a valid empty selection — the caller must be able
// to tell that apart from "anchor unresolvable" (nil), or V on an empty day
// would exit SELECT before the user can extend the range onto a day that does
// have items.
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
	out := []editTarget{}
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
		if ev.Modifiers() == tcell.ModNone {
			return ev
		}
		// A modified arrow (Ctrl-Left/Right resizes the left column) is not
		// motion — falling through would leak a layout mutation past SELECT's
		// swallow-everything guarantee, so it's swallowed like everything else.
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
			a.bulkComplete()
			return nil
		case r == 'd':
			a.bulkDelete()
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
