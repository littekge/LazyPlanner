package store_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/littekge/LazyPlanner/internal/store"
)

// TestCreateCalendarLocalRollsBackOnSidecarFailure guards M5: if the sidecar
// write fails during a create, the in-memory entry must not linger (which would
// leave a phantom pendingCreate that never persists, so it's never MKCALENDAR'd on
// the server). A pre-existing directory must be preserved, and a retry must work.
func TestCreateCalendarLocalRollsBackOnSidecarFailure(t *testing.T) {
	ctx := context.Background()
	dataDir := t.TempDir()
	s, err := store.Open(ctx, dataDir)
	if err != nil {
		t.Fatal(err)
	}

	// Deterministically fail the sidecar write: pre-create the calendar dir and put
	// a directory where the sidecar file must go, so the atomic rename fails.
	calDir := filepath.Join(dataDir, "calendars", "newcal")
	if err := os.MkdirAll(filepath.Join(calDir, ".lazyplanner.json"), 0o755); err != nil {
		t.Fatal(err)
	}
	sentinel := filepath.Join(calDir, "keep.txt")
	if err := os.WriteFile(sentinel, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := s.CreateCalendarLocal(ctx, "newcal", store.CalendarMeta{DisplayName: "New"}, []string{"VEVENT"}); err == nil {
		t.Fatal("CreateCalendarLocal should fail when the sidecar can't be persisted")
	}
	// In-memory rollback: no phantom calendar.
	if _, ok := s.Calendar("newcal"); ok {
		t.Error("failed create left a phantom calendar in memory")
	}
	// Pre-existing dir + its content must be preserved (only a freshly-created dir
	// is removed on rollback).
	if _, err := os.Stat(sentinel); err != nil {
		t.Errorf("pre-existing dir content was destroyed: %v", err)
	}

	// Un-sabotage and retry: the clean rollback lets a fresh create succeed.
	if err := os.RemoveAll(filepath.Join(calDir, ".lazyplanner.json")); err != nil {
		t.Fatal(err)
	}
	if err := s.CreateCalendarLocal(ctx, "newcal", store.CalendarMeta{DisplayName: "New"}, []string{"VEVENT"}); err != nil {
		t.Errorf("retry after rollback failed: %v", err)
	}
	if _, ok := s.Calendar("newcal"); !ok {
		t.Error("calendar missing after successful retry")
	}
}
