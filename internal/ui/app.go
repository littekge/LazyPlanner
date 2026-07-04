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
)

// app holds the widgets and state of the read-only TUI shell. It reads from the
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
	detail    *tview.TextView
	status    *tview.TextView

	panes       []*tview.Box
	focusables  []tview.Primitive
	focusIndex  int
	agendaItems []model.AgendaItem

	showCompleted bool
}

// Run builds the read-only TUI over the given store and blocks until the user
// quits (q or Ctrl-C). title is shown in the status bar.
func Run(s *store.Store, title string) error {
	a := &app{
		tv:        tview.NewApplication(),
		store:     s,
		title:     title,
		now:       time.Now(),
		loc:       time.Local,
		calendars: tview.NewList(),
		tree:      tview.NewTreeView(),
		agenda:    tview.NewList(),
		detail:    tview.NewTextView(),
		status:    tview.NewTextView(),
	}
	a.build()
	a.reload()
	a.focusAt(1) // start on the task tree, the centerpiece

	if err := a.tv.SetRoot(a.layout(), true).EnableMouse(true).SetInputCapture(a.globalKeys).Run(); err != nil {
		return fmt.Errorf("running tui: %w", err)
	}
	return nil
}

func (a *app) build() {
	a.calendars.ShowSecondaryText(false).SetHighlightFullLine(true)
	a.agenda.ShowSecondaryText(false).SetHighlightFullLine(true)
	a.detail.SetDynamicColors(true).SetWrap(true)
	a.status.SetDynamicColors(true)

	decorate(a.calendars.Box, "1 Calendars")
	decorate(a.tree.Box, "2 Tasks")
	decorate(a.agenda.Box, "3 Agenda")
	decorate(a.detail.Box, "Detail")

	a.panes = []*tview.Box{a.calendars.Box, a.tree.Box, a.agenda.Box}
	a.focusables = []tview.Primitive{a.calendars, a.tree, a.agenda}

	a.calendars.SetChangedFunc(func(i int, _, _ string, _ rune) { a.showCalendarAt(i) })
	a.agenda.SetChangedFunc(func(i int, _, _ string, _ rune) { a.showAgendaAt(i) })
	a.tree.SetChangedFunc(func(node *tview.TreeNode) { a.showTreeNode(node) })
	// Enter / Space toggles expansion of the selected task.
	a.tree.SetSelectedFunc(func(node *tview.TreeNode) { node.SetExpanded(!node.IsExpanded()) })
}

func (a *app) layout() tview.Primitive {
	left := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(a.calendars, 0, 1, false).
		AddItem(a.agenda, 0, 1, false)

	// Default direction is FlexColumn: items sit side by side.
	body := tview.NewFlex().
		AddItem(left, 28, 0, false).
		AddItem(a.tree, 0, 2, true).
		AddItem(a.detail, 0, 2, false)

	return tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(body, 0, 1, true).
		AddItem(a.status, 1, 0, false)
}

func decorate(b *tview.Box, title string) {
	b.SetBorder(true)
	b.SetTitle(" " + title + " ")
	b.SetBorderColor(borderIdle)
}

// focusAt moves keyboard focus to the i-th focusable pane and repaints borders.
func (a *app) focusAt(i int) {
	if i < 0 || i >= len(a.focusables) {
		return
	}
	a.focusIndex = i
	for j, box := range a.panes {
		if j == i {
			box.SetBorderColor(borderFocused)
		} else {
			box.SetBorderColor(borderIdle)
		}
	}
	a.tv.SetFocus(a.focusables[i])
	a.refreshDetailForFocus()
	a.updateStatus()
}

func (a *app) globalKeys(ev *tcell.EventKey) *tcell.EventKey {
	switch ev.Key() {
	case tcell.KeyTab:
		a.focusAt((a.focusIndex + 1) % len(a.focusables))
		return nil
	case tcell.KeyBacktab:
		a.focusAt((a.focusIndex - 1 + len(a.focusables)) % len(a.focusables))
		return nil
	case tcell.KeyRune:
		switch ev.Rune() {
		case 'q':
			a.tv.Stop()
			return nil
		case '1':
			a.focusAt(0)
			return nil
		case '2':
			a.focusAt(1)
			return nil
		case '3':
			a.focusAt(2)
			return nil
		case '.':
			a.showCompleted = !a.showCompleted
			a.buildTree()
			a.updateStatus()
			return nil
		}
	}
	return ev
}

// reload rebuilds every view from the current store contents.
func (a *app) reload() {
	a.buildCalendars()
	a.buildTree()
	a.buildAgenda()
	a.updateStatus()
}
