package ui

import (
	"context"
	"strings"
	"testing"
	"time"
	// Embed the IANA database in the test binary. internal/ui does not import it
	// (only cmd/lazyplanner does), so on a host without system zoneinfo the zone
	// lookups below would fail — they are guarded with t.Fatalf, not t.Skip.
	_ "time/tzdata"

	"github.com/gdamore/tcell/v2"

	"github.com/littekge/LazyPlanner/internal/model"
	"github.com/littekge/LazyPlanner/internal/store"
)

// zoneDifferingFromLocal returns a loadable zone that renders `at` at a different
// hour than time.Local does. The point of these tests is that a.loc and time.Local
// can now DISAGREE (config.LocalZone() prefers a loadable /etc/timezone name, which
// need not resolve to the same zone as /etc/localtime), so a zone that happens to
// render `at` at the host's hour would make every assertion below vacuous — and the
// suite must not depend on the host's TZ to be meaningful.
func zoneDifferingFromLocal(t *testing.T, at time.Time) *time.Location {
	t.Helper()
	localHour := at.In(time.Local).Hour()
	// A spread wide enough that some entry must differ from any host offset.
	for _, name := range []string{
		"Pacific/Kiritimati", "Asia/Tokyo", "Europe/Berlin", "UTC",
		"America/New_York", "Pacific/Honolulu",
	} {
		loc, err := time.LoadLocation(name)
		if err != nil {
			t.Fatalf("zone %q must load (tzdata is embedded): %v", name, err)
		}
		if at.In(loc).Hour() != localHour {
			return loc
		}
	}
	t.Fatal("no candidate zone renders the probe at a different hour than time.Local — the guards would be vacuous")
	return nil
}

// seedTimedEvent stores a one-hour timed event starting at `start`.
func seedTimedEvent(t *testing.T, a *app, uid string, start time.Time) {
	t.Helper()
	obj, err := model.NewEventObject(model.EventDraft{
		Summary: "Zone probe", Start: start, End: start.Add(time.Hour),
	}, start)
	if err != nil {
		t.Fatal(err)
	}
	// The generated UID would be random; rewrite it so the test can find the item.
	obj.Events[0].Raw.Props.SetText("UID", uid)
	reparsed, err := model.Parse(obj.Calendar, a.loc)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := a.store.Put(context.Background(), "ev", store.ResourceName(uid), reparsed); err != nil {
		t.Fatal(err)
	}
}

// Every render path must read the display zone from a.loc, not time.Local. The two
// were always equal before the TZID arc — cmd/lazyplanner now passes
// config.LocalZone(), which can name a zone /etc/localtime does not resolve to — so
// a form that parses and writes in a.loc could store 20:00 and render some other
// hour.
func TestRenderPathsUseTheAppZoneNotTimeLocal(t *testing.T) {
	now := time.Date(2026, 7, 15, 12, 0, 0, 0, time.UTC)
	loc := zoneDifferingFromLocal(t, now)

	a := newRootedTestApp(t, now)
	a.loc = loc

	// 12:00 UTC, an instant the display zone and time.Local render at different hours.
	start := time.Date(2026, 7, 15, 12, 0, 0, 0, time.UTC)
	seedTimedEvent(t, a, "zone-probe", start)
	a.anchor = model.DayStart(start.In(loc))
	a.reload()

	wantHour := start.In(loc).Hour()
	localHour := start.In(time.Local).Hour()
	if wantHour == localHour {
		t.Fatalf("the display zone and time.Local agree on the hour (%d) — the guard is vacuous", wantHour)
	}

	t.Run("month grid day-cell label", func(t *testing.T) {
		if a.month.loc != a.loc {
			t.Fatalf("month view loc = %v, want the app's %v", a.month.loc, a.loc)
		}
		out := renderPrimitive(t, a.month, 120, 40)
		// A narrow day cell truncates the title, so match on the hour label plus the
		// start of the summary rather than the whole line.
		want := hourAxisLabel(wantHour, a.clock24) + " Zone"
		bad := hourAxisLabel(localHour, a.clock24) + " Zone"
		if !strings.Contains(out, want) {
			t.Errorf("month cell does not label the event %q (the a.loc hour); got:\n%s", want, out)
		}
		if strings.Contains(out, bad) {
			t.Errorf("month cell labels the event with the time.Local hour %q", bad)
		}
	})

	t.Run("agenda line label", func(t *testing.T) {
		items := a.dayItems(model.DayStart(start.In(loc)))
		if len(items) == 0 {
			t.Fatal("no agenda item for the seeded day")
		}
		var probe model.AgendaItem
		for _, it := range items {
			if it.Title == "Zone probe" {
				probe = it
			}
		}
		if probe.Title == "" {
			t.Fatal("the seeded event is not in the day's agenda")
		}
		label := a.agendaLeftLabel(probe)
		want := clockStr(start.In(loc), a.clock24)
		bad := clockStr(start.In(time.Local), a.clock24)
		if !strings.Contains(label, want) {
			t.Errorf("agenda label %q does not carry the a.loc time %q", label, want)
		}
		if strings.Contains(label, bad) {
			t.Errorf("agenda label %q carries the time.Local time %q", label, bad)
		}
	})

	t.Run("time-grid scroll anchor", func(t *testing.T) {
		a.viewMode = viewDay
		a.buildCenterCalendar()
		if a.timegrid.loc != a.loc {
			t.Fatalf("time grid loc = %v, want the app's %v", a.timegrid.loc, a.loc)
		}
		// Drill onto the seeded event so anchorHour reads the drilled item's start.
		a.timegrid.enterEventMode()
		sel := a.timegrid.selectedItem()
		if sel == nil {
			t.Fatal("nothing drilled in the day grid")
		}
		got := a.timegrid.anchorHour()
		if want := hourFloat(sel.Start.In(loc)); got != want {
			t.Errorf("drilled anchorHour = %v, want the a.loc hour %v (time.Local would give %v)",
				got, want, hourFloat(sel.Start.In(time.Local)))
		}

		// Undrilled, the anchor falls back to "now" on a shown day — the sibling
		// site, read in the display zone as well.
		a.timegrid.eventMode = false
		if !model.SameDay(a.timegrid.days[0], a.timegrid.now) {
			t.Fatal("the shown day is not today; the now-anchored branch is unreachable")
		}
		got = a.timegrid.anchorHour()
		if want := hourFloat(a.now.In(loc)); got != want {
			t.Errorf("now-anchored anchorHour = %v, want the a.loc hour %v (time.Local would give %v)",
				got, want, hourFloat(a.now.In(time.Local)))
		}
	})

	t.Run("time-grid event block row", func(t *testing.T) {
		a.viewMode = viewDay
		a.buildCenterCalendar()
		a.timegrid.rowsPerHour = 1 // one row per hour: a row delta IS an hour delta
		rows := blockRows(t, a.timegrid, 40, 30)
		if len(rows) == 0 {
			t.Fatal("no event block was painted")
		}

		// Re-render in a zone one hour further east: the block must move down by
		// exactly that hour. Reading the item in time.Local would pin it in place.
		_, off := start.In(loc).Zone()
		a.timegrid.loc = time.FixedZone("probe", off+3600)
		moved := blockRows(t, a.timegrid, 40, 30)
		if len(moved) == 0 {
			t.Fatal("no event block was painted in the shifted zone")
		}
		if moved[0]-rows[0] != 1 {
			t.Errorf("block top row %d → %d for a +1h display zone, want a 1-row shift",
				rows[0], moved[0])
		}
		a.timegrid.loc = a.loc
	})
}

