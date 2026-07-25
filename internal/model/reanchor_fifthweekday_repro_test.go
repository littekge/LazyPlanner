package model

import (
	"testing"
	"time"

	"github.com/teambition/rrule-go"
)

// TestReanchoredRecurrencePositiveNthOntoFifthWeekdayRepro reproduces the
// Pass 19 MED finding: a positive-nth monthly rule ("2nd Wednesday") re-anchored
// onto a date that is its new weekday's 5th occurrence in the month must resolve
// to BYDAY=-1<wd> (the editable vocabulary's "last"), never a literal
// MonthlyNth=5 — BYDAY=5<wd> means something different (and thinner) than "last":
// it fires only in months that happen to have five of that weekday, dropping the
// series' occurrences in every other month.
//
// April 2024: DTSTART on the 2nd Wednesday (Apr 10). Re-anchoring onto Apr 30 (a
// Tuesday) — April's 5th and last Tuesday (2, 9, 16, 23, 30) — must NOT carry the
// old positive-nth derivation forward into MonthlyNth=5; it must recognize Apr 30
// as the month's last Tuesday and emit BYDAY=-1TU instead.
func TestReanchoredRecurrencePositiveNthOntoFifthWeekdayRepro(t *testing.T) {
	dtstart := "DTSTART:20240410T090000Z\r\nDTEND:20240410T093000Z\r\n" // 2nd Wednesday of April 2024
	ev := eventForReanchor(t, "FREQ=MONTHLY;BYDAY=2WE", dtstart)

	newStart := time.Date(2024, 4, 30, 9, 0, 0, 0, time.UTC) // Tuesday, the month's 5th (and last)
	if newStart.Weekday() != time.Tuesday {
		t.Fatalf("test setup: newStart is %v, want Tuesday", newStart.Weekday())
	}

	spec, blocked := ReanchoredRecurrence(ev, newStart)
	if blocked || spec == nil {
		t.Fatalf("got blocked=%v spec=%v, want a re-anchored spec (Apr 30 IS representable as \"last Tuesday\")", blocked, spec)
	}
	t.Logf("re-anchored spec: MonthlyNth=%d MonthlyWeekday=%v", spec.MonthlyNth, spec.MonthlyWeekday)

	if spec.MonthlyNth == 5 {
		t.Fatalf("BUG: MonthlyNth=5 escapes the editable 1st-4th/last vocabulary — want -1 (last), since Apr 30 is April's last Tuesday")
	}
	if spec.MonthlyNth != -1 {
		t.Fatalf("MonthlyNth=%d, want -1 (last) — Apr 30 is April's last Tuesday, not just its 5th", spec.MonthlyNth)
	}

	// Confirm the emitted rule actually fires on the moved DTSTART (the same
	// non-vanishing invariant the HIGH finding covers) rather than just checking
	// the decoded field in isolation.
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
		t.Errorf("moved DTSTART %v is NOT in its own recurrence set %v", newStart, occs)
	}
}
