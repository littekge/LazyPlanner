package ui

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"

	"github.com/littekge/LazyPlanner/internal/model"
)

func TestCalendarViewDrawsMonth(t *testing.T) {
	cv := newCalendarView()
	anchor := time.Date(2026, 7, 4, 0, 0, 0, 0, time.UTC)
	ev := &model.Event{Summary: "Team Standup", Start: time.Date(2026, 7, 4, 9, 0, 0, 0, time.UTC)}
	items := map[string][]model.AgendaItem{
		"2026-07-04": {{Start: ev.Start, Title: "Team Standup", Event: ev}},
	}
	cv.setData(model.MonthGrid(anchor, true), items, time.July, anchor, anchor, true)

	out := renderPrimitive(t, cv, 140, 30)

	for _, want := range []string{"Mon", "Sun", "15", "Team Standup"} {
		if !strings.Contains(out, want) {
			t.Errorf("month render missing %q:\n%s", want, out)
		}
	}
}

func TestCalendarViewDrawsWeek(t *testing.T) {
	cv := newCalendarView()
	anchor := time.Date(2026, 7, 4, 0, 0, 0, 0, time.UTC)
	cv.setData([][]time.Time{model.Week(anchor, true)}, nil, 0, anchor, anchor, true)

	out := renderPrimitive(t, cv, 140, 12)
	if !strings.Contains(out, "Mon") || !strings.Contains(out, "Sun") {
		t.Errorf("week render missing weekday headers:\n%s", out)
	}
}

func TestCalendarViewArrowMovesSelection(t *testing.T) {
	cv := newCalendarView()
	anchor := time.Date(2026, 7, 4, 0, 0, 0, 0, time.UTC)
	cv.setData(model.MonthGrid(anchor, true), nil, time.July, anchor, anchor, true)

	var got time.Time
	cv.onSelectDay = func(day time.Time) { got = day }

	handle := cv.InputHandler()
	handle(tcell.NewEventKey(tcell.KeyRight, 0, tcell.ModNone), func(tview.Primitive) {})
	if want := anchor.AddDate(0, 0, 1); !got.Equal(want) {
		t.Errorf("Right selected %s, want %s", got.Format("2006-01-02"), want.Format("2006-01-02"))
	}

	handle(tcell.NewEventKey(tcell.KeyDown, 0, tcell.ModNone), func(tview.Primitive) {})
	if want := anchor.AddDate(0, 0, 7); !got.Equal(want) {
		t.Errorf("Down selected %s, want %s", got.Format("2006-01-02"), want.Format("2006-01-02"))
	}
}

// TestCalendarViewLetterMotionsAreNotHandledDirectly locks matrix finding #19:
// hjkl letter motions on the calendar grid must be translated to arrow keys
// exactly once, upstream, by the global key layer (motionArrow in keys.go,
// which always runs first — Application.SetInputCapture(a.globalKeys) sees
// every key before any focused primitive's own InputHandler does, and it
// returns nil after translating a letter motion, so a raw letter rune never
// reaches here in the running app). The view itself must not also special-case
// a raw letter rune, or the two translations could silently diverge.
func TestCalendarViewLetterMotionsAreNotHandledDirectly(t *testing.T) {
	cv := newCalendarView()
	anchor := time.Date(2026, 7, 4, 0, 0, 0, 0, time.UTC)
	cv.setData(model.MonthGrid(anchor, true), nil, time.July, anchor, anchor, true)

	moved := false
	cv.onSelectDay = func(time.Time) { moved = true }

	handle := cv.InputHandler()
	for _, r := range []rune{'h', 'j', 'k', 'l'} {
		moved = false
		handle(tcell.NewEventKey(tcell.KeyRune, r, tcell.ModNone), func(tview.Primitive) {})
		if moved {
			t.Errorf("calendarView.InputHandler moved the selection on raw rune %q; hjkl must reach it only pre-translated to arrow keys by the global key layer", string(r))
		}
	}
}

