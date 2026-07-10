package ui

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/littekge/LazyPlanner/internal/store"
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

// TestMoveSubtreeRollsBackOnFailure: if a cross-list move fails partway (here the
// source delete fails because its directory is read-only), the already-moved
// nodes are rolled back so the subtree stays entirely in the source list.
func TestMoveSubtreeRollsBackOnFailure(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("read-only-dir fault injection doesn't hold for root")
	}
	now := time.Date(2026, 7, 5, 9, 0, 0, 0, time.UTC)
	dir := t.TempDir()
	copyTree(t, "../store/testdata/vdir", dir)
	s, err := store.Open(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	a := newApp(s, "test", now)
	a.build()
	a.reload()
	a.setMode(modeTasks)

	cals := s.Calendars()
	if len(cals) < 2 {
		t.Skip("need two calendars for a cross-list move")
	}
	srcCal, dstCal := cals[0].ID, cals[1].ID

	a.createTask(srcCal, "", "Mover")
	mover := todoBySummary(s, "Mover")
	a.createTask(srcCal, mover.UID, "Child")
	child := todoBySummary(s, "Child")

	// Read-only source dir → store.Delete's os.Remove fails mid-move (the dest
	// Put still succeeds), forcing the rollback path.
	srcDir := filepath.Join(dir, "calendars", srcCal)
	if err := os.Chmod(srcDir, 0o500); err != nil {
		t.Fatal(err)
	}
	defer os.Chmod(srcDir, 0o700) // let t.TempDir clean up

	a.yankUID = mover.UID
	a.moveSubtree(mover.UID, "", srcCal, dstCal)

	// Both nodes remain in the source; the dest holds no stray copy.
	if loc, ok := s.Locate(mover.UID); !ok || loc.CalID != srcCal {
		t.Errorf("Mover should remain in source after rollback, got %v", locCalID(a, mover.UID))
	}
	if loc, ok := s.Locate(child.UID); !ok || loc.CalID != srcCal {
		t.Errorf("Child should remain in source after rollback, got %v", locCalID(a, child.UID))
	}
	dcal, _ := s.Calendar(dstCal)
	for _, r := range dcal.Resources {
		for _, td := range r.Object.Todos {
			if td.UID == mover.UID || td.UID == child.UID {
				t.Errorf("dest retains moved node %q after rollback", td.UID)
			}
		}
	}
}
