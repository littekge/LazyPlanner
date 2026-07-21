package main

import (
	"errors"
	"path/filepath"
	"testing"

	"github.com/littekge/LazyPlanner/internal/config"
	"github.com/littekge/LazyPlanner/internal/state"
	"github.com/littekge/LazyPlanner/internal/ui"
)

func twoAccountCfg() config.Config {
	return config.Config{Accounts: []config.Account{
		{Name: "personal", Server: config.Server{URL: "https://home/dav", Username: "me"}},
		{Name: "work", Server: config.Server{URL: "https://work/dav", Username: "emp"}},
	}}
}

// TestRunTUILoopQuitPersistsActive: a plain quit runs the resolved account once
// and records it as last-active in the global state file.
func TestRunTUILoopQuitPersistsActive(t *testing.T) {
	cfg := twoAccountCfg()
	gp := filepath.Join(t.TempDir(), state.GlobalFileName)

	var calls []string
	err := runTUILoop(cfg, gp, func(a config.Account) (ui.RunResult, error) {
		calls = append(calls, a.Name)
		return ui.RunResult{}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(calls) != 1 || calls[0] != "personal" {
		t.Errorf("calls = %v, want [personal] (first-block resolution)", calls)
	}
	if got := state.LoadGlobal(gp).ActiveAccountID; got != cfg.Accounts[0].ID() {
		t.Errorf("persisted active id = %q, want personal's id", got)
	}
}

// TestRunTUILoopResolvesStoredAccount: a stored last-active id opens that account,
// not the first block.
func TestRunTUILoopResolvesStoredAccount(t *testing.T) {
	cfg := twoAccountCfg()
	gp := filepath.Join(t.TempDir(), state.GlobalFileName)
	if err := state.SaveGlobal(gp, state.Global{ActiveAccountID: cfg.Accounts[1].ID()}); err != nil {
		t.Fatal(err)
	}

	var calls []string
	if err := runTUILoop(cfg, gp, func(a config.Account) (ui.RunResult, error) {
		calls = append(calls, a.Name)
		return ui.RunResult{}, nil
	}); err != nil {
		t.Fatal(err)
	}
	if len(calls) != 1 || calls[0] != "work" {
		t.Errorf("calls = %v, want [work] (resolved from stored id)", calls)
	}
}

// TestRunTUILoopSwitch: a switch request reopens the named account and updates the
// persisted last-active id.
func TestRunTUILoopSwitch(t *testing.T) {
	cfg := twoAccountCfg()
	gp := filepath.Join(t.TempDir(), state.GlobalFileName)

	var calls []string
	i := 0
	err := runTUILoop(cfg, gp, func(a config.Account) (ui.RunResult, error) {
		calls = append(calls, a.Name)
		i++
		if i == 1 {
			return ui.RunResult{SwitchAccount: "work"}, nil
		}
		return ui.RunResult{}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(calls) != 2 || calls[0] != "personal" || calls[1] != "work" {
		t.Errorf("calls = %v, want [personal work]", calls)
	}
	if got := state.LoadGlobal(gp).ActiveAccountID; got != cfg.Accounts[1].ID() {
		t.Errorf("persisted active id = %q, want work's id after the switch", got)
	}
}

// TestRunTUILoopFallsBackWhenSwitchTargetFailsToOpen: if the switch target fails
// to open, the loop falls back to the previously-working account rather than
// exiting, and does not return an error.
func TestRunTUILoopFallsBackWhenSwitchTargetFailsToOpen(t *testing.T) {
	cfg := twoAccountCfg()
	gp := filepath.Join(t.TempDir(), state.GlobalFileName)

	var calls []string
	i := 0
	err := runTUILoop(cfg, gp, func(a config.Account) (ui.RunResult, error) {
		calls = append(calls, a.Name)
		i++
		switch i {
		case 1:
			return ui.RunResult{SwitchAccount: "work"}, nil // personal → switch to work
		case 2:
			return ui.RunResult{}, errors.New("store open failed") // work can't open
		default:
			return ui.RunResult{}, nil // personal reopened → quit
		}
	})
	if err != nil {
		t.Fatalf("fallback should recover without error, got %v", err)
	}
	if len(calls) != 3 || calls[2] != "personal" {
		t.Errorf("calls = %v, want [personal work personal] (fallback to the working account)", calls)
	}
}

// TestRunTUILoopInitialOpenErrorIsFatal: an error on the very first open (no
// previous account to fall back to) is returned, not swallowed.
func TestRunTUILoopInitialOpenErrorIsFatal(t *testing.T) {
	cfg := twoAccountCfg()
	gp := filepath.Join(t.TempDir(), state.GlobalFileName)

	want := errors.New("initial open failed")
	err := runTUILoop(cfg, gp, func(a config.Account) (ui.RunResult, error) {
		return ui.RunResult{}, want
	})
	if !errors.Is(err, want) {
		t.Errorf("err = %v, want the initial open error", err)
	}
}

// TestRunTUILoopUnknownSwitchTargetQuits: a switch request naming an account that
// isn't configured (a stale request) quits cleanly instead of looping or crashing.
func TestRunTUILoopUnknownSwitchTargetQuits(t *testing.T) {
	cfg := twoAccountCfg()
	gp := filepath.Join(t.TempDir(), state.GlobalFileName)

	calls := 0
	err := runTUILoop(cfg, gp, func(a config.Account) (ui.RunResult, error) {
		calls++
		return ui.RunResult{SwitchAccount: "ghost"}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if calls != 1 {
		t.Errorf("calls = %d, want 1 (unknown target must not reopen)", calls)
	}
}
