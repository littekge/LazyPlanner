package ui

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"

	"github.com/littekge/LazyPlanner/internal/model"
	"github.com/littekge/LazyPlanner/internal/store"
)

// Field validation messages surfaced in the status bar.
var (
	errDateFormat = errors.New("use YYYY-MM-DD")
	errTimeFormat = errors.New("use HH:MM or h:mmpm")
)

func errField(name string, err error) error { return errors.New(name + ": " + err.Error()) }
func errFieldMsg(msg string) error          { return errors.New(msg) }

// reparentDir is the direction of an H/L re-parent.
type reparentDir int

const (
	outdent reparentDir = iota // H: move up to the grandparent
	indent                     // L: nest under the previous sibling
)

// undoOp reverses one resource write: a nil prev marks a creation (undo by
// deleting), a non-nil prev restores that earlier snapshot exactly.
type undoOp struct {
	calID string
	name  string
	prev  *store.Resource
}

// undoStep is one user action's worth of reversal — usually a single op, but a
// recursive folder delete carries one op per removed resource. selUID is the
// item to reselect after undoing.
type undoStep struct {
	label  string
	selUID string
	ops    []undoOp
}

func (a *app) pushUndo(label, selUID string, ops ...undoOp) {
	a.undo = append(a.undo, undoStep{label: label, selUID: selUID, ops: ops})
}

// editTarget is the item the editing shortcuts act on in the current context.
type editTarget struct {
	isTodo bool
	uid    string
}

// --- target resolution ---

// currentTarget returns the item the editing keys should act on, based on the
// active pane: the selected tree node (tasks), the cycled calendar event (month
// event mode), or the highlighted agenda row.
func (a *app) currentTarget() (editTarget, bool) {
	switch a.mode {
	case modeTasks:
		if node := a.tree.GetCurrentNode(); node != nil {
			if t, ok := node.GetReference().(*model.Todo); ok {
				return editTarget{isTodo: true, uid: t.UID}, true
			}
		}
	case modeCalendar:
		if a.viewMode == viewMonth && a.month.eventMode {
			items := a.month.selectedItems()
			if i := a.month.eventIndex; i >= 0 && i < len(items) {
				return targetFromItem(items[i]), true
			}
		}
	case modeAgenda:
		items := a.dayItems(model.DayStart(a.now))
		if i := a.agendaList.GetCurrentItem(); i >= 0 && i < len(items) {
			return targetFromItem(items[i]), true
		}
	}
	return editTarget{}, false
}

func targetFromItem(it model.AgendaItem) editTarget {
	if it.IsTodo() {
		return editTarget{isTodo: true, uid: it.Todo.UID}
	}
	return editTarget{isTodo: false, uid: it.Event.UID}
}

// --- quick add (a) ---

// addQuick (a): quick-add a top-level task (Tasks) or an event (Calendar/Agenda).
func (a *app) addQuick() {
	if a.mode == modeTasks {
		calID := a.selectedTasklistID()
		if calID == "" {
			a.flash("No task list selected — create one first")
			return
		}
		a.promptInput("New task", "Task: ", func(text string) { a.createTask(calID, "", text) })
		return
	}
	calID, base, ok := a.eventCreateContext()
	if !ok {
		return
	}
	a.promptInput("New event", "Event: ", func(text string) { a.createEvent(calID, base, text) })
}

// addSubtaskQuick (s): quick-add a subtask under the highlighted task.
func (a *app) addSubtaskQuick() {
	if a.mode != modeTasks {
		return
	}
	calID, parentUID, ok := a.subtaskContext()
	if !ok {
		return
	}
	a.promptInput("New subtask", "Subtask: ", func(text string) { a.createTask(calID, parentUID, text) })
}

// addFull (A): open the full create form for a top-level task or an event.
func (a *app) addFull() {
	if a.mode == modeTasks {
		calID := a.selectedTasklistID()
		if calID == "" {
			a.flash("No task list selected — create one first")
			return
		}
		a.showCreateTodoForm(calID, "")
		return
	}
	calID, base, ok := a.eventCreateContext()
	if !ok {
		return
	}
	a.showCreateEventForm(calID, base)
}

