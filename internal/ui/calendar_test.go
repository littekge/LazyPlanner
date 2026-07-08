package ui

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/littekge/LazyPlanner/internal/store"
)

func TestComponentsForType(t *testing.T) {
	tests := []struct {
		label string
		want  []string
	}{
		{"Event calendar", []string{"VEVENT"}},
		{"Task list", []string{"VTODO"}},
		{"Both", []string{"VEVENT", "VTODO"}}, // explicit so the type is "known"
	}
	for _, tc := range tests {
		got := componentsForType(tc.label)
		if !equalStringSlice(got, tc.want) {
			t.Errorf("componentsForType(%q) = %v, want %v", tc.label, got, tc.want)
		}
	}
}

func equalStringSlice(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestGuardComponentLocksItemTypeToCalendar(t *testing.T) {
	a := newWritableTestApp(t, time.Date(2026, 7, 5, 12, 0, 0, 0, time.UTC))
	ctx := context.Background()
	mk := func(id string, comps []string) {
		if err := a.store.CreateCalendarLocal(ctx, id, store.CalendarMeta{DisplayName: id}, comps); err != nil {
			t.Fatalf("create %s: %v", id, err)
		}
	}
	mk("ev", []string{"VEVENT"})
	mk("td", []string{"VTODO"})
	mk("both", []string{"VEVENT", "VTODO"})
	mk("unk", nil) // unknown/unconfirmed type

	cases := []struct {
		cal, want string
		ok        bool
	}{
		{"ev", compEvent, true}, {"ev", compTodo, false},
		{"td", compTodo, true}, {"td", compEvent, false},
		{"both", compEvent, true}, {"both", compTodo, true},
		{"unk", compEvent, false}, {"unk", compTodo, false}, // blocked until known
	}
	for _, c := range cases {
		if got := a.guardComponent(c.cal, c.want); got != c.ok {
			t.Errorf("guardComponent(%q, %q) = %v, want %v", c.cal, c.want, got, c.ok)
		}
	}
}

func TestCalTypeMarker(t *testing.T) {
	cases := []struct {
		comps []string
		want  string
	}{
		{[]string{"VEVENT"}, "[events]"},
		{[]string{"VTODO"}, "[tasks]"},
		{[]string{"VEVENT", "VTODO"}, "[both]"},
		{nil, "[?]"},
	}
	for _, c := range cases {
		if got := calTypeMarker(store.Calendar{Components: c.comps}); got != c.want {
			t.Errorf("calTypeMarker(%v) = %q, want %q", c.comps, got, c.want)
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
