package ui

import (
	"fmt"
	"strings"
	"time"

	"github.com/rivo/tview"

	"github.com/littekge/LazyPlanner/internal/model"
	"github.com/littekge/LazyPlanner/internal/store"
)

// calRef marks a tree node as a calendar (task list) rather than a todo.
type calRef struct{ cal store.Calendar }

// buildCalendars fills the Calendars list from the store.
func (a *app) buildCalendars() {
	a.calendars.Clear()
	for _, cal := range a.store.Calendars() {
		events, todos := calCounts(cal)
		a.calendars.AddItem(cal.DisplayName, fmt.Sprintf("%d events, %d tasks", events, todos), 0, nil)
	}
}

// buildTree fills the Tasks tree: calendars containing todos become top-level
// folders, with their subtask forests beneath.
func (a *app) buildTree() {
	root := tview.NewTreeNode("").SetSelectable(false)

	for _, cal := range a.store.Calendars() {
		var todos []*model.Todo
		for _, r := range cal.Resources {
			todos = append(todos, r.Object.Todos...)
		}
		forest := model.BuildTree(todos, a.showCompleted)
		if len(forest) == 0 {
			continue // only calendars with (visible) tasks appear
		}
		calNode := tview.NewTreeNode(cal.DisplayName).
			SetReference(calRef{cal}).
			SetColor(accentColor).
			SetExpanded(true)
		for _, n := range forest {
			calNode.AddChild(a.todoNode(n))
		}
		root.AddChild(calNode)
	}

	a.tree.SetRoot(root)
	if kids := root.GetChildren(); len(kids) > 0 {
		a.tree.SetCurrentNode(kids[0])
	} else {
		a.setWelcomeDetail()
	}
}

func (a *app) todoNode(n *model.TodoNode) *tview.TreeNode {
	node := tview.NewTreeNode(taskLabel(n.Todo)).SetReference(n.Todo).SetExpanded(true)
	for _, c := range n.Children {
		node.AddChild(a.todoNode(c))
	}
	return node
}

// buildAgenda fills the Agenda list with today's events and due tasks.
func (a *app) buildAgenda() {
	start := time.Date(a.now.Year(), a.now.Month(), a.now.Day(), 0, 0, 0, 0, a.loc)
	end := start.AddDate(0, 0, 1)
	occs, _ := a.store.EventOccurrences(start, end)
	a.agendaItems = model.DayAgenda(occs, a.store.Todos(), start, end)

	a.agenda.Clear()
	if len(a.agendaItems) == 0 {
		a.agenda.AddItem("(nothing today)", "", 0, nil)
		return
	}
	for _, it := range a.agendaItems {
		a.agenda.AddItem(agendaLabel(it), "", 0, nil)
	}
}

// refreshDetailForFocus updates the Detail pane to the current selection of the
// focused pane, so Detail always tracks where the user is.
func (a *app) refreshDetailForFocus() {
	switch a.focusIndex {
	case focusCalendars:
		a.showCalendarAt(a.calendars.GetCurrentItem())
	case focusTree:
		a.showTreeNode(a.tree.GetCurrentNode())
	case focusAgenda:
		a.showAgendaAt(a.agenda.GetCurrentItem())
	case focusMain:
		a.showDayDetail(a.anchor)
	}
}

func (a *app) showCalendarAt(i int) {
	cals := a.store.Calendars()
	if i < 0 || i >= len(cals) {
		return
	}
	a.setCalendarDetail(cals[i])
}

func (a *app) showAgendaAt(i int) {
	if i < 0 || i >= len(a.agendaItems) {
		return
	}
	a.setAgendaDetail(a.agendaItems[i])
}

func (a *app) showTreeNode(node *tview.TreeNode) {
	if node == nil {
		return
	}
	switch ref := node.GetReference().(type) {
	case *model.Todo:
		a.setTodoDetail(ref)
	case calRef:
		a.setCalendarDetail(ref.cal)
	}
}

// --- Detail rendering ---

func (a *app) setDetail(s string) { a.detail.SetText(s) }

