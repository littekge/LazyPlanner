package store_test

import (
	"context"
	"sync"
	"testing"

	"github.com/littekge/LazyPlanner/internal/store"
)

// TestCommitPushDoesNotResurrectDeletedResource reproduces the mid-push delete
// race for a SYNCED resource (the pushUpdate path): the background sync goroutine
// PUT the resource, the user deletes it (leaving a tombstone) while the PUT is in
// flight, and the PUT's CommitPush must NOT resurrect the resource or wipe the
// tombstone — the deletion must survive and still be pushed on the next sync, with
// its ETag advanced to the server's post-PUT value so the conditional DELETE matches.
func TestCommitPushDoesNotResurrectDeletedResource(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	name := seedSyncedResource(t, dir, "cal1", "synced@test", "Synced")

	s, err := store.Open(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}

	// The sync goroutine captured this snapshot and PUT it to the server.
	loc, ok := s.Locate("synced@test")
	if !ok {
		t.Fatal("resource not located")
	}
	pushed := loc.Prev
	href := pushed.Href

	// Mid-push: the user presses delete on the event loop. A tombstone is left.
	if err := s.Delete(ctx, "cal1", name); err != nil {
		t.Fatal(err)
	}
	if len(s.Tombstones()) != 1 {
		t.Fatalf("expected a tombstone after delete, got %d", len(s.Tombstones()))
	}

	// The PUT returns; sync finalizes it with the server's new ETag.
	if _, err := s.CommitPush(ctx, "cal1", name, pushed, `"srv-2"`, href); err != nil {
		t.Fatal(err)
	}

	// The deletion must not be lost.
	ts := s.Tombstones()
	if len(ts) != 1 {
		t.Fatalf("CommitPush wiped the tombstone of a mid-push deletion: got %d tombstones, want 1 (deletion silently lost)", len(ts))
	}
	// The ETag baseline must advance to what our PUT put on the server, or the
	// next sync's conditional DELETE (If-Match) fails with a 412.
	if ts[0].ETag != `"srv-2"` {
		t.Errorf("tombstone ETag = %q, want %q (must track the post-PUT server state)", ts[0].ETag, `"srv-2"`)
	}
	cal, ok := s.Calendar("cal1")
	if !ok {
		t.Fatal("calendar cal1 missing")
	}
	if r := findResource(cal, name); r != nil {
		t.Errorf("CommitPush resurrected the deleted resource %s (Dirty=%v, ETag=%q)", name, r.Dirty, r.ETag)
	}
}

// TestCommitPushHonorsDeleteOfNeverSyncedCreate reproduces the mid-push delete
// race for the pushCreate path: a brand-new local resource (Href=="") is created
// on the server by the sync goroutine, and the user deletes it while the create-PUT
// is in flight. Deleting a never-synced resource leaves NO tombstone (there was no
// server identity yet) — but our PUT just created it on the server, so CommitPush
// must SCHEDULE a deletion (create a tombstone) or the next sync re-pulls the
// server copy and silently resurrects the item.
func TestCommitPushHonorsDeleteOfNeverSyncedCreate(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()

	s, err := store.Open(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}

	// A never-synced local create: Dirty, no Href.
	obj := mustDecode(t, "created@test", "Created")
	name := store.ResourceName("created@test")
	if _, err := s.Put(ctx, "cal1", name, obj); err != nil {
		t.Fatal(err)
	}
	loc, ok := s.Locate("created@test")
	if !ok {
		t.Fatal("resource not located")
	}
	pushed := loc.Prev
	if pushed.Href != "" {
		t.Fatalf("precondition: a fresh create must have no Href, got %q", pushed.Href)
	}

	// Mid-push: the user deletes it. No tombstone (never had a server identity).
	if err := s.Delete(ctx, "cal1", name); err != nil {
		t.Fatal(err)
	}
	if len(s.Tombstones()) != 0 {
		t.Fatalf("a never-synced delete should leave no tombstone, got %d", len(s.Tombstones()))
	}

	// The create-PUT returns with the server-assigned href/ETag.
	href := "/dav/cal1/" + name
	if _, err := s.CommitPush(ctx, "cal1", name, pushed, `"srv-new"`, href); err != nil {
		t.Fatal(err)
	}

	// The just-created server resource must be scheduled for deletion, or it comes
	// back on the next pull.
	ts := s.Tombstones()
	if len(ts) != 1 {
		t.Fatalf("CommitPush did not schedule deletion of the just-created server resource: got %d tombstones, want 1 (resource will be resurrected on next pull)", len(ts))
	}
	if ts[0].Href != href || ts[0].ETag != `"srv-new"` {
		t.Errorf("tombstone = {Href:%q ETag:%q}, want {Href:%q ETag:%q}", ts[0].Href, ts[0].ETag, href, `"srv-new"`)
	}
	cal, ok := s.Calendar("cal1")
	if !ok {
		t.Fatal("calendar cal1 missing")
	}
	if r := findResource(cal, name); r != nil {
		t.Errorf("CommitPush resurrected the deleted resource %s", name)
	}
}

// TestCommitPushDeleteRaceInvariant runs the mid-push delete concurrently with
// CommitPush many times. Whichever order the two land in, the end state must be
// identical: exactly one tombstone (the deletion is never silently lost) and no
// resurrected resource. Run under -race to also exercise the store's locking.
func TestCommitPushDeleteRaceInvariant(t *testing.T) {
	ctx := context.Background()
	for i := 0; i < 200; i++ {
		dir := t.TempDir()
		name := seedSyncedResource(t, dir, "cal1", "synced@test", "Synced")
		s, err := store.Open(ctx, dir)
		if err != nil {
			t.Fatal(err)
		}
		loc, ok := s.Locate("synced@test")
		if !ok {
			t.Fatal("resource not located")
		}
		pushed := loc.Prev
		href := pushed.Href

		var wg sync.WaitGroup
		wg.Add(2)
		go func() {
			defer wg.Done()
			_ = s.Delete(ctx, "cal1", name)
		}()
		go func() {
			defer wg.Done()
			_, _ = s.CommitPush(ctx, "cal1", name, pushed, `"srv-2"`, href)
		}()
		wg.Wait()

		if got := len(s.Tombstones()); got != 1 {
			t.Fatalf("iter %d: tombstones = %d, want 1 (deletion lost or duplicated under the race)", i, got)
		}
		cal, ok := s.Calendar("cal1")
		if !ok {
			t.Fatalf("iter %d: calendar missing", i)
		}
		if r := findResource(cal, name); r != nil {
			t.Fatalf("iter %d: resource resurrected under the race (Dirty=%v)", i, r.Dirty)
		}
	}
}
