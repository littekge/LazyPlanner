package ui

import (
	"testing"
	"time"
	// Embed the IANA database in the test binary. internal/ui does not import it
	// (only cmd/lazyplanner does), so on a host without system zoneinfo the zone
	// lookup below would fail and this guard would skip its way to a vacuous green.
	_ "time/tzdata"

	"github.com/emersion/go-ical"

	"github.com/littekge/LazyPlanner/internal/model"
)

// TestWeeklyPresetKeepsItsWeekdayViaRealUIPath drives the path the reported
// defect was found on: a New York user opens the event form on a Tuesday, enters
// a 20:00 start, and picks the Repeat preset the form itself labelled "Weekly on
// Tue". The series must then fall on Tuesdays at 20:00 — including after the
// November DST transition.
//
// Before the anchor carried a TZID, DTSTART was flattened to UTC (Wednesday
// 00:00Z) while BYDAY was derived from the local Tuesday, so RFC 5545 evaluated
// the rule against the UTC weekday and the whole series fired on Mondays, one
// hour earlier after DST. The helper-level test in internal/model proves the
// serialization; this one proves the form actually produces an anchor carrying
// the zone, since a.loc is where that derivation lives.
func TestWeeklyPresetKeepsItsWeekdayViaRealUIPath(t *testing.T) {
	// Fatal, not Skip: tzdata is embedded just above, so a failure here means the
	// guard is not running — which is exactly how this class hides.
	ny, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Fatalf("America/New_York must load (tzdata is embedded): %v", err)
	}
	a := newRootedTestApp(t, time.Date(2026, 8, 1, 9, 0, 0, 0, ny))
	// The app resolves every form value in a.loc; pin it so the test does not
	// depend on the host zone (CI runs UTC, where this defect is invisible).
	a.loc = ny

	day := time.Date(2026, 8, 25, 0, 0, 0, 0, ny) // a Tuesday
	choices := a.newEventRepeat(nil, day)
	_, fields := a.newEventForm(nil, day, choices)

	fields.allDay.SetChecked(false)
	fields.startDate.SetText("2026-08-25")
	fields.startTime.SetText("20:00")
	fields.endDate.SetText("2026-08-25")
	fields.endTime.SetText("21:00")

	// Index 2 is the weekly preset; assert on its own label so a reordering of the
	// choice list fails loudly rather than silently testing a different rule.
	const weeklyPreset = 2
	if got := choices.Labels()[weeklyPreset]; got != "Weekly on Tue" {
		t.Fatalf("preset %d = %q, want %q", weeklyPreset, got, "Weekly on Tue")
	}
	fields.repeat.SetCurrentOption(weeklyPreset)

	draft, err := a.readEventDraft(fields)
	if err != nil {
		t.Fatalf("readEventDraft: %v", err)
	}
	if draft.Recur == nil {
		t.Fatal("no recurrence resolved from the form")
	}

	p, err := model.NewEventObject(draft, draft.Start)
	if err != nil {
		t.Fatalf("NewEventObject: %v", err)
	}

	// Round-trip through the bytes that would reach the server: the anchor's zone
	// only survives if it is actually serialized.
	raw, err := p.Encode()
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}
	stored, err := model.Decode(raw, ny)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	dtstart := stored.Events[0].Raw.Props.Get(ical.PropDateTimeStart)
	if tzid := dtstart.Params.Get(ical.ParamTimezoneID); tzid != "America/New_York" {
		t.Errorf("stored DTSTART TZID = %q, want America/New_York (value %q)", tzid, dtstart.Value)
	}

	occs, err := stored.EventOccurrences(
		time.Date(2026, 8, 20, 0, 0, 0, 0, ny),
		time.Date(2026, 11, 20, 0, 0, 0, 0, ny),
	)
	if err != nil {
		t.Fatalf("EventOccurrences: %v", err)
	}
	if len(occs) < 10 {
		t.Fatalf("expected a full weekly series, got %d occurrences", len(occs))
	}
	if !occs[0].Start.Equal(draft.Start) {
		t.Errorf("first occurrence = %v, want the event's own start %v", occs[0].Start, draft.Start)
	}
	for _, o := range occs {
		local := o.Start.In(ny)
		if local.Weekday() != time.Tuesday || local.Hour() != 20 {
			t.Errorf("occurrence %v is a %v at %02d:00, want Tuesday 20:00",
				local, local.Weekday(), local.Hour())
		}
	}
}
