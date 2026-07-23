package model

import (
	"testing"
	"time"
)

// TestRecurSpecROptionExtended pins the RRULE serialization for the v1.3.0
// extended vocabulary: interval, a weekly weekday set, monthly by nth weekday
// (incl. last), and the two end conditions (UNTIL / COUNT). Monthly by
// day-of-month and yearly carry no BY* — the anchor (DTSTART/DUE) supplies the
// day, so those specs serialize to a bare FREQ.
func TestRecurSpecROptionExtended(t *testing.T) {
	until := time.Date(2026, 12, 12, 0, 0, 0, 0, time.UTC)
	tests := []struct {
		name string
		spec RecurSpec
		want string
	}{
		{"daily every 3", RecurSpec{Freq: FreqDaily, Interval: 3}, "FREQ=DAILY;INTERVAL=3"},
		{"interval 1 is bare", RecurSpec{Freq: FreqDaily, Interval: 1}, "FREQ=DAILY"},
		{"interval 0 is bare", RecurSpec{Freq: FreqDaily, Interval: 0}, "FREQ=DAILY"},
		{
			"weekly set", RecurSpec{Freq: FreqWeekly, Weekdays: []time.Weekday{time.Tuesday, time.Thursday}},
			"FREQ=WEEKLY;BYDAY=TU,TH",
		},
		{
			"weekly interval + set + count",
			RecurSpec{Freq: FreqWeekly, Interval: 2, Weekdays: []time.Weekday{time.Tuesday, time.Thursday}, Count: 10},
			"FREQ=WEEKLY;INTERVAL=2;COUNT=10;BYDAY=TU,TH",
		},
		{
			"weekly set until",
			RecurSpec{Freq: FreqWeekly, Weekdays: []time.Weekday{time.Tuesday, time.Thursday}, Until: &until},
			"FREQ=WEEKLY;UNTIL=20261212T000000Z;BYDAY=TU,TH",
		},
		{"monthly by day-of-month is bare", RecurSpec{Freq: FreqMonthly}, "FREQ=MONTHLY"},
		{
			"monthly nth weekday",
			RecurSpec{Freq: FreqMonthly, MonthlyNth: 4, MonthlyWeekday: time.Tuesday},
			"FREQ=MONTHLY;BYDAY=+4TU",
		},
		{
			"monthly last weekday",
			RecurSpec{Freq: FreqMonthly, MonthlyNth: -1, MonthlyWeekday: time.Tuesday},
			"FREQ=MONTHLY;BYDAY=-1TU",
		},
		{"yearly is bare", RecurSpec{Freq: FreqYearly}, "FREQ=YEARLY"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.spec.ROption().RRuleString(); got != tc.want {
				t.Errorf("RRuleString = %q, want %q", got, tc.want)
			}
		})
	}
}

// TestRecurSpecHumanize covers the humanizer used by the Repeat dropdown and the
// Detail pane. Interval-1 rules render as capitalized single words ("Weekly on
// Tue"); interval>1 as "every N units". The anchor supplies the parts the spec
// derives from the start/due date: a plain monthly rule's day-of-month, a yearly
// rule's month and day, and a weekly rule with no explicit weekday set.
func TestRecurSpecHumanize(t *testing.T) {
	until := time.Date(2026, 12, 12, 0, 0, 0, 0, time.UTC)
	anchor := time.Date(2026, 8, 25, 0, 0, 0, 0, time.UTC) // a Tuesday
	if anchor.Weekday() != time.Tuesday {
		t.Fatalf("test setup: anchor is %v, expected Tuesday", anchor.Weekday())
	}
	tests := []struct {
		name string
		spec RecurSpec
		want string
	}{
		{"daily", RecurSpec{Freq: FreqDaily}, "Daily"},
		{"every 3 days", RecurSpec{Freq: FreqDaily, Interval: 3}, "every 3 days"},
		{"weekly explicit day", RecurSpec{Freq: FreqWeekly, Weekdays: []time.Weekday{time.Tuesday}}, "Weekly on Tue"},
		{"weekly derives day from anchor", RecurSpec{Freq: FreqWeekly}, "Weekly on Tue"},
		{
			"weekly set sorted monday-first",
			RecurSpec{Freq: FreqWeekly, Weekdays: []time.Weekday{time.Thursday, time.Tuesday}},
			"Weekly on Tue, Thu",
		},
		{
			"every 2 weeks on set until",
			RecurSpec{Freq: FreqWeekly, Interval: 2, Weekdays: []time.Weekday{time.Tuesday, time.Thursday}, Until: &until},
			"every 2 weeks on Tue, Thu until Dec 12, 2026",
		},
		{"monthly by day-of-month", RecurSpec{Freq: FreqMonthly}, "Monthly on day 25"},
		{
			"monthly nth weekday",
			RecurSpec{Freq: FreqMonthly, MonthlyNth: 4, MonthlyWeekday: time.Tuesday},
			"Monthly on the 4th Tuesday",
		},
		{
			"monthly last weekday",
			RecurSpec{Freq: FreqMonthly, MonthlyNth: -1, MonthlyWeekday: time.Tuesday},
			"Monthly on the last Tuesday",
		},
		{"yearly", RecurSpec{Freq: FreqYearly}, "Yearly on Aug 25"},
		{"count", RecurSpec{Freq: FreqWeekly, Weekdays: []time.Weekday{time.Monday}, Count: 10}, "Weekly on Mon for 10 times"},
		{"count one", RecurSpec{Freq: FreqDaily, Count: 1}, "Daily for 1 time"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.spec.Humanize(anchor); got != tc.want {
				t.Errorf("Humanize = %q, want %q", got, tc.want)
			}
		})
	}
}
