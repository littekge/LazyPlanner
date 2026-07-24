package ui

import (
	"context"
	"errors"
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
// prompts for scope (this occurrence / this & future / all) via the picker.
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
		a.pickRecurrenceScope("event", true, func(s recurScope) {
			a.beginGrab(loc, t, s)
		})
		return
	}
	a.beginGrab(loc, t, scopeAll)
}

// beginGrab enters grab mode on the resolved target at the given recurrence scope.
func (a *app) beginGrab(loc store.Located, t editTarget, scope recurScope) {
	if scope == scopeFuture && !t.isTodo {
		a.beginGrabFuture(loc, t)
		return
	}
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

// beginGrabFuture starts a this-and-future grab of a recurring event: it splits
// the series now (cap the master at the occurrence, spawn a new series starting
// there with the same fields), then grabs the new series so nudges move the whole
// tail. A cancel deletes the new series and restores the master; a commit keeps
// both as one undo step (see cancelGrab/commitGrab).
func (a *app) beginGrabFuture(loc store.Located, t editTarget) {
	master := findEvent(loc.Object, t.uid)
	if master == nil {
		a.flash("Event not found")
		return
	}
	// The new series is the master's content starting at the occurrence (no field
	// change — the move happens via the nudges that follow).
	d := draftFromEvent(master)
	d.Start = t.occStart
	if dur := master.End.Sub(master.Start); dur > 0 {
		d.End = t.occStart.Add(dur)
	} else {
		d.End = time.Time{}
	}
	capped, future, err := model.SplitEvent(loc.Object, t.uid, t.occStart, d, a.now, a.loc)
	if err != nil {
		a.flashErr("Grab", err)
		return
	}
	if len(future.Events) == 0 {
		// Defensive: SplitEvent always yields one future event, but the TUI must
		// never index into an empty model result (crash-on-model-data rule).
		a.flashErr("Grab", errors.New("split produced no event"))
		return
	}
	newUID := future.Events[0].UID
	newName := store.ResourceName(newUID)
	if _, err := a.store.Put(context.Background(), loc.CalID, loc.Name, capped); err != nil {
		a.flashErr("Grab", err)
		return
	}
	if _, err := a.store.Put(context.Background(), loc.CalID, newName, future); err != nil {
		// The master was already capped above. Roll it back so a failed second
		// write can't half-complete the split — silently dropping the series'
		// tail occurrences with no grab state to cancel and no undo step.
		_, _ = a.store.Restore(context.Background(), loc.CalID, loc.Name, loc.Prev)
		a.flashErr("Grab", err)
		return
	}
	a.grabbing = true
	a.grabIsEvent = true
	a.grabScope = scopeFuture // nudges take the EditEvent (whole-series) path on the new series
	a.grabOccStart = t.occStart
	a.grabAllDay = t.allDay
	a.grabCalID = loc.CalID
	a.grabUID = newUID
	a.grabName = newName
	a.grabPrev = nil // the new series is created here; a cancel deletes it
	a.grabSplitMaster = loc.Name
	a.grabSplitMasterPrev = loc.Prev
	a.focusGrabbed() // rebuild + drill onto the new series (its UID replaced the old slot)
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
// rather than a dead key. A grabbed all-day event has no time to act on
// regardless of the active view, so it never tells the user to switch to a
// view they may already be in.
func (a *app) grabTimeHint(action string) string {
	if a.grabAllDay {
		return "all-day events have no time to " + action
	}
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
		if !td.HasDue {
			// startGrab gated on HasDue, but that was a stale snapshot: a concurrent
			// sync can clear the due date mid-grab. Nudging draftFromTodo's zero Due
			// would fabricate a year-1 date and flash it as a move. Refuse and end the
			// grab without reverting (reverting would re-add the due, clobbering the
			// server's clear) — same handling as any concurrent change underneath.
			a.abortGrabStale()
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
			// A whole-series day-move (scope all/future) must keep a day-pinning
			// rule (weekly BYDAY, monthly nth-weekday) consistent with the moved
			// anchor — otherwise the series stays on the old day, the moved DTSTART
			// falls outside its own rule, and the event vanishes from the calendar.
			// Re-anchor the rule; block an opaque "kept" rule we can't reason about.
			if a.grabScope != scopeThis && base.Recurring {
				recur, blocked := model.ReanchoredRecurrence(base, d.Start)
				if blocked {
					a.flash("Can't shift the day of this custom repeat rule — edit the rule instead")
					return
				}
				d.Recur = recur
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
				a.flash("Event can't be that short")
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
		a.flashErr("Grab", err)
		return
	}
	// Version-checked write: newObj was derived from loc's snapshot, so if a
	// background sync pulled a newer version of this resource since the Locate
	// above, committing would clobber it (adopt the pulled ETag while persisting
	// stale content). Skip and abort instead of overwriting the server's edit.
	applied, err := a.store.PutIfUnchanged(context.Background(), a.grabCalID, a.grabName, newObj, loc.Prev)
	if err != nil {
		a.flash("Grab failed: " + err.Error())
		return
	}
	if !applied {
		a.abortGrabStale()
		return
	}
	a.focusGrabbed()
	a.flash(label + "  ·  Enter keep · Esc cancel")
}

// abortGrabStale ends grab mode without reverting, used when a nudge detects the
// grabbed item changed underneath (a background sync pulled a newer version).
// Reverting to the pre-grab snapshot would re-clobber that pulled edit, so keep
// the server's version, drop the grab, and refresh onto the item.
func (a *app) abortGrabStale() {
	uid := a.grabUID
	a.endGrab()
	a.refresh(uid)
	a.flash("Item changed on the server — grab ended (move not applied)")
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

// commitGrab keeps the edits as one undo step. A this-and-future grab bundles two
// ops — delete the new series (prev nil) and restore the original master — so undo
// reverses the whole split; a normal grab records just the pre-grab snapshot.
func (a *app) commitGrab() {
	if a.grabSplitMaster != "" {
		a.pushUndo("grab", a.grabUID,
			undoOp{calID: a.grabCalID, name: a.grabName, prev: nil},
			undoOp{calID: a.grabCalID, name: a.grabSplitMaster, prev: a.grabSplitMasterPrev})
	} else if a.grabPrev != nil {
		a.pushUndo("grab", a.grabUID, undoOp{calID: a.grabCalID, name: a.grabName, prev: a.grabPrev})
	}
	a.focusGrabbed()
	a.endGrab()
	a.flash("Rescheduled (u to undo)")
}

// cancelGrab reverts and ends grab mode. A this-and-future grab undoes the split
// (restore the master, then delete the new series); a normal grab restores the
// single pre-grab snapshot. A revert write can fail (ENOSPC / permission), which
// must be surfaced rather than reported as a clean cancel — otherwise the user is
// told the series is intact while data was silently lost.
func (a *app) cancelGrab() {
	var revertErr error
	if a.grabSplitMaster != "" {
		// Restore the master (un-cap) first, and only drop the new tail series if
		// that succeeded — so a failed un-cap never leaves the future occurrences
		// held by neither the master nor the tail (both copies gone).
		restored := true
		if a.grabSplitMasterPrev != nil {
			if _, err := a.store.Restore(context.Background(), a.grabCalID, a.grabSplitMaster, a.grabSplitMasterPrev); err != nil {
				revertErr, restored = err, false
			}
		}
		if restored {
			if err := a.store.Delete(context.Background(), a.grabCalID, a.grabName); err != nil {
				revertErr = errors.Join(revertErr, err)
			}
		}
	} else if a.grabPrev != nil {
		if _, err := a.store.Restore(context.Background(), a.grabCalID, a.grabName, a.grabPrev); err != nil {
			revertErr = err
		}
	}
	a.focusGrabbed()
	a.endGrab()
	if revertErr != nil {
		a.flashErr("Grab cancel", revertErr)
		return
	}
	a.flash("Grab cancelled")
}

func (a *app) endGrab() {
	a.grabbing = false
	a.grabUID = ""
	a.grabPrev = nil
	a.grabCalID = ""
	a.grabName = ""
	a.grabScope = scopeAll
	a.grabSplitMaster = ""
	a.grabSplitMasterPrev = nil
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
