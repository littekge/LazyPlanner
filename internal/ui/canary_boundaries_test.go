package ui

import (
	"testing"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"

	"github.com/littekge/LazyPlanner/internal/model"
)

// TestClampIndexBoundaries closes a pass-11 escaped-canary hole: clampIndex's
// upper bound (i >= n → n-1) had no test at the i == n boundary, so flipping it
// to i > n (returning the out-of-bounds n) escaped the suite. clampIndex backs
// vim-count nav and drilled-event selection, where a count landing exactly on the
// list length (2j on a 2-item list) hits i == n.
func TestClampIndexBoundaries(t *testing.T) {
	cases := []struct {
		i, n, want int
	}{
		{-1, 3, 0}, // below range clamps to 0
		{0, 3, 0},
		{2, 3, 2}, // last valid index
		{3, 3, 2}, // i == n: the escaped-canary boundary — must clamp to n-1
		{5, 3, 2}, // above range clamps to n-1
		{1, 1, 0}, // i == n with a single-item list
	}
	for _, c := range cases {
		if got := clampIndex(c.i, c.n); got != c.want {
			t.Errorf("clampIndex(%d, %d) = %d, want %d", c.i, c.n, got, c.want)
		}
	}
}

// TestCalendarViewEventDrillJKBoundaries closes a pass-11 escaped-canary hole: no
// test stepped Down/Up to the lower/upper boundary of the month-grid event
// drill, so flipping the down guard (< len(items)-1 → <= len(items)-1) let
// eventIndex advance one past the last item.
//
// This used to also drive the guard via raw KeyRune('j'/'k') events, back when
// calendarView special-cased those runes itself. Matrix finding #19 established
// that letter motions never reach a view's own InputHandler in the running
// app — the global key layer (motionArrow in keys.go) always translates them to
// arrow keys first via Application.SetInputCapture(a.globalKeys), which runs
// before any focused primitive sees the event — so that rune case was dead code
// and has been removed (see TestCalendarViewLetterMotionsAreNotHandledDirectly).
// Only the arrow-key path is exercised here now; it's the one hjkl is always
// translated into before reaching the grid.
func TestCalendarViewEventDrillJKBoundaries(t *testing.T) {
	mk := func(title string, h int) model.AgendaItem {
		e := &model.Event{Summary: title, Start: time.Date(2026, 7, 4, h, 0, 0, 0, time.Local)}
		return model.AgendaItem{Start: e.Start, Title: title, Event: e}
	}
	cv := newCalendarView()
	day := time.Date(2026, 7, 4, 0, 0, 0, 0, time.Local)
	items := []model.AgendaItem{mk("A", 8), mk("B", 10), mk("C", 12)}
	cv.setData(model.MonthGrid(day, true), map[string][]model.AgendaItem{dayKey(day): items}, day.Month(), day, day, true)
	handle := cv.InputHandler()
	handle(tcell.NewEventKey(tcell.KeyEnter, 0, tcell.ModNone), func(tview.Primitive) {}) // drill in at index 0

	press := func(k tcell.Key) {
		handle(tcell.NewEventKey(k, 0, tcell.ModNone), func(tview.Primitive) {})
	}

	// Step down well past the last item; the index must stop at the last (2).
	for i := 0; i < 5; i++ {
		press(tcell.KeyDown)
	}
	if cv.eventIndex != len(items)-1 {
		t.Errorf("eventIndex = %d after stepping past the end, want %d (stop at last)", cv.eventIndex, len(items)-1)
	}

	// Step up well past the first item; the index must stop at 0.
	for i := 0; i < 5; i++ {
		press(tcell.KeyUp)
	}
	if cv.eventIndex != 0 {
		t.Errorf("eventIndex = %d after stepping past the start, want 0", cv.eventIndex)
	}
}
