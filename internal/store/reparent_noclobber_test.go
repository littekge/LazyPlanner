package store_test

import (
	"context"
	"testing"
	"time"

	"github.com/littekge/LazyPlanner/internal/model"
	"github.com/littekge/LazyPlanner/internal/store"
)

// TestReparentDoesNotClobberConcurrentPull guards pass-13 MED #3: ui.reparentSelected
// (H/L) did Locate -> SetTodoParent(stale object) -> plain store.Put, so a background
// sync pull landing between the Locate and the Put was silently clobbered (the write
// adopted the pulled ETag while persisting content derived from the now-stale
// snapshot, and the next push's CAS matched the server). reparentSelected now commits
// via store.PutIfUnchanged(loc.Prev). This replays the store sequence — the TOCTOU
// window is internal to reparentSelected (between its own Locate and Put), so it can't
// be reproduced black-box; matching grabclobber/quickfield/complete no-clobber tests.
func TestReparentDoesNotClobberConcurrentPull(t *testing.T) {
	ctx := context.Background()
	s, err := store.Open(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	const cal, childUID = "personal", "child-1"
	name := store.ResourceName(childUID)
	href := "/dav/personal/child-1.ics"

	// Seed a clean, synced child todo (ETag etag-v1).
	if _, err := s.PullRemote(ctx, cal, name, todoICS(t, childUID, "Original summary", 0), "etag-v1", href, nil); err != nil {
		t.Fatalf("seed pull: %v", err)
	}

	// (1) reparentSelected's internal Locate captures object + snapshot.
	loc, ok := s.Locate(childUID)
	if !ok {
		t.Fatal("Locate failed")
	}

	// (2) A background sync pull lands: the server changed the SUMMARY (a field the
	// reparent does not touch). expectedPrev == loc.Prev, so the guarded pull applies.
	applied, err := s.PullRemote(ctx, cal, name, todoICS(t, childUID, "SERVER EDITED THE TITLE", 0), "etag-v2", href, loc.Prev)
	if err != nil {
		t.Fatalf("concurrent pull: %v", err)
	}
	if !applied {
		t.Fatal("concurrent pull not applied; interleaving precondition not met")
	}

	// (3) reparentSelected derives newObj from the STALE loc.Object (outdent to root).
	newObj, err := model.SetTodoParent(loc.Object, childUID, "", time.Now(), time.UTC)
	if err != nil {
		t.Fatalf("SetTodoParent: %v", err)
	}

	// (4) The fixed commit: version-checked against loc.Prev. Because the resource
	// changed underneath, the write must be SKIPPED (applied=false), not clobbering.
	applied2, err := s.PutIfUnchanged(ctx, loc.CalID, loc.Name, newObj, loc.Prev)
	if err != nil {
		t.Fatalf("PutIfUnchanged: %v", err)
	}
	if applied2 {
		t.Fatal("stale reparent was applied; PutIfUnchanged must skip a write over a changed resource")
	}

	// The pulled server edit survives intact, at etag-v2, and clean.
	cs, _ := s.Calendar(cal)
	res := findResource(cs, name)
	if res == nil {
		t.Fatal("resource missing")
	}
	got := findTd(res.Object, childUID)
	if got == nil {
		t.Fatal("todo missing")
	}
	if got.Summary != "SERVER EDITED THE TITLE" {
		t.Errorf("SERVER EDIT LOST: summary = %q, want %q", got.Summary, "SERVER EDITED THE TITLE")
	}
	if res.Dirty {
		t.Errorf("resource left Dirty after a skipped write; the pulled edit should stay clean (ETag=%q)", res.ETag)
	}
	if got.ParentUID != "" {
		// The stale outdent must not have taken effect either.
		t.Errorf("stale reparent leaked through: ParentUID = %q, want unchanged", got.ParentUID)
	}
}
