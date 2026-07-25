package ui

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/littekge/LazyPlanner/internal/sync"
)

func noopSync(context.Context) (sync.SyncResult, error) { return sync.SyncResult{}, nil }

func TestRenderSyncStatus(t *testing.T) {
	now := time.Date(2026, 7, 5, 12, 0, 0, 0, time.UTC)

	tests := []struct {
		name    string
		setup   func(a *app)
		want    string
		notWant string
	}{
		{
			name:  "not configured",
			setup: func(a *app) { a.syncFn = nil },
			want:  "not configured",
		},
		{
			name:  "syncing",
			setup: func(a *app) { a.syncFn = noopSync; a.syncing = true },
			want:  "syncing",
		},
		{
			name:  "offline on error",
			setup: func(a *app) { a.syncFn = noopSync; a.lastSyncErr = errors.New("boom") },
			want:  "offline",
		},
		{
			name: "synced shows time",
			setup: func(a *app) {
				a.syncFn = noopSync
				a.clock24 = true // 24h clock (time_format)
				a.lastSyncAt = time.Date(2026, 7, 5, 14, 32, 0, 0, time.Local)
			},
			want:    "synced 14:32",
			notWant: "conflict",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			a := newTestApp(t, now)
			tc.setup(a)
			a.renderSyncStatus()
			got := a.statusRight.GetText(true)
			if !strings.Contains(got, tc.want) {
				t.Errorf("status = %q, want substring %q", got, tc.want)
			}
			if tc.notWant != "" && strings.Contains(got, tc.notWant) {
				t.Errorf("status = %q, should not contain %q", got, tc.notWant)
			}
		})
	}
}

func TestSyncSummary(t *testing.T) {
	if got := syncSummary(sync.SyncResult{}); got != "" {
		t.Errorf("empty result summary = %q, want empty (quiet sync)", got)
	}
	got := syncSummary(sync.SyncResult{Pushed: 2, Pulled: 1, Conflicts: 1})
	if !strings.Contains(got, "up") || !strings.Contains(got, "down") || !strings.Contains(got, "conflict") {
		t.Errorf("summary = %q, want up/down/conflict mentioned", got)
	}
}

// TestTriggerSyncNotConfigured verifies the no-op path flashes a hint rather
// than launching a goroutine when no server is configured.
func TestTriggerSyncNotConfigured(t *testing.T) {
	a := newTestApp(t, time.Date(2026, 7, 5, 12, 0, 0, 0, time.UTC))
	a.syncFn = nil
	a.triggerSync()
	if got := a.statusLeft.GetText(true); !strings.Contains(got, "not configured") {
		t.Errorf("flash = %q, want a not-configured hint", got)
	}
	if a.syncing {
		t.Error("syncing flag set despite no sync function")
	}
}

// TestTriggerSyncNoAccountHintMatchesConfigFormat: the no-account hint must
// point at the config format the loader actually accepts. The single
// [server] section was replaced by named [[account]] blocks in v1.2.0, and
// config.Load hard-rejects a config containing [server] (internal/config's
// removed-[server] check) — so a hint telling the user to "set [server]"
// sends them to do something the app refuses to load.
func TestTriggerSyncNoAccountHintMatchesConfigFormat(t *testing.T) {
	a := newTestApp(t, time.Date(2026, 7, 5, 12, 0, 0, 0, time.UTC))
	a.syncFn = nil
	a.triggerSync()
	got := a.statusLeft.GetText(true)
	if strings.Contains(got, "[server]") {
		t.Errorf("flash = %q, still references the removed [server] section", got)
	}
	if !strings.Contains(strings.ToLower(got), "account") {
		t.Errorf("flash = %q, want it to reference an [[account]] block", got)
	}
}

// TestSyncUsesCancellableContext: triggerSync passes the app's cancellable
// context, and cancelling the app cancels the in-flight sync's context.
func TestSyncUsesCancellableContext(t *testing.T) {
	a := newTestApp(t, time.Date(2026, 7, 5, 12, 0, 0, 0, time.UTC))
	got := make(chan context.Context, 1)
	a.syncFn = func(ctx context.Context) (sync.SyncResult, error) {
		got <- ctx
		return sync.SyncResult{}, nil
	}
	a.triggerSync()
	ctx := <-got
	if ctx.Err() != nil {
		t.Fatal("sync context already cancelled before quit")
	}
	a.cancel() // simulate quit
	select {
	case <-ctx.Done():
	case <-time.After(time.Second):
		t.Error("app cancel() did not cancel the sync's context")
	}
}
