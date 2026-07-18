package model

import (
	"testing"
	"time"
)

// TestSplitDoesNotDuplicateTrailingRDate guards the fix for the duplicated-RDATE
// bug: UNTIL bounds only the RRULE generator, so a trailing RDATE beyond the
// split point used to be emitted by the capped past half (CapSeries left it in
// place) AND the future series (NewSeriesFrom copied it) — one more occurrence
// than the original. filterRDates now partitions RDATEs at the split point so
// each lands in exactly one half.
const weeklyEventWithTrailingRDate = "BEGIN:VCALENDAR\r\nVERSION:2.0\r\nPRODID:-//t//t//EN\r\n" +
	"BEGIN:VEVENT\r\nUID:wkrd@t\r\nSUMMARY:Standup\r\nDTSTAMP:20260701T000000Z\r\n" +
	"DTSTART:20260706T090000Z\r\nDTEND:20260706T093000Z\r\nRRULE:FREQ=WEEKLY;COUNT=4\r\n" +
	"RDATE:20260907T090000Z\r\n" +
	"END:VEVENT\r\nEND:VCALENDAR\r\n"

func TestSplitDoesNotDuplicateTrailingRDate(t *testing.T) {
	obj := decodeForTest(t, weeklyEventWithTrailingRDate)
	now := d(2026, 7, 1, 0)

	// Original series: 07-06, 07-13, 07-20, 07-27 (RRULE COUNT=4) + 09-07 (RDATE) = 5.
	orig := eventStarts(t, obj)
	if len(orig) != 5 {
		t.Fatalf("original series has %d occurrences, want 5", len(orig))
	}

	occ := d(2026, 7, 20, 9) // split at the 3rd RRULE instance

	capped, future, err := SplitEvent(obj, "wkrd@t", occ,
		EventDraft{Summary: "Renamed", Start: d(2026, 7, 20, 11), End: d(2026, 7, 20, 12)},
		now, time.UTC)
	if err != nil {
		t.Fatal(err)
	}

	cappedStarts := eventStarts(t, capped)
	futureStarts := eventStarts(t, future)

	// Count how many times the trailing RDATE instant (09-07) appears across halves.
	rdate := d(2026, 9, 7, 9)
	count := 0
	for _, s := range append(append([]time.Time{}, cappedStarts...), futureStarts...) {
		if s.Equal(rdate) {
			count++
		}
	}

	t.Logf("capped starts:  %v", cappedStarts)
	t.Logf("future starts:  %v", futureStarts)
	t.Logf("trailing RDATE (%s) appears %d time(s) across both halves", rdate, count)

	if count != 1 {
		t.Errorf("trailing RDATE appears %d times across the split halves, want exactly 1", count)
	}

	// The split must conserve occurrence count: 5 original -> 5 total across halves.
	total := len(cappedStarts) + len(futureStarts)
	if total != len(orig) {
		t.Errorf("split total occurrences = %d, want %d (original count must be conserved)", total, len(orig))
	}
}
