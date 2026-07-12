package ui

import (
	"context"
	"time"

	"github.com/gdamore/tcell/v2"

	"github.com/littekge/LazyPlanner/internal/model"
	"github.com/littekge/LazyPlanner/internal/store"
)

// Grab mode (m) is the temporal-manipulation layer, unified across the tree,
// calendar drill, and agenda: move an event's day/hour and resize it, or nudge a
// task's due date. It's modal — hjkl edit the grabbed item, Enter keeps, Esc
// reverts to the pre-grab snapshot. Edits commit to the store on each nudge so the
// views update live; the whole grab is one undo step. Structural moves (parent/
// list) stay on yank/paste — grab only touches "when".

const grabHourStep = time.Hour

// startGrab enters grab mode on the current target. Events and dated tasks are
// grabbable; an undated task is skipped with a hint. A recurring event first
// prompts for scope (this occurrence / all) via the picker.
func (a *app) startGrab() {
	t, ok := a.currentTarget()
	if !ok {
		a.flash("Nothing selected to grab (m)")
		return
	}
	loc, ok := a.store.Locate(t.uid)
	if !ok {
		a.flash("Item not found")
		return
	}
	if !a.guardWrite(loc.CalID) {
		return
	}
	if t.isTodo {
		if td := findTodo(loc.Object, t.uid); td == nil || !td.HasDue {
			a.flash("No due date to move (set one with sd)")
			return
		}
		// A recurring todo's due nudge moves the series anchor (scope all); no picker.
		a.beginGrab(loc, t, scopeAll)
		return
	}
	ev := findEvent(loc.Object, t.uid)
	if ev == nil {
		a.flash("Event not found")
		return
	}
	if ev.Recurring {
		// This / All only — a this-and-future *move* would split into a second
		// resource, which grab's single-snapshot revert can't cleanly undo; use the
		// edit form's "This & future" for that.
		a.pickRecurrenceScope("event", false, func(s recurScope) {
			a.beginGrab(loc, t, s)
		})
		return
	}
	a.beginGrab(loc, t, scopeAll)
}

// beginGrab enters grab mode on the resolved target at the given recurrence scope.
func (a *app) beginGrab(loc store.Located, t editTarget, scope recurScope) {
	a.grabbing = true
	a.grabUID = t.uid
	a.grabIsEvent = !t.isTodo
	a.grabCalID, a.grabName = loc.CalID, loc.Name
	a.grabPrev = loc.Prev
	a.grabScope = scope
	a.grabOccStart = t.occStart
	a.grabAllDay = t.allDay
	a.flash(a.grabStatus())
}

// grabStatus describes the current grab controls for the flash line.
func (a *app) grabStatus() string {
	switch {
	case !a.grabIsEvent:
		return "GRAB due · j/k ±day · h/l ±week · Enter keep · Esc cancel"
	case a.mode == modeCalendar && a.viewMode != viewMonth:
		return "GRAB event · j/k ±hour · h/l ±day · J/K resize · Enter keep · Esc cancel"
	default:
		return "GRAB event · h/l ±day · Enter keep · Esc cancel"
	}
}

// grabTimeHint explains how to reach the week/day time-grid to do action (change
// the time / resize), which only works on a timed event there. In calendar mode
// `v` cycles to week/day; in agenda mode `v` is a no-op, so name the destination
// rather than a dead key.
func (a *app) grabTimeHint(action string) string {
	if a.mode == modeCalendar {
		return "switch to week/day view (v) to " + action
	}
	return "open the week/day calendar view to " + action
}

// handleGrabKey processes a key while grab mode is active; every key is consumed
// so nothing leaks to the views.
func (a *app) handleGrabKey(ev *tcell.EventKey) *tcell.EventKey {
	switch ev.Key() {
	case tcell.KeyEnter:
		a.commitGrab()
	case tcell.KeyEscape:
		a.cancelGrab()
	case tcell.KeyLeft:
		a.grabNudge('h')
	case tcell.KeyRight:
		a.grabNudge('l')
	case tcell.KeyDown:
		a.grabNudge('j')
	case tcell.KeyUp:
		a.grabNudge('k')
	case tcell.KeyRune:
		switch ev.Rune() {
		case 'h', 'l', 'j', 'k', 'J', 'K':
			a.grabNudge(ev.Rune())
		}
	}
	return nil
}

