package ui

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/littekge/LazyPlanner/internal/model"
	"github.com/littekge/LazyPlanner/internal/store"
)

// putZeroLengthAllDayEvent writes a foreign/hand-edited style all-day VEVENT
// whose DTEND equals its DTSTART (zero length) — emitted by some exporters.
func putZeroLengthAllDayEvent(t *testing.T, a *app, calID, summary string, day time.Time) string {
	t.Helper()
	uid := summary + "@ev"
	d := day.Format("20060102")
	ics := "BEGIN:VCALENDAR\r\nVERSION:2.0\r\nPRODID:-//t//t//EN\r\nBEGIN:VEVENT\r\nUID:" + uid +
		"\r\nSUMMARY:" + summary + "\r\nDTSTAMP:20260701T000000Z" +
		"\r\nDTSTART;VALUE=DATE:" + d +
		"\r\nDTEND;VALUE=DATE:" + d +
		"\r\nEND:VEVENT\r\nEND:VCALENDAR\r\n"
	parsed, err := model.Decode([]byte(ics), time.Local)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := a.store.Put(context.Background(), calID, store.ResourceName(uid), parsed); err != nil {
		t.Fatal(err)
	}
	return uid
}

// TestZeroLengthAllDayEventVisibleInWeekAndDay locks the pass-23 MED fix: a
// zero-length all-day event (DTSTART == DTEND, both VALUE=DATE) is returned by
// dayItems — so the drill cursor can land on it — and must therefore also be
// bucketed by splitOccs and drawn in the week/day all-day band. Before the fix
// splitOccs walked [Start, End) by day, which is empty for a zero-length span,
// so the item was selectable but invisible.
func TestZeroLengthAllDayEventVisibleInWeekAndDay(t *testing.T) {
	loc := time.Local
	day := time.Date(2026, 7, 23, 0, 0, 0, 0, loc)

	a := newRootedTestApp(t, time.Date(2026, 7, 23, 12, 0, 0, 0, loc))
	if err := a.store.CreateCalendarLocal(context.Background(), "ev", store.CalendarMeta{DisplayName: "EV"}, []string{"VEVENT"}); err != nil {
		t.Fatal(err)
	}
	putZeroLengthAllDayEvent(t, a, "ev", "ZeroLenHoliday", day)
	a.reload()

	a.setMode(modeCalendar)
	a.anchor = day

	// (1) The drill list contains the item — drilling can select it.
	items := a.dayItems(day)
	if len(items) != 1 {
		t.Fatalf("dayItems(7/23) = %d items, want 1 (setup failed)", len(items))
	}

	// (2) splitOccs must bucket it onto the same day, so the band draws it.
	_, allday := a.splitOccs([]time.Time{day})
	if got := len(allday[dayKey(day)]); got != 1 {
		t.Errorf("splitOccs all-day bucket for 7/23 = %d occurrences, want 1", got)
	}

	// (3) Month view renders it...
	a.viewMode = viewMonth
	a.buildCenterCalendar()
	month := renderPrimitive(t, a.month, 120, 40)
	if !strings.Contains(month, "ZeroLen") {
		t.Errorf("month view does not render the zero-length all-day event:\n%s", month)
	}

	// (4) ...and so must week and day.
	a.viewMode = viewWeek
	a.buildCenterCalendar()
	week := renderPrimitive(t, a.timegrid, 160, 48)
	if !strings.Contains(week, "ZeroLen") {
		t.Errorf("week view does not render the zero-length all-day event:\n%s", week)
	}

	a.viewMode = viewDay
	a.buildCenterCalendar()
	dayView := renderPrimitive(t, a.timegrid, 160, 48)
	if !strings.Contains(dayView, "ZeroLen") {
		t.Errorf("day view does not render the zero-length all-day event:\n%s", dayView)
	}
}

