package state_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/littekge/LazyPlanner/internal/state"
)

func TestRoundTrip(t *testing.T) {
	// Nested path exercises MkdirAll in Save.
	p := filepath.Join(t.TempDir(), "acct", state.FileName)

	if got := state.Load(p); got != (state.State{}) {
		t.Errorf("missing file loaded %+v, want zero", got)
	}
	if err := state.Save(p, state.State{LeftWidth: 33}); err != nil {
		t.Fatal(err)
	}
	if got := state.Load(p); got.LeftWidth != 33 {
		t.Errorf("LeftWidth = %d, want 33", got.LeftWidth)
	}
}

func TestLoadBadFileIsZero(t *testing.T) {
	p := filepath.Join(t.TempDir(), state.FileName)
	if err := os.WriteFile(p, []byte("{ not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := state.Load(p); got != (state.State{}) {
		t.Errorf("bad file loaded %+v, want zero (best-effort)", got)
	}
}
