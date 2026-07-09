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

// TestMonthGridOverflowIndicatorReflectsBelow: the "+N more" line counts only
// items below the window, so it shrinks as you drill down and disappears once
// the bottommost item is selected.
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

	// At the top of the list, there are items below → indicator shows.
	cv.eventIndex = 0
	if !strings.Contains(renderPrimitive(t, cv, 140, 44), " more") {
		t.Error("at the top of an overflowing day, expected a +N more indicator")
	}

	// Drilled to the last item, nothing is below → indicator gone.
	cv.eventIndex = len(items) - 1
	if strings.Contains(renderPrimitive(t, cv, 140, 44), " more") {
		t.Error("at the bottom item, the +N more indicator should disappear")
	}
}
