package ui

import (
	"testing"
	"time"
)

func TestNormalizeColor(t *testing.T) {
	cases := []struct {
		in   string
		want string
		ok   bool
	}{
		{"#3366cc", "#3366cc", true},
		{"3366cc", "#3366cc", true}, // leading # added
		{"#3366ccff", "#3366ccff", true},
		{"", "", false},
		{"#12345", "", false},  // wrong length
		{"#gggggg", "", false}, // non-hex
		{"blueish", "", false},
	}
	for _, c := range cases {
		got, ok := normalizeColor(c.in)
		if got != c.want || ok != c.ok {
			t.Errorf("normalizeColor(%q) = (%q,%v), want (%q,%v)", c.in, got, ok, c.want, c.ok)
		}
	}
}

func TestCalendarRenameUpdatesLocalName(t *testing.T) {
	now := time.Date(2026, 7, 5, 9, 0, 0, 0, time.UTC)
	a := newWritableTestApp(t, now)
	a.setMode(modeCalendar)
	if a.calendars.GetItemCount() == 0 {
		t.Skip("fixture has no calendars")
	}
	a.calendars.SetCurrentItem(0)
	id := a.selectedCalendarID()

	a.cmdCalendar("rename Renamed List")
	cal, ok := a.store.Calendar(id)
	if !ok || cal.DisplayName != "Renamed List" {
		t.Errorf("calendar %q display name = %q, want %q", id, cal.DisplayName, "Renamed List")
	}
}

func TestCalendarHideShowViaCommand(t *testing.T) {
	now := time.Date(2026, 7, 5, 9, 0, 0, 0, time.UTC)
	a := newWritableTestApp(t, now)
	a.setMode(modeCalendar)
	if a.calendars.GetItemCount() == 0 {
		t.Skip("fixture has no calendars")
	}
	a.saveState = func(int, int, []string, int) {}
	a.calendars.SetCurrentItem(0)
	id := a.selectedCalendarID()

	a.cmdCalendar("hide")
	if !a.hidden[id] {
		t.Errorf(":calendar hide did not hide %q", id)
	}
	a.cmdCalendar("show")
	if a.hidden[id] {
		t.Errorf(":calendar show did not un-hide %q", id)
	}
}

func TestCalendarNewOpensForm(t *testing.T) {
	a := newRootedTestApp(t, time.Date(2026, 7, 5, 9, 0, 0, 0, time.UTC))
	a.setMode(modeCalendar)
	a.cmdCalendar("new")
	if !a.root.HasPage(pageForm) {
		t.Error(":calendar new should open the create/edit calendar form")
	}
}
