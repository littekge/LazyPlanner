package sync_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/littekge/LazyPlanner/internal/caldav"
	"github.com/littekge/LazyPlanner/internal/store"
	"github.com/littekge/LazyPlanner/internal/sync"
)

// TestTombstoneNameCannotEscapeCacheRoot guards the pass-23 HIGH: a tombstone key
// read from a calendar's sidecar was never validated as a single path element, so
// the 412 (delete-vs-server-change) resurrect wrote the server's iCalendar through
// filepath.Join(root, calID, name) — landing outside the data dir.
func TestTombstoneNameCannotEscapeCacheRoot(t *testing.T) {
	ctx := context.Background()

	base := t.TempDir()
	victim := filepath.Join(base, "victim.txt")
	if err := os.WriteFile(victim, []byte("ORIGINAL CONTENT"), 0o600); err != nil {
		t.Fatal(err)
	}

	// dataDir is nested so the traversal name below resolves to base/victim.txt:
	// <base>/x/y/data/calendars/personal/../../../../../victim.txt
	dataDir := filepath.Join(base, "x", "y", "data")
	calDir := filepath.Join(dataDir, "calendars", "personal")
	if err := os.MkdirAll(calDir, 0o700); err != nil {
		t.Fatal(err)
	}

	const escapeName = "../../../../../victim.txt"
	const objHref = calPath + "x.ics"
	sidecar := map[string]any{
		"display_name": "Personal",
		"href":         calPath,
		"tombstones": map[string]any{
			escapeName: map[string]string{"href": objHref, "etag": "srv-1"},
		},
	}
	data, err := json.Marshal(sidecar)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(calDir, ".lazyplanner.json"), data, 0o600); err != nil {
		t.Fatal(err)
	}

	st, err := store.Open(ctx, dataDir)
	if err != nil {
		t.Fatal(err)
	}

	srv := newFakeServer()
	// The server still has the resource and refuses the conditional DELETE (412) —
	// an ordinary delete-vs-remote-change race.
	srv.data[objHref] = caldav.Object{Path: objHref, ETag: "srv-2", Data: mkICal(t, eventICS("x@test", "ServerEdit"))}
	srv.failDel[objHref] = caldav.ErrPreconditionFailed

	if _, err := sync.Sync(ctx, srv, st); err != nil {
		t.Fatal(err)
	}

	got, err := os.ReadFile(victim)
	if err != nil {
		t.Fatalf("reading victim: %v", err)
	}
	if string(got) != "ORIGINAL CONTENT" {
		t.Errorf("BUG: file outside the data dir was overwritten by sync:\n%s", got)
	}

	cal, ok := st.Calendar("personal")
	if ok {
		for _, r := range cal.Resources {
			if r.Name == escapeName {
				t.Errorf("BUG: in-memory index gained a resource whose name is a traversal path: %q", r.Name)
			}
		}
	}
}

// TestOrdinaryTombstoneFromSidecarStillPushes is the boundary sibling of the test
// above: the path-element validation must reject only names that could escape the
// calendar directory. An ordinary sidecar-loaded tombstone still has to push its
// server-side DELETE, or the guard would silently strand every pending delete
// that outlived the session which made it.
func TestOrdinaryTombstoneFromSidecarStillPushes(t *testing.T) {
	ctx := context.Background()

	dataDir := t.TempDir()
	calDir := filepath.Join(dataDir, "calendars", "personal")
	if err := os.MkdirAll(calDir, 0o700); err != nil {
		t.Fatal(err)
	}

	name := store.ResourceName("e1@test")
	objHref := calPath + name
	sidecar := map[string]any{
		"display_name": "Personal",
		"href":         calPath,
		"tombstones": map[string]any{
			name: map[string]string{"href": objHref, "etag": "srv-1"},
		},
	}
	data, err := json.Marshal(sidecar)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(calDir, ".lazyplanner.json"), data, 0o600); err != nil {
		t.Fatal(err)
	}

	st, err := store.Open(ctx, dataDir)
	if err != nil {
		t.Fatal(err)
	}
	if got := len(st.Tombstones()); got != 1 {
		t.Fatalf("ordinary tombstone dropped on load: %d tombstones, want 1", got)
	}

	srv := newFakeServer()
	srv.data[objHref] = caldav.Object{Path: objHref, ETag: "srv-1", Data: mkICal(t, eventICS("e1@test", "Base"))}

	res, err := sync.Sync(ctx, srv, st)
	if err != nil {
		t.Fatal(err)
	}
	if res.PushedDeletes != 1 {
		t.Errorf("PushedDeletes = %d, want 1 (skips: %v)", res.PushedDeletes, res.Skipped)
	}
	if _, still := srv.data[objHref]; still {
		t.Error("the pending delete never reached the server")
	}
	if got := len(st.Tombstones()); got != 0 {
		t.Errorf("tombstone not cleared after a successful push: %d", got)
	}
}
