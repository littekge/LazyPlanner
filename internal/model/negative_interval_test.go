package model

import (
	"testing"
	"time"

	"github.com/teambition/rrule-go"
)

// TestNegativeIntervalIsNotRepresentable guards the pass-23 MED: the decomposer
// only copied `Interval > 1`, so a negative INTERVAL was silently dropped and an
// unbuildable rule was declared inside the editable vocabulary. rrule-go refuses
// to build FREQ=WEEKLY;INTERVAL=-1, so the item really has a single occurrence —
// but the Detail pane rendered "Weekly on Mon" and a grab day-move rewrote the
// rule into a real, unbounded weekly series, discarding the original bytes.
func TestNegativeIntervalIsNotRepresentable(t *testing.T) {
	// 2026-07-13 is a Monday.
	dtstart := "DTSTART:20260713T090000Z\r\nDTEND:20260713T093000Z\r\n"
	const rule = "FREQ=WEEKLY;INTERVAL=-1;BYDAY=MO"
	ev := eventForReanchor(t, rule, dtstart)

	from := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	occ, err := ev.Occurrences(from, to)
	if err == nil && len(occ) > 1 {
		t.Fatalf("premise broken: rule actually expands (%d occurrences)", len(occ))
	}

	// (1) An unbuildable rule is not representable.
	option, err := ev.Raw.Props.RecurrenceRule()
	if err != nil || option == nil {
		t.Fatalf("RecurrenceRule: %v", err)
	}
	if option.Interval != -1 {
		t.Fatalf("premise broken: parsed Interval = %d, want -1", option.Interval)
	}
	if spec, ok := RecurSpecFromRule(option, ev.Start); ok {
		t.Errorf("RecurSpecFromRule accepted an unbuildable INTERVAL=-1 rule as representable; spec=%+v", spec)
	}

	// (2) The Detail pane must not claim a weekly series that does not exist.
	if summary := RecurrenceSummary(ev.Raw, ev.Start, time.UTC); summary == "Weekly on Mon" {
		t.Errorf("RecurrenceSummary = %q for a rule that fires once", summary)
	}

	// (3) A one-day grab nudge must block rather than mint an infinite series.
	newStart := ev.Start.AddDate(0, 0, 1) // Tue
	spec, blocked := ReanchoredRecurrence(ev, newStart)
	if !blocked {
		t.Errorf("ReanchoredRecurrence: blocked=false for an unbuildable rule — a day-move "+
			"silently converts a 1-instance item into an unbounded weekly series (spec=%+v)", spec)
	}

	// (4) ok=false means the original bytes survive untouched.
	if prop := ev.Raw.Props.Get("RRULE"); prop == nil || prop.Value != rule {
		t.Errorf("RRULE bytes = %v, want %q untouched", prop, rule)
	}
}

// TestIntervalBoundaryDecomposition pins the whole INTERVAL boundary row so the
// negative-interval fix is neither under- nor over-broad: negative is rejected,
// 0 and 1 both normalize to the spec's zero value ("every"), and 2 is carried.
func TestIntervalBoundaryDecomposition(t *testing.T) {
	anchor := time.Date(2026, 7, 13, 9, 0, 0, 0, time.UTC) // a Monday
	tests := []struct {
		rule         string
		wantOK       bool
		wantInterval int
	}{
		{"FREQ=WEEKLY;INTERVAL=-2;BYDAY=MO", false, 0},
		{"FREQ=WEEKLY;INTERVAL=-1;BYDAY=MO", false, 0},
		{"FREQ=WEEKLY;INTERVAL=0;BYDAY=MO", true, 0},
		{"FREQ=WEEKLY;INTERVAL=1;BYDAY=MO", true, 0},
		{"FREQ=WEEKLY;INTERVAL=2;BYDAY=MO", true, 2},
		{"FREQ=DAILY;INTERVAL=-1", false, 0},
		{"FREQ=MONTHLY;INTERVAL=-1", false, 0},
		{"FREQ=YEARLY;INTERVAL=-1", false, 0},
	}
	for _, tc := range tests {
		t.Run(tc.rule, func(t *testing.T) {
			opt, err := rrule.StrToROption(tc.rule)
			if err != nil {
				t.Fatalf("StrToROption(%q): %v", tc.rule, err)
			}
			spec, ok := RecurSpecFromRule(opt, anchor)
			if ok != tc.wantOK {
				t.Fatalf("RecurSpecFromRule(%q) ok=%v, want %v", tc.rule, ok, tc.wantOK)
			}
			if ok && spec.Interval != tc.wantInterval {
				t.Errorf("spec.Interval = %d, want %d", spec.Interval, tc.wantInterval)
			}
		})
	}
}
