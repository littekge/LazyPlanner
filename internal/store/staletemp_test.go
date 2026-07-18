package store_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/littekge/LazyPlanner/internal/store"
)

// TestStaleTempSweepSpareLegitimateResource guards the fix for the sweep-eats-real-
// resource bug: a real .ics whose UID sanitizes to a name starting with "." and
// containing ".tmp-" (but ending in .ics) must NOT be deleted by the load-time
// stale-temp sweep. The sweep only removes actual writeFileAtomic leftovers, which
// end in ".tmp-<digits>"; the companion TestOpenSweepsStaleTempFiles pins that a
// genuine leftover is still removed, so this fix doesn't disable the sweep.
func TestStaleTempSweepSpareLegitimateResource(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()

	s, err := store.Open(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}

	uid := ".tmp-important@host"
	name := store.ResourceName(uid)
	t.Logf("resource name = %q", name)

	obj := mustDecode(t, uid, "Do not lose me")
	if _, err := s.Put(ctx, "cal1", name, obj); err != nil {
		t.Fatal(err)
	}

	// The file was written as a real cache file.
	path := filepath.Join(dir, "calendars", "cal1", name)
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected written file at %s: %v", path, err)
	}

	// Reopening from disk must still find the resource — instead the sweep deletes it.
	s2, err := store.Open(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}
	cal, ok := s2.Calendar("cal1")
	if !ok {
		t.Fatalf("cal1 missing after reload — resource was swept as a stale temp file")
	}
	if got := findResource(cal, name); got == nil {
		t.Fatalf("resource %q was deleted on Open (silent data loss)", name)
	}
}