func (a *app) setWelcomeDetail() {
	a.setDetail("[teal]LazyPlanner[-]\n\nNo tasks to show yet.\n\nImport your calendars with:\n  lazyplanner import\n\nThen relaunch.")
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

func (a *app) setAgendaDetail(it model.AgendaItem) {
	if it.Todo != nil {
		a.setTodoDetail(it.Todo)
		return
	}
	a.setEventDetail(it.Event)
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

func (a *app) setCalendarDetail(cal store.Calendar) {
	events, todos := calCounts(cal)
	var b strings.Builder
	fmt.Fprintf(&b, "[teal]Calendar[-]\n%s\n\n", cal.DisplayName)
	fmt.Fprintf(&b, "[gray]Events[-]    %d\n", events)
	fmt.Fprintf(&b, "[gray]Tasks[-]     %d\n", todos)
	if cal.Color != "" {
		fmt.Fprintf(&b, "[gray]Color[-]     %s\n", cal.Color)
	}
	if cal.Href != "" {
		fmt.Fprintf(&b, "[gray]Path[-]      %s\n", cal.Href)
	}
	a.setDetail(b.String())
}

// updateStatus repaints the bottom status bar: key hints and live counts.
func (a *app) updateStatus() {
	view := [...]string{"month", "week", "day"}[a.viewMode]
	completed := "off"
	if a.showCompleted {
		completed = "on"
	}
	hints := fmt.Sprintf("[1]Cal [2]Tasks [3]Agenda Tab:cycle | v:view(%s) n/p:prev/next t:today | .:done(%s) q:quit", view, completed)
	counts := fmt.Sprintf("%s · %d cals · %d tasks", a.anchor.Format("Jan 2 2006"), len(a.store.Calendars()), len(a.store.Todos()))
	line := fmt.Sprintf("%s   [gray]|[-]   %s", hints, counts)
	if n := len(a.store.LoadErrors()); n > 0 {
		line += fmt.Sprintf("   [red]!%d load error(s)[-]", n)
	}
	a.status.SetText(line)
}

// --- Calendar (Main pane) rendering ---

func (a *app) buildCalendar() {
	switch a.viewMode {
	case viewMonth:
		a.renderGrid(model.MonthGrid(a.anchor, a.weekStartMonday), a.anchor.Format("January 2006"))
	case viewWeek:
		week := model.Week(a.anchor, a.weekStartMonday)
		title := "Week of " + week[0].Format("Jan 2, 2006")
		a.renderGrid([][]time.Time{week}, title)
	default:
		a.renderDay()
	}
}

func (a *app) renderGrid(weeks [][]time.Time, title string) {
	a.gridWeeks = weeks
	a.main.SwitchToPage("grid")
	a.grid.Box.SetTitle(" " + title + " ")
	a.grid.Clear()

	for c, name := range a.weekdayHeaders() {
		a.grid.SetCell(0, c, tview.NewTableCell(name).
			SetSelectable(false).
			SetTextColor(accentColor).
			SetAlign(tview.AlignCenter))
	}

	counts := a.countsFor(weeks)
	selRow, selCol := 1, 0
	for r, week := range weeks {
		for c, day := range week {
			cell := tview.NewTableCell(dayCellLabel(day, counts)).
				SetReference(day).
				SetAlign(tview.AlignLeft)
			if a.viewMode == viewMonth && day.Month() != a.anchor.Month() {
				cell.SetTextColor(adjacentColor)
			}
			if model.SameDay(day, a.now) {
				cell.SetTextColor(todayColor)
			}
			a.grid.SetCell(r+1, c, cell)
			if model.SameDay(day, a.anchor) {
				selRow, selCol = r+1, c
			}
		}
	}
	a.grid.Select(selRow, selCol)
	a.paintBorders()
}

func (a *app) renderDay() {
	a.main.SwitchToPage("day")
	a.dayView.Box.SetTitle(" " + a.anchor.Format("Monday, Jan 2 2006") + " ")
	a.dayView.SetText(a.dayAgendaText(a.anchor))
	a.paintBorders()
}

// onDaySelected fires when the grid selection moves; it updates the anchor and
// the Detail pane with that day's agenda.
func (a *app) onDaySelected(row, col int) {
	r := row - 1 // row 0 is the weekday header
	if r < 0 || r >= len(a.gridWeeks) || col < 0 || col >= len(a.gridWeeks[r]) {
		return
	}
	a.anchor = a.gridWeeks[r][col]
	a.showDayDetail(a.anchor)
	a.updateStatus()
}

func (a *app) showDayDetail(day time.Time) {
	items := a.dayItems(day)
	var b strings.Builder
	fmt.Fprintf(&b, "[teal]%s[-]\n\n", day.Format("Monday, January 2, 2006"))
	if len(items) == 0 {
		b.WriteString("[gray]No events or due tasks.[-]\n")
	}
	for _, it := range items {
		when := "all-day"
		if !it.AllDay {
			when = it.Start.Format("3:04pm")
		}
		kind := "event"
		if it.IsTodo() {
			kind = "task"
		}
		fmt.Fprintf(&b, "[gray]%-8s[-] %s  [gray](%s)[-]\n", when, nonEmpty(it.Title, "(untitled)"), kind)
	}
	a.setDetail(b.String())
}

func (a *app) dayAgendaText(day time.Time) string {
	items := a.dayItems(day)
	if len(items) == 0 {
		return "[gray]No events or due tasks.[-]"
	}
	var b strings.Builder
	for _, it := range items {
		when := "all-day"
		if !it.AllDay {
			when = it.Start.Format("3:04pm")
		}
		mark := "   "
		if it.IsTodo() {
			mark = "[ ]"
		}
		fmt.Fprintf(&b, "%-8s %s %s\n", when, mark, nonEmpty(it.Title, "(untitled)"))
	}
	return b.String()
}

// dayItems returns the agenda (events + due todos) for a single day.
func (a *app) dayItems(day time.Time) []model.AgendaItem {
	start := model.DayStart(day)
	end := start.AddDate(0, 0, 1)
	occs, _ := a.store.EventOccurrences(start, end)
	return model.DayAgenda(occs, a.store.Todos(), start, end)
}

// countsFor buckets events (by covered day) and due todos across the grid range
// so each day cell can show how busy it is.
func (a *app) countsFor(weeks [][]time.Time) map[string]int {
	m := map[string]int{}
	if len(weeks) == 0 {
		return m
	}
	start := weeks[0][0]
	gridEnd := weeks[len(weeks)-1][6].AddDate(0, 0, 1)

	occs, _ := a.store.EventOccurrences(start, gridEnd)
	for _, o := range occs {
		end := o.End
		if !end.After(o.Start) {
			end = o.Start.Add(time.Nanosecond) // count a zero-length event on its start day
		}
		for d := model.DayStart(o.Start); d.Before(end); d = d.AddDate(0, 0, 1) {
			if !d.Before(start) && d.Before(gridEnd) {
				m[dayKey(d)]++
			}
		}
	}
	for _, t := range a.store.Todos() {
		if t.HasDue {
			d := model.DayStart(t.Due)
			if !d.Before(start) && d.Before(gridEnd) {
				m[dayKey(d)]++
			}
		}
	}
	return m
}

func (a *app) weekdayHeaders() []string {
	if a.weekStartMonday {
		return []string{"Mon", "Tue", "Wed", "Thu", "Fri", "Sat", "Sun"}
	}
	return []string{"Sun", "Mon", "Tue", "Wed", "Thu", "Fri", "Sat"}
}

func dayCellLabel(day time.Time, counts map[string]int) string {
	if n := counts[dayKey(day)]; n > 0 {
		return fmt.Sprintf("%2d *%d", day.Day(), n)
	}
	return fmt.Sprintf("%2d", day.Day())
}

func dayKey(t time.Time) string { return t.Format("2006-01-02") }

// --- small helpers ---

func calCounts(cal store.Calendar) (events, todos int) {
	for _, r := range cal.Resources {
		events += len(r.Object.Events)
		todos += len(r.Object.Todos)
	}
	return events, todos
}

func taskLabel(t *model.Todo) string {
	mark := "[ ] "
	if t.Completed() {
		mark = "[x] "
	}
	label := mark + nonEmpty(t.Summary, "(untitled)")
	if t.HasDue {
		label += "  (" + fmtDate(t.Due, t.DueAllDay) + ")"
	}
	return label
}

func agendaLabel(it model.AgendaItem) string {
	when := "all-day"
	if !it.AllDay {
		when = it.Start.Format("3:04pm")
	}
	mark := ""
	if it.IsTodo() {
		mark = "[ ] "
	}
	return fmt.Sprintf("%-8s %s%s", when, mark, nonEmpty(it.Title, "(untitled)"))
}

func fmtWhen(t time.Time, allDay bool) string {
	if allDay {
		return t.Format("Mon Jan 2, 2006")
	}
	return t.Format("Mon Jan 2, 2006 3:04pm")
}

func fmtDate(t time.Time, allDay bool) string {
	if allDay {
		return t.Format("Jan 2")
	}
	return t.Format("Jan 2 3:04pm")
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

func nonEmpty(s, fallback string) string {
	if strings.TrimSpace(s) == "" {
		return fallback
	}
	return s
}
