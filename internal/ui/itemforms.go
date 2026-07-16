package ui

import (
	"strings"
	"time"

	"github.com/rivo/tview"

	"github.com/littekge/LazyPlanner/internal/model"
	"github.com/littekge/LazyPlanner/internal/store"
)

// newTodoForm builds the task field set, pre-filled from td (nil = a blank
// create form). Buttons and border are added by the caller.
// todoFields holds references to a task form's inputs so values are read
// directly (labels change as the ▸ caret moves, so label lookup won't work).
type todoFields struct {
	summary, desc, dueDate, dueTime, tags *tview.InputField
	priority                              *tview.DropDown
	completed                             *tview.Checkbox
}

func (a *app) newTodoForm(td *model.Todo) (*caretForm, *todoFields) {
	summary, desc, tags, dueDate, dueTime := "", "", "", "", ""
	prio, completed := 0, false
	if td != nil {
		summary, desc = td.Summary, td.Description
		tags = strings.Join(td.Categories, ", ")
		prio, completed = td.Priority, td.Completed()
		if td.HasDue {
			dueDate = td.Due.In(a.loc).Format("2006-01-02")
			if !td.DueAllDay {
				dueTime = td.Due.In(a.loc).Format("15:04")
			}
		}
	}
	f := newCaretForm()
	fields := &todoFields{
		summary:   f.addInput("Summary", summary, 0),
		desc:      f.addInput("Description", desc, 0),
		dueDate:   f.addInput("Due date (YYYY-MM-DD)", dueDate, 12),
		dueTime:   f.addInput("Due time (HH:MM)", dueTime, 8),
		priority:  f.addDropDown("Priority", priorityOptions, prio),
		tags:      f.addInput("Tags (comma-sep)", tags, 0),
		completed: f.addCheckbox("Completed", completed),
	}
	f.stylePopup()
	return f, fields
}

// readTodoDraft reads the task fields. ParentUID is left empty for the caller to
// set (preserve on edit, assign on create).
func (a *app) readTodoDraft(f *todoFields) (model.TodoDraft, error) {
	date, hasDate, err := parseDateField(f.dueDate.GetText(), a.loc)
	if err != nil {
		return model.TodoDraft{}, errField("Due date", err)
	}
	h, m, hasTime, err := parseTimeField(f.dueTime.GetText())
	if err != nil {
		return model.TodoDraft{}, errField("Due time", err)
	}
	prio, _ := f.priority.GetCurrentOption()
	d := model.TodoDraft{
		Summary:     f.summary.GetText(),
		Description: f.desc.GetText(),
		Priority:    prio, // dropdown index maps directly: 0 = none, 1..9 = priority
		Categories:  splitTags(f.tags.GetText()),
		Completed:   f.completed.IsChecked(),
	}
	if hasDate {
		d.HasDue = true
		if hasTime {
			d.Due = time.Date(date.Year(), date.Month(), date.Day(), h, m, 0, 0, a.loc)
		} else {
			d.Due, d.DueAllDay = date, true
		}
	}
	return d, nil
}

func (a *app) showTodoForm(loc store.Located, uid string) {
	td := findTodo(loc.Object, uid)
	if td == nil {
		a.flash("Task not found")
		return
	}
	a.presentTodoForm(td, " Edit task ", func(d model.TodoDraft) {
		// Enforce the folder rule here too: the form's Completed checkbox must not
		// complete a task that still has incomplete children (Space is guarded in
		// toggleComplete; EditTodo has no child check).
		if d.Completed && !td.Completed() && a.hasIncompleteChildren(uid) {
			a.flash("Finish or remove its subtasks first")
			return
		}
		d.ParentUID = td.ParentUID // preserve the existing parent
		newObj, err := model.EditTodo(loc.Object, uid, d, a.now, a.loc)
		if err != nil {
			a.flashErr("Save", err)
			return
		}
		a.commitMutation(loc.CalID, loc.Name, newObj, loc.Prev, "edit task", uid, "Saved")
	})
}

