package ui

import (
	"fmt"
	"strings"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"

	"github.com/littekge/LazyPlanner/internal/model"
	"github.com/littekge/LazyPlanner/internal/store"
)

// --- Left overview column ---

func (a *app) buildCalendars() {
	a.rebuildColorIndex()
	a.rebuildFolders()
	a.calendars.Clear()
	for _, cal := range a.store.Calendars() {
		events, todos := calCounts(cal)
		desc := fmt.Sprintf("%s  (%de %dt)", cal.DisplayName, events, todos)
		if m := calTypeMarker(cal); m != "" {
			desc += " " + m
		}
		if cal.ReadOnly {
			desc += " [ro]"
		}
		if a.hidden[cal.ID] {
			desc += " (hidden)"
		}
		// Escape so the bracketed markers ([both]/[ro]/…) render literally rather
		// than being swallowed as tview style tags; prepend a bullet in the
		// calendar's synced color when it has one — but drop the bullet for a
		// hidden calendar, so its items being off the views reads at a glance.
		label := tview.Escape(desc)
		if cc, ok := a.calColors[cal.ID]; ok && !a.hidden[cal.ID] {
			label = "[" + cc.name + "]●[-] " + label
		}
		a.calendars.AddItem(label, "", 0, nil)
	}
	if a.calendars.GetItemCount() == 0 {
		a.calendars.AddItem("(no calendars)", "", 0, nil)
	}
}

func (a *app) buildTasklists() {
	// Suspend the change-callback: Clear/AddItem park the selection at index 0
	// mid-rebuild, which must not be mistaken for the user switching lists.
	a.suspendTree = true
	defer func() { a.suspendTree = false }()

	a.tasklists.Clear()
	a.tasklistIDs = a.tasklistIDs[:0]
	for _, cal := range a.store.Calendars() {
		if !supportsTodos(cal) {
			continue
		}
		label := cal.DisplayName
		if cal.ReadOnly {
			label += " [ro]"
		}
		// Escape so a name (or the [ro] marker) containing [brackets] renders
		// literally instead of being eaten by tview's style-tag parser.
		a.tasklists.AddItem(tview.Escape(label), "", 0, nil)
		a.tasklistIDs = append(a.tasklistIDs, cal.ID)
	}
	if len(a.tasklistIDs) == 0 {
		a.tasklists.AddItem("(no task lists)", "", 0, nil)
	}
}

func (a *app) buildAgendaLeft() {
	a.agendaList.Clear()
	items := a.dayItems(model.DayStart(a.now))
	if len(items) == 0 {
		a.agendaList.AddItem("(nothing today)", "", 0, nil)
		return
	}
	for _, it := range items {
		a.agendaList.AddItem(a.agendaLeftLabel(it), "", 0, nil)
	}
}

func (a *app) selectedTasklistID() string {
	i := a.tasklists.GetCurrentItem()
	if i >= 0 && i < len(a.tasklistIDs) {
		return a.tasklistIDs[i]
	}
	return ""
}

// selectedCalendarID is the id of the highlighted calendar in the Calendars
// overview — the target for new events. The panel lists a.store.Calendars() in
// order, so the current item index maps straight onto that slice.
func (a *app) selectedCalendarID() string {
	cals := a.store.Calendars()
	i := a.calendars.GetCurrentItem()
	if i >= 0 && i < len(cals) {
		return cals[i].ID
	}
	return ""
}

// --- Calendar center (month grid / time-grid) ---

func (a *app) buildCenterCalendar() {
	if a.viewMode == viewMonth {
		weeks := model.MonthGrid(a.anchor, a.weekStartMonday)
		a.month.setData(weeks, a.calItems(weeks), a.anchor.Month(), a.anchor, a.now, a.weekStartMonday)
		a.month.Box.SetTitle(" " + a.anchor.Format("January 2006") + " ")
		a.center.SwitchToPage("month")
		a.setDayDetail(a.anchor)
		return
	}

	var days []time.Time
	var title string
	if a.viewMode == viewWeek {
		days = model.Week(a.anchor, a.weekStartMonday)
		title = " Week of " + days[0].Format("Jan 2, 2006") + " "
	} else {
		days = []time.Time{model.DayStart(a.anchor)}
		title = " " + a.anchor.Format("Monday, Jan 2 2006") + " "
	}
	timed, allday := a.splitOccs(days)
	a.timegrid.setData(days, timed, allday, a.anchor, a.now)
	a.timegrid.dueTasks = a.dueTasksByDay(days)
	a.timegrid.items = a.dayItemsForDays(days)
	a.timegrid.Box.SetTitle(title)
	a.center.SwitchToPage("time")
	a.setDayDetail(a.anchor)
}

