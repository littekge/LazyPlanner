package ui

import (
	"strings"
	"testing"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"

	"github.com/littekge/LazyPlanner/internal/model"
)

// renderPrimitive draws any primitive onto an in-memory screen and returns the
// visible text, for headless assertions.
func renderPrimitive(t *testing.T, p tview.Primitive, w, h int) string {
	t.Helper()
	screen := tcell.NewSimulationScreen("")
	if err := screen.Init(); err != nil {
		t.Fatalf("init simulation screen: %v", err)
	}
	defer screen.Fini()
	screen.SetSize(w, h)
	p.SetRect(0, 0, w, h)
	p.Draw(screen)
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

func TestTimeGridDrawsDay(t *testing.T) {
	tg := newTimeGridView()
	tg.scrollHour = 8
	day := time.Date(2026, 7, 4, 0, 0, 0, 0, time.UTC)

	timedEv := &model.Event{Summary: "Team sync", Start: time.Date(2026, 7, 4, 9, 0, 0, 0, time.UTC)}
	allDayEv := &model.Event{Summary: "Holiday", AllDay: true, Start: day}
	timed := map[string][]model.Occurrence{
		dayKey(day): {{Start: timedEv.Start, End: timedEv.Start.Add(90 * time.Minute), Event: timedEv}},
	}
	allday := map[string][]model.Occurrence{
		dayKey(day): {{Start: day, End: day.AddDate(0, 0, 1), Event: allDayEv}},
	}
	tg.setData([]time.Time{day}, timed, allday, day, day)

	out := renderPrimitive(t, tg, 100, 24)

	// July 4 2026 is a Saturday.
	for _, want := range []string{"Sat 4", "Holiday", "9am", "Team sync"} {
		if !strings.Contains(out, want) {
			t.Errorf("time-grid render missing %q:\n%s", want, out)
		}
	}
}

func TestTimeGridArrowChangesDay(t *testing.T) {
	tg := newTimeGridView()
	day := time.Date(2026, 7, 4, 0, 0, 0, 0, time.UTC)
	tg.setData(model.Week(day, true), nil, nil, day, day)

	var got time.Time
	tg.onSelectDay = func(d time.Time) { got = d }
	handle := tg.InputHandler()
	handle(tcell.NewEventKey(tcell.KeyRight, 0, tcell.ModNone), func(tview.Primitive) {})
	if want := day.AddDate(0, 0, 1); !got.Equal(want) {
		t.Errorf("Right selected %s, want %s", got.Format("2006-01-02"), want.Format("2006-01-02"))
	}
}
