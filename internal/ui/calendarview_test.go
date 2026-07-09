package ui

import (
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
