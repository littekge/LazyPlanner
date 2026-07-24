package ui

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/rivo/tview"

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

func TestConflictsModalTitleAdvertisesQ(t *testing.T) {
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
	// Verify the conflicts overlay opened
	if !a.root.HasPage(pageConflicts) {
		t.Fatal("conflicts overlay did not open")
	}

	// Get the page name and primitive
	pageName, prim := a.root.GetFrontPage()
	if pageName != pageConflicts {
		t.Fatalf("front page = %q, want %q", pageName, pageConflicts)
	}

	// modalWrap wraps the list in a Flex; we need to find the actual list inside
	flex, ok := prim.(*tview.Flex)
	if !ok {
		t.Fatal("conflicts page wrapper is not a Flex")
	}

	// Extract the list from the flex structure (it should be in the inner flex)
	var list *tview.List
	for i := 0; i < flex.GetItemCount(); i++ {
		item := flex.GetItem(i)
		innerFlex, ok := item.(*tview.Flex)
		if ok {
			for j := 0; j < innerFlex.GetItemCount(); j++ {
				innerItem := innerFlex.GetItem(j)
				if l, ok := innerItem.(*tview.List); ok {
					list = l
					break
				}
			}
		}
	}

	if list == nil {
		t.Fatal("could not find conflicts list in page")
	}

	title := list.GetTitle()
	if !strings.Contains(title, "Esc/q") {
		t.Errorf("conflicts title = %q, want to advertise Esc/q", title)
	}
}
