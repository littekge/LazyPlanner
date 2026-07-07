package ui

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestComponentsForType(t *testing.T) {
	tests := []struct {
		label string
		want  []string
	}{
		{"Event calendar", []string{"VEVENT"}},
		{"Task list", []string{"VTODO"}},
		{"Both", nil},
	}
	for _, tc := range tests {
		got := componentsForType(tc.label)
		if len(got) != len(tc.want) || (len(got) == 1 && got[0] != tc.want[0]) {
			t.Errorf("componentsForType(%q) = %v, want %v", tc.label, got, tc.want)
		}
	}
}

func TestGuardWriteBlocksReadOnly(t *testing.T) {
	a := newWritableTestApp(t, time.Date(2026, 7, 5, 12, 0, 0, 0, time.UTC))
	cals := a.store.Calendars()
	if len(cals) == 0 {
		t.Skip("fixture has no calendars")
	}
	id := cals[0].ID
	if !a.guardWrite(id) {
		t.Fatal("a writable calendar should not be guarded")
	}
	if err := a.store.SetCalendarReadOnly(context.Background(), id, true); err != nil {
		t.Fatal(err)
	}
	if a.guardWrite(id) {
		t.Error("guardWrite should block a read-only calendar")
	}
	if got := a.statusLeft.GetText(true); !strings.Contains(got, "read-only") {
		t.Errorf("flash = %q, want a read-only hint", got)
	}
}

func TestReadOnlyCalendarShowsMarker(t *testing.T) {
	a := newWritableTestApp(t, time.Date(2026, 7, 5, 12, 0, 0, 0, time.UTC))
	cals := a.store.Calendars()
	if len(cals) == 0 {
		t.Skip("fixture has no calendars")
	}
	if err := a.store.SetCalendarReadOnly(context.Background(), cals[0].ID, true); err != nil {
		t.Fatal(err)
	}
	a.buildCalendars()
	found := false
	for i := 0; i < a.calendars.GetItemCount(); i++ {
		main, _ := a.calendars.GetItemText(i)
		if strings.Contains(main, "[ro]") {
			found = true
		}
	}
	if !found {
		t.Error("read-only calendar not marked [ro] in the Calendars list")
	}
}

// TestDeleteCollectionNeedsCollectionPane guards that D outside the Calendars /
// Tasks panes flashes a hint rather than acting.
func TestDeleteCollectionNeedsCollectionPane(t *testing.T) {
	a := newTestApp(t, time.Date(2026, 7, 5, 12, 0, 0, 0, time.UTC))
	a.mode = modeAgenda
	a.deleteCollection()
	if got := a.statusLeft.GetText(true); !strings.Contains(got, "Calendars") {
		t.Errorf("flash = %q, want a hint to switch panes", got)
	}
}
