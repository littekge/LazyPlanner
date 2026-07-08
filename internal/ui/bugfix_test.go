package ui

import (
	"context"
	"testing"
	"time"

	"github.com/littekge/LazyPlanner/internal/store"
)

func TestOverviewPaneTitlesUseLetterKeys(t *testing.T) {
	a := newTestApp(t, time.Date(2026, 7, 5, 12, 0, 0, 0, time.UTC))
	want := map[string]string{
		a.calendars.Box.GetTitle():  " c Calendars ",
		a.tasklists.Box.GetTitle():  " t Tasks ",
		a.agendaList.Box.GetTitle(): " a Agenda ",
	}
	for got, exp := range want {
		if got != exp {
			t.Errorf("pane title = %q, want %q", got, exp)
		}
	}
}

func TestSupportsTodos(t *testing.T) {
	cases := []struct {
		comps []string
		want  bool
	}{
		{[]string{"VTODO"}, true},
		{[]string{"VEVENT", "VTODO"}, true},
		{[]string{"VEVENT"}, false},
		{nil, false}, // unknown component set + no todos → not a task list
	}
	for _, c := range cases {
		if got := supportsTodos(store.Calendar{Components: c.comps}); got != c.want {
			t.Errorf("supportsTodos(%v) = %v, want %v", c.comps, got, c.want)
		}
	}
}

func TestEmptyInAppListAppearsInTasksPanel(t *testing.T) {
	a := newWritableTestApp(t, time.Date(2026, 7, 5, 12, 0, 0, 0, time.UTC))
	// A brand-new, empty task list (VTODO) created in-app.
	if err := a.store.CreateCalendarLocal(context.Background(), "errands", store.CalendarMeta{DisplayName: "Errands"}, []string{"VTODO"}); err != nil {
		t.Fatalf("create list: %v", err)
	}
	a.buildTasklists()
	found := false
	for _, id := range a.tasklistIDs {
		if id == "errands" {
			found = true
		}
	}
	if !found {
		t.Error("an empty VTODO list should appear in the Tasks panel")
	}
}

func TestHideKeepsCalendarSelection(t *testing.T) {
	a := newWritableTestApp(t, time.Date(2026, 7, 5, 12, 0, 0, 0, time.UTC))
	a.setMode(modeCalendar)
	if a.calendars.GetItemCount() < 2 {
		t.Skip("need at least two calendars")
	}
	a.saveState = func(int, []string) {}
	a.calendars.SetCurrentItem(1)
	want := a.selectedCalendarID()

	a.toggleCalendarVisibility()
	if got := a.calendars.GetCurrentItem(); got != 1 {
		t.Errorf("selection moved to %d after hiding, want 1", got)
	}
	if got := a.selectedCalendarID(); got != want {
		t.Errorf("selected calendar = %q after hiding, want %q (stay put)", got, want)
	}
}
