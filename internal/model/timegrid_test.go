package model_test

import (
	"testing"
	"time"

	"github.com/littekge/LazyPlanner/internal/model"
)

func occ(startHour, endHour int) model.Occurrence {
	day := time.Date(2026, 7, 4, 0, 0, 0, 0, time.UTC)
	return model.Occurrence{
		Start: day.Add(time.Duration(startHour) * time.Hour),
		End:   day.Add(time.Duration(endHour) * time.Hour),
	}
}

func TestLayoutDay(t *testing.T) {
	tests := []struct {
		name  string
		occs  []model.Occurrence
		lanes []int // expected Lane per input (after LayoutDay's own sort, matched by start)
		total int   // expected Lanes value on every placement
	}{
		{
			name:  "non-overlapping share lane 0",
			occs:  []model.Occurrence{occ(9, 10), occ(11, 12)},
			total: 1,
		},
		{
			name:  "two overlapping split into two lanes",
			occs:  []model.Occurrence{occ(9, 11), occ(10, 12)},
			total: 2,
		},
		{
			name:  "three-way peak concurrency of two",
			occs:  []model.Occurrence{occ(9, 10), occ(9, 11), occ(10, 12)},
			total: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := model.LayoutDay(tt.occs)
			if len(got) != len(tt.occs) {
				t.Fatalf("got %d placements, want %d", len(got), len(tt.occs))
			}
			for i, p := range got {
				if p.Lanes != tt.total {
					t.Errorf("placement %d Lanes = %d, want %d", i, p.Lanes, tt.total)
				}
				if p.Lane < 0 || p.Lane >= p.Lanes {
					t.Errorf("placement %d Lane = %d out of range [0,%d)", i, p.Lane, p.Lanes)
				}
			}
			// No two concurrent occurrences may share a lane.
			for i := 0; i < len(got); i++ {
				for j := i + 1; j < len(got); j++ {
					a, b := got[i], got[j]
					overlap := a.Occ.Start.Before(b.Occ.End) && b.Occ.Start.Before(a.Occ.End)
					if overlap && a.Lane == b.Lane {
						t.Errorf("overlapping occurrences share lane %d", a.Lane)
					}
				}
			}
		})
	}
}

func TestLayoutDayEmpty(t *testing.T) {
	if got := model.LayoutDay(nil); got != nil {
		t.Errorf("LayoutDay(nil) = %v, want nil", got)
	}
}
