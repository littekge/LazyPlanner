package model

import (
	"testing"
	"time"
)

// eventForReanchor decodes a one-VEVENT calendar and returns the event.
func eventForReanchor(t *testing.T, rrule, dtstart string) *Event {
	t.Helper()
	ics := "BEGIN:VCALENDAR\r\nVERSION:2.0\r\nPRODID:-//t//t//EN\r\n" +
		"BEGIN:VEVENT\r\nUID:e@t\r\nSUMMARY:X\r\nDTSTAMP:20260701T000000Z\r\n" +
		dtstart + "RRULE:" + rrule + "\r\nEND:VEVENT\r\nEND:VCALENDAR\r\n"
	obj := decodeForTest(t, ics)
	for _, e := range obj.Events {
		if e.UID == "e@t" {
			return e
		}
	}
	t.Fatal("event not decoded")
	return nil
}

// TestReanchoredRecurrence covers the grab-day-move rule re-anchoring: a
// day-pinning BY* is shifted/re-derived to the moved start; a rule with no
// day-pinning BY* needs no rewrite (nil); an unrepresentable rule blocks.
func TestReanchoredRecurrence(t *testing.T) {
	dtMon := "DTSTART:20260706T090000Z\r\nDTEND:20260706T093000Z\r\n" // Mon 2026-07-06
	nextDay := time.Date(2026, 7, 7, 9, 0, 0, 0, time.UTC)            // Tue (a +1 day move)

	t.Run("weekly single weekday shifts", func(t *testing.T) {
		ev := eventForReanchor(t, "FREQ=WEEKLY;BYDAY=MO", dtMon)
		spec, blocked := ReanchoredRecurrence(ev, nextDay)
		if blocked || spec == nil {
			t.Fatalf("got blocked=%v spec=%v, want a re-anchored spec", blocked, spec)
		}
		if len(spec.Weekdays) != 1 || spec.Weekdays[0] != time.Tuesday {
			t.Errorf("weekdays = %v, want [Tuesday] (Mon shifted +1)", spec.Weekdays)
		}
	})

	t.Run("weekly multi weekday shifts as a set", func(t *testing.T) {
		ev := eventForReanchor(t, "FREQ=WEEKLY;BYDAY=MO,TH", dtMon)
		spec, blocked := ReanchoredRecurrence(ev, nextDay)
		if blocked || spec == nil {
			t.Fatalf("got blocked=%v spec=%v", blocked, spec)
		}
		if len(spec.Weekdays) != 2 || spec.Weekdays[0] != time.Tuesday || spec.Weekdays[1] != time.Friday {
			t.Errorf("weekdays = %v, want [Tue Fri] (Mon,Thu shifted +1)", spec.Weekdays)
		}
	})

	t.Run("weekly plain needs no rewrite", func(t *testing.T) {
		ev := eventForReanchor(t, "FREQ=WEEKLY", dtMon)
		spec, blocked := ReanchoredRecurrence(ev, nextDay)
		if blocked || spec != nil {
			t.Errorf("plain weekly: got blocked=%v spec=%v, want (nil,false) — DTSTART move suffices", blocked, spec)
		}
	})

	t.Run("monthly by day-of-month needs no rewrite", func(t *testing.T) {
		ev := eventForReanchor(t, "FREQ=MONTHLY", dtMon)
		spec, blocked := ReanchoredRecurrence(ev, nextDay)
		if blocked || spec != nil {
			t.Errorf("monthly-by-day: got blocked=%v spec=%v, want (nil,false)", blocked, spec)
		}
	})

	t.Run("monthly nth-weekday re-derives", func(t *testing.T) {
		// 2026-07-06 is the 1st Monday of July; +1 day = 2026-07-07, the 1st Tuesday.
		ev := eventForReanchor(t, "FREQ=MONTHLY;BYDAY=1MO", dtMon)
		spec, blocked := ReanchoredRecurrence(ev, nextDay)
		if blocked || spec == nil {
			t.Fatalf("got blocked=%v spec=%v", blocked, spec)
		}
		if spec.MonthlyNth != 1 || spec.MonthlyWeekday != time.Tuesday {
			t.Errorf("monthly nth = %d/%v, want 1/Tuesday", spec.MonthlyNth, spec.MonthlyWeekday)
		}
	})

	t.Run("daily needs no rewrite", func(t *testing.T) {
		ev := eventForReanchor(t, "FREQ=DAILY", dtMon)
		spec, blocked := ReanchoredRecurrence(ev, nextDay)
		if blocked || spec != nil {
			t.Errorf("daily: got blocked=%v spec=%v, want (nil,false)", blocked, spec)
		}
	})

	t.Run("unrepresentable rule blocks", func(t *testing.T) {
		// BYSETPOS is outside the editable vocabulary → RecurSpecFromRule rejects it.
		ev := eventForReanchor(t, "FREQ=MONTHLY;BYDAY=MO;BYSETPOS=1", dtMon)
		spec, blocked := ReanchoredRecurrence(ev, nextDay)
		if !blocked {
			t.Errorf("kept/custom rule: got blocked=%v spec=%v, want blocked=true", blocked, spec)
		}
	})

	t.Run("non-recurring event is a no-op", func(t *testing.T) {
		ics := "BEGIN:VCALENDAR\r\nVERSION:2.0\r\nPRODID:-//t//t//EN\r\n" +
			"BEGIN:VEVENT\r\nUID:e@t\r\nSUMMARY:X\r\nDTSTAMP:20260701T000000Z\r\n" +
			dtMon + "END:VEVENT\r\nEND:VCALENDAR\r\n"
		obj := decodeForTest(t, ics)
		spec, blocked := ReanchoredRecurrence(obj.Events[0], nextDay)
		if blocked || spec != nil {
			t.Errorf("non-recurring: got blocked=%v spec=%v, want (nil,false)", blocked, spec)
		}
	})
}