// TestCalendarViewEventModeLetterMotionsAreNotHandledDirectly is the event-mode
// counterpart of TestCalendarViewLetterMotionsAreNotHandledDirectly: j/k must
// not cycle the drilled event list on a raw rune either.
func TestCalendarViewEventModeLetterMotionsAreNotHandledDirectly(t *testing.T) {
	cv := newCalendarView()
	day := time.Date(2026, 7, 4, 0, 0, 0, 0, time.Local)
	mk := func(title string, h int) model.AgendaItem {
		e := &model.Event{Summary: title, Start: time.Date(2026, 7, 4, h, 0, 0, 0, time.Local)}
		return model.AgendaItem{Start: e.Start, Title: title, Event: e}
	}
	items := map[string][]model.AgendaItem{dayKey(day): {mk("A", 8), mk("B", 10), mk("C", 12)}}
	cv.setData(model.MonthGrid(day, true), items, day.Month(), day, day, true)

	var got string
	cv.onSelectEvent = func(it model.AgendaItem) { got = it.Title }
	handle := cv.InputHandler()
	handle(tcell.NewEventKey(tcell.KeyEnter, 0, tcell.ModNone), func(tview.Primitive) {}) // event mode, index 0 = A

	got = ""
	handle(tcell.NewEventKey(tcell.KeyRune, 'j', tcell.ModNone), func(tview.Primitive) {})
	if got != "" {
		t.Errorf("calendarView event-mode InputHandler moved on raw rune 'j' (got %q); hjkl must reach it only pre-translated by the global key layer", got)
	}

	got = ""
	handle(tcell.NewEventKey(tcell.KeyRune, 'k', tcell.ModNone), func(tview.Primitive) {})
	if got != "" {
		t.Errorf("calendarView event-mode InputHandler moved on raw rune 'k' (got %q); hjkl must reach it only pre-translated by the global key layer", got)
	}
}

// TestCalendarViewEventModeHomeEnd: in event mode, Home/End (gg/G) jump to the
// first / last event of the selected day.
func TestCalendarViewEventModeHomeEnd(t *testing.T) {
	cv := newCalendarView()
	day := time.Date(2026, 7, 4, 0, 0, 0, 0, time.Local)
	mk := func(title string, h int) model.AgendaItem {
		e := &model.Event{Summary: title, Start: time.Date(2026, 7, 4, h, 0, 0, 0, time.Local)}
		return model.AgendaItem{Start: e.Start, Title: title, Event: e}
	}
	items := map[string][]model.AgendaItem{dayKey(day): {mk("A", 8), mk("B", 10), mk("C", 12)}}
	cv.setData(model.MonthGrid(day, true), items, day.Month(), day, day, true)

	var got string
	cv.onSelectEvent = func(it model.AgendaItem) { got = it.Title }
	handle := cv.InputHandler()
	handle(tcell.NewEventKey(tcell.KeyEnter, 0, tcell.ModNone), func(tview.Primitive) {}) // enter event mode (index 0 = A)

	handle(tcell.NewEventKey(tcell.KeyEnd, 0, tcell.ModNone), func(tview.Primitive) {})
	if got != "C" {
		t.Errorf("End selected %q, want last event C", got)
	}
	handle(tcell.NewEventKey(tcell.KeyHome, 0, tcell.ModNone), func(tview.Primitive) {})
	if got != "A" {
		t.Errorf("Home selected %q, want first event A", got)
	}
}

// TestMonthGridScrollsToDrilledOverflowItem: when a day has more items than fit,
// drilling to one in the overflow region keeps it visible and highlighted
// (reverse), rather than leaving it hidden behind "+N more".
func TestMonthGridScrollsToDrilledOverflowItem(t *testing.T) {
	cv := newCalendarView()
	day := time.Date(2026, 7, 15, 0, 0, 0, 0, time.Local)
	var items []model.AgendaItem
	for i := 0; i < 8; i++ {
		s := fmt.Sprintf("Task%d", i)
		items = append(items, model.AgendaItem{Title: s, Todo: &model.Todo{UID: s, Summary: s}})
	}
	cv.setData(model.MonthGrid(day, true), map[string][]model.AgendaItem{dayKey(day): items}, day.Month(), day, day, true)
	cv.eventMode = true
	cv.eventIndex = 6 // deep in the overflow region

	cells, cw, ch := drawCells(t, cv, 140, 44)

	// The drilled item's title is on screen...
	row := -1
	col := -1
	for r := 0; r < ch && row < 0; r++ {
		for c := 0; c+5 <= cw; c++ {
			run := make([]rune, 0, 5)
			for k := 0; k < 5; k++ {
				if rs := cells[r*cw+c+k].Runes; len(rs) > 0 {
					run = append(run, rs[0])
				} else {
					run = append(run, ' ')
				}
			}
			if string(run) == "Task6" {
				row, col = r, c
				break
			}
		}
	}
	if row < 0 {
		t.Fatal("drilled overflow item Task6 not rendered — it stayed hidden behind +N more")
	}
	// ...and drawn highlighted (reverse video).
	_, _, attr := cells[row*cw+col].Style.Decompose()
	if attr&tcell.AttrReverse == 0 {
		t.Error("drilled overflow item is not highlighted (no reverse attribute)")
	}
}

