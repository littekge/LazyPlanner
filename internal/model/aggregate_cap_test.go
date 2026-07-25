package model_test

import (
	"strings"
	"testing"
	"time"

	"github.com/littekge/LazyPlanner/internal/model"
)

// floodICS builds a VCALENDAR of n VEVENTs, each anchored 126 years before the
// query window with FREQ=SECONDLY — the pathological shape that forces every
// event's skip-forward loop to run its full per-event step budget and collect
// nothing. Before the aggregate StepBudget these multiplied: N × per-event cost.
func floodICS(n int) string {
	var b strings.Builder
	b.WriteString("BEGIN:VCALENDAR\r\nVERSION:2.0\r\nPRODID:-//x//x//EN\r\n")
	for i := 0; i < n; i++ {
		b.WriteString("BEGIN:VEVENT\r\nUID:flood-")
		b.WriteByte(byte('a' + i%26))
		b.WriteByte(byte('0' + (i/26)%10))
		b.WriteByte(byte('0' + (i/260)%10))
		b.WriteString("\r\nDTSTART:19000101T000000Z\r\nRRULE:FREQ=SECONDLY\r\n")
		b.WriteString("SUMMARY:Flood\r\nEND:VEVENT\r\n")
	}
	b.WriteString("END:VCALENDAR\r\n")
	return b.String()
}

// TestAggregateRecurrenceCapBounded is the Pass-21 MED regression guard: the
// per-event step cap holds, but EventOccurrences loops every event, so before
// the shared StepBudget a resource full of far-anchored high-frequency events
// froze each redraw (50 events ≈ 5s, returning 0 occurrences). The aggregate cap
// keeps the whole expansion bounded regardless of how many such events exist.
func TestAggregateRecurrenceCapBounded(t *testing.T) {
	from := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)

	// The bound is by budget, not by event count: expanding 250 events must not
	// take ~5× as long as 50. If cost still scaled with N, 250 far-anchored
	// SECONDLY events would take ~25s; the shared budget holds it well under the
	// ceiling. A generous threshold keeps the guard robust on slow CI/Pi hardware
	// while still catching a regression that reintroduces the per-N multiplication.
	const budgetBoundMs = 800
	for _, n := range []int{50, 250} {
		p, err := model.Decode([]byte(floodICS(n)), time.UTC)
		if err != nil {
			t.Fatalf("n=%d decode: %v", n, err)
		}
		if len(p.Events) != n {
			t.Fatalf("n=%d: got %d events, want %d", n, len(p.Events), n)
		}
		start := time.Now()
		occs, err := p.EventOccurrences(from, to)
		elapsed := time.Since(start)
		if err != nil {
			t.Fatalf("n=%d EventOccurrences: %v", n, err)
		}
		t.Logf("n=%d: EventOccurrences took %v, returned %d occurrences", n, elapsed, len(occs))
		if elapsed > budgetBoundMs*time.Millisecond {
			t.Fatalf("n=%d: aggregate expansion took %v (> %dms) — the shared StepBudget is not bounding the sum across events",
				n, elapsed, budgetBoundMs)
		}
	}
}

// TestAggregateCapDoesNotStarveLegitEvent pins the other half of the trade-off:
// the aggregate budget sits far above what a realistic view needs, so a single
// pathological sibling must not suppress a normal event's expansion. A weekly
// event alongside a far-anchored SECONDLY flood still yields its real instances.
func TestAggregateCapDoesNotStarveLegitEvent(t *testing.T) {
	const ics = "BEGIN:VCALENDAR\r\nVERSION:2.0\r\nPRODID:-//x//x//EN\r\n" +
		"BEGIN:VEVENT\r\nUID:flood\r\nDTSTART:19000101T000000Z\r\nRRULE:FREQ=SECONDLY\r\n" +
		"SUMMARY:Flood\r\nEND:VEVENT\r\n" +
		"BEGIN:VEVENT\r\nUID:standup\r\nDTSTART:20260105T090000Z\r\nDTEND:20260105T093000Z\r\n" +
		"RRULE:FREQ=WEEKLY;BYDAY=MO\r\nSUMMARY:Standup\r\nEND:VEVENT\r\n" +
		"END:VCALENDAR\r\n"

	p, err := model.Decode([]byte(ics), time.UTC)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	from := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)
	occs, err := p.EventOccurrences(from, to)
	if err != nil {
		t.Fatalf("EventOccurrences: %v", err)
	}
	var standups int
	for _, o := range occs {
		if o.Event.UID == "standup" {
			standups++
		}
	}
	// Mondays in Jan 2026: 5, 12, 19, 26 → 4 weekly instances, none starved by
	// the pathological sibling sharing the budget.
	if standups != 4 {
		t.Fatalf("legit weekly event starved by pathological sibling: got %d standup occurrences, want 4", standups)
	}
}
