// Package ui contains all terminal UI code for LazyPlanner. It is the only
// package permitted to import tview/tcell; every other package compiles and
// tests headlessly. It reaches disk and network only through store and sync,
// never directly.
package ui

import (
	"context"
	"fmt"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"

	"github.com/littekge/LazyPlanner/internal/model"
	"github.com/littekge/LazyPlanner/internal/store"
	"github.com/littekge/LazyPlanner/internal/sync"
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

// selectionStyle marks the highlighted row in the lists and task tree. Reverse
// video is theme-agnostic — it's the inverse of the (legible) normal text, so it
// stays readable on any light or dark terminal background.
var selectionStyle = tcell.StyleDefault.Reverse(true)

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
	pageMain     = "main"
	pageInput    = "modal-input"
	pageForm     = "modal-form"
	pageConfirm  = "modal-confirm"
	pageWhichKey = "which-key"
	pageColor    = "modal-color"
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

	pendingPrefix   rune // active chord prefix (e.g. 'i'); 0 when none
	pendingForce    bool // '!' armed after the create prefix (i!… force)
	forceCreate     bool // set for the duration of a forced create action
	pendingCount    int  // accumulated vim count (e.g. 3 in "3j"); 0 when none
	searchQuery     string
	searchIdx       int    // which match n/N is currently on
	searchRestore   func() // restores the pre-search selection on cancel
	yankUID         string // task yanked for a move (y); "" when clipboard empty
	mode            int
	viewMode        int
	anchor          time.Time
	weekStartMonday bool
	clock24         bool // time_format: 24h clock when true
	dateISO         bool // date_format: ISO (2006-01-02) when true, else US
	showCompleted   bool
	tasklistIDs     []string            // calendar ids parallel to the tasklists panel
	calColors       map[string]calColor // calendar id → resolved server color; mappable only
	itemColors      map[string]calColor // event/todo UID → its calendar's color
	colorMode       colorMode           // how server colors are rendered (auto/16/off)
	folders         map[string]bool     // task UIDs that are folders (≥1 incomplete child); global, for tree + calendar + agenda markers
	treeListID      string              // calendar id the tree currently shows (to detect list changes)
	zoomUID         string              // task UID the tree is re-rooted at (> zoom-in / < zoom-out); "" = list root
	suspendTree     bool                // ignore tasklist change events while the panel is rebuilt
	stickyDone      map[string]bool     // tasks completed while hidden, kept visible until the list is left
	focusStack      []focusState        // pre-modal focus states, one per open modal (supports nesting, e.g. a color picker over the calendar form)

	// ctx is cancelled on shutdown so an in-flight background sync unwinds cleanly
	// at its next ctx checkpoint (the sync/caldav stack honors it) rather than
	// being detached or hard-killed on quit.
	ctx    context.Context
	cancel context.CancelFunc

	// Sync (wired in step 9). syncFn is nil when no server is configured.
	syncFn      func(context.Context) (sync.SyncResult, error)
	editConfig  func() (ConfigReload, error)
	syncing     bool
	lastSyncAt  time.Time
	lastSyncErr error

	// Pane sizing (step 10). leftCol is the left overview column so its width can
	// be resized (Ctrl-←/→) or collapsed (accordion +/-). saveState persists the
	// chosen width.
	leftCol   *tview.Flex
	leftWidth int
	accordion bool
	saveState func(leftWidth int, hidden []string, rowsPerHour int)

	// hourRows is the week/day time-grid hour-row height set with +/- (0 =
	// auto-fit the whole day to the pane); mirrored onto the time grid and
	// persisted in the state file.
	hourRows int

	// hidden holds the calendar ids the user has hidden from the calendar/agenda
	// views (persisted in the state file). A local view preference, not server data.
	hidden map[string]bool
}

// Left-column sizing bounds (columns).
const (
	defaultLeftWidth = 26
	minLeftWidth     = 16
	maxLeftWidth     = 50
	leftWidthStep    = 3
)

