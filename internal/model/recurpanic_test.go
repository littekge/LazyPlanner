package model

import (
	"testing"
	"time"
)

// TestAdvanceRecurringTodoDegradesOnDegenerateRule guards H2: the write-side
// recurrence expansion (nextInstantAfter) previously called rrule-go directly and
// panicked (index out of range in calcDaySet) on a near-zero anchor year — a
// crash reachable by Space-completing a malformed recurring todo. It must now
// degrade gracefully like the read path, never panic.
func TestAdvanceRecurringTodoDegradesOnDegenerateRule(t *testing.T) {
	ics := "BEGIN:VCALENDAR\r\nVERSION:2.0\r\nPRODID:-//x//x//EN\r\n" +
		"BEGIN:VTODO\r\nUID:degen-1\r\nDTSTAMP:20260101T000000Z\r\n" +
		"DTSTART:00000101T000000Z\r\nDUE:00000102T000000Z\r\n" +
		"RRULE:FREQ=WEEKLY\r\nSUMMARY:t\r\nEND:VTODO\r\nEND:VCALENDAR\r\n"
	obj, err := Decode([]byte(ics), time.UTC)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	// Must not panic. A degenerate rule degrades to "series exhausted" (done).
	out, done, err := AdvanceRecurringTodo(obj, "degen-1", time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), time.UTC)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out == nil {
		t.Fatal("nil result")
	}
	if !done {
		t.Log("note: degenerate rule did not report done; acceptable as long as it did not panic")
	}
}

// TestSplitEventDegradesOnDegenerateRule guards the same class for the event
// split path (occurrencesBefore → safeBetween).
func TestSplitEventDegradesOnDegenerateRule(t *testing.T) {
	ics := "BEGIN:VCALENDAR\r\nVERSION:2.0\r\nPRODID:-//x//x//EN\r\n" +
		"BEGIN:VEVENT\r\nUID:degen-2\r\nDTSTAMP:20260101T000000Z\r\n" +
		"DTSTART:00000101T000000Z\r\nDTEND:00000101T010000Z\r\n" +
		"RRULE:FREQ=WEEKLY;COUNT=5\r\nSUMMARY:e\r\nEND:VEVENT\r\nEND:VCALENDAR\r\n"
	obj, err := Decode([]byte(ics), time.UTC)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	occ := time.Date(1, 1, 8, 0, 0, 0, 0, time.UTC)
	// Must not panic; either succeeds or returns an error, but never crashes.
	_, _, err = SplitEvent(obj, "degen-2", occ, EventDraft{Summary: "x"}, time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), time.UTC)
	t.Logf("SplitEvent on degenerate rule returned err=%v (no panic)", err)
}
