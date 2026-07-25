package ui

import (
	"fmt"
	"math/rand"
	"runtime"
	"testing"
	"time"

	"github.com/littekge/LazyPlanner/internal/model"
)

// overlappingDayGrid builds a drilled time-grid whose selected day holds n timed
// events that all overlap each other (each spans the whole day), the shape a
// bounded-but-pathological recurrence expansion produces. Both the draw data
// (timed occurrences) and the drill list (items) are populated, since navCells
// joins the two.
func overlappingDayGrid(n int) *timeGridView {
	day := time.Date(2026, 3, 4, 0, 0, 0, 0, time.Local)
	occs := make([]model.Occurrence, n)
	items := make([]model.AgendaItem, n)
	for i := range occs {
		start := day.Add(time.Duration(i) * time.Second)
		ev := &model.Event{Summary: fmt.Sprintf("E%d", i), Start: start}
		occs[i] = model.Occurrence{Start: start, End: start.Add(24 * time.Hour), Event: ev}
		items[i] = model.AgendaItem{Start: start, End: occs[i].End, Title: ev.Summary, Event: ev}
	}

	tg := newTimeGridView()
	tg.setData([]time.Time{day}, map[string][]model.Occurrence{dayKey(day): occs}, nil, day, day)
	tg.items = map[string][]model.AgendaItem{dayKey(day): items}
	tg.eventMode = true
	tg.eventIndex = 0
	return tg
}

// TestNavCellsIsNotQuadratic is the repro for the keypress half of the pass-23
// HIGH: model.LayoutDay is now a sweep line, but navCells re-scanned the whole
// placements slice once per item to find that item's own placement, so a single
// h/j/k/l keypress still cost O(items x placements) on the same pathological
// day. spatialTarget runs on every navigation keypress, so this hangs the UI on
// each arrow press even though the draw path is fast.
func TestNavCellsIsNotQuadratic(t *testing.T) {
	const (
		n      = 40000
		budget = 500 * time.Millisecond
	)

	tg := overlappingDayGrid(n)
	done := make(chan time.Duration, 1)
	go func() {
		start := time.Now()
		tg.spatialTarget(navDown)
		done <- time.Since(start)
	}()
	select {
	case d := <-done:
		t.Logf("spatialTarget(navDown) over %d overlapping items took %v", n, d)
		if d > budget {
			t.Fatalf("one navigation keypress over %d items took %v, want < %v — the item->placement join is superlinear", n, d, budget)
		}
	case <-time.After(180 * time.Second):
		t.Fatalf("spatialTarget over %d items did not finish in 180s", n)
	}
}

// TestNavCellsGrowthIsSubQuadratic asserts the shape of the curve rather than a
// wall-clock budget, so it stays meaningful on a loaded machine: a per-item scan
// over every placement costs ~16x when n quadruples, an indexed lookup ~4-7x.
// Interleaved best-of-N damps scheduler noise.
func TestNavCellsGrowthIsSubQuadratic(t *testing.T) {
	const (
		small        = 10000     // large enough that one measurement is milliseconds, not scheduler noise
		large        = 4 * small // quadratic => ~16x, indexed lookup => ~3-7x
		maxGrowth    = 11.0      // observed 5.5-7.3x; the per-item scan measured ~16x
		measureTries = 5
	)

	// Both grids are built up front and the two sizes are timed alternately, so a
	// load spike from a concurrent build lands on both measurements rather than
	// inflating the ratio; best-of-N then keeps the least-disturbed round.
	tgSmall, tgLarge := overlappingDayGrid(small), overlappingDayGrid(large)
	runtime.GC() // don't charge this test for the heap the rest of the suite left behind

	timeOnce := func(tg *timeGridView) time.Duration {
		start := time.Now()
		tg.spatialTarget(navDown)
		return time.Since(start)
	}
	ds, dl := time.Duration(1<<62-1), time.Duration(1<<62-1)
	for i := 0; i < measureTries; i++ {
		if d := timeOnce(tgSmall); d < ds {
			ds = d
		}
		if d := timeOnce(tgLarge); d < dl {
			dl = d
		}
	}
	growth := float64(dl) / float64(ds)
	t.Logf("spatialTarget: n=%d took %v, n=%d took %v (growth %.1fx for 4x input)", small, ds, large, dl, growth)
	if growth > maxGrowth {
		t.Fatalf("quadrupling the day's item count multiplied one keypress by %.1fx (want <= %.1fx) — the item->placement join is superlinear",
			growth, maxGrowth)
	}
}

