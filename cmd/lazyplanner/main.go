// Command lazyplanner is the entry point for the LazyPlanner TUI. It is thin
// wiring only: it builds the application identity string and hands off to the
// UI. Application logic lives in the internal packages.
package main

import (
	"fmt"
	"os"

	"github.com/littekge/LazyPlanner/internal/ui"
)

// Application identity, surfaced in the UI and (later) in version output.
const (
	appName    = "LazyPlanner"
	appVersion = "0.0.1"
)

func main() {
	title := fmt.Sprintf("%s %s", appName, appVersion)
	if err := ui.Run(title); err != nil {
		fmt.Fprintln(os.Stderr, "lazyplanner:", err)
		os.Exit(1)
	}
}
