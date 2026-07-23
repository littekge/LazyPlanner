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
	accent := false
	for _, c := range cells {
		if fg, _, _ := c.Style.Decompose(); fg == accentColor {
			accent = true
			break
		}
	}
	if !accent {
		t.Error("no accent-colored cell — the confirm modal is missing its accent border/title chrome")
	}
}

// TestConfirmModalHasNoContrastHighlight guards against tview.Modal's default
// contrast background bleeding through: the modal's Box background (border fill +
// the padding ring around the content) must be the terminal default, not
// Styles.ContrastBackgroundColor, so the border has no highlighted band.
// tview.Modal.SetBackgroundColor resets only the frame/form, never the Box.
func TestConfirmModalHasNoContrastHighlight(t *testing.T) {
	modal := tview.NewModal().
		SetText(`Delete "Write docs"?`).
		AddButtons([]string{"Delete", "Cancel"})
	styleModal(modal, " Delete task ")

	cells, _, _ := drawCells(t, modal, 60, 12)
	for _, c := range cells {
		if _, bg, _ := c.Style.Decompose(); bg == tview.Styles.ContrastBackgroundColor {
			t.Fatalf("modal renders a %v (contrast) cell — the border/box highlight band is still present", bg)
		}
	}
}