// addSubtaskFull (S): full create form for a subtask under the highlighted task.
func (a *app) addSubtaskFull() {
	if a.mode != modeTasks {
		return
	}
	calID, parentUID, ok := a.subtaskContext()
	if !ok {
		return
	}
	a.showCreateTodoForm(calID, parentUID)
}

// eventCreateContext resolves the target calendar and default day for a new event.
func (a *app) eventCreateContext() (calID string, base time.Time, ok bool) {
	calID = a.selectedCalendarID()
	if calID == "" {
		a.flash("No calendar selected")
		return "", time.Time{}, false
	}
	base = a.anchor
	if a.mode == modeAgenda {
		base = model.DayStart(a.now)
	}
	return calID, base, true
}

// subtaskContext resolves the target list and the highlighted task as parent.
func (a *app) subtaskContext() (calID, parentUID string, ok bool) {
	calID = a.selectedTasklistID()
	if calID == "" {
		a.flash("No task list selected")
		return "", "", false
	}
	node := a.tree.GetCurrentNode()
	if node == nil {
		a.flash("Select a task to add a subtask under")
		return "", "", false
	}
	t, isTodo := node.GetReference().(*model.Todo)
	if !isTodo {
		a.flash("Select a task to add a subtask under")
		return "", "", false
	}
	return calID, t.UID, true
}

// createTask parses a quick-add line and writes a new task under parentUID.
func (a *app) createTask(calID, parentUID, text string) {
	qa := model.ParseQuickAdd(text, a.now, a.loc)
	if strings.TrimSpace(qa.Title) == "" {
		a.flash("Nothing added")
		return
	}
	d := model.TodoDraft{
		Summary:    qa.Title,
		Priority:   qa.Priority,
		Categories: qa.Tags,
		ParentUID:  parentUID,
	}
	if qa.HasDate || qa.HasTime {
		when, allDay := qa.At(model.DayStart(a.now), a.loc)
		d.HasDue, d.Due, d.DueAllDay = true, when, allDay
	}
	obj := model.NewTodoObject(d, a.now)
	uid := obj.Todos[0].UID
	name := store.ResourceName(uid)
	if _, err := a.store.Put(context.Background(), calID, name, obj); err != nil {
		a.flash("Add failed: " + err.Error())
		return
	}
	a.pushUndo("add task", uid, undoOp{calID: calID, name: name})
	a.refresh(uid)
	a.flash("Added task")
}

// createEvent parses a quick-add line and writes a new event, defaulting its day
// to base when the text carries no explicit date.
func (a *app) createEvent(calID string, base time.Time, text string) {
	qa := model.ParseQuickAdd(text, a.now, a.loc)
	if strings.TrimSpace(qa.Title) == "" {
		a.flash("Nothing added")
		return
	}
	start, allDay := qa.At(base, a.loc)
	end := start.Add(time.Hour)
	if allDay {
		end = start.AddDate(0, 0, 1)
	}
	obj, err := model.NewEventObject(model.EventDraft{
		Summary: qa.Title,
		Start:   start,
		End:     end,
		AllDay:  allDay,
	}, a.now)
	if err != nil {
		a.flash("Add failed: " + err.Error())
		return
	}
	uid := obj.Events[0].UID
	name := store.ResourceName(uid)
	if _, err := a.store.Put(context.Background(), calID, name, obj); err != nil {
		a.flash("Add failed: " + err.Error())
		return
	}
	a.pushUndo("add event", uid, undoOp{calID: calID, name: name})
	a.refresh(uid)
	a.flash("Added event")
}

// --- complete toggle (Space) ---

