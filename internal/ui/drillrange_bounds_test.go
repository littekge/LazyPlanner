package ui

import (
	"testing"
	"time"

	"github.com/littekge/LazyPlanner/internal/model"
)

// TestDrillRangeAtTerminalIndexDoesNotPanic closes the Pass-22 canary escape on
// drillRange's upper-bound guard (`idx >= len(items)`). drillRange is never
// referenced by name in the UI tests and no test drove a drilled-day SELECT range
// with the cursor at the day's terminal index, so weakening `>=` to `>` escaped —
// with idx == len(items) the slice `items[ai : idx+1]` runs one past the end and
// panics the whole TUI. This is reachable in practice when a remote delete shrinks
// the drilled day under a stale cursor index.
//
// The test sets the drill cursor exactly to len(items) with a valid SELECT anchor
// (so ai >= 0 and the idx branch of the guard is the one under test): correct code
// returns nil (guard fires); the `>=`→`>` mutation panics, failing the test.
func TestDrillRangeAtTerminalIndexDoesNotPanic(t *testing.T) {
	now := time.Date(2026, 7, 6, 9, 0, 0, 0, time.UTC)
	a := newRootedTestApp(t, now)
	a.setMode(modeCalendar)
	day := model.DayStart(now)

	putEvent(t, a, testCalID(a), "e1", now, false)
	putEvent(t, a, testCalID(a), "e2", now.Add(time.Hour), false)
	a.refresh("")
	a.month.reDrill(day, 0) // drill into the day, cursor on item 0
	a.setFocus(a.calendarPrimitive())
	a.enterSelect() // anchor = the drilled item 0 (ai >= 0 in drillRange)

	items := a.dayItems(day)
	if len(items) < 2 {
		t.Fatalf("setup: expected ≥2 items on the drilled day, got %d", len(items))
	}
	// Force the drill cursor one past the last item — the exact boundary the guard
	// defends against.
	a.month.eventIndex = len(items)

	if got := a.drillRange(); got != nil {
		t.Fatalf("drillRange at the terminal index (idx==len) must return nil via the guard, got %d targets", len(got))
	}
}
