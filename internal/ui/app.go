// Package ui contains all terminal UI code for LazyPlanner. It is the only
// package permitted to import tview/tcell; every other package compiles and
// tests headlessly. It reaches disk and network only through store and sync,
// never directly.
package ui

import (
	"fmt"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// Run builds the LazyPlanner TUI, displays it, and blocks until the user
// quits. title is shown in the placeholder window and is replaced by the real
// three-region pane layout in later build steps. It returns any error from the
// tview event loop.
func Run(title string) error {
	app := tview.NewApplication()

	view := tview.NewTextView().
		SetTextAlign(tview.AlignCenter).
		SetText(fmt.Sprintf("%s\n\nPress q or Ctrl-C to quit.", title))
	view.SetBorder(true).SetTitle(" LazyPlanner ")

	// q quits, mirroring lazygit's single-key habit. tview already stops the
	// app on Ctrl-C; handling q here keeps the placeholder usable on its own.
	app.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		if event.Key() == tcell.KeyRune && event.Rune() == 'q' {
			app.Stop()
			return nil
		}
		return event
	})

	if err := app.SetRoot(view, true).EnableMouse(true).Run(); err != nil {
		return fmt.Errorf("running tui: %w", err)
	}
	return nil
}