// TestSplitOccsRenderSetMatchesDrillSet closes the class rather than the single
// case: for a mixed day (zero-length all-day, ordinary single-day all-day,
// multi-day all-day, and a timed event) the set splitOccs draws must be exactly
// the set dayItems lets the user drill onto — same events, same count. It also
// pins the ordinary items' drill order, so healing the zero-length case cannot
// shift or drop a normal item.
func TestSplitOccsRenderSetMatchesDrillSet(t *testing.T) {
	loc := time.Local
	day := time.Date(2026, 7, 23, 0, 0, 0, 0, loc)

	a := newRootedTestApp(t, time.Date(2026, 7, 23, 12, 0, 0, 0, loc))
	if err := a.store.CreateCalendarLocal(context.Background(), "ev", store.CalendarMeta{DisplayName: "EV"}, []string{"VEVENT"}); err != nil {
		t.Fatal(err)
	}
	putZeroLengthAllDayEvent(t, a, "ev", "ZeroLenHoliday", day)
	// Ordinary all-day event: DTEND is the exclusive next midnight.
	putEvent(t, a, "ev", "NormalAllDay", day, true)
	// Multi-day all-day event straddling the day from the day before.
	putMultiDayAllDayEvent(t, a, "ev", "SpanningAllDay", day.AddDate(0, 0, -1), 3)
	// Ordinary multi-hour timed event on the same day.
	putTimedEvent(t, a, "ev", "TimedMeeting", day.Add(10*time.Hour))
	a.reload()

	a.setMode(modeCalendar)
	a.anchor = day

	items := a.dayItems(day)
	var drillTitles []string
	for _, it := range items {
		if it.Event != nil {
			drillTitles = append(drillTitles, it.Title)
		}
	}
	// DayAgenda orders all-day first (by start, so the spanning event — which
	// began the previous day — leads; ties by lowercased title), then timed.
	wantDrill := []string{"SpanningAllDay", "NormalAllDay", "ZeroLenHoliday", "TimedMeeting"}
	if strings.Join(drillTitles, ",") != strings.Join(wantDrill, ",") {
		t.Fatalf("drill list = %v, want %v", drillTitles, wantDrill)
	}

	timed, allday := a.splitOccs([]time.Time{day})
	rendered := map[string]bool{}
	for _, o := range append(append([]model.Occurrence{}, allday[dayKey(day)]...), timed[dayKey(day)]...) {
		rendered[o.Event.Summary] = true
	}
	if len(rendered) != len(drillTitles) {
		t.Errorf("render set = %d events %v, drill set = %d events %v", len(rendered), rendered, len(drillTitles), drillTitles)
	}
	for _, title := range drillTitles {
		if !rendered[title] {
			t.Errorf("%q is drillable but splitOccs never renders it", title)
		}
	}

	// Every drillable item must be reachable on screen. The one-row all-day band
	// leads with the first item and collapses the rest into "+N", so the count it
	// advertises has to match the drill set's all-day count; and drilling onto any
	// all-day item must put that item's own title in the band (the pass-23 bug was
	// exactly this failing — the band skipped a day whose only occupant was the
	// zero-length event, so the drilled item stayed invisible).
	a.viewMode = viewWeek
	a.buildCenterCalendar()
	week := renderPrimitive(t, a.timegrid, 200, 60)
	if !strings.Contains(week, "+2") {
		t.Errorf("all-day band does not advertise the 2 collapsed items (3 all-day, 1 shown):\n%s", week)
	}
	if !strings.Contains(week, "TimedMeeting") {
		t.Errorf("week view no longer renders the ordinary timed event:\n%s", week)
	}

	a.timegrid.selected = day
	a.timegrid.eventMode = true
	for i, want := range wantDrill {
		a.timegrid.eventIndex = i
		sel := a.timegrid.selectedItem()
		if sel == nil || sel.Title != want {
			t.Fatalf("drill index %d = %v, want %q", i, sel, want)
		}
		if !sel.AllDay {
			continue
		}
		out := renderPrimitive(t, a.timegrid, 200, 60)
		if !strings.Contains(out, want) {
			t.Errorf("drilled onto all-day item %d (%q) but the band does not show it:\n%s", i, want, out)
		}
	}
}

// putMultiDayAllDayEvent writes an all-day VEVENT covering days consecutive days
// starting at start (DTEND is the exclusive midnight after the last day).
func putMultiDayAllDayEvent(t *testing.T, a *app, calID, summary string, start time.Time, days int) {
	t.Helper()
	obj, err := model.NewEventObject(model.EventDraft{
		Summary: summary,
		Start:   start,
		End:     start.AddDate(0, 0, days),
		AllDay:  true,
	}, a.now)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := a.store.Put(context.Background(), calID, store.ResourceName(obj.Events[0].UID), obj); err != nil {
		t.Fatal(err)
	}
}
