package sync_test

import (
	"context"
	"testing"

	"github.com/littekge/LazyPlanner/internal/caldav"
	"github.com/littekge/LazyPlanner/internal/store"
	"github.com/littekge/LazyPlanner/internal/sync"
)

// undoRaceServer fires a hook the moment DeleteObject is called (before the 412
// is returned), standing in for the user pressing `u` to undo the delete during
// the network round-trip of the background sync's conditional DELETE.
type undoRaceServer struct {
	*fakeServer
	onDelete func()
}

func (s *undoRaceServer) DeleteObject(ctx context.Context, href, etag string) error {
	if s.onDelete != nil {
		hook := s.onDelete
		s.onDelete = nil // fire once
		hook()
	}
	return s.fakeServer.DeleteObject(ctx, href, etag)
}

// TestTombstone412ResurrectDoesNotClobberConcurrentUndo reproduces the HIGH
// finding: pushDelete's 412 branch resurrects the server version with an
// UNCONDITIONAL PutRemote. If the user undoes the delete during the DELETE's
// network round-trip, RestoreDirty re-creates the resource holding the user's
// content — and the unconditional PutRemote then clobbers it with the server
// version, losing the undone content from both the live copy and the conflict
// stash with no skip surfaced.
func TestTombstone412ResurrectDoesNotClobberConcurrentUndo(t *testing.T) {
	ctx := context.Background()
	st := newStore(t)
	srv := &undoRaceServer{fakeServer: newFakeServer()}

	name := store.ResourceName("r1@test")
	href := calPath + name

	// (1) Synced clean resource R: local content "MyContent" at ETag srv-1.
	if _, err := st.PutRemote(ctx, "personal", name, mkParsed(t, eventICS("r1@test", "MyContent")), "srv-1", href); err != nil {
		t.Fatal(err)
	}
	// (2) Server was edited independently -> srv-2 "ServerEdit"; the conditional
	//     DELETE (If-Match srv-1) will be refused with 412.
	srv.data[href] = caldav.Object{Path: href, ETag: "srv-2", Data: mkICal(t, eventICS("r1@test", "ServerEdit"))}
	srv.failDel[href] = caldav.ErrPreconditionFailed

	// Capture the pre-delete snapshot the UI stashes on the undo stack.
	loc, ok := st.Locate("r1@test")
	if !ok {
		t.Fatal("resource not locatable before delete")
	}
	prev := loc.Prev

	// (3) User deletes R locally -> tombstone{name, href, srv-1}.
	if err := st.Delete(ctx, "personal", name); err != nil {
		t.Fatal(err)
	}

	// (4) Mid-DELETE, the user presses `u`: undoLast -> RestoreDirty re-creates R
	//     as a dirty resource holding "MyContent" and clears the tombstone.
	srv.onDelete = func() {
		if _, err := st.RestoreDirty(ctx, "personal", name, prev); err != nil {
			t.Errorf("undo RestoreDirty failed: %v", err)
		}
	}

	// (5) Background periodic sync runs the tombstone push -> pushDelete -> 412.
	res, err := sync.Sync(ctx, srv, st)
	if err != nil {
		t.Fatal(err)
	}

	final := findRes(t, st, name)
	if final == nil {
		t.Fatalf("BUG: the undone resource vanished entirely; Conflicts=%d Skipped=%d", res.Conflicts, len(res.Skipped))
	}
	if got := final.Object.Events[0].Summary; got != "MyContent" {
		t.Fatalf("BUG: the user's undone content was clobbered by the 412 resurrect: summary = %q, want %q (Conflicts=%d Skipped=%d)",
			got, "MyContent", res.Conflicts, len(res.Skipped))
	}
}
