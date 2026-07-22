package ui

import (
	"testing"
	"time"
)

// findEventOccurrence returns the first stored event occurrence whose summary
// matches, over a wide window around now.
func findEventOccurrence(t *testing.T, a *app, summary string) (start, end time.Time, found bool) {
	t.Helper()
	from := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC)
	occs, err := a.store.EventOccurrences(from, to)
	if err != nil {
		t.Fatalf("EventOccurrences: %v", err)
	}
	for _, o := range occs {
		if o.Event.Summary == summary {
			return o.Start, o.End, true
		}
	}
	return time.Time{}, time.Time{}, false
}

// TestCreateEventTimeRange verifies the quick-add time range flows into the
// created event: the event gets the parsed end, and a range crossing midnight
// rolls the end to the next day.
func TestCreateEventTimeRange(t *testing.T) {
	now := time.Date(2026, 7, 5, 9, 0, 0, 0, time.UTC)
	base := time.Date(2026, 7, 8, 0, 0, 0, 0, time.UTC)

	tests := []struct {
		name    string
		text    string
		summary string
		start   time.Time
		end     time.Time
	}{
		{
			name:    "same-day range",
			text:    "Meeting 5-6pm",
			summary: "Meeting",
			start:   time.Date(2026, 7, 8, 17, 0, 0, 0, time.UTC),
			end:     time.Date(2026, 7, 8, 18, 0, 0, 0, time.UTC),
		},
		{
			name:    "range crossing midnight rolls the end forward",
			text:    "Party 11pm-1am",
			summary: "Party",
			start:   time.Date(2026, 7, 8, 23, 0, 0, 0, time.UTC),
			end:     time.Date(2026, 7, 9, 1, 0, 0, 0, time.UTC),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			a := newWritableTestApp(t, now)
			a.loc = time.UTC
			calID := a.selectedCalendarID()
			if calID == "" {
				t.Fatal("no calendar selected")
			}
			a.createEvent(calID, base, tc.text)

			start, end, found := findEventOccurrence(t, a, tc.summary)
			if !found {
				t.Fatalf("created event %q not found", tc.summary)
			}
			if !start.Equal(tc.start) {
				t.Errorf("start = %s, want %s", start.Format("2006-01-02 15:04"), tc.start.Format("2006-01-02 15:04"))
			}
			if !end.Equal(tc.end) {
				t.Errorf("end = %s, want %s", end.Format("2006-01-02 15:04"), tc.end.Format("2006-01-02 15:04"))
			}
		})
	}
}

// TestCreateEventNoRangeKeepsHourDefault guards that a single time (no range)
// still gets the one-hour default duration.
func TestCreateEventNoRangeKeepsHourDefault(t *testing.T) {
	now := time.Date(2026, 7, 5, 9, 0, 0, 0, time.UTC)
	base := time.Date(2026, 7, 8, 0, 0, 0, 0, time.UTC)
	a := newWritableTestApp(t, now)
	a.loc = time.UTC
	calID := a.selectedCalendarID()
	a.createEvent(calID, base, "Standup 9am")

	start, end, found := findEventOccurrence(t, a, "Standup")
	if !found {
		t.Fatal("created event not found")
	}
	if got := end.Sub(start); got != time.Hour {
		t.Errorf("duration = %s, want 1h", got)
	}
}

// TestCreateTaskIgnoresRangeEnd verifies a task created from a time range uses
// the range start as its due time and ignores the end (documented behavior).
func TestCreateTaskIgnoresRangeEnd(t *testing.T) {
	now := time.Date(2026, 7, 5, 9, 0, 0, 0, time.UTC)
	a := newWritableTestApp(t, now)
	a.loc = time.UTC
	a.setMode(modeTasks)
	calID := a.selectedTasklistID()
	if calID == "" {
		t.Fatal("no task list selected")
	}
	a.createTask(calID, "", "Review 2-3pm tomorrow")

	td := todoBySummary(a.store, "Review")
	if td == nil {
		t.Fatal("created task not found")
	}
	if !td.HasDue || td.DueAllDay {
		t.Fatalf("due = %+v, want a timed due", td)
	}
	if h, m := td.Due.In(time.UTC).Hour(), td.Due.In(time.UTC).Minute(); h != 14 || m != 0 {
		t.Errorf("due time = %d:%02d, want 14:00 (the range start)", h, m)
	}
}
