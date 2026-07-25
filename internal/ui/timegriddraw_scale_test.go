package ui

import (
	"fmt"
	"math/rand"
	"runtime"
	"testing"
	"time"

	"github.com/gdamore/tcell/v2"

	"github.com/littekge/LazyPlanner/internal/model"
)

// selectRangeDayGrid builds a drilled time-grid whose selected day holds n timed
// events that all overlap each other (each spans the whole day) plus a timed due
// task, the shape a busy day — or a bounded-but-pathological recurrence
// expansion — produces. Every item carries a distinct UID so the drill list is
// searched, not short-circuited on the first entry. withRange opens a SELECT
// range spanning the whole day, which is what arms the draw path's in-range test.
func selectRangeDayGrid(n int, withRange bool) *timeGridView {
	day := time.Date(2026, 3, 4, 0, 0, 0, 0, time.Local)
	occs := make([]model.Occurrence, n)
	items := make([]model.AgendaItem, 0, n+1)
	for i := range occs {
		start := day.Add(time.Duration(i) * time.Second)
		ev := &model.Event{UID: fmt.Sprintf("e%d", i), Summary: fmt.Sprintf("E%d", i), Start: start}
		occs[i] = model.Occurrence{Start: start, End: start.Add(24 * time.Hour), Event: ev}
		items = append(items, model.AgendaItem{Start: start, End: occs[i].End, Title: ev.Summary, Event: ev})
	}
	due := day.Add(12 * time.Hour)
	td := &model.Todo{UID: "task", Summary: "T", HasDue: true, Due: due}
	items = append(items, model.AgendaItem{Start: due, Title: td.Summary, Todo: td})

	tg := newTimeGridView()
	tg.setData([]time.Time{day}, map[string][]model.Occurrence{dayKey(day): occs}, nil, day, day)
	tg.items = map[string][]model.AgendaItem{dayKey(day): items}
	tg.dueTasks = map[string][]*model.Todo{dayKey(day): {td}}
	tg.eventMode = true
	tg.eventIndex = len(items) - 1
	if withRange && n > 0 {
		tg.selAnchorUID = items[0].Event.UID
		tg.selAnchorOcc = items[0].Start
	}
	return tg
}

// drawTimeGrid renders tg once onto a simulation screen, returning how long the
// draw took.
func drawTimeGrid(t *testing.T, screen tcell.Screen, tg *timeGridView) time.Duration {
	t.Helper()
	tg.SetRect(0, 0, 100, 40)
	start := time.Now()
	tg.Draw(screen)
	return time.Since(start)
}

// timeGridDrawGrowth times Draw at two input sizes and returns the ratio. Both
// grids are built up front and the sizes are timed alternately, so a load spike
// from a concurrent build lands on both measurements rather than inflating the
// ratio; best-of-N then keeps the least-disturbed round.
func timeGridDrawGrowth(t *testing.T, small, large int, withRange bool) (time.Duration, time.Duration, float64) {
	t.Helper()
	const measureTries = 5

	screen := tcell.NewSimulationScreen("")
	if err := screen.Init(); err != nil {
		t.Fatalf("init simulation screen: %v", err)
	}
	defer screen.Fini()
	screen.SetSize(100, 40)

	tgSmall, tgLarge := selectRangeDayGrid(small, withRange), selectRangeDayGrid(large, withRange)
	runtime.GC() // don't charge this test for the heap the rest of the suite left behind

	ds, dl := time.Duration(1<<62-1), time.Duration(1<<62-1)
	for i := 0; i < measureTries; i++ {
		if d := drawTimeGrid(t, screen, tgSmall); d < ds {
			ds = d
		}
		if d := drawTimeGrid(t, screen, tgLarge); d < dl {
			dl = d
		}
	}
	return ds, dl, float64(dl) / float64(ds)
}

const (
	// 4x the input: a linear draw costs ~4x (measured 2.8x), the per-placement
	// scan of the drill list costs ~16x (measured 14.6x).
	drawScaleSmall     = 2000
	drawScaleLarge     = 4 * drawScaleSmall
	drawScaleMaxGrowth = 8.0
)

// TestTimeGridDrawWithSelectRangeIsNotQuadratic is the repro for the draw half of
// the pass-23 scale class: Draw's inSelRange closure called itemIndex — a linear
// scan of the day's drill list — once per placement and once per timed due task,
// so opening a SELECT range on a busy day made every frame O(placements x items).
// It only bites with a range active, which is why it went unnoticed.
func TestTimeGridDrawWithSelectRangeIsNotQuadratic(t *testing.T) {
	ds, dl, growth := timeGridDrawGrowth(t, drawScaleSmall, drawScaleLarge, true)
	t.Logf("Draw (SELECT range active): n=%d took %v, n=%d took %v (growth %.1fx for 4x input)",
		drawScaleSmall, ds, drawScaleLarge, dl, growth)
	if growth > drawScaleMaxGrowth {
		t.Fatalf("quadrupling the day's item count multiplied one draw by %.1fx (want <= %.1fx) — the placement->item join is superlinear",
			growth, drawScaleMaxGrowth)
	}
}

