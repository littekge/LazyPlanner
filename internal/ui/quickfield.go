package ui

import (
	"context"
	"strings"
	"time"

	"github.com/littekge/LazyPlanner/internal/model"
)

// Quick field-set: change one field of the selected task without the full edit
// form. Grouped under the `s` ("set") chord — `sp` priority, `sd` due — each a
// one-line prompt. The rest of the task (and any unknown iCal properties) is
// preserved via a draft cloned from the current values.

// draftFromTodo builds an edit draft that reproduces td exactly, so a quick set
// can change a single field and leave everything else intact.
func draftFromTodo(td *model.Todo) model.TodoDraft {
	return model.TodoDraft{
		Summary:     td.Summary,
		Description: td.Description,
		HasDue:      td.HasDue,
		Due:         td.Due,
		DueAllDay:   td.DueAllDay,
		Priority:    td.Priority,
		Categories:  td.Categories,
		ParentUID:   td.ParentUID,
		Completed:   td.Completed(),
	}
}

// applyTodoField relocates the task, applies mut to a draft of its current
// values, writes it, and records an undo step. Used by the quick field-sets.
func (a *app) applyTodoField(uid, label string, mut func(*model.TodoDraft)) {
	loc, ok := a.store.Locate(uid)
	if !ok {
		a.flash("Task not found")
		return
	}
	if !a.guardWrite(loc.CalID) {
		return
	}
	td := findTodo(loc.Object, uid)
	if td == nil {
		a.flash("Task not found")
		return
	}
	draft := draftFromTodo(td)
	mut(&draft)
	obj, err := model.EditTodo(loc.Object, uid, draft, a.now, a.loc)
	if err != nil {
		a.flashErr("Set", err)
		return
	}
	if _, err := a.store.Put(context.Background(), loc.CalID, loc.Name, obj); err != nil {
		a.flash("Save failed: " + err.Error())
		return
	}
	a.pushUndo(label, uid, undoOp{calID: loc.CalID, name: loc.Name, prev: loc.Prev})
	a.refresh(uid)
	a.flash(label + undoHint)
}

// quickTaskTarget returns the selected task's uid, guarding that a writable task
// is actually selected. It flashes and returns ok=false otherwise.
//
// Note: on a recurring task, sp/sd edit the whole series (its master fields), with
// no this/future/all picker — like grab's due-nudge. Only e/d offer per-occurrence
// scope (detach), because changing a single field/due of one occurrence of a task
// (shown as a single live instance) doesn't map cleanly; use e to detach first.
func (a *app) quickTaskTarget() (string, bool) {
	t, ok := a.currentTarget()
	if !ok || !t.isTodo {
		a.flash("Select a task first")
		return "", false
	}
	loc, ok := a.store.Locate(t.uid)
	if !ok {
		a.flash("Task not found")
		return "", false
	}
	if !a.guardWrite(loc.CalID) {
		return "", false
	}
	return t.uid, true
}

// setPriorityPrompt (sp) sets the selected task's priority from a one-line input.
func (a *app) setPriorityPrompt() {
	uid, ok := a.quickTaskTarget()
	if !ok {
		return
	}
	a.promptInput("Priority (1-9 / high·med·low, blank clears)", "! ", func(text string) {
		p, ok := parseSetPriority(text, a.now, a.loc)
		if !ok {
			a.flash("priority: 1-9 or high/med/low (blank clears)")
			return
		}
		a.applyTodoField(uid, "set priority", func(d *model.TodoDraft) { d.Priority = p })
	})
}

// setDuePrompt (sd) sets or clears the selected task's due date from a
// smart-parsed one-line input (same tokens as quick-add; blank clears).
func (a *app) setDuePrompt() {
	uid, ok := a.quickTaskTarget()
	if !ok {
		return
	}
	a.promptInput("Due (e.g. 'fri', 'jul 20', 3pm; blank clears)", "due ", func(text string) {
		text = strings.TrimSpace(text)
		if text == "" {
			a.applyTodoField(uid, "clear due", func(d *model.TodoDraft) {
				d.HasDue, d.Due, d.DueAllDay = false, time.Time{}, false
			})
			return
		}
		qa := model.ParseQuickAdd(text, a.now, a.loc)
		if !qa.HasDate && !qa.HasTime {
			a.flash("due: couldn't read a date from " + text)
			return
		}
		when, allDay := qa.At(model.DayStart(a.now), a.loc)
		a.applyTodoField(uid, "set due", func(d *model.TodoDraft) {
			d.HasDue, d.Due, d.DueAllDay = true, when, allDay
		})
	})
}

// parseSetPriority reads a priority from the quick-set input, reusing the
// quick-add token rules. Blank / "0" / "none" clears it (returns 0, true).
func parseSetPriority(text string, now time.Time, loc *time.Location) (int, bool) {
	text = strings.TrimSpace(text)
	text = strings.TrimPrefix(text, "!")
	if text == "" || text == "0" || strings.EqualFold(text, "none") {
		return 0, true
	}
	qa := model.ParseQuickAdd("!"+text, now, loc)
	if qa.Priority == 0 {
		return 0, false
	}
	return qa.Priority, true
}