// presentTodoForm opens the task form seeded from td, wiring Save to call onSave
// with the read draft. Shared by the plain edit and the scope-aware recurrence
// edits (a detached this-occurrence copy).
func (a *app) presentTodoForm(td *model.Todo, title string, onSave func(model.TodoDraft)) {
	f, fields := a.newTodoForm(td)
	f.AddButton("Save", func() {
		d, err := a.readTodoDraft(fields)
		if err != nil {
			a.flash(err.Error())
			return
		}
		onSave(d)
	})
	f.AddButton("Cancel", func() { a.closeModal(pageForm) })
	f.SetCancelFunc(func() { a.closeModal(pageForm) })
	f.SetBorder(true).SetTitle(title)
	a.openModal(pageForm, f, 62, 19)
}

func (a *app) showCreateTodoForm(calID, parentUID string) {
	title := " New task "
	if parentUID != "" {
		title = " New subtask "
	}
	f, fields := a.newTodoForm(nil)
	f.AddButton("Create", func() {
		d, err := a.readTodoDraft(fields)
		if err != nil {
			a.flash(err.Error())
			return
		}
		if strings.TrimSpace(d.Summary) == "" {
			a.flash("A summary is required")
			return
		}
		d.ParentUID = parentUID
		obj := model.NewTodoObject(d, a.now)
		uid := obj.Todos[0].UID
		a.commitMutation(calID, store.ResourceName(uid), obj, nil, "add task", uid, "Added task")
	})
	f.AddButton("Cancel", func() { a.closeModal(pageForm) })
	f.SetCancelFunc(func() { a.closeModal(pageForm) })
	f.SetBorder(true).SetTitle(title)
	a.openModal(pageForm, f, 62, 19)
}

// eventFields holds references to an event form's inputs.
type eventFields struct {
	summary, desc, location *tview.InputField
	startDate, startTime    *tview.InputField
	endDate, endTime        *tview.InputField
	allDay                  *tview.Checkbox
}

// newEventForm builds the event field set, pre-filled from ev (nil = a blank
// create form defaulting the start date to defaultDay).
func (a *app) newEventForm(ev *model.Event, defaultDay time.Time) (*caretForm, *eventFields) {
	summary, desc, location := "", "", ""
	allDay := true
	startDate := defaultDay.In(a.loc).Format("2006-01-02")
	startTime, endDate, endTime := "", "", ""
	if ev != nil {
		summary, desc, location = ev.Summary, ev.Description, ev.Location
		allDay = ev.AllDay
		startDate = ev.Start.In(a.loc).Format("2006-01-02")
		if ev.AllDay {
			if !ev.End.IsZero() { // DTEND is exclusive; show the inclusive last day
				endDate = ev.End.In(a.loc).AddDate(0, 0, -1).Format("2006-01-02")
			}
		} else {
			startTime = ev.Start.In(a.loc).Format("15:04")
			if !ev.End.IsZero() {
				endDate = ev.End.In(a.loc).Format("2006-01-02")
				endTime = ev.End.In(a.loc).Format("15:04")
			}
		}
	}
	f := newCaretForm()
	fields := &eventFields{
		summary:   f.addInput("Summary", summary, 0),
		desc:      f.addInput("Description", desc, 0),
		location:  f.addInput("Location", location, 0),
		allDay:    f.addCheckbox("All day", allDay),
		startDate: f.addInput("Start date (YYYY-MM-DD)", startDate, 12),
		startTime: f.addInput("Start time (HH:MM)", startTime, 8),
		endDate:   f.addInput("End date (YYYY-MM-DD)", endDate, 12),
		endTime:   f.addInput("End time (HH:MM)", endTime, 8),
	}
	f.stylePopup()
	return f, fields
}

