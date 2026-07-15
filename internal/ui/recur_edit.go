package ui

import (
	"context"
	"errors"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"

	"github.com/littekge/LazyPlanner/internal/model"
	"github.com/littekge/LazyPlanner/internal/store"
)

// recurScope is the reach of an edit/delete on a recurring item.
type recurScope int

const (
	// scopeAll is the zero value deliberately: any grab/edit path that doesn't set a
	// scope (a non-recurring item, or a test) behaves as a whole-series/master edit,
	// which is the safe default and matches the pre-recurrence-editing behavior.
	scopeAll    recurScope = iota // the whole series (edit the master)
	scopeThis                     // just the selected occurrence
	scopeFuture                   // this occurrence and every later one
)

// pickRecurrenceScope shows the recurrence scope picker. noun is "event" or
// "task"; includeFuture adds "This & future" (events only — a todo shows a single
// live instance, so this-and-future collapses into all for it). onPick fires with
// the chosen scope; Cancel does nothing.
func (a *app) pickRecurrenceScope(noun string, includeFuture bool, onPick func(recurScope)) {
	type choice struct {
		label string
		scope recurScope
	}
	choices := []choice{{"This " + noun, scopeThis}}
	if includeFuture {
		choices = append(choices, choice{"This & future", scopeFuture})
	}
	choices = append(choices, choice{"All " + noun + "s", scopeAll})

	labels := make([]string, 0, len(choices)+1)
	for _, c := range choices {
		labels = append(labels, c.label)
	}
	labels = append(labels, "Cancel")

	modal := tview.NewModal().
		SetText("Recurring " + noun + " — apply change to:").
		AddButtons(labels).
		SetDoneFunc(func(_ int, label string) {
			a.closeModal(pageConfirm)
			for _, c := range choices {
				if c.label == label {
					onPick(c.scope)
					return
				}
			}
		})
	// Shared popup look (matching confirm).
	modal.SetBackgroundColor(tcell.ColorDefault)
	modal.SetTextColor(tcell.ColorDefault)
	modal.SetButtonBackgroundColor(tcell.ColorDefault)
	modal.SetButtonTextColor(tcell.ColorDefault)
	modal.SetButtonActivatedStyle(tcell.StyleDefault.Reverse(true))
	a.captureFocus()
	a.root.AddPage(pageConfirm, modal, true, true)
	a.tv.SetFocus(modal)
}

// editRecurring routes an edit of a recurring item through the scope picker.
func (a *app) editRecurring(loc store.Located, t editTarget) {
	if t.isTodo {
		a.pickRecurrenceScope("task", false, func(s recurScope) {
			switch s {
			case scopeAll:
				a.showTodoForm(loc, t.uid)
			case scopeThis:
				a.editTodoThisOccurrence(loc, t.uid)
			}
		})
		return
	}
	a.pickRecurrenceScope("event", true, func(s recurScope) {
		a.editEventScoped(loc, t, s)
	})
}

// editEventScoped opens the event form and, on save, applies the edit at the
// chosen scope: the master (all), a RECURRENCE-ID override (this), or a series
// split (this & future). The form seeds its start from the occurrence, not the
// master, so this/future edits start from what the user sees.
func (a *app) editEventScoped(loc store.Located, t editTarget, scope recurScope) {
	ev := findEvent(loc.Object, t.uid)
	if ev == nil {
		a.flash("Event not found")
		return
	}
	switch scope {
	case scopeAll:
		a.showEventForm(loc, t.uid)
	case scopeThis:
		// Seed from an existing override for this occurrence (so re-editing shows the
		// prior per-occurrence values, not the master's), else from the master at the
		// occurrence's slot.
		seed, seedStart := ev, t.occStart
		if ov := loc.Object.FindOverride(t.uid, t.occStart); ov != nil {
			seed, seedStart = ov, ov.Start
		}
		a.presentEventForm(seed, seedStart, " Edit this occurrence ", func(d model.EventDraft) {
			newObj, err := model.EditEventOccurrence(loc.Object, t.uid, t.occStart, t.allDay, d, a.now, a.loc)
			if err != nil {
				a.flashErr("Edit", err)
				return
			}
			a.commitMutation(loc.CalID, loc.Name, newObj, loc.Prev, "edit occurrence", t.uid, "Saved this occurrence")
		})
	case scopeFuture:
		a.presentEventForm(ev, t.occStart, " Edit this & future ", func(d model.EventDraft) {
			capped, future, err := model.SplitEvent(loc.Object, t.uid, t.occStart, d, a.now, a.loc)
			if err != nil {
				a.flashErr("Edit", err)
				return
			}
			if len(future.Events) == 0 {
				// Defensive: never index into an empty model result (crash-on-model-data rule).
				a.flashErr("Edit", errors.New("split produced no event"))
				return
			}
			a.commitSplit(loc, future.Events[0].UID, capped, future, "edit this & future", "Split series (u to undo)")
		})
	}
}

