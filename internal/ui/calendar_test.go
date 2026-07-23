package ui

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"

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

func TestForceOverridesUnknownTypeOnly(t *testing.T) {
	a := newWritableTestApp(t, time.Date(2026, 7, 5, 12, 0, 0, 0, time.UTC))
	ctx := context.Background()
	if err := a.store.CreateCalendarLocal(ctx, "unk", store.CalendarMeta{DisplayName: "U"}, nil); err != nil {
		t.Fatal(err)
	}
	if err := a.store.CreateCalendarLocal(ctx, "td", store.CalendarMeta{DisplayName: "T"}, []string{"VTODO"}); err != nil {
		t.Fatal(err)
	}

	// Without force: unknown type blocks.
	if a.guardComponent("unk", compEvent) {
		t.Error("unknown type should block without force")
	}
	a.forceCreate = true
	defer func() { a.forceCreate = false }()
	if !a.guardComponent("unk", compEvent) {
		t.Error("force should allow creation on an unknown-type calendar")
	}
	// Force must NOT override a *known* wrong type.
	if a.guardComponent("td", compEvent) {
		t.Error("force must not put an event on a known task-only list")
	}
}

func TestForceKeyArmsThenCreatesOnUnknownCalendar(t *testing.T) {
	a := newRootedTestApp(t, time.Date(2026, 7, 5, 12, 0, 0, 0, time.UTC))
	a.setMode(modeCalendar)
	cals := a.store.Calendars()
	idx := -1
	for i, c := range cals {
		if c.ID == "work" { // fixture "work" has no declared component set → [?]
			idx = i
		}
	}
	if idx < 0 {
		t.Skip("no unknown-type calendar in fixture")
	}
	a.calendars.SetCurrentItem(idx)

	// Plain `ie` on a [?] calendar is refused (no modal).
	a.startPrefix('i')
	a.resolvePrefix(runeKey('e'))
	if a.modalOpen() {
		t.Fatal("unforced ie on an unknown-type calendar should be blocked")
	}
	if !strings.Contains(a.statusLeft.GetText(true), "unknown type") {
		t.Errorf("expected an unknown-type flash, got %q", a.statusLeft.GetText(true))
	}

	// `i` `!` arms force (prefix stays pending); `e` then creates.
	a.startPrefix('i')
	a.resolvePrefix(runeKey('!'))
	if !a.pendingForce || a.pendingPrefix != 'i' {
		t.Fatalf("! should arm force and keep the prefix pending (force=%v prefix=%q)", a.pendingForce, a.pendingPrefix)
	}
	a.resolvePrefix(runeKey('e'))
	if !a.modalOpen() {
		t.Error("forced i!e should open the event prompt on an unknown-type calendar")
	}
	if a.forceCreate {
		t.Error("forceCreate should reset after the action runs")
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
		if strings.Contains(main, tview.Escape("[ro]")) {
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

func TestCollectionDeleteNameMatches(t *testing.T) {
	cases := []struct {
		name, typed, target string
		want                bool
	}{
		{"exact", "School", "School", true},
		{"trailing space trimmed", "School ", "School", true},
		{"leading space trimmed", "  School", "School", true},
		{"both sides trimmed", "  School  ", "  School  ", true},
		{"wrong case rejected", "school", "School", false},
		{"substring rejected", "Scho", "School", false},
		{"empty rejected", "", "School", false},
		{"internal spaces significant", "My List", "My List", true},
		{"internal spaces mismatch", "MyList", "My List", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := collectionDeleteNameMatches(c.typed, c.target); got != c.want {
				t.Errorf("collectionDeleteNameMatches(%q, %q) = %v, want %v", c.typed, c.target, got, c.want)
			}
		})
	}
}

// TestCollectionDeleteRequiresTypedName: the collection-delete dialog deletes
// only when the typed text matches the name — a wrong name keeps the form open
// and deletes nothing; the correct name (whitespace-padded, to exercise the trim)
// deletes and closes the modal.
func TestCollectionDeleteRequiresTypedName(t *testing.T) {
	a := newRootedTestApp(t, time.Date(2026, 7, 5, 12, 0, 0, 0, time.UTC))
	a.setMode(modeCalendar)
	id := a.selectedCalendarID()
	if id == "" {
		t.Skip("no calendar in the fixture")
	}
	cal, ok := a.store.Calendar(id)
	if !ok {
		t.Fatalf("selected calendar %q not found", id)
	}

	f := a.promptDeleteCollection(id, cal)
	in, ok := a.tv.GetFocus().(*tview.InputField)
	if !ok {
		t.Fatalf("focus after opening the dialog is %T, want the confirm *tview.InputField", a.tv.GetFocus())
	}

	// NORMAL-mode nav: g → first element (the input), j → the Delete button, Enter → activate.
	var focus func(tview.Primitive)
	focus = func(p tview.Primitive) { p.Focus(focus) }
	activateDelete := func() {
		focus(f)
		f.InputHandler()(runeKey('g'), focus)
		f.InputHandler()(runeKey('j'), focus)
		f.InputHandler()(keyEv(tcell.KeyEnter), focus)
	}

	// Wrong name: nothing deleted, form still open.
	in.SetText("definitely not the name")
	activateDelete()
	found := false
	for _, c := range a.store.Calendars() {
		if c.ID == id {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("a wrong name deleted the calendar")
	}
	if name, _ := a.root.GetFrontPage(); name != pageForm {
		t.Errorf("front page after a wrong name = %q, want the dialog still open (%q)", name, pageForm)
	}

	// Correct name, whitespace-padded: deleted, modal closed. MarkCalendarDeleted
	// marks a pendingDelete tombstone (the server DELETE + local removal happen on
	// the next sync), so Store.Calendar(id) — the internal lookup the sync engine
	// still needs to find the tombstone — keeps returning it; Calendars(), the
	// UI-facing list, is where the deletion is observable.
	in.SetText("  " + cal.DisplayName + "  ")
	activateDelete()
	for _, c := range a.store.Calendars() {
		if c.ID == id {
			t.Error("typing the correct name did not delete the calendar")
			break
		}
	}
	if name, _ := a.root.GetFrontPage(); name != pageMain {
		t.Errorf("front page after delete = %q, want the modal closed (%q)", name, pageMain)
	}
}
