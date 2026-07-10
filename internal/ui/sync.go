package ui

import (
	"context"
	"fmt"
	"time"

	"github.com/littekge/LazyPlanner/internal/sync"
)

// triggerSync runs a two-way sync in the background so the UI never blocks on
// the network. It is a no-op (with a hint) when no server is configured, and
// coalesces overlapping requests. On completion it refreshes the views and the
// sync-status indicator on the event-loop goroutine.
func (a *app) triggerSync() {
	if a.syncFn == nil {
		a.flash("Sync not configured — set [server] in config.toml")
		return
	}
	if a.syncing {
		return
	}
	a.syncing = true
	a.renderSyncStatus()

	go func() {
		res, err := a.syncFn(context.Background())
		a.tv.QueueUpdateDraw(func() {
			a.syncing = false
			a.lastSyncErr = err
			if err != nil {
				a.flash("Sync failed: " + err.Error())
				a.renderSyncStatus()
				return
			}
			a.lastSyncAt = time.Now()
			a.refresh("") // rebuild views from the freshly-synced store
			if summary := syncSummary(res); summary != "" {
				a.flash(summary)
			}
			a.renderSyncStatus()
		})
	}()
}

// renderSyncStatus paints the right section of the status bar from the current
// sync state. Words + color (not glyphs) keep it legible on a bare TTY.
func (a *app) renderSyncStatus() {
	switch {
	case a.syncFn == nil:
		a.statusRight.SetText("[gray]not configured[-]")
	case a.syncing:
		a.statusRight.SetText("[yellow]syncing...[-]")
	case a.lastSyncErr != nil:
		a.statusRight.SetText("[red]offline[-]")
	default:
		text := "[green]synced"
		if !a.lastSyncAt.IsZero() {
			text += " " + clockStr(a.lastSyncAt.In(a.loc), a.clock24)
		}
		text += "[-]"
		if n := len(a.store.Conflicts()); n > 0 {
			text += fmt.Sprintf("  [red]! %d conflict(s)[-]", n)
		}
		a.statusRight.SetText(text)
	}
}

// syncSummary is a one-line result for the status flash, or "" when nothing
// changed (a quiet, uneventful sync needs no announcement).
func syncSummary(res sync.SyncResult) string {
	if res.Pushed+res.Pulled+res.PushedDeletes+res.PulledDeletes+res.Conflicts == 0 {
		return ""
	}
	s := fmt.Sprintf("Synced: %d up, %d down", res.Pushed+res.PushedDeletes, res.Pulled+res.PulledDeletes)
	if res.Conflicts > 0 {
		s += fmt.Sprintf(", %d conflict(s) — resolve in-app", res.Conflicts)
	}
	return s
}
