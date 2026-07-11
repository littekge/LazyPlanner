package ui

import (
	"context"
	"testing"
	"time"

	"github.com/gdamore/tcell/v2"

	"github.com/littekge/LazyPlanner/internal/model"
	"github.com/littekge/LazyPlanner/internal/store"
)

// TestGrabTaskDueNudge: grabbing a dated task and pressing j twice pushes its due
// date +2 days; commit keeps it.
func TestGrabTaskDueNudge(t *testing.T) {
	a := newWritableTestApp(t, time.Date(2026, 7, 5, 9, 0, 0, 0, time.UTC))
	a.setMode(modeTasks)
	cal := a.selectedTasklistID()
	a.createTask(cal, "", "Deadline 2026-07-20")
	td := todoBySummary(a.store, "Deadline")
	if td == nil || !td.HasDue {
		t.Fatalf("setup: dated task not created (td=%v)", td)
	}
	a.buildTree()
	a.selectTreeByUID(td.UID)

	a.startGrab()
	if !a.grabbing {
		t.Fatal("should be grabbing a dated task")
	}
	a.grabNudge('j')
	a.grabNudge('j')
	a.commitGrab()
	if a.grabbing {
		t.Error("commit should exit grab mode")
	}
	got := todoBySummary(a.store, "Deadline")
	if d := got.Due; d.Year() != 2026 || d.Month() != 7 || d.Day() != 22 {
		t.Errorf("due after +2 days = %v, want 2026-07-22", got.Due)
	}
}

// TestGrabSkipsUndatedTask: an undated task can't be grabbed (flash-and-skip).
func TestGrabSkipsUndatedTask(t *testing.T) {
	a := newWritableTestApp(t, time.Date(2026, 7, 5, 9, 0, 0, 0, time.UTC))
	a.setMode(modeTasks)
	cal := a.selectedTasklistID()
	a.createTask(cal, "", "NoDue")
	nd := todoBySummary(a.store, "NoDue")
	a.buildTree()
	a.selectTreeByUID(nd.UID)
	a.startGrab()
	if a.grabbing {
		t.Error("an undated task should not enter grab mode")
	}
}

// TestGrabEventMoveResizeCancel: grabbing an event moves it by a day and resizes
// its end by an hour; Esc reverts to the original.
func TestGrabEventMoveResizeCancel(t *testing.T) {
	a := newWritableTestApp(t, time.Date(2026, 7, 5, 9, 0, 0, 0, time.UTC))
	ctx := context.Background()
	cal := a.store.Calendars()[0].ID
	start0 := time.Date(2026, 7, 15, 14, 0, 0, 0, time.UTC)
	end0 := time.Date(2026, 7, 15, 15, 0, 0, 0, time.UTC)
	obj, err := model.NewEventObject(model.EventDraft{Summary: "Meet", Start: start0, End: end0}, a.now)
	if err != nil {
		t.Fatal(err)
	}
	evUID := obj.Events[0].UID
	name := store.ResourceName(evUID)
	if _, err := a.store.Put(ctx, cal, name, obj); err != nil {
		t.Fatal(err)
	}

	// Grab it in week view (bypass startGrab's calendar-drill selection).
	a.setMode(modeCalendar)
	a.viewMode = viewWeek
	a.grabbing = true
	a.grabUID = evUID
	a.grabIsEvent = true
	a.grabCalID, a.grabName = cal, name
	if loc, ok := a.store.Locate(evUID); ok {
		a.grabPrev = loc.Prev
	}

	ev := func() *model.Event {
		loc, _ := a.store.Locate(evUID)
		return findEvent(loc.Object, evUID)
	}

	a.grabNudge('l') // +1 day
	if got := ev().Start; !got.Equal(start0.AddDate(0, 0, 1)) {
		t.Errorf("after day move, start = %v, want %v", got, start0.AddDate(0, 0, 1))
	}
	a.grabNudge('J') // resize end +1h
	if got := ev().End; !got.After(start0.AddDate(0, 0, 1).Add(time.Hour)) {
		t.Errorf("after resize, end = %v, want later", got)
	}

	a.cancelGrab()
	if a.grabbing {
		t.Error("cancel should exit grab mode")
	}
	if got := ev().Start; !got.Equal(start0) {
		t.Errorf("cancel should revert start to %v, got %v", start0, got)
	}
	if got := ev().End; !got.Equal(end0) {
		t.Errorf("cancel should revert end to %v, got %v", end0, got)
	}
}

// TestGrabKeyWiring: the m key enters grab mode and grab keys are intercepted by
// globalKeys (not leaked to the views).
func TestGrabKeyWiring(t *testing.T) {
	a := newRootedTestApp(t, time.Date(2026, 7, 5, 9, 0, 0, 0, time.UTC))
	a.setMode(modeTasks)
	cal := a.selectedTasklistID()
	a.createTask(cal, "", "Wire 2026-07-20")
	td := todoBySummary(a.store, "Wire")
	a.buildTree()
	a.selectTreeByUID(td.UID)

	a.globalKeys(runeKey('m'))
	if !a.grabbing {
		t.Fatal("m should enter grab mode")
	}
	a.globalKeys(runeKey('j')) // due +1 day
	a.globalKeys(tcell.NewEventKey(tcell.KeyEnter, 0, tcell.ModNone))
	if a.grabbing {
		t.Error("Enter should commit and exit grab mode")
	}
	if d := todoBySummary(a.store, "Wire").Due; d.Day() != 21 {
		t.Errorf("due after one j = %v, want the 21st", d)
	}
}
