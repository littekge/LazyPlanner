package ui

import "testing"

// TestResizeCancelRestoresAccordion: entering resize mode (Ctrl-W) while the
// accordion is collapsed un-collapses it (resizing a collapsed column is
// meaningless); Esc/cancel must restore that collapse it destroyed, matching
// grab's keep/cancel semantics (Enter keeps the adjustments — and, since the
// user just chose explicit widths, leaves the accordion un-collapsed too).
func TestResizeCancelRestoresAccordion(t *testing.T) {
	a, screen := drawnApp(t)
	a.setMode(modeTasks)
	w0 := detailWidth(a, screen)
	if w0 == 0 {
		t.Fatal("precondition: Detail visible in Tasks mode")
	}
	_, _, leftW0, _ := a.leftCol.GetRect()
	if leftW0 == 0 {
		t.Fatal("precondition: overview column visible")
	}

	a.setAccordion(true)
	if !a.accordion {
		t.Fatal("precondition: accordion collapsed")
	}

	a.enterResizeMode()
	if a.accordion {
		t.Fatal("entering resize mode must un-collapse the accordion (resizing a collapsed column is meaningless)")
	}

	a.exitResizeMode(true) // Esc/cancel

	if !a.accordion {
		t.Error("Esc/cancel must restore the accordion it un-collapsed on entry")
	}
	if w := detailWidth(a, screen); w != 0 {
		t.Errorf("after cancel: Detail width = %d, want 0 (accordion restored)", w)
	}
	a.root.Draw(screen)
	if _, _, leftW, _ := a.leftCol.GetRect(); leftW != 0 {
		t.Errorf("after cancel: overview width = %d, want 0 (accordion restored)", leftW)
	}
}

// TestResizeKeepLeavesAccordionUncollapsed pins the companion path: Enter
// (keep) leaves the accordion un-collapsed, since the user has just chosen
// explicit pane widths.
func TestResizeKeepLeavesAccordionUncollapsed(t *testing.T) {
	a, screen := drawnApp(t)
	a.setMode(modeTasks)

	a.setAccordion(true)
	a.enterResizeMode()
	a.exitResizeMode(false) // Enter/keep

	if a.accordion {
		t.Error("Enter/keep must leave the accordion un-collapsed")
	}
	if w := detailWidth(a, screen); w == 0 {
		t.Error("after keep: Detail should be visible (accordion stayed off)")
	}
}
