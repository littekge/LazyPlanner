package ui

import (
	"strings"
	"testing"
	"time"

	"github.com/littekge/LazyPlanner/internal/model"
)

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
