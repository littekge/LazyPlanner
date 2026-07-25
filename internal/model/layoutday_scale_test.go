package model_test

import (
	"fmt"
	"math/rand"
	"sort"
	"testing"
	"time"

	"github.com/littekge/LazyPlanner/internal/model"
)

// overlappingOccs builds n occurrences that all overlap each other (every one
// spans the whole day), which is exactly what a bounded pathological rule
// (FREQ=SECONDLY;COUNT=10000 with DURATION:P1D) yields on a single day.
func overlappingOccs(n int) []model.Occurrence {
	day := time.Date(2026, 3, 4, 0, 0, 0, 0, time.UTC)
	occs := make([]model.Occurrence, n)
	for i := range occs {
		start := day.Add(time.Duration(i) * time.Second)
		occs[i] = model.Occurrence{Start: start, End: start.Add(24 * time.Hour)}
	}
	return occs
}

// TestPathologicalRuleGuardDoesNotCoverLayout is the end-to-end shape of the
// repro: three ordinary-looking .ics resources whose expansion the aggregate
// StepBudget happily allows (the pathological-rule guard "succeeds", cheaply),
// yet whose one-day layout — what every time-grid Draw and every navigation
// keypress runs — takes seconds.
func TestPathologicalRuleGuardDoesNotCoverLayout(t *testing.T) {
	const resources = 3
	from := time.Date(2026, 3, 4, 0, 0, 0, 0, time.UTC)
	to := from.Add(24 * time.Hour)

	budget := model.NewStepBudget()
	var occs []model.Occurrence
	expandStart := time.Now()
	for i := 0; i < resources; i++ {
		ics := "BEGIN:VCALENDAR\r\nVERSION:2.0\r\nPRODID:-//x//x//EN\r\n" +
			"BEGIN:VEVENT\r\nUID:flood" + fmt.Sprint(i) + "\r\nDTSTART:20260304T000000Z\r\n" +
			"DURATION:P1D\r\nRRULE:FREQ=SECONDLY;COUNT=10000\r\nSUMMARY:Flood\r\n" +
			"END:VEVENT\r\nEND:VCALENDAR\r\n"
		p, err := model.Decode([]byte(ics), time.UTC)
		if err != nil {
			t.Fatalf("decode: %v", err)
		}
		got, err := p.EventOccurrencesBudgeted(from, to, budget)
		if err != nil {
			t.Fatalf("expand: %v", err)
		}
		occs = append(occs, got...)
	}
	expandTook := time.Since(expandStart)
	t.Logf("expansion of %d resources -> %d occurrences took %v (guard reports success)",
		resources, len(occs), expandTook)

	layoutStart := time.Now()
	model.LayoutDay(occs)
	layoutTook := time.Since(layoutStart)
	t.Logf("LayoutDay(%d occurrences) took %v", len(occs), layoutTook)

	if layoutTook > 500*time.Millisecond {
		t.Fatalf("layout of one day took %v (expansion only %v) — a single Draw blocks the UI",
			layoutTook, expandTook)
	}
}

// TestLayoutDayIsNotQuadratic is the repro for the pass-23 HIGH: LayoutDay's
// per-occurrence linear scan over laneEnds makes layout O(n^2) when every
// occurrence overlaps every other. Recurrence expansion is bounded, so the
// pathological-rule guard reports success while a single time-grid Draw (and
// every keypress that redraws) takes seconds to minutes.
func TestLayoutDayIsNotQuadratic(t *testing.T) {
	const budget = 500 * time.Millisecond

	for _, n := range []int{20000, 100000} {
		t.Run(fmt.Sprintf("n=%d", n), func(t *testing.T) {
			occs := overlappingOccs(n)
			done := make(chan time.Duration, 1)
			go func() {
				start := time.Now()
				p := model.LayoutDay(occs)
				if len(p) != n {
					panic("bad placement count")
				}
				done <- time.Since(start)
			}()
			select {
			case d := <-done:
				t.Logf("LayoutDay(n=%d) took %v", n, d)
				if d > budget {
					t.Fatalf("LayoutDay(n=%d) took %v, want < %v — lane packing is superlinear", n, d, budget)
				}
			case <-time.After(120 * time.Second):
				t.Fatalf("LayoutDay(n=%d) did not finish in 120s", n)
			}
		})
	}
}

