package model

import (
	"container/heap"
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
	laneCount := 0
	// Sweep line: occupied lanes wait in a min-heap keyed by release time and
	// freed indices in a min-heap keyed by index. Scanning every lane per
	// occurrence instead (the obvious first-fit) is O(n^2), and LayoutDay runs
	// on the time-grid Draw path and on every navigation keypress — a day
	// holding a bounded-but-large expansion would freeze the UI.
	var busy busyLanes
	var free freeLanes
	var clusterEnd time.Time

	flush := func(end int) {
		for k := clusterStart; k < end; k++ {
			placements[k].Lanes = laneCount
		}
		laneCount = 0
		busy = busy[:0]
		free = free[:0]
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

		for len(busy) > 0 && !busy[0].end.After(start) { // lane free: its end <= this start
			heap.Push(&free, heap.Pop(&busy).(laneRelease).lane)
		}

		var lane int
		if len(free) > 0 {
			// Lowest free index — the same lane the linear first-fit scan picked.
			lane = heap.Pop(&free).(int)
		} else {
			lane = laneCount
			laneCount++
		}
		heap.Push(&busy, laneRelease{end: end, lane: lane})

		placements[i] = Placement{Occ: o, Lane: lane}
		if end.After(clusterEnd) {
			clusterEnd = end
		}
	}
	flush(len(sorted))
	return placements
}

// laneRelease is an occupied lane and the time it frees up.
type laneRelease struct {
	end  time.Time
	lane int
}

// busyLanes is a min-heap of occupied lanes ordered by release time.
type busyLanes []laneRelease

func (h busyLanes) Len() int           { return len(h) }
func (h busyLanes) Less(i, j int) bool { return h[i].end.Before(h[j].end) }
func (h busyLanes) Swap(i, j int)      { h[i], h[j] = h[j], h[i] }

func (h *busyLanes) Push(x any) { *h = append(*h, x.(laneRelease)) }

func (h *busyLanes) Pop() any {
	old := *h
	n := len(old)
	v := old[n-1]
	*h = old[:n-1]
	return v
}

// freeLanes is a min-heap of released lane indices. Ordering by index is what
// keeps the packing byte-identical to the original first-fit scan: of all the
// lanes free at this start, the lowest-numbered one wins.
type freeLanes []int

func (h freeLanes) Len() int           { return len(h) }
func (h freeLanes) Less(i, j int) bool { return h[i] < h[j] }
func (h freeLanes) Swap(i, j int)      { h[i], h[j] = h[j], h[i] }

func (h *freeLanes) Push(x any) { *h = append(*h, x.(int)) }

func (h *freeLanes) Pop() any {
	old := *h
	n := len(old)
	v := old[n-1]
	*h = old[:n-1]
	return v
}

func layoutEnd(o Occurrence) time.Time {
	if o.End.After(o.Start) {
		return o.End
	}
	return o.Start.Add(time.Minute)
}
