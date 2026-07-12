package ui

import (
	"testing"
	"time"
)

// TestCountGSelectsNthTreeNode locks M2: <count>G must select the count-th item in
// the task tree (a list-like view), not just in tview.List overviews. Previously
// the count was honored only for *tview.List, so NG silently jumped to the last
// node in the tree.
func TestCountGSelectsNthTreeNode(t *testing.T) {
	now := time.Date(2026, 7, 5, 9, 0, 0, 0, time.UTC)
	a := newWritableTestApp(t, now)
	a.setMode(modeTasks)
	cal := a.selectedTasklistID()
	for _, n := range []string{"Task A", "Task B", "Task C", "Task D"} {
		a.createTask(cal, "", n)
	}
	a.buildTree()
	a.tv.SetFocus(a.tree)

	nodes := visibleTreeNodes(a.tree.GetRoot())
	if len(nodes) < 4 {
		t.Fatalf("want >=4 visible tree nodes, got %d", len(nodes))
	}

	a.gotoBottom(2) // 2G → 2nd visible node
	if a.tree.GetCurrentNode() != nodes[1] {
		t.Error("2G did not select the 2nd visible tree node")
	}
	a.gotoBottom(0) // G → last node
	if a.tree.GetCurrentNode() != nodes[len(nodes)-1] {
		t.Error("G did not select the last visible tree node")
	}
	a.gotoBottom(9999) // count past the end clamps to the last node
	if a.tree.GetCurrentNode() != nodes[len(nodes)-1] {
		t.Error("an over-large count did not clamp to the last visible tree node")
	}
}
