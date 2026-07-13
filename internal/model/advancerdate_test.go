package model

import (
	"testing"
	"time"
)

// TestAdvanceRecurringTodoCountOnePlusRDate guards L8: a recurring todo whose
// RRULE is COUNT=1 but which also carries an RDATE has a further occurrence, so
// completing it must advance to the RDATE instant, not mark the series done.
func TestAdvanceRecurringTodoCountOnePlusRDate(t *testing.T) {
	ics := "BEGIN:VCALENDAR\r\nVERSION:2.0\r\nPRODID:-//x//x//EN\r\n" +
		"BEGIN:VTODO\r\nUID:rd-1\r\nDTSTAMP:20260101T000000Z\r\n" +
		"DTSTART:20260106T090000Z\r\nDUE:20260106T100000Z\r\n" +
		"RRULE:FREQ=WEEKLY;COUNT=1\r\nRDATE:20260113T090000Z\r\nSUMMARY:t\r\n" +
		"END:VTODO\r\nEND:VCALENDAR\r\n"
	obj, err := Decode([]byte(ics), time.UTC)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	out, done, err := AdvanceRecurringTodo(obj, "rd-1", time.Date(2026, 1, 6, 12, 0, 0, 0, time.UTC), time.UTC)
	if err != nil {
		t.Fatalf("advance: %v", err)
	}
	if done {
		t.Fatal("series marked done, but the RDATE occurrence remains — completed one occurrence early")
	}
	// Advanced to the RDATE instant (2026-01-13), not still on 2026-01-06.
	if len(out.Todos) == 0 {
		t.Fatal("no todo after advance")
	}
	td := out.Todos[0]
	if !td.HasDue || td.Due.Day() != 13 {
		t.Errorf("advanced DUE = %v, want day 13 (the RDATE occurrence)", td.Due)
	}
}
