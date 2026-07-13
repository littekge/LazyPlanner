package model

import (
	"strings"
	"testing"
	"time"

	"github.com/emersion/go-ical"
)

func countAlarms(comp *ical.Component) int {
	n := 0
	for _, ch := range comp.Children {
		if ch.Name == "VALARM" {
			n++
		}
	}
	return n
}

const alarmedRecurringEvent = "BEGIN:VCALENDAR\r\nVERSION:2.0\r\nPRODID:-//x//x//EN\r\n" +
	"BEGIN:VEVENT\r\nUID:alarm-1\r\nDTSTAMP:20260101T000000Z\r\n" +
	"DTSTART:20260106T090000Z\r\nDTEND:20260106T100000Z\r\n" +
	"RRULE:FREQ=WEEKLY\r\nSUMMARY:e\r\n" +
	"BEGIN:VALARM\r\nACTION:DISPLAY\r\nTRIGGER:-PT15M\r\nX-CUSTOM:keepme\r\nEND:VALARM\r\n" +
	"END:VEVENT\r\nEND:VCALENDAR\r\n"

// TestOccurrenceOverridePreservesAlarm guards H3: "edit this occurrence" of an
// alarmed recurring event must keep the VALARM on the override, not drop it.
func TestOccurrenceOverridePreservesAlarm(t *testing.T) {
	obj, err := Decode([]byte(alarmedRecurringEvent), time.UTC)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	occ := time.Date(2026, 1, 13, 9, 0, 0, 0, time.UTC)
	out, err := AddOccurrenceOverride(obj, "alarm-1", occ, false, func(c *ical.Component) {
		c.Props.SetText(ical.PropSummary, "edited")
	}, time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), time.UTC)
	if err != nil {
		t.Fatalf("override: %v", err)
	}
	// Two components share the UID now: the master (1 alarm) and the override (must
	// also have 1). Total 2 — before the fix the override contributed 0.
	total := 0
	var overrideHadAlarm bool
	for _, c := range out.Calendar.Children {
		if text(c.Props, ical.PropUID) != "alarm-1" {
			continue
		}
		total += countAlarms(c)
		if c.Props.Get(ical.PropRecurrenceID) != nil && countAlarms(c) == 1 {
			overrideHadAlarm = true
		}
	}
	if total != 2 || !overrideHadAlarm {
		t.Errorf("override dropped the VALARM: total alarms=%d, overrideHadAlarm=%v (want 2, true)", total, overrideHadAlarm)
	}
	// The custom X- param inside the alarm must survive too (iron rule).
	if enc := encodeToString(t, out); !strings.Contains(enc, "X-CUSTOM:keepme") {
		t.Error("override lost the VALARM's X-CUSTOM property")
	}
}

// TestSplitEventPreservesAlarm guards H4: the future half of a this-and-future
// split must carry the master's VALARM.
func TestSplitEventPreservesAlarm(t *testing.T) {
	obj, err := Decode([]byte(alarmedRecurringEvent), time.UTC)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	occ := time.Date(2026, 1, 20, 9, 0, 0, 0, time.UTC)
	_, future, err := SplitEvent(obj, "alarm-1", occ, EventDraft{Summary: "future"}, time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), time.UTC)
	if err != nil {
		t.Fatalf("split: %v", err)
	}
	if len(future.Events) == 0 {
		t.Fatal("split produced no future event")
	}
	alarms := 0
	for _, c := range future.Calendar.Children {
		alarms += countAlarms(c)
	}
	if alarms != 1 {
		t.Errorf("future series has %d VALARMs, want 1 (dropped on split)", alarms)
	}
	if enc := encodeToString(t, future); !strings.Contains(enc, "X-CUSTOM:keepme") {
		t.Error("future series lost the VALARM's X-CUSTOM property")
	}
}

func encodeToString(t *testing.T, p *Parsed) string {
	t.Helper()
	var b strings.Builder
	if err := ical.NewEncoder(&b).Encode(p.Calendar); err != nil {
		t.Fatalf("encode: %v", err)
	}
	return b.String()
}