func (a *app) toggleComplete() {
	t, ok := a.currentTarget()
	if !ok || !t.isTodo {
		return // nothing toggleable here; stay silent
	}
	loc, ok := a.store.Locate(t.uid)
	if !ok {
		a.flash("Task not found")
		return
	}
	td := findTodo(loc.Object, t.uid)
	if td == nil {
		a.flash("Task not found")
		return
	}
	// A folder (has incomplete children) can't be completed until they are.
	if !td.Completed() && a.hasIncompleteChildren(t.uid) {
		a.flash("Finish or remove its subtasks first")
		return
	}
	completing := !td.Completed()
	newObj, err := model.SetTodoCompleted(loc.Object, t.uid, completing, a.now, a.loc)
	if err != nil {
		a.flash(err.Error())
		return
	}
	if _, err := a.store.Put(context.Background(), loc.CalID, loc.Name, newObj); err != nil {
		a.flash("Update failed: " + err.Error())
		return
	}
	// Keep a just-completed task visible until the list is left (see refresh).
	if completing && !a.showCompleted && a.mode == modeTasks {
		a.stickyDone[t.uid] = true
	}
	a.pushUndo("toggle done", t.uid, undoOp{calID: loc.CalID, name: loc.Name, prev: loc.Prev})
	a.refresh(t.uid)
}

// hasIncompleteChildren reports whether any todo anywhere is an incomplete child
// of uid — the definition of a "folder".
func (a *app) hasIncompleteChildren(uid string) bool {
	for _, t := range a.store.Todos() {
		if t.ParentUID == uid && !t.Completed() {
			return true
		}
	}
	return false
}

// descendants returns every UID beneath uid in the subtask tree (all depths),
// so deleting a task can take its whole subtree with it.
func (a *app) descendants(uid string) []string {
	childrenOf := map[string][]string{}
	for _, t := range a.store.Todos() {
		if t.ParentUID != "" {
			childrenOf[t.ParentUID] = append(childrenOf[t.ParentUID], t.UID)
		}
	}
	var out []string
	seen := map[string]bool{uid: true} // guard against malformed cycles
	var walk func(u string)
	walk = func(u string) {
		for _, c := range childrenOf[u] {
			if seen[c] {
				continue
			}
			seen[c] = true
			out = append(out, c)
			walk(c)
		}
	}
	walk(uid)
	return out
}

// --- delete (d) ---

func (a *app) deleteSelected() {
	t, ok := a.currentTarget()
	if !ok {
		a.flash("Nothing selected to delete")
		return
	}
	loc, ok := a.store.Locate(t.uid)
	if !ok {
		a.flash("Item not found")
		return
	}
	what := summaryOf(loc.Object, t.uid)

	// A task's subtree is deleted with it (deleting a folder is recursive).
	kids := a.descendants(t.uid)
	prompt := "Delete \"" + oneLine(what) + "\"?"
	if n := len(kids); n > 0 {
		prompt = "Delete \"" + oneLine(what) + "\" and its " + strconv.Itoa(n) + " subtask(s)?"
	}

	a.confirm(prompt, func() {
		var ops []undoOp
		for _, uid := range append([]string{t.uid}, kids...) {
			l, ok := a.store.Locate(uid)
			if !ok {
				continue
			}
			if err := a.store.Delete(context.Background(), l.CalID, l.Name); err != nil {
				a.flash("Delete failed: " + err.Error())
				return
			}
			ops = append(ops, undoOp{calID: l.CalID, name: l.Name, prev: l.Prev})
		}
		a.pushUndo("delete", "", ops...)
		a.refresh("")
		a.flash("Deleted (u to undo)")
	})
}

// --- re-parent (H / L) ---

