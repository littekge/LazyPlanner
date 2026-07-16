package ui

import (
	"testing"
	"time"
)

// TestCopyTaskDuplicatesAndPersists: Y copies; p duplicates under the selection
// with a fresh UID leaving the original; the clipboard persists for multi-paste.
func TestCopyTaskDuplicatesAndPersists(t *testing.T) {
	a := newWritableTestApp(t, time.Date(2026, 7, 5, 9, 0, 0, 0, time.UTC))
	a.setMode(modeTasks)
	cal := a.selectedTasklistID()
	a.createTask(cal, "", "Parent")
	parent := todoBySummary(a.store, "Parent")
	a.createTask(cal, "", "Src")
	src := todoBySummary(a.store, "Src")
	a.buildTree()

	a.selectTreeByUID(src.UID)
	a.copyTask()
	if a.yankCut {
		t.Error("Y should set copy mode (not cut)")
	}
	a.selectTreeByUID(parent.UID)
	a.pasteUnderSelection()

	dupes := todosBySummary(a, "Src")
	if len(dupes) != 2 {
		t.Fatalf("after copy want 2 'Src' (original + copy), got %d", len(dupes))
	}
	// The original is untouched (still top-level); the copy is under Parent with a new UID.
	var copyUID string
	for _, d := range dupes {
		if d.UID == src.UID {
			if d.ParentUID != "" {
				t.Errorf("original Src should be untouched, parent=%q", d.ParentUID)
			}
		} else {
			copyUID = d.UID
			if d.ParentUID != parent.UID {
				t.Errorf("copy parent = %q, want %q", d.ParentUID, parent.UID)
			}
		}
	}
	if copyUID == "" {
		t.Error("copy should have a fresh UID distinct from the original")
	}
	// Clipboard persists → paste again makes a third.
	a.pasteUnderSelection()
	if n := len(todosBySummary(a, "Src")); n != 3 {
		t.Errorf("multi-paste should create another copy, got %d", n)
	}
}

// TestPasteAtTopLevel: P pastes at the list's top level (parent = none).
func TestPasteAtTopLevel(t *testing.T) {
	a := newWritableTestApp(t, time.Date(2026, 7, 5, 9, 0, 0, 0, time.UTC))
	a.setMode(modeTasks)
	cal := a.selectedTasklistID()
	a.createTask(cal, "", "Parent")
	parent := todoBySummary(a.store, "Parent")
	a.createTask(cal, parent.UID, "Child")
	child := todoBySummary(a.store, "Child")
	a.buildTree()

	// Cut the child, then paste it at top level with P.
	a.selectTreeByUID(child.UID)
	a.yankTask()
	a.pasteAtTop()
	if got := todoBySummary(a.store, "Child").ParentUID; got != "" {
		t.Errorf("child parent after paste-at-top = %q, want empty (top level)", got)
	}
}

// TestCopySubtreeRemapsChildren: copying a folder+child duplicates both with new
// UIDs, and the copied child points at the copied parent (not the original).
func TestCopySubtreeRemapsChildren(t *testing.T) {
	a := newWritableTestApp(t, time.Date(2026, 7, 5, 9, 0, 0, 0, time.UTC))
	a.setMode(modeTasks)
	cal := a.selectedTasklistID()
	a.createTask(cal, "", "Root")
	root := todoBySummary(a.store, "Root")
	a.createTask(cal, root.UID, "Kid")
	kid := todoBySummary(a.store, "Kid")
	a.buildTree()

	a.selectTreeByUID(root.UID)
	a.copyTask()
	a.pasteAtTop() // duplicate the Root subtree at top level

	roots := todosBySummary(a, "Root")
	kids := todosBySummary(a, "Kid")
	if len(roots) != 2 || len(kids) != 2 {
		t.Fatalf("want 2 Root + 2 Kid after subtree copy, got %d/%d", len(roots), len(kids))
	}
	// Find the copied root (new UID) and the copied kid; the copy's kid must point
	// at the copied root, not the original.
	var copyRoot string
	for _, r := range roots {
		if r.UID != root.UID {
			copyRoot = r.UID
		}
	}
	linked := false
	for _, k := range kids {
		if k.UID != kid.UID && k.ParentUID == copyRoot {
			linked = true
		}
	}
	if !linked {
		t.Error("copied child should link to the copied root, not the original")
	}
}
