package model

import (
	"testing"
	"time"

	"github.com/emersion/go-ical"
)

// TestRDatePeriodTZIDZoned guards the Pass-17 fix: a VALUE=PERIOD RDATE must be
// zoned by its TZID, and the IANA-name path must agree with the Windows-name
// path. Before the fix the stale VALUE=PERIOD param made go-ical reject the
// reduced value, and resolveDateTime had no IANA recovery branch, so an IANA
// TZID silently fell through to the floating fallback (mis-zoned by its offset)
// while a Windows TZID resolved correctly — the two paths disagreed.
func TestRDatePeriodTZIDZoned(t *testing.T) {
	// 10:00 EST == 15:00 UTC. Both TZID spellings must land here.
	want := time.Date(2026, 1, 1, 15, 0, 0, 0, time.UTC)

	cases := []struct {
		name string
		tzid string
	}{
		{"IANA name", "America/New_York"},
		{"Windows name", "Eastern Standard Time"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			prop := &ical.Prop{
				Name:  ical.PropRecurrenceDates,
				Value: "20260101T100000/PT1H",
				Params: ical.Params{
					ical.ParamValue:      []string{string(ical.ValuePeriod)},
					ical.ParamTimezoneID: []string{tc.tzid},
				},
			}

			// loc (the calendar's fallback zone) is UTC and deliberately wrong for
			// this value; a correct resolution must ignore it and use the TZID.
			got, err := resolveDateTimeValues(prop, time.UTC)
			if err != nil {
				t.Fatalf("resolveDateTimeValues: %v", err)
			}
			if len(got) != 1 {
				t.Fatalf("got %d times, want 1", len(got))
			}
			if !got[0].Equal(want) {
				t.Fatalf("PERIOD RDATE mis-zoned:\n got  = %s\n want = %s",
					got[0].UTC().Format(time.RFC3339), want.Format(time.RFC3339))
			}
		})
	}
}

// TestResolveDateTimeIANATZIDRecovery guards the IANA-TZID recovery branch added
// to resolveDateTime directly: when go-ical's DateTime rejects a prop for a
// reason other than the zone but the TZID is a loadable IANA zone, the value is
// zoned by that TZID rather than dropped to floating time.
func TestResolveDateTimeIANATZIDRecovery(t *testing.T) {
	prop := &ical.Prop{
		Name:  ical.PropRecurrenceDates,
		Value: "20260101T100000",
		Params: ical.Params{
			// A value-type param go-ical's DateTime switch cannot handle, forcing
			// the recovery path while the TZID itself is a valid IANA zone.
			ical.ParamValue:      []string{string(ical.ValuePeriod)},
			ical.ParamTimezoneID: []string{"America/New_York"},
		},
	}
	got, err := resolveDateTime(prop, time.UTC)
	if err != nil {
		t.Fatalf("resolveDateTime: %v", err)
	}
	want := time.Date(2026, 1, 1, 15, 0, 0, 0, time.UTC)
	if !got.Equal(want) {
		t.Fatalf("IANA-TZID recovery mis-zoned:\n got  = %s\n want = %s",
			got.UTC().Format(time.RFC3339), want.Format(time.RFC3339))
	}
}