// inSelRangeLinearReference is the pre-index in-range test from Draw: resolve
// the anchor by scanning the day's drill list, then scan it again for the probed
// item. It is quadratic when called per drawn item, which is why it was replaced
// — but it decides which cells the user sees highlighted, so it stays here as
// the reference the indexed lookup is diffed against.
func inSelRangeLinearReference(tg *timeGridView, uid string, start time.Time) bool {
	selFrom, selTo := -1, -1
	if tg.eventMode && tg.selAnchorUID != "" {
		if ai := itemIndex(tg.daySelectables(), tg.selAnchorUID, tg.selAnchorOcc); ai >= 0 {
			selFrom, selTo = ai, tg.eventIndex
			if selFrom > selTo {
				selFrom, selTo = selTo, selFrom
			}
		}
	}
	if selFrom < 0 {
		return false
	}
	i := itemIndex(tg.daySelectables(), uid, start)
	return i >= selFrom && i <= selTo
}

// assignSelectUIDs spreads a small UID pool over a random day's events and tasks,
// so the drill list holds duplicate and empty UIDs as well as unique ones —
// exercising itemIndex's first-match-wins rule, which the index must reproduce.
func assignSelectUIDs(rng *rand.Rand, tg *timeGridView) {
	pool := []string{"u0", "u1", "u2", "", "dup", "dup"}
	pick := func() string { return pool[rng.Intn(len(pool))] }
	for _, occs := range tg.timed {
		for _, o := range occs {
			o.Event.UID = pick()
		}
	}
	for _, occs := range tg.allDay {
		for _, o := range occs {
			o.Event.UID = pick()
		}
	}
	for _, items := range tg.items {
		for _, it := range items {
			if it.Event != nil {
				it.Event.UID = pick()
			}
			if it.Todo != nil {
				it.Todo.UID = pick()
			}
		}
	}
}

// TestDrawSelRangeMatchesLinearScanReference is the rendering-identity guard for
// the indexed membership test: the in-range decision is user-visible selection
// feedback, so a faster lookup that highlights a different set of blocks is a
// regression, not a fix. Every item the draw path probes — every placement,
// every timed due task, every drill item — plus misses (unknown UID, shifted
// instant) and an equal instant in another location must decide exactly as the
// scan did, across random anchors and cursor positions.
func TestDrawSelRangeMatchesLinearScanReference(t *testing.T) {
	rng := rand.New(rand.NewSource(20260726))
	day := time.Date(2026, 3, 4, 0, 0, 0, 0, time.Local)

	for _, n := range []int{0, 1, 2, 5, 17, 64, 200} {
		for trial := 0; trial < 10; trial++ {
			tg := randomDayGrid(rng, n)
			assignSelectUIDs(rng, tg)
			items := tg.daySelectables()

			// Anchor: usually a real item (sometimes matched via a different
			// location, since itemIndex matches by instant), sometimes a miss, and
			// sometimes no range at all.
			switch {
			case len(items) == 0 || rng.Intn(6) == 0:
				tg.selAnchorUID, tg.selAnchorOcc = "", time.Time{}
			case rng.Intn(6) == 0:
				tg.selAnchorUID, tg.selAnchorOcc = "nosuchuid", day
			default:
				anchor := items[rng.Intn(len(items))]
				tg.selAnchorUID = targetFromItem(anchor).uid
				tg.selAnchorOcc = anchor.Start
				if rng.Intn(2) == 0 {
					tg.selAnchorOcc = tg.selAnchorOcc.UTC()
				}
			}
			tg.eventMode = rng.Intn(8) != 0
			if len(items) > 0 {
				tg.eventIndex = rng.Intn(len(items))
			} else {
				tg.eventIndex = 0
			}

			type probe struct {
				uid   string
				start time.Time
			}
			var probes []probe
			for _, p := range model.LayoutDay(tg.timed[dayKey(day)]) {
				probes = append(probes, probe{p.Occ.Event.UID, p.Occ.Start})
			}
			_, timedTasks := tg.dueParts(day)
			for _, td := range timedTasks {
				probes = append(probes, probe{td.UID, td.Due})
			}
			for _, it := range items {
				tgt := targetFromItem(it)
				probes = append(probes,
					probe{tgt.uid, tgt.occStart},
					probe{tgt.uid, tgt.occStart.UTC()},                // same instant, other location
					probe{tgt.uid, tgt.occStart.Add(time.Nanosecond)}, // near miss
					probe{tgt.uid + "x", tgt.occStart},                // unknown uid
				)
			}
			probes = append(probes, probe{"", day}, probe{"nosuchuid", day})

			from, to, itemAt := tg.selDrillRange()
			for _, pr := range probes {
				i, ok := itemAt[newItemKey(pr.uid, pr.start)]
				got := from >= 0 && ok && i >= from && i <= to
				want := inSelRangeLinearReference(tg, pr.uid, pr.start)
				if got != want {
					t.Fatalf("n=%d trial=%d probe %q@%v: in-range %v, want %v (range %d..%d)",
						n, trial, pr.uid, pr.start, got, want, from, to)
				}
			}
		}
	}
}

// TestTimeGridDrawWithoutSelectRangeIsLinear pins the common path so a future
// change cannot pay for the range-active fix by moving its cost into every frame.
func TestTimeGridDrawWithoutSelectRangeIsLinear(t *testing.T) {
	ds, dl, growth := timeGridDrawGrowth(t, drawScaleSmall, drawScaleLarge, false)
	t.Logf("Draw (no SELECT range): n=%d took %v, n=%d took %v (growth %.1fx for 4x input)",
		drawScaleSmall, ds, drawScaleLarge, dl, growth)
	if growth > drawScaleMaxGrowth {
		t.Fatalf("quadrupling the day's item count multiplied one draw by %.1fx (want <= %.1fx) — the common draw path is superlinear",
			growth, drawScaleMaxGrowth)
	}
}
