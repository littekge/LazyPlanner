package ui

import (
	"strings"
	"testing"
	"time"
)

// TestSearchBackwardWraps closes the pass-10 canary hole: no test drove
// searchNext(-1), so a regression breaking the negative-index wrap
// ((idx + dir + len) % len) would panic on the N key undetected. This exercises
// backward cycling from the first match, which must wrap to the last.
func TestSearchBackwardWraps(t *testing.T) {
	now := time.Date(2026, 7, 5, 9, 0, 0, 0, time.UTC)
	a := newWritableTestApp(t, now)
	a.setMode(modeTasks)
	cal := a.selectedTasklistID()
	for _, s := range []string{"Beta one", "Alpha", "Beta two"} {
		a.createTask(cal, "", s)
	}
	a.buildTree()

	a.runSearch("beta") // lands on the first Beta match
	first := currentTaskSummary(a)

	a.searchNext(-1) // N from the first match must wrap to the last (no panic)
	last := currentTaskSummary(a)
	if last == first {
		t.Errorf("N did not move from the first match (still %q)", first)
	}
	if !strings.Contains(strings.ToLower(last), "beta") {
		t.Errorf("backward wrap landed on %q, want a Beta match", last)
	}

	a.searchNext(-1) // full cycle back to the first
	if got := currentTaskSummary(a); got != first {
		t.Errorf("second N did not return to the first: got %q, want %q", got, first)
	}
}
