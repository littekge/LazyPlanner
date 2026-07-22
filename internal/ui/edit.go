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
	// Every local mutation pushes an undo step, so this is the single signal to
	// schedule the debounced background push (offline → no-op).
	a.scheduleSyncDebounced()
}

// editTarget is the item the editing shortcuts act on in the current context.
// For a recurring item, occStart is the specific occurrence's instant (the
// RECURRENCE-ID target) and recurring is true, so the editing keys can offer the
// this/future/all scope picker.
type editTarget struct {
	isTodo    bool
	uid       string
	occStart  time.Time
	allDay    bool
	recurring bool
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
				return editTarget{isTodo: true, uid: t.UID, occStart: t.Due, allDay: t.DueAllDay, recurring: t.Recurring}, true
			}
		}
	case modeCalendar:
		// Both grids expose the drilled item via calGrid.selectedItem, so month and
		// week/day are read the same way (no per-view drill shape here).
		if g, ok := a.calendarPrimitive().(calGrid); ok {
			if it := g.selectedItem(); it != nil {
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
		return editTarget{isTodo: true, uid: it.Todo.UID, occStart: it.Start, allDay: it.AllDay, recurring: it.Todo.Recurring}
	}
	return editTarget{isTodo: false, uid: it.Event.UID, occStart: it.Start, allDay: it.AllDay, recurring: it.Event.Recurring}
}

// --- quick add (a) ---

// addTaskQuick (at): quick-add a top-level task in the selected list.
func (a *app) addTaskQuick() {
	calID, ok := a.taskCreateContext()
	if !ok {
		return
	}
	a.promptQuickAdd("New task", "Task: ", func(text string) { a.createTask(calID, "", text) })
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
	a.promptQuickAdd("New event", "Event: ", func(text string) { a.createEvent(calID, base, text) })
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
	a.promptQuickAdd("New subtask", "Subtask: ", func(text string) { a.createTask(calID, parentUID, text) })
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
	// No guardComponent here: the parent is itself a VTODO living in loc.CalID, so
	// the collection provably accepts tasks. The RELATED-TO invariant (child shares
	// the parent's collection) makes a component re-check redundant.
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
		Recur:      qa.Recur,
		Location:   qa.Location,
	}
	if qa.HasDate || qa.HasTime {
		when, allDay := qa.At(model.DayStart(a.now), a.loc)
		d.HasDue, d.Due, d.DueAllDay = true, when, allDay
	}
	obj := model.NewTodoObject(d, a.now)
	uid := obj.Todos[0].UID
	name := store.ResourceName(uid)
	if _, err := a.store.Put(context.Background(), calID, name, obj); err != nil {
		a.flashErr("Add", err)
		return
	}
	a.pushUndo("add task", uid, undoOp{calID: calID, name: name})
	a.refresh(uid)
	a.flash("Added task" + undoHint)
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
	end := start.Add(time.Hour) // default duration when no end is given
	switch {
	case allDay:
		end = start.AddDate(0, 0, 1)
	case qa.HasEnd:
		end = qa.EndAt(start)
	}
	obj, err := model.NewEventObject(model.EventDraft{
		Summary:  qa.Title,
		Start:    start,
		End:      end,
		AllDay:   allDay,
		Recur:    qa.Recur,
		Location: qa.Location,
	}, a.now)
	if err != nil {
		a.flashErr("Add", err)
		return
	}
	uid := obj.Events[0].UID
	name := store.ResourceName(uid)
	if _, err := a.store.Put(context.Background(), calID, name, obj); err != nil {
		a.flashErr("Add", err)
		return
	}
	a.pushUndo("add event", uid, undoOp{calID: calID, name: name})
	a.refresh(uid)
	a.flash("Added event" + undoHint)
}

// --- complete toggle (Space) ---

