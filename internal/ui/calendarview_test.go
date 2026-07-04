package ui

import (
	"strings"
	"testing"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"

	"github.com/littekge/LazyPlanner/internal/model"
)

// drawCalendar renders cv onto an in-memory screen of the given size and returns
// the visible text, so the custom drawing can be asserted headlessly.
func drawCalendar(t *testing.T, cv *calendarView, w, h int) string {
	t.Helper()
	screen := tcell.NewSimulationScreen("")
	if err := screen.Init(); err != nil {
		t.Fatalf("init simulation screen: %v", err)
	}
	defer screen.Fini()
	screen.SetSize(w, h)
	cv.SetRect(0, 0, w, h)
	cv.Draw(screen)
	screen.Show()

	cells, cw, ch := screen.GetContents()
	var b strings.Builder
	for row := 0; row < ch; row++ {
		for col := 0; col < cw; col++ {
			if cell := cells[row*cw+col]; len(cell.Runes) > 0 {
				b.WriteRune(cell.Runes[0])
			} else {
				b.WriteByte(' ')
			}
		}
		b.WriteByte('\n')
	}
	return b.String()
}

func TestCalendarViewDrawsMonth(t *testing.T) {
	cv := newCalendarView()
	anchor := time.Date(2026, 7, 4, 0, 0, 0, 0, time.UTC)
	ev := &model.Event{Summary: "Team Standup", Start: time.Date(2026, 7, 4, 9, 0, 0, 0, time.UTC)}
	items := map[string][]model.AgendaItem{
		"2026-07-04": {{Start: ev.Start, Title: "Team Standup", Event: ev}},
	}
	cv.setData(model.MonthGrid(anchor, true), items, time.July, anchor, anchor, true)

	out := drawCalendar(t, cv, 140, 30)

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

	out := drawCalendar(t, cv, 140, 12)
	if !strings.Contains(out, "Mon") || !strings.Contains(out, "Sun") {
		t.Errorf("week render missing weekday headers:\n%s", out)
	}
}

func TestCalendarViewArrowMovesSelection(t *testing.T) {
	cv := newCalendarView()
	anchor := time.Date(2026, 7, 4, 0, 0, 0, 0, time.UTC)
	cv.setData(model.MonthGrid(anchor, true), nil, time.July, anchor, anchor, true)

	var got time.Time
	cv.onSelect = func(day time.Time) { got = day }

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
