package ui

import (
	"testing"
	"time"
)

func TestPasteReparentsWithinList(t *testing.T) {
	now := time.Date(2026, 7, 5, 9, 0, 0, 0, time.UTC)
	a := newWritableTestApp(t, now)
	a.setMode(modeTasks)
	cal := a.selectedTasklistID()
	a.createTask(cal, "", "Parent")
	parent := todoBySummary(a.store, "Parent")
	a.createTask(cal, "", "Mover")
	mover := todoBySummary(a.store, "Mover")
	a.buildTree()

	a.selectTreeByUID(mover.UID)
	a.yankTask()
	if a.yankUID != mover.UID {
		t.Fatalf("yankUID = %q, want %q", a.yankUID, mover.UID)
	}

	a.selectTreeByUID(parent.UID) // paste target
	a.pasteTask()
	if got := todoBySummary(a.store, "Mover").ParentUID; got != parent.UID {
		t.Errorf("Mover parent after paste = %q, want %q", got, parent.UID)
	}
	if a.yankUID != "" {
		t.Error("clipboard should clear after a paste")
	}

	a.undoLast()
	if got := todoBySummary(a.store, "Mover").ParentUID; got != "" {
		t.Errorf("Mover parent after undo = %q, want empty", got)
	}
}

func TestMoveSubtreeAcrossLists(t *testing.T) {
	now := time.Date(2026, 7, 5, 9, 0, 0, 0, time.UTC)
	a := newWritableTestApp(t, now)
	a.setMode(modeTasks)

	cals := a.store.Calendars()
	if len(cals) < 2 {
		t.Skip("need two calendars for a cross-list move")
	}
	srcCal, dstCal := cals[0].ID, cals[1].ID

	a.createTask(srcCal, "", "Mover")
	mover := todoBySummary(a.store, "Mover")
	a.createTask(srcCal, mover.UID, "Child")
	child := todoBySummary(a.store, "Child")

	a.yankUID = mover.UID
	a.moveSubtree(mover.UID, "", srcCal, dstCal)

	if loc, ok := a.store.Locate(mover.UID); !ok || loc.CalID != dstCal {
		t.Errorf("Mover calendar after move = %v, want %s", locCalID(a, mover.UID), dstCal)
	}
	if loc, ok := a.store.Locate(child.UID); !ok || loc.CalID != dstCal {
		t.Errorf("Child calendar after move = %v, want %s", locCalID(a, child.UID), dstCal)
	}
	if got := todoBySummary(a.store, "Child").ParentUID; got != mover.UID {
		t.Errorf("Child link broke in the move: parent = %q, want %q", got, mover.UID)
	}
	if got := todoBySummary(a.store, "Mover").ParentUID; got != "" {
		t.Errorf("moved root parent = %q, want empty (top level)", got)
	}
	if a.yankUID != "" {
		t.Error("clipboard should clear after a move")
	}

	a.undoLast()
	if loc, ok := a.store.Locate(mover.UID); !ok || loc.CalID != srcCal {
		t.Errorf("Mover calendar after undo = %v, want %s (restored)", locCalID(a, mover.UID), srcCal)
	}
	if loc, ok := a.store.Locate(child.UID); !ok || loc.CalID != srcCal {
		t.Errorf("Child calendar after undo = %v, want %s (restored)", locCalID(a, child.UID), srcCal)
	}
}

func locCalID(a *app, uid string) string {
	if loc, ok := a.store.Locate(uid); ok {
		return loc.CalID
	}
	return "(gone)"
}
