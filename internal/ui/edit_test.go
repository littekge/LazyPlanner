package ui

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/littekge/LazyPlanner/internal/model"
	"github.com/littekge/LazyPlanner/internal/store"
)

// newWritableTestApp copies the store fixture into a temp dir and opens the app
// over it, so editing tests can mutate the cache without touching the fixture.
func newWritableTestApp(t *testing.T, now time.Time) *app {
	t.Helper()
	dir := t.TempDir()
	copyTree(t, "../store/testdata/vdir", dir)
	s, err := store.Open(context.Background(), dir)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	a := newApp(s, "test", now)
	a.build()
	a.reload()
	return a
}

func copyTree(t *testing.T, src, dst string) {
	t.Helper()
	err := filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(target, data, 0o644)
	})
	if err != nil {
		t.Fatalf("copy fixture: %v", err)
	}
}

func todoBySummary(s *store.Store, summary string) *model.Todo {
	for _, td := range s.Todos() {
		if td.Summary == summary {
			return td
		}
	}
	return nil
}

func TestCreateTaskAndUndo(t *testing.T) {
	now := time.Date(2026, 7, 5, 9, 0, 0, 0, time.UTC)
	a := newWritableTestApp(t, now)
	a.setMode(modeTasks)

	calID := a.selectedTasklistID()
	if calID == "" {
		t.Fatal("no task list selected")
	}
	a.createTask(calID, "", "Write docs tomorrow !2 #work")

	td := todoBySummary(a.store, "Write docs")
	if td == nil {
		t.Fatal("created task not found in store")
	}
	if td.Priority != 2 {
		t.Errorf("priority = %d, want 2", td.Priority)
	}
	if !td.HasDue || !td.DueAllDay {
		t.Errorf("due = %+v, want an all-day due", td)
	}
	if len(td.Categories) != 1 || td.Categories[0] != "work" {
		t.Errorf("categories = %v, want [work]", td.Categories)
	}

	a.undoLast()
	if todoBySummary(a.store, "Write docs") != nil {
		t.Error("undo did not remove the created task")
	}
}

func TestToggleCompleteAndUndo(t *testing.T) {
	now := time.Date(2026, 7, 5, 9, 0, 0, 0, time.UTC)
	a := newWritableTestApp(t, now)
	a.setMode(modeTasks)
	a.selectTreeByUID("grocery@lazyplanner.test")

	a.toggleComplete()
	if loc, ok := a.store.Locate("grocery@lazyplanner.test"); !ok || !findTodo(loc.Object, "grocery@lazyplanner.test").Completed() {
		t.Fatal("task not completed after toggle")
	}

	a.undoLast()
	loc, ok := a.store.Locate("grocery@lazyplanner.test")
	if !ok || findTodo(loc.Object, "grocery@lazyplanner.test").Completed() {
		t.Error("undo did not restore the incomplete state")
	}
}

func TestReparentIndentAndUndo(t *testing.T) {
	now := time.Date(2026, 7, 5, 9, 0, 0, 0, time.UTC)
	a := newWritableTestApp(t, now)
	a.setMode(modeTasks)
	calID := a.selectedTasklistID()

	// Two root siblings; titles sort so "Alpha" precedes "Beta".
	a.createTask(calID, "", "Alpha")
	a.createTask(calID, "", "Beta")

	alpha := todoBySummary(a.store, "Alpha")
	beta := todoBySummary(a.store, "Beta")
	if alpha == nil || beta == nil {
		t.Fatal("seed tasks missing")
	}

	a.selectTreeByUID(beta.UID)
	a.reparentSelected(indent)

	moved := todoBySummary(a.store, "Beta")
	if moved.ParentUID != alpha.UID {
		t.Errorf("Beta.ParentUID = %q, want %q (Alpha)", moved.ParentUID, alpha.UID)
	}

	a.undoLast()
	if got := todoBySummary(a.store, "Beta").ParentUID; got != "" {
		t.Errorf("undo did not clear the parent: %q", got)
	}
}
