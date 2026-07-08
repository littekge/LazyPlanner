package ui

import (
	"strings"
	"testing"
	"time"

	"github.com/littekge/LazyPlanner/internal/model"
)

func currentTaskSummary(a *app) string {
	n := a.tree.GetCurrentNode()
	if n == nil {
		return ""
	}
	if t, ok := n.GetReference().(*model.Todo); ok {
		return t.Summary
	}
	return ""
}

func TestSearchTasksJumpsAndCycles(t *testing.T) {
	now := time.Date(2026, 7, 5, 9, 0, 0, 0, time.UTC)
	a := newWritableTestApp(t, now)
	a.setMode(modeTasks)
	cal := a.selectedTasklistID()
	for _, s := range []string{"Alpha", "Beta apples", "Gamma", "Beta bananas"} {
		a.createTask(cal, "", s)
	}
	a.buildTree()

	a.runSearch("beta")
	first := currentTaskSummary(a)
	if !strings.Contains(strings.ToLower(first), "beta") {
		t.Fatalf("search landed on %q, want a Beta match", first)
	}

	a.searchNext(1)
	second := currentTaskSummary(a)
	if second == first {
		t.Errorf("n did not advance to the next match (still %q)", second)
	}
	if !strings.Contains(strings.ToLower(second), "beta") {
		t.Errorf("second match %q is not a Beta match", second)
	}

	a.searchNext(1) // only two matches → wraps back to the first
	if got := currentTaskSummary(a); got != first {
		t.Errorf("cycle did not wrap: got %q, want %q", got, first)
	}
}

func TestSearchNoMatchFlashes(t *testing.T) {
	now := time.Date(2026, 7, 5, 9, 0, 0, 0, time.UTC)
	a := newWritableTestApp(t, now)
	a.setMode(modeTasks)
	cal := a.selectedTasklistID()
	a.createTask(cal, "", "Alpha")
	a.buildTree()

	a.runSearch("zzzznope")
	if got := a.statusLeft.GetText(true); !strings.Contains(got, "no match") {
		t.Errorf("expected a no-match flash, got %q", got)
	}
}

func TestSearchNextWithoutQuery(t *testing.T) {
	now := time.Date(2026, 7, 5, 9, 0, 0, 0, time.UTC)
	a := newWritableTestApp(t, now)
	a.setMode(modeTasks)
	a.searchNext(1)
	if got := a.statusLeft.GetText(true); !strings.Contains(got, "no active search") {
		t.Errorf("expected a no-active-search flash, got %q", got)
	}
}

func TestSearchCalendarNames(t *testing.T) {
	now := time.Date(2026, 7, 5, 9, 0, 0, 0, time.UTC)
	a := newWritableTestApp(t, now)
	a.setMode(modeCalendar)
	if a.calendars.GetItemCount() == 0 {
		t.Skip("fixture has no calendars")
	}
	a.runSearch("work")
	main, _ := a.calendars.GetItemText(a.calendars.GetCurrentItem())
	if !strings.Contains(strings.ToLower(main), "work") {
		t.Errorf("calendar search selected %q, want one containing 'work'", main)
	}
}
