package store_test

import (
	"context"
	"testing"

	"github.com/littekge/LazyPlanner/internal/store"
)

// setupConflict seeds a synced resource, applies a local edit, and marks it
// conflicted with a diverging server version at etag "srv-2".
func setupConflict(t *testing.T, dir string) (*store.Store, string) {
	t.Helper()
	ctx := context.Background()
	name := seedSyncedResource(t, dir, "cal1", "e@test", "Base")
	s, err := store.Open(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Put(ctx, "cal1", name, mustDecode(t, "e@test", "Local edit")); err != nil {
		t.Fatal(err)
	}
	serverBytes, _ := mustDecode(t, "e@test", "Server edit").Encode()
	if err := s.MarkConflict(ctx, "cal1", name, serverBytes, "srv-2"); err != nil {
		t.Fatal(err)
	}
	if len(s.Conflicts()) != 1 {
		t.Fatalf("expected 1 conflict after setup, got %d", len(s.Conflicts()))
	}
	return s, name
}

func TestResolveKeepLocal(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	s, name := setupConflict(t, dir)

	if err := s.ResolveKeepLocal(ctx, "cal1", name); err != nil {
		t.Fatal(err)
	}
	if len(s.Conflicts()) != 0 {
		t.Error("conflict not cleared")
	}
	cal, _ := s.Calendar("cal1")
	r := findResource(cal, name)
	if r == nil || r.Conflicted || !r.Dirty {
		t.Fatalf("resource = %+v, want clean-of-conflict, dirty", r)
	}
	if r.ETag != "srv-2" {
		t.Errorf("ETag = %q, want the adopted server etag so the next push wins", r.ETag)
	}
	if r.Object.Events[0].Summary != "Local edit" {
		t.Errorf("content = %q, want the local edit kept", r.Object.Events[0].Summary)
	}
	// Survives reload as resolved (not conflicted).
	s2, _ := store.Open(ctx, dir)
	if len(s2.Conflicts()) != 0 {
		t.Error("conflict reappeared after reload")
	}
}

func TestResolveKeepServer(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	s, name := setupConflict(t, dir)

	if err := s.ResolveKeepServer(ctx, "cal1", name); err != nil {
		t.Fatal(err)
	}
	if len(s.Conflicts()) != 0 {
		t.Error("conflict not cleared")
	}
	cal, _ := s.Calendar("cal1")
	r := findResource(cal, name)
	if r == nil || r.Conflicted || r.Dirty {
		t.Fatalf("resource = %+v, want clean (server version adopted)", r)
	}
	if r.ETag != "srv-2" {
		t.Errorf("ETag = %q, want srv-2", r.ETag)
	}
	if r.Object.Events[0].Summary != "Server edit" {
		t.Errorf("content = %q, want the server version", r.Object.Events[0].Summary)
	}
}
