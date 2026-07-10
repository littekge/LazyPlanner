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

// TestTimeGridUndrilledVerticalDoesNothing: un-drilled, Up/Down do nothing
// (days are horizontal); Enter drills in.
func TestTimeGridUndrilledVerticalDoesNothing(t *testing.T) {
	tg := newTimeGridView()
	day := time.Date(2026, 7, 4, 0, 0, 0, 0, time.Local)
	e := &model.Event{Summary: "E", Start: time.Date(2026, 7, 4, 9, 0, 0, 0, time.Local)}
	tg.setData([]time.Time{day}, map[string][]model.Occurrence{dayKey(day): {{Start: e.Start, End: e.Start.Add(time.Hour), Event: e}}}, nil, day, day)
	tg.items = map[string][]model.AgendaItem{dayKey(day): {{Start: e.Start, Title: e.Summary, Event: e}}}
	handle := tg.InputHandler()

	handle(tcell.NewEventKey(tcell.KeyDown, 0, tcell.ModNone), func(tview.Primitive) {})
	if tg.eventMode {
		t.Error("Down should not drill in un-drilled week/day view")
	}
	handle(tcell.NewEventKey(tcell.KeyEnter, 0, tcell.ModNone), func(tview.Primitive) {})
	if !tg.eventMode {
		t.Error("Enter should drill in")
	}
}

// TestTimeGridSpatialDrillNav exercises the 2D drill from the spec example:
// A (11–12, full width) above B and C (both 12–1, concurrent → side by side).
// j/k move by time, h/l move between the concurrent pair.
func TestTimeGridSpatialDrillNav(t *testing.T) {
	tg := newTimeGridView()
	day := time.Date(2026, 7, 4, 0, 0, 0, 0, time.Local)
	at := func(h, m int) time.Time { return time.Date(2026, 7, 4, h, m, 0, 0, time.Local) }
	a := &model.Event{Summary: "A", Start: at(11, 0)}
	b := &model.Event{Summary: "B", Start: at(12, 0)}
	c := &model.Event{Summary: "C", Start: at(12, 0)}
	occ := func(e *model.Event, endH int) model.Occurrence {
		return model.Occurrence{Start: e.Start, End: at(endH, 0), Event: e}
	}
	timed := map[string][]model.Occurrence{dayKey(day): {occ(a, 12), occ(b, 13), occ(c, 13)}}
	tg.setData([]time.Time{day}, timed, nil, day, day)
	tg.items = map[string][]model.AgendaItem{dayKey(day): {
		{Start: a.Start, Title: "A", Event: a},
		{Start: b.Start, Title: "B", Event: b},
		{Start: c.Start, Title: "C", Event: c},
	}}

	var got string
	tg.onSelectEvent = func(it model.AgendaItem) { got = it.Title }
	handle := tg.InputHandler()
	key := func(k tcell.Key) { handle(tcell.NewEventKey(k, 0, tcell.ModNone), func(tview.Primitive) {}) }

	key(tcell.KeyEnter) // drill → A (first item)
	if got != "A" {
		t.Fatalf("drill landed on %q, want A", got)
	}
	key(tcell.KeyDown) // A ↓ → B (leftmost of the concurrent pair)
	if got != "B" {
		t.Errorf("Down from A = %q, want B (leftmost concurrent)", got)
	}
	key(tcell.KeyRight) // B → C (the other concurrent lane)
	if got != "C" {
		t.Errorf("Right from B = %q, want C", got)
	}
	key(tcell.KeyLeft) // C → B
	if got != "B" {
		t.Errorf("Left from C = %q, want B", got)
	}
	key(tcell.KeyUp) // B ↑ → A
	if got != "A" {
		t.Errorf("Up from B = %q, want A", got)
	}
	key(tcell.KeyLeft) // A has no left lane → no move
	if got != "A" {
		t.Errorf("Left from A = %q, want A (no move at edge)", got)
	}
}

// hourLabelRows maps each visible hour label to its screen row, matching the
// gutter text exactly (so "1am" isn't confused with "11am").
func hourLabelRows(t *testing.T, tg *timeGridView, w, h int) map[int]int {
	t.Helper()
	cells, cw, ch := drawCells(t, tg, w, h)
	rows := map[int]int{}
	for r := 0; r < ch; r++ {
		var b strings.Builder
		for c := 0; c < gutterWidth-1 && c < cw; c++ { // stop before the separator column
			if rs := cells[r*cw+c].Runes; len(rs) > 0 {
				b.WriteRune(rs[0])
			} else {
				b.WriteByte(' ')
			}
		}
		gut := strings.TrimSpace(b.String())
		for hour := 0; hour < hoursPerDay; hour++ {
			if gut == hourLabel(hour) {
				rows[hour] = r
			}
		}
	}
	return rows
}

