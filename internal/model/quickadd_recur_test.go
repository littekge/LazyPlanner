package model

import (
	"testing"
	"time"

	"github.com/emersion/go-ical"
)

// TestParseQuickAddRecurrence covers the v1.2.0 simple recurrence set: bare
// daily/weekly/monthly/yearly, "every day/week/month/year", "every <weekday>",
// and "every <month> <day>", plus the anchoring rule (a form implying a
// specific date sets the date when none was typed; an explicit date wins).
func TestParseQuickAddRecurrence(t *testing.T) {
	loc := time.UTC
	now := time.Date(2026, 7, 5, 9, 0, 0, 0, loc) // a Sunday

	tests := []struct {
		name        string
		input       string
		title       string
		wantRecur   bool
		freq        RecurFreq
		hasWeekday  bool
		weekday     time.Weekday
		hasMonthDay bool
		month       time.Month
		day         int
		hasDate     bool
		date        time.Time
	}{
		{name: "bare daily", input: "standup daily", title: "standup", wantRecur: true, freq: FreqDaily},
		{name: "bare weekly", input: "review weekly", title: "review", wantRecur: true, freq: FreqWeekly},
		{name: "bare monthly", input: "rent monthly", title: "rent", wantRecur: true, freq: FreqMonthly},
		{name: "bare yearly", input: "birthday yearly", title: "birthday", wantRecur: true, freq: FreqYearly},
		{name: "every day", input: "sync every day", title: "sync", wantRecur: true, freq: FreqDaily},
		{name: "every week", input: "sync every week", title: "sync", wantRecur: true, freq: FreqWeekly},
		{name: "every month", input: "sync every month", title: "sync", wantRecur: true, freq: FreqMonthly},
		{name: "every year", input: "sync every year", title: "sync", wantRecur: true, freq: FreqYearly},
		{
			name: "every weekday anchors to soonest", input: "gym every mon", title: "gym",
			wantRecur: true, freq: FreqWeekly, hasWeekday: true, weekday: time.Monday,
			hasDate: true, date: time.Date(2026, 7, 6, 0, 0, 0, 0, loc), // Mon after Sun Jul 5
		},
		{
			name: "every month day anchors to that date", input: "party every jul 20", title: "party",
			wantRecur: true, freq: FreqYearly, hasMonthDay: true, month: time.July, day: 20,
			hasDate: true, date: time.Date(2026, 7, 20, 0, 0, 0, 0, loc),
		},
		{
			name: "explicit date wins the anchor", input: "meeting fri every week", title: "meeting",
			wantRecur: true, freq: FreqWeekly,
			hasDate: true, date: time.Date(2026, 7, 10, 0, 0, 0, 0, loc), // Fri, not the every-week anchor
		},
		{
			name: "explicit date wins over every weekday anchor", input: "standup every mon 2026-08-01", title: "standup",
			wantRecur: true, freq: FreqWeekly, hasWeekday: true, weekday: time.Monday,
			hasDate: true, date: time.Date(2026, 8, 1, 0, 0, 0, 0, loc),
		},
		{
			name: "accepted trade-off daily is a token", input: "daily standup 9am", title: "standup",
			wantRecur: true, freq: FreqDaily,
		},
		// Non-matches: the word stays in the title.
		{name: "everyone is not every", input: "everyone is here", title: "everyone is here"},
		{name: "trailing every with no follower", input: "clean every", title: "clean every"},
		{name: "every with unknown follower", input: "every so often", title: "every so often"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			qa := ParseQuickAdd(tc.input, now, loc)
			if qa.Title != tc.title {
				t.Errorf("Title = %q, want %q", qa.Title, tc.title)
			}
			if (qa.Recur != nil) != tc.wantRecur {
				t.Fatalf("Recur present = %v, want %v (Title=%q)", qa.Recur != nil, tc.wantRecur, qa.Title)
			}
			if !tc.wantRecur {
				return
			}
			if qa.Recur.Freq != tc.freq {
				t.Errorf("Freq = %v, want %v", qa.Recur.Freq, tc.freq)
			}
			gotHasWeekday := len(qa.Recur.Weekdays) > 0
			gotWeekday := time.Weekday(-1)
			if gotHasWeekday {
				gotWeekday = qa.Recur.Weekdays[0]
			}
			if gotHasWeekday != tc.hasWeekday || (tc.hasWeekday && gotWeekday != tc.weekday) {
				t.Errorf("weekday = (%v,%v), want (%v,%v)", gotHasWeekday, gotWeekday, tc.hasWeekday, tc.weekday)
			}
			if qa.Recur.HasMonthDay != tc.hasMonthDay || (tc.hasMonthDay && (qa.Recur.Month != tc.month || qa.Recur.Day != tc.day)) {
				t.Errorf("monthday = (%v,%v,%v), want (%v,%v,%v)", qa.Recur.HasMonthDay, qa.Recur.Month, qa.Recur.Day, tc.hasMonthDay, tc.month, tc.day)
			}
			if qa.HasDate != tc.hasDate {
				t.Fatalf("HasDate = %v, want %v", qa.HasDate, tc.hasDate)
			}
			if tc.hasDate && !qa.Date.Equal(tc.date) {
				t.Errorf("Date = %s, want %s", qa.Date.Format("2006-01-02"), tc.date.Format("2006-01-02"))
			}
		})
	}
}

