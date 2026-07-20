package ui

import (
	"testing"
	"time"

	"github.com/littekge/LazyPlanner/internal/model"
)

// TestSyncKeepsTreeHighlight reproduces the v1.0.0 bug where a background sync
// (which refreshes the views via refresh("")) reset the task-tree highlight back
// to the first task. The user's current position must survive a sync refresh.
func TestSyncKeepsTreeHighlight(t *testing.T) {
	now := time.Date(2026, 7, 5, 9, 0, 0, 0, time.UTC)
	a := newWritableTestApp(t, now)
	a.setMode(modeTasks)

	calID := a.selectedTasklistID()
	if calID == "" {
		t.Fatal("no task list selected")
	}
	// Three root siblings; the smart sort orders them by title, so "Gamma" is not
	// the first row — selecting it makes a reset-to-first regression observable.
	a.createTask(calID, "", "Alpha")
	a.createTask(calID, "", "Beta")
	a.createTask(calID, "", "Gamma")

	gamma := todoBySummary(a.store, "Gamma")
	if gamma == nil {
		t.Fatal("seed task Gamma missing")
	}
	a.selectTreeByUID(gamma.UID)
	if got := a.currentTreeUID(); got != gamma.UID {
		t.Fatalf("precondition: highlight = %q, want Gamma %q", got, gamma.UID)
	}

	// The exact call a completed background sync makes.
	a.refresh("")

	if got := a.currentTreeUID(); got != gamma.UID {
		t.Errorf("after sync refresh: highlight = %q, want Gamma %q (sync reset the highlight)", got, gamma.UID)
	}
}

// TestSyncKeepsCalendarDrill is the calendar-view sibling of the tree bug: a
// background sync's refresh("") ran buildCenterCalendar → setData, which resets
// eventMode/eventIndex, kicking the user out of a day's event-cycling back to day
// navigation on every sync. The day-drill must survive a position-agnostic refresh.
func TestSyncKeepsCalendarDrill(t *testing.T) {
	now := time.Date(2026, 7, 5, 9, 0, 0, 0, time.UTC)
	a := newWritableTestApp(t, now)
	a.setMode(modeCalendar)
	a.viewMode = viewDay
	a.anchor = time.Date(2026, 7, 5, 0, 0, 0, 0, time.UTC)

	// Two timed events on the anchor day, so the drilled index (1) is non-zero and
	// a reset-to-first regression is observable.
	calID := a.selectedCalendarID()
	if calID == "" {
		t.Fatal("no calendar selected")
	}
	a.createEvent(calID, a.anchor, "Alpha 9am")
	a.createEvent(calID, a.anchor, "Beta 11am")
	a.buildCenterCalendar()

	g, ok := a.calendarPrimitive().(calGrid)
	if !ok {
		t.Fatal("calendar primitive is not a calGrid")
	}
	g.reDrill(a.anchor, 1) // drill in, second item
	day, drilled, idx := g.drillState()
	if !drilled || idx != 1 {
		t.Fatalf("precondition: drillState(%v, %v, %d), want drilled at index 1", day, drilled, idx)
	}

	// The exact call a completed background sync makes.
	a.refresh("")

	g2, ok := a.calendarPrimitive().(calGrid)
	if !ok {
		t.Fatal("calendar primitive is not a calGrid after refresh")
	}
	_, drilled, idx = g2.drillState()
	if !drilled || idx != 1 {
		t.Errorf("after sync refresh: drilled=%v index=%d, want drilled at index 1 (sync reset the drill)", drilled, idx)
	}
}

// TestSyncKeepsAgendaHighlight guards that the agenda view is not affected by the
// reset-to-first class: its center highlight follows the agendaList's index, which
// refresh restores, so a background sync's refresh("") preserves the position.
func TestSyncKeepsAgendaHighlight(t *testing.T) {
	now := time.Date(2026, 7, 5, 9, 0, 0, 0, time.UTC)
	a := newRootedTestApp(t, now) // Agenda mode hides the Detail pane, which needs the full layout

	// Two events today so the agenda has a non-zero selectable index.
	calID := a.selectedCalendarID()
	if calID == "" {
		t.Fatal("no calendar selected")
	}
	today := model.DayStart(now)
	a.createEvent(calID, today, "Alpha 9am")
	a.createEvent(calID, today, "Beta 11am")

	a.setMode(modeAgenda)
	if n := a.agendaList.GetItemCount(); n < 2 {
		t.Fatalf("agenda has %d items, need >=2 to observe index preservation", n)
	}
	a.agendaList.SetCurrentItem(1)
	a.buildAgendaCenter()
	if a.agenda.selected != 1 {
		t.Fatalf("precondition: agenda.selected = %d, want 1", a.agenda.selected)
	}

	// The exact call a completed background sync makes.
	a.refresh("")

	if a.agendaList.GetCurrentItem() != 1 || a.agenda.selected != 1 {
		t.Errorf("after sync refresh: list index = %d, agenda.selected = %d, want both 1",
			a.agendaList.GetCurrentItem(), a.agenda.selected)
	}
}
