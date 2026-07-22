package config

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/BurntSushi/toml"
)

// maxConfigBytes bounds the config-file read so a huge or endless file can't
// exhaust memory or hang startup. A real config is well under a kilobyte.
const maxConfigBytes = 4 << 20

// maxTOMLNestingDepth caps the structural nesting (inline tables {} and arrays [])
// the config decoder will accept. BurntSushi/toml decodes deeply nested inline
// tables in O(depth^2) time, so a crafted/corrupt config of only a few KB — well
// under maxConfigBytes — can hang Load() for minutes (measured ~1s at depth 4000).
// A real config nests at most a couple of levels; 64 is far above any legitimate
// config yet keeps decode cost trivial (64^2 is microseconds).
const maxTOMLNestingDepth = 64

// configName is the config file LazyPlanner reads under ConfigDir.
const configName = "config.toml"

// accountIDLen is the number of hex characters kept from the account hash. 12
// hex chars (48 bits) is ample to keep two personal accounts from colliding
// while staying short in the on-disk path.
const accountIDLen = 12

// defaultSyncIntervalMinutes is the periodic background-sync cadence used when
// the config omits sync_interval_minutes. 15 minutes balances freshness against
// server load for a personal CalDAV account.
const defaultSyncIntervalMinutes = 15

// Config is the parsed contents of config.toml. The app reads it once at startup
// and never writes it (see main.md) — calendar names/colors are server-owned
// data, and app-remembered state lives in the data dir, not here.
type Config struct {
	Accounts   []Account  `toml:"account"`
	Appearance Appearance `toml:"appearance"`
	Behavior   Behavior   `toml:"behavior"`
}

// Account is one configured CalDAV account: a unique display Name (the label the
// :account switcher and status bar use) plus its connection. It embeds Server so
// the connection fields and their credential logic (ResolvePassword, Configured)
// are shared with the single-account plumbing.
type Account struct {
	Name string `toml:"name"`
	Server
}

// ID returns the account's cache-namespacing id, derived from its connection.
// The vdir cache lives under this id (see AccountID), so it is the stable key
// used to remember which account was last active across launches.
func (a Account) ID() string {
	return AccountID(a.URL, a.Username)
}

// FirstAccount returns the first configured account, or false when none are
// configured (a fully-offline run).
func (c Config) FirstAccount() (Account, bool) {
	if len(c.Accounts) == 0 {
		return Account{}, false
	}
	return c.Accounts[0], true
}

// ResolveActiveAccount picks the account to open at startup: the one whose ID
// matches activeID (the last-active id from the global state file), else the
// first configured account. Falling back to the first block means a removed or
// renamed account can't strand the user on nothing. Returns false only when no
// accounts are configured (an offline run).
func (c Config) ResolveActiveAccount(activeID string) (Account, bool) {
	if activeID != "" {
		for _, a := range c.Accounts {
			if a.ID() == activeID {
				return a, true
			}
		}
	}
	return c.FirstAccount()
}

// Account returns the configured account with the given name, matched
// case-insensitively after trimming (names are unique per validateAccounts). It
// is the switch-target lookup for the :account command; false when not found.
func (c Config) Account(name string) (Account, bool) {
	want := strings.ToLower(strings.TrimSpace(name))
	for _, a := range c.Accounts {
		if strings.ToLower(strings.TrimSpace(a.Name)) == want {
			return a, true
		}
	}
	return Account{}, false
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
	// ColorMode is how server calendar colors render: "auto" (default; exact
	// truecolor, downsampled by the terminal), "truecolor" (force-enable
	// truecolor for terminals that underreport), "16" (nearest themed ANSI
	// color, inherits the terminal theme), or "off" (no calendar colors).
	ColorMode string `toml:"color_mode"`
}

