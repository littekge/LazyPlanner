package sync_test

import (
	"context"
	"errors"
	"testing"

	"github.com/littekge/LazyPlanner/internal/caldav"
	"github.com/littekge/LazyPlanner/internal/store"
	"github.com/littekge/LazyPlanner/internal/sync"
)

// TestReadOnlyDegradedDownloadKeptVsDeleted closes the Pass-17 canary escape on
// reconcileReadOnly's degraded-download guard (`case !onServer && unfetched`).
// The read-write twin (degraded_download_deletion_test.go) was covered; the
// read-only path's equivalent guard had no test combining a read-only calendar
// with a degraded/partial download, so inverting the guard (unfetched ->
// !unfetched) escaped the suite.
//
// It exercises both sides of the guard on a read-only calendar at once:
//   - a previously-synced clean resource whose GET fails this pass (unfetched)
//     is still on the server → must be KEPT (not a deletion), and
//   - a previously-synced clean resource genuinely absent from the server
//     → must be Forgotten (a real deletion, not masked as degraded).
//
// Inverting the guard flips both outcomes, so either assertion catches it.
func TestReadOnlyDegradedDownloadKeptVsDeleted(t *testing.T) {
	ctx := context.Background()
	st := newStore(t)
	srv := newFakeServer()
	srv.cals[0].ReadOnly = true

	keptName := store.ResourceName("kept@test")
	goneName := store.ResourceName("gone@test")
	keptHref := calPath + keptName
	goneHref := calPath + goneName

	// Both start as clean, previously-synced local mirrors of server resources.
	if _, err := st.PutRemote(ctx, "personal", keptName, mkParsed(t, eventICS("kept@test", "Kept")), "k1", keptHref); err != nil {
		t.Fatal(err)
	}
	if _, err := st.PutRemote(ctx, "personal", goneName, mkParsed(t, eventICS("gone@test", "Gone")), "g1", goneHref); err != nil {
		t.Fatal(err)
	}

	// keptName still exists on the server but its GET fails this pass (degraded
	// download → unfetched). goneName is genuinely deleted server-side: it is not
	// in srv.data, so it is neither listed nor fetched (onServer=false,
	// unfetched=false).
	srv.data[keptHref] = caldav.Object{Path: keptHref, ETag: "k1", Data: mkICal(t, eventICS("kept@test", "Kept"))}
	srv.failDownload[calPath] = errors.New("bulk REPORT: connection reset")
	srv.getErr[keptHref] = errors.New("504 gateway timeout")

	res, err := sync.Sync(ctx, srv, st)
	if err != nil {
		t.Fatal(err)
	}

	// The unfetched resource must survive — a transient GET failure is not a
	// remote deletion.
	if findRes(t, st, keptName) == nil {
		t.Error("read-only kept@test was Forgotten after a transient fetch failure (false deletion / data loss); it still exists on the server")
	}
	// The genuinely-deleted resource must be Forgotten — a real deletion must not
	// be masked as a degraded download.
	if findRes(t, st, goneName) != nil {
		t.Error("read-only gone@test survived though it was deleted on the server (a real deletion leaked as degraded)")
	}
	if res.PulledDeletes != 1 {
		t.Errorf("PulledDeletes = %d, want 1 (only the genuinely-deleted resource)", res.PulledDeletes)
	}
}