// grabNudge applies one motion to the grabbed item, commits it, and re-focuses it.
func (a *app) grabNudge(r rune) {
	loc, ok := a.store.Locate(a.grabUID)
	if !ok {
		a.cancelGrab()
		return
	}
	var newObj *model.Parsed
	var err error
	var label string

	if !a.grabIsEvent {
		td := findTodo(loc.Object, a.grabUID)
		if td == nil {
			a.cancelGrab()
			return
		}
		days := map[rune]int{'j': 1, 'k': -1, 'l': 7, 'h': -7}[r]
		if days == 0 {
			return
		}
		d := draftFromTodo(td)
		d.Due = d.Due.AddDate(0, 0, days)
		newObj, err = model.EditTodo(loc.Object, a.grabUID, d, a.now, a.loc)
		label = "due " + a.fmtDate(d.Due, d.DueAllDay)
	} else {
		master := findEvent(loc.Object, a.grabUID)
		if master == nil {
			a.cancelGrab()
			return
		}
		// For a this-occurrence grab, read (and move) the override's own position —
		// or, before the first nudge creates one, the occurrence's own slot rather
		// than the master's series start.
		base := master
		if a.grabScope == scopeThis {
			if ov := loc.Object.FindOverride(a.grabUID, a.grabOccStart); ov != nil {
				base = ov
			} else {
				occEnd := time.Time{}
				if dur := master.End.Sub(master.Start); dur > 0 {
					occEnd = a.grabOccStart.Add(dur)
				}
				base = &model.Event{Summary: master.Summary, Description: master.Description, Location: master.Location, AllDay: master.AllDay, Start: a.grabOccStart, End: occEnd}
			}
		}
		d := draftFromEvent(base)
		timed := a.mode == modeCalendar && a.viewMode != viewMonth && !base.AllDay
		switch r {
		case 'l', 'h': // ±1 day
			n := 1
			if r == 'h' {
				n = -1
			}
			d.Start = d.Start.AddDate(0, 0, n)
			if !d.End.IsZero() {
				d.End = d.End.AddDate(0, 0, n)
			}
		case 'j', 'k': // ±1 hour (timed events, in week/day view)
			if !timed {
				a.flash(a.grabTimeHint("change the time"))
				return
			}
			delta := grabHourStep
			if r == 'k' {
				delta = -grabHourStep
			}
			d.Start = d.Start.Add(delta)
			if !d.End.IsZero() {
				d.End = d.End.Add(delta)
			}
		case 'J', 'K': // resize the end ±1 hour
			if !timed {
				a.flash(a.grabTimeHint("resize"))
				return
			}
			base := d.End
			if base.IsZero() {
				base = d.Start
			}
			delta := grabHourStep
			if r == 'K' {
				delta = -grabHourStep
			}
			d.End = base.Add(delta)
			if !d.End.After(d.Start) {
				a.flash("event can't be that short")
				return
			}
		default:
			return
		}
		if a.grabScope == scopeThis {
			newObj, err = model.EditEventOccurrence(loc.Object, a.grabUID, a.grabOccStart, a.grabAllDay, d, a.now, a.loc)
		} else {
			newObj, err = model.EditEvent(loc.Object, a.grabUID, d, a.now, a.loc)
		}
		label = a.grabEventLabel(d)
	}
	if err != nil {
		a.flash(err.Error())
		return
	}
	if _, err := a.store.Put(context.Background(), a.grabCalID, a.grabName, newObj); err != nil {
		a.flash("Grab failed: " + err.Error())
		return
	}
	a.focusGrabbed()
	a.flash(label + "  ·  Enter keep · Esc cancel")
}

// grabEventLabel formats the grabbed event's new time for the flash line.
func (a *app) grabEventLabel(d model.EventDraft) string {
	if d.AllDay {
		return a.fmtDate(d.Start, true)
	}
	s := a.fmtWhen(d.Start, false)
	if !d.End.IsZero() {
		s += "–" + clockStr(d.End.In(a.loc), a.clock24)
	}
	return s
}

// focusGrabbed rebuilds the views and keeps the grabbed item selected/visible —
// a task by UID; a calendar event by navigating to its (possibly new) day and
// re-drilling onto its block.
func (a *app) focusGrabbed() {
	if a.grabIsEvent && a.mode == modeCalendar {
		if loc, ok := a.store.Locate(a.grabUID); ok {
			if ev := findEvent(loc.Object, a.grabUID); ev != nil {
				start := ev.Start
				// A this-occurrence grab moves the override, not the master, so anchor
				// on the override's (moved) start when one exists.
				if a.grabScope == scopeThis {
					if ov := loc.Object.FindOverride(a.grabUID, a.grabOccStart); ov != nil {
						start = ov.Start
					}
				}
				a.anchor = model.DayStart(start.In(a.loc))
			}
		}
		a.buildCenterCalendar()
		a.drillCalendarToUID(a.anchor, a.grabUID)
		a.updateStatus()
		return
	}
	a.refresh(a.grabUID)
}

// drillCalendarToUID drills the active calendar grid onto the item with uid on the
// given day (found by its position in that day's agenda order).
func (a *app) drillCalendarToUID(day time.Time, uid string) {
	items := a.dayItems(day)
	for i, it := range items {
		u := ""
		switch {
		case it.Todo != nil:
			u = it.Todo.UID
		case it.Event != nil:
			u = it.Event.UID
		}
		if u == uid {
			if g, ok := a.calendarPrimitive().(calGrid); ok {
				g.reDrill(day, i)
				a.setFocus(a.calendarPrimitive())
			}
			return
		}
	}
}

// commitGrab keeps the edits and records the pre-grab snapshot as the single undo
// step for the whole grab.
func (a *app) commitGrab() {
	if a.grabPrev != nil {
		a.pushUndo("grab", a.grabUID, undoOp{calID: a.grabCalID, name: a.grabName, prev: a.grabPrev})
	}
	a.focusGrabbed()
	a.endGrab()
	a.flash("Rescheduled (u to undo)")
}

// cancelGrab reverts to the pre-grab snapshot and ends grab mode.
func (a *app) cancelGrab() {
	if a.grabPrev != nil {
		_, _ = a.store.Restore(context.Background(), a.grabCalID, a.grabName, a.grabPrev)
	}
	a.focusGrabbed()
	a.endGrab()
	a.flash("Grab cancelled")
}

func (a *app) endGrab() {
	a.grabbing = false
	a.grabUID = ""
	a.grabPrev = nil
	a.grabCalID = ""
	a.grabName = ""
}

// draftFromEvent seeds an EventDraft from an event so a grab edit changes only the
// timing while EditEvent preserves every other property (recurrence, alarms, …).
func draftFromEvent(ev *model.Event) model.EventDraft {
	return model.EventDraft{
		Summary:     ev.Summary,
		Description: ev.Description,
		Location:    ev.Location,
		Start:       ev.Start,
		End:         ev.End,
		AllDay:      ev.AllDay,
	}
}
