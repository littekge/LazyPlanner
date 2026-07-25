package ui

import (
	"context"
	"strings"
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

// TestCalendarColorInvalidHexDoesNotEcho: the command-echo slot (a.statusMid,
// written by a.echo) is documented (main.md) as showing "the most recently
// *executed* action" — a rejected ":calendar color <bad-hex>" must not land
// there, or the status bar lies about what actually ran.
func TestCalendarColorInvalidHexDoesNotEcho(t *testing.T) {
	now := time.Date(2026, 7, 5, 9, 0, 0, 0, time.UTC)
	a := newWritableTestApp(t, now)
	a.setMode(modeCalendar)
	if a.calendars.GetItemCount() == 0 {
		t.Skip("fixture has no calendars")
	}
	a.calendars.SetCurrentItem(0)

	a.cmdCalendar("color not-a-hex-color")
	if got := a.statusMid.GetText(true); strings.Contains(got, ":calendar color") {
		t.Errorf("echo = %q, a rejected :calendar color must not echo", got)
	}
}

// TestCalendarColorReadOnlyDoesNotEcho: ":calendar color <hex>" on a read-only
// calendar is rejected by guardWrite — same "no echo on rejection" rule as the
// invalid-hex case above, for the guardWrite path specifically.
func TestCalendarColorReadOnlyDoesNotEcho(t *testing.T) {
	now := time.Date(2026, 7, 5, 9, 0, 0, 0, time.UTC)
	a := newWritableTestApp(t, now)
	a.setMode(modeCalendar)
	if a.calendars.GetItemCount() == 0 {
		t.Skip("fixture has no calendars")
	}
	a.calendars.SetCurrentItem(0)
	id := a.selectedCalendarID()
	if err := a.store.SetCalendarReadOnly(context.Background(), id, true); err != nil {
		t.Fatal(err)
	}

	a.cmdCalendar("color #3366cc")
	if got := a.statusMid.GetText(true); strings.Contains(got, ":calendar color") {
		t.Errorf("echo = %q, :calendar color on a read-only calendar must not echo", got)
	}
}

// TestCalendarColorValidStillEchoes: the fix for the two rejection cases above
// must not cost the success path its echo.
func TestCalendarColorValidStillEchoes(t *testing.T) {
	now := time.Date(2026, 7, 5, 9, 0, 0, 0, time.UTC)
	a := newWritableTestApp(t, now)
	a.setMode(modeCalendar)
	if a.calendars.GetItemCount() == 0 {
		t.Skip("fixture has no calendars")
	}
	a.calendars.SetCurrentItem(0)

	a.cmdCalendar("color #3366cc")
	if got := a.statusMid.GetText(true); !strings.Contains(got, ":calendar color") {
		t.Errorf("echo = %q, a successful :calendar color should echo", got)
	}
}
