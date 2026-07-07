package ui

import (
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
