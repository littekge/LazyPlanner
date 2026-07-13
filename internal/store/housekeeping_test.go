package store_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/littekge/LazyPlanner/internal/store"
)

// TestSafeNameLengthCapped guards L5: an over-long UID/href still yields a
// writable, stable, collision-resistant file name under the filesystem limit.
func TestSafeNameLengthCapped(t *testing.T) {
	long := strings.Repeat("a", 1000)
	got := store.SafeName(long)
	if len(got)+len(".ics") > 255 {
		t.Errorf("SafeName length %d + .ics exceeds 255", len(got))
	}
	// Deterministic: same input → same name (so a resource maps to a stable file).
	if store.SafeName(long) != got {
		t.Error("SafeName is not deterministic for a long input")
	}
	// Two long inputs sharing the capped prefix must not collide.
	other := strings.Repeat("a", 1000) + "-different"
	if store.SafeName(other) == got {
		t.Error("distinct long inputs collided after capping")
	}
}

// TestOpenSweepsStaleTempFiles guards L6: a temp file left behind by an
// interrupted atomic write is removed on Open and doesn't disturb real resources.
func TestOpenSweepsStaleTempFiles(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	name := seedSyncedResource(t, dir, "cal1", "e@test", "Base")

	calDir := filepath.Join(dir, "calendars", "cal1")
	stale := filepath.Join(calDir, ".e@test.ics.tmp-123456")
	if err := os.WriteFile(stale, []byte("garbage"), 0o600); err != nil {
		t.Fatal(err)
	}

	s, err := store.Open(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(stale); !os.IsNotExist(err) {
		t.Errorf("stale temp file not swept on Open (err=%v)", err)
	}
	// The real resource still loaded.
	cal, _ := s.Calendar("cal1")
	if findResource(cal, name) == nil {
		t.Error("real resource missing after temp-file sweep")
	}
}
