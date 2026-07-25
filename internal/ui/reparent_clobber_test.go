package ui

import (
	"context"
	"testing"
	"time"
)

// TestReparentToDoesNotClobberConcurrentPull demonstrates the finding: the
// single-item same-list reparent (reparentTo, yankpaste.go) commits with a bare
// store.Put. A background sync pull landing between paste()'s Locate and that
// Put is silently overwritten — the stale-content object is written but inherits
// the freshly-pulled ETag, so the next push's CAS matches the server and the
// remote edit is lost with no conflict. reparentTo must version-check via
// PutIfUnchanged and skip the write on a mismatch, the way reparentOps (the
// multi-root path) already does.
func TestReparentToDoesNotClobberConcurrentPull(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 7, 5, 9, 0, 0, 0, time.UTC)
	a := newWritableTestApp(t, now)

	const cal, mover, stay = "personal", "reparent-mover", "reparent-stay"
	name := "reparent-bundle.ics"
	href := "/dav/personal/" + name

	// Seed a clean, synced resource holding the mover plus a co-resident bystander
	// whose DESCRIPTION carries a revision marker, then reload so the app sees it.
	if _, err := a.store.PullRemote(ctx, cal, name, bundledTodosObj(t, mover, stay, "rev-0"), "etag-v1", href, nil); err != nil {
		t.Fatalf("seed pull: %v", err)
	}
	a.reload()

	// `p` pressed: paste()'s Locate captures the current object + snapshot.
	loc, ok := a.store.Locate(mover)
	if !ok {
		t.Fatal("Locate failed")
	}

	// A background sync pull lands in the window: another device edited the
	// co-resident todo's DESCRIPTION. expectedPrev == loc.Prev so it applies.
	applied, err := a.store.PullRemote(ctx, cal, name, bundledTodosObj(t, mover, stay, "rev-1"), "etag-v2", href, loc.Prev)
	if err != nil {
		t.Fatalf("concurrent pull: %v", err)
	}
	if !applied {
		t.Fatal("concurrent pull not applied; interleaving precondition not met")
	}

	// The re-parent proceeds from the STALE loc snapshot (rev-0 bystander).
	a.yankUIDs = []string{mover}
	a.yankCut = true
	a.reparentTo(loc, "some-parent-uid")

	// The pulled bystander edit must survive. With a bare Put, reparentTo rewrites
	// the whole resource from stale content (reverting the bystander to rev-0)
	// while inheriting the pulled ETag — a silent clobber.
	got, ok := a.store.Locate(stay)
	if !ok {
		t.Fatal("bystander vanished")
	}
	td := findTdDesc(got.Object, stay)
	if td == nil {
		t.Fatal("bystander todo missing from its resource")
	}
	if td.Description != "rev-1" {
		t.Errorf("CLOBBER: reparentTo's bare Put overwrote the concurrent pull; bystander reads %q, want %q", td.Description, "rev-1")
	}
}
