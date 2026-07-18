// Command lazyplanner is the entry point for the LazyPlanner TUI. It is thin
// wiring only: it builds the application identity string and hands off to the
// UI. Application logic lives in the internal packages.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	// Embed the IANA time-zone database in the binary so time.LoadLocation
	// resolves zones even where the OS ships none (a minimal Pi image, Windows),
	// keeping LazyPlanner a robust single static binary. Without this, timed
	// events carrying a TZID could fail to resolve and be dropped.
	_ "time/tzdata"

	"github.com/littekge/LazyPlanner/internal/caldav"
	"github.com/littekge/LazyPlanner/internal/config"
	"github.com/littekge/LazyPlanner/internal/state"
	"github.com/littekge/LazyPlanner/internal/store"
	"github.com/littekge/LazyPlanner/internal/sync"
	"github.com/littekge/LazyPlanner/internal/ui"
)

// Application identity, surfaced in the UI and (later) in version output.
const (
	appName    = "LazyPlanner"
	appVersion = "0.0.1"
)

func main() { os.Exit(run(os.Args[1:])) }

// run dispatches the CLI and returns a process exit code. Kept separate from main
// (which only wraps os.Exit) so the dispatch — unknown-command, help, version — is
// testable without spawning a process. All real work lives in the internal packages.
func run(args []string) int {
	if len(args) == 0 {
		return report(runTUI())
	}
	switch args[0] {
	case "import":
		return report(runImport(args[1:]))
	case "sync":
		return report(runSync(args[1:]))
	case "calendar":
		return report(runCalendar(args[1:]))
	case "help", "-h", "--help":
		printUsage(os.Stdout)
		return 0
	case "version", "-v", "--version":
		fmt.Fprintf(os.Stdout, "%s %s\n", appName, appVersion)
		return 0
	default:
		// An unrecognized first argument used to fall through and silently open the
		// TUI, hiding a typo like "lazyplanner imprt". Report it and show usage.
		fmt.Fprintf(os.Stderr, "lazyplanner: unknown command %q\n\n", args[0])
		printUsage(os.Stderr)
		return 2
	}
}

// report prints err (if any) and maps it to an exit code. The two flag-parsing
// outcomes are special: `flag` already wrote output for them, so report must not
// print again. -h/--help (flag.ErrHelp) is a clean request → exit 0; a bad flag
// (wrapped in errFlagParsed by parseFlags) already had its error + usage printed
// by flag → exit 2 without a second, duplicate message.
func report(err error) int {
	switch {
	case err == nil:
		return 0
	case errors.Is(err, flag.ErrHelp):
		return 0
	case errors.Is(err, errFlagParsed):
		return 2
	default:
		fmt.Fprintln(os.Stderr, "lazyplanner:", err)
		return 1
	}
}

// errFlagParsed tags a subcommand flag-parse failure that flag.FlagSet already
// reported (error message + usage) so report() exits non-zero without printing a
// duplicate.
var errFlagParsed = errors.New("invalid flags")

// parseFlags parses a subcommand's flags, normalizing flag's two
// already-emitted-output cases (see report): -h/--help returns flag.ErrHelp
// unchanged; any other parse error is tagged errFlagParsed. Both stop the
// subcommand, but neither is re-printed.
func parseFlags(fs *flag.FlagSet, args []string) error {
	err := fs.Parse(args)
	if err == nil || errors.Is(err, flag.ErrHelp) {
		return err
	}
	return fmt.Errorf("%w: %w", errFlagParsed, err)
}

// printUsage writes the top-level command summary.
func printUsage(w io.Writer) {
	fmt.Fprintf(w, `%s %s — terminal CalDAV todo & calendar manager

Usage:
  lazyplanner            open the TUI (default)
  lazyplanner import     one-way pull (server → local cache)
  lazyplanner sync       two-way sync of the local cache
  lazyplanner calendar   manage calendars (list/create/delete)
  lazyplanner version    print the version
  lazyplanner help       show this help

Run a subcommand with -h for its flags; see the README for configuration.
`, appName, appVersion)
}

