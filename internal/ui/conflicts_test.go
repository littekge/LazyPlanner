package ui

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/littekge/LazyPlanner/internal/store"
)

// firstResource returns any (calendar, resource-name) present in the fixture.
func firstResource(a *app) (calID, name string, ok bool) {
	for _, c := range a.store.Calendars() {
		if len(c.Resources) > 0 {
			return c.ID, c.Resources[0].Name, true
		}
	}
	return "", "", false
}

func TestShowConflictsEmptyFlashes(t *testing.T) {
	a := newRootedTestApp(t, time.Date(2026, 7, 5, 12, 0, 0, 0, time.UTC))
	a.showConflicts()
	if a.root.HasPage(pageConflicts) {
		t.Error("conflicts overlay should not open when there are none")
	}
	if got := a.statusLeft.GetText(true); !strings.Contains(got, "No conflicts") {
		t.Errorf("flash = %q, want a no-conflicts hint", got)
	}
}

func TestShowConflictsListsItems(t *testing.T) {
	a := newRootedTestApp(t, time.Date(2026, 7, 5, 12, 0, 0, 0, time.UTC))
	calID, name, ok := firstResource(a)
	if !ok {
		t.Skip("fixture has no resources")
	}
	cal, _ := a.store.Calendar(calID)
	var r *store.Resource
	for _, x := range cal.Resources {
		if x.Name == name {
			r = x
		}
	}
	serverBytes, _ := r.Object.Encode()
	if err := a.store.MarkConflict(context.Background(), calID, name, serverBytes, "srv-x", false); err != nil {
		t.Fatal(err)
	}

	a.showConflicts()
	if !a.root.HasPage(pageConflicts) {
		t.Fatal("conflicts overlay did not open with a conflict present")
	}
	if len(a.store.Conflicts()) != 1 {
		t.Errorf("Conflicts() = %d, want 1", len(a.store.Conflicts()))
	}
}