// onCalDay handles day navigation from either calendar widget.
func (a *app) onCalDay(day time.Time) {
	a.anchor = day
	if a.viewMode == viewMonth && a.dayInGrid(day) {
		a.month.selected = day
		a.month.eventMode = false
	} else {
		a.buildCenterCalendar()
	}
	a.setDayDetail(day)
	a.updateStatus()
}

func (a *app) dayInGrid(day time.Time) bool {
	if len(a.month.weeks) == 0 {
		return false
	}
	first := a.month.weeks[0][0]
	last := a.month.weeks[len(a.month.weeks)-1][6]
	d := model.DayStart(day)
	return !d.Before(first) && !d.After(last)
}

// calItems builds each visible day's agenda for the month grid, from one query.
func (a *app) calItems(weeks [][]time.Time) map[string][]model.AgendaItem {
	m := map[string][]model.AgendaItem{}
	if len(weeks) == 0 {
		return m
	}
	start := weeks[0][0]
	end := weeks[len(weeks)-1][6].AddDate(0, 0, 1)
	occs, _ := a.store.EventOccurrencesVisible(start, end, a.hidden)
	todos := a.visibleTodos(a.store.TodosVisible(a.hidden))
	for _, week := range weeks {
		for _, day := range week {
			ds := model.DayStart(day)
			if items := model.DayAgenda(model.OccurrencesOn(occs, day), todos, ds, ds.AddDate(0, 0, 1)); len(items) > 0 {
				m[dayKey(day)] = items
			}
		}
	}
	return m
}

// dayItemsForDays builds the per-day drill list for the week/day time-grid: each
// day's events and due tasks in agenda order (all-day first, then by time), so
// the grid's drill can select tasks as well as events.
func (a *app) dayItemsForDays(days []time.Time) map[string][]model.AgendaItem {
	m := map[string][]model.AgendaItem{}
	for _, day := range days {
		if items := a.dayItems(day); len(items) > 0 {
			m[dayKey(day)] = items
		}
	}
	return m
}

// dueTasksByDay buckets tasks with a due date onto the day they're due, for the
// week/day time-grid, keyed like splitOccs. Hidden calendars are excluded (via
// TodosVisible); completed tasks obey the `.` toggle (completedVisible), matching
// the month grid, agenda, and tree.
func (a *app) dueTasksByDay(days []time.Time) map[string][]*model.Todo {
	m := map[string][]*model.Todo{}
	if len(days) == 0 {
		return m
	}
	start := days[0]
	end := days[len(days)-1].AddDate(0, 0, 1)
	for _, t := range a.store.TodosVisible(a.hidden) {
		if !t.HasDue || !a.completedVisible(t) {
			continue
		}
		// A recurring task shows only its current occurrence (it advances on
		// completion rather than painting future occurrences).
		day := model.DayStart(t.Due.In(a.loc))
		if day.Before(start) || !day.Before(end) {
			continue
		}
		m[dayKey(day)] = append(m[dayKey(day)], t)
	}
	return m
}

// splitOccs buckets a range's occurrences into timed and all-day, keyed by day,
// for the time-grid. Both timed and all-day events land on every day they cover;
// drawBlock clips a multi-day timed event to each day's column.
func (a *app) splitOccs(days []time.Time) (timed, allday map[string][]model.Occurrence) {
	timed = map[string][]model.Occurrence{}
	allday = map[string][]model.Occurrence{}
	if len(days) == 0 {
		return
	}
	start := days[0]
	end := days[len(days)-1].AddDate(0, 0, 1)
	occs, _ := a.store.EventOccurrencesVisible(start, end, a.hidden)
	for _, o := range occs {
		if o.Event.AllDay {
			for d := model.DayStart(o.Start); d.Before(o.End); d = d.AddDate(0, 0, 1) {
				if !d.Before(start) && d.Before(end) {
					allday[dayKey(d)] = append(allday[dayKey(d)], o)
				}
			}
			continue
		}
		for _, d := range days {
			if o.OverlapsDay(d) {
				timed[dayKey(d)] = append(timed[dayKey(d)], o)
			}
		}
	}
	return timed, allday
}

