package store_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/littekge/LazyPlanner/internal/store"
)

// TestDeleteRollsBackOnSidecarFailure: if the sidecar write fails after the .ics
// is already removed, the delete is rolled back — the resource stays present with
// no lingering tombstone — so a crash can't leave a lost tombstone that resurrects
// the item.
func TestDeleteRollsBackOnSidecarFailure(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	name := seedSyncedResource(t, dir, "cal1", "e@test", "Base")
	s, err := store.Open(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}

	// Sabotage the sidecar: replace it with a directory so the atomic rename
	// fails, while the .ics write/remove still works.
	sidecar := filepath.Join(dir, "calendars", "cal1", ".lazyplanner.json")
	if err := os.Remove(sidecar); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(sidecar, 0o755); err != nil {
		t.Fatal(err)
	}

	if err := s.Delete(ctx, "cal1", name); err == nil {
		t.Fatal("Delete should fail when the sidecar can't be persisted")
	}
	// Rolled back: resource present in memory and on disk, no tombstone.
	cal, _ := s.Calendar("cal1")
	if findResource(cal, name) == nil {
		t.Error("resource dropped in memory despite sidecar failure (not rolled back)")
	}
	if _, err := os.Stat(filepath.Join(dir, "calendars", "cal1", name)); err != nil {
		t.Errorf(".ics missing after rollback: %v", err)
	}
	if len(s.Tombstones()) != 0 {
		t.Errorf("a tombstone survived the rolled-back delete: %d", len(s.Tombstones()))
	}

	// Un-sabotage: a normal delete now succeeds (state was left consistent).
	if err := os.Remove(sidecar); err != nil {
		t.Fatal(err)
	}
	if err := s.Delete(ctx, "cal1", name); err != nil {
		t.Errorf("delete after un-sabotage failed: %v", err)
	}
}

// TestPutRollsBackOnSidecarFailure: an edit whose sidecar write fails leaves the
// resource's previous content on disk and in memory (not the failed edit).
func TestPutRollsBackOnSidecarFailure(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	name := seedSyncedResource(t, dir, "cal1", "e@test", "Base")
	s, err := store.Open(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}

	sidecar := filepath.Join(dir, "calendars", "cal1", ".lazyplanner.json")
	if err := os.Remove(sidecar); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(sidecar, 0o755); err != nil {
		t.Fatal(err)
	}

	if _, err := s.Put(ctx, "cal1", name, mustDecode(t, "e@test", "Edited")); err == nil {
		t.Fatal("Put should fail when the sidecar can't be persisted")
	}
	cal, _ := s.Calendar("cal1")
	r := findResource(cal, name)
	if r == nil {
		t.Fatal("resource missing after rolled-back Put")
	}
	if got := r.Object.Events[0].Summary; got != "Base" {
		t.Errorf("in-memory content = %q, want the reverted Base", got)
	}
	if r.Dirty {
		t.Error("resource left dirty after a rolled-back Put")
	}
}
