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
		if a.viewMode != viewMonth {
			if it := a.timegrid.selectedItem(); it != nil {
				return targetFromItem(*it), true
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

// addTaskQuick (at): quick-add a top-level task in the selected list.
func (a *app) addTaskQuick() {
	calID, ok := a.taskCreateContext()
	if !ok {
		return
	}
	a.promptInput("New task", "Task: ", func(text string) { a.createTask(calID, "", text) })
}

// addTaskFull (aT): full create form for a top-level task.
func (a *app) addTaskFull() {
	calID, ok := a.taskCreateContext()
	if !ok {
		return
	}
	a.showCreateTodoForm(calID, "")
}

// addEventQuick (ae): quick-add an event on the selected/current day.
func (a *app) addEventQuick() {
	calID, base, ok := a.eventCreateContext()
	if !ok {
		return
	}
	a.promptInput("New event", "Event: ", func(text string) { a.createEvent(calID, base, text) })
}

// addEventFull (aE): full create form for an event.
func (a *app) addEventFull() {
	calID, base, ok := a.eventCreateContext()
	if !ok {
		return
	}
	a.showCreateEventForm(calID, base)
}

// taskCreateContext resolves the writable target list for a new top-level task.
func (a *app) taskCreateContext() (string, bool) {
	calID := a.selectedTasklistID()
	if calID == "" {
		a.flash("Select a task list first (press t)")
		return "", false
	}
	if !a.guardWrite(calID) || !a.guardComponent(calID, compTodo) {
		return "", false
	}
	return calID, true
}

// addSubtaskQuick (is): quick-add a subtask under the selected task (in any pane).
func (a *app) addSubtaskQuick() {
	calID, parentUID, ok := a.subtaskContext()
	if !ok {
		return
	}
	a.promptInput("New subtask", "Subtask: ", func(text string) { a.createTask(calID, parentUID, text) })
}

// addSubtaskFull (iS): full create form for a subtask under the selected task.
func (a *app) addSubtaskFull() {
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
	if !a.guardWrite(calID) || !a.guardComponent(calID, compEvent) {
		return "", time.Time{}, false
	}
	base = a.anchor
	if a.mode == modeAgenda {
		base = model.DayStart(a.now)
	}
	return calID, base, true
}

// subtaskContext resolves the parent for a new subtask: the selected task in any
// pane (tree, calendar drill, or agenda). The subtask is created in the parent's
// own calendar — never the Tasks-overview highlight — since RELATED-TO parent and
// child must live in the same collection.
func (a *app) subtaskContext() (calID, parentUID string, ok bool) {
	t, targeted := a.currentTarget()
	if !targeted || !t.isTodo {
		a.flash("Select a task to add a subtask under")
		return "", "", false
	}
	loc, found := a.store.Locate(t.uid)
	if !found {
		a.flash("Task not found")
		return "", "", false
	}
	if !a.guardWrite(loc.CalID) {
		return "", "", false
	}
	return loc.CalID, t.uid, true
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
	if !a.guardWrite(loc.CalID) {
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
	// Keep a just-completed task visible until the view is left, in any view now
	// that the calendar/agenda also honor the `.` toggle — otherwise completing a
	// task there while completed are hidden would make it vanish instantly.
	// stickyDone clears on switching list (buildTree changed-func) or pane (setMode).
	if completing && !a.showCompleted {
		a.stickyDone[t.uid] = true
	}
	a.pushUndo("toggle done", t.uid, undoOp{calID: loc.CalID, name: loc.Name, prev: loc.Prev})
	// Completing a drilled task in a calendar view must not undrill the day: the
	// rebuild resets the grid's event-cycling, so re-enter it on the same day/index.
	a.refreshKeepingDrill(t.uid)
}

// refreshKeepingDrill rebuilds the views like refresh, but preserves the calendar
// grid's drill-in (event-cycling) across the rebuild — so a direct mutation
// (e.g. Space to complete) doesn't kick the user back out to day navigation. The
// modal create/edit paths handle this via captureFocus/restoreFocus instead.
func (a *app) refreshKeepingDrill(selUID string) {
	if a.mode != modeCalendar {
		a.refresh(selUID)
		return
	}
	g, ok := a.calendarPrimitive().(calGrid)
	if !ok {
		a.refresh(selUID)
		return
	}
	day, drilled, idx := g.drillState()
	a.refresh(selUID)
	if drilled {
		if g2, ok := a.calendarPrimitive().(calGrid); ok {
			g2.reDrill(day, idx)
			a.setFocus(a.calendarPrimitive())
		}
	}
}

// hasIncompleteChildren reports whether uid is a "folder" — has any incomplete
// child. It shares folderSet's definition (one source of truth) so the folder
// caret shown in the views can never disagree with the completion guard that uses
// this. Computed fresh from the store since it's called on completion, not a draw.
func (a *app) hasIncompleteChildren(uid string) bool {
	return folderSet(a.store.Todos())[uid]
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
	if !a.guardWrite(loc.CalID) {
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
	// Read parent / previous-sibling from the tree actually on screen so H/L
	// always match what the user sees (folders, sticky-completed rows included),
	// rather than a separately-rebuilt forest that can drift from the rendering.
	parentNode, idx := treeNodeContext(a.tree.GetRoot(), node)
	if parentNode == nil {
		return
	}

	var newParent string
	switch dir {
	case indent:
		if idx <= 0 {
			a.flash("Can't indent: no task above at this level")
			return
		}
		prev, ok := parentNode.GetChildren()[idx-1].GetReference().(*model.Todo)
		if !ok {
			a.flash("Can't indent: no task above at this level")
			return
		}
		newParent = prev.UID
	case outdent:
		// If the parent node is the list root (not a task), we're already at the
		// top level; otherwise the new parent is the current parent's parent.
		pt, ok := parentNode.GetReference().(*model.Todo)
		if !ok {
			a.flash("Already at the top level")
			return
		}
		newParent = pt.ParentUID // grandparent, or "" to become a root
	}

	loc, ok := a.store.Locate(td.UID)
	if !ok {
		a.flash("Task not found")
		return
	}
	if !a.guardWrite(loc.CalID) {
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

// treeNodeContext finds target within the tview tree, returning its parent node
// and its index among that parent's children (parent nil / idx -1 if not found).
// It reads the tree exactly as displayed, so H/L stay consistent with what's on
// screen regardless of how buildTree filters/renders.
func treeNodeContext(root, target *tview.TreeNode) (parent *tview.TreeNode, idx int) {
	if root == nil {
		return nil, -1
	}
	for i, child := range root.GetChildren() {
		if child == target {
			return root, i
		}
		if p, j := treeNodeContext(child, target); p != nil {
			return p, j
		}
	}
	return nil, -1
}

// --- edit (e) ---

func (a *app) editSelected() {
	// When an overview list is focused, `e` edits that collection's name + color —
	// symmetric with delete (deleteContextual): the Calendars pane edits the
	// highlighted calendar, the Tasks pane the highlighted list (both are calendars,
	// so both open showCalendarForm). Checked before currentTarget because in
	// modeTasks currentTarget always returns the tree node, even when the Tasks
	// panel (not the tree) holds focus.
	switch a.tv.GetFocus() {
	case a.calendars:
		if id := a.currentCalendarID(); id != "" {
			a.showCalendarForm(id, 0)
			return
		}
	case a.tasklists:
		if id := a.selectedTasklistID(); id != "" {
			a.showCalendarForm(id, 0)
			return
		}
	}
	t, ok := a.currentTarget()
	if !ok {
		// In Calendar mode with the grid focused but no item drilled, `e` still edits
		// the highlighted calendar (a convenience shortcut from the grid).
		if a.mode == modeCalendar {
			if id := a.currentCalendarID(); id != "" {
				a.showCalendarForm(id, 0)
				return
			}
		}
		a.flash("Nothing selected to edit")
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
		a.showTodoForm(loc, t.uid)
	} else {
		a.showEventForm(loc, t.uid)
	}
}

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
	f, fields := a.newTodoForm(td)
	f.AddButton("Save", func() {
		d, err := a.readTodoDraft(fields)
		if err != nil {
			a.flash(err.Error())
			return
		}
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
			a.flash(err.Error())
			return
		}
		a.commitMutation(loc.CalID, loc.Name, newObj, loc.Prev, "edit task", uid, "Saved")
	})
	f.AddButton("Cancel", func() { a.closeModal(pageForm) })
	f.SetCancelFunc(func() { a.closeModal(pageForm) })
	f.SetBorder(true).SetTitle(" Edit task ")
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
	f, fields := a.newEventForm(ev, ev.Start)
	f.AddButton("Save", func() {
		d, err := a.readEventDraft(fields)
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
	f.AddButton("Cancel", func() { a.closeModal(pageForm) })
	f.SetCancelFunc(func() { a.closeModal(pageForm) })
	f.SetBorder(true).SetTitle(" Edit event ")
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
			a.flash("Add failed: " + err.Error())
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
	fs := focusState{prim: a.tv.GetFocus()}
	if a.mode == modeCalendar {
		if g, ok := a.calendarPrimitive().(calGrid); ok {
			fs.calDay, fs.calEvent, fs.calIndex = g.drillState()
		}
	}
	a.focusStack = append(a.focusStack, fs)
}

// restoreFocus returns focus to where it was before the most recent modal
// opened (popping the focus stack so nested modals unwind correctly),
// re-entering the calendar's event-cycling on the same day when that's where the
// user was.
func (a *app) restoreFocus() {
	if len(a.focusStack) == 0 {
		a.setFocus(a.focusForMode())
		return
	}
	fs := a.focusStack[len(a.focusStack)-1]
	a.focusStack = a.focusStack[:len(a.focusStack)-1]
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
	// Shared popup look: terminal-default (unified) background, high-contrast
	// default text, accent rounded border/title.
	in.SetFieldBackgroundColor(tcell.ColorDefault)
	in.SetFieldTextColor(tcell.ColorDefault)
	in.SetLabelColor(tcell.ColorDefault)
	in.SetBackgroundColor(tcell.ColorDefault)
	in.SetBorder(true)
	in.SetBorderColor(accentColor)
	in.SetTitleColor(accentColor)
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
	// Shared popup look: terminal-default background, default text, the focused
	// button reversed.
	modal.SetBackgroundColor(tcell.ColorDefault)
	modal.SetTextColor(tcell.ColorDefault)
	modal.SetButtonBackgroundColor(tcell.ColorDefault)
	modal.SetButtonTextColor(tcell.ColorDefault)
	modal.SetButtonActivatedStyle(tcell.StyleDefault.Reverse(true))
	a.captureFocus()
	a.root.AddPage(pageConfirm, modal, true, true)
	a.tv.SetFocus(modal)
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
