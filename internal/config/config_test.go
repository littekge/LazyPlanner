package config

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// withConfigDir points ConfigDir at a temp directory for the duration of a test
// by setting the OS-appropriate environment variable os.UserConfigDir reads.
func withConfigDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	switch runtime.GOOS {
	case "windows":
		t.Setenv("AppData", dir)
	case "darwin":
		// os.UserConfigDir uses $HOME/Library/Application Support on darwin.
		t.Setenv("HOME", dir)
		return filepath.Join(dir, "Library", "Application Support", appDir)
	default:
		t.Setenv("XDG_CONFIG_HOME", dir)
	}
	return filepath.Join(dir, appDir)
}

func TestLoadMissingReturnsDefaults(t *testing.T) {
	withConfigDir(t)
	cfg, configured, warning, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if configured {
		t.Error("configured = true for a missing file, want false")
	}
	if warning != "" {
		t.Errorf("warning = %q, want none", warning)
	}
	if cfg.Appearance.FirstDayOfWeek != "monday" || cfg.Behavior.SyncIntervalMinutes != 15 {
		t.Errorf("defaults not applied: %+v", cfg)
	}
}

func TestLoadOverlaysFile(t *testing.T) {
	dir := withConfigDir(t)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	body := `
[server]
url = "https://nc.example.com/remote.php/dav"
username = "jdoe"
password_command = "echo hunter2"

[appearance]
time_format = "24h"
`
	if err := os.WriteFile(filepath.Join(dir, configName), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}

	cfg, configured, warning, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if !configured {
		t.Error("configured = false, want true")
	}
	if warning != "" {
		t.Errorf("warning = %q for a 0600 file, want none", warning)
	}
	if !cfg.Server.Configured() {
		t.Error("server should be configured")
	}
	if cfg.Server.URL != "https://nc.example.com/remote.php/dav" || cfg.Server.Username != "jdoe" {
		t.Errorf("server = %+v", cfg.Server)
	}
	// File overrides the default...
	if cfg.Appearance.TimeFormat != "24h" {
		t.Errorf("TimeFormat = %q, want 24h", cfg.Appearance.TimeFormat)
	}
	// ...but omitted options keep their default.
	if cfg.Appearance.FirstDayOfWeek != "monday" {
		t.Errorf("FirstDayOfWeek = %q, want the default monday", cfg.Appearance.FirstDayOfWeek)
	}
	if cfg.Appearance.ColorMode != "auto" {
		t.Errorf("ColorMode = %q, want the default auto", cfg.Appearance.ColorMode)
	}
}

func TestLoadWarnsOnLoosePermissions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("permission bits are POSIX-only")
	}
	dir := withConfigDir(t)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, configName)
	if err := os.WriteFile(path, []byte("[server]\nurl=\"x\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, _, warning, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if warning == "" || !strings.Contains(warning, "600") {
		t.Errorf("warning = %q, want a chmod-600 hint", warning)
	}
}

func TestResolvePassword(t *testing.T) {
	// PasswordCommand takes precedence and is trimmed.
	s := Server{Password: "inline", PasswordCommand: "printf 'from-cmd\\n'"}
	got, err := s.ResolvePassword()
	if err != nil {
		t.Fatal(err)
	}
	if got != "from-cmd" {
		t.Errorf("ResolvePassword = %q, want from-cmd", got)
	}

	// Falls back to inline when no command is set.
	s2 := Server{Password: "inline"}
	got2, err := s2.ResolvePassword()
	if err != nil {
		t.Fatal(err)
	}
	if got2 != "inline" {
		t.Errorf("ResolvePassword = %q, want inline", got2)
	}
}

func TestAccountIDStableAndDistinct(t *testing.T) {
	// Stable and insensitive to trailing slash / case in URL and username.
	a := AccountID("https://nc.example.com/remote.php/dav/", "JDoe")
	b := AccountID("https://NC.example.com/remote.php/dav", "jdoe")
	if a != b {
		t.Errorf("AccountID not normalized: %q vs %q", a, b)
	}
	if len(a) != accountIDLen {
		t.Errorf("AccountID length = %d, want %d", len(a), accountIDLen)
	}
	// Different account -> different id.
	if AccountID("https://other.example.com/dav", "jdoe") == a {
		t.Error("different server produced the same account id")
	}
}

func TestGenerateDefault(t *testing.T) {
	dir := withConfigDir(t)
	path, written, err := GenerateDefault()
	if err != nil {
		t.Fatal(err)
	}
	if !written {
		t.Fatal("written = false on first generation")
	}
	if path != filepath.Join(dir, configName) {
		t.Errorf("path = %q", path)
	}

	// The generated file must parse and re-loading it must succeed (a broken
	// template would be a nasty first-run experience).
	if _, _, _, err := Load(); err != nil {
		t.Fatalf("generated config does not load: %v", err)
	}
	if runtime.GOOS != "windows" {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm() != 0o600 {
			t.Errorf("generated config mode = %#o, want 0600", info.Mode().Perm())
		}
	}

	// A second call must not overwrite an existing file.
	_, written2, err := GenerateDefault()
	if err != nil {
		t.Fatal(err)
	}
	if written2 {
		t.Error("GenerateDefault overwrote an existing config")
	}
}

func TestLoadWarnsOnUnknownAppearance(t *testing.T) {
	dir := withConfigDir(t)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	body := "[server]\nurl=\"https://x/dav\"\nusername=\"u\"\n[appearance]\ndefault_view=\"wek\"\ntime_format=\"bogus\"\n"
	if err := os.WriteFile(filepath.Join(dir, configName), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	_, _, warning, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(warning, "default_view") || !strings.Contains(warning, "time_format") {
		t.Errorf("warning = %q, want it to name the unknown default_view and time_format", warning)
	}
}
