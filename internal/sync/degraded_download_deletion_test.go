package sync_test

import (
	"context"
	"errors"
	"testing"

	"github.com/littekge/LazyPlanner/internal/caldav"
	"github.com/littekge/LazyPlanner/internal/store"
	sync "github.com/littekge/LazyPlanner/internal/sync"
)

// When the bulk REPORT fails, sync falls back to a per-resource GET. A resource
// whose GET fails transiently is still listed on the server, so it must NOT be
// mistaken for a remote deletion. These guard the Pass-13 HIGH: a degraded
// download was Forgetting a clean local item (and raising a false ServerDeleted
// conflict on a dirty one) that still existed on the server — silent data loss.

// setupDegraded seeds one good + one bad resource locally and on the server, then
// forces the degraded path with the bad resource's GET failing.
func setupDegraded(t *testing.T, st *store.Store, srv *fakeServer, badDirty bool) (goodName, badName string) {
	t.Helper()
	ctx := context.Background()
	goodName = store.ResourceName("good@test")
	badName = store.ResourceName("bad@test")
	goodHref := calPath + goodName
	badHref := calPath + badName

	if _, err := st.PutRemote(ctx, "personal", goodName, mkParsed(t, eventICS("good@test", "Good")), "g1", goodHref); err != nil {
		t.Fatal(err)
	}
	if _, err := st.PutRemote(ctx, "personal", badName, mkParsed(t, eventICS("bad@test", "Bad")), "b1", badHref); err != nil {
		t.Fatal(err)
	}
	if badDirty {
		// A pending local edit on the bad resource: it must not become a false
		// ServerDeleted conflict when its GET fails.
		if _, err := st.Put(ctx, "personal", badName, mkParsed(t, eventICS("bad@test", "LocalEdit"))); err != nil {
			t.Fatal(err)
		}
	}
	srv.data[goodHref] = caldav.Object{Path: goodHref, ETag: "g1", Data: mkICal(t, eventICS("good@test", "Good"))}
	srv.data[badHref] = caldav.Object{Path: badHref, ETag: "b1", Data: mkICal(t, eventICS("bad@test", "Bad"))}

	srv.failDownload[calPath] = errors.New("bulk REPORT: connection reset")
	srv.getErr[badHref] = errors.New("504 gateway timeout")
	return goodName, badName
}

func TestDegradedDownloadNotTreatedAsDeletion(t *testing.T) {
	ctx := context.Background()
	st := newStore(t)
	srv := newFakeServer()
	_, badName := setupDegraded(t, st, srv, false)

	res, err := sync.Sync(ctx, srv, st)
	if err != nil {
		t.Fatal(err)
	}
	if res.PulledDeletes != 0 {
		t.Errorf("PulledDeletes = %d, want 0 (bad.ics still exists on the server; the GET just failed)", res.PulledDeletes)
	}
	r := findRes(t, st, badName)
	if r == nil {
		t.Fatal("bad.ics was Forgotten after a transient fetch failure (data loss); it still exists on the server")
	}
	if r.Conflicted {
		t.Error("bad.ics was wrongly flagged conflicted; a clean unfetched resource should be left untouched")
	}
}

func TestDegradedDownloadDirtyResourceNotFalselyConflicted(t *testing.T) {
	ctx := context.Background()
	st := newStore(t)
	srv := newFakeServer()
	_, badName := setupDegraded(t, st, srv, true)

	if _, err := sync.Sync(ctx, srv, st); err != nil {
		t.Fatal(err)
	}
	r := findRes(t, st, badName)
	if r == nil {
		t.Fatal("dirty bad.ics vanished after a transient fetch failure")
	}
	if r.Conflicted {
		t.Error("dirty bad.ics raised a false ServerDeleted conflict; its fetch merely failed, the server copy still exists")
	}
	if !r.Dirty {
		t.Error("the pending local edit was cleared; it should remain dirty to push next sync")
	}
	if got := r.Object.Events[0].Summary; got != "LocalEdit" {
		t.Errorf("local edit lost: summary = %q, want LocalEdit", got)
	}
}

// TestDegradedDownloadDoesNotCacheCTagSoNextSyncRetries: a calendar with a
// per-resource skip must not cache its CTag, so the next sync re-downloads and
// picks up the resource whose fetch failed (rather than short-circuiting forever).
// Also the sync-side guard for canary escape #2 (CTag cached after a failed reconcile).
func TestDegradedDownloadDoesNotCacheCTagSoNextSyncRetries(t *testing.T) {
	ctx := context.Background()
	st := newStore(t)
	srv := newFakeServer()
	srv.cals = []caldav.Calendar{{Path: calPath, Name: "Personal", CTag: "ctag-1"}}
	_, badName := setupDegraded(t, st, srv, false)

	if _, err := sync.Sync(ctx, srv, st); err != nil {
		t.Fatal(err)
	}
	if got := st.CalendarCTag("personal"); got == "ctag-1" {
		t.Fatal("CTag was cached despite a per-resource skip; the next sync would short-circuit and never retry the failed resource")
	}

	// The transient failure clears; the next sync must actually re-download
	// (no short-circuit) because the CTag was never cached.
	delete(srv.getErr, calPath+badName)
	delete(srv.failDownload, calPath)
	res2, err := sync.Sync(ctx, srv, st)
	if err != nil {
		t.Fatal(err)
	}
	if res2.CalendarsUnchanged != 0 {
		t.Errorf("second sync short-circuited (CalendarsUnchanged=%d); it must re-download after a degraded pass", res2.CalendarsUnchanged)
	}
	if findRes(t, st, badName) == nil {
		t.Error("bad.ics still missing after the retry sync")
	}
}