// blockRows returns the sorted rows carrying event-block background, for a grid
// drawn at the given size.
func blockRows(t *testing.T, tg *timeGridView, w, h int) []int {
	t.Helper()
	screen := tcell.NewSimulationScreen("")
	if err := screen.Init(); err != nil {
		t.Fatalf("init simulation screen: %v", err)
	}
	defer screen.Fini()
	screen.SetSize(w, h)
	tg.SetRect(0, 0, w, h)
	tg.Draw(screen)
	screen.Show()

	cells, cw, ch := screen.GetContents()
	var rows []int
	for row := 0; row < ch; row++ {
		for col := 0; col < cw; col++ {
			if _, bg, _ := cells[row*cw+col].Style.Decompose(); bg == blockColor {
				rows = append(rows, row)
				break
			}
		}
	}
	return rows
}

// The timed-due task marker sits at the due time, read in the display zone too.
func TestTaskMarkerRowFollowsTheAppZone(t *testing.T) {
	now := time.Date(2026, 7, 15, 12, 0, 0, 0, time.UTC)
	loc := zoneDifferingFromLocal(t, now)

	a := newRootedTestApp(t, now)
	a.loc = loc
	due := time.Date(2026, 7, 15, 12, 0, 0, 0, time.UTC)
	obj := model.NewTodoObject(model.TodoDraft{Summary: "Zone task", HasDue: true, Due: due}, due)
	obj.Todos[0].Raw.Props.SetText("UID", "zone-task")
	reparsed, err := model.Parse(obj.Calendar, a.loc)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := a.store.Put(context.Background(), "tasks", store.ResourceName("zone-task"), reparsed); err != nil {
		t.Fatal(err)
	}
	a.anchor = model.DayStart(due.In(loc))
	a.viewMode = viewDay
	a.reload()
	a.timegrid.rowsPerHour = 1

	row := markerRow(t, a.timegrid, 40, 30)
	if row < 0 {
		t.Fatal("the due-task marker was not drawn")
	}
	_, off := due.In(loc).Zone()
	a.timegrid.loc = time.FixedZone("probe", off+3600)
	movedRow := markerRow(t, a.timegrid, 40, 30)
	if movedRow < 0 {
		t.Fatal("the due-task marker was not drawn in the shifted zone")
	}
	if movedRow-row != 1 {
		t.Errorf("marker row %d → %d for a +1h display zone, want a 1-row shift", row, movedRow)
	}
}

// markerRow is the row carrying the due-task marker text, or -1.
func markerRow(t *testing.T, tg *timeGridView, w, h int) int {
	t.Helper()
	out := renderPrimitive(t, tg, w, h)
	for i, line := range strings.Split(out, "\n") {
		if strings.Contains(line, "Zone task") {
			return i
		}
	}
	return -1
}
