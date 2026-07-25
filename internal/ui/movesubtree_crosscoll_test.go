package ui

import (
	"testing"
	"time"
)

// TestMoveSubtreeCrossCollectionChildIsLost reproduces the MED finding:
// moveSubtreeOps assumes every descendant lives in srcCal. When a child already
// lives in the destination list (a legitimate real-world state: another client
// created the child in list B with RELATED-TO pointing at a parent in list A),
// cutting the parent and pasting into list B loses the child's local resource.
//
// Setup: parent P in "personal" (personal/P.ics); child C in "work" (work/C.ics)
// with ParentUID=P. Cut P, paste into "work". moveSubtreeOps walks uids=[P,C].
// For C, Locate(C)->work/C.ics (NOT a fresh create), so the bare Put clobbers
// C's real resource, then Delete(srcCal="personal", "C.ics") errors ("personal"
// has no C.ics), aborting the op. Rollback runs newest-first: the entry for C is
// Forget(work, "C.ics") — which deletes C's REAL resource with no tombstone.
// Net: C vanishes from the local store.
func TestMoveSubtreeCrossCollectionChildIsLost(t *testing.T) {
	now := time.Date(2026, 7, 5, 9, 0, 0, 0, time.UTC)
	a := newWritableTestApp(t, now)
	a.setMode(modeTasks)

	const srcCal = "personal"
	const dstCal = "work"

	// Parent P in list A (personal), as its own resource.
	a.createTask(srcCal, "", "Parent")
	parent := todoBySummary(a.store, "Parent")
	if parent == nil {
		t.Fatal("Parent not created")
	}
	// Child C in list B (work), as its own resource, linked to P.
	a.createTask(dstCal, parent.UID, "Child")
	child := todoBySummary(a.store, "Child")
	if child == nil {
		t.Fatal("Child not created")
	}
	if loc, ok := a.store.Locate(child.UID); !ok || loc.CalID != dstCal {
		t.Fatalf("precondition: Child must live in %q, got %v", dstCal, loc.CalID)
	}

	// Cut P and paste it into list B (the child's own list).
	a.yankUIDs = []string{parent.UID}
	a.moveSubtree(parent.UID, "", srcCal, dstCal)

	// The move failed (Delete of the phantom personal/C.ics errored) and rolled
	// back. The bug: rollback's Forget(work, C.ics) deleted the child's REAL
	// resource. The child should still exist somewhere in the store.
	if _, ok := a.store.Locate(child.UID); !ok {
		t.Fatalf("DEFECT CONFIRMED: child %q (Summary %q) vanished from the local store "+
			"after a failed cross-collection subtree move — its real resource %s/%s was Forgotten by rollback",
			child.UID, "Child", dstCal, "<C>.ics")
	}
}