// editTodoThisOccurrence detaches the current instance of a recurring todo as a
// standalone task carrying the edits, and advances the series past it — so "edit
// this occurrence" needs no override-on-read for todos.
func (a *app) editTodoThisOccurrence(loc store.Located, uid string) {
	td := findTodo(loc.Object, uid)
	if td == nil {
		a.flash("Task not found")
		return
	}
	// This scope splits the task in two, which isn't obvious — confirm first.
	a.confirmOK("Edit only this occurrence? It becomes a separate one-off task and the recurring series advances to its next occurrence.", "Detach", func() {
		a.editTodoDetachForm(loc, uid, td)
	})
}

// editTodoDetachForm opens the edit form whose save detaches the current instance
// as a standalone task and advances the series (the confirmed this-occurrence path).
func (a *app) editTodoDetachForm(loc store.Located, uid string, td *model.Todo) {
	a.presentTodoForm(td, " Edit this occurrence ", func(d model.TodoDraft) {
		d.ParentUID = td.ParentUID
		advanced, _, err := model.AdvanceRecurringTodo(loc.Object, uid, a.now, a.loc)
		if err != nil {
			a.flashErr("Edit", err)
			return
		}
		standalone := model.NewTodoObject(d, a.now)
		newUID := standalone.Todos[0].UID
		a.commitDetach(loc, newUID, advanced, standalone)
	})
}

// commitDetach writes the advanced series (same resource) and the detached
// standalone one-off (new resource) as one undo step — the store side of a
// this-occurrence todo detach. If the standalone write fails, the series is
// rolled back so the detach is atomic: the occurrence is never lost (gone from
// the series yet never a one-off), mirroring commitSplit/beginGrabFuture.
func (a *app) commitDetach(loc store.Located, newUID string, advanced, standalone *model.Parsed) {
	newName := store.ResourceName(newUID)
	if _, err := a.store.Put(context.Background(), loc.CalID, loc.Name, advanced); err != nil {
		a.flash("Save failed: " + err.Error())
		return
	}
	if _, err := a.store.Put(context.Background(), loc.CalID, newName, standalone); err != nil {
		// The series was already advanced (this occurrence consumed) above. Roll it
		// back so a failed standalone write can't lose the occurrence — it would be
		// gone from the series and never the one-off task the confirm promised.
		_, _ = a.store.Restore(context.Background(), loc.CalID, loc.Name, loc.Prev)
		a.flash("Save failed: " + err.Error())
		return
	}
	a.pushUndo("edit occurrence", newUID,
		undoOp{calID: loc.CalID, name: loc.Name, prev: loc.Prev},
		undoOp{calID: loc.CalID, name: newName, prev: nil})
	a.refresh(newUID)
	a.closeModal(pageForm)
	a.flash("Detached this occurrence (u to undo)")
}

// deleteRecurring routes a delete of a recurring item through the scope picker.
func (a *app) deleteRecurring(loc store.Located, t editTarget) {
	noun, includeFuture := "event", true
	if t.isTodo {
		noun, includeFuture = "task", false
	}
	a.pickRecurrenceScope(noun, includeFuture, func(s recurScope) {
		switch s {
		case scopeAll:
			a.deleteWholeObject(loc, t.uid)
		case scopeFuture:
			capped, err := model.CapSeries(loc.Object, t.uid, t.occStart.Add(-time.Second), a.now, a.loc)
			if err != nil {
				a.flashErr("Delete", err)
				return
			}
			a.commitMutationKeepingDrill(loc.CalID, loc.Name, capped, loc.Prev, "delete this & future", t.uid, "Deleted this & future")
		case scopeThis:
			a.deleteOccurrence(loc, t)
		}
	})
}

