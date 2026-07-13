package model

import (
	"strings"
	"testing"
	"time"

	"github.com/emersion/go-ical"
)

// TestCapSeriesAllDayUntilIsDate guards M4: capping an all-day (VALUE=DATE)
// recurring series must write UNTIL as a DATE, not a DATE-TIME, per RFC 5545
// §3.3.10 (a mismatched value type can be rejected by strict servers/clients).
func TestCapSeriesAllDayUntilIsDate(t *testing.T) {
	ics := "BEGIN:VCALENDAR\r\nVERSION:2.0\r\nPRODID:-//x//x//EN\r\n" +
		"BEGIN:VEVENT\r\nUID:ad-1\r\nDTSTAMP:20260101T000000Z\r\n" +
		"DTSTART;VALUE=DATE:20260706\r\nRRULE:FREQ=WEEKLY\r\nSUMMARY:allday\r\n" +
		"END:VEVENT\r\nEND:VCALENDAR\r\n"
	obj, err := Decode([]byte(ics), time.UTC)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	until := time.Date(2026, 7, 19, 0, 0, 0, 0, time.UTC)
	out, err := CapSeries(obj, "ad-1", until, time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), time.UTC)
	if err != nil {
		t.Fatalf("cap: %v", err)
	}
	var rrule string
	for _, c := range out.Calendar.Children {
		if prop := c.Props.Get(ical.PropRecurrenceRule); prop != nil {
			rrule = prop.Value
		}
	}
	if rrule == "" {
		t.Fatal("no RRULE after cap")
	}
	untilVal := untilValue(rrule)
	if strings.ContainsRune(untilVal, 'T') {
		t.Errorf("all-day series UNTIL is a DATE-TIME (%q), want DATE, in %q", untilVal, rrule)
	}
	if untilVal != "20260719" {
		t.Errorf("UNTIL = %q, want DATE 20260719 (in %q)", untilVal, rrule)
	}
}

// untilValue extracts the UNTIL value from an RRULE string, or "" if absent.
func untilValue(rule string) string {
	for _, part := range strings.Split(rule, ";") {
		if strings.HasPrefix(part, "UNTIL=") {
			return part[len("UNTIL="):]
		}
	}
	return ""
}

// TestCapSeriesTimedUntilUnchanged confirms a timed series still gets a
// DATE-TIME UNTIL (the fix is scoped to all-day masters only).
func TestCapSeriesTimedUntilUnchanged(t *testing.T) {
	ics := "BEGIN:VCALENDAR\r\nVERSION:2.0\r\nPRODID:-//x//x//EN\r\n" +
		"BEGIN:VEVENT\r\nUID:t-1\r\nDTSTAMP:20260101T000000Z\r\n" +
		"DTSTART:20260706T090000Z\r\nDTEND:20260706T100000Z\r\nRRULE:FREQ=WEEKLY\r\nSUMMARY:timed\r\n" +
		"END:VEVENT\r\nEND:VCALENDAR\r\n"
	obj, err := Decode([]byte(ics), time.UTC)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	until := time.Date(2026, 7, 19, 9, 0, 0, 0, time.UTC)
	out, err := CapSeries(obj, "t-1", until, time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), time.UTC)
	if err != nil {
		t.Fatalf("cap: %v", err)
	}
	for _, c := range out.Calendar.Children {
		if prop := c.Props.Get(ical.PropRecurrenceRule); prop != nil {
			if until := untilValue(prop.Value); !strings.ContainsRune(until, 'T') {
				t.Errorf("timed series lost its DATE-TIME UNTIL (%q) in %q", until, prop.Value)
			}
		}
	}
}
