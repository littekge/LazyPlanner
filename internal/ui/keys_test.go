package ui

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"

	"github.com/littekge/LazyPlanner/internal/store"
)

func runeKey(r rune) *tcell.EventKey { return tcell.NewEventKey(tcell.KeyRune, r, tcell.ModNone) }

// newRootedTestApp builds a writable app with the top-level Pages root wired, so
// modal/overlay paths (which-key, forms) work in tests.
func newRootedTestApp(t *testing.T, now time.Time) *app {
	t.Helper()
	a := newWritableTestApp(t, now)
	a.root = tview.NewPages()
	a.root.AddPage(pageMain, a.layout(), true, true)
	a.setMode(modeTasks)
	return a
}

// TestBracketAndBraceCycleGlobally: [ / ] cycle the calendar highlight and
// { / } cycle the task-list highlight, from any mode (here Agenda), wrapping.
func TestBracketAndBraceCycleGlobally(t *testing.T) {
	a := newRootedTestApp(t, time.Date(2026, 7, 5, 12, 0, 0, 0, time.UTC))
	// Ensure a second task list exists so cycling has somewhere to go.
	if err := a.store.CreateCalendarLocal(context.Background(), "errands", store.CalendarMeta{DisplayName: "Errands"}, []string{"VTODO"}); err != nil {
		t.Fatal(err)
	}
	a.reload()
	if a.calendars.GetItemCount() < 2 || len(a.tasklistIDs) < 2 {
		t.Skipf("need 2 calendars and 2 task lists (have %d cals, %d lists)", a.calendars.GetItemCount(), len(a.tasklistIDs))
	}
	a.setMode(modeAgenda) // neither Calendar nor Tasks, to prove the keys are global

	calBefore := a.calendars.GetCurrentItem()
	a.globalKeys(runeKey(']'))
	if a.calendars.GetCurrentItem() == calBefore {
		t.Error("] did not cycle the calendar highlight from Agenda mode")
	}
	a.globalKeys(runeKey('['))
	if a.calendars.GetCurrentItem() != calBefore {
		t.Error("[ did not cycle the calendar highlight back")
	}

	tlBefore := a.tasklists.GetCurrentItem()
	a.globalKeys(runeKey('}'))
	if a.tasklists.GetCurrentItem() == tlBefore {
		t.Error("} did not cycle the task-list highlight from Agenda mode")
	}
	a.globalKeys(runeKey('{'))
	if a.tasklists.GetCurrentItem() != tlBefore {
		t.Error("{ did not cycle the task-list highlight back")
	}
}

// TestGoTopBottomInMonthGrid: gg / G work when focus is in the month grid (not
// just the overview lists) — the custom grid handles Home/End. gg lands on the
// first grid cell, G on the last.
func TestGoTopBottomInMonthGrid(t *testing.T) {
	a := newRootedTestApp(t, time.Date(2026, 7, 15, 12, 0, 0, 0, time.UTC))
	a.setMode(modeCalendar)
	a.viewMode = viewMonth
	a.buildCenterCalendar()
	a.setFocus(a.month) // dive into the grid

	weeks := a.month.weeks
	first := weeks[0][0]
	last := weeks[len(weeks)-1][6]

	a.globalKeys(runeKey('g'))
	a.globalKeys(runeKey('g'))
	if !a.month.selected.Equal(first) {
		t.Errorf("gg selected %v, want first grid day %v", a.month.selected.Format("2006-01-02"), first.Format("2006-01-02"))
	}

	a.globalKeys(runeKey('G'))
	if !a.month.selected.Equal(last) {
		t.Errorf("G selected %v, want last grid day %v", a.month.selected.Format("2006-01-02"), last.Format("2006-01-02"))
	}
}

func TestPrefixShowsWhichKeyThenCancels(t *testing.T) {
	a := newRootedTestApp(t, time.Date(2026, 7, 5, 12, 0, 0, 0, time.UTC))

	a.startPrefix('i')
	if a.pendingPrefix != 'i' {
		t.Fatalf("pendingPrefix = %q, want 'i'", a.pendingPrefix)
	}
	if !a.root.HasPage(pageWhichKey) {
		t.Error("which-key popup not shown after prefix")
	}

	// Esc cancels: clears the prefix and removes the popup.
	a.resolvePrefix(tcell.NewEventKey(tcell.KeyEscape, 0, tcell.ModNone))
	if a.pendingPrefix != 0 {
		t.Error("prefix not cleared on Esc")
	}
	if a.root.HasPage(pageWhichKey) {
		t.Error("which-key popup not removed on cancel")
	}
}

