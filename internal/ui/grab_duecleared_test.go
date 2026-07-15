package ui

import (
	"context"
	"testing"
	"time"

	"github.com/littekge/LazyPlanner/internal/model"
)

// TestGrabTodoDueClearedMidGrab guards pass-11 LOW #7: a todo whose due date is
// cleared by a concurrent sync AFTER startGrab (which checks HasDue) but BEFORE a
// nudge. grabNudge used to re-locate without re-checking HasDue, so it shifted a
// zero (year-1) time and flashed a bogus "due Jan 1, year 1" while writing nothing
// meaningful. After the fix the nudge re-checks HasDue, refuses to fabricate a
// date, and ends the grab.
func TestGrabTodoDueClearedMidGrab(t *testing.T) {
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

	// Simulate a background sync replacing the object with a server version that
	// dropped the due date. Write a HasDue=false version under the same resource.
	loc, ok := a.store.Locate(a.grabUID)
	if !ok {
		t.Fatal("locate before clear")
	}
	cleared := draftFromTodo(td)
	cleared.HasDue = false
	cleared.Due = time.Time{}
	clearedObj, err := model.EditTodo(loc.Object, a.grabUID, cleared, a.now, a.loc)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := a.store.Put(context.Background(), a.grabCalID, a.grabName, clearedObj); err != nil {
		t.Fatal(err)
	}
	if got := todoBySummary(a.store, "Deadline"); got.HasDue {
		t.Fatal("setup: concurrent clear should have removed the due date")
	}

	// Nudge +1 day. Correct behavior: recognize the task is now undated and refuse
	// (like startGrab does) instead of reporting a fabricated date.
	a.grabNudge('j')

	flash := a.statusLeft.GetText(true)
	after := todoBySummary(a.store, "Deadline")

	// After the fix: no fabricated due date is persisted (HasDue stays false; never
	// a year-1 sentinel), and the grab ends rather than pretending to move.
	if after.HasDue {
		t.Errorf("undated task gained a due date (%v) after a nudge — the nudge should have refused", after.Due)
	}
	if a.grabbing {
		t.Errorf("grab should have ended when the due date was cleared underneath; still grabbing (flash %q)", flash)
	}
}
