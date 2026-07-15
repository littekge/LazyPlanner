package state

import (
	"os"
	"path/filepath"
	"testing"
)

// TestSaveWritesViaTempFile closes a pass-12 escaped-canary hole: Save's doc
// promises "writes to a temp file and renames so a crash never leaves a
// half-written state file," but no test asserted it — replacing the temp+rename
// with a direct os.WriteFile escaped the suite. The atomicity is what protects an
// offline-edit state file on a Pi/SD card from a crash mid-write.
//
// The tell: point Save at a path that is an existing DIRECTORY. temp+rename writes
// path+".tmp" (a regular file, succeeds) and only then fails at the rename onto a
// directory — leaving the temp file behind. A direct in-place write would instead
// fail immediately with no temp file. Both root- and platform-independent (a file
// cannot be renamed onto, or written as, a directory even as root).
func TestSaveWritesViaTempFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.json")
	if err := os.Mkdir(path, 0o700); err != nil {
		t.Fatal(err)
	}

	if err := Save(path, State{}); err == nil {
		t.Fatal("Save onto a directory path should fail")
	}
	if _, err := os.Stat(path + ".tmp"); err != nil {
		t.Errorf("Save did not write via a temp file (no %q after the rename failed): %v — the crash-atomic temp+rename was bypassed", path+".tmp", err)
	}
}