// --- Tasks center (tree) ---

func (a *app) buildTree() { a.buildTreeForList(a.selectedTasklistID()) }

// zoomInTree re-roots the task tree at the selected task (`>`), like cd-ing into
// a subtree; `<` (zoomOutTree) pops one level back toward the list root.
func (a *app) zoomInTree() {
	node := a.tree.GetCurrentNode()
	if node == nil {
		a.flash("Select a task to zoom into (>)")
		return
	}
	td, ok := node.GetReference().(*model.Todo)
	if !ok {
		a.flash("Select a task to zoom into (>)")
		return
	}
	a.zoomUID = td.UID
	a.buildTree()
}

// zoomOutTree pops one level out of the task-subtree zoom (`<`) — to the zoomed
// task's parent, or the list root at the top.
func (a *app) zoomOutTree() {
	if a.zoomUID == "" {
		a.flash("Already at the list root")
		return
	}
	if td := a.todoByUID(a.zoomUID); td != nil {
		a.zoomUID = td.ParentUID
	} else {
		a.zoomUID = ""
	}
	a.buildTree()
}

// todoByUID returns the task with the given UID, or nil.
func (a *app) todoByUID(uid string) *model.Todo {
	if uid == "" {
		return nil
	}
	if loc, ok := a.store.Locate(uid); ok {
		return findTodo(loc.Object, uid)
	}
	return nil
}

// findTodoNode locates the node with uid anywhere in the forest.
func findTodoNode(forest []*model.TodoNode, uid string) *model.TodoNode {
	for _, n := range forest {
		if n.Todo.UID == uid {
			return n
		}
		if got := findTodoNode(n.Children, uid); got != nil {
			return got
		}
	}
	return nil
}

// zoomBreadcrumb builds the "List / ancestor / task" path shown as the zoomed
// tree's root, walking up from zoomUID via ParentUID.
func (a *app) zoomBreadcrumb(listName string, all []*model.Todo) string {
	byUID := make(map[string]*model.Todo, len(all))
	for _, t := range all {
		byUID[t.UID] = t
	}
	var chain []string
	seen := map[string]bool{}
	for u := a.zoomUID; u != "" && !seen[u]; {
		seen[u] = true
		td := byUID[u]
		if td == nil {
			break
		}
		chain = append([]string{oneLine(nonEmpty(td.Summary, "(untitled)"))}, chain...)
		u = td.ParentUID
	}
	return listName + " / " + strings.Join(chain, " / ")
}

// buildTreeForList rebuilds the task tree for the given list id. Callers that
// know the target id explicitly (the tasklists changed-callback) must pass it
// rather than relying on selectedTasklistID: tview fires List.changed BEFORE it
// updates the current item, so GetCurrentItem is stale inside that callback and
// would rebuild the previously-selected list's tree.
func (a *app) buildTreeForList(id string) {
	// The root node shows the list's own name, so the tree reads as a file tree
	// rooted at the directory name. (Branch-connector graphics are disabled — see
	// build's SetGraphics(false) — so nesting is shown by indentation + ▸/▾ carets.)
	name := "Tasks"
	root := tview.NewTreeNode("").SetSelectable(false).SetColor(accentColor)

	a.rebuildFolders()
	if id != "" {
		if cal, ok := a.store.Calendar(id); ok {
			name = cal.DisplayName
			var all []*model.Todo
			for _, r := range cal.Resources {
				all = append(all, r.Object.Todos...)
			}

			// Show incomplete tasks, plus completed ones when toggled on or pinned.
			forest := model.BuildTree(a.visibleTodos(all), true)

			// `>`/`<` zoom: re-root the tree at zoomUID (cd-into-a-subtree), showing
			// its children and a breadcrumb; a stale zoom (task gone) resets.
			roots := forest
			if a.zoomUID != "" {
				if zn := findTodoNode(forest, a.zoomUID); zn != nil {
					roots = zn.Children
					name = a.zoomBreadcrumb(cal.DisplayName, all)
				} else {
					a.zoomUID = ""
				}
			}
			for _, n := range roots {
				root.AddChild(a.treeNode(n))
			}
		}
	}
	root.SetText(name)
	a.tree.SetRoot(root)
	if kids := root.GetChildren(); len(kids) > 0 {
		a.tree.SetCurrentNode(kids[0])
	} else {
		a.setDetail("[gray]No tasks in this list.[-]")
	}
}

