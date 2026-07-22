package model

import (
	"testing"
	"time"
)

// TestParseQuickAddTimeRange covers the v1.2.0 time-range token: one
// start-end token where at least one half carries a colon or am/pm, a
// right-side am/pm distributing to a bare left half, and two bare numbers
// never reading as a time.
func TestParseQuickAddTimeRange(t *testing.T) {
	loc := time.UTC
	now := time.Date(2026, 7, 5, 9, 0, 0, 0, loc)

	tests := []struct {
		name               string
		input              string
		title              string
		hasTime            bool
		hour, minute       int
		hasEnd             bool
		endHour, endMinute int
	}{
		{
			name:    "am/pm distributes to bare left half",
			input:   "Meeting 5-6pm",
			title:   "Meeting",
			hasTime: true, hour: 17, minute: 0,
			hasEnd: true, endHour: 18, endMinute: 0,
		},
		{
			name:    "both halves explicit",
			input:   "Call 5pm-6:30pm",
			title:   "Call",
			hasTime: true, hour: 17, minute: 0,
			hasEnd: true, endHour: 18, endMinute: 30,
		},
		{
			name:    "24-hour colon halves",
			input:   "Sync 14:00-15:30",
			title:   "Sync",
			hasTime: true, hour: 14, minute: 0,
			hasEnd: true, endHour: 15, endMinute: 30,
		},
		{
			name:    "crosses midnight keeps raw end fields",
			input:   "Party 11pm-1am",
			title:   "Party",
			hasTime: true, hour: 23, minute: 0,
			hasEnd: true, endHour: 1, endMinute: 0,
		},
		{
			name:    "12am to 12pm",
			input:   "x 12:00am-12:00pm",
			title:   "x",
			hasTime: true, hour: 0, minute: 0,
			hasEnd: true, endHour: 12, endMinute: 0,
		},
		{
			name:  "two bare numbers are not a time",
			input: "Lunch 3-4",
			title: "Lunch 3-4",
		},
		{
			name:    "single time still has no end",
			input:   "Call 3pm",
			title:   "Call",
			hasTime: true, hour: 15, minute: 0,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			qa := ParseQuickAdd(tc.input, now, loc)
			if qa.Title != tc.title {
				t.Errorf("Title = %q, want %q", qa.Title, tc.title)
			}
			if qa.HasTime != tc.hasTime {
				t.Fatalf("HasTime = %v, want %v", qa.HasTime, tc.hasTime)
			}
			if tc.hasTime && (qa.Hour != tc.hour || qa.Minute != tc.minute) {
				t.Errorf("start = %d:%02d, want %d:%02d", qa.Hour, qa.Minute, tc.hour, tc.minute)
			}
			if qa.HasEnd != tc.hasEnd {
				t.Fatalf("HasEnd = %v, want %v", qa.HasEnd, tc.hasEnd)
			}
			if tc.hasEnd && (qa.EndHour != tc.endHour || qa.EndMinute != tc.endMinute) {
				t.Errorf("end = %d:%02d, want %d:%02d", qa.EndHour, qa.EndMinute, tc.endHour, tc.endMinute)
			}
		})
	}
}

// TestParseQuickAddTimeRangeDoesNotEatISODate guards that an ISO date (two
// dashes) is not mistaken for a time range (one dash between two halves).
func TestParseQuickAddTimeRangeDoesNotEatISODate(t *testing.T) {
	loc := time.UTC
	now := time.Date(2026, 7, 5, 9, 0, 0, 0, loc)
	qa := ParseQuickAdd("review 2026-08-01", now, loc)
	if qa.HasEnd {
		t.Errorf("HasEnd = true, want false (an ISO date is not a range)")
	}
	if !qa.HasDate {
		t.Errorf("HasDate = false, want true (ISO date should still parse)")
	}
}

// TestQuickAddEndAtRollover checks the end-datetime helper: the end lands on the
// start's day, rolling to the next day when it is at or before the start.
func TestQuickAddEndAtRollover(t *testing.T) {
	loc := time.UTC

	tests := []struct {
		name  string
		start time.Time
		endH  int
		endM  int
		want  time.Time
	}{
		{
			name:  "same day when end after start",
			start: time.Date(2026, 7, 5, 17, 0, 0, 0, loc),
			endH:  18, endM: 0,
			want: time.Date(2026, 7, 5, 18, 0, 0, 0, loc),
		},
		{
			name:  "rolls to next day when end before start",
			start: time.Date(2026, 7, 5, 23, 0, 0, 0, loc),
			endH:  1, endM: 0,
			want: time.Date(2026, 7, 6, 1, 0, 0, 0, loc),
		},
		{
			name:  "rolls to next day when end equals start",
			start: time.Date(2026, 7, 5, 17, 0, 0, 0, loc),
			endH:  17, endM: 0,
			want: time.Date(2026, 7, 6, 17, 0, 0, 0, loc),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			qa := QuickAdd{HasTime: true, Hour: tc.start.Hour(), Minute: tc.start.Minute(), HasEnd: true, EndHour: tc.endH, EndMinute: tc.endM}
			got := qa.EndAt(tc.start)
			if !got.Equal(tc.want) {
				t.Errorf("EndAt = %s, want %s", got.Format("2006-01-02 15:04"), tc.want.Format("2006-01-02 15:04"))
			}
		})
	}
}
