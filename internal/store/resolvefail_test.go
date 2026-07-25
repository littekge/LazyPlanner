package store_test

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/littekge/LazyPlanner/internal/store"
)

// A conflict resolution that fails to persist must not stand in memory: the
// resolved-but-unpersisted state discards the server's version, drops the
// conflict from the UI, and lets the next sync silently overwrite the server.
// Both halves of the class are covered here — a FAILED resolve reverts, and a
// SUCCESSFUL one still resolves fully (so the revert can't break the happy path).

// serverICS is the server's diverging version stashed by MarkConflict.
const serverICS = "BEGIN:VCALENDAR\r\nVERSION:2.0\r\nPRODID:-//Server//EN\r\n" +
	"BEGIN:VEVENT\r\nUID:e@test\r\nDTSTAMP:20260701T120000Z\r\n" +
	"DTSTART:20260704T130000Z\r\nDTEND:20260704T133000Z\r\n" +
	"SUMMARY:ServerEdit\r\nEND:VEVENT\r\nEND:VCALENDAR\r\n"

// sabotageSidecar replaces the calendar's sidecar file with a directory so the
// atomic rename in writeSidecar fails, while .ics writes still succeed.
func sabotageSidecar(t *testing.T, dir, calID string) {
	t.Helper()
	sc := filepath.Join(dir, "calendars", calID, ".lazyplanner.json")
	if err := os.Remove(sc); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(sc, 0o755); err != nil {
		t.Fatal(err)
	}
}

