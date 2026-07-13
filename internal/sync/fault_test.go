package sync_test

import (
	"context"
	"errors"
	"testing"

	"github.com/littekge/LazyPlanner/internal/store"
	"github.com/littekge/LazyPlanner/internal/sync"
)

// TestSyncDiscoverFailureIsCleanError: a discovery failure must surface as an
// error without panicking or mutating the local cache.
func TestSyncDiscoverFailureIsCleanError(t *testing.T) {
	ctx := context.Background()
	st := newStore(t)
	name := store.ResourceName("keep@test")
	if _, err := st.PutRemote(ctx, "personal", name, mkParsed(t, eventICS("keep@test", "Keep")), "srv-1", calPath+name); err != nil {
		t.Fatal(err)
	}

	srv := newFakeServer()
	srv.discoverErr = errors.New("network down")

	if _, err := sync.Sync(ctx, srv, st); err == nil {
		t.Fatal("expected Sync to return an error on discovery failure")
	}
	// The cache is untouched: the calendar and its resource are still present.
	if r := findRes(t, st, name); r == nil {
		t.Fatal("local resource was lost after a failed discovery")
	}
}

// TestSyncPushFailureKeepsEditDirty: a transient network failure on a push must
// leave the local edit intact and still dirty (so it retries), never mark it
// clean or drop it — and a later sync pushes it once the server recovers.
func TestSyncPushFailureKeepsEditDirty(t *testing.T) {
	ctx := context.Background()
	st := newStore(t)
	srv := newFakeServer()

	name := store.ResourceName("e1@test")
	if _, err := st.Put(ctx, "personal", name, mkParsed(t, eventICS("e1@test", "Local"))); err != nil {
		t.Fatal(err)
	}
	href := calPath + name
	srv.failPut[href] = errors.New("network hiccup")

	res, err := sync.Sync(ctx, srv, st)
	if err != nil {
		t.Fatalf("a per-resource push failure should not abort the whole sync: %v", err)
	}
	if len(res.Skipped) == 0 {
		t.Error("expected the failed push to be recorded as a skip")
	}
	r := findRes(t, st, name)
	if r == nil || !r.Dirty || r.Href != "" {
		t.Fatalf("edit not preserved-and-dirty after a failed push: %+v", r)
	}

	// Server recovers → the still-dirty edit pushes cleanly on the next sync.
	delete(srv.failPut, href)
	if _, err := sync.Sync(ctx, srv, st); err != nil {
		t.Fatal(err)
	}
	r = findRes(t, st, name)
	if r == nil || r.Dirty || r.Href == "" {
		t.Fatalf("edit not pushed after the server recovered: %+v", r)
	}
	if srv.puts == 0 {
		t.Error("expected a PUT after recovery")
	}
}