// rowStrings renders cv and returns each screen row as a string, so a test can
// reason about which row a piece of text (an item, a "+N more" marker) lands on.
func rowStrings(t *testing.T, cv *calendarView) []string {
	t.Helper()
	cells, cw, ch := drawCells(t, cv, 140, 44)
	rows := make([]string, ch)
	for r := 0; r < ch; r++ {
		var b strings.Builder
		for c := 0; c < cw; c++ {
			if rs := cells[r*cw+c].Runes; len(rs) > 0 {
				b.WriteRune(rs[0])
			} else {
				b.WriteByte(' ')
			}
		}
		rows[r] = b.String()
	}
	return rows
}

// firstRowContaining returns the index of the first row whose text contains sub,
// or -1 if none do.
func firstRowContaining(rows []string, sub string) int {
	for i, r := range rows {
		if strings.Contains(r, sub) {
			return i
		}
	}
	return -1
}

// TestMonthGridOverflowIndicatorReflectsBelow: the below-the-window "+N more"
// line counts only items below the window, so it shrinks as you drill down and
// disappears (drops below the last visible item) once the bottommost item is
// selected.
func TestMonthGridOverflowIndicatorReflectsBelow(t *testing.T) {
	cv := newCalendarView()
	day := time.Date(2026, 7, 15, 0, 0, 0, 0, time.Local)
	var items []model.AgendaItem
	for i := 0; i < 8; i++ {
		s := fmt.Sprintf("Task%d", i)
		items = append(items, model.AgendaItem{Title: s, Todo: &model.Todo{UID: s, Summary: s}})
	}
	weeks := model.MonthGrid(day, true)
	cv.setData(weeks, map[string][]model.AgendaItem{dayKey(day): items}, day.Month(), day, day, true)
	cv.eventMode = true

	// At the top of the list, items are below and none above: a "+N more" marker
	// sits below the first item, and the last item isn't on screen yet.
	cv.eventIndex = 0
	rows := rowStrings(t, cv)
	moreRow := firstRowContaining(rows, " more")
	task0Row := firstRowContaining(rows, "Task0")
	if moreRow < 0 || task0Row < 0 {
		t.Fatalf("at the top: expected Task0 and a +N more marker (Task0 row %d, more row %d)", task0Row, moreRow)
	}
	if moreRow <= task0Row {
		t.Errorf("at the top the +N more marker should be below the first item (Task0 row %d, more row %d)", task0Row, moreRow)
	}
	if firstRowContaining(rows, "Task7") >= 0 {
		t.Error("at the top the last item (Task7) should be scrolled out below the window")
	}

	// Drilled to the last item: nothing is below, so the below-indicator is gone
	// — the last item now occupies the bottom row of the cell.
	cv.eventIndex = len(items) - 1
	rows = rowStrings(t, cv)
	task7Row := firstRowContaining(rows, "Task7")
	belowMoreRow := -1
	if task7Row >= 0 {
		for i := task7Row + 1; i < len(rows); i++ {
			if strings.Contains(rows[i], " more") {
				belowMoreRow = i
				break
			}
		}
	}
	if belowMoreRow >= 0 {
		t.Error("at the bottom item, the below-the-window +N more indicator should disappear")
	}
}

// TestMonthGridTopOverflowIndicator: once a drilled day has scrolled down far
// enough to push items off the top of its cell, a "+N more" marker appears
// above the first visible item (mirroring the below-the-window marker).
func TestMonthGridTopOverflowIndicator(t *testing.T) {
	cv := newCalendarView()
	day := time.Date(2026, 7, 15, 0, 0, 0, 0, time.Local)
	var items []model.AgendaItem
	for i := 0; i < 8; i++ {
		s := fmt.Sprintf("Task%d", i)
		items = append(items, model.AgendaItem{Title: s, Todo: &model.Todo{UID: s, Summary: s}})
	}
	cv.setData(model.MonthGrid(day, true), map[string][]model.AgendaItem{dayKey(day): items}, day.Month(), day, day, true)
	cv.eventMode = true
	cv.eventIndex = len(items) - 1 // scrolled all the way down

	rows := rowStrings(t, cv)
	moreRow := firstRowContaining(rows, " more")
	if moreRow < 0 {
		t.Fatal("expected a +N more marker at the top when items are hidden above")
	}
	// The first hidden item (Task0) must be off screen, and the marker must sit
	// above whatever item now leads the visible window.
	if firstRowContaining(rows, "Task0") >= 0 {
		t.Error("Task0 should be hidden above the window when drilled to the bottom")
	}
	firstItemRow := -1
	for i := moreRow + 1; i < len(rows); i++ {
		if strings.Contains(rows[i], "Task") {
			firstItemRow = i
			break
		}
	}
	if firstItemRow < 0 {
		t.Fatal("expected an item below the top +N more marker")
	}
}
