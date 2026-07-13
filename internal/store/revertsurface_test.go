package store_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/littekge/LazyPlanner/internal/store"
)

// TestPutSingleFailureUsesCleanError guards M3's branching: when only the sidecar
// write fails and the revert succeeds (the common case), the error is the plain
// "updating sidecar" message — not the "cache may be inconsistent" double-failure
// message, which must be reserved for a revert that itself failed. The store must
// also reload consistently (the reverted content, clean).
func TestPutSingleFailureUsesCleanError(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	name := seedSyncedResource(t, dir, "cal1", "e@test", "Base")
	s, err := store.Open(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}

	// Sabotage only the sidecar (replace with a directory); the .ics dir stays
	// writable, so the revert write succeeds.
	sidecar := filepath.Join(dir, "calendars", "cal1", ".lazyplanner.json")
	if err := os.Remove(sidecar); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(sidecar, 0o755); err != nil {
		t.Fatal(err)
	}

	_, err = s.Put(ctx, "cal1", name, mustDecode(t, "e@test", "Edited"))
	if err == nil {
		t.Fatal("Put should fail when the sidecar can't be persisted")
	}
	if strings.Contains(err.Error(), "inconsistent") {
		t.Errorf("single sidecar failure wrongly reported as a double-failure: %v", err)
	}

	// Un-sabotage and reload: the on-disk .ics must be the reverted Base content,
	// loading clean (no divergence from the successful revert).
	if err := os.Remove(sidecar); err != nil {
		t.Fatal(err)
	}
	s2, err := store.Open(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}
	cal, _ := s2.Calendar("cal1")
	r := findResource(cal, name)
	if r == nil {
		t.Fatal("resource missing after reload")
	}
	if got := r.Object.Events[0].Summary; got != "Base" {
		t.Errorf("reloaded content = %q, want reverted Base", got)
	}
	if r.Dirty {
		t.Error("reverted resource reloaded dirty")
	}
}
