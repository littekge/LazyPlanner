package model

import (
	"testing"
	"time"
	// Embed the IANA database so a host without system zoneinfo cannot skip these
	// into a vacuous green (the same reason vtimezone_test.go embeds it).
	_ "time/tzdata"

	"github.com/emersion/go-ical"
	"github.com/teambition/rrule-go"
)

// selfConsistencyZones are the zones swept for "the emitted RRULE generates the
// emitted DTSTART". The first three are the confirmed failures of deriving BYMONTH/
// BYDAY from the transition rendered in the TO offset: each switches at or near
// local midnight, so the FROM-offset wall clock (which the DTSTART carries) and the
// TO-offset one fall on different calendar days.
//
// The property asserted is deliberately self-consistency, NOT forward-projection
// fidelity: a compact one-observance-per-offset VTIMEZONE cannot express a zone
// whose real rule is lunar-political (Africa/Casablanca, Asia/Gaza) or was abolished
// mid-window (America/Vancouver), and that residual is inherent to the shape every
// other client emits — not a defect.
var selfConsistencyZones = []string{
	"Africa/Cairo",       // STANDARD onset 00:00 local: Friday in FROM, Thursday in TO
	"Asia/Beirut",        // midnight onset
	"America/Santiago",   // midnight onset, southern hemisphere
	"America/New_York",   // the common case, 02:00 onsets
	"Europe/Berlin",      // EU last-Sunday rules
	"Europe/Dublin",      // negative-DST zone
	"Australia/Sydney",   // southern hemisphere
	"Asia/Kolkata",       // no DST, half-hour offset
	"Pacific/Kiritimati", // no DST, +14
	"America/Sao_Paulo",  // DST abolished
	"Asia/Amman",         // last-Friday rules
	"Pacific/Auckland",
	"Europe/Lisbon",
	"America/Havana", // midnight onset in the west
	"Asia/Tehran",
}

// A generated observance's RRULE must generate its own DTSTART. Deriving BYMONTH/
// BYDAY from a different rendering of the transition than the one DTSTART carries
// leaves the rule contradicting its own anchor — the project's codified "never leave
// DTSTART contradicting its own BY*" invariant, inside the VTIMEZONE generator.
func TestObservanceRuleGeneratesItsOwnDTSTART(t *testing.T) {
	// Several anchors: which transition is "most recent" (and therefore emitted)
	// depends on where in the year the anchor sits.
	anchors := []time.Time{
		time.Date(2026, 1, 15, 12, 0, 0, 0, time.UTC),
		time.Date(2026, 4, 15, 12, 0, 0, 0, time.UTC),
		time.Date(2026, 7, 15, 12, 0, 0, 0, time.UTC),
		time.Date(2026, 11, 15, 12, 0, 0, 0, time.UTC),
	}
	checked := 0
	for _, name := range selfConsistencyZones {
		loc, err := time.LoadLocation(name)
		if err != nil {
			t.Fatalf("zone %q must load (tzdata is embedded): %v", name, err)
		}
		for _, anchor := range anchors {
			tz := BuildVTimezone(loc, anchor)
			if tz == nil {
				t.Fatalf("%s: BuildVTimezone returned nil for a named zone", name)
			}
			for _, sub := range tz.Children {
				rule := sub.Props.Get(ical.PropRecurrenceRule)
				if rule == nil {
					continue // the transition-less "one rule, always" observance
				}
				dtstart := sub.Props.Get(ical.PropDateTimeStart)
				if dtstart == nil {
					t.Fatalf("%s %s: observance has an RRULE but no DTSTART", name, sub.Name)
				}
				// An observance DTSTART is floating; parsing it in UTC gives the same
				// wall clock the rule's BY* parts are meant to describe.
				start, err := time.ParseInLocation(vtimezoneDateTimeLayout, dtstart.Value, time.UTC)
				if err != nil {
					t.Fatalf("%s %s: DTSTART %q does not parse: %v", name, sub.Name, dtstart.Value, err)
				}
				opt, err := rrule.StrToROption(rule.Value)
				if err != nil {
					t.Fatalf("%s %s: RRULE %q does not parse: %v", name, sub.Name, rule.Value, err)
				}
				opt.Dtstart = start
				r, err := rrule.NewRRule(*opt)
				if err != nil {
					t.Fatalf("%s %s: RRULE %q is not buildable: %v", name, sub.Name, rule.Value, err)
				}
				if got := r.After(start.Add(-time.Second), true); !got.Equal(start) {
					t.Errorf("%s %s anchored %s: RRULE %q first fires %s, but its own DTSTART is %s",
						name, sub.Name, anchor.Format("2006-01"), rule.Value,
						got.Format(vtimezoneDateTimeLayout), dtstart.Value)
				}
				checked++
			}
		}
	}
	if checked == 0 {
		t.Fatal("no recurring observance was checked — the sweep is vacuous")
	}
}

