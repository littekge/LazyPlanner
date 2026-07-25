package ui

import (
	"testing"
	"time"

	"github.com/littekge/LazyPlanner/internal/model"
)

// TestDaysRangeHighlightMatchesMaterialization is a REPRO for the LOW finding:
// the SELECT day-range visual highlight (dayInRange, used by drawCell) is not
// capped at maxSelectDays, but the materialization (daysRange) is. An item on a
// highlighted day beyond the cap renders as selected but is silently never
// acted on.
func TestDaysRangeHighlightMatchesMaterialization(t *testing.T) {
	now := time.Date(2026, 7, 6, 9, 0, 0, 0, time.UTC)
	a := newRootedTestApp(t, now)
	a.setMode(modeCalendar)

	// An event well past the 366-day materialization cap.
	beyond := model.DayStart(now.AddDate(0, 0, maxSelectDays+34)) // anchor+400
	putEvent(t, a, testCalID(a), "faraway", beyond, false)

	a.refresh("")
	a.setFocus(a.calendarPrimitive())
	a.month.selected = model.DayStart(now)
	a.enterSelect() // selDays: anchor = DayStart(now)
	a.month.selected = beyond
	a.syncSelectionVisuals()

	// Resolve the uid of the item sitting on the beyond-cap day.
	beyondItems := a.dayItems(beyond)
	if len(beyondItems) == 0 {
		t.Fatal("setup: expected an item on the beyond-cap day")
	}
	uid := targetFromItem(beyondItems[0]).uid

	// 1) The UI highlights the beyond-cap day: drawCell paints it in-range.
	highlighted := dayInRange(a.month.selDayAnchor, a.month.selected, beyond)

	// 2) The materialized target set (what bulkDelete/bulkComplete/startBulkGrab
	// actually act on) excludes the item on that day.
	materialized := false
	for _, tg := range a.daysRange() {
		if tg.uid == uid {
			materialized = true
			break
		}
	}

	t.Logf("beyond-cap day highlighted=%v, item materialized=%v", highlighted, materialized)
	if highlighted && !materialized {
		t.Fatalf("BUG: day is highlighted as selected but its item (uid=%s) is not in the acted-on set; "+
			"highlight (dayInRange) is uncapped while materialization (daysRange) clamps to maxSelectDays=%d",
			uid, maxSelectDays)
	}
}