func (a *app) toggleComplete() {
	// Every path that presses Space must get feedback, not a silent no-op: in
	// Agenda/Tasks views Space routes straight here (Calendar mode pre-empts the
	// event and no-selection cases in its own switch before calling in).
	t, ok := a.currentTarget()
	if !ok {
		a.flash("Select a task first")
		return
	}
	if !t.isTodo {
		a.flash("Can't complete an event")
		return
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
	// A recurring todo advances to its next occurrence on completion (NextCloud
	// style) rather than being marked done, until the series is exhausted.
	if !td.Completed() && td.Recurring {
		a.advanceRecurringTodo(loc, t.uid)
		return
	}
	completing := !td.Completed()
	newObj, err := model.SetTodoCompleted(loc.Object, t.uid, completing, a.now, a.loc)
	if err != nil {
		a.flashErr("Complete", err)
		return
	}
	// Version-checked write (newObj derived from loc's snapshot): a background sync
	// pull landing since the Locate above must not be clobbered by the toggle.
	applied, err := a.store.PutIfUnchanged(context.Background(), loc.CalID, loc.Name, newObj, loc.Prev)
	if err != nil {
		a.flashErr("Complete", err)
		return
	}
	if !applied {
		a.refreshKeepingDrill(t.uid)
		a.flash("Task changed on the server — not applied; retry")
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
	if completing {
		a.flash("Completed" + undoHint)
	} else {
		a.flash("Reopened" + undoHint)
	}
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
	// A recurring item offers the this/future/all scope picker before deleting.
	if t.recurring {
		a.deleteRecurring(loc, t)
		return
	}
	a.deleteWholeObject(loc, t.uid)
}

// deleteWholeObject removes the item's resource (and, for a task, its whole
// subtree) after a confirm — the "delete all occurrences" path for a series and
// the only path for a non-recurring item.
func (a *app) deleteWholeObject(loc store.Located, uid string) {
	what := summaryOf(loc.Object, uid)

	// A task's subtree is deleted with it (deleting a folder is recursive).
	kids := a.descendants(uid)
	prompt := "Delete \"" + oneLine(what) + "\"?"
	if n := len(kids); n > 0 {
		prompt = "Delete \"" + oneLine(what) + "\" and its " + strconv.Itoa(n) + " subtask(s)?"
	}

	a.confirm(prompt, func() {
		var ops []undoOp
		for _, u := range append([]string{uid}, kids...) {
			l, ok := a.store.Locate(u)
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
		a.flashErr("Move", err)
		return
	}
	// Version-checked against loc.Prev: a background sync pull landing between the
	// Locate above and this write must not be clobbered (the write would otherwise
	// adopt the pulled ETag while persisting content derived from the now-stale
	// snapshot, and the next push's CAS would overwrite the server edit).
	applied, err := a.store.PutIfUnchanged(context.Background(), loc.CalID, loc.Name, newObj, loc.Prev)
	if err != nil {
		a.flash("Move failed: " + err.Error())
		return
	}
	if !applied {
		a.refresh(td.UID)
		a.flash("Task changed on the server — move not applied; retry")
		return
	}
	a.pushUndo("re-parent", td.UID, undoOp{calID: loc.CalID, name: loc.Name, prev: loc.Prev})
	a.refresh(td.UID)
	a.flash("Moved task" + undoHint)
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
	// A recurring item offers the this/future/all scope picker before editing.
	if t.recurring {
		a.editRecurring(loc, t)
		return
	}
	if t.isTodo {
		a.showTodoForm(loc, t.uid)
	} else {
		a.showEventForm(loc, t.uid)
	}
}

// staleWriteMsg is flashed when a version-checked edit is skipped because the
// resource changed on the server (a concurrent background pull) after it was
// located — the edit is not applied rather than clobbering the pulled change.
const staleWriteMsg = "Changed on the server — not applied; reopen and retry"

// applyMutation writes obj and records an undo step. For an edit (prev != nil) the
// write is version-checked against prev via PutIfUnchanged: if the resource
// changed on the server since it was located (a concurrent pull), the write is
// skipped so the pulled edit isn't clobbered, and stale=true is returned. A
// creation (prev == nil) has no prior version to guard and always writes. Returns
// ok=false on a write error or a stale skip; flashes on a hard error only.
func (a *app) applyMutation(calID, name string, obj *model.Parsed, prev *store.Resource, label, selUID string) (ok, stale bool) {
	if prev != nil {
		applied, err := a.store.PutIfUnchanged(context.Background(), calID, name, obj, prev)
		if err != nil {
			a.flashErr("Save", err)
			return false, false
		}
		if !applied {
			return false, true
		}
	} else if _, err := a.store.Put(context.Background(), calID, name, obj); err != nil {
		a.flashErr("Save", err)
		return false, false
	}
	a.pushUndo(label, selUID, undoOp{calID: calID, name: name, prev: prev})
	return true, false
}

// commitMutation writes obj, records undo, closes the form, refreshes, and
// flashes — the shared tail of every form Save/Create. prev nil marks a creation.
func (a *app) commitMutation(calID, name string, obj *model.Parsed, prev *store.Resource, label, selUID, done string) {
	ok, stale := a.applyMutation(calID, name, obj, prev, label, selUID)
	if !ok {
		if stale {
			// The resource changed underneath: show the server's version and tell
			// the user, instead of leaving the form open over stale content.
			a.refresh(selUID)
			a.closeModal(pageForm)
			a.flash(staleWriteMsg)
		}
		return
	}
	// Refresh first (rebuilds the calendar grid), then close — so restoreFocus
	// can re-drill into the day the user was on.
	a.refresh(selUID)
	a.closeModal(pageForm)
	a.flash(done + undoHint)
}

// commitMutationKeepingDrill is the commit tail for a mutation triggered from the
// recurrence scope picker (a pageConfirm that already closed) rather than a form.
// It must NOT close pageForm — no form is open, so that extra restoreFocus would
// pop an empty focus stack and kick focus off the drilled calendar day. It keeps
// the drill instead, mirroring Space-complete.
func (a *app) commitMutationKeepingDrill(calID, name string, obj *model.Parsed, prev *store.Resource, label, selUID, done string) {
	ok, stale := a.applyMutation(calID, name, obj, prev, label, selUID)
	if !ok {
		if stale {
			a.refreshKeepingDrill(selUID)
			a.flash(staleWriteMsg)
		}
		return
	}
	a.refreshKeepingDrill(selUID)
	a.flash(done + undoHint)
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
			// RestoreDirty (not Restore): an undo is a fresh local change to push. A
			// clean replay of a snapshot whose edit/delete already synced would be
			// silently pulled back over / Forgotten on the next reconcile.
			_, err = a.store.RestoreDirty(ctx, op.calID, op.name, op.prev)
		}
		if err != nil {
			a.flash("Undo failed: " + err.Error())
			return
		}
	}
	a.refresh(step.selUID)
	// Undo is itself a local change to push (it doesn't call pushUndo).
	a.scheduleSyncDebounced()
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
		// With no explicit selUID (a background sync's refresh("")), preserve an
		// active day-drill across the rebuild — buildCenterCalendar → setData resets
		// eventMode/eventIndex, so without this a sync kicks the user out of a day's
		// event-cycling back to day navigation. A mutation that passes selUID uses
		// refreshKeepingDrill, which captures the drill and also restores focus.
		var drillDay time.Time
		var drilled bool
		var drillIdx int
		if selUID == "" {
			if g, ok := a.calendarPrimitive().(calGrid); ok {
				drillDay, drilled, drillIdx = g.drillState()
			}
		}
		a.buildCenterCalendar()
		if drilled {
			if g, ok := a.calendarPrimitive().(calGrid); ok {
				g.reDrill(drillDay, drillIdx)
			}
		}
	case modeTasks:
		// An explicit selUID (a mutation that knows what to reselect) wins;
		// otherwise keep the current highlight so a background sync's refresh("")
		// doesn't yank the cursor back to the first task on every sync.
		keepUID := selUID
		if keepUID == "" {
			keepUID = a.currentTreeUID()
		}
		a.buildTree()
		if keepUID != "" {
			a.selectTreeByUID(keepUID)
		}
	case modeAgenda:
		a.buildAgendaCenter()
	}
	a.updateStatus()
}

// currentTreeUID returns the UID of the task currently highlighted in the tree,
// or "" when the current node is not a task (the list root, or an empty list).
func (a *app) currentTreeUID() string {
	node := a.tree.GetCurrentNode()
	if node == nil {
		return ""
	}
	if td, ok := node.GetReference().(*model.Todo); ok {
		return td.UID
	}
	return ""
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
	// A debounced push that fell due while the form was open was deferred
	// (fireDebouncedSync); re-arm it now so a still-pending local edit syncs
	// promptly rather than waiting for the next periodic tick.
	if !a.modalOpen() && a.store != nil && a.store.HasPendingChanges() {
		a.scheduleSyncDebounced()
	}
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

// quickAddShouldReprompt reports whether a quick-add submit should keep the
// input open (showing a warning) rather than create: true when the parse
// produced warnings and this is not an identical resubmit of the already-warned
// text. Editing the text (text != warnedText) always re-prompts if it still
// warns; resubmitting the same warned text accepts it as-is.
func quickAddShouldReprompt(warnings []string, text, warnedText string, haveWarned bool) bool {
	return len(warnings) > 0 && !(haveWarned && text == warnedText)
}

// promptQuickAdd is promptInput specialised for the quick-add creators: on Enter
// it parses the text and, if the parse produced obvious-error warnings, keeps
// the input open showing the first warning instead of creating anything. Editing
// the text re-parses fresh; submitting the *identical* warned text again accepts
// it as-is (the failed tokens stay in the title), matching main.md's keep-open
// re-prompt. create is the same callback used elsewhere (it re-parses).
func (a *app) promptQuickAdd(title, label string, create func(text string)) {
	in := tview.NewInputField().SetLabel(label)
	in.SetFieldBackgroundColor(tcell.ColorDefault)
	in.SetFieldTextColor(tcell.ColorDefault)
	in.SetLabelColor(tcell.ColorDefault)
	in.SetBackgroundColor(tcell.ColorDefault)
	in.SetBorder(true)
	in.SetBorderColor(accentColor)
	in.SetTitleColor(accentColor)
	in.SetTitle(" " + title + " ")

	var warnedText string
	haveWarned := false
	in.SetDoneFunc(func(key tcell.Key) {
		switch key {
		case tcell.KeyEnter:
			text := in.GetText()
			qa := model.ParseQuickAdd(text, a.now, a.loc)
			if quickAddShouldReprompt(qa.Warnings, text, warnedText, haveWarned) {
				warnedText, haveWarned = text, true
				in.SetTitleColor(warnColor)
				in.SetTitle(" ⚠ " + qa.Warnings[0] + " — Enter to keep, edit to change ")
				return // keep the input open
			}
			a.closeModal(pageInput)
			create(text)
		case tcell.KeyEscape:
			a.closeModal(pageInput)
		}
	})
	a.openModal(pageInput, in, 72, 3)
}

// confirm shows a yes/no modal; onYes runs when the user confirms.
func (a *app) confirm(text string, onYes func()) {
	a.confirmOK(text, "Delete", onYes)
}

// confirmOK is a confirm with a custom affirmative button label (e.g. "Detach"),
// for actions that aren't deletions. Cancel is always the other button.
func (a *app) confirmOK(text, okLabel string, onYes func()) {
	modal := tview.NewModal().
		SetText(text).
		AddButtons([]string{okLabel, "Cancel"}).
		SetDoneFunc(func(_ int, label string) {
			a.closeModal(pageConfirm)
			if label == okLabel {
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

// undoHint is appended to the result flash of every undoable action, so the undo
// affordance is advertised consistently (all these paths call pushUndo).
const undoHint = " (u to undo)"

// flashErr shows a failure the one consistent way: "<Action> failed: <err>". Used
// for store/model mutation failures (field-validation errors stay descriptive).
func (a *app) flashErr(action string, err error) {
	a.flash(action + " failed: " + err.Error())
}

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
