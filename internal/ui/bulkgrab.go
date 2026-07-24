package ui

import (
	"context"
	"errors"
	"fmt"
	"time"

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
	a.bulkGrabDrilled = false
	if a.mode == modeCalendar {
		if g, ok := a.calendarPrimitive().(calGrid); ok {
			if day, drilled, idx := g.drillState(); drilled {
				a.bulkGrabDay, a.bulkGrabIdx, a.bulkGrabDrilled = day, idx, true
			}
		}
	}
	a.grabbing = true
	a.flash(a.bulkGrabStatus())
}

// bulkGrabStatus describes the bulk-grab controls, shared by the entry flash and
// the persistent help bar (updateStatus) so the two can't drift.
func (a *app) bulkGrabStatus() string {
	return fmt.Sprintf("GRAB ×%d · h/l ±day · j/k ±week · Enter keep · Esc cancel", len(a.bulkGrab))
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
	drilled, day, idx := a.bulkGrabDrilled, a.bulkGrabDay, a.bulkGrabIdx
	// Not refreshKeepingDrill: its "keep" is the grid's current drill state,
	// which a mid-grab nudge may have already collapsed (see bulkGrabDrilled's
	// doc). The pre-grab snapshot is restored explicitly instead — valid because
	// every item is back exactly as it was, so the day is guaranteed non-empty.
	//
	// a.bulkGrab is deliberately left populated across this rebuild+redrill —
	// refresh() ends with its own syncSelectionVisuals() call, and clearing
	// bulkGrab before that would drop the guard (see that function's comment)
	// during the transient un-drilled window, wrongly ending the selection
	// before the redrill below gets a chance to fix the grid's view.
	a.grabbing = false
	a.refresh("")
	if drilled {
		if g, ok := a.calendarPrimitive().(calGrid); ok {
			g.reDrill(day, idx)
		}
	}
	a.endBulkGrab()
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
	a.bulkGrabDrilled = false
	a.bulkGrabDay = time.Time{}
	a.bulkGrabIdx = 0
}
