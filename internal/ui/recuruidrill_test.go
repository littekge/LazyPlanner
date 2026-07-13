package ui

import (
	"context"
	"testing"
	"time"

	"github.com/littekge/LazyPlanner/internal/store"
)

// TestDeleteOccurrenceKeepsDrillFocus guards UI-2: deleting one occurrence of a
// recurring event (a scope-picker flow, no form open) must keep focus on the
// drilled calendar grid, not kick it to the Calendars overview. Before the fix
// commitMutation closed a never-opened pageForm, whose extra restoreFocus popped
// an empty focus stack and fell through to the Calendars list.
func TestDeleteOccurrenceKeepsDrillFocus(t *testing.T) {
	when := time.Date(2026, 7, 6, 12, 0, 0, 0, time.UTC)
	a := newRootedTestApp(t, when)
	if err := a.store.CreateCalendarLocal(context.Background(), "ev", store.CalendarMeta{DisplayName: "EV"}, []string{"VEVENT"}); err != nil {
		t.Fatal(err)
	}
	uid := putRecurringEvent(t, a, "ev", "Standup", time.Date(2026, 7, 6, 9, 0, 0, 0, time.UTC), "FREQ=WEEKLY;COUNT=4")
	a.reload()

	a.setMode(modeCalendar)
	g, ok := a.calendarPrimitive().(calGrid)
	if !ok {
		t.Fatalf("calendar primitive is not a grid: %T", a.calendarPrimitive())
	}
	// Drill into the first occurrence's day so drillState reports drilled.
	day := time.Date(2026, 7, 6, 0, 0, 0, 0, time.Local)
	g.reDrill(day, 0)
	if _, drilled, _ := g.drillState(); !drilled {
		t.Skip("could not drill the grid in this build/view; focus assertion N/A")
	}
	a.setFocus(a.calendarPrimitive())
	// Simulate the post-scope-picker state: the picker's own closeModal already
	// popped the focus stack.
	a.focusStack = nil

	loc, _ := a.store.Locate(uid)
	occ := time.Date(2026, 7, 13, 9, 0, 0, 0, time.UTC) // 2nd occurrence
	a.deleteOccurrence(loc, editTarget{uid: uid, occStart: occ})

	if a.tv.GetFocus() == a.calendars {
		t.Error("delete-occurrence kicked focus to the Calendars overview instead of keeping the drill")
	}
	if _, drilled, _ := a.calendarPrimitive().(calGrid).drillState(); !drilled {
		t.Error("drill was lost after deleting an occurrence")
	}
}