// TestRecurSpecRRuleString pins the RRULE serialization for each form.
func TestRecurSpecRRuleString(t *testing.T) {
	tests := []struct {
		name string
		spec RecurSpec
		want string
	}{
		{"daily", RecurSpec{Freq: FreqDaily}, "FREQ=DAILY"},
		{"weekly", RecurSpec{Freq: FreqWeekly}, "FREQ=WEEKLY"},
		{"monthly", RecurSpec{Freq: FreqMonthly}, "FREQ=MONTHLY"},
		{"yearly", RecurSpec{Freq: FreqYearly}, "FREQ=YEARLY"},
		{"every weekday", RecurSpec{Freq: FreqWeekly, Weekdays: []time.Weekday{time.Monday}}, "FREQ=WEEKLY;BYDAY=MO"},
		{"every sunday", RecurSpec{Freq: FreqWeekly, Weekdays: []time.Weekday{time.Sunday}}, "FREQ=WEEKLY;BYDAY=SU"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.spec.ROption().RRuleString(); got != tc.want {
				t.Errorf("RRuleString = %q, want %q", got, tc.want)
			}
		})
	}
}

// TestNewObjectsSerializeRecurrence verifies a draft carrying a RecurSpec
// produces an object that parses back as recurring with the expected RRULE, for
// both events and todos.
func TestNewObjectsSerializeRecurrence(t *testing.T) {
	now := time.Date(2026, 7, 6, 9, 0, 0, 0, time.UTC)
	spec := &RecurSpec{Freq: FreqWeekly, Weekdays: []time.Weekday{time.Monday}}

	ev, err := NewEventObject(EventDraft{
		Summary: "Standup",
		Start:   time.Date(2026, 7, 6, 9, 0, 0, 0, time.UTC),
		End:     time.Date(2026, 7, 6, 9, 30, 0, 0, time.UTC),
		Recur:   spec,
	}, now)
	if err != nil {
		t.Fatalf("NewEventObject: %v", err)
	}
	if !ev.Events[0].Recurring {
		t.Error("event not recurring")
	}
	if got := rruleString(t, ev.Events[0].Raw); got != "FREQ=WEEKLY;BYDAY=MO" {
		t.Errorf("event RRULE = %q, want FREQ=WEEKLY;BYDAY=MO", got)
	}

	td := NewTodoObject(TodoDraft{
		Summary: "Chores",
		HasDue:  true,
		Due:     time.Date(2026, 7, 6, 0, 0, 0, 0, time.UTC),
		Recur:   spec,
	}, now)
	if !td.Todos[0].Recurring {
		t.Error("todo not recurring")
	}
	if got := rruleString(t, td.Todos[0].Raw); got != "FREQ=WEEKLY;BYDAY=MO" {
		t.Errorf("todo RRULE = %q, want FREQ=WEEKLY;BYDAY=MO", got)
	}
}

func rruleString(t *testing.T, comp *ical.Component) string {
	t.Helper()
	p := comp.Props.Get(ical.PropRecurrenceRule)
	if p == nil {
		t.Fatal("no RRULE property")
	}
	return p.Value
}
