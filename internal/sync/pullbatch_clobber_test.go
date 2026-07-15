package sync_test

import (
	"context"
	"testing"

	"github.com/littekge/LazyPlanner/internal/caldav"
	"github.com/littekge/LazyPlanner/internal/store"
	"github.com/littekge/LazyPlanner/internal/sync"
)

// TestReproPullBatchClobbersConcurrentEditToOrphan reproduces the HIGH finding:
// PullRemoteBatch unconditionally clobbers a concurrent local edit to a
// pull-orphan (silent data loss).
//
// Setup mirrors the failure scenario:
//   - "d@test" is a synced resource; a local edit makes it dirty so reconcile
//     step (A) issues a PutObject for it (the network window).
//   - "o@test" is a pull orphan: an .ics on disk with no sidecar entry, so on
//     reload it is clean with Href=="" (an interrupted bulk pull left it). The
//     server still holds the same href H, unchanged.
//   - During D's PUT (the onPut hook = "network latency"), the user edits the
//     orphan: it becomes Dirty with new content, still Href=="".
//
// Step (A) built localByHref from the pre-loop snapshot, which never contained
// the href-less orphan, so step (B) includes H in the pulls and PullRemoteBatch
// stages the server version over the user's just-made edit — Dirty=false.
func TestReproPullBatchClobbersConcurrentEditToOrphan(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()

	dName := store.ResourceName("d@test")
	oName := store.ResourceName("o@test")
	dHref := calPath + dName
	oHref := calPath + oName

	// Open, seed the synced resource D, and drop the orphan .ics (no sidecar).
	st, err := store.Open(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.SetCalendarMeta(ctx, "personal",
		store.CalendarMeta{DisplayName: "Personal", Href: calPath}); err != nil {
		t.Fatal(err)
	}
	if _, err := st.PutRemote(ctx, "personal", dName,
		mkParsed(t, eventICS("d@test", "D-Original")), "d-1", dHref); err != nil {
		t.Fatal(err)
	}
	writeOrphan(t, dir, oName, eventICS("o@test", "Orphan"))

	// Reopen: D loads clean/synced, O loads as a clean href-less pull orphan.
	st, err = store.Open(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}

	// Local edit to D → dirty; step (A) will push it (server ETag still matches).
	if _, err := st.Put(ctx, "personal", dName,
		mkParsed(t, eventICS("d@test", "D-Edited"))); err != nil {
		t.Fatal(err)
	}

	srv := newFakeServer()
	srv.data[dHref] = caldav.Object{Path: dHref, ETag: "d-1", Data: mkICal(t, eventICS("d@test", "D-Original"))}
	srv.data[oHref] = caldav.Object{Path: oHref, ETag: "srv-o", Data: mkICal(t, eventICS("o@test", "Server"))}

	// While D's PUT is in flight, the user edits the orphan.
	srv.onPut = func() {
		srv.onPut = nil // once
		if _, err := st.Put(ctx, "personal", oName,
			mkParsed(t, eventICS("o@test", "user-edit"))); err != nil {
			t.Fatal(err)
		}
	}

	if _, err := sync.Sync(ctx, srv, st); err != nil {
		t.Fatal(err)
	}

	r := findRes(t, st, oName)
	if r == nil {
		t.Fatal("orphan resource gone after sync")
	}
	got := r.Object.Events[0].Summary
	t.Logf("after sync: orphan summary=%q dirty=%v", got, r.Dirty)
	if got != "user-edit" || !r.Dirty {
		t.Fatalf("orphan clobbered: summary=%q dirty=%v; want %q dirty=true (the concurrent edit must survive and stay pending)",
			got, r.Dirty, "user-edit")
	}
}
