package store_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/littekge/LazyPlanner/internal/store"
)

// seedSyncedResource writes one calendar with a single resource whose sidecar
// records a server ETag/href, so it looks like it came from a sync.
func seedSyncedResource(t *testing.T, dir, calID, uid, summary string) string {
	t.Helper()
	calDir := filepath.Join(dir, "calendars", calID)
	if err := os.MkdirAll(calDir, 0o700); err != nil {
		t.Fatal(err)
	}
	obj := mustDecode(t, uid, summary)
	b, _ := obj.Encode()
	name := store.ResourceName(uid)
	if err := os.WriteFile(filepath.Join(calDir, name), b, 0o600); err != nil {
		t.Fatal(err)
	}
	sc := `{"resources":{"` + name + `":{"etag":"\"srv-1\"","href":"/dav/` + calID + `/` + name + `"}}}`
	if err := os.WriteFile(filepath.Join(calDir, ".lazyplanner.json"), []byte(sc), 0o600); err != nil {
		t.Fatal(err)
	}
	return name
}

func TestDeleteSyncedResourceLeavesTombstone(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	name := seedSyncedResource(t, dir, "cal1", "synced@test", "Synced")

	s, err := store.Open(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Delete(ctx, "cal1", name); err != nil {
		t.Fatal(err)
	}

	ts := s.Tombstones()
	if len(ts) != 1 {
		t.Fatalf("Tombstones = %d, want 1", len(ts))
	}
	if ts[0].CalID != "cal1" || ts[0].Name != name || ts[0].ETag != `"srv-1"` || ts[0].Href != "/dav/cal1/"+name {
		t.Errorf("tombstone = %+v", ts[0])
	}

	// It survives a reload (the pending deletion must not be forgotten on restart).
	s2, err := store.Open(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}
	if got := s2.Tombstones(); len(got) != 1 || got[0].Name != name {
		t.Errorf("tombstone lost across reload: %+v", got)
	}

	// Clearing it (after a successful server DELETE) removes it, permanently.
	if err := s2.ClearTombstone(ctx, "cal1", name); err != nil {
		t.Fatal(err)
	}
	if got := s2.Tombstones(); len(got) != 0 {
		t.Errorf("tombstone not cleared: %+v", got)
	}
	s3, _ := store.Open(ctx, dir)
	if got := s3.Tombstones(); len(got) != 0 {
		t.Errorf("cleared tombstone reappeared after reload: %+v", got)
	}
}

func TestDeleteNeverSyncedLeavesNoTombstone(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	s, err := store.Open(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}
	// Put creates a dirty, never-synced resource (no Href).
	obj := mustDecode(t, "local@test", "Local only")
	name := store.ResourceName("local@test")
	if _, err := s.Put(ctx, "cal1", name, obj); err != nil {
		t.Fatal(err)
	}
	if err := s.Delete(ctx, "cal1", name); err != nil {
		t.Fatal(err)
	}
	if got := s.Tombstones(); len(got) != 0 {
		t.Errorf("never-synced delete left a tombstone: %+v", got)
	}
}

func TestRestoreClearsTombstone(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	name := seedSyncedResource(t, dir, "cal1", "synced@test", "Synced")

	s, err := store.Open(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}
	// Capture the pre-delete snapshot the way the UI undo stack does.
	loc, ok := s.Locate("synced@test")
	if !ok {
		t.Fatal("resource not located")
	}
	prev := loc.Prev

	if err := s.Delete(ctx, "cal1", name); err != nil {
		t.Fatal(err)
	}
	if len(s.Tombstones()) != 1 {
		t.Fatal("expected a tombstone after delete")
	}

	// Undo: restoring the resource must cancel the pending deletion.
	if _, err := s.Restore(ctx, "cal1", name, prev); err != nil {
		t.Fatal(err)
	}
	if got := s.Tombstones(); len(got) != 0 {
		t.Errorf("Restore did not clear the tombstone: %+v", got)
	}
}
