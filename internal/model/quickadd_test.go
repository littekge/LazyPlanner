package model

import (
	"testing"
	"time"
)

// TestParsePriority pins the numeric priority boundary directly against
// parsePriority: the accepted range is 1-9 inclusive (iCal PRIORITY's high end
// of "in use" values), 0 and 10 are out of range and must not parse. This
// closes a pass-19 canary escape: a mutation flipping the upper bound's
// n<=9 to n<=8 slipped through undetected because the existing "!high"/"!1"
// coverage never exercised !9, the boundary a n<=8 mutant would break.
func TestParsePriority(t *testing.T) {
	tests := []struct {
		in     string
		wantN  int
		wantOK bool
	}{
		{"1", 1, true},
		{"9", 9, true}, // upper edge: would fail under an n<=9 -> n<=8 mutation
		{"5", 5, true},
		{"0", 0, false},  // below range
		{"10", 0, false}, // above range
		{"-1", 0, false},
		{"high", 1, true},
		{"h", 1, true},
		{"med", 5, true},
		{"medium", 5, true},
		{"m", 5, true},
		{"low", 9, true},
		{"l", 9, true},
		{"HIGH", 1, true}, // case-insensitive alias
		{"", 0, false},
		{"abc", 0, false},
	}
	for _, tc := range tests {
		t.Run(tc.in, func(t *testing.T) {
			n, ok := parsePriority(tc.in)
			if ok != tc.wantOK || (ok && n != tc.wantN) {
				t.Errorf("parsePriority(%q) = (%d, %v), want (%d, %v)", tc.in, n, ok, tc.wantN, tc.wantOK)
			}
		})
	}
}

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
		date     time.Time
		hasTime  bool
		hour     int
		minute   int
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
			date:    today,
		},
		{
			name:    "tomorrow",
			input:   "Ship it tomorrow",
			title:   "Ship it",
			hasDate: true,
			date:    today.AddDate(0, 0, 1),
		},
		{
			name:    "weekday rolls to same-or-future",
			input:   "Gym wed",
			title:   "Gym",
			hasDate: true,
			date:    time.Date(2026, 7, 8, 0, 0, 0, 0, loc), // Wed after Sun Jul 5
		},
		{
			name:    "month name and day",
			input:   "Trip jul 20",
			title:   "Trip",
			hasDate: true,
			date:    time.Date(2026, 7, 20, 0, 0, 0, 0, loc),
		},
		{
			name:    "past month day rolls to next year",
			input:   "Taxes apr 15",
			title:   "Taxes",
			hasDate: true,
			date:    time.Date(2027, 4, 15, 0, 0, 0, 0, loc),
		},
		{
			name:    "slashed date with year",
			input:   "Review 12/31/2026",
			title:   "Review",
			hasDate: true,
			date:    time.Date(2026, 12, 31, 0, 0, 0, 0, loc),
		},
		{
			name:    "iso date",
			input:   "Deadline 2026-08-01",
			title:   "Deadline",
			hasDate: true,
			date:    time.Date(2026, 8, 1, 0, 0, 0, 0, loc),
		},
		{
			name:    "bare time has no date",
			input:   "Call 3pm",
			title:   "Call",
			hasTime: true,
			hour:    15,
		},
		{
			name:    "date and time together",
			input:   "Dentist jul 20 9:30am",
			title:   "Dentist",
			hasDate: true,
			date:    time.Date(2026, 7, 20, 0, 0, 0, 0, loc),
			hasTime: true,
			hour:    9,
			minute:  30,
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
			date:     time.Date(2026, 8, 1, 0, 0, 0, 0, loc),
			hasTime:  true,
			hour:     17,
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
			if tc.hasDate && !qa.Date.Equal(tc.date) {
				t.Errorf("Date = %s, want %s", qa.Date, tc.date)
			}
			if tc.hasTime && (qa.Hour != tc.hour || qa.Minute != tc.minute) {
				t.Errorf("time = %d:%02d, want %d:%02d", qa.Hour, qa.Minute, tc.hour, tc.minute)
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