func TestChordCreatesTask(t *testing.T) {
	a := newRootedTestApp(t, time.Date(2026, 7, 5, 12, 0, 0, 0, time.UTC))
	// Select the first task list so a top-level task has a home.
	if a.tasklists.GetItemCount() == 0 {
		t.Skip("fixture has no task lists")
	}
	before := len(a.store.Todos())

	// `i` then `t` opens the quick-add task prompt.
	a.startPrefix('i')
	a.resolvePrefix(runeKey('t'))
	if a.pendingPrefix != 0 {
		t.Error("prefix should be cleared after completing the chord")
	}
	// The quick-add input modal should now be open.
	if !a.modalOpen() {
		t.Fatal("quick-add prompt did not open on `it`")
	}
	// Command view echoes the chord.
	if got := a.statusMid.GetText(true); got == "" {
		t.Error("command view not echoed after a chord")
	}
	_ = before
}

func TestUnknownChordFlashesAndClears(t *testing.T) {
	a := newRootedTestApp(t, time.Date(2026, 7, 5, 12, 0, 0, 0, time.UTC))
	a.startPrefix('i')
	a.resolvePrefix(runeKey('z')) // no `iz` action
	if a.pendingPrefix != 0 {
		t.Error("prefix not cleared after an unknown continuation")
	}
	if a.root.HasPage(pageWhichKey) {
		t.Error("which-key popup left open after an unknown continuation")
	}
	if got := a.statusLeft.GetText(true); got == "" {
		t.Error("expected a flash for the unknown chord")
	}
}

// focusableList builds a standalone list with n items, used to exercise the
// count/gg/G navigation without depending on fixture item counts.
func focusableList(n int) *tview.List {
	lst := tview.NewList().ShowSecondaryText(false)
	for i := 0; i < n; i++ {
		lst.AddItem(fmt.Sprintf("item %d", i), "", 0, nil)
	}
	return lst
}

func TestCountPrefixRepeatsMotion(t *testing.T) {
	a := newRootedTestApp(t, time.Date(2026, 7, 5, 12, 0, 0, 0, time.UTC))
	lst := focusableList(6)
	a.tv.SetFocus(lst)
	lst.SetCurrentItem(0)

	// "3" accumulates a count; the next Down arrow moves three rows.
	a.globalKeys(runeKey('3'))
	if a.pendingCount != 3 {
		t.Fatalf("pendingCount = %d, want 3", a.pendingCount)
	}
	a.globalKeys(tcell.NewEventKey(tcell.KeyDown, 0, tcell.ModNone))
	if a.pendingCount != 0 {
		t.Error("count should reset after the motion")
	}
	if got := lst.GetCurrentItem(); got != 3 {
		t.Errorf("after 3j-style move, current item = %d, want 3", got)
	}
}

func TestHJKLMovesOverviewList(t *testing.T) {
	a := newRootedTestApp(t, time.Date(2026, 7, 5, 12, 0, 0, 0, time.UTC))
	lst := focusableList(4) // stands in for an overview list (arrows-only natively)
	a.tv.SetFocus(lst)
	lst.SetCurrentItem(0)

	// j / k move the highlight even though tview.List binds only the arrow keys.
	a.globalKeys(runeKey('j'))
	if got := lst.GetCurrentItem(); got != 1 {
		t.Fatalf("after j, current = %d, want 1", got)
	}
	a.globalKeys(runeKey('j'))
	if got := lst.GetCurrentItem(); got != 2 {
		t.Fatalf("after jj, current = %d, want 2", got)
	}
	a.globalKeys(runeKey('k'))
	if got := lst.GetCurrentItem(); got != 1 {
		t.Errorf("after k, current = %d, want 1", got)
	}
}

func TestCountAppliesToLetterMotion(t *testing.T) {
	a := newRootedTestApp(t, time.Date(2026, 7, 5, 12, 0, 0, 0, time.UTC))
	lst := focusableList(6)
	a.tv.SetFocus(lst)
	lst.SetCurrentItem(0)

	a.globalKeys(runeKey('3'))
	a.globalKeys(runeKey('j')) // 3j via the letter alias
	if got := lst.GetCurrentItem(); got != 3 {
		t.Errorf("after 3j, current = %d, want 3", got)
	}
}

