package store_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/littekge/LazyPlanner/internal/store"
)

// TestCommitPushDoesNotClobberConcurrentEdit is the store-side guard for the
// "concurrent writes are version-checked" invariant on CommitPush's mid-push EDIT
// branch — the sibling of the mid-push DELETE branch covered by
// commitpush_deletemidpush_test.go.
//
// Sync PUTs a snapshot on a background goroutine; the user edits the same resource
// on the event loop before the PUT returns. CommitPush distinguishes the two cases
// by POINTER IDENTITY (every mutation swaps in a fresh *Resource), so it must keep
// the newer edit — dirty, with the ETag baseline advanced to the server's post-PUT
// value — instead of writing the stale pushed snapshot back as clean.
//
// It lives in package store because the only test in the repo that caught a
// weakened identity check was internal/sync's TestSyncPushDoesNotClobberConcurrent-
// Edit, one package away: an inner-loop `go test ./internal/store/` reported green
// on a silent lost update.
func TestCommitPushDoesNotClobberConcurrentEdit(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	const uid = "synced@test"
	name := seedSyncedResource(t, dir, "cal1", uid, "Synced")

	s, err := store.Open(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}

	// The sync goroutine captured this snapshot and PUT it to the server.
	loc, ok := s.Locate(uid)
	if !ok {
		t.Fatal("resource not located")
	}
	pushed := loc.Prev
	href := pushed.Href

	// Mid-push: the user edits on the event loop. This swaps in a fresh *Resource,
	// so `pushed` is now a stale snapshot.
	edited := mustDecode(t, uid, "Edited mid-push")
	applied, err := s.PutIfUnchanged(ctx, "cal1", name, edited, pushed)
	if err != nil {
		t.Fatal(err)
	}
	if !applied {
		t.Fatal("precondition: the mid-push edit must apply (nothing else changed the resource)")
	}

	// The PUT returns; sync finalizes it with the server's new ETag.
	if _, err := s.CommitPush(ctx, "cal1", name, pushed, `"srv-2"`, href); err != nil {
		t.Fatal(err)
	}

	cal, ok := s.Calendar("cal1")
	if !ok {
		t.Fatal("calendar cal1 missing")
	}
	r := findResource(cal, name)
	if r == nil {
		t.Fatal("resource vanished after CommitPush")
	}
	ev := findEvt(r.Object, uid)
	if ev == nil {
		t.Fatal("event missing from the committed object")
	}
	if ev.Summary != "Edited mid-push" {
		t.Errorf("in-memory SUMMARY = %q, want %q — CommitPush wrote the stale pushed snapshot back over the concurrent edit (silent lost update)", ev.Summary, "Edited mid-push")
	}
	if !r.Dirty {
		t.Error("resource is clean after CommitPush; the concurrent edit was never pushed to the server and is now indistinguishable from synced state")
	}
	// The baseline must advance to what our PUT left on the server, or the next
	// push's If-Match fails with a 412.
	if r.ETag != `"srv-2"` {
		t.Errorf("ETag = %q, want %q (baseline must track the post-PUT server state)", r.ETag, `"srv-2"`)
	}
	if r.Href != href {
		t.Errorf("Href = %q, want %q", r.Href, href)
	}

	// The cache file is the local source of truth, so the clobber must not reach
	// disk either.
	b, err := os.ReadFile(filepath.Join(dir, "calendars", "cal1", name))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), "Edited mid-push") {
		t.Errorf("on-disk .ics does not carry the concurrent edit:\n%s", b)
	}
}

// TestCommitPushEditRaceInvariant runs the mid-push edit concurrently with
// CommitPush many times — the edit sibling of TestCommitPushDeleteRaceInvariant.
// Whichever order the two land in, one invariant must hold: an edit the store
// ACCEPTED is never silently reverted. (If CommitPush wins the race first, the
// edit's expectedPrev no longer matches and PutIfUnchanged cleanly skips it —
// applied=false — which the UI surfaces as a retry, not a lost update.) Run under
// -race to also exercise the store's locking.
//
// Scheduling decides which order each iteration takes, so this test is a locking
// and invariant check, NOT the guard that kills a weakened identity check —
// TestCommitPushDoesNotClobberConcurrentEdit above is, deterministically. The
// applied count is logged so a run where the interleaving never happened is
// visible rather than silently vacuous.
func TestCommitPushEditRaceInvariant(t *testing.T) {
	ctx := context.Background()
	const uid = "synced@test"
	appliedCount := 0
	for i := 0; i < 200; i++ {
		dir := t.TempDir()
		name := seedSyncedResource(t, dir, "cal1", uid, "Synced")
		s, err := store.Open(ctx, dir)
		if err != nil {
			t.Fatal(err)
		}
		loc, ok := s.Locate(uid)
		if !ok {
			t.Fatal("resource not located")
		}
		pushed := loc.Prev
		href := pushed.Href

		// Decode outside the goroutine: doing it inside makes the edit reliably lose
		// the race, so the interesting interleaving would never be exercised.
		edited := mustDecode(t, uid, "Edited mid-push")
		start := make(chan struct{})
		var applied bool
		editFirst := func() {
			applied, _ = s.PutIfUnchanged(ctx, "cal1", name, edited, pushed)
		}
		commit := func() {
			_, _ = s.CommitPush(ctx, "cal1", name, pushed, `"srv-2"`, href)
		}
		// Alternate which side is launched first: a fixed launch order lets one side
		// win the store lock every single time, leaving half the interleaving space
		// (and, with it, the invariant this test exists for) unexercised.
		first, second := editFirst, commit
		if i%2 == 1 {
			first, second = commit, editFirst
		}
		var wg sync.WaitGroup
		wg.Add(2)
		go func() {
			defer wg.Done()
			<-start
			first()
		}()
		go func() {
			defer wg.Done()
			<-start
			second()
		}()
		close(start)
		wg.Wait()

		if !applied {
			continue // the edit was cleanly rejected; nothing was promised to the user
		}
		appliedCount++
		cal, ok := s.Calendar("cal1")
		if !ok {
			t.Fatalf("iter %d: calendar missing", i)
		}
		r := findResource(cal, name)
		if r == nil {
			t.Fatalf("iter %d: resource vanished", i)
		}
		ev := findEvt(r.Object, uid)
		if ev == nil {
			t.Fatalf("iter %d: event missing from the resource after the race", i)
		}
		if ev.Summary != "Edited mid-push" {
			t.Fatalf("iter %d: accepted edit was reverted under the race (SUMMARY=%q, Dirty=%v)", i, ev.Summary, r.Dirty)
		}
		if !r.Dirty {
			t.Fatalf("iter %d: accepted edit left clean under the race; it will never be pushed", i)
		}
	}
	t.Logf("edit won the race in %d/200 iterations", appliedCount)
}
