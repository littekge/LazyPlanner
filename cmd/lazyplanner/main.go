// Command lazyplanner is the entry point for the LazyPlanner TUI. It is thin
// wiring only: it builds the application identity string and hands off to the
// UI. Application logic lives in the internal packages.
package main

import (
	"context"
	"fmt"
	"os"

	"github.com/littekge/LazyPlanner/internal/config"
	"github.com/littekge/LazyPlanner/internal/store"
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
	title := fmt.Sprintf("%s %s", appName, appVersion)
	return ui.Run(s, title)
}
