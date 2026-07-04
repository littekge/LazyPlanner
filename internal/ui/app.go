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

// Border colors are drawn from the terminal's 16-color palette so LazyPlanner
// inherits the terminal theme (see main.md).
const (
	borderIdle    = tcell.ColorGray
	borderFocused = tcell.ColorYellow
	accentColor   = tcell.ColorTeal
	todayColor    = tcell.ColorYellow
	adjacentColor = tcell.ColorGray
	eventColor    = tcell.ColorGreen
)

// Calendar view modes for the Main pane.
const (
	viewMonth = iota
	viewWeek
	viewDay
)

// Focus indices into the pane cycle.
const (
	focusCalendars = iota
	focusTree
	focusAgenda
	focusMain
	focusCount
)

// app holds the widgets and state of the read-only TUI. It reads from the
// store; it does not mutate data (editing arrives in a later step).
type app struct {
	tv    *tview.Application
	store *store.Store
	title string
	now   time.Time
	loc   *time.Location

	calendars *tview.List
	tree      *tview.TreeView
	agenda    *tview.List
	main      *tview.Pages
	grid      *tview.Table    // month / week grid
	dayView   *tview.TextView // day view
	detail    *tview.TextView
	status    *tview.TextView

	focusIndex  int
	agendaItems []model.AgendaItem

	viewMode        int
	anchor          time.Time     // the focused day in the calendar
	gridWeeks       [][]time.Time // dates currently shown in the grid (row→day)
	weekStartMonday bool
	showCompleted   bool
}

// Run builds the read-only TUI over the given store and blocks until the user
// quits (q or Ctrl-C). title is shown in the status bar.
func Run(s *store.Store, title string) error {
	now := time.Now()
	a := &app{
		tv:              tview.NewApplication(),
		store:           s,
		title:           title,
		now:             now,
		loc:             time.Local,
		calendars:       tview.NewList(),
		tree:            tview.NewTreeView(),
		agenda:          tview.NewList(),
		main:            tview.NewPages(),
		grid:            tview.NewTable(),
		dayView:         tview.NewTextView(),
		detail:          tview.NewTextView(),
		status:          tview.NewTextView(),
		viewMode:        viewMonth,
		anchor:          model.DayStart(now),
		weekStartMonday: true,
	}
	a.build()
	a.reload()
	a.focusAt(focusMain) // start on the calendar

	if err := a.tv.SetRoot(a.layout(), true).EnableMouse(true).SetInputCapture(a.globalKeys).Run(); err != nil {
		return fmt.Errorf("running tui: %w", err)
	}
	return nil
}

func (a *app) build() {
	a.calendars.ShowSecondaryText(false).SetHighlightFullLine(true)
	a.agenda.ShowSecondaryText(false).SetHighlightFullLine(true)
	a.detail.SetDynamicColors(true).SetWrap(true)
	a.dayView.SetDynamicColors(true).SetWrap(true)
	a.status.SetDynamicColors(true)

	a.grid.SetSelectable(true, true).SetFixed(1, 0)
	a.grid.SetSelectedStyle(tcell.StyleDefault.Background(borderFocused).Foreground(tcell.ColorBlack))

	decorate(a.calendars.Box, "1 Calendars")
	decorate(a.tree.Box, "2 Tasks")
	decorate(a.agenda.Box, "3 Agenda")
	decorate(a.grid.Box, "Calendar")
	decorate(a.dayView.Box, "Calendar")
	decorate(a.detail.Box, "Detail")

	a.main.AddPage("grid", a.grid, true, true)
	a.main.AddPage("day", a.dayView, true, false)

	a.calendars.SetChangedFunc(func(i int, _, _ string, _ rune) { a.showCalendarAt(i) })
	a.agenda.SetChangedFunc(func(i int, _, _ string, _ rune) { a.showAgendaAt(i) })
	a.tree.SetChangedFunc(func(node *tview.TreeNode) { a.showTreeNode(node) })
	a.tree.SetSelectedFunc(func(node *tview.TreeNode) { node.SetExpanded(!node.IsExpanded()) })
	a.grid.SetSelectionChangedFunc(func(row, col int) { a.onDaySelected(row, col) })
}

func (a *app) layout() tview.Primitive {
	left := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(a.calendars, 0, 1, false).
		AddItem(a.tree, 0, 2, false).
		AddItem(a.agenda, 0, 1, false)

	body := tview.NewFlex(). // default FlexColumn: side by side
					AddItem(left, 30, 0, false).
					AddItem(a.main, 0, 2, true).
					AddItem(a.detail, 0, 1, false)

	return tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(body, 0, 1, true).
		AddItem(a.status, 1, 0, false)
}

func decorate(b *tview.Box, title string) {
	b.SetBorder(true)
	b.SetTitle(" " + title + " ")
	b.SetBorderColor(borderIdle)
}

// mainBox returns the box of the currently visible Main widget.
func (a *app) mainBox() *tview.Box {
	if a.viewMode == viewDay {
		return a.dayView.Box
	}
	return a.grid.Box
}

// focusTarget returns the primitive to focus for a pane index.
func (a *app) focusTarget(i int) tview.Primitive {
	switch i {
	case focusCalendars:
		return a.calendars
	case focusTree:
		return a.tree
	case focusAgenda:
		return a.agenda
	default:
		if a.viewMode == viewDay {
			return a.dayView
		}
		return a.grid
	}
}

// focusAt moves keyboard focus to pane i and repaints borders.
func (a *app) focusAt(i int) {
	a.focusIndex = i
	a.paintBorders()
	a.tv.SetFocus(a.focusTarget(i))
	a.refreshDetailForFocus()
	a.updateStatus()
}

func (a *app) paintBorders() {
	boxes := []*tview.Box{a.calendars.Box, a.tree.Box, a.agenda.Box, a.mainBox()}
	for i, b := range boxes {
		if i == a.focusIndex {
			b.SetBorderColor(borderFocused)
		} else {
			b.SetBorderColor(borderIdle)
		}
	}
}

func (a *app) globalKeys(ev *tcell.EventKey) *tcell.EventKey {
	switch ev.Key() {
	case tcell.KeyTab:
		a.focusAt((a.focusIndex + 1) % focusCount)
		return nil
	case tcell.KeyBacktab:
		a.focusAt((a.focusIndex - 1 + focusCount) % focusCount)
		return nil
	case tcell.KeyRune:
		switch ev.Rune() {
		case 'q':
			a.tv.Stop()
			return nil
		case '1':
			a.focusAt(focusCalendars)
			return nil
		case '2':
			a.focusAt(focusTree)
			return nil
		case '3':
			a.focusAt(focusAgenda)
			return nil
		case '.':
			a.showCompleted = !a.showCompleted
			a.buildTree()
			a.updateStatus()
			return nil
		case 'v':
			a.viewMode = (a.viewMode + 1) % 3
			a.buildCalendar()
			a.focusAt(focusMain)
			return nil
		case 'n':
			a.shiftAnchor(1)
			return nil
		case 'p':
			a.shiftAnchor(-1)
			return nil
		case 't':
			a.anchor = model.DayStart(a.now)
			a.buildCalendar()
			a.focusAt(focusMain)
			return nil
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
	a.buildCalendar()
	a.focusAt(focusMain)
}

// reload rebuilds every view from the current store contents.
func (a *app) reload() {
	a.buildCalendars()
	a.buildTree()
	a.buildAgenda()
	a.buildCalendar()
	a.updateStatus()
}