func (a *app) readEventDraft(f *eventFields) (model.EventDraft, error) {
	allDay := f.allDay.IsChecked()
	sd, hasSD, err := parseDateField(f.startDate.GetText(), a.loc)
	if err != nil {
		return model.EventDraft{}, errField("Start date", err)
	}
	if !hasSD {
		return model.EventDraft{}, errFieldMsg("Start date is required")
	}

	var start, end time.Time
	if allDay {
		start = sd
		ed, hasED, err := parseDateField(f.endDate.GetText(), a.loc)
		if err != nil {
			return model.EventDraft{}, errField("End date", err)
		}
		last := sd
		if hasED {
			last = ed
		}
		end = last.AddDate(0, 0, 1) // DTEND is exclusive
	} else {
		sh, sm, _, err := parseTimeField(f.startTime.GetText())
		if err != nil {
			return model.EventDraft{}, errField("Start time", err)
		}
		start = time.Date(sd.Year(), sd.Month(), sd.Day(), sh, sm, 0, 0, a.loc)
		ed, hasED, err := parseDateField(f.endDate.GetText(), a.loc)
		if err != nil {
			return model.EventDraft{}, errField("End date", err)
		}
		eh, em, _, err := parseTimeField(f.endTime.GetText())
		if err != nil {
			return model.EventDraft{}, errField("End time", err)
		}
		if hasED {
			end = time.Date(ed.Year(), ed.Month(), ed.Day(), eh, em, 0, 0, a.loc)
		}
		if !end.After(start) {
			end = start.Add(time.Hour) // sensible default when end is blank/invalid
		}
	}
	return model.EventDraft{
		Summary:     f.summary.GetText(),
		Description: f.desc.GetText(),
		Location:    f.location.GetText(),
		Start:       start,
		End:         end,
		AllDay:      allDay,
	}, nil
}

func (a *app) showEventForm(loc store.Located, uid string) {
	ev := findEvent(loc.Object, uid)
	if ev == nil {
		a.flash("Event not found")
		return
	}
	a.presentEventForm(ev, ev.Start, " Edit event ", func(d model.EventDraft) {
		newObj, err := model.EditEvent(loc.Object, uid, d, a.now, a.loc)
		if err != nil {
			a.flashErr("Save", err)
			return
		}
		a.commitMutation(loc.CalID, loc.Name, newObj, loc.Prev, "edit event", uid, "Saved")
	})
}

// presentEventForm opens the event form seeded from ev at seedStart, wiring Save to
// call onSave with the read draft. Shared by the plain edit and the scope-aware
// recurrence edits (which pass a different onSave and seedStart / title).
func (a *app) presentEventForm(ev *model.Event, seedStart time.Time, title string, onSave func(model.EventDraft)) {
	f, fields := a.newEventForm(ev, seedStart)
	f.AddButton("Save", func() {
		d, err := a.readEventDraft(fields)
		if err != nil {
			a.flash(err.Error())
			return
		}
		onSave(d)
	})
	f.AddButton("Cancel", func() { a.closeModal(pageForm) })
	f.SetCancelFunc(func() { a.closeModal(pageForm) })
	f.SetBorder(true).SetTitle(title)
	a.openModal(pageForm, f, 62, 21)
}

func (a *app) showCreateEventForm(calID string, base time.Time) {
	f, fields := a.newEventForm(nil, base)
	f.AddButton("Create", func() {
		d, err := a.readEventDraft(fields)
		if err != nil {
			a.flash(err.Error())
			return
		}
		if strings.TrimSpace(d.Summary) == "" {
			a.flash("A summary is required")
			return
		}
		obj, err := model.NewEventObject(d, a.now)
		if err != nil {
			a.flashErr("Add", err)
			return
		}
		uid := obj.Events[0].UID
		a.commitMutation(calID, store.ResourceName(uid), obj, nil, "add event", uid, "Added event")
	})
	f.AddButton("Cancel", func() { a.closeModal(pageForm) })
	f.SetCancelFunc(func() { a.closeModal(pageForm) })
	f.SetBorder(true).SetTitle(" New event ")
	a.openModal(pageForm, f, 62, 21)
}
