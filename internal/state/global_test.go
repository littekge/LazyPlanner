package state_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/littekge/LazyPlanner/internal/state"
)

func TestGlobalRoundTrip(t *testing.T) {
	// Nested path exercises MkdirAll in SaveGlobal.
	p := filepath.Join(t.TempDir(), "data", state.GlobalFileName)

	if got := state.LoadGlobal(p); got.ActiveAccountID != "" {
		t.Errorf("missing file loaded %+v, want zero", got)
	}
	if err := state.SaveGlobal(p, state.Global{ActiveAccountID: "abc123def456"}); err != nil {
		t.Fatal(err)
	}
	got := state.LoadGlobal(p)
	if got.ActiveAccountID != "abc123def456" {
		t.Errorf("ActiveAccountID = %q, want abc123def456", got.ActiveAccountID)
	}
}

func TestLoadGlobalMissingIsZero(t *testing.T) {
	p := filepath.Join(t.TempDir(), state.GlobalFileName)
	if got := state.LoadGlobal(p); got.ActiveAccountID != "" {
		t.Errorf("missing file loaded %+v, want zero (best-effort)", got)
	}
}

func TestLoadGlobalCorruptIsZero(t *testing.T) {
	p := filepath.Join(t.TempDir(), state.GlobalFileName)
	if err := os.WriteFile(p, []byte("{ not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := state.LoadGlobal(p); got.ActiveAccountID != "" {
		t.Errorf("corrupt file loaded %+v, want zero (last-active must never block startup)", got)
	}
}
