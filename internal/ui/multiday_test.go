package ui

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/littekge/LazyPlanner/internal/model"
	"github.com/littekge/LazyPlanner/internal/store"
)

// putSpanningEvent writes a timed VEVENT running from start to end (which may be
// on a later day) into calID and returns its UID.
func putSpanningEvent(t *testing.T, a *app, calID, summary string, start, end time.Time) string {
	t.Helper()
	uid := summary + "@ev"
	ics := "BEGIN:VCALENDAR\r\nVERSION:2.0\r\nPRODID:-//t//t//EN\r\nBEGIN:VEVENT\r\nUID:" + uid +
		"\r\nSUMMARY:" + summary + "\r\nDTSTAMP:20260701T000000Z\r\nDTSTART:" + start.UTC().Format("20060102T150405Z") +
		"\r\nDTEND:" + end.UTC().Format("20060102T150405Z") +
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

// TestTimeGridRendersMultiDayTimedEventOnEveryDay locks the v1.0.2 week/day fix:
// a timed event spanning several days must render on every day it covers, not
// only its start day. It runs the real pipeline (splitOccs -> setData -> Draw).
func TestTimeGridRendersMultiDayTimedEventOnEveryDay(t *testing.T) {
	loc := time.Local
	// Week of 2026-07-20 (Mon) .. 2026-07-26 (Sun); event covers Thu 23 .. Sun 26.
	start := time.Date(2026, 7, 23, 11, 0, 0, 0, loc)
	end := time.Date(2026, 7, 26, 17, 0, 0, 0, loc)

	a := newRootedTestApp(t, time.Date(2026, 7, 24, 12, 0, 0, 0, loc))
	if err := a.store.CreateCalendarLocal(context.Background(), "ev", store.CalendarMeta{DisplayName: "EV"}, []string{"VEVENT"}); err != nil {
		t.Fatal(err)
	}
	putSpanningEvent(t, a, "ev", "Conference", start, end)
	a.reload()

	a.setMode(modeCalendar)
	a.viewMode = viewWeek
	a.anchor = model.DayStart(start)
	a.buildCenterCalendar()

	out := renderPrimitive(t, a.timegrid, 160, 48)
	if n := strings.Count(out, "Conference"); n < 4 {
		t.Errorf("multi-day timed event rendered on %d day columns, want >=4 (Thu 23 .. Sun 26):\n%s", n, out)
	}
}

// TestItemLabelMultiDayTimedEvent locks the v1.0.2 month-view fix: a timed event
// spanning several days must not repeat its start time on every day. The start
// day shows the start time, middle days show the title only (a continuation),
// and the final day shows the end time (→5pm).
func TestItemLabelMultiDayTimedEvent(t *testing.T) {
	loc := time.Local
	start := time.Date(2026, 7, 23, 11, 0, 0, 0, loc)
	end := time.Date(2026, 7, 26, 17, 0, 0, 0, loc)
	ev := &model.Event{Summary: "Conference", Start: start, End: end}
	it := model.AgendaItem{Start: start, End: end, Title: "Conference", Event: ev}
	day := func(d int) time.Time { return time.Date(2026, 7, d, 0, 0, 0, 0, loc) }

	startLbl := itemLabel(it, day(23), false, false)
	if !strings.Contains(startLbl, "11am") || !strings.Contains(startLbl, "Conference") {
		t.Errorf("start day label = %q, want start time (11am) + title", startLbl)
	}

	for _, d := range []int{24, 25} {
		mid := itemLabel(it, day(d), false, false)
		if strings.Contains(mid, "11am") {
			t.Errorf("middle day 7/%d repeats the start time: %q", d, mid)
		}
		if !strings.Contains(mid, "Conference") {
			t.Errorf("middle day 7/%d dropped the title: %q", d, mid)
		}
	}

	endLbl := itemLabel(it, day(26), false, false)
	if strings.Contains(endLbl, "11am") {
		t.Errorf("last day shows the start time instead of the end: %q", endLbl)
	}
	if !strings.Contains(endLbl, "5pm") {
		t.Errorf("last day missing the end time (5pm): %q", endLbl)
	}
}

// TestItemLabelSingleDayTimedEventUnchanged guards that the multi-day logic does
// not regress an ordinary same-day timed event — it still shows its start time.
func TestItemLabelSingleDayTimedEventUnchanged(t *testing.T) {
	loc := time.Local
	start := time.Date(2026, 7, 23, 14, 0, 0, 0, loc)
	end := time.Date(2026, 7, 23, 15, 0, 0, 0, loc)
	ev := &model.Event{Summary: "Standup", Start: start, End: end}
	it := model.AgendaItem{Start: start, End: end, Title: "Standup", Event: ev}
	day := time.Date(2026, 7, 23, 0, 0, 0, 0, loc)

	got := itemLabel(it, day, false, false)
	if !strings.Contains(got, "2pm") || !strings.Contains(got, "Standup") {
		t.Errorf("single-day timed label = %q, want start time (2pm) + title", got)
	}
}
