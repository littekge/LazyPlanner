package state_test

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/littekge/LazyPlanner/internal/state"
)

// TestSaveFilesAre0600 closes a Pass 18 canary hole: Save and SaveGlobal document
// a 0o600 file mode (the files sit under the data dir and the global one is
// account-adjacent), but the mode was unasserted. Both go through writeJSONFile,
// so this pins the shared 0o600 contract against a mode-widening mutation.
func TestSaveFilesAre0600(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX permission bits only")
	}
	dir := t.TempDir()

	sp := filepath.Join(dir, "sub", "state.json")
	if err := state.Save(sp, state.State{}); err != nil {
		t.Fatal(err)
	}
	assertMode0600(t, sp)

	gp := filepath.Join(dir, "global.json")
	if err := state.SaveGlobal(gp, state.Global{}); err != nil {
		t.Fatal(err)
	}
	assertMode0600(t, gp)
}

func assertMode0600(t *testing.T, path string) {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Errorf("%s mode = %#o, want 0o600 (may hold connection state)", path, got)
	}
}