// TestTimeGridUniformHourSpacing: when the whole day fits, every hour occupies
// the same number of rows, so consecutive hour labels are evenly spaced. The old
// float-scaled mapping produced a mix of gaps at a height like this (body 52 →
// 2.17 rows/hour truncated unevenly); the uniform grid gives a constant 2.
func TestTimeGridUniformHourSpacing(t *testing.T) {
	tg := newTimeGridView()
	day := time.Date(2026, 7, 4, 0, 0, 0, 0, time.Local)
	tg.setData([]time.Time{day}, nil, nil, day, day)

	// Body height = h-3 = 52 rows → 24*2=48 fit with a small margin (no scroll).
	rows := hourLabelRows(t, tg, 100, 55)
	if len(rows) != hoursPerDay {
		t.Fatalf("expected all %d hour labels to fit, got %d", hoursPerDay, len(rows))
	}
	gap := rows[1] - rows[0]
	for hour := 1; hour < hoursPerDay; hour++ {
		if d := rows[hour] - rows[hour-1]; d != gap {
			t.Errorf("uneven spacing: %s..%s gap %d, want a uniform %d",
				hourLabel(hour-1), hourLabel(hour), d, gap)
		}
	}
	if gap != 2 {
		t.Errorf("expected 2 rows per hour at this height, got %d", gap)
	}
}

// TestTimeGridRowsPerHourOverride: an explicit rowsPerHour (the +/- zoom) sets a
// uniform hour height regardless of the pane, and is recorded in lastRowsPerHour.
func TestTimeGridRowsPerHourOverride(t *testing.T) {
	tg := newTimeGridView()
	day := time.Date(2026, 7, 4, 0, 0, 0, 0, time.Local)
	tg.setData([]time.Time{day}, nil, nil, day, day)
	tg.rowsPerHour = 3 // zoom to 3 rows/hour

	// Body height = h-3 = 72 = 24*3, so the whole day fits at 3 rows/hour.
	rows := hourLabelRows(t, tg, 100, 75)
	if len(rows) != hoursPerDay {
		t.Fatalf("expected all %d hour labels at 3 rows/hour, got %d", hoursPerDay, len(rows))
	}
	for hour := 1; hour < hoursPerDay; hour++ {
		if d := rows[hour] - rows[hour-1]; d != 3 {
			t.Errorf("override spacing: %s..%s gap %d, want 3", hourLabel(hour-1), hourLabel(hour), d)
		}
	}
	if tg.lastRowsPerHour != 3 {
		t.Errorf("lastRowsPerHour = %d, want 3", tg.lastRowsPerHour)
	}
}

// TestTimeGridScrollsShortPaneToDrilledItem: on a pane too short to show all 24
// hours at one row each, the grid scrolls to keep the drilled item visible
// instead of squashing the hours together.
func TestTimeGridScrollsShortPaneToDrilledItem(t *testing.T) {
	tg := newTimeGridView()
	day := time.Date(2026, 7, 4, 0, 0, 0, 0, time.Local)
	ev := &model.Event{Summary: "LateMtg", Start: time.Date(2026, 7, 4, 21, 0, 0, 0, time.Local)}
	timed := map[string][]model.Occurrence{
		dayKey(day): {{Start: ev.Start, End: ev.Start.Add(time.Hour), Event: ev}},
	}
	tg.setData([]time.Time{day}, timed, nil, day, day)
	tg.items = map[string][]model.AgendaItem{dayKey(day): {{Start: ev.Start, Title: ev.Summary, Event: ev}}}

	// Body height = h-3 = 15 rows < 24, so the day must scroll. Undrilled it
	// anchors to the current time (midnight here), leaving a 9pm event off-screen.
	const w, h = 100, 18
	if strings.Contains(renderPrimitive(t, tg, w, h), "LateMtg") {
		t.Fatal("expected the 9pm event off-screen before drilling on a short pane")
	}

	// Drilling to it scrolls the grid so it comes into view.
	tg.eventMode = true
	tg.eventIndex = 0
	if !strings.Contains(renderPrimitive(t, tg, w, h), "LateMtg") {
		t.Error("drilled 9pm event should scroll into view on a short pane")
	}
}
