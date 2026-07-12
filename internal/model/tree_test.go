package model_test

import (
	"testing"
	"time"

	"github.com/littekge/LazyPlanner/internal/model"
)

func TestBuildTree(t *testing.T) {
	todos := []*model.Todo{
		{UID: "a", Summary: "Root A"},
		{UID: "b", Summary: "Root B"},
		{UID: "a1", Summary: "Child A1", ParentUID: "a"},
		{UID: "a2", Summary: "Child A2", ParentUID: "a"},
		{UID: "a1x", Summary: "Grandchild", ParentUID: "a1"},
		{UID: "orphan", Summary: "Orphan", ParentUID: "missing"},
		{UID: "done", Summary: "Done child", ParentUID: "b", Status: model.StatusCompleted},
	}

	t.Run("hides completed", func(t *testing.T) {
		roots := model.BuildTree(todos, false)
		// Roots sorted by title: Orphan, Root A, Root B.
		if got := uids(roots); !equalStrings(got, []string{"orphan", "a", "b"}) {
			t.Fatalf("roots = %v, want [orphan a b]", got)
		}
		rootA := roots[1]
		if got := uids(rootA.Children); !equalStrings(got, []string{"a1", "a2"}) {
			t.Errorf("Root A children = %v, want [a1 a2]", got)
		}
		if got := uids(rootA.Children[0].Children); !equalStrings(got, []string{"a1x"}) {
			t.Errorf("A1 children = %v, want [a1x]", got)
		}
		rootB := roots[2]
		if len(rootB.Children) != 0 {
			t.Errorf("Root B should have no children (done is hidden), got %d", len(rootB.Children))
		}
	})

	t.Run("shows completed when requested", func(t *testing.T) {
		roots := model.BuildTree(todos, true)
		var rootB *model.TodoNode
		for _, r := range roots {
			if r.Todo.UID == "b" {
				rootB = r
			}
		}
		if rootB == nil || len(rootB.Children) != 1 || rootB.Children[0].Todo.UID != "done" {
			t.Errorf("Root B should show the completed child when included")
		}
	})
}

func TestBuildTreeBreaksCycles(t *testing.T) {
	// a -> b -> a is malformed; BuildTree must not loop forever.
	todos := []*model.Todo{
		{UID: "a", Summary: "A", ParentUID: "b"},
		{UID: "b", Summary: "B", ParentUID: "a"},
	}
	roots := model.BuildTree(todos, true)
	// Whatever the arrangement, it must terminate and produce a finite tree.
	if countNodes(roots) > 2 {
		t.Errorf("cycle produced %d nodes, want <= 2", countNodes(roots))
	}
}

// TestBuildTreeCycleWithExtraChild guards a stack-overflow regression: a 2-cycle
// (B↔C) plus a third node D parented to B made the old unguarded descends() walk
// B→C→B→… forever. It must terminate and drop the cyclic nodes.
func TestBuildTreeCycleWithExtraChild(t *testing.T) {
	todos := []*model.Todo{
		{UID: "B", Summary: "B", ParentUID: "C"},
		{UID: "C", Summary: "C", ParentUID: "B"},
		{UID: "D", Summary: "D", ParentUID: "B"},
	}
	roots := model.BuildTree(todos, true) // must not crash
	// All three are only reachable through the cycle, so none are acyclic roots.
	if got := countNodes(roots); got != 0 {
		t.Errorf("cyclic forest produced %d nodes, want 0 (all dropped)", got)
	}
}

// TestBuildTreeEmptyUIDsDontCollapse guards Pass-3 #7: multiple UID-less todos
// (malformed data) must each surface once, not alias a single "" map slot where
// only the last survives and gets duplicated.
func TestBuildTreeEmptyUIDsDontCollapse(t *testing.T) {
	todos := []*model.Todo{
		{UID: "", Summary: "No-UID A"},
		{UID: "", Summary: "No-UID B"},
		{UID: "keyed", Summary: "Keyed"},
	}
	roots := model.BuildTree(todos, true)
	if len(roots) != 3 {
		t.Fatalf("roots = %d, want 3 (both UID-less todos + the keyed one)", len(roots))
	}
	got := map[string]int{}
	for _, r := range roots {
		got[r.Todo.Summary]++
	}
	for _, want := range []string{"No-UID A", "No-UID B", "Keyed"} {
		if got[want] != 1 {
			t.Errorf("summary %q appeared %d times, want exactly 1", want, got[want])
		}
	}
}

// TestBuildTreeDuplicateUIDPlacedOnce guards that a duplicate non-empty UID
// (same UID in two resources) places its node exactly once instead of appending
// the first occurrence's node twice.
func TestBuildTreeDuplicateUIDPlacedOnce(t *testing.T) {
	todos := []*model.Todo{
		{UID: "dup", Summary: "First"},
		{UID: "dup", Summary: "Second (ignored)"},
	}
	roots := model.BuildTree(todos, true)
	if len(roots) != 1 {
		t.Fatalf("roots = %d, want 1 (duplicate UID placed once)", len(roots))
	}
}

func TestSortTodos(t *testing.T) {
	day1 := time.Date(2026, 7, 4, 9, 0, 0, 0, time.UTC)
	day2 := time.Date(2026, 7, 5, 9, 0, 0, 0, time.UTC)
	todos := []*model.Todo{
		{UID: "F", Summary: "bbb", Priority: 0},
		{UID: "E", Summary: "aaa", Priority: 0},
		{UID: "D", Summary: "prio5", Priority: 5},
		{UID: "C", Summary: "prio1", Priority: 1},
		{UID: "A", Summary: "due-day2", HasDue: true, Due: day2},
		{UID: "B", Summary: "due-day1", HasDue: true, Due: day1},
	}
	model.SortTodos(todos)
	want := []string{"B", "A", "C", "D", "E", "F"}
	if got := todoUIDs(todos); !equalStrings(got, want) {
		t.Errorf("sorted order = %v, want %v", got, want)
	}
}

func uids(nodes []*model.TodoNode) []string {
	out := make([]string, len(nodes))
	for i, n := range nodes {
		out[i] = n.Todo.UID
	}
	return out
}

func todoUIDs(todos []*model.Todo) []string {
	out := make([]string, len(todos))
	for i, t := range todos {
		out[i] = t.UID
	}
	return out
}

func countNodes(nodes []*model.TodoNode) int {
	n := len(nodes)
	for _, node := range nodes {
		n += countNodes(node.Children)
	}
	return n
}