func (a *app) reparentSelected(dir reparentDir) {
	node := a.tree.GetCurrentNode()
	if node == nil {
		return
	}
	td, ok := node.GetReference().(*model.Todo)
	if !ok {
		return
	}
	calID := a.selectedTasklistID()
	if calID == "" {
		return
	}
	cal, ok := a.store.Calendar(calID)
	if !ok {
		return
	}
	var todos []*model.Todo
	for _, r := range cal.Resources {
		todos = append(todos, r.Object.Todos...)
	}
	roots := model.BuildTree(todos, a.showCompleted)
	_, parent, siblings, idx := findInForest(roots, td.UID)

	var newParent string
	switch dir {
	case indent:
		if idx <= 0 {
			a.flash("Can't indent: no task above at this level")
			return
		}
		newParent = siblings[idx-1].Todo.UID
	case outdent:
		if parent == nil {
			a.flash("Already at the top level")
			return
		}
		newParent = parent.Todo.ParentUID // grandparent, or "" to become a root
	}

	loc, ok := a.store.Locate(td.UID)
	if !ok {
		a.flash("Task not found")
		return
	}
	newObj, err := model.SetTodoParent(loc.Object, td.UID, newParent, a.now, a.loc)
	if err != nil {
		a.flash(err.Error())
		return
	}
	if _, err := a.store.Put(context.Background(), loc.CalID, loc.Name, newObj); err != nil {
		a.flash("Move failed: " + err.Error())
		return
	}
	a.pushUndo("re-parent", td.UID, undoOp{calID: loc.CalID, name: loc.Name, prev: loc.Prev})
	a.refresh(td.UID)
	a.flash("Moved task")
}

// findInForest locates uid in a TodoNode forest, returning its node, its parent
// (nil at the root level), the sibling slice it lives in, and its index there.
func findInForest(roots []*model.TodoNode, uid string) (node, parent *model.TodoNode, siblings []*model.TodoNode, idx int) {
	var rec func(list []*model.TodoNode, par *model.TodoNode) bool
	rec = func(list []*model.TodoNode, par *model.TodoNode) bool {
		for i, n := range list {
			if n.Todo.UID == uid {
				node, parent, siblings, idx = n, par, list, i
				return true
			}
			if rec(n.Children, n) {
				return true
			}
		}
		return false
	}
	rec(roots, nil)
	return node, parent, siblings, idx
}

// --- edit (e) ---

func (a *app) editSelected() {
	t, ok := a.currentTarget()
	if !ok {
		a.flash("Nothing selected to edit")
		return
	}
	loc, ok := a.store.Locate(t.uid)
	if !ok {
		a.flash("Item not found")
		return
	}
	if t.isTodo {
		a.showTodoForm(loc, t.uid)
	} else {
		a.showEventForm(loc, t.uid)
	}
}

// newTodoForm builds the task field set, pre-filled from td (nil = a blank
// create form). Buttons and border are added by the caller.
func (a *app) newTodoForm(td *model.Todo) *tview.Form {
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
	form := tview.NewForm()
	form.AddInputField("Summary", summary, 0, nil, nil)
	form.AddInputField("Description", desc, 0, nil, nil)
	form.AddInputField("Due date (YYYY-MM-DD)", dueDate, 12, nil, nil)
	form.AddInputField("Due time (HH:MM)", dueTime, 8, nil, nil)
	form.AddDropDown("Priority", priorityOptions, prio, nil)
	form.AddInputField("Tags (comma-sep)", tags, 0, nil, nil)
	form.AddCheckbox("Completed", completed, nil)
	styleBWForm(form)
	return form
}

