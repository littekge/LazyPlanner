package ui

import (
	"context"
	"testing"
	"time"

	"github.com/rivo/tview"

	"github.com/littekge/LazyPlanner/internal/store"
)

// TestDebouncedSyncDefersWhileModalFormOpen locks the v1.0.2 Bug 2 fix: the
// debounced background push must not fire while a create/edit form is open — a
// sync landing then replaces the store pointer the open form captured, so the
// form's version-checked Save reads as stale and discards the user's input.
// While a modal is open the fire defers (re-arms); once it closes it may fire.
func TestDebouncedSyncDefersWhileModalFormOpen(t *testing.T) {
	a := newRootedTestApp(t, time.Date(2026, 7, 20, 12, 0, 0, 0, time.UTC))
	a.syncFn = noopSync

	// A create/edit form is open.
	a.root.AddPage(pageForm, tview.NewBox(), true, true)
	if !a.modalOpen() {
		t.Fatal("precondition: a modal should be open")
	}

	a.fireDebouncedSync()
	if a.syncing {
		t.Error("debounced sync fired while a modal form was open; it must defer")
	}
	if a.syncTimer == nil {
		t.Error("debounced sync did not re-arm to retry after the form closes")
	}
	a.stopSyncTimer()

	// Form closed: the deferred push may now fire.
	a.root.RemovePage(pageForm)
	if a.modalOpen() {
		t.Fatal("precondition: the modal should be closed")
	}
	a.fireDebouncedSync()
	if !a.syncing {
		t.Error("debounced sync did not fire once the form was closed")
	}
}

// TestCloseModalRearmsDeferredPushWhenPending locks the companion behavior: when
// a form closes with local changes still pending a push, closeModal re-arms the
// debounced push so the deferred edit still syncs promptly (rather than waiting
// for the next periodic tick).
func TestCloseModalRearmsDeferredPushWhenPending(t *testing.T) {
	a := newRootedTestApp(t, time.Date(2026, 7, 20, 12, 0, 0, 0, time.UTC))
	a.syncFn = noopSync

	if err := a.store.CreateCalendarLocal(context.Background(), "ev", store.CalendarMeta{DisplayName: "EV"}, []string{"VEVENT"}); err != nil {
		t.Fatal(err)
	}
	// A local create leaves a Dirty resource pending a push.
	putSpanningEvent(t, a, "ev", "Pending",
		time.Date(2026, 7, 20, 9, 0, 0, 0, time.Local),
		time.Date(2026, 7, 20, 10, 0, 0, 0, time.Local))
	if !a.store.HasPendingChanges() {
		t.Fatal("precondition: the local create should be pending a push")
	}

	a.stopSyncTimer()
	a.syncTimer = nil

	a.openModal(pageForm, tview.NewBox(), 10, 10)
	a.closeModal(pageForm)

	if a.syncTimer == nil {
		t.Error("closeModal did not re-arm the deferred push despite pending changes")
	}
	a.stopSyncTimer()
}
