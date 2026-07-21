package main

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/littekge/LazyPlanner/internal/config"
)

// TestConfigReloadPreservesLoadWarning guards the fix for editConfigFn having
// discarded the warning returned by config.Load() on a :config reload, so an
// appearance typo (default_view="wek") and a world-readable password file are
// never surfaced to the user. buildSyncFn's warning is empty because the (here
// unconfigured) server is still valid, so ui.ConfigReload.Warning ends up empty
// even though the reloaded config is objectively problematic.
func TestConfigReloadPreservesLoadWarning(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("permission warning is Unix-only")
	}

	// Point config resolution (os.UserConfigDir honors XDG_CONFIG_HOME on Linux)
	// at a temp dir holding a config with a typo and 0644 (group/other-readable)
	// perms — exactly what config.Load flags.
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)
	cfgDir := filepath.Join(tmp, "lazyplanner")
	if err := os.MkdirAll(cfgDir, 0o755); err != nil {
		t.Fatal(err)
	}
	cfgPath := filepath.Join(cfgDir, "config.toml")
	body := "[appearance]\ndefault_view = \"wek\"\n"
	if err := os.WriteFile(cfgPath, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}

	// Sanity: config.Load itself does produce a warning for this file.
	_, _, warning, err := config.Load()
	if err != nil {
		t.Fatalf("config.Load: %v", err)
	}
	if warning == "" {
		t.Fatal("precondition failed: expected config.Load to return a warning")
	}
	t.Logf("config.Load warning (what the user should be told): %q", warning)

	// Use a no-op editor so the reload closure runs without a real $EDITOR.
	t.Setenv("EDITOR", "true")

	// The run is offline (no configured account), so buildSyncFn returns warn=""
	// and the reload must still surface config.Load's own warning.
	edit := editConfigFn(cfgPath, nil, config.Account{}, nil)
	if edit == nil {
		t.Fatal("editConfigFn returned nil")
	}
	reload, err := edit()
	if err != nil {
		t.Fatalf("reload: %v", err)
	}

	if reload.Warning == "" {
		t.Fatalf("BUG reproduced: config.Load warning %q was discarded — reload.Warning is empty, "+
			"the appearance typo and world-readable password file are silently lost on :config", warning)
	}
}
