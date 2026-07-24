package ui

import (
	"strings"
	"testing"
	"time"

	"github.com/gdamore/tcell/v2"
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

// TestNormalHintBarIsModeAdaptive guards findings #3+#13 of the v1.5.0 phase-2
// matrix triage: f/b (prev/next anchor) and v (view cycle) are Calendar-mode-only
// keys — they silently no-op in Tasks/Agenda mode (see app.go's 'f'/'b'/'v' cases,
// each gated on a.mode == modeCalendar) — so the resting NORMAL hint bar must not
// advertise them outside Calendar mode. Tasks mode gets its own tree-relevant
// keys (>/< zoom, H/L indent) instead, which are themselves gated on modeTasks.
func TestNormalHintBarIsModeAdaptive(t *testing.T) {
	a := newTestApp(t, time.Date(2026, 7, 5, 9, 0, 0, 0, time.UTC))

	a.mode = modeCalendar
	a.updateStatus()
	got := a.hints.GetText(true)
	if !strings.Contains(got, "f/b prev/next") || !strings.Contains(got, "v view") {
		t.Errorf("Calendar-mode hint bar should show f/b and v view; got %q", got)
	}

	a.mode = modeTasks
	a.updateStatus()
	got = a.hints.GetText(true)
	if strings.Contains(got, "f/b") || strings.Contains(got, "v view") {
		t.Errorf("Tasks-mode hint bar wrongly shows Calendar-only f/b/v (they no-op outside Calendar mode); got %q", got)
	}
	if !strings.Contains(got, "zoom") && !strings.Contains(got, "indent") {
		t.Errorf("Tasks-mode hint bar should surface tree-relevant keys (zoom/indent); got %q", got)
	}

	a.mode = modeAgenda
	a.updateStatus()
	got = a.hints.GetText(true)
	if strings.Contains(got, "f/b") || strings.Contains(got, "v view") {
		t.Errorf("Agenda-mode hint bar wrongly shows Calendar-only f/b/v; got %q", got)
	}
	if strings.Contains(got, "zoom") || strings.Contains(got, "indent") {
		t.Errorf("Agenda-mode hint bar wrongly shows Tasks-only zoom/indent keys; got %q", got)
	}
}

// TestResizeHintBar guards finding #13: the Ctrl-W pane-resize sub-mode
// (a.resizing) fell through to the fixed NORMAL hint string before this fix,
// silently mismatching the mode-indicator badge (RESIZE) and the flash shown on
// entry. The hint bar must instead show the resize controls, matching the
// wording already used by help.go's Ctrl-W row and enterResizeMode's flash.
func TestResizeHintBar(t *testing.T) {
	a := newRootedTestApp(t, time.Date(2026, 7, 5, 9, 0, 0, 0, time.UTC))
	a.enterResizeMode()
	got := a.hints.GetText(true)

	if !strings.HasPrefix(got, "RESIZE") {
		t.Fatalf("resize hint bar should lead with RESIZE; got %q", got)
	}
	if !strings.Contains(got, "Esc") || !strings.Contains(got, "cancel") {
		t.Errorf("resize hint bar missing the cancel hint; got %q", got)
	}
	if strings.Contains(got, "hjkl move") {
		t.Errorf("resize hint bar should not show the NORMAL movement line; got %q", got)
	}
}

// TestFormOpenHintBar guards finding #13's other gap: a form/modal left the hint
// bar showing the fixed NORMAL movement line (hjkl move, Enter open, ...), which
// is misleading — those keys are owned by the form while it's open. Opening the
// task edit form (a *caretForm, page pageForm) must switch the hint bar to a
// form-context hint instead.
func TestFormOpenHintBar(t *testing.T) {
	a := newRootedTestApp(t, time.Date(2026, 7, 5, 9, 0, 0, 0, time.UTC))
	list := a.selectedTasklistID()
	putTodo(t, a, list, "", "task", time.Time{}, false)
	a.reload()
	a.buildTree()
	a.globalKeys(tcell.NewEventKey(tcell.KeyDown, 0, tcell.ModNone))
	a.globalKeys(runeKey('e'))
	if !a.modalOpen() {
		t.Fatal("expected the edit form to be open after 'e'")
	}
	a.updateStatus()
	got := a.hints.GetText(true)

	if !strings.Contains(got, "FORM") {
		t.Errorf("form-open hint bar should show a FORM context hint; got %q", got)
	}
	if strings.Contains(got, "hjkl move") {
		t.Errorf("form-open hint bar should not show the NORMAL movement line; got %q", got)
	}
}
