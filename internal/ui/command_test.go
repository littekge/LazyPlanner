package ui

import (
	"testing"
	"time"
)

func TestCommandView(t *testing.T) {
	a := newRootedTestApp(t, time.Date(2026, 7, 5, 12, 0, 0, 0, time.UTC))

	a.runCommand("view week")
	if a.mode != modeCalendar || a.viewMode != viewWeek {
		t.Errorf("after :view week, mode=%d view=%d, want calendar/week", a.mode, a.viewMode)
	}
	if got := a.statusMid.GetText(true); got != ":view week" {
		t.Errorf("command view = %q, want :view week", got)
	}

	a.runCommand("view nonsense")
	if a.viewMode != viewWeek {
		t.Error("invalid :view should not change the view")
	}
	if got := a.statusLeft.GetText(true); got == "" {
		t.Error("expected a flash for an invalid :view arg")
	}
}

func TestCommandGoto(t *testing.T) {
	a := newRootedTestApp(t, time.Date(2026, 7, 5, 12, 0, 0, 0, time.UTC))

	a.runCommand("goto 2026-12-25")
	if a.mode != modeCalendar {
		t.Error("goto should switch to calendar mode")
	}
	if a.anchor.Year() != 2026 || a.anchor.Month() != time.December || a.anchor.Day() != 25 {
		t.Errorf("anchor = %s, want 2026-12-25", a.anchor.Format("2006-01-02"))
	}

	a.runCommand("goto not-a-date")
	if got := a.statusLeft.GetText(true); got == "" {
		t.Error("expected a flash when goto can't parse a date")
	}
}

func TestCommandUnknown(t *testing.T) {
	a := newRootedTestApp(t, time.Date(2026, 7, 5, 12, 0, 0, 0, time.UTC))
	a.runCommand("frobnicate")
	if got := a.statusLeft.GetText(true); got == "" {
		t.Error("expected a flash for an unknown command")
	}
}

func TestHelpOverlayOpensAndCloses(t *testing.T) {
	a := newRootedTestApp(t, time.Date(2026, 7, 5, 12, 0, 0, 0, time.UTC))
	a.showHelp()
	if !a.root.HasPage(pageHelp) {
		t.Fatal("help overlay did not open")
	}
	a.closeModal(pageHelp)
	if a.root.HasPage(pageHelp) {
		t.Error("help overlay did not close")
	}
}
