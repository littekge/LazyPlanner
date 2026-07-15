package model

import (
	"testing"
	"time"

	"github.com/emersion/go-ical"
)

// TestReproQuickFieldSetPreservesCompleted reproduces the finding: a quick sp/sd
// edit on an already-completed task must not rewrite its original COMPLETED
// timestamp to "now".
func TestReproQuickFieldSetPreservesCompleted(t *testing.T) {
	completedAt := time.Date(2025, 1, 1, 9, 0, 0, 0, time.UTC)

	// A task completed on 2025-01-01.
	obj := NewTodoObject(TodoDraft{
		Summary:   "old task",
		Completed: true,
	}, completedAt)

	todo := obj.Todos[0]
	if !todo.Completed() {
		t.Fatal("setup: task should be completed")
	}
	orig := todo.Raw.Props.Get(ical.PropCompleted).Value

	// A quick priority set on 2026-07-15. draftFromTodo carries Completed=true.
	now := time.Date(2026, 7, 15, 14, 0, 0, 0, time.UTC)
	edited, err := EditTodo(obj, todo.UID, TodoDraft{
		Summary:   "old task",
		Priority:  1,
		Completed: true,
	}, now, time.UTC)
	if err != nil {
		t.Fatalf("EditTodo: %v", err)
	}

	after := edited.Todos[0].Raw.Props.Get(ical.PropCompleted).Value
	if after != orig {
		t.Fatalf("COMPLETED overwritten: was %q, now %q", orig, after)
	}
}
