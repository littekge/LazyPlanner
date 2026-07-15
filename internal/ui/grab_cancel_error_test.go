package ui

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/rivo/tview"

	"github.com/littekge/LazyPlanner/internal/model"
	"github.com/littekge/LazyPlanner/internal/store"
)

// TestGrabFutureCancelSurfacesRestoreFailure guards pass-11 MED #4: cancelGrab
// used to discard the Delete/Restore error returns and always flash "Grab
// cancelled". When the master un-cap fails on a this-&-future cancel, the user
// was told the series was intact while it stayed capped — silent data loss. The
// fix surfaces the error and, because it restores before deleting, leaves the new
// tail in place rather than compounding the loss.
func TestGrabFutureCancelSurfacesRestoreFailure(t *testing.T) {
	dataDir := t.TempDir()
	s, err := store.Open(context.Background(), dataDir)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	when := time.Date(2026, 7, 6, 12, 0, 0, 0, time.UTC)
	a := newApp(s, "test", when)
	a.build()
	a.root = tview.NewPages()
	a.root.AddPage(pageMain, a.layout(), true, true)
	a.setMode(modeTasks)

	if err := s.CreateCalendarLocal(context.Background(), "ev", store.CalendarMeta{DisplayName: "EV"}, []string{"VEVENT"}); err != nil {
		t.Fatal(err)
	}
	uid := putRecurringEvent(t, a, "ev", "Standup", time.Date(2026, 7, 6, 9, 0, 0, 0, time.UTC), "FREQ=WEEKLY;COUNT=4")
	a.reload()
	a.mode = modeCalendar
	a.viewMode = viewWeek
	occ := time.Date(2026, 7, 20, 9, 0, 0, 0, time.UTC) // 3rd instance
	a.anchor = model.DayStart(occ)

	loc, _ := a.store.Locate(uid)
	a.beginGrab(loc, editTarget{uid: uid, occStart: occ, recurring: true}, scopeFuture)
	masterName := a.grabSplitMaster
	newName := a.grabName
	if masterName == "" || newName == "" {
		t.Fatalf("this-&-future grab did not set split state (master=%q new=%q)", masterName, newName)
	}

	// Force the master's un-cap Restore to fail: replace the master .ics file with
	// a directory so writeFileAtomic's rename-over fails deterministically.
	masterPath := filepath.Join(dataDir, "calendars", "ev", masterName)
	if err := os.Remove(masterPath); err != nil {
		t.Fatalf("removing master .ics: %v", err)
	}
	if err := os.Mkdir(masterPath, 0o755); err != nil {
		t.Fatalf("planting blocker dir: %v", err)
	}

	a.cancelGrab()

	// The failure must be surfaced, not reported as a clean cancel.
	got := a.statusLeft.GetText(true)
	if !strings.Contains(strings.ToLower(got), "failed") {
		t.Errorf("cancelGrab swallowed the revert failure — flashed %q, want an error surfaced", got)
	}
	// Restore-before-delete: the un-cap failed, so the new tail must NOT have been
	// deleted (both copies gone would be the worse, compounded loss).
	cal, ok := a.store.Calendar("ev")
	if !ok {
		t.Fatal("calendar ev vanished")
	}
	found := false
	for _, r := range cal.Resources {
		if r.Name == newName {
			found = true
		}
	}
	if !found {
		t.Errorf("new tail series %q was deleted despite the failed un-cap — cancel compounded the loss", newName)
	}
}