// conflictedStore seeds a synced resource, applies a local edit, and marks it
// conflicted against the given server version.
func conflictedStore(t *testing.T, dir string, serverData []byte, serverETag string, serverDeleted bool) (*store.Store, string) {
	t.Helper()
	ctx := context.Background()
	name := seedSyncedResource(t, dir, "cal1", "e@test", "Base")
	s, err := store.Open(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Put(ctx, "cal1", name, mustDecode(t, "e@test", "LocalEdit")); err != nil {
		t.Fatal(err)
	}
	if err := s.MarkConflict(ctx, "cal1", name, serverData, serverETag, serverDeleted); err != nil {
		t.Fatal(err)
	}
	if len(s.Conflicts()) != 1 {
		t.Fatalf("setup: Conflicts = %d, want 1", len(s.Conflicts()))
	}
	return s, name
}

// TestResolveKeepLocalFailureLeavesResolvedState: ResolveKeepLocal mutates
// in-memory state (adopts the server ETag, clears Conflicted, drops the conflict
// stash) before persisting the sidecar. When the sidecar write fails the call
// must revert — otherwise the in-memory conflict is gone and the resource carries
// the server's ETag, so the next sync's conditional PUT matches and silently
// overwrites the server's diverging version.
func TestResolveKeepLocalFailureLeavesResolvedState(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	s, name := conflictedStore(t, dir, []byte(serverICS), `"srv-2"`, false)

	sabotageSidecar(t, dir, "cal1")

	if err := s.ResolveKeepLocal(ctx, "cal1", name); err == nil {
		t.Fatal("setup: ResolveKeepLocal should fail when the sidecar can't be persisted")
	} else {
		t.Logf("ResolveKeepLocal returned (as expected): %v", err)
	}

	cal, _ := s.Calendar("cal1")
	r := findResource(cal, name)
	if r == nil {
		t.Fatal("resource vanished")
	}
	if got := len(s.Conflicts()); got != 1 {
		t.Errorf("Conflicts = %d, want 1 — a FAILED resolve dropped the stashed server version", got)
	}
	if r.ETag != `"srv-1"` {
		t.Errorf("ETag = %q, want the pre-conflict %q — a FAILED resolve adopted the server ETag, "+
			"so the next conditional PUT will silently overwrite the server", r.ETag, `"srv-1"`)
	}
	if !r.Conflicted {
		t.Error("Conflicted cleared by a FAILED resolve — sync will no longer skip this resource")
	}
	// The stash itself must survive intact, or a retry has nothing to keep.
	if got := string(s.Conflicts()[0].ServerData); got != serverICS {
		t.Errorf("stashed server version lost by a failed resolve: %q", got)
	}
}

// TestResolveKeepLocalServerDeletedFailureClearsHref: the ServerDeleted flavour
// additionally clears Href, so an un-reverted failure makes the next sync take
// the create path and upload a duplicate.
func TestResolveKeepLocalServerDeletedFailureClearsHref(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	s, name := conflictedStore(t, dir, nil, "", true)

	sabotageSidecar(t, dir, "cal1")

	if err := s.ResolveKeepLocal(ctx, "cal1", name); err == nil {
		t.Fatal("setup: ResolveKeepLocal should fail when the sidecar can't be persisted")
	}

	cal, _ := s.Calendar("cal1")
	r := findResource(cal, name)
	if r == nil {
		t.Fatal("resource vanished")
	}
	if r.Href == "" {
		t.Error("Href cleared by a FAILED resolve — next sync takes the create path and duplicates the item")
	}
	if !r.Conflicted {
		t.Error("Conflicted cleared by a FAILED resolve")
	}
	if len(s.Conflicts()) != 1 {
		t.Errorf("Conflicts = %d, want 1 after a failed resolve", len(s.Conflicts()))
	}
}

// TestResolveKeepServerFailureLeavesConflictIntact is the sibling half: keep-server
// writes the server version as the local .ics, so a failed sidecar write must roll
// back both the file and the conflict — leaving the user's local edit and a
// retryable conflict rather than a half-applied resolution.
func TestResolveKeepServerFailureLeavesConflictIntact(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	s, name := conflictedStore(t, dir, []byte(serverICS), `"srv-2"`, false)

	sabotageSidecar(t, dir, "cal1")

	if err := s.ResolveKeepServer(ctx, "cal1", name); err == nil {
		t.Fatal("setup: ResolveKeepServer should fail when the sidecar can't be persisted")
	}

	cal, _ := s.Calendar("cal1")
	r := findResource(cal, name)
	if r == nil {
		t.Fatal("resource vanished")
	}
	if !r.Conflicted || len(s.Conflicts()) != 1 {
		t.Errorf("after a failed keep-server: Conflicted=%v Conflicts=%d, want true/1",
			r.Conflicted, len(s.Conflicts()))
	}
	if r.Object.Events[0].Summary != "LocalEdit" {
		t.Errorf("in-memory content = %q, want the local edit kept", r.Object.Events[0].Summary)
	}
	local, err := os.ReadFile(filepath.Join(dir, "calendars", "cal1", name))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(local, []byte("SUMMARY:LocalEdit")) {
		t.Errorf("on-disk .ics was left holding the server version after a failed keep-server")
	}
}

// TestResolveKeepServerDeletionFailureLeavesConflictIntact: accepting a server
// deletion removes the local .ics; a failed sidecar write must restore it, or the
// user's edit is gone with no conflict left to retry.
func TestResolveKeepServerDeletionFailureLeavesConflictIntact(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	s, name := conflictedStore(t, dir, nil, "", true)

	sabotageSidecar(t, dir, "cal1")

	if err := s.ResolveKeepServer(ctx, "cal1", name); err == nil {
		t.Fatal("setup: ResolveKeepServer should fail when the sidecar can't be persisted")
	}
	cal, _ := s.Calendar("cal1")
	if r := findResource(cal, name); r == nil || !r.Conflicted {
		t.Errorf("resource = %+v, want the local copy restored and still conflicted", r)
	}
	if len(s.Conflicts()) != 1 {
		t.Errorf("Conflicts = %d, want 1 after a failed keep-server deletion", len(s.Conflicts()))
	}
	if _, err := os.Stat(filepath.Join(dir, "calendars", "cal1", name)); err != nil {
		t.Errorf("local .ics not restored after a failed keep-server deletion: %v", err)
	}
}

// TestResolveSuccessFullyResolves guards the other side of the class: the revert
// path must not fire on success. A resolve that persists clears the conflict in
// memory AND in the sidecar on disk, for both resolutions.
func TestResolveSuccessFullyResolves(t *testing.T) {
	for _, tc := range []struct {
		name    string
		resolve func(*store.Store, context.Context, string) error
	}{
		{"keep-local", func(s *store.Store, ctx context.Context, n string) error {
			return s.ResolveKeepLocal(ctx, "cal1", n)
		}},
		{"keep-server", func(s *store.Store, ctx context.Context, n string) error {
			return s.ResolveKeepServer(ctx, "cal1", n)
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			dir := t.TempDir()
			s, name := conflictedStore(t, dir, []byte(serverICS), `"srv-2"`, false)

			if err := tc.resolve(s, ctx, name); err != nil {
				t.Fatalf("resolve: %v", err)
			}
			if got := len(s.Conflicts()); got != 0 {
				t.Errorf("Conflicts = %d after a successful resolve, want 0", got)
			}
			cal, _ := s.Calendar("cal1")
			if r := findResource(cal, name); r == nil || r.Conflicted {
				t.Errorf("resource = %+v, want present and no longer conflicted", r)
			}
			// The sidecar on disk must agree — a reload must not resurrect it.
			raw, err := os.ReadFile(filepath.Join(dir, "calendars", "cal1", ".lazyplanner.json"))
			if err != nil {
				t.Fatal(err)
			}
			var sc struct {
				Resources map[string]map[string]json.RawMessage `json:"resources"`
			}
			if err := json.Unmarshal(raw, &sc); err != nil {
				t.Fatal(err)
			}
			if _, still := sc.Resources[name]["conflict"]; still {
				t.Error("sidecar still carries a conflict stash after a successful resolve")
			}
			s2, err := store.Open(ctx, dir)
			if err != nil {
				t.Fatal(err)
			}
			if got := len(s2.Conflicts()); got != 0 {
				t.Errorf("conflict reappeared after reload: %d", got)
			}
		})
	}
}
