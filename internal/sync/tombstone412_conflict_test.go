package sync_test

import (
	"context"
	"errors"
	"testing"

	"github.com/littekge/LazyPlanner/internal/caldav"
	"github.com/littekge/LazyPlanner/internal/store"
	"github.com/littekge/LazyPlanner/internal/sync"
)

// TestTombstone412DegradedDownloadKeepsConflict guards the fix for the silent
// conflict-swallow: a delete-vs-server-change conflict where the conditional
// DELETE returns 412 but the server version is unavailable this pass (degraded
// download: bulk failed, individual GET failed, so serverByHref lacks the href).
// The 412 handler used to skip the resurrect/flag block yet still clear the
// tombstone — silently resolving the delete with no conflict recorded. It must
// now keep the tombstone (and skip) so the next full sync retries.
func TestTombstone412DegradedDownloadKeepsConflict(t *testing.T) {
	ctx := context.Background()
	st := newStore(t)
	srv := newFakeServer()

	name := store.ResourceName("e1@test")
	href := calPath + name
	if _, err := st.PutRemote(ctx, "personal", name, mkParsed(t, eventICS("e1@test", "Base")), "srv-1", href); err != nil {
		t.Fatal(err)
	}
	// Server has a changed version; the conditional DELETE will be refused (412).
	srv.data[href] = caldav.Object{Path: href, ETag: "srv-2", Data: mkICal(t, eventICS("e1@test", "ServerEdit"))}
	srv.failDel[href] = caldav.ErrPreconditionFailed
	// Degraded download: bulk fails, listing succeeds (so href is known/still on
	// server), but the individual GET of this resource fails this pass — so
	// serverByHref lacks the href.
	srv.failDownload[calPath] = errors.New("bulk boom")
	srv.getErr[href] = errors.New("get boom")

	if err := st.Delete(ctx, "personal", name); err != nil {
		t.Fatal(err)
	}

	res, err := sync.Sync(ctx, srv, st)
	if err != nil {
		t.Fatal(err)
	}

	// The delete lost a race to a real server change. That MUST NOT be silently
	// resolved: either a conflict is flagged, or the tombstone survives to retry.
	tombstones := st.Tombstones()
	if res.Conflicts == 0 && len(tombstones) == 0 {
		t.Fatalf("BUG: delete-vs-server-change silently swallowed — Conflicts=%d, tombstones=%d (want a conflict flagged OR the tombstone retained)", res.Conflicts, len(tombstones))
	}
}
