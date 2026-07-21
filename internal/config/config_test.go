package config

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
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
[[account]]
name = "personal"
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
	if len(cfg.Accounts) != 1 {
		t.Fatalf("Accounts = %d, want 1", len(cfg.Accounts))
	}
	acct := cfg.Accounts[0]
	if acct.Name != "personal" {
		t.Errorf("account name = %q, want personal", acct.Name)
	}
	if !acct.Configured() {
		t.Error("account should be configured")
	}
	if acct.URL != "https://nc.example.com/remote.php/dav" || acct.Username != "jdoe" {
		t.Errorf("account connection = %+v", acct.Server)
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

// TestLoadCapsReadSize closes pass-13 canary escape #4: the config read is bounded
// by io.LimitReader(maxConfigBytes) so a huge/endless file can't exhaust memory or
// hang startup. It's crafted so the bytes before the cap are valid TOML and the
// bytes after it are NOT — a capped read parses cleanly, but dropping the cap reads
// the whole file and hits the trailing garbage, turning Load into a syntax error.
func TestLoadCapsReadSize(t *testing.T) {
	dir := withConfigDir(t)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	var b strings.Builder
	b.WriteString("[[account]]\nname = \"main\"\nurl = \"https://nc.example.com\"\nusername = \"u\"\n")
	// One long comment line that runs past the cap; a TOML comment needs no closing
	// newline, so a read truncated mid-comment is still valid.
	b.WriteString("# ")
	b.WriteString(strings.Repeat("x", maxConfigBytes))
	// Beyond the cap: a bare line that is invalid TOML if it is ever read.
	b.WriteString("\n!!! this line is not valid toml !!!\n")
	if err := os.WriteFile(filepath.Join(dir, configName), []byte(b.String()), 0o600); err != nil {
		t.Fatal(err)
	}

	cfg, configured, _, err := Load()
	if err != nil {
		t.Fatalf("Load failed on an oversized file; the read cap should have truncated before the trailing garbage: %v", err)
	}
	if !configured || len(cfg.Accounts) != 1 || cfg.Accounts[0].Username != "u" {
		t.Errorf("expected the pre-cap [[account]] block to parse, got configured=%v accounts=%+v", configured, cfg.Accounts)
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
	if err := os.WriteFile(path, []byte("[[account]]\nname=\"m\"\nurl=\"x\"\n"), 0o644); err != nil {
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
	got, err := s.ResolvePassword(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if got != "from-cmd" {
		t.Errorf("ResolvePassword = %q, want from-cmd", got)
	}

	// Falls back to inline when no command is set.
	s2 := Server{Password: "inline"}
	got2, err := s2.ResolvePassword(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if got2 != "inline" {
		t.Errorf("ResolvePassword = %q, want inline", got2)
	}

	// A failing command surfaces an error (not a silent empty password).
	if _, err := (Server{PasswordCommand: "exit 3"}).ResolvePassword(context.Background()); err == nil {
		t.Error("failing password_command returned no error")
	}

	// M6: exit 0 with no output is an error, not a silent empty password.
	if _, err := (Server{PasswordCommand: "true"}).ResolvePassword(context.Background()); err == nil {
		t.Error("empty password_command output returned no error")
	}

	// M6: a hung command is bounded by the context — it returns an error promptly
	// rather than blocking for the command's full duration. ("exec" so the deadline
	// kill lands on sleep directly, keeping the test fast; WaitDelay additionally
	// bounds the exotic case where a command orphans a child holding stdout open.)
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	start := time.Now()
	if _, err := (Server{PasswordCommand: "exec sleep 30"}).ResolvePassword(ctx); err == nil {
		t.Error("hung password_command returned no error")
	}
	if elapsed := time.Since(start); elapsed > 5*time.Second {
		t.Errorf("hung password_command not bounded: took %v", elapsed)
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
	body := "[[account]]\nname=\"m\"\nurl=\"https://x/dav\"\nusername=\"u\"\n[appearance]\ndefault_view=\"wek\"\ntime_format=\"bogus\"\n"
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

// TestLoadMalformedTOMLIsActionableError guards M1: a malformed config.toml is a
// fatal-by-design error (the account-keyed cache path is unknown without a
// parseable config), and the message must name the file and be actionable.
func TestLoadMalformedTOMLIsActionableError(t *testing.T) {
	dir := withConfigDir(t)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	// A stray unquoted value / dangling bracket — invalid TOML.
	body := "[server\nurl = not valid = toml\n"
	if err := os.WriteFile(filepath.Join(dir, configName), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	_, configured, _, err := Load()
	if err == nil {
		t.Fatal("Load() = nil error for malformed TOML, want a fatal error")
	}
	if configured {
		t.Error("configured = true for malformed TOML")
	}
	if !strings.Contains(err.Error(), configName) {
		t.Errorf("error %q does not name the config file", err)
	}
}

// TestServerConfigured pins that a CalDAV server counts as configured only when
// BOTH the URL and username are present — a partial config (one field set) must
// return false, or sync would run against an incomplete connection. Guards the
// pass-16 canary: flipping the && to || made a URL-only config read as configured.
func TestServerConfigured(t *testing.T) {
	cases := []struct {
		name string
		srv  Server
		want bool
	}{
		{"both set", Server{URL: "https://host/dav", Username: "me"}, true},
		{"url only", Server{URL: "https://host/dav"}, false},
		{"username only", Server{Username: "me"}, false},
		{"neither", Server{}, false},
	}
	for _, tc := range cases {
		if got := tc.srv.Configured(); got != tc.want {
			t.Errorf("%s: Configured() = %v, want %v", tc.name, got, tc.want)
		}
	}
}

// writeConfig writes body to the temp config path and returns the ConfigDir.
func writeConfig(t *testing.T, body string) {
	t.Helper()
	dir := withConfigDir(t)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, configName), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

// TestLoadParsesMultipleAccounts pins that several [[account]] blocks parse into
// Config.Accounts in file order, each carrying its name and connection fields.
func TestLoadParsesMultipleAccounts(t *testing.T) {
	writeConfig(t, `
[[account]]
name = "personal"
url = "https://home.example.com/dav"
username = "me"

[[account]]
name = "work"
url = "https://work.example.com/dav"
username = "employee"
password_command = "bw get password work"
`)
	cfg, configured, _, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if !configured {
		t.Error("configured = false, want true")
	}
	if len(cfg.Accounts) != 2 {
		t.Fatalf("Accounts = %d, want 2", len(cfg.Accounts))
	}
	if cfg.Accounts[0].Name != "personal" || cfg.Accounts[0].URL != "https://home.example.com/dav" {
		t.Errorf("account[0] = %+v", cfg.Accounts[0])
	}
	if cfg.Accounts[1].Name != "work" || cfg.Accounts[1].PasswordCommand != "bw get password work" {
		t.Errorf("account[1] = %+v", cfg.Accounts[1])
	}
}

// TestLoadRejectsLegacyServerSection guards the migration boundary: the removed
// [server] section must fail Load with an actionable message, never be silently
// ignored (which would leave the app with zero accounts and no explanation).
func TestLoadRejectsLegacyServerSection(t *testing.T) {
	writeConfig(t, "[server]\nurl = \"https://x/dav\"\nusername = \"u\"\n")
	_, configured, _, err := Load()
	if err == nil {
		t.Fatal("Load() = nil error for a legacy [server] section, want a migration error")
	}
	if configured {
		t.Error("configured = true for a rejected legacy config")
	}
	if !strings.Contains(err.Error(), "server") || !strings.Contains(err.Error(), "account") {
		t.Errorf("error %q should mention the removed [server] section and [[account]]", err)
	}
}

// TestLoadRejectsNamelessAccount pins that an [[account]] with no name is a fatal
// error — the name is the switch key, so an unnamed account can't be selected.
func TestLoadRejectsNamelessAccount(t *testing.T) {
	writeConfig(t, "[[account]]\nurl = \"https://x/dav\"\nusername = \"u\"\n")
	_, _, _, err := Load()
	if err == nil {
		t.Fatal("Load() = nil error for a nameless account, want an error")
	}
	if !strings.Contains(err.Error(), "name") {
		t.Errorf("error %q should mention the missing name", err)
	}
}

// TestLoadRejectsDuplicateAccountNames pins that two accounts sharing a name
// (case-insensitively) is fatal — the picker/switch key must be unambiguous.
func TestLoadRejectsDuplicateAccountNames(t *testing.T) {
	writeConfig(t, `
[[account]]
name = "Work"
url = "https://a/dav"
username = "u"

[[account]]
name = "work"
url = "https://b/dav"
username = "u2"
`)
	_, _, _, err := Load()
	if err == nil {
		t.Fatal("Load() = nil error for duplicate account names, want an error")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "duplicate") {
		t.Errorf("error %q should flag the duplicate name", err)
	}
}

// TestLoadZeroAccountsIsOfflineNotError pins that a config with no accounts is a
// valid offline configuration (configured=true, no error) — the app runs over
// the cache with nothing to sync, matching a config whose account lacks a URL.
func TestLoadZeroAccountsIsOfflineNotError(t *testing.T) {
	writeConfig(t, "[appearance]\ntime_format = \"24h\"\n")
	cfg, configured, _, err := Load()
	if err != nil {
		t.Fatalf("zero-account config should load cleanly, got %v", err)
	}
	if !configured {
		t.Error("configured = false; a present-but-account-less file still counts as configured")
	}
	if len(cfg.Accounts) != 0 {
		t.Errorf("Accounts = %d, want 0", len(cfg.Accounts))
	}
}

// TestFirstAccount pins the trivial resolver used until the state file drives
// active-account selection: the first block, or false when none are configured.
func TestFirstAccount(t *testing.T) {
	cfg := Config{Accounts: []Account{
		{Name: "a", Server: Server{URL: "https://a/dav", Username: "u"}},
		{Name: "b", Server: Server{URL: "https://b/dav", Username: "u2"}},
	}}
	got, ok := cfg.FirstAccount()
	if !ok || got.Name != "a" {
		t.Errorf("FirstAccount() = %+v, %v; want account a", got, ok)
	}
	if _, ok := (Config{}).FirstAccount(); ok {
		t.Error("FirstAccount() ok = true for zero accounts, want false")
	}
}

// TestAccountID pins that an account derives the same cache id as the free
// AccountID function over its connection — the cache namespacing must not shift
// when a [server] config is migrated to a named [[account]] with the same URL.
func TestAccountID(t *testing.T) {
	a := Account{Name: "personal", Server: Server{URL: "https://nc.example.com/dav", Username: "jdoe"}}
	if a.ID() != AccountID(a.URL, a.Username) {
		t.Errorf("Account.ID() = %q, want %q", a.ID(), AccountID(a.URL, a.Username))
	}
}

func twoAccountConfig() Config {
	return Config{Accounts: []Account{
		{Name: "personal", Server: Server{URL: "https://home/dav", Username: "me"}},
		{Name: "work", Server: Server{URL: "https://work/dav", Username: "emp"}},
	}}
}

// TestResolveActiveAccount pins the startup resolver: the account whose id matches
// the stored last-active id wins; an empty or unmatched id falls back to the first
// block (a removed/renamed account can't strand the user on nothing).
func TestResolveActiveAccount(t *testing.T) {
	cfg := twoAccountConfig()
	workID := cfg.Accounts[1].ID()

	if got, ok := cfg.ResolveActiveAccount(workID); !ok || got.Name != "work" {
		t.Errorf("stored work id resolved to %+v/%v, want work", got, ok)
	}
	if got, ok := cfg.ResolveActiveAccount(""); !ok || got.Name != "personal" {
		t.Errorf("empty id resolved to %+v/%v, want the first block personal", got, ok)
	}
	if got, ok := cfg.ResolveActiveAccount("deadbeefcafe"); !ok || got.Name != "personal" {
		t.Errorf("unmatched id resolved to %+v/%v, want first-block fallback personal", got, ok)
	}
	if _, ok := (Config{}).ResolveActiveAccount("anything"); ok {
		t.Error("zero-account config resolved ok = true, want false")
	}
}

// TestAccountLookupByName pins the switch-target lookup used by :account: match by
// name, case-insensitively and trimmed, false when not found.
func TestAccountLookupByName(t *testing.T) {
	cfg := twoAccountConfig()
	if got, ok := cfg.Account("  WORK "); !ok || got.Name != "work" {
		t.Errorf("Account(\"  WORK \") = %+v/%v, want work (case-insensitive, trimmed)", got, ok)
	}
	if _, ok := cfg.Account("nonesuch"); ok {
		t.Error("Account(nonesuch) ok = true, want false")
	}
}
