package model

import (
	"testing"
	"time"
)

// TestSplitAtSeriesEndKeepsFutureBounded closes the Pass-17 canary escape on
// NewSeriesFrom's future-COUNT clamp. When a this-and-future split lands at or
// after the final occurrence, every RRULE iteration is consumed by the past
// half, so pastCount == COUNT and `remaining := roption.Count - pastCount`
// computes to 0. The clamp `if remaining < 1 { remaining = 1 }` forces a single
// occurrence; without it (the mutation remaining < 0) a COUNT of 0 reaches
// rrule-go, which treats COUNT=0 as *unbounded* — the future series would then
// recur forever, breaking the "two halves sum to the original count" invariant.
//
// The existing split tests only cover pre-split EXDATE/RDATE COUNT preservation;
// none splits at the series end where remaining hits 0, so the clamp was
// untested and the mutation escaped.
func TestSplitAtSeriesEndKeepsFutureBounded(t *testing.T) {
	// FREQ=DAILY;COUNT=3 from 07-06 → occurrences 07-06, 07-07, 07-08.
	const src = "BEGIN:VCALENDAR\r\nVERSION:2.0\r\nPRODID:-//t//t//EN\r\n" +
		"BEGIN:VEVENT\r\nUID:cnt@t\r\nSUMMARY:Daily\r\nDTSTAMP:20260701T000000Z\r\n" +
		"DTSTART:20260706T090000Z\r\nDTEND:20260706T093000Z\r\nRRULE:FREQ=DAILY;COUNT=3\r\n" +
		"END:VEVENT\r\nEND:VCALENDAR\r\n"
	obj := decodeForTest(t, src)
	now := d(2026, 7, 1, 0)

	base := eventStarts(t, obj)
	if len(base) != 3 {
		t.Fatalf("precondition: expected 3 occurrences, got %d: %v", len(base), base)
	}

	// Split at 07-09 — one day past the final occurrence, so pastCount == COUNT
	// and the future half's remaining COUNT computes to 0.
	occ := d(2026, 7, 9, 9)
	capped, future, err := SplitEvent(obj, "cnt@t", occ, EventDraft{
		Summary: "Daily", Start: occ, End: occ.Add(30 * time.Minute),
	}, now, nil)
	if err != nil {
		t.Fatal(err)
	}

	// The clamp must keep the future series finite: exactly one occurrence (the
	// new "this and future" instance at occ), never an unbounded COUNT=0 series.
	futureStarts := eventStarts(t, future)
	if len(futureStarts) != 1 {
		show := futureStarts
		if len(show) > 5 {
			show = show[:5]
		}
		t.Fatalf("future series is unbounded: got %d occurrences (want exactly 1); "+
			"COUNT collapsed to 0 → rrule-go treats it as infinite. first few: %v",
			len(futureStarts), show)
	}
	if !futureStarts[0].Equal(occ.UTC()) {
		t.Errorf("future occurrence at %s, want the split point %s", futureStarts[0], occ.UTC())
	}

	// The past half is unaffected by the future clamp (all three original
	// occurrences precede the split point and stay with the capped master).
	if got := len(eventStarts(t, capped)); got != len(base) {
		t.Errorf("capped half = %d occurrences, want %d", got, len(base))
	}
}
