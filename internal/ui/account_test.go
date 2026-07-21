package ui

import (
	"strings"
	"testing"
	"time"
)

func multiAccountApp(t *testing.T) *app {
	t.Helper()
	a := newTestApp(t, time.Date(2026, 7, 21, 12, 0, 0, 0, time.UTC))
	a.accounts = []string{"personal", "work"}
	a.activeAccount = "personal"
	return a
}

// TestSwitchAccountValidRequestsSwitch: switching to a different configured
// account records the request (which Run returns so main reopens it).
func TestSwitchAccountValidRequestsSwitch(t *testing.T) {
	a := multiAccountApp(t)
	a.switchAccount("work")
	if a.switchTo != "work" {
		t.Errorf("switchTo = %q, want work", a.switchTo)
	}
}

// TestSwitchAccountCaseInsensitive: the request records the configured spelling,
// so main's by-name lookup matches.
func TestSwitchAccountCaseInsensitive(t *testing.T) {
	a := multiAccountApp(t)
	a.switchAccount("  WORK ")
	if a.switchTo != "work" {
		t.Errorf("switchTo = %q, want the configured spelling work", a.switchTo)
	}
}

// TestSwitchAccountActiveIsNoop: switching to the already-active account does not
// request a switch (no needless teardown), and says so.
func TestSwitchAccountActiveIsNoop(t *testing.T) {
	a := multiAccountApp(t)
	a.switchAccount("personal")
	if a.switchTo != "" {
		t.Errorf("switchTo = %q, want empty (already active)", a.switchTo)
	}
	if got := a.statusLeft.GetText(true); !strings.Contains(strings.ToLower(got), "already") {
		t.Errorf("flash = %q, want an already-on message", got)
	}
}

// TestSwitchAccountUnknownFlashes: an unknown name does not switch and flashes.
func TestSwitchAccountUnknownFlashes(t *testing.T) {
	a := multiAccountApp(t)
	a.switchAccount("ghost")
	if a.switchTo != "" {
		t.Errorf("switchTo = %q, want empty for an unknown account", a.switchTo)
	}
	if got := a.statusLeft.GetText(true); !strings.Contains(strings.ToLower(got), "unknown") {
		t.Errorf("flash = %q, want an unknown-account message", got)
	}
}

// TestRunCommandAccountDispatch: ":account work" routed through runCommand reaches
// the switch, guarding the command-table wiring.
func TestRunCommandAccountDispatch(t *testing.T) {
	a := multiAccountApp(t)
	a.runCommand("account work")
	if a.switchTo != "work" {
		t.Errorf("switchTo = %q after :account work, want work", a.switchTo)
	}
}

// TestCmdAccountNoAccountsFlashes: :account with no configured accounts (offline)
// flashes rather than opening an empty picker or switching.
func TestCmdAccountNoAccountsFlashes(t *testing.T) {
	a := newTestApp(t, time.Date(2026, 7, 21, 12, 0, 0, 0, time.UTC))
	a.cmdAccount("")
	if got := a.statusLeft.GetText(true); !strings.Contains(strings.ToLower(got), "no accounts") {
		t.Errorf("flash = %q, want a no-accounts message", got)
	}
}

// TestStatusShowsActiveAccountWhenMultiple: the status bar names the active
// account only when more than one account is configured.
func TestStatusShowsActiveAccountWhenMultiple(t *testing.T) {
	a := multiAccountApp(t)
	a.updateStatus()
	if got := a.statusLeft.GetText(true); !strings.Contains(got, "personal") {
		t.Errorf("status = %q, want it to name the active account personal", got)
	}

	// A single-account (or offline) run must not clutter the status bar.
	a.accounts = []string{"personal"}
	a.updateStatus()
	if got := a.statusLeft.GetText(true); strings.Contains(got, "personal") {
		t.Errorf("status = %q, should not name the account when only one is configured", got)
	}
}
