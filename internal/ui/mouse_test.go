package ui

import (
	"testing"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

func TestMouseClickSwitchesMode(t *testing.T) {
	a := newRootedTestApp(t, time.Date(2026, 7, 5, 12, 0, 0, 0, time.UTC)) // starts in Tasks

	// Draw the layout so the panels get real rects for InRect hit-testing.
	screen := tcell.NewSimulationScreen("")
	if err := screen.Init(); err != nil {
		t.Fatal(err)
	}
	defer screen.Fini()
	screen.SetSize(120, 40)
	a.root.SetRect(0, 0, 120, 40)
	a.root.Draw(screen)

	click := func(list *tview.List) {
		x, y, w, h := list.GetRect()
		if w == 0 || h == 0 {
			t.Fatalf("panel has no rect after draw")
		}
		a.mouseCapture(tcell.NewEventMouse(x+1, y+1, tcell.Button1, tcell.ModNone), tview.MouseLeftClick)
	}

	click(a.calendars)
	if a.mode != modeCalendar {
		t.Errorf("clicking Calendars → mode %d, want calendar", a.mode)
	}
	click(a.agendaList)
	if a.mode != modeAgenda {
		t.Errorf("clicking Agenda → mode %d, want agenda", a.mode)
	}
	click(a.tasklists)
	if a.mode != modeTasks {
		t.Errorf("clicking Tasks → mode %d, want tasks", a.mode)
	}
}

// TestMouseSwallowedDuringModalFlagStates locks Pass-3 #5: grab and the resize
// sub-mode are flag-only modal states (no overlay page), so the mouse must be
// swallowed just like the keyboard — a click can't switch panes or open a form
// mid-grab/resize.
func TestMouseSwallowedDuringModalFlagStates(t *testing.T) {
	a := newRootedTestApp(t, time.Date(2026, 7, 5, 12, 0, 0, 0, time.UTC)) // starts in Tasks

	screen := tcell.NewSimulationScreen("")
	if err := screen.Init(); err != nil {
		t.Fatal(err)
	}
	defer screen.Fini()
	screen.SetSize(120, 40)
	a.root.SetRect(0, 0, 120, 40)
	a.root.Draw(screen)

	x, y, _, _ := a.calendars.GetRect()
	for _, flag := range []string{"grabbing", "resizing"} {
		a.setMode(modeTasks)
		a.grabbing = flag == "grabbing"
		a.resizing = flag == "resizing"

		gotEv, _ := a.mouseCapture(tcell.NewEventMouse(x+1, y+1, tcell.Button1, tcell.ModNone), tview.MouseLeftClick)
		if gotEv != nil {
			t.Errorf("[%s] mouse event not swallowed; tview would still act on it", flag)
		}
		if a.mode != modeTasks {
			t.Errorf("[%s] click switched mode to %d during a modal flag-state; want it ignored", flag, a.mode)
		}
	}
	a.grabbing, a.resizing = false, false
}