// Behavior controls non-visual behavior.
type Behavior struct {
	// SyncIntervalMinutes is the periodic background-sync interval; 0 disables
	// it. Default 15.
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
			ColorMode:      "auto",
		},
		Behavior: Behavior{
			SyncIntervalMinutes: defaultSyncIntervalMinutes,
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

	var warns []string
	if w := permissionWarning(path, info.Mode()); w != "" {
		warns = append(warns, w)
	}

	// Read with a size cap so a huge or endless (e.g. a /dev/zero symlink) config
	// can't exhaust memory or hang startup, then parse the bytes.
	f, err := os.Open(path)
	if err != nil {
		return Config{}, false, strings.Join(warns, "; "), fmt.Errorf("opening config %q: %w", path, err)
	}
	data, err := io.ReadAll(io.LimitReader(f, maxConfigBytes))
	f.Close()
	if err != nil {
		return Config{}, false, strings.Join(warns, "; "), fmt.Errorf("reading config %q: %w", path, err)
	}
	// Bound the decode before running it: BurntSushi/toml decodes deeply nested
	// inline tables/arrays in O(depth^2), so a crafted/corrupt config of only a few
	// KB — well under maxConfigBytes — would otherwise hang Load() (and thus
	// startup) for minutes. The byte cap bounds size, not decode CPU; this bounds
	// nesting. A real config nests at most a couple of levels.
	if derr := checkNestingDepth(data); derr != nil {
		return Config{}, false, strings.Join(warns, "; "),
			fmt.Errorf("config %q %w — fix it and run lazyplanner again", path, derr)
	}
	meta, err := toml.Decode(string(data), &cfg)
	if err != nil {
		// Fatal by design: the local cache is namespaced by account (server URL +
		// username), so an unparseable config leaves the account — and thus which
		// cache to open — unknown. Degrading to defaults would open an empty/wrong
		// account cache, which is more confusing than a clear, actionable error.
		return Config{}, false, strings.Join(warns, "; "),
			fmt.Errorf("config %q has a syntax error — fix it and run lazyplanner again: %w", path, err)
	}
	// The single [server] section was replaced by named [[account]] blocks. A
	// leftover [server] would otherwise be silently ignored (leaving zero accounts
	// with no explanation), so reject it with an actionable migration message.
	if meta.IsDefined("server") {
		return Config{}, false, strings.Join(warns, "; "),
			fmt.Errorf("config %q uses the removed [server] section — rename it to a [[account]] block and give it a name (see the README migration note), then run lazyplanner again", path)
	}
	if verr := validateAccounts(cfg.Accounts); verr != nil {
		return Config{}, false, strings.Join(warns, "; "), fmt.Errorf("config %q: %w", path, verr)
	}
	warns = append(warns, appearanceWarnings(cfg.Appearance)...)
	return cfg, true, strings.Join(warns, "; "), nil
}

// checkNestingDepth scans raw TOML and rejects it if structural bracket nesting
// (inline tables {} and arrays []) exceeds maxTOMLNestingDepth, guarding the
// O(depth^2) decoder against a hang. Brackets inside strings and comments do not
// count. It is a deliberately conservative pre-check, not a full parser: on
// malformed input it errs toward accepting (and letting toml.Decode report the
// real error), but a well-formed config's real nesting is shallow, so it never
// rejects one.
func checkNestingDepth(data []byte) error {
	depth := 0
	for i, n := 0, len(data); i < n; {
		switch data[i] {
		case '#': // comment to end of line
			for i < n && data[i] != '\n' {
				i++
			}
		case '"':
			if i+2 < n && data[i+1] == '"' && data[i+2] == '"' {
				i = skipDelimited(data, i+3, '"', true, true)
			} else {
				i = skipDelimited(data, i+1, '"', true, false)
			}
		case '\'':
			if i+2 < n && data[i+1] == '\'' && data[i+2] == '\'' {
				i = skipDelimited(data, i+3, '\'', false, true)
			} else {
				i = skipDelimited(data, i+1, '\'', false, false)
			}
		case '{', '[':
			depth++
			if depth > maxTOMLNestingDepth {
				return fmt.Errorf("is nested more than %d levels deep — this looks corrupt or maliciously crafted (a real config nests only a couple of levels)", maxTOMLNestingDepth)
			}
			i++
		case '}', ']':
			if depth > 0 {
				depth--
			}
			i++
		default:
			i++
		}
	}
	return nil
}

// skipDelimited returns the index just past a TOML string that opened at i. quote
// is the delimiter (" or '); escapes honors backslash escapes (basic strings only);
// multiline treats the delimiter tripled as the terminator (and spans newlines). A
// single-line string that hits a newline before its close is treated as ended
// there (malformed input — toml.Decode reports it; the scan just must not run away).
func skipDelimited(data []byte, i int, quote byte, escapes, multiline bool) int {
	for n := len(data); i < n; {
		c := data[i]
		if escapes && c == '\\' {
			i += 2
			continue
		}
		if multiline {
			if c == quote && i+2 < n && data[i+1] == quote && data[i+2] == quote {
				return i + 3
			}
		} else {
			if c == quote {
				return i + 1
			}
			if c == '\n' {
				return i
			}
		}
		i++
	}
	return i
}

