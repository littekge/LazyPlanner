package sync_test

import (
	"context"
	"testing"

	"github.com/littekge/LazyPlanner/internal/caldav"
	"github.com/littekge/LazyPlanner/internal/store"
	"github.com/littekge/LazyPlanner/internal/sync"
)

// TestImportEmptyHrefNotSilentlyLost guards the Pass-17 fix: a malformed/hostile
// server that returns objects with an empty href must not have them silently
// overwrite each other and be counted as imported. reconcileCalendar already
// skips empty-href objects (errEmptyHref); the Import loop must do the same.
//
// Before the fix, two distinct empty-href objects both mapped to the placeholder
// name resource.ics, collided in PullRemoteBatch (each clean, so no
// ErrKeptLocalEdit), and each overwrite was counted a success — res.Objects=2
// while only one resource was actually stored, zero skips recorded.
func TestImportEmptyHrefNotSilentlyLost(t *testing.T) {
	ctx := context.Background()
	src := &fakeSource{
		cals: []caldav.Calendar{{Path: "/dav/cal/personal/", Name: "Personal"}},
		objs: map[string][]caldav.Object{
			"/dav/cal/personal/": {
				{Path: "", ETag: `"etag-a"`, Data: mustCal(t, goodEvent1)},
				{Path: "", ETag: `"etag-b"`, Data: mustCal(t, goodEvent2)},
			},
		},
	}

	st, err := store.Open(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	res, err := sync.Import(ctx, src, st)
	if err != nil {
		t.Fatal(err)
	}

	// An unaddressable empty-href object can't be imported: both must be skipped,
	// none counted as a stored object.
	if res.Objects != 0 {
		t.Errorf("res.Objects = %d, want 0 (both empty-href objects unimportable)", res.Objects)
	}
	if len(res.Skipped) != 2 {
		t.Errorf("res.Skipped = %d, want 2 empty-href skips", len(res.Skipped))
	}

	// The store must not silently hold a colliding placeholder, and the reported
	// count must never exceed what is actually persisted (the silent-loss signal).
	cal, ok := st.Calendar("personal")
	if !ok {
		t.Fatal("personal calendar missing")
	}
	if got := len(cal.Resources); got != 0 {
		t.Errorf("stored resources = %d, want 0", got)
	}
	if res.Objects > len(cal.Resources) {
		t.Errorf("Import reported %d objects but stored %d — silent loss", res.Objects, len(cal.Resources))
	}
}
