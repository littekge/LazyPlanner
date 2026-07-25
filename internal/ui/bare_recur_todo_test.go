package ui

import (
	"strings"
	"testing"
	"time"

	"github.com/littekge/LazyPlanner/internal/model"
)

// TestBareFrequencyRecurringTaskIsCompletable guards the Pass-20 MED fix: a
// quick-add recurring task typed with no explicit date ("water plants daily")
// must still carry a DUE, anchored to the base day per main.md ("daily → the base
// day"). Before the fix createTask set a DUE only when HasDate||HasTime, so a bare
// frequency produced a VTODO with an RRULE but no DTSTART/DUE — AdvanceRecurringTodo
// then errored "has no DTSTART/DUE to advance" and Space could never complete it.
func TestBareFrequencyRecurringTaskIsCompletable(t *testing.T) {
	now := time.Date(2026, 7, 25, 12, 0, 0, 0, time.UTC)
	a := newWritableTestApp(t, now)
	a.loc = time.UTC
	a.setMode(modeTasks)
	tcalID := a.selectedTasklistID()

	cases := []struct {
		text, summary string
	}{
		{"water plants daily", "water plants"},
		{"groceries every week", "groceries"},
		{"rent monthly", "rent"},
		{"taxes yearly", "taxes"},
	}
	for _, c := range cases {
		a.createTask(tcalID, "", c.text)
		td := todoBySummary(a.store, c.summary)
		if td == nil {
			t.Errorf("%q: task not created", c.text)
			continue
		}
		if !td.Recurring {
			t.Errorf("%q: task is not recurring", c.text)
		}
		if !td.HasDue {
			t.Errorf("%q: recurring task has no DUE — it can never be completed", c.text)
			continue
		}
		// The anchor is the base day (today), all-day since no time was typed.
		if td.Due.Year() != 2026 || td.Due.Month() != time.July || td.Due.Day() != 25 {
			t.Errorf("%q: DUE = %s, want anchored to the base day 2026-07-25",
				c.text, td.Due.Format("2006-01-02"))
		}
		// The whole point: completion (advance) must succeed, not error.
		loc, ok := a.store.Locate(td.UID)
		if !ok {
			t.Errorf("%q: created task not locatable", c.text)
			continue
		}
		if _, _, err := model.AdvanceRecurringTodo(loc.Object, td.UID, now, a.loc); err != nil {
			t.Errorf("%q: cannot complete recurring task: %v", c.text, err)
		}
		// And through the UI Space path (advanceRecurringTodo) — no error flash.
		a.advanceRecurringTodo(loc, td.UID)
		if msg := a.statusLeft.GetText(true); strings.Contains(msg, "Complete failed") || strings.Contains(msg, "has no DTSTART") {
			t.Errorf("%q: UI complete failed: %q", c.text, msg)
		}
	}
}