// navCellsLinearReference is the pre-index navCells: for every timed event item
// it scans the whole placements slice, taking the first placement whose Event
// pointer and Start match. It is quadratic, which is why it was replaced — but
// it defines the cursor positions users actually navigate, so it stays here as
// the reference the indexed implementation is diffed against.
func navCellsLinearReference(tg *timeGridView) []navCell {
	items := tg.daySelectables()
	cells := make([]navCell, len(items))
	placements := model.LayoutDay(tg.timed[dayKey(tg.selected)])
	band := 0
	for i, it := range items {
		switch {
		case isAllDayItem(it):
			cells[i] = navCell{kind: cellBand, lane: band}
			band++
		case it.Todo != nil:
			cells[i] = navCell{kind: cellTask, start: it.Start, end: it.Start, lane: 0}
		default:
			lane, end := 0, it.Start
			for _, p := range placements {
				if p.Occ.Event == it.Event && p.Occ.Start.Equal(it.Start) {
					lane, end = p.Lane, p.Occ.End
					break
				}
			}
			cells[i] = navCell{kind: cellEvent, start: it.Start, end: end, lane: lane}
		}
	}
	return cells
}

// randomDayGrid builds a messy day for the equivalence diff: all-day events and
// all-day-due tasks (the band), timed due tasks, timed events, events sharing a
// start instant, one Event pointer occurring twice at different starts, duplicate
// occurrences at the same (Event, Start) — which exercise the "first placement
// wins" rule — and drill items whose event has no placement at all.
func randomDayGrid(rng *rand.Rand, n int) *timeGridView {
	day := time.Date(2026, 3, 4, 0, 0, 0, 0, time.Local)
	var occs, allDay []model.Occurrence
	var items []model.AgendaItem
	var dues []*model.Todo

	var repeated *model.Event
	for i := 0; i < n; i++ {
		start := day.Add(time.Duration(rng.Intn(24*60)) * time.Minute)
		switch rng.Intn(6) {
		case 0: // all-day event
			ev := &model.Event{Summary: fmt.Sprintf("A%d", i), AllDay: true, Start: day}
			allDay = append(allDay, model.Occurrence{Start: day, End: day.AddDate(0, 0, 1), Event: ev})
			items = append(items, model.AgendaItem{Start: day, AllDay: true, Title: ev.Summary, Event: ev})
		case 1: // all-day-due task
			td := &model.Todo{UID: fmt.Sprintf("t%d", i), Summary: "T", HasDue: true, DueAllDay: true, Due: day}
			dues = append(dues, td)
			items = append(items, model.AgendaItem{Start: day, AllDay: true, Title: td.Summary, Todo: td})
		case 2: // timed due task
			td := &model.Todo{UID: fmt.Sprintf("t%d", i), Summary: "T", HasDue: true, Due: start}
			dues = append(dues, td)
			items = append(items, model.AgendaItem{Start: start, Title: td.Summary, Todo: td})
		case 3: // a drill item whose event never made it into the layout
			ev := &model.Event{Summary: fmt.Sprintf("G%d", i), Start: start}
			items = append(items, model.AgendaItem{Start: start, Title: ev.Summary, Event: ev})
		case 4: // reuse an earlier event pointer at a different start
			if repeated == nil {
				repeated = &model.Event{Summary: "R", Start: start}
			}
			occs = append(occs, model.Occurrence{Start: start, End: start.Add(time.Duration(rng.Intn(180)) * time.Minute), Event: repeated})
			items = append(items, model.AgendaItem{Start: start, Title: repeated.Summary, Event: repeated})
		default: // plain timed event, sometimes duplicated at the same instant
			ev := &model.Event{Summary: fmt.Sprintf("E%d", i), Start: start}
			end := start.Add(time.Duration(rng.Intn(180)) * time.Minute)
			occs = append(occs, model.Occurrence{Start: start, End: end, Event: ev})
			if rng.Intn(4) == 0 {
				occs = append(occs, model.Occurrence{Start: start, End: end.Add(time.Hour), Event: ev})
			}
			items = append(items, model.AgendaItem{Start: start, End: end, Title: ev.Summary, Event: ev})
		}
	}

	tg := newTimeGridView()
	tg.setData([]time.Time{day}, map[string][]model.Occurrence{dayKey(day): occs},
		map[string][]model.Occurrence{dayKey(day): allDay}, day, day)
	tg.items = map[string][]model.AgendaItem{dayKey(day): items}
	tg.dueTasks = map[string][]*model.Todo{dayKey(day): dues}
	tg.eventMode = true
	return tg
}

