package ui

import (
	"context"
	"testing"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"

	"github.com/littekge/LazyPlanner/internal/store"
)

// TestNestedModalOverDrilledCalendarKeepsFormFocus guards against the recurring-
// event softlock: closing a modal nested over the item form (the Custom… repeat
// sub-form) must return focus to the still-open form, not teleport to the drilled
// calendar behind it. captureFocus used to record the calendar's drill state for
// every modal — including a nested one — so restoreFocus re-drilled the calendar
// and left the outer form open but unreachable.
func TestNestedModalOverDrilledCalendarKeepsFormFocus(t *testing.T) {
	when := time.Date(2026, 7, 6, 12, 0, 0, 0, time.UTC)
	a := newRootedTestApp(t, when)
	if err := a.store.CreateCalendarLocal(context.Background(), "ev", store.CalendarMeta{DisplayName: "EV"}, []string{"VEVENT"}); err != nil {
		t.Fatal(err)
	}
	putRecurringEvent(t, a, "ev", "Standup", time.Date(2026, 7, 6, 9, 0, 0, 0, time.UTC), "FREQ=WEEKLY;COUNT=4")
	a.reload()

	a.setMode(modeCalendar)
	g, ok := a.calendarPrimitive().(calGrid)
	if !ok {
		t.Fatalf("calendar primitive is not a grid: %T", a.calendarPrimitive())
	}
	day := time.Date(2026, 7, 6, 0, 0, 0, 0, time.Local)
	g.reDrill(day, 0)
	if _, drilled, _ := g.drillState(); !drilled {
		t.Skip("could not drill the grid in this build/view; focus assertion N/A")
	}
	a.setFocus(a.calendarPrimitive())
	a.focusStack = nil

	// First modal: the item edit form, over the drilled calendar.
	form := tview.NewBox()
	a.openModal(pageForm, form, 40, 10)
	// Nested modal: the Custom… repeat sub-form.
	a.openModal(pageRepeat, tview.NewBox(), 30, 10)
	// Its OK/Cancel closes only the nested modal.
	a.closeModal(pageRepeat)

	if !a.root.HasPage(pageForm) {
		t.Fatal("the item form was closed by the nested modal")
	}
	if got := a.tv.GetFocus(); got != form {
		t.Errorf("focus after the nested modal closed = %T, want the item form; it escaped to the calendar (softlock)", got)
	}
}

// drawOpenDropDown focuses and opens dd, then renders it so its option list is
// drawn — mirroring drawCells for a plain List.
func drawOpenDropDown(t *testing.T, dd *tview.DropDown, w, h int) []tcell.SimCell {
	t.Helper()
	focus := func(p tview.Primitive) { p.Focus(func(tview.Primitive) {}) }
	dd.Focus(focus)
	if handler := dd.InputHandler(); handler != nil {
		handler(tcell.NewEventKey(tcell.KeyEnter, 0, tcell.ModNone), focus)
	}
	if !dd.IsOpen() {
		t.Skip("could not open the dropdown in this build")
	}
	cells, _, _ := drawCells(t, dd, w, h)
	return cells
}

// TestDropDownSelectionIsLegible guards the form dropdowns (priority, Repeat, and
// the Custom sub-form's dropdowns) against the recurring white-on-white selection
// bug: the open list's highlighted row must be reverse-video, like every List.
func TestDropDownSelectionIsLegible(t *testing.T) {
	f := newCaretForm()
	dd := f.addDropDown("Repeat", []string{"None", "Daily", "Weekly on Tue"}, 0)
	f.stylePopup()

	cells := drawOpenDropDown(t, dd, 30, 12)
	for _, c := range cells {
		if _, _, attr := c.Style.Decompose(); attr&tcell.AttrReverse != 0 {
			return // a reverse-video cell means the selected row is legible
		}
	}
	t.Error("open dropdown's selected row is not reverse-video — illegible on terminal-default themes")
}
