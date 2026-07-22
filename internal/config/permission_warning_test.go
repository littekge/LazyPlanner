package config

import (
	"runtime"
	"testing"
)

// TestPermissionWarningFlagsGroupAndOther closes a Pass 18 canary hole: the
// permission warning masks 0o077 (any group- OR other-access), because the config
// may hold a password. A mutation narrowing the mask to 0o007 (other-only) would
// silently stop warning on a group-readable file — this pins the group bit.
func TestPermissionWarningFlagsGroupAndOther(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX permission bits only")
	}
	// Owner-only is fine — no warning.
	if w := permissionWarning("config.toml", 0o600); w != "" {
		t.Errorf("0o600 (owner-only) warned unexpectedly: %q", w)
	}
	// Group-readable must warn (this is the bit the 0o077→0o007 mutation drops).
	if w := permissionWarning("config.toml", 0o640); w == "" {
		t.Error("0o640 (group-readable) did not warn — a password-bearing config must warn on group access")
	}
	// Other-readable must warn too.
	if w := permissionWarning("config.toml", 0o604); w == "" {
		t.Error("0o604 (other-readable) did not warn")
	}
}
