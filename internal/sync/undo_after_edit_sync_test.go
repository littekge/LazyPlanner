package sync_test

import (
	"context"
	"testing"

	"github.com/littekge/LazyPlanner/internal/caldav"
	"github.com/littekge/LazyPlanner/internal/store"
	"github.com/littekge/LazyPlanner/internal/sync"
)

// TestUndoOfSyncedEditSurvivesNextSync reproduces the MED finding: undoing an
// edit AFTER that edit has already synced to the server silently loses the
// user's revert on the next sync (the server copy is pulled back over the undo).
//
// Timeline:
//  1. Synced clean resource R ("Original", Dirty=false, Href set, ETag "srv-1").
//  2. User edits R -> "Edited" (Dirty=true, still ETag "srv-1"); UI stashes the
//     pre-edit snapshot on the undo stack.
//  3. Debounced push fires -> sync PUTs "Edited" to the server, which now holds
//     it at a new ETag; the local resource becomes clean at that new ETag.
//  4. User presses u -> undoLast calls store.Restore(prev), writing "Original"
//     back with the snapshot's sync state: Dirty=false, ETag "srv-1".
//  5. Next sync: R is clean, its ETag "srv-1" != the server's new ETag, so the
//     reconcile treats the server as newer and pulls "Edited" back over
//     "Original" — the undo is silently reverted.
//
// A correct undo must make the revert stick (push "Original", or flag a
// conflict), never silently pull the pre-undo content back.
func TestUndoOfSyncedEditSurvivesNextSync(t *testing.T) {
	ctx := context.Background()
	st := newStore(t)
	srv := newFakeServer()

	name := store.ResourceName("e1@test")
	href := calPath + name

	// (1) Synced clean copy on both sides at ETag srv-1.
	if _, err := st.PutRemote(ctx, "personal", name, mkParsed(t, eventICS("e1@test", "Original")), "srv-1", href); err != nil {
		t.Fatal(err)
	}
	srv.data[href] = caldav.Object{Path: href, ETag: "srv-1", Data: mkICal(t, eventICS("e1@test", "Original"))}

	// Capture the pre-edit snapshot the UI stashes on the undo stack.
	loc, ok := st.Locate("e1@test")
	if !ok {
		t.Fatal("resource not locatable before edit")
	}
	prev := loc.Prev

	// (2) User edits R -> "Edited".
	if _, err := st.Put(ctx, "personal", name, mkParsed(t, eventICS("e1@test", "Edited"))); err != nil {
		t.Fatal(err)
	}

	// (3) Debounced push fires: the edit syncs to the server.
	if _, err := sync.Sync(ctx, srv, st); err != nil {
		t.Fatal(err)
	}
	if got := srv.data[href].Data.Children[0].Props.Get("SUMMARY").Value; got != "Edited" {
		t.Fatalf("precondition failed: server summary = %q, want Edited after push", got)
	}
	if r := findRes(t, st, name); r == nil || r.Dirty {
		t.Fatalf("precondition failed: resource still dirty after push: %+v", r)
	}

	// (4) User presses u -> undoLast restores the pre-edit snapshot via RestoreDirty
	// ("Original", ETag srv-1, marked Dirty so the revert is a pending local change).
	if _, err := st.RestoreDirty(ctx, "personal", name, prev); err != nil {
		t.Fatal(err)
	}
	restored := findRes(t, st, name)
	if restored == nil || restored.Object.Events[0].Summary != "Original" {
		t.Fatalf("Restore did not write Original back: %+v", restored)
	}
	t.Logf("after Restore: Summary=%q Dirty=%v ETag=%q", restored.Object.Events[0].Summary, restored.Dirty, restored.ETag)

	// (5) Next sync (the one undoLast arms via scheduleSyncDebounced).
	if _, err := sync.Sync(ctx, srv, st); err != nil {
		t.Fatal(err)
	}

	// The user's explicit undo must not have been silently pulled back.
	final := findRes(t, st, name)
	if final == nil {
		t.Fatalf("resource vanished after sync")
	}
	if final.Object.Events[0].Summary != "Original" {
		t.Fatalf("BUG: undo silently reverted — local summary = %q after sync, want Original (the undone state)", final.Object.Events[0].Summary)
	}
}
