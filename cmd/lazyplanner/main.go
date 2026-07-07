// Command lazyplanner is the entry point for the LazyPlanner TUI. It is thin
// wiring only: it builds the application identity string and hands off to the
// UI. Application logic lives in the internal packages.
package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

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

func main() {
	// Subcommands stay minimal wiring; all logic lives in the internal packages.
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "import":
			exitOnError(runImport(os.Args[2:]))
			return
		case "sync":
			exitOnError(runSync(os.Args[2:]))
			return
		case "calendar":
			exitOnError(runCalendar(os.Args[2:]))
			return
		}
	}

	exitOnError(runTUI())
}

func exitOnError(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, "lazyplanner:", err)
		os.Exit(1)
	}
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

	title := fmt.Sprintf("%s %s", appName, appVersion)
	return ui.Run(ui.Options{
		Store:     s,
		Title:     title,
		Sync:      buildSyncFn(cfg.Server, s),
		LeftWidth: uiState.LeftWidth,
		SaveState: func(leftWidth int) {
			_ = state.Save(statePath, state.State{LeftWidth: leftWidth})
		},
	})
}

// buildSyncFn returns a closure the UI calls to sync, or nil when no server is
// configured (the app then runs fully offline). A failing password_command is a
// warning, not a fatal error — the app still opens over the cache.
func buildSyncFn(srv config.Server, s *store.Store) func(context.Context) (sync.SyncResult, error) {
	if !srv.Configured() {
		return nil
	}
	password, err := srv.ResolvePassword()
	if err != nil {
		fmt.Fprintln(os.Stderr, "lazyplanner:", err, "(starting offline)")
		return nil
	}
	client, err := caldav.NewClient(caldav.Config{Endpoint: srv.URL, Username: srv.Username, Password: password})
	if err != nil {
		fmt.Fprintln(os.Stderr, "lazyplanner:", err, "(starting offline)")
		return nil
	}
	return func(ctx context.Context) (sync.SyncResult, error) {
		return sync.Sync(ctx, client, s)
	}
}
