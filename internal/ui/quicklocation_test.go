package ui

import (
	"strings"
	"testing"
	"time"
)

// TestCreateWithLocationFromQuickAdd verifies an @location token flows into a
// created task (and its Detail-pane row) and a created event.
func TestCreateWithLocationFromQuickAdd(t *testing.T) {
	now := time.Date(2026, 7, 5, 9, 0, 0, 0, time.UTC)
	base := time.Date(2026, 7, 8, 0, 0, 0, 0, time.UTC)
	a := newWritableTestApp(t, now)
	a.loc = time.UTC

	// Event with a quoted multi-word location.
	calID := a.selectedCalendarID()
	a.createEvent(calID, base, "Class @\"room 204\" 9am")
	loc, ok := a.store.Locate(eventUID(t, a, "Class"))
	if !ok {
		t.Fatal("event not created")
	}
	if got := loc.Object.Events[0].Location; got != "room 204" {
		t.Errorf("event Location = %q, want %q", got, "room 204")
	}

	// Task with a single-word location; the Detail pane shows it.
	a.setMode(modeTasks)
	tcalID := a.selectedTasklistID()
	a.createTask(tcalID, "", "Pickup @depot tomorrow")
	td := todoBySummary(a.store, "Pickup")
	if td == nil {
		t.Fatal("task not created")
	}
	if td.Location != "depot" {
		t.Errorf("task Location = %q, want %q", td.Location, "depot")
	}
	a.setTodoDetail(td)
	if detail := a.detail.GetText(true); !strings.Contains(detail, "Location") || !strings.Contains(detail, "depot") {
		t.Errorf("task Detail pane missing Location row; got:\n%s", detail)
	}
}
