package model

import (
	"testing"
	"time"
)

// TestParseQuickAddRelativeDates covers the v1.2.0 relative-date grammar:
// "next <weekday>" (bare-weekday result + 7 days), "next week" (today+7),
// "next month" (same day-of-month next month, clamped), and "in N days/weeks/
// months" (singular units too; N is 1–3 digits; months clamp like next month).
func TestParseQuickAddRelativeDates(t *testing.T) {
	loc := time.UTC
	// A Sunday, so weekday math is easy to reason about.
	now := time.Date(2026, 7, 5, 9, 0, 0, 0, loc)

	tests := []struct {
		name    string
		input   string
		title   string
		hasDate bool
		date    time.Time
	}{
		{
			name:    "next weekday is bare-weekday plus seven",
			input:   "Party next fri",
			title:   "Party",
			hasDate: true,
			// Fri after Sun Jul 5 is Jul 10; +7 = Jul 17.
			date: time.Date(2026, 7, 17, 0, 0, 0, 0, loc),
		},
		{
			name:    "next week is today plus seven",
			input:   "next week review",
			title:   "review",
			hasDate: true,
			date:    time.Date(2026, 7, 12, 0, 0, 0, 0, loc),
		},
		{
			name:    "next month keeps day of month",
			input:   "plan next month",
			title:   "plan",
			hasDate: true,
			date:    time.Date(2026, 8, 5, 0, 0, 0, 0, loc),
		},
		{
			name:    "in N days",
			input:   "call in 3 days",
			title:   "call",
			hasDate: true,
			date:    time.Date(2026, 7, 8, 0, 0, 0, 0, loc),
		},
		{
			name:    "in 1 day singular unit",
			input:   "in 1 day thing",
			title:   "thing",
			hasDate: true,
			date:    time.Date(2026, 7, 6, 0, 0, 0, 0, loc),
		},
		{
			name:    "in N weeks",
			input:   "trip in 2 weeks",
			title:   "trip",
			hasDate: true,
			date:    time.Date(2026, 7, 19, 0, 0, 0, 0, loc),
		},
		{
			name:    "in N months",
			input:   "review in 2 months",
			title:   "review",
			hasDate: true,
			date:    time.Date(2026, 9, 5, 0, 0, 0, 0, loc),
		},
		{
			name:    "in 999 months upper bound",
			input:   "x in 999 months",
			title:   "x",
			hasDate: true,
			// Jul 2026 + 999 months = Jul 2109 + 3 months = Oct 2109.
			date: time.Date(2109, 10, 5, 0, 0, 0, 0, loc),
		},
		// Non-matches: the anchor word stays in the title.
		{
			name:  "in with a non-unit follower stays in title",
			input: "meeting in room 5",
			title: "meeting in room 5",
		},
		{
			name:  "in with a four-digit count stays in title",
			input: "in 2026 days",
			title: "in 2026 days",
		},
		{
			name:  "in with an unrecognized unit stays in title",
			input: "in 5 minutes",
			title: "in 5 minutes",
		},
		{
			name:  "next with a non-keyword follower stays in title",
			input: "next steps",
			title: "next steps",
		},
		{
			name:  "next year is not a relative date",
			input: "next year",
			title: "next year",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			qa := ParseQuickAdd(tc.input, now, loc)
			if qa.Title != tc.title {
				t.Errorf("Title = %q, want %q", qa.Title, tc.title)
			}
			if qa.HasDate != tc.hasDate {
				t.Fatalf("HasDate = %v, want %v (Title=%q)", qa.HasDate, tc.hasDate, qa.Title)
			}
			if tc.hasDate && !qa.Date.Equal(tc.date) {
				t.Errorf("Date = %s, want %s", qa.Date.Format("2006-01-02"), tc.date.Format("2006-01-02"))
			}
		})
	}
}

// TestParseQuickAddNextWeekdayOnThatWeekday is the boundary the plan calls out:
// "next fri" typed on a Friday must land a full week out, never today.
func TestParseQuickAddNextWeekdayOnThatWeekday(t *testing.T) {
	loc := time.UTC
	friday := time.Date(2026, 7, 10, 9, 0, 0, 0, loc) // a Friday
	qa := ParseQuickAdd("standup next fri", friday, loc)
	if !qa.HasDate {
		t.Fatalf("HasDate = false, want true (Title=%q)", qa.Title)
	}
	want := time.Date(2026, 7, 17, 0, 0, 0, 0, loc) // Friday + 7, not the same Friday
	if !qa.Date.Equal(want) {
		t.Errorf("Date = %s, want %s", qa.Date.Format("2006-01-02"), want.Format("2006-01-02"))
	}
}

// TestParseQuickAddRelativeMonthClamp guards the day-of-month clamp for the
// month-carrying relative forms: a day past the target month's length lands on
// its last day rather than spilling into the following month.
func TestParseQuickAddRelativeMonthClamp(t *testing.T) {
	loc := time.UTC

	tests := []struct {
		name  string
		input string
		now   time.Time
		date  time.Time
	}{
		{
			name:  "next month clamps Jan 31 to non-leap Feb 28",
			input: "x next month",
			now:   time.Date(2026, 1, 31, 9, 0, 0, 0, loc),
			date:  time.Date(2026, 2, 28, 0, 0, 0, 0, loc),
		},
		{
			name:  "next month clamps Jan 31 to leap-year Feb 29",
			input: "x next month",
			now:   time.Date(2028, 1, 31, 9, 0, 0, 0, loc),
			date:  time.Date(2028, 2, 29, 0, 0, 0, 0, loc),
		},
		{
			name:  "in 1 month clamps like next month",
			input: "x in 1 month",
			now:   time.Date(2026, 1, 31, 9, 0, 0, 0, loc),
			date:  time.Date(2026, 2, 28, 0, 0, 0, 0, loc),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			qa := ParseQuickAdd(tc.input, tc.now, loc)
			if !qa.HasDate {
				t.Fatalf("HasDate = false, want true (Title=%q)", qa.Title)
			}
			if !qa.Date.Equal(tc.date) {
				t.Errorf("Date = %s, want %s", qa.Date.Format("2006-01-02"), tc.date.Format("2006-01-02"))
			}
		})
	}
}
