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

// Page names for the top-level Pages: the main layout and the modal overlays.
const (
	pageMain    = "main"
	pageInput   = "modal-input"
	pageForm    = "modal-form"
	pageConfirm = "modal-confirm"
)

type bordered struct {
	prim tview.Primitive
	box  *tview.Box
}

// app holds the widgets and state of the TUI. It reads from and writes to the
// store (create/edit/complete/delete); all writes stay local until the sync
// layer pushes them.
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
	center   *tview.Pages
	month    *calendarView
	timegrid *timeGridView
	tree     *tview.TreeView
	agenda   *agendaBoard
	// Right + bottom. The bottom is two lines: a 3-section status bar
	// (general/results · command-view · sync) above an always-visible controls line.
	detail      *tview.TextView
	statusLeft  *tview.TextView // general status + action results (and flashes)
	statusMid   *tview.TextView // command view — populated in step 10
	statusRight *tview.TextView // sync status — wired in step 9
	hints       *tview.TextView // permanent key-hints line at the very bottom

	body     *tview.Flex  // holds left | center | detail; used to hide detail
	root     *tview.Pages // top-level: the main layout plus modal overlays
	borders  []bordered
	detailOn bool
	undo     []undoStep // session undo stack (most recent last)

	mode            int
	viewMode        int
	anchor          time.Time
	weekStartMonday bool
	showCompleted   bool
	tasklistIDs     []string        // calendar ids parallel to the tasklists panel
	treeFolders     map[string]bool // task UIDs that are folders (≥1 incomplete child)
	treeListID      string          // calendar id the tree currently shows (to detect list changes)
	stickyDone      map[string]bool // tasks completed while hidden, kept visible until the list is left
	savedFocus      focusState      // where focus was before a modal opened, to restore on close
}

// focusState remembers where focus was so closing a modal returns there — down
// to a drilled-into calendar day, not just the overview pane.
type focusState struct {
	prim     tview.Primitive
	calDay   time.Time
	calEvent bool // was the calendar in event-cycling mode
	calIndex int
}

// calGrid is implemented by the month and time-grid views so drill-in (event
// cycling) state can be captured and restored uniformly.
type calGrid interface {
	drillState() (day time.Time, active bool, index int)
	reDrill(day time.Time, index int)
}

// useRoundedBorders switches tview's global border runes to rounded (soft)
// corners and keeps single-line edges even when a box is focused — focus is
// shown by border color, not a heavier line. Set once at startup; tview.Borders
// is the library's border-config surface (not app state).
func useRoundedBorders() {
	tview.Borders.TopLeft = '╭'
	tview.Borders.TopRight = '╮'
	tview.Borders.BottomLeft = '╰'
	tview.Borders.BottomRight = '╯'
	tview.Borders.TopLeftFocus = '╭'
	tview.Borders.TopRightFocus = '╮'
	tview.Borders.BottomLeftFocus = '╰'
	tview.Borders.BottomRightFocus = '╯'
	tview.Borders.HorizontalFocus = tview.Borders.Horizontal
	tview.Borders.VerticalFocus = tview.Borders.Vertical
}

// Run builds the TUI over the given store and blocks until quit.
func Run(s *store.Store, title string) error {
	useRoundedBorders()
	a := newApp(s, title, time.Now())
	a.build()
	a.reload()
	a.setMode(modeCalendar)

	a.root = tview.NewPages()
	a.root.AddPage(pageMain, a.layout(), true, true)

	if err := a.tv.SetRoot(a.root, true).EnableMouse(true).SetInputCapture(a.globalKeys).Run(); err != nil {
		return fmt.Errorf("running tui: %w", err)
	}
	return nil
}

// newApp assembles the app and its widgets over the store. Wiring (build) and
// data load (reload) are separate so tests can drive them headlessly with a
// fixed clock.
func newApp(s *store.Store, title string, now time.Time) *app {
	return &app{
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
		agenda:          newAgendaBoard(),
		detail:          tview.NewTextView(),
		statusLeft:      tview.NewTextView(),
		statusMid:       tview.NewTextView(),
		statusRight:     tview.NewTextView(),
		hints:           tview.NewTextView(),
		detailOn:        true,
		viewMode:        viewMonth,
		anchor:          model.DayStart(now),
		weekStartMonday: true,
		treeFolders:     map[string]bool{},
		stickyDone:      map[string]bool{},
	}
}

