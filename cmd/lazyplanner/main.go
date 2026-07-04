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
	if len(os.Args) > 1 && os.Args[1] == "import" {
		if err := runImport(os.Args[2:]); err != nil {
			fmt.Fprintln(os.Stderr, "lazyplanner:", err)
			os.Exit(1)
		}
		return
	}

	if err := runTUI(); err != nil {
		fmt.Fprintln(os.Stderr, "lazyplanner:", err)
		os.Exit(1)
	}
}

// runTUI opens the local cache and hands it to the UI. It is thin wiring: the
// UI reads everything through the store.
func runTUI() error {
	dataDir, err := config.DataDir()
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
