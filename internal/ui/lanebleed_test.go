package ui

import (
	"fmt"
	"testing"
	"time"

	"github.com/gdamore/tcell/v2"

	"github.com/littekge/LazyPlanner/internal/model"
)

// paintedBlockCells draws tg into a screen of size screenW x screenH after
// giving the primitive a rect of paneW x paneH (screenW may be wider, so writes
// that escape the primitive's own rect are still observable rather than clipped
// away by the screen edge). It returns the x coordinates of every cell painted
// with the event-block background.
func paintedBlockCells(t *testing.T, tg *timeGridView, paneW, paneH, screenW, screenH int) map[int]int {
	t.Helper()
	screen := tcell.NewSimulationScreen("")
	if err := screen.Init(); err != nil {
		t.Fatalf("init simulation screen: %v", err)
	}
	defer screen.Fini()
	screen.SetSize(screenW, screenH)
	tg.SetRect(0, 0, paneW, paneH)
	tg.Draw(screen)
	screen.Show()

	cells, cw, ch := screen.GetContents()
	byX := map[int]int{}
	for row := 0; row < ch; row++ {
		for col := 0; col < cw; col++ {
			_, bg, _ := cells[row*cw+col].Style.Decompose()
			if bg == blockColor {
				byX[col]++
			}
		}
	}
	return byX
}

// concurrentEvents builds n events that all run from 14:00 to 16:00 on day.
func concurrentEvents(day time.Time, n int) map[string][]model.Occurrence {
	start := time.Date(day.Year(), day.Month(), day.Day(), 14, 0, 0, 0, time.Local)
	end := start.Add(2 * time.Hour)
	var occs []model.Occurrence
	for i := 0; i < n; i++ {
		ev := &model.Event{Summary: fmt.Sprintf("E%d", i), Start: start}
		occs = append(occs, model.Occurrence{Start: start, End: end, Event: ev})
	}
	return map[string][]model.Occurrence{dayKey(day): occs}
}

// TestBlockNeverPaintsOutsideItsDayColumn: in a narrow week pane, more overlap
// lanes than the column has cells must not push a block into the neighbouring
// day's column (the event would read as being on the wrong day).
func TestBlockNeverPaintsOutsideItsDayColumn(t *testing.T) {
	tg := newTimeGridView()
	monday := time.Date(2026, 7, 6, 0, 0, 0, 0, time.Local)
	var days []time.Time
	for i := 0; i < 7; i++ {
		days = append(days, monday.AddDate(0, 0, i))
	}
	// All 10 events live on Monday only; every other column must stay empty.
	tg.setData(days, concurrentEvents(monday, 10), nil, monday, monday)

	const paneW = 60
	byX := paintedBlockCells(t, tg, paneW, 40, paneW, 40)

	colW := (paneW - gutterWidth) / len(days) // 7
	mondayEnd := gutterWidth + colW           // first x belonging to Tuesday

	bled := 0
	for x, n := range byX {
		if x >= mondayEnd {
			bled += n
		}
	}
	if bled > 0 {
		t.Errorf("Monday's blocks painted %d cells at x >= %d (inside Tuesday+ columns); per-x counts: %v",
			bled, mondayEnd, byX)
	}
}

// TestOverflowingLanesStillPaintTheWholeColumn is the other side of the clamp:
// degrading must not degrade into blankness. When there are more overlap lanes
// than the column has cells the lanes collapse and share cells, but every cell
// of the column's content width still gets painted — the events are squeezed,
// never dropped.
func TestOverflowingLanesStillPaintTheWholeColumn(t *testing.T) {
	tg := newTimeGridView()
	monday := time.Date(2026, 7, 6, 0, 0, 0, 0, time.Local)
	var days []time.Time
	for i := 0; i < 7; i++ {
		days = append(days, monday.AddDate(0, 0, i))
	}
	tg.setData(days, concurrentEvents(monday, 10), nil, monday, monday)

	const paneW = 60
	byX := paintedBlockCells(t, tg, paneW, 40, paneW, 40)

	colW := (paneW - gutterWidth) / len(days)
	contentW := colW - columnSeparatorWidth
	for x := gutterWidth; x < gutterWidth+contentW; x++ {
		if byX[x] == 0 {
			t.Errorf("column cell x=%d painted nothing; the collapse dropped an event instead of squeezing it (per-x counts: %v)", x, byX)
		}
	}
}

// TestTwoLaneDayKeepsFullBlockWidth guards the legitimate case against the clamp:
// a day whose lanes comfortably fit must still get full-width blocks covering the
// column, so the overflow handling cannot silently shrink ordinary rendering.
func TestTwoLaneDayKeepsFullBlockWidth(t *testing.T) {
	tg := newTimeGridView()
	day := time.Date(2026, 7, 6, 0, 0, 0, 0, time.Local)
	tg.setData([]time.Time{day}, concurrentEvents(day, 2), nil, day, day)

	const (
		paneW = 40
		lanes = 2
	)
	byX := paintedBlockCells(t, tg, paneW, 40, paneW, 40)

	colW := paneW - gutterWidth
	laneW := (colW - columnSeparatorWidth) / lanes
	firstX, lastX := gutterWidth, gutterWidth+lanes*laneW-1
	if laneW < 2 {
		t.Fatalf("test setup: expected a comfortably wide lane, got %d", laneW)
	}
	for x := firstX; x <= lastX; x++ {
		if byX[x] == 0 {
			t.Errorf("x=%d unpainted: the two lanes should tile [%d,%d] at %d cells each (per-x counts: %v)",
				x, firstX, lastX, laneW, byX)
		}
	}
	for x := range byX {
		if x < firstX || x > lastX {
			t.Errorf("x=%d painted outside the two lanes' span [%d,%d] (per-x counts: %v)", x, firstX, lastX, byX)
		}
	}
}

// TestBlockNeverPaintsOutsideThePaneRect: a day view in a very narrow pane must
// keep every painted cell inside the primitive's own rect — anything beyond it
// overwrites the adjacent pane in the real layout.
func TestBlockNeverPaintsOutsideThePaneRect(t *testing.T) {
	tg := newTimeGridView()
	day := time.Date(2026, 7, 6, 0, 0, 0, 0, time.Local)
	tg.setData([]time.Time{day}, concurrentEvents(day, 24), nil, day, day)

	const paneW = 20
	// The screen is far wider than the pane so escaped writes are visible.
	byX := paintedBlockCells(t, tg, paneW, 40, 80, 40)

	overflow := 0
	for x, n := range byX {
		if x >= paneW {
			overflow += n
		}
	}
	if overflow > 0 {
		t.Errorf("blocks painted %d cells at x >= paneW (%d), outside the primitive's rect; per-x counts: %v",
			overflow, paneW, byX)
	}
}
