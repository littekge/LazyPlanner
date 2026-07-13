package state

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestLoadCapsEndlessFile guards the local-read size cap: a state file that is a
// symlink to an endless device (/dev/zero) must not hang or OOM Load — the capped
// read returns bounded garbage, which fails to parse, yielding a zero State.
func TestLoadCapsEndlessFile(t *testing.T) {
	if _, err := os.Stat("/dev/zero"); err != nil {
		t.Skip("/dev/zero not available")
	}
	path := filepath.Join(t.TempDir(), "state.json")
	if err := os.Symlink("/dev/zero", path); err != nil {
		t.Skipf("cannot symlink: %v", err)
	}

	done := make(chan State, 1)
	go func() { done <- Load(path) }()
	select {
	case s := <-done:
		if s.LeftWidth != 0 || s.DetailWidth != 0 || s.RowsPerHour != 0 || len(s.HiddenCalendars) != 0 {
			t.Errorf("expected zero State from an unparseable capped read, got %+v", s)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("Load hung on an endless file — the size cap is not bounding the read")
	}
}