// naiveLayoutDay is the pre-sweep-line first-fit lane packing LayoutDay used to
// do: for each occurrence, scan every lane in index order and take the first one
// whose occupant has ended. It is quadratic, which is why it was replaced — but
// it defines the lane numbering users actually see, so it stays here as the
// reference the fast implementation is diffed against.
func naiveLayoutDay(occs []model.Occurrence) []model.Placement {
	if len(occs) == 0 {
		return nil
	}
	end := func(o model.Occurrence) time.Time {
		if o.End.After(o.Start) {
			return o.End
		}
		return o.Start.Add(time.Minute)
	}

	sorted := make([]model.Occurrence, len(occs))
	copy(sorted, occs)
	sort.SliceStable(sorted, func(i, j int) bool {
		if sorted[i].Start.Equal(sorted[j].Start) {
			return end(sorted[i]).Before(end(sorted[j]))
		}
		return sorted[i].Start.Before(sorted[j].Start)
	})

	placements := make([]model.Placement, len(sorted))
	clusterStart := 0
	var laneEnds []time.Time
	var clusterEnd time.Time

	flush := func(upto int) {
		lanes := len(laneEnds)
		for k := clusterStart; k < upto; k++ {
			placements[k].Lanes = lanes
		}
		laneEnds = laneEnds[:0]
		clusterStart = upto
		clusterEnd = time.Time{}
	}

	for i, o := range sorted {
		start, e := o.Start, end(o)
		if i > clusterStart && !start.Before(clusterEnd) {
			flush(i)
		}
		lane := -1
		for l, le := range laneEnds {
			if !le.After(start) {
				lane = l
				laneEnds[l] = e
				break
			}
		}
		if lane == -1 {
			lane = len(laneEnds)
			laneEnds = append(laneEnds, e)
		}
		placements[i] = model.Placement{Occ: o, Lane: lane}
		if e.After(clusterEnd) {
			clusterEnd = e
		}
	}
	flush(len(sorted))
	return placements
}

// randomOccs builds a day of randomly placed timed occurrences, including
// zero-length ones and exact touching boundaries, for the equivalence diff.
func randomOccs(rng *rand.Rand, n int) []model.Occurrence {
	day := time.Date(2026, 3, 4, 0, 0, 0, 0, time.UTC)
	occs := make([]model.Occurrence, n)
	for i := range occs {
		startMin := rng.Intn(24 * 60)
		durMin := rng.Intn(180) // 0 exercises the zero-length "one minute" rule
		start := day.Add(time.Duration(startMin) * time.Minute)
		occs[i] = model.Occurrence{Start: start, End: start.Add(time.Duration(durMin) * time.Minute)}
	}
	return occs
}

// TestLayoutDayMatchesFirstFitReference is the visual-identity guard for the
// pass-23 sweep-line rewrite: LayoutDay is rendering-visible, so a faster
// packing that assigns different lane numbers is a regression, not a fix. Every
// placement — order, occurrence, Lane and Lanes — must equal the naive first-fit
// reference on randomized days full of ties, touching boundaries and
// zero-length occurrences.
func TestLayoutDayMatchesFirstFitReference(t *testing.T) {
	rng := rand.New(rand.NewSource(20260723))
	for _, n := range []int{0, 1, 2, 5, 17, 64, 250, 1000} {
		for trial := 0; trial < 20; trial++ {
			occs := randomOccs(rng, n)
			got := model.LayoutDay(occs)
			want := naiveLayoutDay(occs)
			if len(got) != len(want) {
				t.Fatalf("n=%d trial=%d: got %d placements, want %d", n, trial, len(got), len(want))
			}
			for i := range want {
				if !got[i].Occ.Start.Equal(want[i].Occ.Start) || !got[i].Occ.End.Equal(want[i].Occ.End) {
					t.Fatalf("n=%d trial=%d placement %d: ordering differs: got %v-%v, want %v-%v",
						n, trial, i, got[i].Occ.Start, got[i].Occ.End, want[i].Occ.Start, want[i].Occ.End)
				}
				if got[i].Lane != want[i].Lane || got[i].Lanes != want[i].Lanes {
					t.Fatalf("n=%d trial=%d placement %d (%v-%v): got Lane=%d Lanes=%d, want Lane=%d Lanes=%d",
						n, trial, i, want[i].Occ.Start, want[i].Occ.End,
						got[i].Lane, got[i].Lanes, want[i].Lane, want[i].Lanes)
				}
			}
		}
	}
}

// TestLayoutDayGrowthIsSubQuadratic asserts the shape of the curve rather than a
// wall-clock budget, so it stays meaningful on a loaded machine: quadratic
// packing costs ~16x when n quadruples, a sweep line ~4-5x. Best-of-3 damps
// scheduler noise.
func TestLayoutDayGrowthIsSubQuadratic(t *testing.T) {
	const (
		small        = 25000
		large        = 4 * small // quadratic => ~16x, sweep line => ~5x
		maxGrowth    = 11.0      // observed 4.6-7.4x; quadratic would be ~16x
		measureTries = 3
	)

	best := func(n int) time.Duration {
		occs := overlappingOccs(n)
		fastest := time.Duration(1<<62 - 1)
		for i := 0; i < measureTries; i++ {
			start := time.Now()
			if p := model.LayoutDay(occs); len(p) != n {
				t.Fatalf("LayoutDay(n=%d) returned %d placements", n, len(p))
			}
			if d := time.Since(start); d < fastest {
				fastest = d
			}
		}
		return fastest
	}

	ds, dl := best(small), best(large)
	growth := float64(dl) / float64(ds)
	t.Logf("LayoutDay: n=%d took %v, n=%d took %v (growth %.1fx for 4x input)", small, ds, large, dl, growth)
	if growth > maxGrowth {
		t.Fatalf("quadrupling n multiplied layout cost by %.1fx (want <= %.1fx) — lane packing is superlinear",
			growth, maxGrowth)
	}
}