// validateAccounts enforces that every [[account]] has a non-empty name and that
// names are unique (case-insensitively) — the name is the switch key, so a
// nameless or ambiguous account can't be selected. Zero accounts is valid (a
// fully-offline run), so an empty slice passes.
func validateAccounts(accounts []Account) error {
	seen := make(map[string]bool, len(accounts))
	for i, a := range accounts {
		name := strings.TrimSpace(a.Name)
		if name == "" {
			return fmt.Errorf("account #%d has no name — every [[account]] needs a unique name", i+1)
		}
		key := strings.ToLower(name)
		if seen[key] {
			return fmt.Errorf("duplicate account name %q — account names must be unique", name)
		}
		seen[key] = true
	}
	return nil
}

// appearanceWarnings flags unknown [appearance] enum values (a typo like
// default_view="wek"). An unknown value is non-fatal — it falls back to the
// default — but naming it makes the mistake visible instead of silent.
func appearanceWarnings(a Appearance) []string {
	var w []string
	check := func(field, val string, allowed ...string) {
		if val == "" {
			return
		}
		for _, ok := range allowed {
			if val == ok {
				return
			}
		}
		w = append(w, fmt.Sprintf("%s %q is unknown; using the default", field, val))
	}
	check("first_day_of_week", a.FirstDayOfWeek, "monday", "sunday")
	check("default_view", a.DefaultView, "month", "week", "day")
	check("time_format", a.TimeFormat, "12h", "24h")
	check("date_format", a.DateFormat, "us", "iso")
	check("color_mode", a.ColorMode, "auto", "truecolor", "16", "off")
	return w
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

// passwordCommandTimeout bounds a password_command so a stalled secret fetch
// (e.g. an unreachable Vaultwarden) can't hang startup indefinitely.
const passwordCommandTimeout = 10 * time.Second

// ResolvePassword returns the effective app password: the output of
// PasswordCommand if set, otherwise the inline Password. It is called at connect
// time (not during Load) so a failing command surfaces only when sync is
// actually attempted.
func (s Server) ResolvePassword(ctx context.Context) (string, error) {
	cmd := strings.TrimSpace(s.PasswordCommand)
	if cmd == "" {
		return s.Password, nil
	}
	// Bound the command so a hung password_command (e.g. a stalled Vaultwarden/bw
	// network call) can't block startup/reload uninterruptibly — the UI must never
	// hang on the network.
	ctx, cancel := context.WithTimeout(ctx, passwordCommandTimeout)
	defer cancel()
	// Run through the shell so command strings like "bw get password foo" work
	// as written in the config, matching the owner's Vaultwarden setup. Capture
	// stderr so a failure (e.g. "bw" not logged in) surfaces the real cause
	// instead of a bare "exit status 1".
	c := exec.CommandContext(ctx, "sh", "-c", cmd)
	// WaitDelay forces the child's pipes closed and reaps it shortly after the
	// context is cancelled, so a command that leaves a grandchild holding stdout
	// open (e.g. one that backgrounds a process) can't make Output's internal Wait
	// block past the timeout — the timeout above bounds sh, this bounds its leftovers.
	c.WaitDelay = passwordCommandTimeout
	var errBuf bytes.Buffer
	c.Stderr = &errBuf
	out, err := c.Output()
	if err != nil {
		if hint := strings.TrimSpace(errBuf.String()); hint != "" {
			return "", fmt.Errorf("running password_command: %w: %s", err, strings.SplitN(hint, "\n", 2)[0])
		}
		return "", fmt.Errorf("running password_command: %w", err)
	}
	pw := strings.TrimSpace(string(out))
	if pw == "" {
		// Exit 0 with no output means the secret store returned nothing (e.g. an
		// empty bw field). Fail with a clear cause instead of silently using an empty
		// password, which would surface later only as an opaque auth failure.
		return "", fmt.Errorf("password_command produced no output")
	}
	return pw, nil
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
