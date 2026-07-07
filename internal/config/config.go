package config

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/BurntSushi/toml"
)

// configName is the config file LazyPlanner reads under ConfigDir.
const configName = "config.toml"

// accountIDLen is the number of hex characters kept from the account hash. 12
// hex chars (48 bits) is ample to keep two personal accounts from colliding
// while staying short in the on-disk path.
const accountIDLen = 12

// Config is the parsed contents of config.toml. The app reads it once at startup
// and never writes it (see main.md) — calendar names/colors are server-owned
// data, and app-remembered state lives in the data dir, not here.
type Config struct {
	Server     Server     `toml:"server"`
	Appearance Appearance `toml:"appearance"`
	Behavior   Behavior   `toml:"behavior"`
}

// Server holds the CalDAV connection. Credentials are always a NextCloud app
// password, never the account password.
type Server struct {
	URL      string `toml:"url"`
	Username string `toml:"username"`
	// Password is the app password inline. Prefer PasswordCommand to keep the
	// secret out of the file.
	Password string `toml:"password"`
	// PasswordCommand, when set, is run and its trimmed stdout used as the
	// password (e.g. "bw get password lazyplanner" with Vaultwarden). It takes
	// precedence over Password.
	PasswordCommand string `toml:"password_command"`
}

// Appearance controls how things are displayed. Every field defaults to the
// owner's preference when unset, so a working config needs only [server].
type Appearance struct {
	// FirstDayOfWeek is "monday" (default) or "sunday".
	FirstDayOfWeek string `toml:"first_day_of_week"`
	// DefaultView is the calendar view on open: "month" (default), "week", "day".
	DefaultView string `toml:"default_view"`
	// TimeFormat is "12h" (default, 2:30pm) or "24h" (14:30).
	TimeFormat string `toml:"time_format"`
	// DateFormat is "us" (default, 07/04/2026) or "iso" (2026-07-04).
	DateFormat string `toml:"date_format"`
}

// Behavior controls non-visual behavior.
type Behavior struct {
	// SyncIntervalMinutes is the periodic background-sync interval; 0 disables
	// it. Default 15. (Periodic sync itself is wired in a later build step; the
	// value is read now so the config schema is stable.)
	SyncIntervalMinutes int `toml:"sync_interval_minutes"`
}

// Default returns a Config with every option set to the owner's preferred
// default. Load starts from this and overlays the file, so any option the file
// omits keeps its default.
func Default() Config {
	return Config{
		Appearance: Appearance{
			FirstDayOfWeek: "monday",
			DefaultView:    "month",
			TimeFormat:     "12h",
			DateFormat:     "us",
		},
		Behavior: Behavior{
			SyncIntervalMinutes: 15,
		},
	}
}

// Load reads config.toml from ConfigDir, overlaying it on the defaults. A
// missing file is not an error: it returns the defaults with configured=false so
// the caller can generate a first-run template. Values the file omits keep their
// default. A too-permissive file (group/other-readable, since it may hold a
// password) yields a non-fatal warning string.
func Load() (cfg Config, configured bool, warning string, err error) {
	dir, err := ConfigDir()
	if err != nil {
		return Config{}, false, "", err
	}
	path := filepath.Join(dir, configName)

	cfg = Default()
	info, statErr := os.Stat(path)
	if errors.Is(statErr, os.ErrNotExist) {
		return cfg, false, "", nil
	}
	if statErr != nil {
		return Config{}, false, "", fmt.Errorf("stat config %q: %w", path, statErr)
	}

	if w := permissionWarning(path, info.Mode()); w != "" {
		warning = w
	}

	if _, err := toml.DecodeFile(path, &cfg); err != nil {
		return Config{}, false, warning, fmt.Errorf("parsing config %q: %w", path, err)
	}
	return cfg, true, warning, nil
}

// permissionWarning returns a warning if the config file is readable by group or
// other on a POSIX system — it may hold an app password and should be 0600.
// Windows ACLs don't map onto these bits, so the check is Unix-only.
func permissionWarning(path string, mode os.FileMode) string {
	if runtime.GOOS == "windows" {
		return ""
	}
	if mode.Perm()&0o077 != 0 {
		return fmt.Sprintf("config file %s is %#o; it may hold a password — chmod 600 it", path, mode.Perm())
	}
	return ""
}

// ResolvePassword returns the effective app password: the output of
// PasswordCommand if set, otherwise the inline Password. It is called at connect
// time (not during Load) so a failing command surfaces only when sync is
// actually attempted.
func (s Server) ResolvePassword() (string, error) {
	cmd := strings.TrimSpace(s.PasswordCommand)
	if cmd == "" {
		return s.Password, nil
	}
	// Run through the shell so command strings like "bw get password foo" work
	// as written in the config, matching the owner's Vaultwarden setup.
	out, err := exec.Command("sh", "-c", cmd).Output()
	if err != nil {
		return "", fmt.Errorf("running password_command: %w", err)
	}
	return strings.TrimSpace(string(out)), nil
}

// Configured reports whether the server connection is filled in enough to
// attempt a sync.
func (s Server) Configured() bool {
	return s.URL != "" && s.Username != ""
}

// AccountID derives a stable, opaque id for a CalDAV account from its server URL
// and username. The local cache is namespaced by this id so changing the server
// connection maps to a separate cache and two accounts' data can never share one
// directory (see the account-model decision in main.md). It is a hash — the
// sidecar's ETags/hrefs are meaningful only against the account that issued
// them, so isolation matters more than readability.
func AccountID(url, username string) string {
	sum := sha256.Sum256([]byte(normalizeURL(url) + "\x00" + strings.ToLower(strings.TrimSpace(username))))
	return hex.EncodeToString(sum[:])[:accountIDLen]
}

// normalizeURL lowercases and trims a URL so trivial spelling differences
// (trailing slash, case) map to the same account.
func normalizeURL(url string) string {
	return strings.ToLower(strings.TrimRight(strings.TrimSpace(url), "/"))
}

// AccountDataDir returns the account-namespaced data directory for the given
// server URL and username: <dataDir>/<account-id>. The store's calendar cache
// lives under its "calendars" subdirectory. The directory is not created.
func AccountDataDir(url, username string) (string, error) {
	base, err := DataDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, AccountID(url, username)), nil
}