// TestNavCellsMatchesLinearScanReference is the behavioural-identity guard for
// the indexed item->placement join: navCells drives the user's cursor, so a
// faster join that lands on a different lane, end or cell kind is a regression,
// not a fix. Every cell — and every spatialTarget answer from every cursor
// position in all four directions — must equal the linear-scan reference.
func TestNavCellsMatchesLinearScanReference(t *testing.T) {
	rng := rand.New(rand.NewSource(20260725))
	for _, n := range []int{0, 1, 2, 5, 17, 64, 200} {
		for trial := 0; trial < 10; trial++ {
			tg := randomDayGrid(rng, n)
			got := tg.navCells()
			want := navCellsLinearReference(tg)
			if len(got) != len(want) {
				t.Fatalf("n=%d trial=%d: got %d cells, want %d", n, trial, len(got), len(want))
			}
			for i := range want {
				if got[i] != want[i] {
					t.Fatalf("n=%d trial=%d cell %d: got %+v, want %+v", n, trial, i, got[i], want[i])
				}
			}
			for idx := 0; idx < len(want); idx++ {
				tg.eventIndex = idx
				for _, dir := range []int{navUp, navDown, navLeft, navRight} {
					gotT := tg.spatialTarget(dir)
					wantT := spatialTargetFrom(tg, want, dir)
					if gotT != wantT {
						t.Fatalf("n=%d trial=%d from index %d dir %d: got target %d, want %d",
							n, trial, idx, dir, gotT, wantT)
					}
				}
			}
		}
	}
}

// spatialTargetFrom runs the real spatial-navigation logic against a caller-
// supplied cell list, so the reference cells can be fed through the same
// direction rules the production path uses.
func spatialTargetFrom(tg *timeGridView, cells []navCell, dir int) int {
	if tg.eventIndex < 0 || tg.eventIndex >= len(cells) {
		return -1
	}
	cur := cells[tg.eventIndex]
	switch dir {
	case navLeft, navRight:
		step := 1
		if dir == navLeft {
			step = -1
		}
		for i, c := range cells {
			switch cur.kind {
			case cellBand:
				if c.kind == cellBand && c.lane == cur.lane+step {
					return i
				}
			case cellEvent:
				if c.kind == cellEvent && c.lane == cur.lane+step && cur.overlaps(c) {
					return i
				}
			}
		}
		return -1
	case navDown:
		if cur.kind == cellBand {
			return tg.edgeTimed(cells, cur.lane, true)
		}
		return tg.nearestLevel(cells, cur, true)
	case navUp:
		if cur.kind == cellBand {
			return -1
		}
		if t := tg.nearestLevel(cells, cur, false); t >= 0 {
			return t
		}
		return tg.bandNearest(cells, cur.lane)
	}
	return -1
}
