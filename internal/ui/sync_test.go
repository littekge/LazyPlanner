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
