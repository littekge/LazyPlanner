package model_test

import (
	"math"
	"runtime"
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

	// The property is a GROWTH RATIO, not a duration. Without the shared budget
	// cost scales with N, so 250 far-anchored SECONDLY events cost ~5× the
	// 50-event case; with it both cost the same and the ratio sits near 1.
	//
	// This used to assert an absolute 800ms per size, which is the wrong shape for
	// the claim and made the test a CI flake: a shared runner measured 804ms — a
	// 0.5% overshoot — and failed a run that had found nothing wrong. Absolute
	// wall-clock thresholds encode the machine, not the invariant.
	const (
		maxGrowthRatio = 3.0 // observed ~1.0 bounded, ~5.0 if per-N scaling returns
		// A deliberately loose backstop: a ratio cannot see a regression that makes
		// BOTH sizes hang, so keep an absolute ceiling far above any real machine
		// (~270ms observed here, so ~37× margin) purely to catch catastrophe.
		sanityCeiling = 10 * time.Second
	)

	parsed := map[int]*model.Parsed{}
	for _, n := range []int{50, 250} {
		p, err := model.Decode([]byte(floodICS(n)), time.UTC)
		if err != nil {
			t.Fatalf("n=%d decode: %v", n, err)
		}
		if len(p.Events) != n {
			t.Fatalf("n=%d: got %d events, want %d", n, len(p.Events), n)
		}
		parsed[n] = p
	}

	// Best-of-3, interleaved across sizes: a CPU-frequency dip or a noisy
	// neighbour then has to hit the same size three times to skew the ratio,
	// rather than landing once on a single timed run.
	best := map[int]time.Duration{50: math.MaxInt64, 250: math.MaxInt64}
	for round := 0; round < 3; round++ {
		for _, n := range []int{50, 250} {
			runtime.GC()
			start := time.Now()
			occs, err := parsed[n].EventOccurrences(from, to)
			elapsed := time.Since(start)
			if err != nil {
				t.Fatalf("n=%d EventOccurrences: %v", n, err)
			}
			if len(occs) != 0 {
				t.Fatalf("n=%d: the flood fixture must collect nothing, got %d occurrences", n, len(occs))
			}
			if elapsed < best[n] {
				best[n] = elapsed
			}
		}
	}

	ratio := float64(best[250]) / float64(best[50])
	t.Logf("best-of-3: n=50 %v, n=250 %v — growth ratio %.2f× (bounded ≈1, per-N scaling ≈5)",
		best[50], best[250], ratio)

	if ratio > maxGrowthRatio {
		t.Fatalf("expansion grew %.2f× for 5× the events (> %.1f×) — the shared StepBudget is not bounding the sum across events",
			ratio, maxGrowthRatio)
	}
	for _, n := range []int{50, 250} {
		if best[n] > sanityCeiling {
			t.Fatalf("n=%d: aggregate expansion took %v, past the %v catastrophe backstop", n, best[n], sanityCeiling)
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