func TestGotoTopAndBottom(t *testing.T) {
	a := newRootedTestApp(t, time.Date(2026, 7, 5, 12, 0, 0, 0, time.UTC))
	lst := focusableList(6)
	a.tv.SetFocus(lst)

	lst.SetCurrentItem(2)
	a.gotoBottom(0) // G with no count → last item
	if got := lst.GetCurrentItem(); got != 5 {
		t.Errorf("G current item = %d, want 5 (last)", got)
	}
	a.gotoTop() // gg → first item
	if got := lst.GetCurrentItem(); got != 0 {
		t.Errorf("gg current item = %d, want 0", got)
	}
	a.gotoBottom(3) // 3G → third item (index 2)
	if got := lst.GetCurrentItem(); got != 2 {
		t.Errorf("3G current item = %d, want 2", got)
	}
}

func TestFoldAllAndToggle(t *testing.T) {
	now := time.Date(2026, 7, 5, 9, 0, 0, 0, time.UTC)
	a := newWritableTestApp(t, now)
	a.setMode(modeTasks)
	calID := a.selectedTasklistID()

	a.createTask(calID, "", "Parent")
	parent := todoBySummary(a.store, "Parent")
	a.createTask(calID, parent.UID, "Child")
	a.buildTree()

	pnode := findTreeNode(a.tree.GetRoot(), parent.UID)
	if pnode == nil {
		t.Fatal("parent node not found in tree")
	}
	if !pnode.IsExpanded() {
		t.Fatal("folder should start expanded")
	}

	a.setFoldAll(false)
	if pnode.IsExpanded() {
		t.Error("zM should collapse every folder")
	}
	a.setFoldAll(true)
	if !pnode.IsExpanded() {
		t.Error("zR should expand every folder")
	}

	// za toggles just the current node.
	a.tree.SetCurrentNode(pnode)
	a.toggleFold()
	if pnode.IsExpanded() {
		t.Error("za should collapse the current (expanded) folder")
	}
}

func TestToggleCalendarVisibility(t *testing.T) {
	now := time.Date(2026, 7, 5, 9, 0, 0, 0, time.UTC)
	a := newWritableTestApp(t, now)
	a.setMode(modeCalendar)
	if a.calendars.GetItemCount() == 0 {
		t.Skip("fixture has no calendars")
	}
	var savedHidden []string
	a.saveState = func(_ int, hidden []string) { savedHidden = hidden }

	a.calendars.SetCurrentItem(0)
	id := a.selectedCalendarID()
	if id == "" {
		t.Fatal("no calendar id for row 0")
	}

	a.toggleCalendarVisibility()
	if !a.hidden[id] {
		t.Errorf("calendar %q should be hidden after toggle", id)
	}
	if len(savedHidden) != 1 || savedHidden[0] != id {
		t.Errorf("saved hidden = %v, want [%s]", savedHidden, id)
	}
	main, _ := a.calendars.GetItemText(0)
	if !strings.Contains(main, "(hidden)") {
		t.Errorf("hidden calendar row = %q, want a (hidden) marker", main)
	}

	a.toggleCalendarVisibility() // toggle back
	if a.hidden[id] {
		t.Errorf("calendar %q should be visible again", id)
	}
	if len(savedHidden) != 0 {
		t.Errorf("saved hidden = %v, want empty after un-hiding", savedHidden)
	}
}

func TestHiddenCalendarDropsFromAgenda(t *testing.T) {
	now := time.Date(2026, 7, 5, 9, 0, 0, 0, time.UTC)
	a := newWritableTestApp(t, now)

	from := time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2100, 1, 1, 0, 0, 0, 0, time.UTC)
	base, _ := a.store.EventOccurrences(from, to)
	if len(base) == 0 {
		t.Skip("fixture has no events")
	}
	// Hide every calendar; no occurrences should survive the filter.
	for _, cal := range a.store.Calendars() {
		a.hidden[cal.ID] = true
	}
	got, _ := a.store.EventOccurrencesVisible(from, to, a.hidden)
	if len(got) != 0 {
		t.Errorf("hiding all calendars left %d occurrences (of %d)", len(got), len(base))
	}
}
