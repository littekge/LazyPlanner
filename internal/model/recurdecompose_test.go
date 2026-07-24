package model

import (
	"testing"
	"time"

	"github.com/teambition/rrule-go"
)

// specEqual compares two RecurSpecs on the fields the decomposer sets (the
// quick-add-only Month/Day anchor is never produced by decomposition).
func specEqual(a, b RecurSpec) bool {
	if a.Freq != b.Freq || a.Interval != b.Interval || a.Count != b.Count {
		return false
	}
	if (a.Until == nil) != (b.Until == nil) {
		return false
	}
	if a.Until != nil && !a.Until.Equal(*b.Until) {
		return false
	}
	if len(a.Weekdays) != len(b.Weekdays) {
		return false
	}
	for i := range a.Weekdays {
		if a.Weekdays[i] != b.Weekdays[i] {
			return false
		}
	}
	if a.MonthlyNth != b.MonthlyNth {
		return false
	}
	if a.MonthlyNth != 0 && a.MonthlyWeekday != b.MonthlyWeekday {
		return false
	}
	return true
}

// TestRecurSpecRoundTrip asserts every representable spec survives
// serialize→parse→decompose unchanged (identity), given a matching anchor.
func TestRecurSpecRoundTrip(t *testing.T) {
	until := time.Date(2026, 12, 12, 0, 0, 0, 0, time.UTC)
	fourthTue := time.Date(2026, 12, 22, 0, 0, 0, 0, time.UTC) // 4th Tuesday, not last
	lastTue := time.Date(2026, 12, 29, 0, 0, 0, 0, time.UTC)   // 5th = last Tuesday
	dayAnchor := time.Date(2026, 8, 25, 0, 0, 0, 0, time.UTC)
	if fourthTue.Weekday() != time.Tuesday || lastTue.Weekday() != time.Tuesday {
		t.Fatal("test setup: anchor weekdays wrong")
	}

	tests := []struct {
		name   string
		spec   RecurSpec
		anchor time.Time
	}{
		{"daily", RecurSpec{Freq: FreqDaily}, dayAnchor},
		{"daily interval", RecurSpec{Freq: FreqDaily, Interval: 3}, dayAnchor},
		{"daily count", RecurSpec{Freq: FreqDaily, Count: 5}, dayAnchor},
		{"weekly bare", RecurSpec{Freq: FreqWeekly}, dayAnchor},
		{"weekly set", RecurSpec{Freq: FreqWeekly, Weekdays: []time.Weekday{time.Tuesday, time.Thursday}}, dayAnchor},
		{
			"weekly interval set until",
			RecurSpec{Freq: FreqWeekly, Interval: 2, Weekdays: []time.Weekday{time.Tuesday, time.Thursday}, Until: &until},
			dayAnchor,
		},
		{"monthly by day-of-month", RecurSpec{Freq: FreqMonthly}, dayAnchor},
		{"monthly 4th tuesday", RecurSpec{Freq: FreqMonthly, MonthlyNth: 4, MonthlyWeekday: time.Tuesday}, fourthTue},
		{"monthly last tuesday", RecurSpec{Freq: FreqMonthly, MonthlyNth: -1, MonthlyWeekday: time.Tuesday}, lastTue},
		{"yearly", RecurSpec{Freq: FreqYearly}, dayAnchor},
		{"yearly count", RecurSpec{Freq: FreqYearly, Count: 3}, dayAnchor},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			str := tc.spec.ROption().RRuleString()
			opt, err := rrule.StrToROption(str)
			if err != nil {
				t.Fatalf("StrToROption(%q): %v", str, err)
			}
			got, ok := RecurSpecFromRule(opt, tc.anchor)
			if !ok {
				t.Fatalf("RecurSpecFromRule(%q) not ok, want representable", str)
			}
			if !specEqual(got, tc.spec) {
				t.Errorf("round-trip of %q\n got %+v\nwant %+v", str, got, tc.spec)
			}
		})
	}
}

