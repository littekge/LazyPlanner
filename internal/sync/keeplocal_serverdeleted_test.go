package sync_test

import (
	"context"
	"testing"

	"github.com/littekge/LazyPlanner/internal/caldav"
	"github.com/littekge/LazyPlanner/internal/store"
	"github.com/littekge/LazyPlanner/internal/sync"
)

// TestKeepLocalServerDeletedConverges guards the fix for the non-convergence
// bug: keep-local resolution of a server-deleted conflict used to leave the
// resource's Href non-empty, so the next reconcile re-hit the !onServer && Dirty
// branch and re-raised the identical conflict every sync. It must instead
// re-create the item on the server (create path) and stop re-raising it.
func TestKeepLocalServerDeletedConverges(t *testing.T) {
	ctx := context.Background()
	st := newStore(t)
	srv := newFakeServer()

	name := store.ResourceName("kl@test")
	href := calPath + name

	// Synced resource present on both sides.
	if _, err := st.PutRemote(ctx, "personal", name, mkParsed(t, eventICS("kl@test", "Original")), "srv-1", href); err != nil {
		t.Fatal(err)
	}
	srv.data[href] = caldav.Object{Path: href, ETag: "srv-1", Data: mkICal(t, eventICS("kl@test", "Original"))}

	// Local edit (Dirty), then the server deletes the resource.
	if _, err := st.Put(ctx, "personal", name, mkParsed(t, eventICS("kl@test", "LocalEdit"))); err != nil {
		t.Fatal(err)
	}
	delete(srv.data, href)

	// First sync: edited-locally + gone-on-server => server-deleted conflict.
	if _, err := sync.Sync(ctx, srv, st); err != nil {
		t.Fatal(err)
	}
	if got := len(st.Conflicts()); got != 1 {
		t.Fatalf("first sync: conflicts = %d, want 1 (server-deleted)", got)
	}

	// User chooses keep-local: keep my edit, put it back on the server.
	if err := st.ResolveKeepLocal(ctx, "personal", name); err != nil {
		t.Fatal(err)
	}
	if got := len(st.Conflicts()); got != 0 {
		t.Fatalf("after resolve: conflicts = %d, want 0", got)
	}

	// Second sync should push the kept-local version back to the server and
	// converge. BUG: it re-raises the same server-deleted conflict instead.
	if _, err := sync.Sync(ctx, srv, st); err != nil {
		t.Fatal(err)
	}
	if got := len(st.Conflicts()); got != 0 {
		t.Fatalf("second sync: conflicts = %d, want 0 (keep-local should have converged, re-creating the item on the server)", got)
	}
	if len(srv.data) == 0 {
		t.Fatalf("second sync: the kept-local item was never re-created on the server")
	}
}
