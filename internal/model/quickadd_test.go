package model

import (
	"testing"
	"time"
)

func TestParseQuickAdd(t *testing.T) {
	loc := time.UTC
	// A Sunday, so weekday math is easy to reason about.
	now := time.Date(2026, 7, 5, 9, 0, 0, 0, loc)
	today := time.Date(2026, 7, 5, 0, 0, 0, 0, loc)

	tests := []struct {
		name     string
		input    string
		title    string
		hasDate  bool
		when     time.Time
		hasTime  bool
		priority int
		tags     []string
	}{
		{
			name:  "plain title",
			input: "Buy groceries",
			title: "Buy groceries",
		},
		{
			name:     "tags and priority",
			input:    "Email Bob !high #work #urgent",
			title:    "Email Bob",
			priority: 1,
			tags:     []string{"work", "urgent"},
		},
		{
			name:    "today keyword",
			input:   "Standup today",
			title:   "Standup",
			hasDate: true,
			when:    today,
		},
		{
			name:    "tomorrow",
			input:   "Ship it tomorrow",
			title:   "Ship it",
			hasDate: true,
			when:    today.AddDate(0, 0, 1),
		},
		{
			name:    "weekday rolls to same-or-future",
			input:   "Gym wed",
			title:   "Gym",
			hasDate: true,
			when:    time.Date(2026, 7, 8, 0, 0, 0, 0, loc), // Wed after Sun Jul 5
		},
		{
			name:    "month name and day",
			input:   "Trip jul 20",
			title:   "Trip",
			hasDate: true,
			when:    time.Date(2026, 7, 20, 0, 0, 0, 0, loc),
		},
		{
			name:    "past month day rolls to next year",
			input:   "Taxes apr 15",
			title:   "Taxes",
			hasDate: true,
			when:    time.Date(2027, 4, 15, 0, 0, 0, 0, loc),
		},
		{
			name:    "slashed date with year",
			input:   "Review 12/31/2026",
			title:   "Review",
			hasDate: true,
			when:    time.Date(2026, 12, 31, 0, 0, 0, 0, loc),
		},
		{
			name:    "iso date",
			input:   "Deadline 2026-08-01",
			title:   "Deadline",
			hasDate: true,
			when:    time.Date(2026, 8, 1, 0, 0, 0, 0, loc),
		},
		{
			name:    "time with pm defaults date to today",
			input:   "Call 3pm",
			title:   "Call",
			hasDate: true,
			hasTime: true,
			when:    time.Date(2026, 7, 5, 15, 0, 0, 0, loc),
		},
		{
			name:    "date and time together",
			input:   "Dentist jul 20 9:30am",
			title:   "Dentist",
			hasDate: true,
			hasTime: true,
			when:    time.Date(2026, 7, 20, 9, 30, 0, 0, loc),
		},
		{
			name:  "bare number stays in title",
			input: "Buy 12 eggs",
			title: "Buy 12 eggs",
		},
		{
			name:     "everything at once",
			input:    "Pay rent aug 1 5pm !1 #bills",
			title:    "Pay rent",
			hasDate:  true,
			hasTime:  true,
			when:     time.Date(2026, 8, 1, 17, 0, 0, 0, loc),
			priority: 1,
			tags:     []string{"bills"},
		},
		{
			name:  "unmatched bang stays in title",
			input: "Say !hello world",
			title: "Say !hello world",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			qa := ParseQuickAdd(tc.input, now, loc)
			if qa.Title != tc.title {
				t.Errorf("Title = %q, want %q", qa.Title, tc.title)
			}
			if qa.HasDate != tc.hasDate {
				t.Errorf("HasDate = %v, want %v", qa.HasDate, tc.hasDate)
			}
			if qa.HasTime != tc.hasTime {
				t.Errorf("HasTime = %v, want %v", qa.HasTime, tc.hasTime)
			}
			if tc.hasDate && !qa.When.Equal(tc.when) {
				t.Errorf("When = %s, want %s", qa.When, tc.when)
			}
			if qa.Priority != tc.priority {
				t.Errorf("Priority = %d, want %d", qa.Priority, tc.priority)
			}
			if len(qa.Tags) != len(tc.tags) {
				t.Fatalf("Tags = %v, want %v", qa.Tags, tc.tags)
			}
			for i := range tc.tags {
				if qa.Tags[i] != tc.tags[i] {
					t.Errorf("Tags[%d] = %q, want %q", i, qa.Tags[i], tc.tags[i])
				}
			}
		})
	}
}
