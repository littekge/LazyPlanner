package model

import (
	"testing"
	"time"

	"github.com/emersion/go-ical"
)

// TestSplitCarriesFutureOverride guards H5: a RECURRENCE-ID override on an
// occurrence after the split point must survive a this-and-future split (carried
// into the new series, re-keyed to its UID) instead of being dropped from both
// halves.
func TestSplitCarriesFutureOverride(t *testing.T) {
	ics := "BEGIN:VCALENDAR\r\nVERSION:2.0\r\nPRODID:-//x//x//EN\r\n" +
		// master: weekly from Jan 6
		"BEGIN:VEVENT\r\nUID:split-1\r\nDTSTAMP:20260101T000000Z\r\n" +
		"DTSTART:20260106T090000Z\r\nDTEND:20260106T100000Z\r\n" +
		"RRULE:FREQ=WEEKLY\r\nSUMMARY:base\r\nEND:VEVENT\r\n" +
		// override: the Jan 20 occurrence is customized
		"BEGIN:VEVENT\r\nUID:split-1\r\nDTSTAMP:20260101T000000Z\r\n" +
		"RECURRENCE-ID:20260120T090000Z\r\n" +
		"DTSTART:20260120T090000Z\r\nDTEND:20260120T100000Z\r\nSUMMARY:custom\r\nEND:VEVENT\r\n" +
		"END:VCALENDAR\r\n"
	obj, err := Decode([]byte(ics), time.UTC)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	// Split "this & future" from the Jan 13 occurrence (before the Jan 20 override).
	occ := time.Date(2026, 1, 13, 9, 0, 0, 0, time.UTC)
	_, future, err := SplitEvent(obj, "split-1", occ, EventDraft{Summary: "future-base"}, time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), time.UTC)
	if err != nil {
		t.Fatalf("split: %v", err)
	}
	if len(future.Events) == 0 {
		t.Fatal("no future series")
	}
	newUID := future.Events[0].UID

	var found bool
	for _, c := range future.Calendar.Children {
		rid := c.Props.Get(ical.PropRecurrenceID)
		if rid == nil {
			continue
		}
		ridTime, err := resolveDateTime(rid, time.UTC)
		if err != nil || ridTime.Unix() != time.Date(2026, 1, 20, 9, 0, 0, 0, time.UTC).Unix() {
			continue
		}
		found = true
		if got := text(c.Props, ical.PropUID); got != newUID {
			t.Errorf("carried override UID = %q, want the new series UID %q", got, newUID)
		}
		if got := text(c.Props, ical.PropSummary); got != "custom" {
			t.Errorf("carried override SUMMARY = %q, want %q (customization lost)", got, "custom")
		}
	}
	if !found {
		t.Error("future override was dropped by the split (lost from both halves)")
	}
}
