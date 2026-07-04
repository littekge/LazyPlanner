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
	case 0:
		a.showCalendarAt(a.calendars.GetCurrentItem())
	case 1:
		a.showTreeNode(a.tree.GetCurrentNode())
	case 2:
		a.showAgendaAt(a.agenda.GetCurrentItem())
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
	completed := "off"
	if a.showCompleted {
		completed = "on"
	}
	hints := fmt.Sprintf("[1]Calendars [2]Tasks [3]Agenda  Tab:cycle  Enter:expand  .:completed(%s)  q:quit", completed)
	counts := fmt.Sprintf("%d calendars · %d tasks · %d today", len(a.store.Calendars()), len(a.store.Todos()), len(a.agendaItems))
	line := fmt.Sprintf("%s   [gray]|[-]   %s", hints, counts)
	if n := len(a.store.LoadErrors()); n > 0 {
		line += fmt.Sprintf("   [red]⚠ %d load error(s)[-]", n)
	}
	a.status.SetText(line)
}

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
