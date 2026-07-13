package sync_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/littekge/LazyPlanner/internal/caldav"
	"github.com/littekge/LazyPlanner/internal/store"
	"github.com/littekge/LazyPlanner/internal/sync"
)

// writeOrphan simulates a crash mid-bulk-pull: it drops an .ics into the
// calendar dir with no sidecar entry, so a reopened store loads it clean and
// href-less — the pull orphan the batched sidecar write can leave behind.
func writeOrphan(t *testing.T, dir, name, ics string) {
	t.Helper()
	// store.Open roots the cache at <dataDir>/calendars/.
	if err := os.WriteFile(filepath.Join(dir, "calendars", "personal", name), []byte(ics), 0o600); err != nil {
		t.Fatal(err)
	}
}

// TestSyncHealsPullOrphanWithoutDuplicating covers the data-safety guarantee of
// the batched-pull optimization: a pull orphan (an .ics whose batched sidecar
// flush was interrupted) must NOT be re-uploaded as a server-side duplicate;
// instead the next sync re-pulls the server's copy over it, healing it clean.
func TestSyncHealsPullOrphanWithoutDuplicating(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	name := store.ResourceName("e1@test")

	st, err := store.Open(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.SetCalendarMeta(ctx, "personal",
		store.CalendarMeta{DisplayName: "Personal", Href: calPath}); err != nil {
		t.Fatal(err)
	}
	writeOrphan(t, dir, name, eventICS("e1@test", "Orphan"))

	// Reopen so the orphan loads (Href=="", Dirty==false).
	st, err = store.Open(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}

	srv := newFakeServer()
	href := calPath + name
	srv.data[href] = caldav.Object{Path: href, ETag: "srv-1", Data: mkICal(t, eventICS("e1@test", "Server"))}

	if _, err := sync.Sync(ctx, srv, st); err != nil {
		t.Fatal(err)
	}

	if srv.puts != 0 {
		t.Fatalf("pull orphan was re-uploaded (%d PUTs) — would create a server-side duplicate", srv.puts)
	}
	r := findRes(t, st, name)
	if r == nil || r.Href != href || r.ETag != "srv-1" || r.Dirty {
		t.Fatalf("orphan not healed by re-pull: %+v", r)
	}
}

// TestSyncOrphanNotOnServerIsNotPushed covers the other orphan case: if the
// server no longer has the resource, the orphan must still not be pushed (which
// would resurrect a deleted item as a server-side create); it simply stays local.
func TestSyncOrphanNotOnServerIsNotPushed(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	name := store.ResourceName("gone@test")

	st, err := store.Open(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.SetCalendarMeta(ctx, "personal",
		store.CalendarMeta{DisplayName: "Personal", Href: calPath}); err != nil {
		t.Fatal(err)
	}
	writeOrphan(t, dir, name, eventICS("gone@test", "Orphan"))
	st, err = store.Open(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}

	srv := newFakeServer() // server has no such resource

	res, err := sync.Sync(ctx, srv, st)
	if err != nil {
		t.Fatal(err)
	}
	if srv.puts != 0 {
		t.Fatalf("orphan absent from server was pushed (%d PUTs) — would resurrect a deleted item", srv.puts)
	}
	if res.Pushed != 0 {
		t.Fatalf("Pushed=%d, want 0", res.Pushed)
	}
}
