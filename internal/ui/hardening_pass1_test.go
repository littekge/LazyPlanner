package ui

import (
	"context"
	"testing"
	"time"

	"github.com/gdamore/tcell/v2"

	"github.com/littekge/LazyPlanner/internal/sync"
)

// TestZeroResetsHourZoom locks the #4/#2 gap fix: `0` in the week/day grid returns
// the hour-row height to auto-fit after a +/- zoom.
func TestZeroResetsHourZoom(t *testing.T) {
	a := newRootedTestApp(t, time.Date(2026, 7, 6, 12, 0, 0, 0, time.UTC))
	a.setMode(modeCalendar)
	a.viewMode = viewWeek
	a.buildCenterCalendar()

	a.zoomHour(3) // set an explicit zoom
	if a.hourRows == 0 {
		t.Fatal("zoomHour did not set an explicit hour-row height")
	}
	a.globalKeys(runeKey('0')) // 0 → auto-fit
	if a.hourRows != 0 {
		t.Errorf("after 0, hourRows = %d, want 0 (auto-fit)", a.hourRows)
	}
}

// TestZeroStillExtendsCount: 0 must still work as a vim count digit once a count is
// pending (so 10j etc. keep working) — it only resets zoom as a bare press.
func TestZeroStillExtendsCount(t *testing.T) {
	a := newRootedTestApp(t, time.Date(2026, 7, 6, 12, 0, 0, 0, time.UTC))
	a.setMode(modeCalendar)
	a.viewMode = viewWeek
	a.globalKeys(runeKey('1'))
	a.globalKeys(runeKey('0'))
	if a.pendingCount != 10 {
		t.Errorf("pendingCount after '1' '0' = %d, want 10 (0 extends a pending count)", a.pendingCount)
	}
}

// TestDebouncedPushArmsAfterEdit locks the #1 gap fix: a local mutation arms the
// debounced background push when a server is configured, and is a silent no-op
// when offline.
func TestDebouncedPushArmsAfterEdit(t *testing.T) {
	a := newRootedTestApp(t, time.Date(2026, 7, 6, 12, 0, 0, 0, time.UTC))

	// Offline: pushUndo must not arm a timer.
	a.syncFn = nil
	a.pushUndo("x", "")
	if a.syncTimer != nil {
		t.Error("offline: pushUndo should not arm a debounced push")
	}

	// Configured: pushUndo arms the debounced push.
	a.syncFn = func(context.Context) (sync.SyncResult, error) { return sync.SyncResult{}, nil }
	a.pushUndo("y", "")
	if a.syncTimer == nil {
		t.Error("configured: pushUndo should arm the debounced push")
	}
	a.stopSyncTimer() // don't let it fire after the test
}

// TestResizeSubModeDetail locks the #4 build: Ctrl-W enters a modal resize mode
// where H/L size the Detail pane and Esc exits.
func TestResizeSubModeDetail(t *testing.T) {
	a := newRootedTestApp(t, time.Date(2026, 7, 6, 12, 0, 0, 0, time.UTC))
	a.setMode(modeCalendar) // Detail is shown in calendar mode
	a.saveState = func(int, int, []string, int) {}

	a.globalKeys(tcell.NewEventKey(tcell.KeyCtrlW, 0, tcell.ModNone))
	if !a.resizing {
		t.Fatal("Ctrl-W did not enter resize mode")
	}
	if a.interactionMode() != modeResize {
		t.Errorf("mode badge = %q, want RESIZE", a.interactionMode())
	}

	w0 := a.detailWidth
	a.globalKeys(runeKey('L')) // grow detail
	if a.detailWidth <= w0 {
		t.Errorf("L did not grow the Detail pane (%d → %d)", w0, a.detailWidth)
	}
	a.globalKeys(runeKey('H')) // shrink detail
	if a.detailWidth != w0 {
		t.Errorf("H did not shrink the Detail pane back (%d, want %d)", a.detailWidth, w0)
	}

	l0 := a.leftWidth
	a.globalKeys(runeKey('l')) // grow overview
	if a.leftWidth <= l0 {
		t.Errorf("l did not grow the overview (%d → %d)", l0, a.leftWidth)
	}

	a.globalKeys(tcell.NewEventKey(tcell.KeyEscape, 0, tcell.ModNone))
	if a.resizing {
		t.Error("Esc did not exit resize mode")
	}
}
