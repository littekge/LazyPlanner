package ui

import (
	"testing"
	"time"

	"github.com/emersion/go-ical"
)

// TestCreateRecurringFromQuickAdd verifies a quick-add recurrence token flows
// into the created event and task as an RRULE, with the anchoring rule setting
// the start/due when the text carries no explicit date.
func TestCreateRecurringFromQuickAdd(t *testing.T) {
	now := time.Date(2026, 7, 5, 9, 0, 0, 0, time.UTC) // a Sunday
	base := time.Date(2026, 7, 8, 0, 0, 0, 0, time.UTC)

	a := newWritableTestApp(t, now)
	a.loc = time.UTC

	// Event: "every mon" anchors to the soonest Monday (Jul 6), weekly on Monday.
	calID := a.selectedCalendarID()
	a.createEvent(calID, base, "Standup every mon 9am")
	start, _, found := findEventOccurrence(t, a, "Standup")
	if !found {
		t.Fatal("recurring event not created")
	}
	loc, ok := a.store.Locate(eventUID(t, a, "Standup"))
	if !ok {
		t.Fatal("event not locatable")
	}
	ev := loc.Object.Events[0]
	if !ev.Recurring {
		t.Error("event not recurring")
	}
	if rp := ev.Raw.Props.Get(ical.PropRecurrenceRule); rp == nil || rp.Value != "FREQ=WEEKLY;BYDAY=MO" {
		t.Errorf("event RRULE = %v, want FREQ=WEEKLY;BYDAY=MO", rp)
	}
	// Anchored to Monday Jul 6, not the base day Jul 8.
	if start.Month() != time.July || start.Day() != 6 {
		t.Errorf("event start = %s, want anchored to Jul 6", start.Format("2006-01-02"))
	}

	// Task: bare "daily" recurs; single-live-instance model keys off the RRULE.
	a.setMode(modeTasks)
	tcalID := a.selectedTasklistID()
	a.createTask(tcalID, "", "water plants daily")
	td := todoBySummary(a.store, "water plants")
	if td == nil {
		t.Fatal("recurring task not created")
	}
	if !td.Recurring {
		t.Error("task not recurring")
	}
	if rp := td.Raw.Props.Get(ical.PropRecurrenceRule); rp == nil || rp.Value != "FREQ=DAILY" {
		t.Errorf("task RRULE = %v, want FREQ=DAILY", rp)
	}
}

func eventUID(t *testing.T, a *app, summary string) string {
	t.Helper()
	from := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC)
	occs, _ := a.store.EventOccurrences(from, to)
	for _, o := range occs {
		if o.Event.Summary == summary {
			return o.Event.UID
		}
	}
	t.Fatalf("event %q not found", summary)
	return ""
}
