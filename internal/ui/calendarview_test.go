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