// rebuildFolders recomputes the global folder set (task UIDs with ≥1 incomplete
// child) across all lists, so the tree, calendar, and agenda all mark folders
// identically. Runs whenever the tree or calendars are rebuilt.
func (a *app) rebuildFolders() { a.folders = folderSet(a.store.Todos()) }

// isFolder reports whether the task with this UID is a folder (has incomplete
// children). Used by the calendar/agenda renderers via a closure.
func (a *app) isFolder(uid string) bool { return a.folders[uid] }

// completedVisible reports whether a task should be shown given the `.` toggle: a
// completed task is hidden unless completed are shown or it's pinned by stickyDone
// (just completed, kept visible until the view is left). The single rule for every
// view, so `.` hides/shows completed tasks identically in the tree, calendar, and
// agenda (it was previously honored only in the tree).
func (a *app) completedVisible(t *model.Todo) bool {
	return a.showCompleted || !t.Completed() || a.stickyDone[t.UID]
}

// visibleTodos applies completedVisible across a slice, for the calendar/agenda
// data builders that feed model.DayAgenda.
func (a *app) visibleTodos(todos []*model.Todo) []*model.Todo {
	if a.showCompleted {
		return todos
	}
	var out []*model.Todo
	for _, t := range todos {
		if a.completedVisible(t) {
			out = append(out, t)
		}
	}
	return out
}

// todoMark is the leading marker for a task line: a ▸ folder caret (task with
// incomplete children, matching the tree), else the [ ]/[■] checkbox.
func todoMark(t *model.Todo, folder bool) string {
	switch {
	case folder:
		return "▸ "
	case t.Completed():
		return "[■] "
	default:
		return "[ ] "
	}
}

// folderSet returns the UIDs that are folders — tasks with at least one
// incomplete child among todos.
func folderSet(todos []*model.Todo) map[string]bool {
	folders := map[string]bool{}
	for _, t := range todos {
		if t.ParentUID != "" && !t.Completed() {
			folders[t.ParentUID] = true
		}
	}
	return folders
}

func (a *app) treeNode(n *model.TodoNode) *tview.TreeNode {
	node := tview.NewTreeNode(a.nodeLabel(n.Todo, true)).SetReference(n.Todo).SetExpanded(true)
	// Reverse-video selection stays legible on any theme (tview's default selected
	// style would draw terminal-default text on a light bar with our theming).
	node.SetSelectedTextStyle(selectionStyle)
	for _, c := range n.Children {
		node.AddChild(a.treeNode(c))
	}
	return node
}

// nodeLabel renders a task's tree line. Folders show a ▸/▾ disclosure marker in
// place of the checkbox (doubling as the expand indicator); regular tasks show
// [ ] / [■] via the shared todoMark.
func (a *app) nodeLabel(t *model.Todo, expanded bool) string {
	var mark string
	if a.folders[t.UID] {
		// The tree adds the expand direction to the folder caret; ▸ collapsed, ▾ open
		// (the other views have no expansion, so they use the plain ▸ from todoMark).
		if expanded {
			mark = "▾ "
		} else {
			mark = "▸ "
		}
	} else {
		// Non-folder: the shared checkbox, so the [ ]/[■] glyph has one source across
		// the tree, month grid, time-grid, and agenda.
		mark = todoMark(t, false)
	}
	label := mark + tview.Escape(nonEmpty(t.Summary, "(untitled)"))
	if t.Priority != model.PriorityUndefined {
		label += fmt.Sprintf("  !%d", t.Priority)
	}
	if t.HasDue {
		label += "  due " + a.fmtDate(t.Due, t.DueAllDay)
	}
	return label
}

func (a *app) showTreeNode(node *tview.TreeNode) {
	if node == nil {
		return
	}
	if t, ok := node.GetReference().(*model.Todo); ok {
		a.setTodoDetail(t)
	}
}

