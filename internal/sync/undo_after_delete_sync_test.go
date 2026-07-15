package sync_test

import (
	"context"
	"testing"

	"github.com/littekge/LazyPlanner/internal/caldav"
	"github.com/littekge/LazyPlanner/internal/store"
	"github.com/littekge/LazyPlanner/internal/sync"
)

// TestUndoOfSyncedDeleteSurvivesNextSync guards pass-12 HIGH #2: undoing a delete
// AFTER the delete's tombstone has already synced must not let the resurrected
// item silently vanish on the next sync.
//
// Timeline:
//  1. Synced clean resource R ("Keep me", Dirty=false, Href set, ETag "srv-1").
//  2. User deletes R -> a tombstone is queued; the UI stashes the pre-delete
//     snapshot on the undo stack.
//  3. Debounced push fires -> sync DELETEs R on the server (tombstone cleared).
//  4. User presses u -> undoLast restores the snapshot via RestoreDirty (marked
//     Dirty so the resurrection is a pending local change).
//  5. Next sync: R is Dirty with an Href the server no longer has -> reconcile
//     keeps it as a conflict (server-deleted) instead of Forgetting it clean.
//
// The bug: a clean Restore (Dirty=false) makes step 5 hit `case !onServer:` and
// Forget the resurrected item — the thing the user explicitly restored is gone
// from both store and server, permanently.
func TestUndoOfSyncedDeleteSurvivesNextSync(t *testing.T) {
	ctx := context.Background()
	st := newStore(t)
	srv := newFakeServer()

	name := store.ResourceName("d1@test")
	href := calPath + name

	// (1) Synced clean copy on both sides at ETag srv-1.
	if _, err := st.PutRemote(ctx, "personal", name, mkParsed(t, eventICS("d1@test", "Keep me")), "srv-1", href); err != nil {
		t.Fatal(err)
	}
	srv.data[href] = caldav.Object{Path: href, ETag: "srv-1", Data: mkICal(t, eventICS("d1@test", "Keep me"))}

	// Capture the pre-delete snapshot the UI stashes on the undo stack.
	loc, ok := st.Locate("d1@test")
	if !ok {
		t.Fatal("resource not locatable before delete")
	}
	prev := loc.Prev

	// (2) User deletes R (leaves a tombstone), then (3) the push syncs the delete.
	if err := st.Delete(ctx, "personal", name); err != nil {
		t.Fatal(err)
	}
	if _, err := sync.Sync(ctx, srv, st); err != nil {
		t.Fatal(err)
	}
	if _, stillThere := srv.data[href]; stillThere {
		t.Fatalf("precondition failed: server still has %s after the delete synced", href)
	}

	// (4) User presses u -> undoLast resurrects via RestoreDirty.
	if _, err := st.RestoreDirty(ctx, "personal", name, prev); err != nil {
		t.Fatal(err)
	}

	// (5) Next sync (the one undoLast arms via scheduleSyncDebounced).
	if _, err := sync.Sync(ctx, srv, st); err != nil {
		t.Fatal(err)
	}

	// The explicitly-restored item must survive (kept as a conflict), not be
	// Forgotten as a clean server-absent orphan.
	final := findRes(t, st, name)
	if final == nil {
		t.Fatalf("BUG: the undone (resurrected) item was silently dropped on the next sync")
	}
	if got := final.Object.Events[0].Summary; got != "Keep me" {
		t.Fatalf("resurrected item content changed: summary = %q, want Keep me", got)
	}
	if !final.Conflicted {
		t.Errorf("resurrected-after-server-delete item should be flagged conflicted for the user to resolve; Conflicted=false")
	}
}