// TestRecurSpecFromRuleUnrepresentable pins the conservative catalogue: any rule
// outside the editable vocabulary, or one that contradicts its own anchor, must
// return ok=false (so the caller keeps it as raw "Custom (kept)" bytes).
func TestRecurSpecFromRuleUnrepresentable(t *testing.T) {
	anchor := time.Date(2026, 12, 22, 0, 0, 0, 0, time.UTC) // 4th Tuesday of Dec 2026
	tests := []struct {
		name string
		rule string
	}{
		{"bysetpos", "FREQ=MONTHLY;BYSETPOS=-1;BYDAY=MO,TU,WE,TH,FR"},
		{"hourly", "FREQ=HOURLY"},
		{"minutely", "FREQ=MINUTELY;INTERVAL=30"},
		{"byyearday", "FREQ=YEARLY;BYYEARDAY=100"},
		{"byweekno", "FREQ=YEARLY;BYWEEKNO=20"},
		{"byhour", "FREQ=DAILY;BYHOUR=9"},
		{"wkst not monday", "FREQ=WEEKLY;WKST=SU;BYDAY=MO"},
		{"nth ordinal on weekly", "FREQ=WEEKLY;BYDAY=+2TU"},
		{"monthly plain byday no ordinal", "FREQ=MONTHLY;BYDAY=TU"},
		{"monthly bymonthday contradicts anchor", "FREQ=MONTHLY;BYMONTHDAY=15"}, // anchor is the 22nd
		{"monthly multi bymonthday", "FREQ=MONTHLY;BYMONTHDAY=15,22"},
		{"monthly nth wrong weekday", "FREQ=MONTHLY;BYDAY=+4MO"},  // anchor is a Tuesday
		{"monthly nth wrong position", "FREQ=MONTHLY;BYDAY=+2TU"}, // anchor is the 4th Tuesday
		{"monthly 5th weekday", "FREQ=MONTHLY;BYDAY=+5TU"},        // outside 1..4/last
		{"monthly byday and bymonthday", "FREQ=MONTHLY;BYMONTHDAY=22;BYDAY=+4TU"},
		{"monthly bymonth", "FREQ=MONTHLY;BYMONTH=12"},
		{"yearly multi bymonth", "FREQ=YEARLY;BYMONTH=1,7"},
		{"yearly bymonth contradicts anchor", "FREQ=YEARLY;BYMONTH=1"}, // anchor is December
		{"yearly byday", "FREQ=YEARLY;BYDAY=+1TU"},
		{"count and until", "FREQ=DAILY;COUNT=3;UNTIL=20261212T000000Z"},
		{"daily byday", "FREQ=DAILY;BYDAY=MO"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			opt, err := rrule.StrToROption(tc.rule)
			if err != nil {
				t.Fatalf("StrToROption(%q): %v", tc.rule, err)
			}
			if _, ok := RecurSpecFromRule(opt, anchor); ok {
				t.Errorf("RecurSpecFromRule(%q) = ok, want unrepresentable", tc.rule)
			}
		})
	}
}

// TestRecurSpecFromRuleAnchorConsistent verifies rules that DO match their
// anchor are accepted (the complement of the contradiction rejections).
func TestRecurSpecFromRuleAnchorConsistent(t *testing.T) {
	anchor := time.Date(2026, 12, 22, 0, 0, 0, 0, time.UTC) // 4th Tuesday of Dec 2026
	rules := []string{
		"FREQ=MONTHLY;BYMONTHDAY=22", // matches anchor day
		"FREQ=MONTHLY;BYDAY=+4TU",    // matches anchor position
		"FREQ=YEARLY;BYMONTH=12",     // matches anchor month
		"FREQ=YEARLY;BYMONTHDAY=22",  // matches anchor day
		"FREQ=YEARLY;BYMONTH=12;BYMONTHDAY=22",
	}
	for _, r := range rules {
		t.Run(r, func(t *testing.T) {
			opt, err := rrule.StrToROption(r)
			if err != nil {
				t.Fatalf("StrToROption(%q): %v", r, err)
			}
			if _, ok := RecurSpecFromRule(opt, anchor); !ok {
				t.Errorf("RecurSpecFromRule(%q) not ok, want representable", r)
			}
		})
	}
}
