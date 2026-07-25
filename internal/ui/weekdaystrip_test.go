package ui

import (
	"testing"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

func TestWeekdayStripSeedAndRead(t *testing.T) {
	w := newWeekdayStrip("Repeat on")
	w.setDays([]time.Weekday{time.Tuesday, time.Thursday})
	got := w.days()
	if len(got) != 2 || got[0] != time.Tuesday || got[1] != time.Thursday {
		t.Fatalf("days() = %v, want [Tue Thu] (Monday-first order)", got)
	}
}

func TestWeekdayStripToggleAndCursor(t *testing.T) {
	w := newWeekdayStrip("Repeat on")
	w.setDays(nil)
	handler := w.InputHandler()
	noFocus := func(tview.Primitive) {}

	// Cursor starts at 0 (Mon). Right twice → Wed (index 2), Space toggles it on.
	handler(keyEv(tcell.KeyRight), noFocus)
	handler(keyEv(tcell.KeyRight), noFocus)
	handler(runeKey(' '), noFocus)
	if got := w.days(); len(got) != 1 || got[0] != time.Wednesday {
		t.Fatalf("after Right,Right,Space days() = %v, want [Wed]", got)
	}

	// Space again toggles it back off.
	handler(runeKey(' '), noFocus)
	if got := w.days(); len(got) != 0 {
		t.Fatalf("after second Space days() = %v, want []", got)
	}

	// Cursor clamps at the left edge.
	handler(keyEv(tcell.KeyLeft), noFocus)
	handler(keyEv(tcell.KeyLeft), noFocus)
	handler(keyEv(tcell.KeyLeft), noFocus)
	handler(runeKey(' '), noFocus)
	if got := w.days(); len(got) != 1 || got[0] != time.Monday {
		t.Fatalf("after clamping left + Space days() = %v, want [Mon]", got)
	}
}

// TestWeekdayStripCursorClampsAtRightEdge closes a Pass-20 mutation-canary escape:
// moveCursor's upper clamp (`w.cursor > daysInWeek-1`) had no test, so weakening it
// to `> daysInWeek` — letting the cursor reach index 7, one past the last cell 6 and
// out of bounds for the [daysInWeek]bool selection array — went uncaught. Stepping
// exactly onto 7 (not overshooting, which clamps under either version) is what
// exposes it.
func TestWeekdayStripCursorClampsAtRightEdge(t *testing.T) {
	w := newWeekdayStrip("Repeat on")
	w.setDays(nil)
	handler := w.InputHandler()
	noFocus := func(tview.Primitive) {}

	// Press Right once more than there are cells; the cursor must stop at the last
	// valid index (Sunday = 6), never step to 7.
	for i := 0; i < daysInWeek; i++ {
		handler(keyEv(tcell.KeyRight), noFocus)
	}
	if w.cursor != daysInWeek-1 {
		t.Fatalf("cursor after over-scrolling right = %d, want %d (last valid cell)", w.cursor, daysInWeek-1)
	}
	// Space at the clamped edge toggles Sunday without an out-of-bounds panic.
	handler(runeKey(' '), noFocus)
	if got := w.days(); len(got) != 1 || got[0] != time.Sunday {
		t.Fatalf("after clamping right + Space days() = %v, want [Sun]", got)
	}
}

func TestWeekdayStripSelectionIsLegible(t *testing.T) {
	w := newWeekdayStrip("Repeat on")
	w.setDays([]time.Weekday{time.Tuesday})
	w.SetRect(0, 0, 40, 1)
	screen := tcell.NewSimulationScreen("")
	if err := screen.Init(); err != nil {
		t.Fatalf("init screen: %v", err)
	}
	defer screen.Fini()
	screen.SetSize(40, 1)
	w.Draw(screen)
	screen.Show()
	cells, _, _ := screen.GetContents()
	for _, c := range cells {
		if _, _, attr := c.Style.Decompose(); attr&tcell.AttrReverse != 0 {
			return // a reverse-video cell means the selected day is legible
		}
	}
	t.Error("no reverse-video cell — selected day is illegible on terminal-default themes")
}

func TestWeekdayStripDrawStress(t *testing.T) {
	w := newWeekdayStrip("Repeat on")
	w.setDays([]time.Weekday{time.Monday, time.Wednesday, time.Friday})
	for _, g := range stressGeoms {
		drawGeom(t, "weekday-strip", w, g.w, g.h)
	}
}