// deleteOccurrence removes just the selected occurrence: an EXDATE for an event,
// or an advance-past-it for a todo (deleting the last remaining occurrence of a
// todo removes the whole resource).
func (a *app) deleteOccurrence(loc store.Located, t editTarget) {
	if !t.isTodo {
		newObj, err := model.AddException(loc.Object, t.uid, t.occStart, t.allDay, a.now, a.loc)
		if err != nil {
			a.flashErr("Delete", err)
			return
		}
		a.commitMutationKeepingDrill(loc.CalID, loc.Name, newObj, loc.Prev, "delete occurrence", t.uid, "Deleted this occurrence")
		return
	}
	advanced, done, err := model.AdvanceRecurringTodo(loc.Object, t.uid, a.now, a.loc)
	if err != nil {
		a.flashErr("Delete", err)
		return
	}
	if done {
		// No further occurrence — deleting "this" removes the task entirely.
		if err := a.store.Delete(context.Background(), loc.CalID, loc.Name); err != nil {
			a.flash("Delete failed: " + err.Error())
			return
		}
		a.pushUndo("delete occurrence", "", undoOp{calID: loc.CalID, name: loc.Name, prev: loc.Prev})
		a.refresh("")
		a.flash("Deleted (last occurrence)")
		return
	}
	a.commitMutationKeepingDrill(loc.CalID, loc.Name, advanced, loc.Prev, "skip occurrence", t.uid, "Skipped this occurrence")
}

// advanceRecurringTodo completes one occurrence of a recurring todo by rolling it
// to the next (Space semantics); the last occurrence marks the todo done.
func (a *app) advanceRecurringTodo(loc store.Located, uid string) {
	advanced, done, err := model.AdvanceRecurringTodo(loc.Object, uid, a.now, a.loc)
	if err != nil {
		a.flashErr("Complete", err)
		return
	}
	if _, err := a.store.Put(context.Background(), loc.CalID, loc.Name, advanced); err != nil {
		a.flash("Update failed: " + err.Error())
		return
	}
	// When the series finishes, the todo is now completed — keep it visible until
	// the view is left, like a plain complete-while-hidden.
	if done && !a.showCompleted {
		a.stickyDone[uid] = true
	}
	a.pushUndo("complete occurrence", uid, undoOp{calID: loc.CalID, name: loc.Name, prev: loc.Prev})
	a.refreshKeepingDrill(uid)
	// A recurring task advances rather than completing, which is easy to miss — make
	// the flash stand out (accent color + a glyph + the new due date) so it's clear
	// the task moved on rather than being checked off.
	if done {
		a.flash("[yellow]✓ Recurring task done — final occurrence completed[-]")
		return
	}
	next := ""
	if td := findTodo(advanced, uid); td != nil && td.HasDue {
		next = " → next due " + a.fmtDate(td.Due, td.DueAllDay)
	}
	a.flash("[yellow]↻ Recurring task advanced (not completed)" + next + "[-]")
}

// commitSplit writes the capped master (same resource) and a new-UID future series
// (new resource) as one undo step — the store side of a this-and-future split.
func (a *app) commitSplit(loc store.Located, futureUID string, capped, future *model.Parsed, label, done string) {
	newName := store.ResourceName(futureUID)
	if _, err := a.store.Put(context.Background(), loc.CalID, loc.Name, capped); err != nil {
		a.flash("Save failed: " + err.Error())
		return
	}
	if _, err := a.store.Put(context.Background(), loc.CalID, newName, future); err != nil {
		// The master was already capped (its RRULE truncated) above. Roll it back so
		// a failed second write can't half-complete the split — permanently dropping
		// the series' tail occurrences with no undo step to recover from. Mirrors
		// beginGrabFuture's rollback.
		_, _ = a.store.Restore(context.Background(), loc.CalID, loc.Name, loc.Prev)
		a.flash("Save failed: " + err.Error())
		return
	}
	a.pushUndo(label, futureUID,
		undoOp{calID: loc.CalID, name: loc.Name, prev: loc.Prev},
		undoOp{calID: loc.CalID, name: newName, prev: nil})
	a.refresh(futureUID)
	a.closeModal(pageForm)
	a.flash(done + " (u to undo)")
}
