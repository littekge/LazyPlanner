// Package ui contains all terminal UI code for LazyPlanner. It is the only
// package permitted to import tview/tcell; every other package compiles and
// tests headlessly. It reaches disk and network only through store and sync,
// never directly.
package ui

import (
	"fmt"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"

	"github.com/littekge/LazyPlanner/internal/model"
	"github.com/littekge/LazyPlanner/internal/store"
)

// Colors are drawn from the terminal's 16-color palette so LazyPlanner inherits
// the terminal theme (see main.md).
const (
	borderIdle    = tcell.ColorGray
	borderFocused = tcell.ColorYellow
	accentColor   = tcell.ColorTeal
	todayColor    = tcell.ColorYellow
	adjacentColor = tcell.ColorGray
	eventColor    = tcell.ColorGreen
)

// Which overview panel is active; the center Main pane follows it.
const (
	modeCalendar = iota
	modeTasks
	modeAgenda
)

// Calendar sub-views (active in modeCalendar).
const (
	viewMonth = iota
	viewWeek
	viewDay
)

type bordered struct {
	prim tview.Primitive
	box  *tview.Box
}

// app holds the widgets and state of the read-only TUI. It reads from the
// store; it does not mutate data (editing arrives in a later step).
type app struct {
	tv    *tview.Application
	store *store.Store
	title string
	now   time.Time
	loc   *time.Location

	// Left overview column.
	calendars  *tview.List
	tasklists  *tview.List
	agendaList *tview.List
	// Center Main pane (Pages: month, time, tree, agenda).
	center     *tview.Pages
	month      *calendarView
	timegrid   *timeGridView
	tree       *tview.TreeView
	agendaView *tview.TextView
	// Right + bottom.
	detail *tview.TextView
	status *tview.TextView

	body     *tview.Flex // holds left | center | detail; used to hide detail
	borders  []bordered
	detailOn bool

	mode            int
	viewMode        int
	anchor          time.Time
	weekStartMonday bool
	showCompleted   bool
	tasklistIDs     []string // calendar ids parallel to the tasklists panel
}

// Run builds the read-only TUI over the given store and blocks until quit.
func Run(s *store.Store, title string) error {
	now := time.Now()
	a := &app{
		tv:              tview.NewApplication(),
		store:           s,
		title:           title,
		now:             now,
		loc:             time.Local,
		calendars:       tview.NewList(),
		tasklists:       tview.NewList(),
		agendaList:      tview.NewList(),
		center:          tview.NewPages(),
		month:           newCalendarView(),
		timegrid:        newTimeGridView(),
		tree:            tview.NewTreeView(),
		agendaView:      tview.NewTextView(),
		detail:          tview.NewTextView(),
		status:          tview.NewTextView(),
		detailOn:        true,
		viewMode:        viewMonth,
		anchor:          model.DayStart(now),
		weekStartMonday: true,
	}
	a.build()
	a.reload()
	a.setMode(modeCalendar)

	if err := a.tv.SetRoot(a.layout(), true).EnableMouse(true).SetInputCapture(a.globalKeys).Run(); err != nil {
		return fmt.Errorf("running tui: %w", err)
	}
	return nil
}

func (a *app) build() {
	a.calendars.ShowSecondaryText(false).SetHighlightFullLine(true)
	a.tasklists.ShowSecondaryText(false).SetHighlightFullLine(true)
	a.agendaList.ShowSecondaryText(false).SetHighlightFullLine(true)
	a.detail.SetDynamicColors(true).SetWrap(true)
	a.agendaView.SetDynamicColors(true).SetWrap(true).SetScrollable(true)
	a.status.SetDynamicColors(true)

	decorate(a.calendars.Box, "1 Calendars")
	decorate(a.tasklists.Box, "2 Tasks")
	decorate(a.agendaList.Box, "3 Agenda")
	decorate(a.month.Box, "Calendar")
	decorate(a.timegrid.Box, "Calendar")
	decorate(a.tree.Box, "Tasks")
	decorate(a.agendaView.Box, "Agenda")
	decorate(a.detail.Box, "Detail")

	a.center.AddPage("month", a.month, true, true)
	a.center.AddPage("time", a.timegrid, true, false)
	a.center.AddPage("tree", a.tree, true, false)
	a.center.AddPage("agenda", a.agendaView, true, false)

	a.borders = []bordered{
		{a.calendars, a.calendars.Box},
		{a.tasklists, a.tasklists.Box},
		{a.agendaList, a.agendaList.Box},
		{a.month, a.month.Box},
		{a.timegrid, a.timegrid.Box},
		{a.tree, a.tree.Box},
		{a.agendaView, a.agendaView.Box},
	}

	// Callbacks.
	a.month.onSelectDay = a.onCalDay
	a.month.onSelectEvent = func(it model.AgendaItem) { a.setAgendaItemDetail(it) }
	a.timegrid.onSelectDay = a.onCalDay
	a.tasklists.SetChangedFunc(func(int, string, string, rune) { a.buildTree() })
	a.tasklists.SetSelectedFunc(func(int, string, string, rune) { a.setFocus(a.tree) })
	a.tree.SetChangedFunc(func(node *tview.TreeNode) { a.showTreeNode(node) })
	a.tree.SetSelectedFunc(func(node *tview.TreeNode) { node.SetExpanded(!node.IsExpanded()) })
}

