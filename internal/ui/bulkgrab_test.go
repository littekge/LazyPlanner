package ui

import (
	"testing"
	"time"

	"github.com/gdamore/tcell/v2"

	"github.com/littekge/LazyPlanner/internal/model"
)

// TestBulkGrabShiftsMixed: a drilled-day range holding an event and a due task
// shifts both by whole days (h/l) and weeks (j/k) — times of day untouched.
func TestBulkGrabShiftsMixed(t *testing.T) {
	now := time.Date(2026, 7, 6, 9, 30, 0, 0, time.UTC)
	a := newRootedTestApp(t, now)
	a.setMode(modeCalendar)
	putEvent(t, a, testCalID(a), "meeting", now, false)
	task := putTodo(t, a, testCalID(a), "", "due today", now, true)
	a.refresh("")
	a.month.reDrill(model.DayStart(now), 0)
	a.setFocus(a.calendarPrimitive())
	a.enterSelect()
	a.month.eventIndex = len(a.dayItems(model.DayStart(now))) - 1
	undoBefore := len(a.undo)
	a.startBulkGrab()
	if !a.grabbing || len(a.bulkGrab) != 2 {
		t.Fatalf("grabbing=%v n=%d, want true/2", a.grabbing, len(a.bulkGrab))
	}
	if m := a.interactionMode(); m != modeGrab {
		t.Fatalf("badge = %q, want GRAB (innermost)", m)
	}
	// Captured now (before the commit clears bulkGrab) so the undo assertion
	// below can Locate the event afterward — putEvent doesn't return a UID.
	var evUID string
	for _, it := range a.bulkGrab {
		if !it.isTodo {
			evUID = it.uid
		}
	}
	if evUID == "" {
		t.Fatal("no event in the bulk grab set")
	}

	a.handleBulkGrabKey(tcell.NewEventKey(tcell.KeyRune, 'l', tcell.ModNone)) // +1 day
	a.handleBulkGrabKey(tcell.NewEventKey(tcell.KeyRune, 'j', tcell.ModNone)) // +1 week
	a.handleBulkGrabKey(tcell.NewEventKey(tcell.KeyEnter, 0, tcell.ModNone))  // keep

	if a.grabbing || a.selecting {
		t.Fatal("Enter must exit both GRAB and SELECT")
	}
	loc, _ := a.store.Locate(task)
	td := findTodo(loc.Object, task)
	want := now.AddDate(0, 0, 8)
	if td == nil || !td.Due.Equal(want) {
		t.Fatalf("task due = %v, want %v (+8 days, time preserved)", td.Due, want)
	}

	// A committed bulk grab is exactly one undo step, and undoLast restores
	// both items in that one step.
	if len(a.undo) != undoBefore+1 {
		t.Fatalf("undo stack grew by %d, want 1", len(a.undo)-undoBefore)
	}
	a.undoLast()
	loc, _ = a.store.Locate(task)
	td = findTodo(loc.Object, task)
	if td == nil || !td.Due.Equal(now) {
		t.Fatalf("undo did not restore task due; got %v want %v", td.Due, now)
	}
	loc, ok := a.store.Locate(evUID)
	if !ok {
		t.Fatal("event vanished after undo")
	}
	ev := findEvent(loc.Object, evUID)
	if ev == nil || !ev.Start.Equal(now) {
		t.Fatalf("undo did not restore event start; got %v want %v", ev.Start, now)
	}
}

// TestBulkGrabEscRevertsToSelect: Esc restores every pre-grab snapshot and
// returns to SELECT with the range intact (retry-friendly), no undo step.
func TestBulkGrabEscRevertsToSelect(t *testing.T) {
	now := time.Date(2026, 7, 6, 9, 0, 0, 0, time.UTC)
	a := newRootedTestApp(t, now)
	a.setMode(modeCalendar)
	putEvent(t, a, testCalID(a), "meeting", now, false)
	task := putTodo(t, a, testCalID(a), "", "due today", now, true)
	a.refresh("")
	a.month.reDrill(model.DayStart(now), 0)
	a.setFocus(a.calendarPrimitive())
	a.enterSelect()
	a.month.eventIndex = len(a.dayItems(model.DayStart(now))) - 1
	undoBefore := len(a.undo)
	a.startBulkGrab()
	a.handleBulkGrabKey(tcell.NewEventKey(tcell.KeyRune, 'l', tcell.ModNone))
	a.handleBulkGrabKey(tcell.NewEventKey(tcell.KeyEscape, 0, tcell.ModNone))

	if a.grabbing {
		t.Fatal("Esc must end the grab")
	}
	if !a.selecting {
		t.Fatal("Esc must return to SELECT (one nesting level)")
	}
	loc, _ := a.store.Locate(task)
	if td := findTodo(loc.Object, task); td == nil || !td.Due.Equal(now) {
		t.Fatal("Esc must restore the pre-grab due date")
	}
	if len(a.undo) != undoBefore {
		t.Fatal("a cancelled grab pushes no undo step")
	}
}

// TestBulkGrabFilters: recurring events and undated tasks never enter the
// grab set; if nothing is grabbable SELECT stays active with a flash.
func TestBulkGrabFilters(t *testing.T) {
	now := time.Date(2026, 7, 6, 9, 0, 0, 0, time.UTC)
	a := newRootedTestApp(t, now)
	a.setMode(modeCalendar)
	putRecurringEvent(t, a, testCalID(a), "weekly", now, "FREQ=WEEKLY")
	a.refresh("")
	a.month.reDrill(model.DayStart(now), 0)
	a.setFocus(a.calendarPrimitive())
	a.enterSelect()
	a.startBulkGrab()
	if a.grabbing {
		t.Fatal("a range of only-recurring events must not start a grab")
	}
	if !a.selecting {
		t.Fatal("SELECT must stay active when nothing is grabbable")
	}
}