func clampLeftWidth(w int) int {
	switch {
	case w < minLeftWidth:
		return minLeftWidth
	case w > maxLeftWidth:
		return maxLeftWidth
	default:
		return w
	}
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

// useTerminalTheme configures tview's globals once at startup: inherit the
// terminal's default background everywhere, and use rounded (soft) border
// corners. Inheriting the background is what keeps text from sitting in a shaded
// box — tview's default primitive background is solid black, which mismatches the
// terminal-default background of our text cells on non-black color schemes.
// tview.Styles/Borders are the library's config surfaces (not app state); this
// must run before any widget is created so NewBox picks up the background.
func useTerminalTheme() {
	tview.Styles.PrimitiveBackgroundColor = tcell.ColorDefault

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

// Options bundles the dependencies the TUI needs from main (thin wiring): the
// store, a sync closure (nil = offline), and the persisted UI state plus a
// callback to save it — so the UI never touches disk itself.
type Options struct {
	Store       *store.Store
	Title       string
	Sync        func(context.Context) (sync.SyncResult, error)
	LeftWidth   int      // remembered left-column width (0 = default)
	Hidden      []string // calendar ids hidden from the calendar/agenda views
	RowsPerHour int      // remembered week/day hour-row height (0 = auto-fit)
	ColorMode   string   // how server calendar colors render: "auto"/"truecolor", "16", or "off"
	// Appearance ([appearance] config): empty = the built-in default.
	FirstDayOfWeek string // "monday" (default) or "sunday"
	DefaultView    string // "month" (default), "week", or "day"
	TimeFormat     string // "12h" (default) or "24h"
	DateFormat     string // "us" (default) or "iso"
	// SaveState persists remembered UI state (nil = don't persist). Every save
	// passes the full state, so the caller can rewrite the file wholesale.
	SaveState func(leftWidth int, hidden []string, rowsPerHour int)
	// EditConfig opens the config file in $EDITOR and reloads it, returning the
	// settings the running app can apply live and an error. The UI calls it
	// inside a tview Suspend so the editor owns the terminal. nil disables
	// :config. main owns the path, editor launch, and parsing.
	EditConfig func() (ConfigReload, error)
}

// ConfigReload carries the reloaded settings the running app applies live after
// the :config editor flow. main parses the config; the UI never does.
type ConfigReload struct {
	// Sync is a rebuilt sync closure (nil = keep the current one / offline).
	Sync func(context.Context) (sync.SyncResult, error)
	// ColorMode is the reloaded [appearance] color_mode ("auto"/"16"/"off"/…).
	ColorMode string
	// Warning, when set, explains why the reloaded connection is offline (e.g. a
	// failed password_command); the UI flashes it so it isn't lost to stderr.
	Warning string
}

// Run builds the TUI and blocks until quit.
func Run(opts Options) error {
	a := newApp(opts.Store, opts.Title, time.Now())
	defer a.cancel() // on quit, unwind any in-flight background sync cleanly
	a.syncFn = opts.Sync
	a.saveState = opts.SaveState
	a.editConfig = opts.EditConfig
	a.colorMode = parseColorMode(opts.ColorMode)
	a.weekStartMonday = parseWeekStartMonday(opts.FirstDayOfWeek)
	a.viewMode = parseDefaultView(opts.DefaultView)
	a.clock24 = opts.TimeFormat == "24h"
	a.dateISO = opts.DateFormat == "iso"
	a.month.clock24 = a.clock24
	a.timegrid.clock24 = a.clock24
	a.agenda.clock24 = a.clock24
	for _, id := range opts.Hidden {
		a.hidden[id] = true
	}
	if opts.LeftWidth != 0 {
		a.leftWidth = clampLeftWidth(opts.LeftWidth)
	}
	if opts.RowsPerHour != 0 {
		a.hourRows = clampRowsPerHour(opts.RowsPerHour)
	}
	a.build()
	a.timegrid.rowsPerHour = a.hourRows
	a.reload()
	a.setMode(modeCalendar)

	a.root = tview.NewPages()
	a.root.AddPage(pageMain, a.layout(), true, true)

	// Sync on startup in the background: the UI opens instantly from cache and
	// refreshes when the sync lands (offline-first). QueueUpdateDraw inside
	// triggerSync waits for the event loop, so starting it now is safe.
	a.triggerSync()

	a.tv.SetMouseCapture(a.mouseCapture)

	if err := a.tv.SetRoot(a.root, true).EnableMouse(true).SetInputCapture(a.globalKeys).Run(); err != nil {
		return fmt.Errorf("running tui: %w", err)
	}
	return nil
}

// newApp assembles the app and its widgets over the store. Wiring (build) and
// data load (reload) are separate so tests can drive them headlessly with a
// fixed clock.
func newApp(s *store.Store, title string, now time.Time) *app {
	useTerminalTheme() // configure tview globals before any widget is created
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
		agenda:          newAgendaBoard(),
		detail:          tview.NewTextView(),
		statusLeft:      tview.NewTextView(),
		statusMid:       tview.NewTextView(),
		statusRight:     tview.NewTextView(),
		hints:           tview.NewTextView(),
		detailOn:        true,
		leftWidth:       defaultLeftWidth,
		viewMode:        viewMonth,
		anchor:          model.DayStart(now),
		weekStartMonday: true,
		calColors:       map[string]calColor{},
		itemColors:      map[string]calColor{},
		folders:         map[string]bool{},
		stickyDone:      map[string]bool{},
		hidden:          map[string]bool{},
	}
	a.ctx, a.cancel = context.WithCancel(context.Background())
	return a
}

func (a *app) build() {
	a.calendars.ShowSecondaryText(false).SetHighlightFullLine(true)
	a.tasklists.ShowSecondaryText(false).SetHighlightFullLine(true)
	a.agendaList.ShowSecondaryText(false).SetHighlightFullLine(true)
	// Selections use reverse video so they stay legible on any terminal theme.
	// (The tview default derives the selected foreground from the primitive
	// background, which we set to the terminal default — leaving it illegible.)
	a.calendars.SetSelectedStyle(selectionStyle)
	a.tasklists.SetSelectedStyle(selectionStyle)
	a.agendaList.SetSelectedStyle(selectionStyle)
	a.detail.SetDynamicColors(true).SetWrap(true)
	a.statusLeft.SetDynamicColors(true)
	a.statusMid.SetDynamicColors(true).SetTextAlign(tview.AlignCenter)
	a.statusRight.SetDynamicColors(true).SetTextAlign(tview.AlignRight)
	a.hints.SetWrap(false) // plain text so [ and ] render literally; always-visible controls

	decorate(a.calendars.Box, "c Calendars")
	decorate(a.tasklists.Box, "t Tasks")
	decorate(a.agendaList.Box, "a Agenda")
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
	a.timegrid.onSelectEvent = func(it model.AgendaItem) { a.setAgendaItemDetail(it) }
	a.timegrid.onExit = func() { a.setFocus(a.calendars) }

	// Color items by their calendar's (synced) color, falling back to the default
	// event/task colors when a calendar has none.
	a.month.itemColor = a.agendaItemColor
	a.agenda.itemColor = a.agendaItemColor
	a.timegrid.occColor = a.occurrenceColor
	a.timegrid.taskColor = a.todoColor
	// Folder markers (▸) for tasks with incomplete children, consistent with the tree.
	a.month.isFolder = a.isFolder
	a.timegrid.isFolder = a.isFolder
	a.calendars.SetSelectedFunc(func(int, string, string, rune) { a.setFocus(a.calendarPrimitive()) })
	a.tasklists.SetChangedFunc(func(index int, _, _ string, _ rune) {
		// Rebuilding the panel briefly parks the selection at index 0; ignore
		// those transient events so they don't look like a real list change.
		if a.suspendTree {
			return
		}
		// Build for the callback's index argument, not GetCurrentItem: tview fires
		// changed BEFORE updating the current item, so GetCurrentItem is stale here
		// and buildTree() would rebuild the previously-selected list. Switching
		// lists also drops the sticky just-completed pins.
		id := a.treeListID
		if index >= 0 && index < len(a.tasklistIDs) {
			id = a.tasklistIDs[index]
			if id != a.treeListID {
				a.stickyDone = map[string]bool{}
				a.zoomUID = "" // a subtree zoom doesn't carry across lists
				a.treeListID = id
			}
		}
		a.buildTreeForList(id)
	})
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
	a.leftCol = left

	a.body = tview.NewFlex(). // default FlexColumn: side by side
					AddItem(left, a.leftWidth, 0, false).
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
	// Switching panels restores the overview if it was collapsed (accordion),
	// since the mode change focuses that overview panel. Guarded: leftCol is nil
	// during the first setMode (before layout builds it).
	if a.accordion && a.leftCol != nil {
		a.accordion = false
		a.body.ResizeItem(a.leftCol, a.leftWidth, 0)
	}
	switch m {
	case modeCalendar:
		a.showDetail(true)
		a.buildCenterCalendar()
		a.setFocus(a.calendars)
	case modeTasks:
		a.showDetail(true)
		a.center.SwitchToPage("tree")
		a.buildTree()
		a.treeListID = a.selectedTasklistID() // sync so restore events aren't seen as a list change
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
	// A pending chord prefix (e.g. `a`) claims the next key before anything else.
	if a.pendingPrefix != 0 {
		a.resolvePrefix(ev)
		return nil
	}
	// While a modal (input/form/confirm) is open it owns all keys; app shortcuts
	// must not fire, or typing letters/digits would trigger them.
	if a.modalOpen() {
		return ev
	}
	// Vim counts: 1-9 start a count and 0 extends one; the next motion repeats.
	// (Digits are free for this now that panel focus lives on c/t/a.)
	if ev.Key() == tcell.KeyRune {
		if r := ev.Rune(); (r >= '1' && r <= '9') || (r == '0' && a.pendingCount > 0) {
			a.pendingCount = a.pendingCount*10 + int(r-'0')
			if a.pendingCount > maxCount {
				a.pendingCount = maxCount
			}
			a.statusLeft.SetText(fmt.Sprintf(" [gray]count[-] %d", a.pendingCount))
			return nil
		}
	}
	// Any non-digit key ends the count. A motion applies it; otherwise it's dropped.
	count := a.pendingCount
	a.pendingCount = 0
	// hjkl are aliases for the arrow keys in every pane (so movement is uniform,
	// incl. the overview lists); arrow keys themselves are only intercepted here to
	// apply a repeat count. A pending count applies to either.
	if arrow, isLetter, ok := motionArrow(ev); ok && (isLetter || count > 1) {
		n := count
		if n < 1 {
			n = 1
		}
		a.repeatKey(tcell.NewEventKey(arrow, 0, tcell.ModNone), n)
		if count > 1 {
			a.updateStatus()
		}
		return nil
	}
	switch ev.Key() {
	case tcell.KeyTab:
		a.setMode((a.mode + 1) % 3)
		return nil
	case tcell.KeyBacktab:
		a.setMode((a.mode + 2) % 3)
		return nil
	case tcell.KeyLeft:
		if ev.Modifiers()&tcell.ModCtrl != 0 {
			a.resizeLeft(-leftWidthStep)
			return nil
		}
	case tcell.KeyRight:
		if ev.Modifiers()&tcell.ModCtrl != 0 {
			a.resizeLeft(leftWidthStep)
			return nil
		}
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
		case 'c':
			a.setMode(modeCalendar)
			return nil
		case 't':
			a.setMode(modeTasks)
			return nil
		case 'a':
			a.setMode(modeAgenda)
			return nil
		case 'i':
			a.startPrefix('i')
			return nil
		case 'g':
			a.startPrefix('g')
			return nil
		case 'G':
			a.gotoBottom(count)
			return nil
		case 'z':
			if a.mode == modeTasks {
				a.startPrefix('z')
			} else {
				a.flash("fold: Tasks view only")
			}
			return nil
		case 's':
			if a.mode == modeTasks {
				a.startPrefix('s')
			} else {
				a.flash("set: Tasks view only")
			}
			return nil
		case 'y':
			a.yankTask()
			return nil
		case 'p':
			a.pasteTask()
			return nil
		case '/':
			a.openSearch()
			return nil
		case 'n':
			a.searchNext(1)
			return nil
		case 'N':
			a.searchNext(-1)
			return nil
		case 'e':
			a.editSelected()
			return nil
		case 'd':
			a.deleteContextual()
			return nil
		case ' ':
			// In Calendar mode Space checks off the drilled task if one is selected;
			// otherwise (navigating days, or on an event) it toggles the highlighted
			// calendar's visibility. Elsewhere it toggles the selected task.
			if a.mode == modeCalendar {
				if t, ok := a.currentTarget(); ok && t.isTodo {
					a.toggleComplete()
				} else {
					a.toggleCalendarVisibility()
				}
			} else {
				a.toggleComplete()
			}
			return nil
		case 'u':
			a.undoLast()
			return nil
		case 'r':
			// Convenience alias for :sync (the command form echoes in the status bar).
			a.triggerSync()
			return nil
		case ':':
			a.openCommandLine("")
			return nil
		case '?':
			a.showHelp()
			return nil
		case '+':
			if a.timeGridActive() {
				a.zoomHour(1)
			} else {
				a.setAccordion(true)
			}
			return nil
		case '-':
			if a.timeGridActive() {
				a.zoomHour(-1)
			} else {
				a.setAccordion(false)
			}
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
		case 'f':
			if a.mode == modeCalendar {
				a.shiftAnchor(1)
				return nil
			}
		case 'b':
			if a.mode == modeCalendar {
				a.shiftAnchor(-1)
				return nil
			}
		case '[':
			a.cycleCalendar(-1)
			return nil
		case ']':
			a.cycleCalendar(1)
			return nil
		case '{':
			a.cycleTasklist(-1)
			return nil
		case '}':
			a.cycleTasklist(1)
			return nil
		case '>':
			if a.mode == modeTasks {
				a.zoomInTree()
				return nil
			}
		case '<':
			if a.mode == modeTasks {
				a.zoomOutTree()
				return nil
			}
		}
	}
	return ev
}

// shiftAnchor moves the calendar by one view-period and re-renders.
func (a *app) shiftAnchor(delta int) {
	// If drilled in, stay drilled across the period change (re-enter on the new
	// period's day). f/b is the intended way to move days/weeks/months while drilled.
	wasDrilled := false
	if g, ok := a.calendarPrimitive().(calGrid); ok {
		_, wasDrilled, _ = g.drillState()
	}
	switch a.viewMode {
	case viewMonth:
		a.anchor = a.anchor.AddDate(0, delta, 0)
	case viewWeek:
		a.anchor = a.anchor.AddDate(0, 0, 7*delta)
	default:
		a.anchor = a.anchor.AddDate(0, 0, delta)
	}
	a.buildCenterCalendar()
	if wasDrilled {
		if g, ok := a.calendarPrimitive().(calGrid); ok {
			g.reDrill(a.anchor, 0)
		}
		a.setFocus(a.calendarPrimitive())
	} else {
		a.refocusCalendar()
	}
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
// from the grid too. Space toggles the highlighted calendar's visibility.
// cycleCalendar moves the Calendars-overview highlight by delta (wrapping). It's
// global (any mode): the left column always shows the Calendars list, and the
// highlight is the target for hide/show, :calendar, and event creation.
func (a *app) cycleCalendar(delta int) {
	n := a.calendars.GetItemCount()
	if n == 0 {
		return
	}
	i := (a.calendars.GetCurrentItem() + delta + n) % n
	a.calendars.SetCurrentItem(i)
}

// cycleTasklist moves the Tasks-overview highlight by delta (wrapping), the
// task-list counterpart to cycleCalendar. It cycles within the real list ids so
// the "(no task lists)" placeholder isn't selectable; the list's changed-callback
// rebuilds the tree when Tasks mode is showing.
func (a *app) cycleTasklist(delta int) {
	n := len(a.tasklistIDs)
	if n == 0 {
		return
	}
	i := (a.tasklists.GetCurrentItem() + delta + n) % n
	a.tasklists.SetCurrentItem(i)
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