// The last-week ordinal is ambiguous, so earlier years of the same observance
// settle it: a year where the transition sat at the same nth WITHOUT being that
// month's last proves the rule pins a positive nth, and rendering it -1 would fire a
// week late in every five-<weekday> year.
//
// No IANA zone is known to use a positive 4th-weekday rule, so this drives the
// helper directly rather than a zone; the end-to-end guard for the surrounding
// generator is TestObservanceRuleGeneratesItsOwnDTSTART.
func TestICalNthWeekdayUsesEarlierYearsToResolveLastWeek(t *testing.T) {
	utc := time.UTC
	for _, tc := range []struct {
		name    string
		at      time.Time
		earlier []time.Time
		want    string
	}{
		{
			// 2026-10-25 is October's 4th Sunday AND its last — the ambiguous shape.
			// 2023-10-22 was the 4th of five, which a "last Sunday" rule could never
			// have produced, so the rule pins a positive 4th.
			name:    "4th weekday confirmed by a year where it was not the last",
			at:      time.Date(2026, 10, 25, 2, 0, 0, 0, utc),
			earlier: []time.Time{time.Date(2023, 10, 22, 2, 0, 0, 0, utc)},
			want:    "4SU",
		},
		{
			// Every sampled year is last-and-nth: nothing proves a positive nth, so
			// "last" (the common real rule) is kept.
			name:    "no decisive evidence keeps last",
			at:      time.Date(2026, 10, 25, 2, 0, 0, 0, utc),
			earlier: []time.Time{time.Date(2025, 10, 26, 2, 0, 0, 0, utc)},
			want:    "-1SU",
		},
		{
			// A different nth in an earlier year is what a genuine "last" rule looks
			// like — never evidence for a positive ordinal. (2027-10-31 is the 5th.)
			name:    "shifting ordinal stays last",
			at:      time.Date(2026, 10, 25, 2, 0, 0, 0, utc), // 4th and last
			earlier: []time.Time{time.Date(2027, 10, 31, 2, 0, 0, 0, utc)},
			want:    "-1SU",
		},
		{
			name:    "no earlier sample keeps last",
			at:      time.Date(2026, 10, 25, 2, 0, 0, 0, utc),
			earlier: nil,
			want:    "-1SU",
		},
		{
			// Outside the final week there is no ambiguity to resolve.
			name:    "second sunday is unambiguous",
			at:      time.Date(2026, 3, 8, 2, 0, 0, 0, utc),
			earlier: []time.Time{time.Date(2025, 3, 9, 2, 0, 0, 0, utc)},
			want:    "2SU",
		},
		{
			// A sample from another month is a rule change, not evidence — even though
			// 2024-09-22 is the 4th Sunday of five and would be decisive in October.
			name:    "evidence from a different month is ignored",
			at:      time.Date(2026, 10, 25, 2, 0, 0, 0, utc),
			earlier: []time.Time{time.Date(2024, 9, 22, 2, 0, 0, 0, utc)},
			want:    "-1SU",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := icalNthWeekday(tc.at, tc.earlier); got != tc.want {
				t.Errorf("icalNthWeekday(%s, %v) = %q, want %q",
					tc.at.Format("2006-01-02"), tc.earlier, got, tc.want)
			}
		})
	}
}

// A transition-less observance is dated in the far past so its rule covers every
// date a reader can ask about — but 1970 is only far-past relative to a modern
// anchor. A pre-1970 anchor (a 1965 start typed into the form) must not be left
// with an observance that begins after it: that is "no rule in effect" for the very
// value the VTIMEZONE exists to describe.
func TestTransitionlessObservanceIsNeverDatedAfterTheAnchor(t *testing.T) {
	// A no-DST named zone, so BuildVTimezone takes the transition-less branch.
	kolkata, err := time.LoadLocation("Asia/Kolkata")
	if err != nil {
		t.Fatalf("Asia/Kolkata must load: %v", err)
	}
	anchor := time.Date(1965, 6, 12, 9, 30, 0, 0, kolkata)
	tz := BuildVTimezone(kolkata, anchor)
	if tz == nil {
		t.Fatal("BuildVTimezone returned nil for a named zone")
	}
	if len(tz.Children) != 1 {
		t.Fatalf("want one observance for a no-DST zone, got %d", len(tz.Children))
	}
	assertOnsetAtOrBefore(t, tz.Children[0], anchor)

	// The live caller: typing a 1965 start into the item form.
	obj, err := NewEventObject(EventDraft{
		Summary: "Moon landing prep",
		Start:   anchor,
		End:     anchor.Add(time.Hour),
		Recur:   &RecurSpec{Freq: FreqWeekly, Weekdays: []time.Weekday{anchor.Weekday()}},
	}, anchor)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, c := range obj.Calendar.Children {
		if c.Name != ical.CompTimezone {
			continue
		}
		found = true
		for _, sub := range c.Children {
			assertOnsetAtOrBefore(t, sub, anchor)
		}
	}
	if !found {
		t.Fatal("the 1965 recurring event carries no VTIMEZONE")
	}

	// A modern anchor keeps the conventional 1970 onset — the guard must not move
	// the common case.
	modern := time.Date(2026, 6, 12, 9, 30, 0, 0, kolkata)
	got := BuildVTimezone(kolkata, modern).Children[0].Props.Get(ical.PropDateTimeStart).Value
	if got != vtimezoneEpochOnset {
		t.Errorf("modern anchor onset = %q, want the conventional %q", got, vtimezoneEpochOnset)
	}
}

// assertOnsetAtOrBefore fails when an observance's floating DTSTART wall clock is
// later than the anchor's own wall clock.
func assertOnsetAtOrBefore(t *testing.T, sub *ical.Component, anchor time.Time) {
	t.Helper()
	p := sub.Props.Get(ical.PropDateTimeStart)
	if p == nil {
		t.Fatalf("%s has no DTSTART", sub.Name)
	}
	onset, err := time.ParseInLocation(vtimezoneDateTimeLayout, p.Value, anchor.Location())
	if err != nil {
		t.Fatalf("%s DTSTART %q does not parse: %v", sub.Name, p.Value, err)
	}
	if onset.After(anchor) {
		t.Errorf("%s onset %s is after the anchor %s — no rule is in effect for the anchor",
			sub.Name, p.Value, anchor.Format(vtimezoneDateTimeLayout))
	}
}
