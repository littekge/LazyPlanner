package ui

import (
	"testing"

	"github.com/rivo/tview"
)

// TestConfirmModalHasAccentChrome guards dialog-chrome consistency: a confirm/
// picker modal styled via styleModal carries the same accent rounded border +
// accent title as the forms and other popups (not tview.Modal's plain default).
func TestConfirmModalHasAccentChrome(t *testing.T) {
	modal := tview.NewModal().
		SetText(`Delete "Write docs"?`).
		AddButtons([]string{"Delete", "Cancel"})
	styleModal(modal, " Delete task ")

	if got := modal.GetTitle(); got != " Delete task " {
		t.Errorf("title = %q, want %q", got, " Delete task ")
	}

	cells, _, _ := drawCells(t, modal, 60, 12)
	for _, c := range cells {
		if fg, _, _ := c.Style.Decompose(); fg == accentColor {
			return // an accent-colored cell means the border/title rendered in the accent
		}
	}
	t.Error("no accent-colored cell — the confirm modal is missing its accent border/title chrome")
}
