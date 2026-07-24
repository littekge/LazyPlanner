package ui

import (
	"strings"
	"testing"
	"time"
)

// TestInteractionMode covers the mode the status-bar indicator reads: NORMAL at
// rest, GRAB while grabbing, and DRILL only when dived into a calendar day. Merely
// focusing the task tree or the calendar grid is ordinary Main navigation (NORMAL)
// — the tree and grid agree, so DRILL never shows in Tasks (M1).
func TestInteractionMode(t *testing.T) {
	a := newRootedTestApp(t, time.Date(2026, 7, 5, 9, 0, 0, 0, time.UTC))

	a.setMode(modeCalendar)
	if m := a.interactionMode(); m != modeNormal {
		t.Errorf("resting calendar mode = %q, want NORMAL", m)
	}

	a.grabbing = true
	if m := a.interactionMode(); m != modeGrab {
		t.Errorf("grabbing mode = %q, want GRAB", m)
	}
	a.grabbing = false

	// Drilling into a calendar day is the one DRILL state.
	a.viewMode = viewMonth
	a.month.eventMode = true
	if m := a.interactionMode(); m != modeDrill {
		t.Errorf("drilled calendar day mode = %q, want DRILL", m)
	}
	a.month.eventMode = false

	// Focusing the task tree is NOT drilled — it reads NORMAL, matching a focused
	// (undrilled) calendar grid.
	a.setMode(modeTasks)
	a.buildTree()
	a.setFocus(a.tree)
	if m := a.interactionMode(); m != modeNormal {
		t.Errorf("focused task tree mode = %q, want NORMAL", m)
	}

	a.setFocus(a.tasklists)
	if m := a.interactionMode(); m != modeNormal {
		t.Errorf("task-list overview mode = %q, want NORMAL", m)
	}

	a.selecting = true
	if m := a.interactionMode(); m != modeSelect {
		t.Errorf("selecting mode = %q, want SELECT", m)
	}
	a.grabbing = true
	if m := a.interactionMode(); m != modeGrab {
		t.Errorf("selecting+grabbing mode = %q, want GRAB (innermost wins)", m)
	}
	a.grabbing, a.selecting = false, false
}

// TestModeIndicatorRenders confirms the mode badge and the status-bar outline
// actually paint: NORMAL at rest, GRAB while grabbing, with the border row present.
func TestModeIndicatorRenders(t *testing.T) {
	a := newTestApp(t, time.Date(2026, 7, 5, 12, 0, 0, 0, time.UTC))
	root := a.layout()

	dump := func() string {
		cells, cw, ch := drawCells(t, root, 100, 24)
		var b strings.Builder
		for row := 0; row < ch; row++ {
			for col := 0; col < cw; col++ {
				if c := cells[row*cw+col]; len(c.Runes) > 0 {
					b.WriteRune(c.Runes[0])
				} else {
					b.WriteByte(' ')
				}
			}
			b.WriteByte('\n')
		}
		return b.String()
	}

	rest := dump()
	if !strings.Contains(rest, "NORMAL") {
		t.Error("status bar should show the NORMAL badge at rest")
	}
	// A rounded border corner proves the status bar is outlined.
	if !strings.ContainsAny(rest, "╭╮╰╯─") {
		t.Error("status bar should be outlined with a border")
	}

	a.grabbing = true
	if g := dump(); !strings.Contains(g, "GRAB") {
		t.Error("status bar should show the GRAB badge while grabbing")
	}

	a.grabbing = false
	a.selecting = true
	if s := dump(); !strings.Contains(s, "SELECT") {
		t.Error("status bar should show the SELECT badge while selecting")
	}
	a.selecting = false
}
