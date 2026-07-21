package model_test

import (
	"testing"
	"time"

	"github.com/littekge/LazyPlanner/internal/model"
)

func date(y int, m time.Month, d int) time.Time {
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}

func TestStartOfWeek(t *testing.T) {
	// 2026-01-01 is a Thursday.
	newYear := date(2026, 1, 1)
	if got := model.StartOfWeek(newYear, true); !got.Equal(date(2025, 12, 29)) {
		t.Errorf("Monday-first start = %s, want 2025-12-29 (Mon)", got.Format("2006-01-02"))
	}
	if got := model.StartOfWeek(newYear, false); !got.Equal(date(2025, 12, 28)) {
		t.Errorf("Sunday-first start = %s, want 2025-12-28 (Sun)", got.Format("2006-01-02"))
	}
}

func TestMonthGrid(t *testing.T) {
	grid := model.MonthGrid(date(2026, 1, 15), true)

	if len(grid) != 6 {
		t.Fatalf("got %d rows, want 6", len(grid))
	}
	for i, row := range grid {
		if len(row) != 7 {
			t.Fatalf("row %d has %d cols, want 7", i, len(row))
		}
	}
	// Monday-first January 2026 starts on 2025-12-29; Jan 1 is the 4th cell.
	if !grid[0][0].Equal(date(2025, 12, 29)) {
		t.Errorf("grid[0][0] = %s, want 2025-12-29", grid[0][0].Format("2006-01-02"))
	}
	if !grid[0][3].Equal(date(2026, 1, 1)) {
		t.Errorf("grid[0][3] = %s, want 2026-01-01", grid[0][3].Format("2006-01-02"))
	}

	// The grid is contiguous and covers every day of January.
	flat := map[string]bool{}
	prev := grid[0][0].AddDate(0, 0, -1)
	for _, row := range grid {
		for _, d := range row {
			if !d.Equal(prev.AddDate(0, 0, 1)) {
				t.Errorf("non-contiguous at %s (prev %s)", d.Format("2006-01-02"), prev.Format("2006-01-02"))
			}
			prev = d
			flat[d.Format("2006-01-02")] = true
		}
	}
	for d := 1; d <= 31; d++ {
		key := date(2026, 1, d).Format("2006-01-02")
		if !flat[key] {
			t.Errorf("January %d missing from grid", d)
		}
	}
}

func TestOccurrencesOn(t *testing.T) {
	loc := time.UTC
	day := date(2026, 7, 4)

	onDay := &model.Event{Summary: "On day", Start: time.Date(2026, 7, 4, 10, 0, 0, 0, loc)}
	spanning := &model.Event{Summary: "Spanning", Start: date(2026, 7, 3), AllDay: true}
	otherDay := &model.Event{Summary: "Other", Start: time.Date(2026, 7, 6, 10, 0, 0, 0, loc)}

	occs := []model.Occurrence{
		{Start: onDay.Start, End: onDay.Start.Add(time.Hour), Event: onDay},
		{Start: spanning.Start, End: date(2026, 7, 5), Event: spanning}, // covers 7/3 and 7/4
		{Start: otherDay.Start, End: otherDay.Start.Add(time.Hour), Event: otherDay},
	}

	got := model.OccurrencesOn(occs, day)
	if len(got) != 2 {
		t.Fatalf("got %d occurrences on 7/4, want 2 (%v)", len(got), got)
	}
}

// TestOccurrenceOverlapsDay guards the multi-day time-grid fan-out: a timed event
// spanning several days overlaps every day it covers, and no others.
func TestOccurrenceOverlapsDay(t *testing.T) {
	loc := time.UTC
	// 11:00 7/23 -> 17:00 7/26.
	occ := model.Occurrence{
		Start: time.Date(2026, 7, 23, 11, 0, 0, 0, loc),
		End:   time.Date(2026, 7, 26, 17, 0, 0, 0, loc),
	}
	cases := []struct {
		day  int
		want bool
	}{
		{22, false}, // day before it starts
		{23, true},  // start day
		{24, true},  // middle
		{25, true},  // middle
		{26, true},  // end day (ends 17:00)
		{27, false}, // day after it ends
	}
	for _, c := range cases {
		if got := occ.OverlapsDay(date(2026, 7, c.day)); got != c.want {
			t.Errorf("OverlapsDay(7/%d) = %v, want %v", c.day, got, c.want)
		}
	}
}
