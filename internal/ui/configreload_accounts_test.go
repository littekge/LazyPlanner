package ui

import (
	"strings"
	"testing"
	"time"
)

// TestConfigReloadRefreshesAccountList: a :config reload that adds an account
// must make it visible/selectable live — the picker/status read a.accounts, and
// :account must reach it — without a process restart (main.md v1.1.0 promises the
// reload re-parses the account list). Before the fix, applyConfigReload never
// touched a.accounts, so a freshly-added account flashed "unknown account".
func TestConfigReloadRefreshesAccountList(t *testing.T) {
	a := newTestApp(t, time.Date(2026, 7, 21, 12, 0, 0, 0, time.UTC))
	a.accounts = []string{"personal"}
	a.activeAccount = "personal"

	a.applyConfigReload(ConfigReload{
		Accounts:      []string{"personal", "work"},
		ActiveAccount: "personal",
	}, nil)

	if len(a.accounts) != 2 || a.accounts[1] != "work" {
		t.Fatalf("a.accounts = %v, want the reloaded [personal work]", a.accounts)
	}
	a.switchAccount("work")
	if a.switchTo != "work" {
		t.Errorf("switchTo = %q after reloading in account %q, want it reachable", a.switchTo, "work")
	}
}

// TestConfigReloadTracksRenamedActiveAccount: renaming the active account in
// :config keeps its cache id (URL+username unchanged), so the reload carries the
// new name; the status bar's "(active)" marker must follow it.
func TestConfigReloadTracksRenamedActiveAccount(t *testing.T) {
	a := newTestApp(t, time.Date(2026, 7, 21, 12, 0, 0, 0, time.UTC))
	a.accounts = []string{"personal", "work"}
	a.activeAccount = "personal"

	a.applyConfigReload(ConfigReload{
		Accounts:      []string{"home", "work"},
		ActiveAccount: "home",
	}, nil)

	if a.activeAccount != "home" {
		t.Errorf("activeAccount = %q, want the renamed home", a.activeAccount)
	}
}

// TestConfigReloadErrorLeavesAccountsUntouched: a failed reload (e.g. the active
// account's connection changed) flashes and must not clobber the live list.
func TestConfigReloadErrorLeavesAccountsUntouched(t *testing.T) {
	a := newTestApp(t, time.Date(2026, 7, 21, 12, 0, 0, 0, time.UTC))
	a.accounts = []string{"personal", "work"}
	a.activeAccount = "personal"

	a.applyConfigReload(ConfigReload{}, errReloadTest)

	if len(a.accounts) != 2 {
		t.Errorf("a.accounts = %v, want the pre-reload list preserved on error", a.accounts)
	}
	if got := a.statusLeft.GetText(true); !strings.Contains(strings.ToLower(got), "config") {
		t.Errorf("flash = %q, want a config error flash", got)
	}
}

var errReloadTest = errTest("reload failed")

type errTest string

func (e errTest) Error() string { return string(e) }
