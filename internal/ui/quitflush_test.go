package ui

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/littekge/LazyPlanner/internal/model"
	"github.com/littekge/LazyPlanner/internal/store"
	"github.com/littekge/LazyPlanner/internal/sync"
)

// quitFlushApp builds a minimal app over the given store with a captured output
// buffer, ready to exercise flushOnQuit in isolation.
func quitFlushApp(t *testing.T, s *store.Store) (*app, *bytes.Buffer) {
	t.Helper()
	a := newApp(s, "test", time.Date(2026, 7, 5, 9, 0, 0, 0, time.UTC))
	var buf bytes.Buffer
	a.quitOut = &buf
	return a, &buf
}

func emptyStore(t *testing.T) *store.Store {
	t.Helper()
	s, err := store.Open(context.Background(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return s
}

// makeDirty adds a never-pushed local task so the store has a pending change.
func makeDirty(t *testing.T, s *store.Store) {
	t.Helper()
	obj := model.NewTodoObject(model.TodoDraft{Summary: "pending"}, time.Now())
	name := store.ResourceName(obj.Todos[0].UID)
	if _, err := s.Put(context.Background(), "personal", name, obj); err != nil {
		t.Fatal(err)
	}
	if !s.HasPendingChanges() {
		t.Fatal("precondition failed: store should report a pending change")
	}
}

// TestFlushOnQuitOfflineNoop: with no server configured (syncFn nil), the flush
// must do nothing — no call, no output — even with pending changes.
func TestFlushOnQuitOfflineNoop(t *testing.T) {
	s := emptyStore(t)
	makeDirty(t, s)
	a, buf := quitFlushApp(t, s)
	a.syncFn = nil

	a.flushOnQuit() // must not panic
	if buf.Len() != 0 {
		t.Errorf("offline flush printed %q, want nothing", buf.String())
	}
}

// TestFlushOnQuitNothingPending: a server is configured but the cache is clean —
// the flush must NOT call sync (keeps quit instant), and print nothing.
func TestFlushOnQuitNothingPending(t *testing.T) {
	s := emptyStore(t)
	a, buf := quitFlushApp(t, s)
	calls := 0
	a.syncFn = func(context.Context) (sync.SyncResult, error) { calls++; return sync.SyncResult{}, nil }

	a.flushOnQuit()
	if calls != 0 {
		t.Errorf("sync called %d times with nothing pending, want 0", calls)
	}
	if buf.Len() != 0 {
		t.Errorf("printed %q with nothing pending, want nothing", buf.String())
	}
}

// TestFlushOnQuitPushesPending: with pending changes and a server, the flush
// calls sync exactly once and announces it.
func TestFlushOnQuitPushesPending(t *testing.T) {
	s := emptyStore(t)
	makeDirty(t, s)
	a, buf := quitFlushApp(t, s)
	calls := 0
	var gotDeadline bool
	a.syncFn = func(ctx context.Context) (sync.SyncResult, error) {
		calls++
		_, gotDeadline = ctx.Deadline()
		return sync.SyncResult{}, nil
	}

	a.flushOnQuit()
	if calls != 1 {
		t.Fatalf("sync called %d times, want 1", calls)
	}
	if !gotDeadline {
		t.Error("flush sync context should carry a deadline (bounded)")
	}
	if !strings.Contains(buf.String(), "Syncing pending changes") {
		t.Errorf("expected a syncing notice, got %q", buf.String())
	}
}

// TestFlushOnQuitReportsError: a failing sync must not panic; it notes that the
// changes are saved locally.
func TestFlushOnQuitReportsError(t *testing.T) {
	s := emptyStore(t)
	makeDirty(t, s)
	a, buf := quitFlushApp(t, s)
	a.syncFn = func(context.Context) (sync.SyncResult, error) {
		return sync.SyncResult{}, errors.New("server down")
	}

	a.flushOnQuit()
	out := buf.String()
	if !strings.Contains(out, "saved locally") {
		t.Errorf("expected a local-fallback note on sync error, got %q", out)
	}
}

// TestFlushOnQuitTimesOut: a sync that ignores context cancellation must not trap
// the user — the flush returns within the (short, injected) timeout and reports
// the timeout. Guards the hard wall-clock bound.
func TestFlushOnQuitTimesOut(t *testing.T) {
	s := emptyStore(t)
	makeDirty(t, s)
	a, buf := quitFlushApp(t, s)
	a.quitFlushTimeout = 100 * time.Millisecond
	a.syncFn = func(context.Context) (sync.SyncResult, error) {
		time.Sleep(2 * time.Second) // ignore ctx, simulating a hung network
		return sync.SyncResult{}, nil
	}

	start := time.Now()
	done := make(chan struct{})
	go func() { a.flushOnQuit(); close(done) }()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("flushOnQuit did not return — the timeout bound is not enforced")
	}
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Errorf("flushOnQuit took %s, want ~100ms (timeout not enforced)", elapsed)
	}
	if !strings.Contains(buf.String(), "timed out") {
		t.Errorf("expected a timeout notice, got %q", buf.String())
	}
}
