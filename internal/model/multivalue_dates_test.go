package model

import "testing"

// TestMultiValuedEXDATE guards against a regression where a comma-listed
// (multi-valued) EXDATE on one property line collapsed a recurring series to its
// DTSTART base instance, because resolveDateTime parsed only a single date-time.
//
// RFC 5545 permits EXDATE (and RDATE) to carry a comma-separated list of values
// on a single property line. Here FREQ=DAILY;COUNT=5 from 2026-01-01 should yield
// 5 instances; excluding 01-02 and 01-04 should leave 3.
func TestMultiValuedEXDATE(t *testing.T) {
	const ics = "BEGIN:VCALENDAR\r\nVERSION:2.0\r\nPRODID:-//t//t//EN\r\n" +
		"BEGIN:VEVENT\r\nUID:mx@t\r\nSUMMARY:Daily\r\nDTSTAMP:20260101T000000Z\r\n" +
		"DTSTART:20260101T100000Z\r\nDTEND:20260101T103000Z\r\n" +
		"RRULE:FREQ=DAILY;COUNT=5\r\n" +
		"EXDATE:20260102T100000Z,20260104T100000Z\r\n" +
		"END:VEVENT\r\nEND:VCALENDAR\r\n"

	obj := decodeForTest(t, ics)
	starts := eventStarts(t, obj)
	t.Logf("visible instances: %v (len %d)", starts, len(starts))

	want := 3 // 5 daily minus 2 excluded
	if len(starts) != want {
		t.Errorf("multi-valued EXDATE collapsed series: got %d instances, want %d", len(starts), want)
	}
}

// TestMultiValuedRDATE is the RDATE analogue: two extra dates listed on one
// comma-joined RDATE line should add two instances to a single-instance event.
func TestMultiValuedRDATE(t *testing.T) {
	const ics = "BEGIN:VCALENDAR\r\nVERSION:2.0\r\nPRODID:-//t//t//EN\r\n" +
		"BEGIN:VEVENT\r\nUID:mr@t\r\nSUMMARY:Extra\r\nDTSTAMP:20260101T000000Z\r\n" +
		"DTSTART:20260101T100000Z\r\nDTEND:20260101T103000Z\r\n" +
		"RDATE:20260103T100000Z,20260105T100000Z\r\n" +
		"END:VEVENT\r\nEND:VCALENDAR\r\n"

	obj := decodeForTest(t, ics)
	starts := eventStarts(t, obj)
	t.Logf("visible instances: %v (len %d)", starts, len(starts))

	want := 3 // DTSTART + 2 RDATE
	if len(starts) != want {
		t.Errorf("multi-valued RDATE collapsed series: got %d instances, want %d", len(starts), want)
	}
}
