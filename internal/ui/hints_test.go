package ui

import (
	"strings"
	"testing"
	"time"
)

// TestHelpBarOrder locks the deliberate ordering of the bottom help bar: because
// wrap is off, a narrow terminal clips the right end, so ? help and q quit must
// lead (survive clipping) and the basic movement/navigation hints must precede
// the editing actions.
func TestHelpBarOrder(t *testing.T) {
	a := newTestApp(t, time.Date(2026, 7, 5, 9, 0, 0, 0, time.UTC))
	a.updateStatus()
	got := a.hints.GetText(true)

	if !strings.HasPrefix(got, "? help · q quit") {
		t.Fatalf("help bar must start with the help/quit hints; got: %q", got)
	}

	// The tokens must appear in this order (movement/nav before editing actions).
	order := []string{
		"? help", "q quit", "hjkl move", "Enter open", "Esc back", "c/t/a panes",
		"f/b prev/next", "v view", "[ ] cal", "{ } list",
		"i… new", "e edit", "d del", "Space", "/ find", "u undo", "r sync", ": cmd",
	}
	prev := -1
	for _, tok := range order {
		i := strings.Index(got, tok)
		if i < 0 {
			t.Errorf("help bar missing %q\n  full: %q", tok, got)
			continue
		}
		if i < prev {
			t.Errorf("help bar token %q is out of the intended order\n  full: %q", tok, got)
		}
		prev = i
	}
}

// TestSelectHintBarDoesNotClaimGGEnds guards finding #1 of the v1.5.0 phase-2
// key×context matrix: gg/G in SELECT mode extend the range to the top/bottom
// (gotoTop/gotoBottom stay in effect while selecting), they do not end/exit
// it — the hint bar must say so, matching :help's own "extend the range"
// wording for the same chord.
func TestSelectHintBarDoesNotClaimGGEnds(t *testing.T) {
	a := newTestApp(t, time.Date(2026, 7, 5, 9, 0, 0, 0, time.UTC))
	a.selecting = true
	a.updateStatus()
	got := a.hints.GetText(true)

	if strings.Contains(got, "gg/G ends") {
		t.Errorf("SELECT hint bar wrongly claims gg/G ends the selection: %q", got)
	}
	if !strings.Contains(got, "gg/G extend") {
		t.Errorf("SELECT hint bar should describe gg/G as extending the range: %q", got)
	}
}

// TestHelpBarCompletedToggle checks the one dynamic field reflects showCompleted.
func TestHelpBarCompletedToggle(t *testing.T) {
	a := newTestApp(t, time.Date(2026, 7, 5, 9, 0, 0, 0, time.UTC))

	a.showCompleted = false
	a.updateStatus()
	if !strings.Contains(a.hints.GetText(true), ". comp:off") {
		t.Errorf("expected 'comp:off' when completed hidden; got %q", a.hints.GetText(true))
	}

	a.showCompleted = true
	a.updateStatus()
	if !strings.Contains(a.hints.GetText(true), ". comp:on") {
		t.Errorf("expected 'comp:on' when completed shown; got %q", a.hints.GetText(true))
	}
}