func (a *app) layout() tview.Primitive {
	left := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(a.calendars, 0, 1, false).
		AddItem(a.tasklists, 0, 1, false).
		AddItem(a.agendaList, 0, 1, false)

	a.body = tview.NewFlex(). // default FlexColumn: side by side
					AddItem(left, 26, 0, false).
					AddItem(a.center, 0, 3, true).
					AddItem(a.detail, 0, 1, false)

	return tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(a.body, 0, 1, true).
		AddItem(a.status, 1, 0, false)
}

func decorate(b *tview.Box, title string) {
	b.SetBorder(true)
	b.SetTitle(" " + title + " ")
	b.SetBorderColor(borderIdle)
}

// setFocus focuses p and repaints borders so the active pane stands out.
func (a *app) setFocus(p tview.Primitive) {
	a.tv.SetFocus(p)
	for _, bp := range a.borders {
		if bp.prim == p {
			bp.box.SetBorderColor(borderFocused)
		} else {
			bp.box.SetBorderColor(borderIdle)
		}
	}
}

// showDetail shows or hides the right Detail pane (hidden in Agenda mode).
func (a *app) showDetail(on bool) {
	if on == a.detailOn {
		return
	}
	a.detailOn = on
	if on {
		a.body.ResizeItem(a.detail, 0, 1)
	} else {
		a.body.ResizeItem(a.detail, 0, 0)
	}
}

// setMode switches the active overview panel and the center pane it drives.
func (a *app) setMode(m int) {
	a.mode = m
	switch m {
	case modeCalendar:
		a.showDetail(true)
		a.buildCenterCalendar()
		a.setFocus(a.calendarPrimitive())
	case modeTasks:
		a.showDetail(true)
		a.center.SwitchToPage("tree")
		a.buildTree()
		a.setFocus(a.tasklists)
	case modeAgenda:
		a.showDetail(false)
		a.center.SwitchToPage("agenda")
		a.buildAgendaCenter()
		a.setFocus(a.agendaView)
	}
	a.updateStatus()
}

// calendarPrimitive returns the active calendar widget for the current view.
func (a *app) calendarPrimitive() tview.Primitive {
	if a.viewMode == viewMonth {
		return a.month
	}
	return a.timegrid
}

func (a *app) globalKeys(ev *tcell.EventKey) *tcell.EventKey {
	switch ev.Key() {
	case tcell.KeyTab:
		a.setMode((a.mode + 1) % 3)
		return nil
	case tcell.KeyBacktab:
		a.setMode((a.mode + 2) % 3)
		return nil
	case tcell.KeyEscape:
		if a.mode == modeTasks {
			a.setFocus(a.tasklists)
			return nil
		}
	case tcell.KeyRune:
		switch ev.Rune() {
		case 'q':
			a.tv.Stop()
			return nil
		case '1':
			a.setMode(modeCalendar)
			return nil
		case '2':
			a.setMode(modeTasks)
			return nil
		case '3':
			a.setMode(modeAgenda)
			return nil
		case '.':
			a.showCompleted = !a.showCompleted
			a.reloadCurrent()
			return nil
		case 'v':
			if a.mode == modeCalendar {
				a.viewMode = (a.viewMode + 1) % 3
				a.buildCenterCalendar()
				a.setFocus(a.calendarPrimitive())
				a.updateStatus()
				return nil
			}
		case 'n':
			if a.mode == modeCalendar {
				a.shiftAnchor(1)
				return nil
			}
		case 'p':
			if a.mode == modeCalendar {
				a.shiftAnchor(-1)
				return nil
			}
		case 't':
			if a.mode == modeCalendar {
				a.anchor = model.DayStart(a.now)
				a.buildCenterCalendar()
				a.setFocus(a.calendarPrimitive())
				a.updateStatus()
				return nil
			}
		}
	}
	return ev
}

// shiftAnchor moves the calendar by one view-period and re-renders.
func (a *app) shiftAnchor(delta int) {
	switch a.viewMode {
	case viewMonth:
		a.anchor = a.anchor.AddDate(0, delta, 0)
	case viewWeek:
		a.anchor = a.anchor.AddDate(0, 0, 7*delta)
	default:
		a.anchor = a.anchor.AddDate(0, 0, delta)
	}
	a.buildCenterCalendar()
	a.setFocus(a.calendarPrimitive())
	a.updateStatus()
}

// reloadCurrent rebuilds whatever the current mode shows (after a data toggle).
func (a *app) reloadCurrent() {
	switch a.mode {
	case modeCalendar:
		a.buildCenterCalendar()
	case modeTasks:
		a.buildTree()
	case modeAgenda:
		a.buildAgendaCenter()
	}
	a.updateStatus()
}

// reload rebuilds every view from the current store contents.
func (a *app) reload() {
	a.buildCalendars()
	a.buildTasklists()
	a.buildAgendaLeft()
	a.buildCenterCalendar()
	a.updateStatus()
}
