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

// placementByStartHour finds the placement whose occurrence starts at hour h.
func placementByStartHour(t *testing.T, ps []model.Placement, h int) model.Placement {
	t.Helper()
	want := time.Date(2026, 7, 4, 0, 0, 0, 0, time.UTC).Add(time.Duration(h) * time.Hour)
	for _, p := range ps {
		if p.Occ.Start.Equal(want) {
			return p
		}
	}
	t.Fatalf("no placement starting at hour %d", h)
	return model.Placement{}
}

// TestLayoutDayTouchingBoundary closes pass-13 canary escape #1: two occurrences
// where one ends exactly when the next starts do NOT overlap, so lane assignment
// must stay minimal at that touching boundary. A `<=`→`<` mutation of the
// cluster-flush or lane-free comparisons folds a touching occurrence into the
// prior cluster and inflates its Lanes; nothing asserted this before.
func TestLayoutDayTouchingBoundary(t *testing.T) {
	t.Run("standalone touching after an overlap cluster keeps Lanes=1", func(t *testing.T) {
		// A[9-11] and B[10-12] overlap (2 lanes); C[12-13] starts exactly when B
		// ends, so it is a separate, standalone block — Lanes 1, not folded to 2.
		got := model.LayoutDay([]model.Occurrence{occ(9, 11), occ(10, 12), occ(12, 13)})
		if p := placementByStartHour(t, got, 9); p.Lanes != 2 {
			t.Errorf("A[9-11] Lanes = %d, want 2", p.Lanes)
		}
		if p := placementByStartHour(t, got, 10); p.Lanes != 2 {
			t.Errorf("B[10-12] Lanes = %d, want 2", p.Lanes)
		}
		c := placementByStartHour(t, got, 12)
		if c.Lanes != 1 {
			t.Errorf("C[12-13] touches B's end but Lanes = %d, want 1 (standalone, not folded into the cluster)", c.Lanes)
		}
		if c.Lane != 0 {
			t.Errorf("C[12-13] Lane = %d, want 0", c.Lane)
		}
	})

	t.Run("a freed lane is reused at a touching boundary", func(t *testing.T) {
		// A[9-10], B[9-11] overlap → 2 lanes. C[10-11] starts exactly when A ends,
		// so A's lane 0 is free and C must reuse it: peak concurrency stays 2, not 3.
		got := model.LayoutDay([]model.Occurrence{occ(9, 10), occ(9, 11), occ(10, 11)})
		for _, h := range []int{9, 10} {
			if p := placementByStartHour(t, got, h); p.Lanes != 2 {
				t.Errorf("start hour %d: Lanes = %d, want 2 (touching C must reuse A's freed lane, not open a third)", h, p.Lanes)
			}
		}
		if c := placementByStartHour(t, got, 10); c.Lane != 0 {
			t.Errorf("C[10-11] Lane = %d, want 0 (reuse the lane A freed at 10:00)", c.Lane)
		}
	})
}

func TestLayoutDayEmpty(t *testing.T) {
	if got := model.LayoutDay(nil); got != nil {
		t.Errorf("LayoutDay(nil) = %v, want nil", got)
	}
}
