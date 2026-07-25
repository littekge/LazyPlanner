package ui

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/littekge/LazyPlanner/internal/model"
)

// findTaskNodeBySummary returns the visible tree node whose Todo summary matches.
func findTaskNodeBySummary(a *app, summary string) bool {
	for _, n := range visibleTreeNodes(a.tree.GetRoot()) {
		if t, ok := n.GetReference().(*model.Todo); ok && t.Summary == summary {
			a.tree.SetCurrentNode(n)
			return true
		}
	}
	return false
}

// todoUIDBySummary returns the UID of the visible tree row with this summary.
func todoUIDBySummary(a *app, summary string) string {
	for _, n := range visibleTreeNodes(a.tree.GetRoot()) {
		if t, ok := n.GetReference().(*model.Todo); ok && t.Summary == summary {
			return t.UID
		}
	}
	return ""
}

// TestSearchNextAfterCollectionShrinksAdvances guards the stale-index defect:
// searchNext used to keep a positional index into a match list it recomputes each
// press, so when the collection changed under the cursor the index no longer
// described where the selection actually was, and `n` re-selected the current row.
func TestSearchNextAfterCollectionShrinksAdvances(t *testing.T) {
	now := time.Date(2026, 7, 5, 9, 0, 0, 0, time.UTC)
	a := newWritableTestApp(t, now)
	a.setMode(modeTasks)
	cal := a.selectedTasklistID()
	for _, s := range []string{"zqx one", "zqx two", "zqx three", "zqx four", "zqx five"} {
		a.createTask(cal, "", s)
	}
	a.buildTree()

	// Display order is whatever buildTree produces; take it from the tree so the
	// repro does not depend on the sort.
	var order []string
	for _, n := range visibleTreeNodes(a.tree.GetRoot()) {
		if td, ok := n.GetReference().(*model.Todo); ok && strings.HasPrefix(td.Summary, "zqx ") {
			order = append(order, td.Summary)
		}
	}
	if len(order) != 5 {
		t.Fatalf("expected 5 zqx rows in the tree, got %v", order)
	}
	t.Logf("display order: %v", order)

	a.runSearch("zqx")
	got := currentTaskSummary(a)
	if got != order[0] {
		t.Fatalf("search landed on %q, want %q", got, order[0])
	}
	for i := 0; i < 3; i++ {
		a.searchNext(1)
	}
	if got := currentTaskSummary(a); got != order[3] {
		t.Fatalf("after 3x n, on %q, want %q", got, order[3])
	}
	t.Logf("searchIdx after 3x n = %d (selection %q)", a.searchIdx, currentTaskSummary(a))

	// A background sync removes the two matches ahead of the cursor. The list is
	// rebuilt; the selection stays on order[3] (now match index 1 of 3).
	for _, s := range []string{order[0], order[1]} {
		uid := todoUIDBySummary(a, s)
		if uid == "" {
			t.Fatalf("node %q not found", s)
		}
		loc, ok := a.store.Locate(uid)
		if !ok {
			t.Fatalf("locate %q: not found", s)
		}
		if err := a.store.Forget(context.Background(), loc.CalID, loc.Name); err != nil {
			t.Fatalf("forget %q: %v", s, err)
		}
	}
	a.reload()
	a.buildTree()
	if !findTaskNodeBySummary(a, order[3]) {
		t.Fatalf("%q vanished", order[3])
	}
	t.Logf("after sync: searchIdx still %d, selection %q", a.searchIdx, currentTaskSummary(a))

	a.searchNext(1)
	t.Logf("flash after n: %q", a.statusLeft.GetText(true))
	if got := currentTaskSummary(a); got == order[3] {
		t.Fatalf("n was a no-op: still on %q (searchIdx=%d); want it to advance to %q",
			got, a.searchIdx, order[4])
	} else if got != order[4] {
		t.Fatalf("n landed on %q, want %q", got, order[4])
	}
}

// TestSearchNextWalksEveryMatchWhenCollectionUnchanged is the other half of the
// stale-index class: re-deriving the cursor must not disturb the ordinary cycle.
// With the collection untouched, n walks the matches in display order and wraps,
// and N walks back the same way.
func TestSearchNextWalksEveryMatchWhenCollectionUnchanged(t *testing.T) {
	now := time.Date(2026, 7, 5, 9, 0, 0, 0, time.UTC)
	a := newWritableTestApp(t, now)
	a.setMode(modeTasks)
	cal := a.selectedTasklistID()
	for _, s := range []string{"zqx one", "zqx two", "zqx three", "zqx four", "zqx five"} {
		a.createTask(cal, "", s)
	}
	a.buildTree()

	var order []string
	for _, n := range visibleTreeNodes(a.tree.GetRoot()) {
		if td, ok := n.GetReference().(*model.Todo); ok && strings.HasPrefix(td.Summary, "zqx ") {
			order = append(order, td.Summary)
		}
	}
	if len(order) != 5 {
		t.Fatalf("expected 5 zqx rows in the tree, got %v", order)
	}

	a.runSearch("zqx")
	if got := currentTaskSummary(a); got != order[0] {
		t.Fatalf("search landed on %q, want %q", got, order[0])
	}
	for i := 1; i < len(order); i++ {
		a.searchNext(1)
		if got := currentTaskSummary(a); got != order[i] {
			t.Fatalf("n #%d landed on %q, want %q", i, got, order[i])
		}
	}
	a.searchNext(1) // past the last match → wraps to the first
	if got := currentTaskSummary(a); got != order[0] {
		t.Fatalf("n did not wrap: on %q, want %q", got, order[0])
	}
	for i := len(order) - 1; i >= 0; i-- {
		a.searchNext(-1) // N walks back, wrapping off the first match
		if got := currentTaskSummary(a); got != order[i] {
			t.Fatalf("N #%d landed on %q, want %q", len(order)-i, got, order[i])
		}
	}
}
