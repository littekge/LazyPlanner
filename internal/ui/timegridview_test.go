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
	day := time.Date(2026, 7, 4, 0, 0, 0, 0, time.UTC)

	// Local time: the grid renders in the local zone, so 9am here stays 9am.
	timedEv := &model.Event{Summary: "Team sync", Start: time.Date(2026, 7, 4, 9, 0, 0, 0, time.Local)}
	allDayEv := &model.Event{Summary: "Holiday", AllDay: true, Start: day}
	timed := map[string][]model.Occurrence{
		dayKey(day): {{Start: timedEv.Start, End: timedEv.Start.Add(90 * time.Minute), Event: timedEv}},
	}
	allday := map[string][]model.Occurrence{
		dayKey(day): {{Start: day, End: day.AddDate(0, 0, 1), Event: allDayEv}},
	}
	tg.setData([]time.Time{day}, timed, allday, day, day)

	// Tall enough that the whole day fits with each hour on its own row.
	out := renderPrimitive(t, tg, 100, 40)

	// July 4 2026 is a Saturday; the whole day fits so midnight..11pm all show.
	for _, want := range []string{"Sat 4", "Holiday", "12am", "9am", "11pm", "Team sync"} {
		if !strings.Contains(out, want) {
			t.Errorf("time-grid render missing %q:\n%s", want, out)
		}
	}
}

func TestTimeGridDrillIn(t *testing.T) {
	tg := newTimeGridView()
	day := time.Date(2026, 7, 4, 0, 0, 0, 0, time.Local)
	ev := &model.Event{Summary: "E", Start: time.Date(2026, 7, 4, 9, 0, 0, 0, time.Local)}
	timed := map[string][]model.Occurrence{
		dayKey(day): {{Start: ev.Start, End: ev.Start.Add(time.Hour), Event: ev}},
	}
	tg.setData([]time.Time{day}, timed, nil, day, day)
	tg.items = map[string][]model.AgendaItem{dayKey(day): {{Start: ev.Start, Title: ev.Summary, Event: ev}}}

	var got *model.Event
	tg.onSelectEvent = func(it model.AgendaItem) { got = it.Event }
	handle := tg.InputHandler()

	handle(tcell.NewEventKey(tcell.KeyEnter, 0, tcell.ModNone), func(tview.Primitive) {})
	if !tg.eventMode {
		t.Fatal("Enter did not enter event mode")
	}
	if got != ev {
		t.Fatal("onSelectEvent not called with the drilled event")
	}

	handle(tcell.NewEventKey(tcell.KeyEscape, 0, tcell.ModNone), func(tview.Primitive) {})
	if tg.eventMode {
		t.Error("Esc did not exit event mode")
	}
}

// TestTimeGridDrillsAllDayFirst checks all-day events are part of the drill
// cycle (before timed events).
func TestTimeGridDrillsAllDayFirst(t *testing.T) {
	tg := newTimeGridView()
	day := time.Date(2026, 7, 4, 0, 0, 0, 0, time.Local)
	allEv := &model.Event{Summary: "Holiday", AllDay: true, Start: day}
	timedEv := &model.Event{Summary: "Sync", Start: time.Date(2026, 7, 4, 9, 0, 0, 0, time.Local)}
	allday := map[string][]model.Occurrence{
		dayKey(day): {{Start: day, End: day.AddDate(0, 0, 1), Event: allEv}},
	}
	timed := map[string][]model.Occurrence{
		dayKey(day): {{Start: timedEv.Start, End: timedEv.Start.Add(time.Hour), Event: timedEv}},
	}
	tg.setData([]time.Time{day}, timed, allday, day, day)
	// Drill order (DayAgenda): all-day first, then timed.
	tg.items = map[string][]model.AgendaItem{dayKey(day): {
		{Start: day, AllDay: true, Title: allEv.Summary, Event: allEv},
		{Start: timedEv.Start, Title: timedEv.Summary, Event: timedEv},
	}}

	var got []*model.Event
	tg.onSelectEvent = func(it model.AgendaItem) { got = append(got, it.Event) }
	handle := tg.InputHandler()

	handle(tcell.NewEventKey(tcell.KeyEnter, 0, tcell.ModNone), func(tview.Primitive) {})
	handle(tcell.NewEventKey(tcell.KeyDown, 0, tcell.ModNone), func(tview.Primitive) {})

	if len(got) != 2 || got[0] != allEv || got[1] != timedEv {
		t.Errorf("drill order = %v, want [Holiday(all-day), Sync(timed)]", got)
	}
}

