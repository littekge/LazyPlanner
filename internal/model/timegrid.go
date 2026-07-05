package model

import (
	"sort"
	"time"
)

// Placement locates a timed occurrence within a day's side-by-side overlap
// layout: Lane is its 0-based column index and Lanes is the number of columns
// its overlapping group needs, so a renderer can size and position blocks so
// concurrent events sit next to each other.
type Placement struct {
	Occ   Occurrence
	Lane  int
	Lanes int
}

// LayoutDay arranges timed occurrences into side-by-side lanes. Overlapping
// occurrences get distinct lanes; a connected overlap cluster shares a lane
// count (its peak concurrency) so every block in the cluster is drawn the same
// width. Input need not be sorted; the caller should exclude all-day
// occurrences. Zero-length occurrences are treated as one minute so they still
// take a lane.
func LayoutDay(occs []Occurrence) []Placement {
	if len(occs) == 0 {
		return nil
	}

	sorted := make([]Occurrence, len(occs))
	copy(sorted, occs)
	sort.SliceStable(sorted, func(i, j int) bool {
		if sorted[i].Start.Equal(sorted[j].Start) {
			return layoutEnd(sorted[i]).Before(layoutEnd(sorted[j]))
		}
		return sorted[i].Start.Before(sorted[j].Start)
	})

	placements := make([]Placement, len(sorted))
	clusterStart := 0
	var laneEnds []time.Time
	var clusterEnd time.Time

	flush := func(end int) {
		lanes := len(laneEnds)
		for k := clusterStart; k < end; k++ {
			placements[k].Lanes = lanes
		}
		laneEnds = laneEnds[:0]
		clusterStart = end
		clusterEnd = time.Time{}
	}

	for i, o := range sorted {
		start := o.Start
		end := layoutEnd(o)

		// A gap (this occurrence starts at/after everything so far) ends the
		// current cluster.
		if i > clusterStart && !start.Before(clusterEnd) {
			flush(i)
		}

		lane := -1
		for l, le := range laneEnds {
			if !le.After(start) { // lane free: its end <= this start
				lane = l
				laneEnds[l] = end
				break
			}
		}
		if lane == -1 {
			lane = len(laneEnds)
			laneEnds = append(laneEnds, end)
		}

		placements[i] = Placement{Occ: o, Lane: lane}
		if end.After(clusterEnd) {
			clusterEnd = end
		}
	}
	flush(len(sorted))
	return placements
}

func layoutEnd(o Occurrence) time.Time {
	if o.End.After(o.Start) {
		return o.End
	}
	return o.Start.Add(time.Minute)
}
