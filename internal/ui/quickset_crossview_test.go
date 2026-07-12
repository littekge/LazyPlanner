package ui

import (
	"context"
	"testing"
	"time"

	"github.com/littekge/LazyPlanner/internal/model"
	"github.com/littekge/LazyPlanner/internal/store"
)

// TestQuickSetUngatedOutsideTasksView locks H3: the `s` quick-set chord (sp/sd)
// must be reachable outside the Tasks view — a task drilled into in the calendar
// (or selected in the agenda) can be completed/edited/deleted/grabbed, so it must
// also be quick-field-settable. Previously `s` was refused with "set: Tasks view
// only" everywhere but the tree, even though its target resolver is view-agnostic.
func TestQuickSetUngatedOutsideTasksView(t *testing.T) {
	when := time.Date(2026, 7, 20, 12, 0, 0, 0, time.UTC)
	a := newRootedTestApp(t, when)
	if err := a.store.CreateCalendarLocal(context.Background(), "tl", store.CalendarMeta{DisplayName: "TL"}, []string{"VTODO"}); err != nil {
		t.Fatal(err)
	}
	uid := putDueTask(t, a, "tl", "PayRent", time.Date(2026, 7, 20, 9, 0, 0, 0, time.Local))
	a.reload()

	// Drill onto the due task in the month grid.
	a.setMode(modeCalendar)
	a.viewMode = viewMonth
	a.anchor = model.DayStart(when)
	a.buildCenterCalendar()
	a.month.selected = a.anchor
	idx := todoIndexIn(a.month.selectedItems())
	if idx < 0 {
		t.Fatal("due task not present in the day's items")
	}
	a.month.eventMode = true
	a.month.eventIndex = idx

	// `s` now enters the set prefix from calendar mode (was refused before H3).
	a.globalKeys(runeKey('s'))
	if a.pendingPrefix != 's' {
		t.Fatalf("pressing s in calendar mode did not enter the set prefix (pendingPrefix=%q)", a.pendingPrefix)
	}
	a.clearPrefix()

	// And the resolver the sp/sd handlers use picks the drilled calendar task.
	got, ok := a.quickTaskTarget()
	if !ok || got != uid {
		t.Errorf("quickTaskTarget in a calendar drill = (%q,%v), want (%q,true)", got, ok, uid)
	}
}
