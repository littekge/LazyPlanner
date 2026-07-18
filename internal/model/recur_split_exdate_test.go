package model

import (
	"testing"
)

// TestSplitPreservesCountWithPreSplitEXDATE guards the fix for the phantom-
// occurrence bug: a this-and-future split on a COUNT-bounded master that carries
// a pre-split EXDATE (reachable via delete-this-occurrence) must not add a
// trailing occurrence. RFC 5545 COUNT bounds the RRULE generator, so an EXDATE'd
// instance still consumes COUNT — the split's past-half count is RRULE
// iterations, not the EXDATE-filtered visible set (which would leave the future
// COUNT one too high).
//
// Scenario: FREQ=DAILY;COUNT=5 from day1 (07-06). Delete day2 (07-07) -> EXDATE.
// Visible = day1,day3,day4,day5 = 4. Then split "this and future" at day4 (07-09).
// Total visible after split must stay 4.
func TestSplitPreservesCountWithPreSplitEXDATE(t *testing.T) {
	const src = "BEGIN:VCALENDAR\r\nVERSION:2.0\r\nPRODID:-//t//t//EN\r\n" +
		"BEGIN:VEVENT\r\nUID:ex@t\r\nSUMMARY:Daily\r\nDTSTAMP:20260701T000000Z\r\n" +
		"DTSTART:20260706T090000Z\r\nDTEND:20260706T093000Z\r\nRRULE:FREQ=DAILY;COUNT=5\r\n" +
		"END:VEVENT\r\nEND:VCALENDAR\r\n"
	obj := decodeForTest(t, src)
	now := d(2026, 7, 1, 0)

	// Delete this occurrence: day2 (07-07 09:00).
	deleted, err := AddException(obj, "ex@t", d(2026, 7, 7, 9), false, now, nil)
	if err != nil {
		t.Fatal(err)
	}
	before := eventStarts(t, deleted)
	t.Logf("after delete-day2, visible: %v (n=%d)", before, len(before))
	if len(before) != 4 {
		t.Fatalf("precondition: expected 4 visible after EXDATE, got %d", len(before))
	}

	// Edit this-and-future starting day4 (07-09 09:00).
	occ := d(2026, 7, 9, 9)
	capped, future, err := SplitEvent(deleted, "ex@t", occ, EventDraft{
		Summary: "Daily", Start: occ, End: occ.Add(30 * 60 * 1e9),
	}, now, nil)
	if err != nil {
		t.Fatal(err)
	}
	cappedStarts := eventStarts(t, capped)
	futureStarts := eventStarts(t, future)
	total := len(cappedStarts) + len(futureStarts)
	t.Logf("capped:  %v (n=%d)", cappedStarts, len(cappedStarts))
	t.Logf("future:  %v (n=%d)", futureStarts, len(futureStarts))
	t.Logf("split total = %d (want %d, the pre-split visible count)", total, len(before))

	if total != len(before) {
		t.Fatalf("phantom occurrence: total after split = %d, want %d; future=%v",
			total, len(before), futureStarts)
	}
}
