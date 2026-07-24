package ui

import (
	"testing"
	"time"

	"github.com/gdamore/tcell/v2"
)

// drawnApp draws the full app once so Flex has assigned real rects.
func drawnApp(t *testing.T) (*app, tcell.SimulationScreen) {
	t.Helper()
	a := newRootedTestApp(t, time.Date(2026, 7, 5, 9, 0, 0, 0, time.UTC))
	screen := tcell.NewSimulationScreen("")
	if err := screen.Init(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(screen.Fini)
	screen.SetSize(120, 40)
	a.root.SetRect(0, 0, 120, 40)
	a.root.Draw(screen)
	return a, screen
}

func detailWidth(a *app, screen tcell.SimulationScreen) int {
	a.root.Draw(screen)
	_, _, w, _ := a.detail.GetRect()
	return w
}

// TestAccordionCollapsesAndRestoresDetail: the +/- accordion must collapse the
// Detail pane together with the overview column (the spec's Pane-sizing wording,
// previously only true for the overview), and restore it on toggle-off.
func TestAccordionCollapsesAndRestoresDetail(t *testing.T) {
	a, screen := drawnApp(t)
	a.setMode(modeTasks)
	w0 := detailWidth(a, screen)
	if w0 == 0 {
		t.Fatal("precondition: Detail visible in Tasks mode")
	}

	a.setAccordion(true)
	if w := detailWidth(a, screen); w != 0 {
		t.Errorf("accordion on: Detail width = %d, want 0", w)
	}

	a.setAccordion(false)
	if w := detailWidth(a, screen); w != w0 {
		t.Errorf("accordion off: Detail width = %d, want %d restored", w, w0)
	}
}

// TestSetModeAfterAccordionRestoresDetail: switching panels auto-restores the
// accordion (existing behavior); the restore must bring Detail back too — except
// into Agenda mode, which hides Detail independently of the accordion.
func TestSetModeAfterAccordionRestoresDetail(t *testing.T) {
	a, screen := drawnApp(t)
	a.setMode(modeTasks)
	w0 := detailWidth(a, screen)

	a.setAccordion(true)
	a.setMode(modeCalendar)
	if a.accordion {
		t.Fatal("setMode must clear the accordion (existing behavior)")
	}
	if w := detailWidth(a, screen); w != w0 {
		t.Errorf("setMode(Calendar) after accordion: Detail width = %d, want %d", w, w0)
	}

	a.setMode(modeTasks)
	a.setAccordion(true)
	a.setMode(modeAgenda)
	if w := detailWidth(a, screen); w != 0 {
		t.Errorf("setMode(Agenda) after accordion: Detail width = %d, want 0 (Agenda hides Detail)", w)
	}
}