func (a *app) build() {
	a.calendars.ShowSecondaryText(false).SetHighlightFullLine(true)
	a.tasklists.ShowSecondaryText(false).SetHighlightFullLine(true)
	a.agendaList.ShowSecondaryText(false).SetHighlightFullLine(true)
	a.detail.SetDynamicColors(true).SetWrap(true)
	a.statusLeft.SetDynamicColors(true)
	a.statusMid.SetDynamicColors(true).SetTextAlign(tview.AlignCenter)
	a.statusRight.SetDynamicColors(true).SetTextAlign(tview.AlignRight)
	a.hints.SetWrap(false) // plain text so [ and ] render literally; always-visible controls

	decorate(a.calendars.Box, "1 Calendars")
	decorate(a.tasklists.Box, "2 Tasks")
	decorate(a.agendaList.Box, "3 Agenda")
	decorate(a.month.Box, "Calendar")
	decorate(a.timegrid.Box, "Calendar")
	decorate(a.tree.Box, "Tasks")
	decorate(a.agenda.Box, "Agenda")
	decorate(a.detail.Box, "Detail")

	a.center.AddPage("month", a.month, true, true)
	a.center.AddPage("time", a.timegrid, true, false)
	a.center.AddPage("tree", a.tree, true, false)
	a.center.AddPage("agenda", a.agenda, true, false)

	a.borders = []bordered{
		{a.calendars, a.calendars.Box},
		{a.tasklists, a.tasklists.Box},
		{a.agendaList, a.agendaList.Box},
		{a.month, a.month.Box},
		{a.timegrid, a.timegrid.Box},
		{a.tree, a.tree.Box},
		{a.agenda, a.agenda.Box},
	}

	// Callbacks.
	a.month.onSelectDay = a.onCalDay
	a.month.onSelectEvent = func(it model.AgendaItem) { a.setAgendaItemDetail(it) }
	a.month.onExit = func() { a.setFocus(a.calendars) }
	a.timegrid.onSelectDay = a.onCalDay
	a.timegrid.onSelectEvent = func(o model.Occurrence) { a.setEventDetail(o.Event) }
	a.timegrid.onExit = func() { a.setFocus(a.calendars) }
	a.calendars.SetSelectedFunc(func(int, string, string, rune) { a.setFocus(a.calendarPrimitive()) })
	a.tasklists.SetChangedFunc(func(int, string, string, rune) { a.buildTree() })
	a.tasklists.SetSelectedFunc(func(int, string, string, rune) { a.setFocus(a.tree) })
	a.agendaList.SetChangedFunc(func(index int, _, _ string, _ rune) {
		if a.mode == modeAgenda {
			a.agenda.setSelected(index)
		}
	})
	a.tree.SetChangedFunc(func(node *tview.TreeNode) { a.showTreeNode(node) })
	a.tree.SetSelectedFunc(func(node *tview.TreeNode) {
		node.SetExpanded(!node.IsExpanded())
		// Keep the folder disclosure marker (▸/▾) in sync with the new state.
		if t, ok := node.GetReference().(*model.Todo); ok {
			node.SetText(a.nodeLabel(t, node.IsExpanded()))
		}
	})
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

	statusBar := tview.NewFlex(). // 3 sections: general | command view | sync
					AddItem(a.statusLeft, 0, 2, false).
					AddItem(a.statusMid, 0, 2, false).
					AddItem(a.statusRight, 24, 0, false)

	return tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(a.body, 0, 1, true).
		AddItem(statusBar, 1, 0, false).
		AddItem(a.hints, 1, 0, false)
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
	// Switching panes counts as leaving the list: stop keeping just-completed
	// tasks pinned visible.
	a.stickyDone = map[string]bool{}
	switch m {
	case modeCalendar:
		a.showDetail(true)
		a.buildCenterCalendar()
		a.setFocus(a.calendars)
	case modeTasks:
		a.showDetail(true)
		a.center.SwitchToPage("tree")
		a.buildTree()
		a.setFocus(a.tasklists)
	case modeAgenda:
		a.showDetail(false)
		a.center.SwitchToPage("agenda")
		a.buildAgendaCenter()
		a.setFocus(a.agendaList)
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
	// While a modal (input/form/confirm) is open it owns all keys; app shortcuts
	// must not fire, or typing "a"/"q"/digits would trigger them.
	if a.modalOpen() {
		return ev
	}
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
		case 'a':
			a.addQuick()
			return nil
		case 'A':
			a.addFull()
			return nil
		case 's':
			a.addSubtaskQuick()
			return nil
		case 'S':
			a.addSubtaskFull()
			return nil
		case 'e':
			a.editSelected()
			return nil
		case 'd':
			a.deleteSelected()
			return nil
		case ' ':
			a.toggleComplete()
			return nil
		case 'u':
			a.undoLast()
			return nil
		case 'H':
			if a.mode == modeTasks {
				a.reparentSelected(outdent)
				return nil
			}
		case 'L':
			if a.mode == modeTasks {
				a.reparentSelected(indent)
				return nil
			}
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
				a.refocusCalendar()
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
				a.refocusCalendar()
				a.updateStatus()
				return nil
			}
		case '[':
			if a.mode == modeCalendar {
				a.cycleCalendar(-1)
				return nil
			}
		case ']':
			if a.mode == modeCalendar {
				a.cycleCalendar(1)
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
	a.refocusCalendar()
	a.updateStatus()
}

// refocusCalendar keeps focus on the overview list, but follows the grid to its
// (possibly swapped) primitive when the grid itself was focused — so switching
// view or period while diving through the grid doesn't kick focus back to the list.
func (a *app) refocusCalendar() {
	if a.gridFocused() {
		a.setFocus(a.calendarPrimitive())
	}
}

func (a *app) gridFocused() bool {
	f := a.tv.GetFocus()
	return f == a.month || f == a.timegrid
}

// cycleCalendar moves the highlight in the Calendars overview (wrapping), usable
// from the grid too. Toggling a calendar's visibility arrives in step 10.
func (a *app) cycleCalendar(delta int) {
	n := a.calendars.GetItemCount()
	if n == 0 {
		return
	}
	i := (a.calendars.GetCurrentItem() + delta + n) % n
	a.calendars.SetCurrentItem(i)
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
