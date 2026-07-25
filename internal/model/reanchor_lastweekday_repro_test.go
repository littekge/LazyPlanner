package model

import (
	"testing"
	"time"

	"github.com/teambition/rrule-go"
)

// TestReanchoredRecurrenceLastWeekdayBackwardMoveRepro reproduces the vanishing
// -instance bug: a "last <weekday>" monthly rule moved backward across the
// weekday boundary yields BYDAY=-1<newWeekday> that does not actually fire on the
// moved DTSTART.
//
// April 2024 (30 days): the last Wednesday is Apr 24. grab 'h' (day -1) moves
// DTSTART to Apr 23 (a Tuesday). ReanchoredRecurrence keeps MonthlyNth=-1 and
// sets the weekday to Tuesday => BYDAY=-1TU. But the LAST Tuesday of April 2024
// is Apr 30, not Apr 23 — so the moved anchor falls outside its own rule.
func TestReanchoredRecurrenceLastWeekdayBackwardMoveRepro(t *testing.T) {
	// Last Wednesday of April 2024.
	dtstart := "DTSTART:20240424T090000Z\r\nDTEND:20240424T093000Z\r\n"
	ev := eventForReanchor(t, "FREQ=MONTHLY;BYDAY=-1WE", dtstart)

	newStart := time.Date(2024, 4, 23, 9, 0, 0, 0, time.UTC) // Tuesday, one day back
	if newStart.Weekday() != time.Tuesday {
		t.Fatalf("test setup: newStart is %v, want Tuesday", newStart.Weekday())
	}

	spec, blocked := ReanchoredRecurrence(ev, newStart)
	if blocked || spec == nil {
		t.Fatalf("got blocked=%v spec=%v", blocked, spec)
	}
	t.Logf("re-anchored spec: MonthlyNth=%d MonthlyWeekday=%v", spec.MonthlyNth, spec.MonthlyWeekday)

	// Build the rule the grab move would write, anchored at the moved DTSTART, and
	// ask whether the moved instance actually occurs. This is the exact invariant
	// the function exists to guarantee.
	opt := spec.ROption()
	opt.Dtstart = newStart
	rule, err := rrule.NewRRule(*opt)
	if err != nil {
		t.Fatalf("building rule: %v", err)
	}
	set := &rrule.Set{}
	set.RRule(rule)

	occs := set.Between(
		time.Date(2024, 4, 1, 0, 0, 0, 0, time.UTC),
		time.Date(2024, 5, 1, 0, 0, 0, 0, time.UTC),
		true,
	)
	t.Logf("April 2024 occurrences under re-anchored rule: %v", occs)

	found := false
	for _, o := range occs {
		if o.Equal(newStart) {
			found = true
		}
	}
	if !found {
		t.Errorf("BUG: moved DTSTART %v is NOT in its own recurrence set %v — the moved instance vanishes and the series jumps to the last Tuesday",
			newStart, occs)
	}
}