// readTodoDraft reads the task fields from the form. ParentUID is left empty for
// the caller to set (preserve on edit, assign on create).
func (a *app) readTodoDraft(form *tview.Form) (model.TodoDraft, error) {
	date, hasDate, err := parseDateField(formText(form, "Due date (YYYY-MM-DD)"), a.loc)
	if err != nil {
		return model.TodoDraft{}, errField("Due date", err)
	}
	h, m, hasTime, err := parseTimeField(formText(form, "Due time (HH:MM)"))
	if err != nil {
		return model.TodoDraft{}, errField("Due time", err)
	}
	prio, _ := form.GetFormItemByLabel("Priority").(*tview.DropDown).GetCurrentOption()
	d := model.TodoDraft{
		Summary:     formText(form, "Summary"),
		Description: formText(form, "Description"),
		Priority:    prio, // dropdown index maps directly: 0 = none, 1..9 = priority
		Categories:  splitTags(formText(form, "Tags (comma-sep)")),
		Completed:   form.GetFormItemByLabel("Completed").(*tview.Checkbox).IsChecked(),
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
	form := a.newTodoForm(td)
	form.AddButton("Save", func() {
		d, err := a.readTodoDraft(form)
		if err != nil {
			a.flash(err.Error())
			return
		}
		d.ParentUID = td.ParentUID // preserve the existing parent
		newObj, err := model.EditTodo(loc.Object, uid, d, a.now, a.loc)
		if err != nil {
			a.flash(err.Error())
			return
		}
		a.commitMutation(loc.CalID, loc.Name, newObj, loc.Prev, "edit task", uid, "Saved")
	})
	form.AddButton("Cancel", func() { a.closeModal(pageForm) })
	form.SetCancelFunc(func() { a.closeModal(pageForm) })
	form.SetBorder(true).SetTitle(" Edit task ")
	a.openModal(pageForm, form, 62, 19)
}

func (a *app) showCreateTodoForm(calID, parentUID string) {
	title := " New task "
	if parentUID != "" {
		title = " New subtask "
	}
	form := a.newTodoForm(nil)
	form.AddButton("Create", func() {
		d, err := a.readTodoDraft(form)
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
	form.AddButton("Cancel", func() { a.closeModal(pageForm) })
	form.SetCancelFunc(func() { a.closeModal(pageForm) })
	form.SetBorder(true).SetTitle(title)
	a.openModal(pageForm, form, 62, 19)
}

// newEventForm builds the event field set, pre-filled from ev (nil = a blank
// create form defaulting the start date to defaultDay).
func (a *app) newEventForm(ev *model.Event, defaultDay time.Time) *tview.Form {
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
	form := tview.NewForm()
	form.AddInputField("Summary", summary, 0, nil, nil)
	form.AddInputField("Description", desc, 0, nil, nil)
	form.AddInputField("Location", location, 0, nil, nil)
	form.AddCheckbox("All day", allDay, nil)
	form.AddInputField("Start date (YYYY-MM-DD)", startDate, 12, nil, nil)
	form.AddInputField("Start time (HH:MM)", startTime, 8, nil, nil)
	form.AddInputField("End date (YYYY-MM-DD)", endDate, 12, nil, nil)
	form.AddInputField("End time (HH:MM)", endTime, 8, nil, nil)
	styleBWForm(form)
	return form
}

func (a *app) readEventDraft(form *tview.Form) (model.EventDraft, error) {
	allDay := form.GetFormItemByLabel("All day").(*tview.Checkbox).IsChecked()
	sd, hasSD, err := parseDateField(formText(form, "Start date (YYYY-MM-DD)"), a.loc)
	if err != nil {
		return model.EventDraft{}, errField("Start date", err)
	}
	if !hasSD {
		return model.EventDraft{}, errFieldMsg("Start date is required")
	}

	var start, end time.Time
	if allDay {
		start = sd
		ed, hasED, err := parseDateField(formText(form, "End date (YYYY-MM-DD)"), a.loc)
		if err != nil {
			return model.EventDraft{}, errField("End date", err)
		}
		last := sd
		if hasED {
			last = ed
		}
		end = last.AddDate(0, 0, 1) // DTEND is exclusive
	} else {
		sh, sm, _, err := parseTimeField(formText(form, "Start time (HH:MM)"))
		if err != nil {
			return model.EventDraft{}, errField("Start time", err)
		}
		start = time.Date(sd.Year(), sd.Month(), sd.Day(), sh, sm, 0, 0, a.loc)
		ed, hasED, err := parseDateField(formText(form, "End date (YYYY-MM-DD)"), a.loc)
		if err != nil {
			return model.EventDraft{}, errField("End date", err)
		}
		eh, em, _, err := parseTimeField(formText(form, "End time (HH:MM)"))
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
		Summary:     formText(form, "Summary"),
		Description: formText(form, "Description"),
		Location:    formText(form, "Location"),
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
	form := a.newEventForm(ev, ev.Start)
	form.AddButton("Save", func() {
		d, err := a.readEventDraft(form)
		if err != nil {
			a.flash(err.Error())
			return
		}
		newObj, err := model.EditEvent(loc.Object, uid, d, a.now, a.loc)
		if err != nil {
			a.flash(err.Error())
			return
		}
		a.commitMutation(loc.CalID, loc.Name, newObj, loc.Prev, "edit event", uid, "Saved")
	})
	form.AddButton("Cancel", func() { a.closeModal(pageForm) })
	form.SetCancelFunc(func() { a.closeModal(pageForm) })
	form.SetBorder(true).SetTitle(" Edit event ")
	a.openModal(pageForm, form, 62, 21)
}

func (a *app) showCreateEventForm(calID string, base time.Time) {
	form := a.newEventForm(nil, base)
	form.AddButton("Create", func() {
		d, err := a.readEventDraft(form)
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
			a.flash("Add failed: " + err.Error())
			return
		}
		uid := obj.Events[0].UID
		a.commitMutation(calID, store.ResourceName(uid), obj, nil, "add event", uid, "Added event")
	})
	form.AddButton("Cancel", func() { a.closeModal(pageForm) })
	form.SetCancelFunc(func() { a.closeModal(pageForm) })
	form.SetBorder(true).SetTitle(" New event ")
	a.openModal(pageForm, form, 62, 21)
}

// commitMutation writes obj, records undo, closes the form, refreshes, and
// flashes — the shared tail of every form Save/Create. prev nil marks a creation.
func (a *app) commitMutation(calID, name string, obj *model.Parsed, prev *store.Resource, label, selUID, done string) {
	if _, err := a.store.Put(context.Background(), calID, name, obj); err != nil {
		a.flash("Save failed: " + err.Error())
		return
	}
	a.pushUndo(label, selUID, undoOp{calID: calID, name: name, prev: prev})
	// Refresh first (rebuilds the calendar grid), then close — so restoreFocus
	// can re-drill into the day the user was on.
	a.refresh(selUID)
	a.closeModal(pageForm)
	a.flash(done)
}

// --- undo (u) ---

func (a *app) undoLast() {
	if len(a.undo) == 0 {
		a.flash("Nothing to undo")
		return
	}
	step := a.undo[len(a.undo)-1]
	a.undo = a.undo[:len(a.undo)-1]

	ctx := context.Background()
	for _, op := range step.ops {
		var err error
		if op.prev == nil {
			err = a.store.Delete(ctx, op.calID, op.name) // undo a creation
		} else {
			_, err = a.store.Restore(ctx, op.calID, op.name, op.prev)
		}
		if err != nil {
			a.flash("Undo failed: " + err.Error())
			return
		}
	}
	a.refresh(step.selUID)
	a.flash("Undid " + step.label)
}

// --- refresh after a mutation ---

// refresh rebuilds the overview panels and the active center pane after a data
// change, preserving the left-panel selections and reselecting selUID in the
// task tree when possible.
func (a *app) refresh(selUID string) {
	tlIdx := a.tasklists.GetCurrentItem()
	calIdx := a.calendars.GetCurrentItem()
	agIdx := a.agendaList.GetCurrentItem()

	a.buildCalendars()
	a.buildTasklists()
	a.buildAgendaLeft()

	restoreListIndex(a.calendars, calIdx)
	restoreListIndex(a.tasklists, tlIdx)
	restoreListIndex(a.agendaList, agIdx)

	switch a.mode {
	case modeCalendar:
		a.buildCenterCalendar()
	case modeTasks:
		a.buildTree()
		if selUID != "" {
			a.selectTreeByUID(selUID)
		}
	case modeAgenda:
		a.buildAgendaCenter()
	}
	a.updateStatus()
}

func restoreListIndex(list *tview.List, idx int) {
	if n := list.GetItemCount(); n > 0 {
		if idx < 0 {
			idx = 0
		}
		if idx >= n {
			idx = n - 1
		}
		list.SetCurrentItem(idx)
	}
}

func (a *app) selectTreeByUID(uid string) {
	root := a.tree.GetRoot()
	if root == nil {
		return
	}
	if node := findTreeNode(root, uid); node != nil {
		a.tree.SetCurrentNode(node)
	}
}

func findTreeNode(node *tview.TreeNode, uid string) *tview.TreeNode {
	if t, ok := node.GetReference().(*model.Todo); ok && t.UID == uid {
		return node
	}
	for _, c := range node.GetChildren() {
		if found := findTreeNode(c, uid); found != nil {
			return found
		}
	}
	return nil
}

// --- modal plumbing ---

// modalOpen reports whether a modal overlay is in front of the main layout.
func (a *app) modalOpen() bool {
	if a.root == nil {
		return false
	}
	name, _ := a.root.GetFrontPage()
	return name != pageMain
}

func (a *app) openModal(name string, prim tview.Primitive, width, height int) {
	a.captureFocus()
	a.root.AddPage(name, modalWrap(prim, width, height), true, true)
	a.tv.SetFocus(prim)
}

func (a *app) closeModal(name string) {
	a.root.RemovePage(name)
	a.restoreFocus()
}

// captureFocus records the focused primitive (and any calendar drill-in state)
// before a modal opens.
func (a *app) captureFocus() {
	a.savedFocus = focusState{prim: a.tv.GetFocus()}
	if a.mode == modeCalendar {
		if g, ok := a.calendarPrimitive().(calGrid); ok {
			a.savedFocus.calDay, a.savedFocus.calEvent, a.savedFocus.calIndex = g.drillState()
		}
	}
}

// restoreFocus returns focus to where it was before the modal, re-entering the
// calendar's event-cycling on the same day when that's where the user was.
func (a *app) restoreFocus() {
	fs := a.savedFocus
	if fs.prim == nil {
		a.setFocus(a.focusForMode())
		return
	}
	if fs.calEvent && a.mode == modeCalendar {
		if g, ok := a.calendarPrimitive().(calGrid); ok {
			g.reDrill(fs.calDay, fs.calIndex)
			a.setFocus(a.calendarPrimitive())
			return
		}
	}
	a.setFocus(fs.prim)
}

func clampIndex(i, n int) int {
	if i < 0 {
		return 0
	}
	if i >= n {
		return n - 1
	}
	return i
}

// focusForMode is the fallback pane focus after a modal closes.
func (a *app) focusForMode() tview.Primitive {
	switch a.mode {
	case modeTasks:
		return a.tree
	case modeAgenda:
		return a.agendaList
	default:
		return a.calendars
	}
}

// promptInput shows a one-line input modal; onAccept runs with the entered text
// on Enter, and Esc cancels.
func (a *app) promptInput(title, label string, onAccept func(text string)) {
	in := tview.NewInputField().SetLabel(label)
	in.SetBorder(true)
	in.SetTitle(" " + title + " ")
	in.SetDoneFunc(func(key tcell.Key) {
		switch key {
		case tcell.KeyEnter:
			text := in.GetText()
			a.closeModal(pageInput)
			onAccept(text)
		case tcell.KeyEscape:
			a.closeModal(pageInput)
		}
	})
	a.openModal(pageInput, in, 64, 3)
}

// confirm shows a yes/no modal; onYes runs when the user confirms.
func (a *app) confirm(text string, onYes func()) {
	modal := tview.NewModal().
		SetText(text).
		AddButtons([]string{"Delete", "Cancel"}).
		SetDoneFunc(func(_ int, label string) {
			a.closeModal(pageConfirm)
			if label == "Delete" {
				onYes()
			}
		})
	// Monochrome: white card, black text, black buttons that invert when focused.
	modal.SetBackgroundColor(tcell.ColorWhite)
	modal.SetTextColor(tcell.ColorBlack)
	modal.SetButtonBackgroundColor(tcell.ColorBlack)
	modal.SetButtonTextColor(tcell.ColorWhite)
	modal.SetButtonActivatedStyle(tcell.StyleDefault.Background(tcell.ColorWhite).Foreground(tcell.ColorBlack))
	a.captureFocus()
	a.root.AddPage(pageConfirm, modal, true, true)
	a.tv.SetFocus(modal)
}

// styleBWForm gives a form the monochrome look: a white card with black labels
// and black input boxes (white text). Note: tview applies one field style to
// every field each frame, so a per-field "white when focused" invert isn't
// possible without a custom form — the black boxes on a white card read clearly.
func styleBWForm(form *tview.Form) {
	form.SetBackgroundColor(tcell.ColorWhite)
	form.SetLabelColor(tcell.ColorBlack)
	form.SetFieldBackgroundColor(tcell.ColorBlack)
	form.SetFieldTextColor(tcell.ColorWhite)
	form.SetButtonBackgroundColor(tcell.ColorBlack)
	form.SetButtonTextColor(tcell.ColorWhite)
	form.SetButtonActivatedStyle(tcell.StyleDefault.Background(tcell.ColorWhite).Foreground(tcell.ColorBlack))
	form.SetBorderColor(tcell.ColorBlack)
	form.SetTitleColor(tcell.ColorBlack)
}

// modalWrap centers prim in a transparent full-screen flex so the main layout
// stays visible behind it.
func modalWrap(prim tview.Primitive, width, height int) tview.Primitive {
	return tview.NewFlex().
		AddItem(nil, 0, 1, false).
		AddItem(tview.NewFlex().SetDirection(tview.FlexRow).
			AddItem(nil, 0, 1, false).
			AddItem(prim, height, 0, true).
			AddItem(nil, 0, 1, false), width, 0, true).
		AddItem(nil, 0, 1, false)
}

// flash shows a transient result/status message in the left section of the
// status bar; it persists until the next updateStatus (i.e. the next action).
func (a *app) flash(msg string) { a.statusLeft.SetText(msg) }

// --- small helpers ---

// priorityOptions is the Priority dropdown; the option index is the iCal
// priority value (0 = none, 1 highest .. 9 lowest).
var priorityOptions = []string{"none", "1", "2", "3", "4", "5", "6", "7", "8", "9"}

func findTodo(obj *model.Parsed, uid string) *model.Todo {
	for _, t := range obj.Todos {
		if t.UID == uid {
			return t
		}
	}
	return nil
}

func findEvent(obj *model.Parsed, uid string) *model.Event {
	for _, e := range obj.Events {
		if e.UID == uid {
			return e
		}
	}
	return nil
}

func summaryOf(obj *model.Parsed, uid string) string {
	if t := findTodo(obj, uid); t != nil {
		return nonEmpty(t.Summary, "(untitled)")
	}
	if e := findEvent(obj, uid); e != nil {
		return nonEmpty(e.Summary, "(untitled)")
	}
	return "item"
}

func formText(form *tview.Form, label string) string {
	if item := form.GetFormItemByLabel(label); item != nil {
		if in, ok := item.(*tview.InputField); ok {
			return in.GetText()
		}
	}
	return ""
}

func splitTags(s string) []string {
	var out []string
	for _, part := range strings.Split(s, ",") {
		if p := strings.TrimSpace(part); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// parseDateField parses a YYYY-MM-DD field; a blank value yields ok=false.
func parseDateField(s string, loc *time.Location) (time.Time, bool, error) {
	if strings.TrimSpace(s) == "" {
		return time.Time{}, false, nil
	}
	t, err := time.ParseInLocation("2006-01-02", strings.TrimSpace(s), loc)
	if err != nil {
		return time.Time{}, false, errDateFormat
	}
	return t, true, nil
}

// parseTimeField parses HH:MM (24h) or h:mmam/pm; a blank value yields ok=false.
func parseTimeField(s string) (hour, minute int, ok bool, err error) {
	s = strings.ToLower(strings.TrimSpace(s))
	if s == "" {
		return 0, 0, false, nil
	}
	for _, layout := range []string{"15:04", "3:04pm", "3pm"} {
		if t, e := time.Parse(layout, s); e == nil {
			return t.Hour(), t.Minute(), true, nil
		}
	}
	return 0, 0, false, errTimeFormat
}