// --- Agenda center (full-detail, custom outline-box widget) ---

// buildAgendaCenter feeds today's items to the agenda board and syncs its
// selection to the left Agenda list.
func (a *app) buildAgendaCenter() {
	day := model.DayStart(a.now)
	a.agenda.setData(day, a.dayItems(day))
	a.agenda.setSelected(a.agendaList.GetCurrentItem())
}

// --- Detail pane ---

func (a *app) setDetail(s string) { a.detail.SetText(s) }

func (a *app) setDayDetail(day time.Time) {
	items := a.dayItems(day)
	var b strings.Builder
	fmt.Fprintf(&b, "[teal]%s[-]\n\n", day.Format("Monday, January 2, 2006"))
	if len(items) == 0 {
		b.WriteString("[gray]No events or due tasks.[-]\n")
	}
	for _, it := range items {
		kind := "event"
		if it.IsTodo() {
			kind = "task"
		}
		fmt.Fprintf(&b, "[gray]%-8s[-] %s  [gray](%s)[-]\n", whenLabel(it, a.clock24), tview.Escape(nonEmpty(it.Title, "(untitled)")), kind)
	}
	a.setDetail(b.String())
}

func (a *app) setAgendaItemDetail(it model.AgendaItem) {
	if it.Todo != nil {
		a.setTodoDetail(it.Todo)
		return
	}
	a.setEventDetail(it.Event)
}

// recurLabel guards the Detail pane's Repeats row: it falls back to "yes" if the
// summary is empty (defensive — a flagged-recurring item always summarizes).
func recurLabel(summary string) string {
	if summary == "" {
		return "yes"
	}
	return summary
}

func (a *app) setTodoDetail(t *model.Todo) {
	var b strings.Builder
	fmt.Fprintf(&b, "[teal]Task[-]\n%s\n\n", tview.Escape(nonEmpty(t.Summary, "(untitled)")))
	fmt.Fprintf(&b, "[gray]Status[-]    %s\n", statusText(t.Status))
	fmt.Fprintf(&b, "[gray]Priority[-]  %s\n", priorityText(t.Priority))
	if t.HasDue {
		fmt.Fprintf(&b, "[gray]Due[-]       %s\n", a.fmtWhen(t.Due, t.DueAllDay))
	} else {
		fmt.Fprintf(&b, "[gray]Due[-]       —\n")
	}
	if len(t.Categories) > 0 {
		fmt.Fprintf(&b, "[gray]Tags[-]      %s\n", tview.Escape(strings.Join(t.Categories, ", ")))
	}
	if t.Location != "" {
		fmt.Fprintf(&b, "[gray]Location[-]  %s\n", tview.Escape(t.Location))
	}
	if t.Recurring {
		anchor := a.now
		if t.HasDue {
			anchor = t.Due
		}
		fmt.Fprintf(&b, "[gray]Repeats[-]   %s\n", tview.Escape(recurLabel(model.RecurrenceSummary(t.Raw, anchor, a.loc))))
	}
	if t.Description != "" {
		fmt.Fprintf(&b, "\n%s\n", tview.Escape(t.Description))
	}
	a.setDetail(b.String())
}

func (a *app) setEventDetail(e *model.Event) {
	if e == nil {
		return
	}
	var b strings.Builder
	fmt.Fprintf(&b, "[teal]Event[-]\n%s\n\n", tview.Escape(nonEmpty(e.Summary, "(untitled)")))
	if e.AllDay {
		fmt.Fprintf(&b, "[gray]When[-]      %s (all day)\n", a.fmtWhen(e.Start, true))
	} else {
		fmt.Fprintf(&b, "[gray]When[-]      %s\n", a.fmtWhen(e.Start, false))
		fmt.Fprintf(&b, "[gray]Until[-]     %s\n", a.fmtWhen(e.End, false))
	}
	if e.Location != "" {
		fmt.Fprintf(&b, "[gray]Location[-]  %s\n", tview.Escape(e.Location))
	}
	if e.Recurring {
		fmt.Fprintf(&b, "[gray]Repeats[-]   %s\n", tview.Escape(recurLabel(model.RecurrenceSummary(e.Raw, e.Start, a.loc))))
	}
	if e.HasAlarm {
		fmt.Fprintf(&b, "[gray]Flags[-]     %s\n", "reminder set")
	}
	if e.Description != "" {
		fmt.Fprintf(&b, "\n%s\n", tview.Escape(e.Description))
	}
	a.setDetail(b.String())
}

