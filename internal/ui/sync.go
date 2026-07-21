package ui

import (
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
		res, err := a.syncFn(a.ctx)
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

// syncDebounce is how long after the last local edit a background push fires.
const syncDebounce = 3 * time.Second

// scheduleSyncDebounced arms (or re-arms) a one-shot background sync a few seconds
// after a local edit — the "debounced push after edits" trigger, so other devices
// see changes fast without a sync on every keystroke. It's a no-op offline (no
// server), and triggerSync coalesces, so it never stacks with a running sync. The
// timer is only (re)set from the event-loop goroutine (all mutations run there);
// the fired callback re-enters the loop via QueueUpdateDraw.
func (a *app) scheduleSyncDebounced() {
	if a.syncFn == nil {
		return
	}
	if a.syncTimer != nil {
		a.syncTimer.Stop()
	}
	a.syncTimer = time.AfterFunc(syncDebounce, func() {
		a.tv.QueueUpdateDraw(func() { a.fireDebouncedSync() })
	})
}

// fireDebouncedSync runs the armed debounced push — unless a create/edit form is
// open. A sync landing while a form is open replaces the store pointer the form
// captured, so its version-checked Save reads as stale and silently discards the
// user's input. While a modal is open the push defers (re-arms) and closeModal
// fires it once the form closes.
func (a *app) fireDebouncedSync() {
	if a.modalOpen() {
		a.scheduleSyncDebounced()
		return
	}
	a.triggerSync()
}

// stopSyncTimer cancels a pending debounced push (called on quit).
func (a *app) stopSyncTimer() {
	if a.syncTimer != nil {
		a.syncTimer.Stop()
	}
}

// startPeriodicSync fires a background sync every interval until the app's context
// is cancelled (quit). The tick queues triggerSync onto the event-loop goroutine —
// triggerSync touches UI state (a.syncing) and must not run off it — and
// triggerSync itself coalesces, so a slow sync spanning a tick is not stacked.
func (a *app) startPeriodicSync(interval time.Duration) {
	go func() {
		t := time.NewTicker(interval)
		defer t.Stop()
		for {
			select {
			case <-a.ctx.Done():
				return
			case <-t.C:
				a.tv.QueueUpdateDraw(func() {
					// Skip a periodic tick while a form is open (see fireDebouncedSync);
					// the next tick — or closeModal's re-armed push — catches up.
					if a.modalOpen() {
						return
					}
					a.triggerSync()
				})
			}
		}
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