// runTUI loads the config, opens the account-namespaced local cache, and hands
// it to the UI. It is thin wiring: the UI reads everything through the store.
// On a first run (no config file) it writes a commented starter config and
// exits so the user can fill in the [server] connection before launching.
func runTUI() error {
	cfg, configured, warning, err := config.Load()
	if err != nil {
		return err
	}
	if !configured {
		path, written, gerr := config.GenerateDefault()
		if gerr != nil {
			return gerr
		}
		if written {
			fmt.Fprintf(os.Stderr, "lazyplanner: wrote a starter config to %s\n", path)
			fmt.Fprintln(os.Stderr, "lazyplanner: edit the [server] section, then run lazyplanner again")
			return nil
		}
	}
	if warning != "" {
		fmt.Fprintln(os.Stderr, "lazyplanner:", warning)
	}

	// The cache is namespaced by account so changing the server connection maps
	// to a separate cache and two accounts' data can never share one directory.
	dataDir, err := config.AccountDataDir(cfg.Server.URL, cfg.Server.Username)
	if err != nil {
		return err
	}
	s, err := store.Open(context.Background(), dataDir)
	if err != nil {
		return err
	}

	// Remembered UI state (pane sizes) lives beside the cache, under the data
	// dir — never the config file, which the app must not write.
	statePath := filepath.Join(dataDir, state.FileName)
	uiState := state.Load(statePath)

	accountID := config.AccountID(cfg.Server.URL, cfg.Server.Username)
	configPath, pathErr := config.ConfigPath()

	// color_mode "truecolor" force-enables tcell's 24-bit output for terminals
	// that don't advertise it (tcell reads COLORTERM at screen init). "auto"
	// leaves detection to the terminal; the UI renders RGB either way.
	if strings.EqualFold(strings.TrimSpace(cfg.Appearance.ColorMode), "truecolor") {
		_ = os.Setenv("COLORTERM", "truecolor")
	}

	syncFn, syncWarn := buildSyncFn(cfg.Server, s)
	if syncWarn != "" {
		fmt.Fprintln(os.Stderr, "lazyplanner:", syncWarn)
	}

	title := fmt.Sprintf("%s %s", appName, appVersion)
	return ui.Run(ui.Options{
		Store:               s,
		Title:               title,
		Sync:                syncFn,
		SyncIntervalMinutes: cfg.Behavior.SyncIntervalMinutes,
		LeftWidth:           uiState.LeftWidth,
		DetailWidth:         uiState.DetailWidth,
		Hidden:              uiState.HiddenCalendars,
		RowsPerHour:         uiState.RowsPerHour,
		ColorMode:           cfg.Appearance.ColorMode,
		FirstDayOfWeek:      cfg.Appearance.FirstDayOfWeek,
		DefaultView:         cfg.Appearance.DefaultView,
		TimeFormat:          cfg.Appearance.TimeFormat,
		DateFormat:          cfg.Appearance.DateFormat,
		SaveState: func(leftWidth, detailWidth int, hidden []string, rowsPerHour int) {
			_ = state.Save(statePath, state.State{LeftWidth: leftWidth, DetailWidth: detailWidth, HiddenCalendars: hidden, RowsPerHour: rowsPerHour})
		},
		EditConfig: editConfigFn(configPath, pathErr, accountID, s),
	})
}

// buildSyncFn returns a closure the UI calls to sync, or nil when no server is
// configured (the app then runs fully offline). A failing password_command is a
// warning, not a fatal error — the app still opens over the cache.
// editConfigFn builds the :config callback: open the config file in $EDITOR,
// reload it, and return a fresh sync closure. It refuses a reload that changes
// the account (the local cache is account-keyed, so switching accounts safely
// requires a restart). Returns nil to disable :config when the path is unknown.
func editConfigFn(configPath string, pathErr error, accountID string, s *store.Store) func() (ui.ConfigReload, error) {
	if pathErr != nil || configPath == "" {
		return nil
	}
	return func() (ui.ConfigReload, error) {
		ed := editorCommand(os.Getenv("EDITOR"), configPath)
		ed.Stdin, ed.Stdout, ed.Stderr = os.Stdin, os.Stdout, os.Stderr
		if err := ed.Run(); err != nil {
			return ui.ConfigReload{}, fmt.Errorf("editor: %w", err)
		}
		cfg, _, loadWarn, err := config.Load()
		if err != nil {
			return ui.ConfigReload{}, err
		}
		if config.AccountID(cfg.Server.URL, cfg.Server.Username) != accountID {
			return ui.ConfigReload{}, fmt.Errorf("server/account changed — restart to switch caches")
		}
		syncFn, warn := buildSyncFn(cfg.Server, s)
		// Surface config.Load's own warning too (appearance typo, world-readable
		// password file) — at startup it goes to its own stderr line, but on a
		// :config reload the single flash string must carry it, or an edit that
		// introduces the problem is silently accepted.
		return ui.ConfigReload{Sync: syncFn, ColorMode: cfg.Appearance.ColorMode, Warning: joinWarnings(loadWarn, warn)}, nil
	}
}

// joinWarnings combines two possibly-empty warning strings into one line for the
// :config reload flash (startup emits them as separate stderr lines; the UI flash
// is a single string). Either or both may be empty.
func joinWarnings(a, b string) string {
	switch {
	case a == "":
		return b
	case b == "":
		return a
	default:
		return a + "; " + b
	}
}

// editorCommand builds the command that opens path in the user's editor. $EDITOR
// commonly carries arguments ("code --wait", "subl -w", "emacsclient -c",
// "vim -f"); splitting on whitespace keeps those as arguments instead of folding
// them into the binary name (which would fail with ENOENT). Defaults to vi when
// $EDITOR is empty. (Whitespace-in-path editor values are not supported — rare,
// and shelling out would cost portability on the Windows target.)
func editorCommand(editorEnv, path string) *exec.Cmd {
	fields := strings.Fields(editorEnv)
	if len(fields) == 0 {
		fields = []string{"vi"}
	}
	return exec.Command(fields[0], append(fields[1:], path)...)
}

// buildSyncFn returns the sync closure (nil = offline) and a warning describing
// why it's offline (empty when fine or simply not configured). The warning lets
// the UI flash the reason on a :config reload instead of losing it to stderr
// behind the suspended TUI.
func buildSyncFn(srv config.Server, s *store.Store) (func(context.Context) (sync.SyncResult, error), string) {
	if !srv.Configured() {
		return nil, ""
	}
	password, err := srv.ResolvePassword(context.Background())
	if err != nil {
		return nil, fmt.Sprintf("%v (offline)", err)
	}
	client, err := caldav.NewClient(caldav.Config{Endpoint: srv.URL, Username: srv.Username, Password: password})
	if err != nil {
		return nil, fmt.Sprintf("%v (offline)", err)
	}
	return func(ctx context.Context) (sync.SyncResult, error) {
		return sync.Sync(ctx, client, s)
	}, ""
}
