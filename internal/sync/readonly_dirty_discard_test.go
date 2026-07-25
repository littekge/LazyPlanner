package sync_test

import (
	"context"
	"testing"

	"github.com/littekge/LazyPlanner/internal/caldav"
	"github.com/littekge/LazyPlanner/internal/store"
	"github.com/littekge/LazyPlanner/internal/sync"
)

// TestReadOnlyDiscardsSyncedThenEditedResource closes the Pass-22 canary escape on
// reconcileReadOnly's dirty-discard guard (`if r.Dirty || r.Href == ""`). The only
// existing test used a never-synced (Href=="" AND Dirty) local add, which satisfies
// both the OR and an &&-weakened variant — so weakening `||` to `&&` shipped
// undetected. The case the OR exists for is a *synced-then-edited* resource
// (Dirty AND Href!=""): a local edit to an item on a read-only calendar can never
// be pushed, so the read-only invariant requires it be discarded. Under the &&
// mutation such an edit is silently KEPT (un-pushable local change stranded on a
// read-only calendar) — this test fails under that mutation and passes on correct
// code.
func TestReadOnlyDiscardsSyncedThenEditedResource(t *testing.T) {
	ctx := context.Background()
	st := newStore(t)
	srv := newFakeServer()
	srv.cals[0].ReadOnly = true

	name := store.ResourceName("e1@test")
	href := calPath + name

	// A previously-synced clean mirror of a server resource (Href + ETag set)...
	if _, err := st.PutRemote(ctx, "personal", name, mkParsed(t, eventICS("e1@test", "Original")), "srv-1", href); err != nil {
		t.Fatal(err)
	}
	// ...then locally edited → Dirty == true with Href still set. This is the
	// OR-covered case the never-synced-only test misses.
	if _, err := st.Put(ctx, "personal", name, mkParsed(t, eventICS("e1@test", "LocalEdit"))); err != nil {
		t.Fatal(err)
	}
	// The resource still exists on the server.
	srv.data[href] = caldav.Object{Path: href, ETag: "srv-1", Data: mkICal(t, eventICS("e1@test", "Original"))}

	if _, err := sync.Sync(ctx, srv, st); err != nil {
		t.Fatal(err)
	}

	// A read-only calendar can never keep a local change. The dirty edit must be
	// discarded — the resource is either Forgotten (then re-pulled clean from the
	// server) or gone, but it must NOT remain a dirty local edit.
	if r := findRes(t, st, name); r != nil && r.Dirty {
		t.Fatal("read-only calendar kept an un-pushable local edit (Dirty) — the dirty-discard guard is not firing for a synced-then-edited resource")
	}
}