// --- Status bar ---

// Interaction modes surfaced by the status-bar indicator — the modal input
// context that decides what the movement keys act on right now (distinct from the
// view context — Calendar/Tasks/Agenda — shown as text in the left section).
const (
	modeNormal = "NORMAL"
	modeDrill  = "DRILL"
	modeGrab   = "GRAB"
	modeResize = "RESIZE"
)

// interactionMode reports the current modal input context for the mode indicator.
// It's derived from existing state (no separate state machine): grab mode, a
// drilled-in calendar day, or the resting NORMAL.
//
// DRILL means "dived into a sub-element" — uniformly the calendar-day drill (where
// the movement keys navigate within the day). Merely focusing the task tree or the
// calendar grid from the overview is NOT drilled: both are ordinary Main-pane
// navigation and read NORMAL, so the tree and grid agree (the tree has no deeper
// level, so DRILL never shows in Tasks).
func (a *app) interactionMode() string {
	switch {
	case a.resizing:
		return modeResize
	case a.grabbing:
		return modeGrab
	case a.modalOpen():
		// A form modal drives the badge from its own NORMAL/DRILL field state; this
		// also takes precedence over a calendar drill left standing behind the form.
		if a.formDrill {
			return modeDrill
		}
		return modeNormal
	case a.gridDrilled():
		return modeDrill
	default:
		return modeNormal
	}
}

// gridDrilled reports whether the active calendar grid is drilled into a day
// (event-cycling), where the movement keys navigate within the day.
func (a *app) gridDrilled() bool {
	if a.mode != modeCalendar {
		return false
	}
	if g, ok := a.calendarPrimitive().(calGrid); ok {
		_, active, _ := g.drillState()
		return active
	}
	return false
}

// drawModeIndicator paints the interaction-mode badge: a quiet dim label at rest
// (NORMAL) and a filled, high-contrast chip for the active modes (DRILL, GRAB) so
// a mode-sensitive key (hjkl) is never a surprise.
func (a *app) drawModeIndicator(screen tcell.Screen, x, y, width, height int) (int, int, int, int) {
	m := a.interactionMode()
	label := " " + m + " "
	style := tcell.StyleDefault.Foreground(tcell.ColorGray) // NORMAL: dim, no fill
	switch m {
	case modeDrill:
		style = tcell.StyleDefault.Foreground(tcell.ColorBlack).Background(accentColor).Bold(true)
	case modeGrab:
		style = tcell.StyleDefault.Foreground(tcell.ColorBlack).Background(todayColor).Bold(true)
	case modeResize:
		style = tcell.StyleDefault.Foreground(tcell.ColorBlack).Background(tcell.ColorFuchsia).Bold(true)
	}
	col := x + (width-len(label))/2
	if col < x {
		col = x
	}
	for _, r := range label {
		if col >= x+width {
			break
		}
		screen.SetContent(col, y, r, nil, style)
		col++
	}
	return x, y, width, height
}

// updateStatus refreshes the permanent controls line and the resting content of
// the 3-section status bar. The left section shows the current context; a flash()
// temporarily replaces it and persists until the next updateStatus.
func (a *app) updateStatus() {
	mode := [...]string{"Calendar", "Tasks", "Agenda"}[a.mode]
	if a.mode == modeCalendar {
		mode += " · " + [...]string{"month", "week", "day"}[a.viewMode]
	}
	left := fmt.Sprintf("%s · %s · %d cals · %d tasks", mode, dateStr(a.anchor, a.dateISO), len(a.store.Calendars()), len(a.store.Todos()))
	// Name the active account only when more than one is configured, so a
	// single-account run keeps the status line uncluttered. Escaped because
	// statusLeft has dynamic color tags and a name could contain '['.
	if len(a.accounts) > 1 && a.activeAccount != "" {
		left = tview.Escape(a.activeAccount) + " · " + left
	}
	if n := len(a.store.LoadErrors()); n > 0 {
		left += fmt.Sprintf("  [red]!%d load error(s)[-]", n)
	}
	a.statusLeft.SetText(left)

	a.renderSyncStatus()

	completed := "off"
	if a.showCompleted {
		completed = "on"
	}
	// Plain text (no color tags) so the [ and ] calendar keys read literally.
	// The full keymap lives in ? help; this line is a curated subset. Order is
	// deliberate: with wrap off a narrow terminal clips the right end, so the two
	// most important hints (? help, q quit) lead, then the basic movement/navigation
	// a new user needs, then the editing actions, then the rest.
	a.hints.SetText(fmt.Sprintf("? help · q quit · hjkl move · Enter open · Esc back · c/t/a panes · f/b prev/next · v view · [ ] cal · { } list · i… new · e edit · d del · Space done/hide · / find · u undo · r sync · . comp:%s · : cmd", completed))
}

