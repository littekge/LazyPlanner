package ui

import (
	"testing"
	"time"

	"github.com/littekge/LazyPlanner/internal/model"
)

// TestEndsOnDateIncludesTimedOccurrence is the Pass-22 MED regression guard: the
// Custom repeat "Ends on date D" must include the occurrence ON D for a TIMED
// item. Before the fix readCustomRecur stored UNTIL at midnight-local of D, and
// the model only made UNTIL inclusive-of-the-day for all-day anchors — so a timed
// series' occurrence on D (at the anchor's time-of-day) fell after UNTIL 00:00 and
// was silently dropped, disagreeing with the all-day meaning of "Ends on D".
func TestEndsOnDateIncludesTimedOccurrence(t *testing.T) {
	a := newTestApp(t, time.Date(2026, 7, 1, 9, 0, 0, 0, time.UTC))

	// A timed daily series starting 15:00; user ends it "on 2026-07-25".
	start := time.Date(2026, 7, 20, 15, 0, 0, 0, a.loc)
	_, cf := a.newCustomRepeatForm(model.RecurSpec{}, start)
	cf.every.SetText("1")
	cf.unit.SetCurrentOption(0) // days
	cf.ends.SetCurrentOption(1) // On date
	cf.until.SetText("2026-07-25")
	spec, err := a.readCustomRecur(cf, start)
	if err != nil {
		t.Fatal(err)
	}

	draft := model.EventDraft{Summary: "Timed daily", Start: start, End: start.Add(time.Hour), Recur: &spec}
	p, err := model.NewEventObject(draft, start)
	if err != nil {
		t.Fatal(err)
	}
	occs, err := p.EventOccurrences(
		time.Date(2026, 7, 20, 0, 0, 0, 0, a.loc),
		time.Date(2026, 8, 1, 0, 0, 0, 0, a.loc),
	)
	if err != nil {
		t.Fatal(err)
	}

	var got []string
	found := false
	for _, o := range occs {
		ds := o.Start.In(a.loc).Format("2006-01-02")
		got = append(got, ds)
		if ds == "2026-07-25" {
			found = true
		}
	}
	if !found {
		t.Fatalf("timed 'Ends on date 2026-07-25' dropped the selected end date; occurrences: %v", got)
	}
	for _, ds := range got {
		if ds == "2026-07-26" {
			t.Fatalf("series ran past the selected end date; occurrences: %v", got)
		}
	}
}

// TestEndsOnDateAllDayUnchanged pins that the fix leaves the all-day path exactly
// as before: an all-day (midnight) anchor still yields a midnight Until, which the
// model turns into an inclusive VALUE=DATE UNTIL via dateOnlyUntil.
func TestEndsOnDateAllDayUnchanged(t *testing.T) {
	a := newTestApp(t, time.Date(2026, 7, 1, 9, 0, 0, 0, time.UTC))

	anchor := time.Date(2026, 7, 20, 0, 0, 0, 0, a.loc) // all-day: midnight anchor
	_, cf := a.newCustomRepeatForm(model.RecurSpec{}, anchor)
	cf.every.SetText("1")
	cf.unit.SetCurrentOption(0)
	cf.ends.SetCurrentOption(1)
	cf.until.SetText("2026-07-25")
	spec, err := a.readCustomRecur(cf, anchor)
	if err != nil {
		t.Fatal(err)
	}
	want := time.Date(2026, 7, 25, 0, 0, 0, 0, a.loc)
	if spec.Until == nil || !spec.Until.Equal(want) {
		t.Fatalf("all-day Until = %v, want midnight %v (unchanged)", spec.Until, want)
	}
}
