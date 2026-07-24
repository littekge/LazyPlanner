package ui

import (
	"strings"
	"testing"
	"time"

	"github.com/littekge/LazyPlanner/internal/model"
)

// The always-visible help bar (a.hints) must reflect the active grab's motion
// granularity for the whole grab, not just the transient entry flash — and for
// a bulk grab it must NOT keep showing the stale SELECT "hjkl extend" hints
// (hjkl shifts dates during the grab, it no longer extends the range).

// TestGrabHelpBarShowsEventGranularity: a single-item event grab in week view
// puts the ±hour controls on the persistent help bar.
func TestGrabHelpBarShowsEventGranularity(t *testing.T) {
	a := newWritableTestApp(t, time.Date(2026, 7, 5, 9, 0, 0, 0, time.UTC))
	a.setMode(modeCalendar)
	a.viewMode = viewWeek
	// Enter grab directly on an event (mirrors TestGrabEventMoveResizeCancel's
	// bypass of startGrab's calendar-drill selection).
	a.grabbing = true
	a.grabIsEvent = true

	a.updateStatus() // the repaint every nudge's refresh triggers

	got := a.hints.GetText(true)
	if !strings.Contains(got, "±hour") {
		t.Errorf("help bar during event grab = %q, want it to name the ±hour granularity", got)
	}
	if strings.Contains(got, "hjkl move") {
		t.Errorf("help bar during grab still shows the ordinary controls line: %q", got)
	}
}

// TestBulkGrabHelpBarShowsShiftGranularity: while a bulk grab is active the help
// bar names the ±day/±week shift and drops the SELECT extend hints.
func TestBulkGrabHelpBarShowsShiftGranularity(t *testing.T) {
	now := time.Date(2026, 7, 6, 9, 30, 0, 0, time.UTC)
	a := newRootedTestApp(t, now)
	a.setMode(modeCalendar)
	putEvent(t, a, testCalID(a), "meeting", now, false)
	a.refresh("")
	a.month.reDrill(model.DayStart(now), 0)
	a.setFocus(a.calendarPrimitive())
	a.enterSelect()
	a.startBulkGrab()
	if !a.grabbing || len(a.bulkGrab) == 0 {
		t.Fatalf("bulk grab did not start: grabbing=%v n=%d", a.grabbing, len(a.bulkGrab))
	}

	a.updateStatus() // the repaint every nudge's refreshKeepingDrill triggers

	got := a.hints.GetText(true)
	if !strings.Contains(got, "±week") || !strings.Contains(got, "±day") {
		t.Errorf("help bar during bulk grab = %q, want it to name the ±day/±week shift", got)
	}
	if strings.Contains(got, "extend") {
		t.Errorf("help bar during bulk grab still shows the stale SELECT extend hints: %q", got)
	}
}