// --- shared helpers ---

func (a *app) dayItems(day time.Time) []model.AgendaItem {
	start := model.DayStart(day)
	end := start.AddDate(0, 0, 1)
	occs, _ := a.store.EventOccurrencesVisible(start, end, a.hidden)
	return model.DayAgenda(occs, a.visibleTodos(a.store.TodosVisible(a.hidden)), start, end)
}

// calTypeMarker labels a calendar by what it can hold, from its supported
// component set: [events], [tasks], [both], or [?] when the set is unknown (a
// foreign vdir, or a calendar not yet synced) — which is also why creation is
// blocked there until a sync confirms the type.
func calTypeMarker(cal store.Calendar) string {
	ev, td := hasComponent(cal, compEvent), hasComponent(cal, compTodo)
	switch {
	case ev && td:
		return "[both]"
	case ev:
		return "[events]"
	case td:
		return "[tasks]"
	default:
		return "[?]"
	}
}

// supportsTodos reports whether a calendar belongs in the Tasks panel — i.e. it
// can hold todos, so an empty task list still shows (and can receive tasks). A
// calendar whose server component set is known lists in Tasks iff it includes
// VTODO; when the component set is unknown (a vdir populated by another tool, or
// imported before component capture) fall back to whether it currently holds todos.
func supportsTodos(cal store.Calendar) bool {
	if len(cal.Components) > 0 {
		for _, c := range cal.Components {
			if strings.EqualFold(c, "VTODO") {
				return true
			}
		}
		return false
	}
	_, todos := calCounts(cal)
	return todos > 0
}

func calCounts(cal store.Calendar) (events, todos int) {
	for _, r := range cal.Resources {
		events += len(r.Object.Events)
		todos += len(r.Object.Todos)
	}
	return events, todos
}

func (a *app) agendaLeftLabel(it model.AgendaItem) string {
	mark := ""
	if it.IsTodo() {
		mark = todoMark(it.Todo, a.isFolder(it.Todo.UID))
	}
	return fmt.Sprintf("%-8s %s%s", whenLabel(it, a.clock24), mark, tview.Escape(nonEmpty(it.Title, "(untitled)")))
}

func whenLabel(it model.AgendaItem, use24 bool) string {
	if it.AllDay {
		return "all-day"
	}
	return clockStr(it.Start.In(time.Local), use24)
}

func (a *app) fmtWhen(t time.Time, allDay bool) string {
	local := t.In(a.loc)
	if allDay {
		return local.Format("Mon ") + dateStr(local, a.dateISO)
	}
	return local.Format("Mon ") + dateStr(local, a.dateISO) + " " + clockStr(local, a.clock24)
}

func (a *app) fmtDate(t time.Time, allDay bool) string {
	local := t.In(a.loc)
	if allDay {
		return dateShortStr(local, a.dateISO)
	}
	return dateShortStr(local, a.dateISO) + " " + clockStr(local, a.clock24)
}

func statusText(s model.TodoStatus) string {
	if s == "" {
		return "—"
	}
	return string(s)
}

func priorityText(p int) string {
	if p == model.PriorityUndefined {
		return "—"
	}
	return fmt.Sprintf("%d", p)
}

func oneLine(s string) string {
	return strings.ReplaceAll(strings.TrimSpace(s), "\n", " ")
}

func nonEmpty(s, fallback string) string {
	if strings.TrimSpace(s) == "" {
		return fallback
	}
	return s
}

func dayKey(t time.Time) string { return t.Format("2006-01-02") }
