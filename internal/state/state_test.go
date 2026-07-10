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

	if got := state.Load(p); got.LeftWidth != 0 || len(got.HiddenCalendars) != 0 {
		t.Errorf("missing file loaded %+v, want zero", got)
	}
	if err := state.Save(p, state.State{LeftWidth: 33, HiddenCalendars: []string{"work"}, RowsPerHour: 3}); err != nil {
		t.Fatal(err)
	}
	got := state.Load(p)
	if got.LeftWidth != 33 {
		t.Errorf("LeftWidth = %d, want 33", got.LeftWidth)
	}
	if len(got.HiddenCalendars) != 1 || got.HiddenCalendars[0] != "work" {
		t.Errorf("HiddenCalendars = %v, want [work]", got.HiddenCalendars)
	}
	if got.RowsPerHour != 3 {
		t.Errorf("RowsPerHour = %d, want 3", got.RowsPerHour)
	}
}

func TestLoadBadFileIsZero(t *testing.T) {
	p := filepath.Join(t.TempDir(), state.FileName)
	if err := os.WriteFile(p, []byte("{ not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := state.Load(p); got.LeftWidth != 0 || len(got.HiddenCalendars) != 0 {
		t.Errorf("bad file loaded %+v, want zero (best-effort)", got)
	}
}
