package model_test

import (
	"testing"
	"time"

	"github.com/littekge/LazyPlanner/internal/model"
)

// TestTodoPriorityOutOfRangeIsUndefined closes the pass-10 canary hole: no test
// exercised an out-of-range PRIORITY, so dropping the >9 clamp in priority()
// shipped undetected and would corrupt smart-sort. iCal PRIORITY is 0–9; anything
// outside is treated as undefined.
func TestTodoPriorityOutOfRangeIsUndefined(t *testing.T) {
	parse := func(pri string) *model.Todo {
		t.Helper()
		ics := "BEGIN:VCALENDAR\r\nVERSION:2.0\r\nPRODID:-//x//x//EN\r\n" +
			"BEGIN:VTODO\r\nUID:t\r\nDTSTAMP:20260101T000000Z\r\nSUMMARY:x\r\n" +
			"PRIORITY:" + pri + "\r\nEND:VTODO\r\nEND:VCALENDAR\r\n"
		p, err := model.Decode([]byte(ics), time.UTC)
		if err != nil {
			t.Fatalf("decode PRIORITY:%s: %v", pri, err)
		}
		return p.Todos[0]
	}
	for _, pri := range []string{"15", "10", "-1"} {
		if got := parse(pri).Priority; got != model.PriorityUndefined {
			t.Errorf("PRIORITY:%s -> %d, want PriorityUndefined (%d)", pri, got, model.PriorityUndefined)
		}
	}
	if got := parse("5").Priority; got != 5 {
		t.Errorf("PRIORITY:5 -> %d, want 5 (in-range preserved)", got)
	}
}
