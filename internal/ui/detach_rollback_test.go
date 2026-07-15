package ui

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/littekge/LazyPlanner/internal/model"
	"github.com/littekge/LazyPlanner/internal/store"
)

// TestCommitDetachRollsBackSeriesOnStandaloneWriteFailure guards pass-11 HIGH #3:
// "edit this occurrence" for a recurring todo advances the series first (consuming
// the current occurrence), then writes the detached standalone one-off. If the
// standalone write fails, the detach must be atomic — the series is rolled back to
// its un-advanced state rather than left advanced with the occurrence lost (gone
// from the series and never the one-off task the confirm promised).
func TestCommitDetachRollsBackSeriesOnStandaloneWriteFailure(t *testing.T) {
	dataDir := t.TempDir()
	s, err := store.Open(context.Background(), dataDir)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	a := newApp(s, "test", time.Date(2026, 7, 6, 12, 0, 0, 0, time.UTC))
	a.build()

	if err := s.CreateCalendarLocal(context.Background(), "todo", store.CalendarMeta{DisplayName: "TODO"}, []string{"VTODO"}); err != nil {
		t.Fatal(err)
	}
	due := time.Date(2026, 7, 6, 9, 0, 0, 0, time.UTC)
	uid := putRecurringTodo(t, a, "todo", "Water plants", due, "FREQ=WEEKLY")
	a.reload()

	loc, _ := a.store.Locate(uid)
	origDue := findTodo(loc.Object, uid).Due

	// Build the two writes the detach performs.
	advanced, _, err := model.AdvanceRecurringTodo(loc.Object, uid, a.now, a.loc)
	if err != nil {
		t.Fatal(err)
	}
	d := model.TodoDraft{Summary: "Water plants (one-off)", HasDue: true, Due: origDue, DueAllDay: false}
	standalone := model.NewTodoObject(d, a.now)
	newUID := standalone.Todos[0].UID

	// Force the SECOND Put (the standalone) to fail deterministically: plant a
	// directory at the exact path the store would write the standalone to, so
	// writeFileAtomic's rename-over fails. The first Put (advancing the series, an
	// existing file) is unaffected.
	calDir := filepath.Join(dataDir, "calendars", "todo")
	if err := os.Mkdir(filepath.Join(calDir, store.ResourceName(newUID)), 0o755); err != nil {
		t.Fatalf("planting blocker dir: %v", err)
	}

	a.commitDetach(loc, newUID, advanced, standalone)

	// After the fix: the standalone write failed, so the series must be rolled back
	// to its original occurrence — the DUE is unchanged (not advanced a week) and
	// the standalone was not left behind.
	after := todoDue(t, a, uid)
	if !after.Due.Equal(origDue) {
		t.Errorf("data loss: series left advanced (due=%s, want the original %s) after a failed standalone write — detach did not roll back",
			after.Due.Format(time.RFC3339), origDue.Format(time.RFC3339))
	}
	if _, exists := a.store.Locate(newUID); exists {
		t.Errorf("standalone %q should not exist after a failed write", newUID)
	}
}
