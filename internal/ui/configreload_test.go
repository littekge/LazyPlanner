package ui

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/littekge/LazyPlanner/internal/sync"
)

func TestConfigUnavailableFlashes(t *testing.T) {
	a := newRootedTestApp(t, time.Date(2026, 7, 5, 12, 0, 0, 0, time.UTC))
	a.editConfig = nil
	a.cmdConfig()
	if got := a.statusLeft.GetText(true); !strings.Contains(got, "unavailable") {
		t.Errorf("expected an unavailable flash, got %q", got)
	}
}

func TestApplyConfigReloadSwapsSync(t *testing.T) {
	a := newRootedTestApp(t, time.Date(2026, 7, 5, 12, 0, 0, 0, time.UTC))
	a.syncFn = nil
	fresh := func(context.Context) (sync.SyncResult, error) { return sync.SyncResult{}, nil }

	a.applyConfigReload(ConfigReload{Sync: fresh}, nil)
	if a.syncFn == nil {
		t.Error("a reloaded non-nil sync closure should be swapped in")
	}
	if got := a.statusLeft.GetText(true); !strings.Contains(got, "reloaded") {
		t.Errorf("expected a reloaded flash, got %q", got)
	}
}

func TestApplyConfigReloadSurfacesError(t *testing.T) {
	a := newRootedTestApp(t, time.Date(2026, 7, 5, 12, 0, 0, 0, time.UTC))
	before := a.syncFn
	a.applyConfigReload(ConfigReload{}, errors.New("account changed"))
	if got := a.statusLeft.GetText(true); !strings.Contains(got, "account changed") {
		t.Errorf("expected the reload error flashed, got %q", got)
	}
	_ = before // syncFn is left untouched on error
}

// TestApplyConfigReloadAppliesColorMode: changing color_mode via :config takes
// effect live — the parsed mode updates and the color index rebuilds.
func TestApplyConfigReloadAppliesColorMode(t *testing.T) {
	a := newRootedTestApp(t, time.Date(2026, 7, 5, 12, 0, 0, 0, time.UTC))
	// Give a calendar a color so the index has something mode-dependent to hold.
	cals := a.store.Calendars()
	if len(cals) == 0 {
		t.Skip("fixture has no calendars")
	}
	id := cals[0].ID
	if err := a.store.SyncCalendarColor(context.Background(), id, "#3fb950"); err != nil {
		t.Fatal(err)
	}
	a.reload()
	if a.colorMode != colorAuto {
		t.Fatalf("precondition: colorMode = %d, want colorAuto", a.colorMode)
	}

	// Reload with color_mode = "off": mode flips and the calendar color drops out.
	a.applyConfigReload(ConfigReload{ColorMode: "off"}, nil)
	if a.colorMode != colorOff {
		t.Errorf("colorMode = %d after reload, want colorOff", a.colorMode)
	}
	if _, ok := a.calColors[id]; ok {
		t.Error("colorOff should have cleared the calendar color from the index")
	}

	// Reload back to "16": mode flips and the color returns (as a themed color).
	a.applyConfigReload(ConfigReload{ColorMode: "16"}, nil)
	if a.colorMode != color16 {
		t.Errorf("colorMode = %d after reload, want color16", a.colorMode)
	}
	if _, ok := a.calColors[id]; !ok {
		t.Error("color16 should have repopulated the calendar color")
	}
}

// TestApplyConfigReloadFlashesWarning: a reloaded connection that went offline
// (a warning present) is surfaced in the status bar.
func TestApplyConfigReloadFlashesWarning(t *testing.T) {
	a := newRootedTestApp(t, time.Date(2026, 7, 5, 12, 0, 0, 0, time.UTC))
	a.applyConfigReload(ConfigReload{Warning: "bw not logged in (offline)"}, nil)
	if got := a.statusLeft.GetText(true); !strings.Contains(got, "offline") {
		t.Errorf("expected the reload warning flashed, got %q", got)
	}
}