// TestTimeGridDrawsDueTasks: a timed due task draws a [ ]/[■] line at its due
// time and an all-day-due task sits in the top band, both in the list's color.
func TestTimeGridDrawsDueTasks(t *testing.T) {
	tg := newTimeGridView()
	tg.taskColor = func(*model.Todo) (calColor, bool) { return calColor{fg: tcell.ColorRed, name: "red", dark: true}, true }
	day := time.Date(2026, 7, 4, 0, 0, 0, 0, time.Local)
	timedTask := &model.Todo{UID: "t1", Summary: "Payrent", HasDue: true, Due: time.Date(2026, 7, 4, 9, 0, 0, 0, time.Local)}
	allDayTask := &model.Todo{UID: "t2", Summary: "Renewpass", HasDue: true, DueAllDay: true, Due: day}
	tg.setData([]time.Time{day}, nil, nil, day, day)
	tg.dueTasks = map[string][]*model.Todo{dayKey(day): {timedTask, allDayTask}}

	out := renderPrimitive(t, tg, 100, 40)
	// Uncompleted due tasks show "[ ]" here, same as the month grid.
	for _, want := range []string{"[ ] Payrent", "[ ] Renewpass"} {
		if !strings.Contains(out, want) {
			t.Errorf("time-grid render missing %q:\n%s", want, out)
		}
	}

	// The timed due task's marker renders in the list color (red).
	cells, cw, ch := drawCells(t, tg, 100, 40)
	row := rowFind(cells, cw, ch, "Payrent")
	if row < 0 {
		t.Fatal("timed due-task line not found")
	}
	if fg, ok := glyphFg(cells, cw, row, 'P'); !ok || fg != tcell.ColorRed {
		t.Errorf("timed due-task fg=%v (found=%v), want red", fg, ok)
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

// TestTimeGridHomeEndSelectsDay: in the week view, Home/End (gg/G) jump to the
// first / last day column.
func TestTimeGridHomeEndSelectsDay(t *testing.T) {
	tg := newTimeGridView()
	anchor := time.Date(2026, 7, 8, 0, 0, 0, 0, time.UTC) // mid-week
	days := model.Week(anchor, true)
	tg.setData(days, nil, nil, anchor, anchor)

	var got time.Time
	tg.onSelectDay = func(d time.Time) { got = d }
	handle := tg.InputHandler()

	handle(tcell.NewEventKey(tcell.KeyHome, 0, tcell.ModNone), func(tview.Primitive) {})
	if !got.Equal(days[0]) {
		t.Errorf("Home selected %v, want first day %v", got.Format("2006-01-02"), days[0].Format("2006-01-02"))
	}
	handle(tcell.NewEventKey(tcell.KeyEnd, 0, tcell.ModNone), func(tview.Primitive) {})
	if !got.Equal(days[len(days)-1]) {
		t.Errorf("End selected %v, want last day %v", got.Format("2006-01-02"), days[len(days)-1].Format("2006-01-02"))
	}
}

// TestTimeGridVerticalMotionCyclesEvents: in the week/day grid, Up/Down (j/k)
// drill into the selected day's events and cycle them — so counts (2j) work too.
func TestTimeGridVerticalMotionCyclesEvents(t *testing.T) {
	tg := newTimeGridView()
	day := time.Date(2026, 7, 4, 0, 0, 0, 0, time.Local)
	e1 := &model.Event{Summary: "Nine", Start: time.Date(2026, 7, 4, 9, 0, 0, 0, time.Local)}
	e2 := &model.Event{Summary: "Eleven", Start: time.Date(2026, 7, 4, 11, 0, 0, 0, time.Local)}
	timed := map[string][]model.Occurrence{dayKey(day): {
		{Start: e1.Start, End: e1.Start.Add(time.Hour), Event: e1},
		{Start: e2.Start, End: e2.Start.Add(time.Hour), Event: e2},
	}}
	tg.setData([]time.Time{day}, timed, nil, day, day)
	tg.items = map[string][]model.AgendaItem{dayKey(day): {
		{Start: e1.Start, Title: e1.Summary, Event: e1},
		{Start: e2.Start, Title: e2.Summary, Event: e2},
	}}

	var got *model.Event
	tg.onSelectEvent = func(it model.AgendaItem) { got = it.Event }
	handle := tg.InputHandler()
	down := tcell.NewEventKey(tcell.KeyDown, 0, tcell.ModNone)
	up := tcell.NewEventKey(tcell.KeyUp, 0, tcell.ModNone)

	handle(down, func(tview.Primitive) {}) // day-mode → enter events, first
	if !tg.eventMode || got != e1 {
		t.Fatalf("Down did not drill into events (eventMode=%v got=%v)", tg.eventMode, got)
	}
	handle(down, func(tview.Primitive) {}) // → second event
	if got != e2 {
		t.Errorf("second Down selected %v, want Eleven", got.Summary)
	}
	handle(up, func(tview.Primitive) {}) // → back to first
	if got != e1 {
		t.Errorf("Up selected %v, want Nine", got.Summary)
	}
}
