package ui

import (
	"context"
	"testing"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"

	"github.com/littekge/LazyPlanner/internal/store"
)

// twoResources returns two distinct (calID, name) resource pairs from the
// fixture, for tests that need more than one conflict to move a selection
// across.
func twoResources(t *testing.T, a *app) (calID1, name1, calID2, name2 string) {
	t.Helper()
	var pairs [][2]string
	for _, c := range a.store.Calendars() {
		for _, r := range c.Resources {
			pairs = append(pairs, [2]string{c.ID, r.Name})
		}
	}
	if len(pairs) < 2 {
		t.Skip("fixture needs at least two resources")
	}
	return pairs[0][0], pairs[0][1], pairs[1][0], pairs[1][1]
}

// TestConflictsListVimKeys: the Conflicts list must accept j/k as Down/Up
// aliases like every other list in the app (the app-wide vim-motion promise) —
// it previously only special-cased Esc/q in its own SetInputCapture, silently
// falling through to tview's List default (arrows only) for j/k.
func TestConflictsListVimKeys(t *testing.T) {
	now := time.Date(2026, 7, 5, 12, 0, 0, 0, time.UTC)
	a := newRootedTestApp(t, now)

	cal1, name1, cal2, name2 := twoResources(t, a)
	for _, p := range [][2]string{{cal1, name1}, {cal2, name2}} {
		cal, _ := a.store.Calendar(p[0])
		var r *store.Resource
		for _, x := range cal.Resources {
			if x.Name == p[1] {
				r = x
			}
		}
		serverBytes, _ := r.Object.Encode()
		if err := a.store.MarkConflict(context.Background(), p[0], p[1], serverBytes, "srv-"+p[1], false); err != nil {
			t.Fatal(err)
		}
	}
	if n := len(a.store.Conflicts()); n != 2 {
		t.Fatalf("precondition: want 2 conflicts, got %d", n)
	}

	a.showConflicts()
	list, ok := a.tv.GetFocus().(*tview.List)
	if !ok {
		t.Fatalf("conflicts list not focused; got %T", a.tv.GetFocus())
	}
	if got := list.GetCurrentItem(); got != 0 {
		t.Fatalf("precondition: selection starts at 0, got %d", got)
	}

	handle := list.InputHandler()
	handle(tcell.NewEventKey(tcell.KeyRune, 'j', tcell.ModNone), func(tview.Primitive) {})
	if got := list.GetCurrentItem(); got != 1 {
		t.Errorf("j: selection = %d, want 1", got)
	}
	handle(tcell.NewEventKey(tcell.KeyRune, 'k', tcell.ModNone), func(tview.Primitive) {})
	if got := list.GetCurrentItem(); got != 0 {
		t.Errorf("k: selection = %d, want 0", got)
	}
}

// TestAccountPickerVimKeys: same vim-motion promise for the :account picker's
// list.
func TestAccountPickerVimKeys(t *testing.T) {
	a := multiAccountApp(t)
	list := a.accountPickerList()
	if got := list.GetCurrentItem(); got != 0 {
		t.Fatalf("precondition: selection starts at 0 (personal), got %d", got)
	}

	handle := list.InputHandler()
	handle(tcell.NewEventKey(tcell.KeyRune, 'j', tcell.ModNone), func(tview.Primitive) {})
	if got := list.GetCurrentItem(); got != 1 {
		t.Errorf("j: selection = %d, want 1", got)
	}
	handle(tcell.NewEventKey(tcell.KeyRune, 'k', tcell.ModNone), func(tview.Primitive) {})
	if got := list.GetCurrentItem(); got != 0 {
		t.Errorf("k: selection = %d, want 0", got)
	}
}
