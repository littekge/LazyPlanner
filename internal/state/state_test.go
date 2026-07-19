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

// TestLoadPartialParseThenErrorIsZero closes the Pass-17 canary escape on Load's
// json.Unmarshal error check. TestLoadBadFileIsZero ("{ not json") fails
// Unmarshal before any mutation, so it passes whether or not the error is
// checked — dropping the check escaped the suite.
//
// The trailing-garbage repro the canary suggested would ALSO have escaped:
// json.Unmarshal runs checkValid over the whole input first, so trailing garbage
// after a valid object is rejected before any decode and leaves the struct zero.
// The case that actually requires the error check is a valid-JSON object with a
// *type mismatch on a later field* (here hidden_calendars: a number where a
// []string is expected): checkValid passes, the decoder populates the earlier
// left_width field, then records the type error and returns it — leaving a
// half-populated struct. With the check, Load must reject it and return a zero
// State; without it, Load would surface the partial {LeftWidth:5}.
func TestLoadPartialParseThenErrorIsZero(t *testing.T) {
	p := filepath.Join(t.TempDir(), state.FileName)
	if err := os.WriteFile(p, []byte(`{"left_width":5,"hidden_calendars":123}`), 0o600); err != nil {
		t.Fatal(err)
	}
	got := state.Load(p)
	if got.LeftWidth != 0 || got.RowsPerHour != 0 || got.DetailWidth != 0 || len(got.HiddenCalendars) != 0 {
		t.Errorf("partial-parse-then-error file loaded %+v, want zero State (a decode error must reject the whole file)", got)
	}
}
