package sync_test

import (
	"context"
	"testing"

	"github.com/littekge/LazyPlanner/internal/caldav"
	"github.com/littekge/LazyPlanner/internal/store"
	"github.com/littekge/LazyPlanner/internal/sync"
)

// TestReproStepAForgetClobbersConcurrentEdit reproduces the HIGH finding:
// reconcile step (A)'s remote-delete Forget branch is unguarded, so it clobbers
// a concurrent local edit that lands during the preceding push's network window
// (a silent lost update).
//
// Setup mirrors the failure scenario:
//   - "a@test" is a synced resource; a local edit makes it dirty so step (A)
//     issues a PutObject for it (the network window / onPut hook).
//   - "b@test" is a synced clean resource that has been DELETED on the server
//     (absent from the download), so step (A) will take its `!onServer` branch.
//   - During A's PUT, the user edits "b@test": cs.resources[b] is replaced with a
//     NEW dirty *Resource. The loop still holds the pre-loop snapshot's CLEAN
//     pointer for b, so `r.Dirty` reads false and it takes the plain `!onServer`
//     branch, calling st.Forget(personal, b) — deleting the user's edit and
//     leaving a tombstone.
func TestReproStepAForgetClobbersConcurrentEdit(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()

	aName := store.ResourceName("a@test")
	bName := store.ResourceName("b@test")
	aHref := calPath + aName
	bHref := calPath + bName

	st, err := store.Open(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.SetCalendarMeta(ctx, "personal",
		store.CalendarMeta{DisplayName: "Personal", Href: calPath}); err != nil {
		t.Fatal(err)
	}
	// Seed both resources as clean/synced with a server identity.
	if _, err := st.PutRemote(ctx, "personal", aName,
		mkParsed(t, eventICS("a@test", "A-Original")), "a-1", aHref); err != nil {
		t.Fatal(err)
	}
	if _, err := st.PutRemote(ctx, "personal", bName,
		mkParsed(t, eventICS("b@test", "B-Original")), "b-1", bHref); err != nil {
		t.Fatal(err)
	}

	// Local edit to A → dirty; step (A) will push it (server ETag still matches).
	if _, err := st.Put(ctx, "personal", aName,
		mkParsed(t, eventICS("a@test", "A-Edited"))); err != nil {
		t.Fatal(err)
	}

	srv := newFakeServer()
	// The server holds A but NOT B — B was deleted remotely.
	srv.data[aHref] = caldav.Object{Path: aHref, ETag: "a-1", Data: mkICal(t, eventICS("a@test", "A-Original"))}

	// While A's PUT is in flight, the user edits B (replaces its *Resource).
	srv.onPut = func() {
		srv.onPut = nil // once
		if _, err := st.Put(ctx, "personal", bName,
			mkParsed(t, eventICS("b@test", "B-EditedByUser"))); err != nil {
			t.Fatal(err)
		}
	}

	res, err := sync.Sync(ctx, srv, st)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("SyncResult: PulledDeletes=%d Conflicts=%d Skipped=%d Pushed=%d",
		res.PulledDeletes, res.Conflicts, len(res.Skipped), res.Pushed)

	r := findRes(t, st, bName)
	if r == nil {
		t.Fatalf("data loss: b@test was Forgotten (removed) despite a concurrent local edit; "+
			"SyncResult PulledDeletes=%d Conflicts=%d — the edit is gone with no conflict/skip",
			res.PulledDeletes, res.Conflicts)
	}
	got := r.Object.Events[0].Summary
	t.Logf("after sync: b summary=%q dirty=%v", got, r.Dirty)
	if got != "B-EditedByUser" || !r.Dirty {
		t.Fatalf("b clobbered: summary=%q dirty=%v; want %q dirty=true (the concurrent edit must survive and stay pending)",
			got, r.Dirty, "B-EditedByUser")
	}
}
