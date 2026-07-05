package ui

import (
	"fmt"
	"strings"
	"time"

	"github.com/rivo/tview"

	"github.com/littekge/LazyPlanner/internal/model"
	"github.com/littekge/LazyPlanner/internal/store"
)

// --- Left overview column ---

func (a *app) buildCalendars() {
	a.calendars.Clear()
	for _, cal := range a.store.Calendars() {
		events, todos := calCounts(cal)
		a.calendars.AddItem(fmt.Sprintf("%s  (%de %dt)", cal.DisplayName, events, todos), "", 0, nil)
	}
	if a.calendars.GetItemCount() == 0 {
		a.calendars.AddItem("(no calendars)", "", 0, nil)
	}
}

func (a *app) buildTasklists() {
	a.tasklists.Clear()
	a.tasklistIDs = a.tasklistIDs[:0]
	for _, cal := range a.store.Calendars() {
		if _, todos := calCounts(cal); todos > 0 {
			a.tasklists.AddItem(cal.DisplayName, "", 0, nil)
			a.tasklistIDs = append(a.tasklistIDs, cal.ID)
		}
	}
	if len(a.tasklistIDs) == 0 {
		a.tasklists.AddItem("(no task lists)", "", 0, nil)
	}
}

func (a *app) buildAgendaLeft() {
	a.agendaList.Clear()
	items := a.dayItems(model.DayStart(a.now))
	a.agendaCount = len(items)
	if len(items) == 0 {
		a.agendaList.AddItem("(nothing today)", "", 0, nil)
		return
	}
	for _, it := range items {
		a.agendaList.AddItem(agendaLeftLabel(it), "", 0, nil)
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
	occs, _ := a.store.EventOccurrences(start, end)
	todos := a.store.Todos()
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

// splitOccs buckets a range's occurrences into timed and all-day, keyed by day,
// for the time-grid. Timed events land on their start day; all-day events mark
// every day they cover.
func (a *app) splitOccs(days []time.Time) (timed, allday map[string][]model.Occurrence) {
	timed = map[string][]model.Occurrence{}
	allday = map[string][]model.Occurrence{}
	if len(days) == 0 {
		return
	}
	start := days[0]
	end := days[len(days)-1].AddDate(0, 0, 1)
	occs, _ := a.store.EventOccurrences(start, end)
	for _, o := range occs {
		if o.Event.AllDay {
			for d := model.DayStart(o.Start); d.Before(o.End); d = d.AddDate(0, 0, 1) {
				if !d.Before(start) && d.Before(end) {
					allday[dayKey(d)] = append(allday[dayKey(d)], o)
				}
			}
			continue
		}
		d := model.DayStart(o.Start)
		timed[dayKey(d)] = append(timed[dayKey(d)], o)
	}
	return timed, allday
}

// --- Tasks center (tree) ---

func (a *app) buildTree() {
	// The root node shows the list's own name so the top-level tasks' connector
	// stems attach to it (like a file tree rooted at the directory name).
	name := "Tasks"
	root := tview.NewTreeNode("").SetSelectable(false).SetColor(accentColor)
	id := a.selectedTasklistID()

	// Leaving one list for another drops the "keep just-completed visible" pins.
	// Ignore the transient empty id seen mid-rebuild (while the panel is cleared).
	if id != "" && id != a.treeListID {
		a.stickyDone = map[string]bool{}
		a.treeListID = id
	}

	a.treeFolders = map[string]bool{}
	if id != "" {
		if cal, ok := a.store.Calendar(id); ok {
			name = cal.DisplayName
			var all []*model.Todo
			for _, r := range cal.Resources {
				all = append(all, r.Object.Todos...)
			}
			a.treeFolders = folderSet(all)

			// Show incomplete tasks, plus completed ones when toggled on or pinned.
			var visible []*model.Todo
			for _, td := range all {
				if a.showCompleted || !td.Completed() || a.stickyDone[td.UID] {
					visible = append(visible, td)
				}
			}
			for _, n := range model.BuildTree(visible, true) {
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
	for _, c := range n.Children {
		node.AddChild(a.treeNode(c))
	}
	return node
}

// nodeLabel renders a task's tree line. Folders show a ▸/▾ disclosure marker in
// place of the checkbox (doubling as the expand indicator); regular tasks show
// [ ] / [x].
func (a *app) nodeLabel(t *model.Todo, expanded bool) string {
	var mark string
	switch {
	case a.treeFolders[t.UID]:
		if expanded {
			mark = "▾ "
		} else {
			mark = "▸ "
		}
	case t.Completed():
		mark = "[x] "
	default:
		mark = "[ ] "
	}
	label := mark + nonEmpty(t.Summary, "(untitled)")
	if t.Priority != model.PriorityUndefined {
		label += fmt.Sprintf("  !%d", t.Priority)
	}
	if t.HasDue {
		label += "  due " + fmtDate(t.Due, t.DueAllDay)
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

// --- Agenda center (full-detail, scrollable) ---

func (a *app) buildAgendaCenter() {
	a.renderAgenda(a.currentAgendaIndex())
}

// currentAgendaIndex is the left Agenda list's selection clamped to the real
// items (the list shows a placeholder row when the day is empty).
func (a *app) currentAgendaIndex() int {
	if a.agendaCount == 0 {
		return -1
	}
	i := a.agendaList.GetCurrentItem()
	if i < 0 {
		i = 0
	}
	if i >= a.agendaCount {
		i = a.agendaCount - 1
	}
	return i
}

// renderAgenda draws the full-detail day agenda, drawing the selected block as an
// explicit black-on-white bar and scrolling it into view. We style the selection
// ourselves (rather than tview's region highlight) because that highlight derives
// contrast from a color's nominal RGB, which the terminal's 16-color palette
// remaps — making a colored title illegible under the highlight.
func (a *app) renderAgenda(selected int) {
	day := model.DayStart(a.now)
	items := a.dayItems(day)
	a.agendaCount = len(items)

	var b strings.Builder
	fmt.Fprintf(&b, "[teal::b]%s[-:-:-]\n\n", day.Format("Monday, January 2, 2006"))
	if len(items) == 0 {
		b.WriteString("[gray]No events or due tasks today.[-]\n")
	}
	line := 2 // date line + the blank line after it
	selStart, selHeight := -1, 0
	for i, it := range items {
		if i > 0 {
			b.WriteString("\n")
			line++
		}
		block := agendaItemBlock(it, i == selected)
		h := strings.Count(block, "\n")
		if i == selected {
			selStart, selHeight = line, h
			fmt.Fprintf(&b, "[black:white]%s[-:-]", block)
		} else {
			b.WriteString(block)
		}
		line += h
	}
	a.agendaView.SetText(b.String())
	if selStart >= 0 {
		a.scrollAgendaTo(selStart, selHeight)
	} else {
		a.agendaView.ScrollToBeginning()
	}
}

// scrollAgendaTo scrolls the agenda only as much as needed to keep the block at
// [start, start+height) fully visible (like a list cursor), never jumping to top.
func (a *app) scrollAgendaTo(start, height int) {
	_, _, _, viewH := a.agendaView.GetInnerRect()
	if viewH <= 0 { // not laid out yet; approximate
		a.agendaView.ScrollTo(max(0, start-1), 0)
		return
	}
	top, _ := a.agendaView.GetScrollOffset()
	if start < top {
		top = start
	} else if start+height > top+viewH {
		top = start + height - viewH
	}
	if top < 0 {
		top = 0
	}
	a.agendaView.ScrollTo(top, 0)
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
		fmt.Fprintf(&b, "[gray]%-8s[-] %s  [gray](%s)[-]\n", whenLabel(it), nonEmpty(it.Title, "(untitled)"), kind)
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

func (a *app) setTodoDetail(t *model.Todo) {
	var b strings.Builder
	fmt.Fprintf(&b, "[teal]Task[-]\n%s\n\n", nonEmpty(t.Summary, "(untitled)"))
	fmt.Fprintf(&b, "[gray]Status[-]    %s\n", statusText(t.Status))
	fmt.Fprintf(&b, "[gray]Priority[-]  %s\n", priorityText(t.Priority))
	if t.HasDue {
		fmt.Fprintf(&b, "[gray]Due[-]       %s\n", fmtWhen(t.Due, t.DueAllDay))
	} else {
		fmt.Fprintf(&b, "[gray]Due[-]       —\n")
	}
	if len(t.Categories) > 0 {
		fmt.Fprintf(&b, "[gray]Tags[-]      %s\n", strings.Join(t.Categories, ", "))
	}
	if t.Recurring {
		fmt.Fprintf(&b, "[gray]Repeats[-]   yes\n")
	}
	if t.Description != "" {
		fmt.Fprintf(&b, "\n%s\n", t.Description)
	}
	a.setDetail(b.String())
}

func (a *app) setEventDetail(e *model.Event) {
	if e == nil {
		return
	}
	var b strings.Builder
	fmt.Fprintf(&b, "[teal]Event[-]\n%s\n\n", nonEmpty(e.Summary, "(untitled)"))
	if e.AllDay {
		fmt.Fprintf(&b, "[gray]When[-]      %s (all day)\n", fmtWhen(e.Start, true))
	} else {
		fmt.Fprintf(&b, "[gray]When[-]      %s\n", fmtWhen(e.Start, false))
		fmt.Fprintf(&b, "[gray]Until[-]     %s\n", fmtWhen(e.End, false))
	}
	if e.Location != "" {
		fmt.Fprintf(&b, "[gray]Location[-]  %s\n", e.Location)
	}
	var flags []string
	if e.Recurring {
		flags = append(flags, "repeats")
	}
	if e.HasAlarm {
		flags = append(flags, "reminder set")
	}
	if len(flags) > 0 {
		fmt.Fprintf(&b, "[gray]Flags[-]     %s\n", strings.Join(flags, ", "))
	}
	if e.Description != "" {
		fmt.Fprintf(&b, "\n%s\n", e.Description)
	}
	a.setDetail(b.String())
}

// --- Status bar ---

// updateStatus refreshes the permanent controls line and the resting content of
// the 3-section status bar. The left section shows the current context; a flash()
// temporarily replaces it and persists until the next updateStatus.
func (a *app) updateStatus() {
	mode := [...]string{"Calendar", "Tasks", "Agenda"}[a.mode]
	if a.mode == modeCalendar {
		mode += " · " + [...]string{"month", "week", "day"}[a.viewMode]
	}
	left := fmt.Sprintf("%s · %s · %d cals · %d tasks", mode, a.anchor.Format("Jan 2 2006"), len(a.store.Calendars()), len(a.store.Todos()))
	if n := len(a.store.LoadErrors()); n > 0 {
		left += fmt.Sprintf("  [red]!%d load error(s)[-]", n)
	}
	a.statusLeft.SetText(left)

	// Sync is wired in step 9; show a neutral placeholder for now.
	a.statusRight.SetText("[gray]— not synced[-]")

	completed := "off"
	if a.showCompleted {
		completed = "on"
	}
	// Plain text (no color tags) so the [ and ] calendar keys read literally.
	// Kept short so it fits without truncation; the full keymap lives in ? help.
	a.hints.SetText(fmt.Sprintf("1/2/3 panes · a/s add · e edit · d del · Space done · u undo · v view · [ ]/n/p/t cal · . completed:%s · q quit", completed))
}

// --- shared helpers ---

func (a *app) dayItems(day time.Time) []model.AgendaItem {
	start := model.DayStart(day)
	end := start.AddDate(0, 0, 1)
	occs, _ := a.store.EventOccurrences(start, end)
	return model.DayAgenda(occs, a.store.Todos(), start, end)
}

func calCounts(cal store.Calendar) (events, todos int) {
	for _, r := range cal.Resources {
		events += len(r.Object.Events)
		todos += len(r.Object.Todos)
	}
	return events, todos
}

// agendaItemBlock is a full-detail block for the Agenda center pane. When plain
// is set it emits no color tags, so the caller can wrap it in a single uniform
// selection style (the highlighted row) without inner tags fighting the colors.
func agendaItemBlock(it model.AgendaItem, plain bool) string {
	title, meta, reset := "[green]", "[gray]", "[-]"
	if it.IsTodo() {
		title = "[aqua]"
	}
	if plain {
		title, meta, reset = "", "", ""
	}
	var b strings.Builder
	if it.Todo != nil {
		t := it.Todo
		fmt.Fprintf(&b, "%s%s  %s%s\n", title, whenLabel(it), nonEmpty(t.Summary, "(untitled)"), reset)
		fmt.Fprintf(&b, "  %stask · %s · priority %s%s\n", meta, statusText(t.Status), priorityText(t.Priority), reset)
		if t.Description != "" {
			fmt.Fprintf(&b, "  %s\n", oneLine(t.Description))
		}
		return b.String()
	}
	e := it.Event
	fmt.Fprintf(&b, "%s%s  %s%s\n", title, whenLabel(it), nonEmpty(e.Summary, "(untitled)"), reset)
	if e.Location != "" {
		fmt.Fprintf(&b, "  %sat %s%s\n", meta, e.Location, reset)
	}
	if e.Description != "" {
		fmt.Fprintf(&b, "  %s\n", oneLine(e.Description))
	}
	return b.String()
}

func agendaLeftLabel(it model.AgendaItem) string {
	mark := ""
	if it.IsTodo() {
		mark = "[ ] "
	}
	return fmt.Sprintf("%-8s %s%s", whenLabel(it), mark, nonEmpty(it.Title, "(untitled)"))
}

func whenLabel(it model.AgendaItem) string {
	if it.AllDay {
		return "all-day"
	}
	return it.Start.In(time.Local).Format("3:04pm")
}

func fmtWhen(t time.Time, allDay bool) string {
	if allDay {
		return t.In(time.Local).Format("Mon Jan 2, 2006")
	}
	return t.In(time.Local).Format("Mon Jan 2, 2006 3:04pm")
}

func fmtDate(t time.Time, allDay bool) string {
	if allDay {
		return t.In(time.Local).Format("Jan 2")
	}
	return t.In(time.Local).Format("Jan 2 3:04pm")
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
