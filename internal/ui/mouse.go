package ui

import (
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// mouseCapture makes the mouse coherent with the mode model on top of tview's
// built-in click-to-select and wheel-scroll: clicking a left overview panel
// switches to that mode (so the center follows), and a double-click on the task
// tree or agenda opens the edit form for the item under the cursor. tview still
// receives the event afterward, so selection and scrolling keep working.
func (a *app) mouseCapture(ev *tcell.EventMouse, action tview.MouseAction) (*tcell.EventMouse, tview.MouseAction) {
	if ev == nil {
		return ev, action
	}
	// Don't reinterpret clicks while a modal/overlay is up — it owns the mouse.
	if a.modalOpen() {
		return ev, action
	}

	switch action {
	case tview.MouseLeftClick:
		x, y := ev.Position()
		switch {
		case a.calendars.InRect(x, y):
			if a.mode != modeCalendar {
				a.setMode(modeCalendar)
			}
		case a.tasklists.InRect(x, y):
			if a.mode != modeTasks {
				a.setMode(modeTasks)
			}
		case a.agendaList.InRect(x, y):
			if a.mode != modeAgenda {
				a.setMode(modeAgenda)
			}
		}
	case tview.MouseLeftDoubleClick:
		x, y := ev.Position()
		// Double-click to edit where a selection maps to an item. The preceding
		// single click has already moved the selection under the cursor.
		if a.tree.InRect(x, y) || a.agenda.InRect(x, y) {
			a.editSelected()
		}
	}
	return ev, action
}
