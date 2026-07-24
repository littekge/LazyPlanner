package ui

import (
	"testing"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

func pickerKey(p *colorPicker) func(tcell.Key, rune) {
	handle := p.InputHandler()
	return func(k tcell.Key, r rune) {
		handle(tcell.NewEventKey(k, r, tcell.ModNone), func(tview.Primitive) {})
	}
}

func TestSameColor(t *testing.T) {
	cases := []struct {
		a, b string
		want bool
	}{
		{"#0082c9", "#0082c9", true},
		{"#0082c9", "#0082C9", true},   // case-insensitive
		{"#0082c9", "0082c9", true},    // leading # optional
		{"#0082c9", "#0082c9ff", true}, // alpha ignored (server form)
		{"#0082c9", "#0082c9aa", true}, // any alpha
		{"#0082c9", "#0082ca", false},  // different RGB
		{"#0082c9", "", false},         // unparseable
		{"nope", "#0082c9", false},
	}
	for _, c := range cases {
		if got := sameColor(c.a, c.b); got != c.want {
			t.Errorf("sameColor(%q, %q) = %v, want %v", c.a, c.b, got, c.want)
		}
	}
}

// TestColorPickerPreselect: a current color (incl. a server alpha suffix)
// preselects the matching palette swatch; a non-palette color lands on Custom;
// empty lands on the first swatch.
func TestColorPickerPreselect(t *testing.T) {
	p := newColorPicker()
	p.preselect("#0082c9ff") // NC blue + alpha → swatch index 6
	if p.cursor != 6 {
		t.Errorf("alpha color preselect = %d, want 6 (NC blue swatch)", p.cursor)
	}
	p.preselect("#123456") // not in the palette → Custom
	if p.cursor != p.customIndex() {
		t.Errorf("non-palette preselect = %d, want Custom %d", p.cursor, p.customIndex())
	}
	p.preselect("") // nothing chosen → first swatch
	if p.cursor != 0 {
		t.Errorf("empty preselect = %d, want 0", p.cursor)
	}
}

func TestColorPickerNavigation(t *testing.T) {
	p := newColorPicker()
	key := pickerKey(p)

	key(tcell.KeyRune, 'l') // 0 → 1
	key(tcell.KeyRune, 'j') // 1 → 6 (down a row)
	if p.cursor != 6 {
		t.Errorf("after l,j cursor=%d, want 6", p.cursor)
	}
	key(tcell.KeyRune, 'h') // 6 → 5
	key(tcell.KeyRune, 'h') // col 0 → no move
	if p.cursor != 5 {
		t.Errorf("after h,h cursor=%d, want 5 (left stops at col 0)", p.cursor)
	}
	// Right stops at the last column / last swatch.
	p.cursor = len(calendarPalette) - 1
	key(tcell.KeyRune, 'l')
	if p.cursor != len(calendarPalette)-1 {
		t.Errorf("right at last swatch moved to %d", p.cursor)
	}
	// Down from the bottom row drops to the Custom entry; up returns to the grid.
	p.cursor = 12 // bottom row
	key(tcell.KeyRune, 'j')
	if p.cursor != p.customIndex() {
		t.Errorf("down from bottom row = %d, want Custom %d", p.cursor, p.customIndex())
	}
	key(tcell.KeyRune, 'k')
	if p.cursor != len(calendarPalette)-1 {
		t.Errorf("up from Custom = %d, want last swatch %d", p.cursor, len(calendarPalette)-1)
	}
}

func TestColorPickerSelectCustomCancel(t *testing.T) {
	p := newColorPicker()
	var picked string
	var custom, cancelled bool
	p.onSelect = func(h string) { picked = h }
	p.onCustom = func() { custom = true }
	p.onCancel = func() { cancelled = true }
	key := pickerKey(p)

	p.cursor = 6
	key(tcell.KeyEnter, 0)
	if picked != calendarPalette[6] {
		t.Errorf("Enter on swatch 6 picked %q, want %q", picked, calendarPalette[6])
	}
	p.cursor = p.customIndex()
	key(tcell.KeyEnter, 0)
	if !custom {
		t.Error("Enter on Custom should call onCustom")
	}
	key(tcell.KeyEscape, 0)
	if !cancelled {
		t.Error("Esc should call onCancel")
	}
}

// TestColorPickerQCloses: ':help'/README promise that 'q' closes non-form
// dialogs — the swatch grid previously only closed on Esc (its InputHandler
// had no 'q' case). The grid is the only focusable primitive on the pageColor
// page; the "Custom hex…" entry hands off to a *separate* promptInput modal
// only after this picker has already closed (see openColorPickerCallback), so
// there is no in-picker hex text field 'q' could be swallowed away from.
func TestColorPickerQCloses(t *testing.T) {
	a := newRootedTestApp(t, time.Date(2026, 7, 5, 12, 0, 0, 0, time.UTC))
	a.openColorPickerCallback("", " Pick a color ", func(string) {})
	if !a.root.HasPage(pageColor) {
		t.Fatal("precondition: color picker should be open")
	}
	p, ok := a.tv.GetFocus().(*colorPicker)
	if !ok {
		t.Fatalf("color picker not focused; got %T", a.tv.GetFocus())
	}

	handle := p.InputHandler()
	handle(tcell.NewEventKey(tcell.KeyRune, 'q', tcell.ModNone), func(tview.Primitive) {})
	if a.root.HasPage(pageColor) {
		t.Error("'q' should close the color picker (the swatch grid is focused)")
	}
}

func TestApplyCalendarColor(t *testing.T) {
	a := newRootedTestApp(t, time.Date(2026, 7, 5, 12, 0, 0, 0, time.UTC))
	cals := a.store.Calendars()
	if len(cals) == 0 {
		t.Skip("fixture has no calendars")
	}
	id := cals[0].ID
	a.applyCalendarColor(id, "#123456")
	cal, ok := a.store.Calendar(id)
	if !ok || cal.Color != "#123456" {
		t.Errorf("calendar color = %q (ok=%v), want #123456", cal.Color, ok)
	}
}

func TestEditOnCalendarsPaneOpensForm(t *testing.T) {
	a := newRootedTestApp(t, time.Date(2026, 7, 5, 12, 0, 0, 0, time.UTC))
	a.setMode(modeCalendar) // no item drilled → e edits the calendar (name + color)
	a.editSelected()
	if !a.root.HasPage(pageForm) {
		t.Error("e on the Calendars pane should open the calendar edit form")
	}
}

// TestFocusStackNesting: the pre-modal focus stack pushes/pops so a modal opened
// over another (e.g. the color picker over the calendar form) unwinds correctly.
func TestFocusStackNesting(t *testing.T) {
	a := newRootedTestApp(t, time.Date(2026, 7, 5, 12, 0, 0, 0, time.UTC))
	base := len(a.focusStack)
	a.captureFocus()
	a.captureFocus()
	if len(a.focusStack) != base+2 {
		t.Fatalf("stack depth = %d, want %d after two captures", len(a.focusStack), base+2)
	}
	a.restoreFocus()
	a.restoreFocus()
	if len(a.focusStack) != base {
		t.Errorf("stack depth = %d, want %d after unwinding", len(a.focusStack), base)
	}
	a.restoreFocus() // extra restore on an empty stack must not panic
}

// TestCalendarFormCreatesWithColor: the create form's Create action stores the
// chosen color, so a new calendar is colored from the start (not left default).
func TestCalendarFormCreatesWithColor(t *testing.T) {
	a := newRootedTestApp(t, time.Date(2026, 7, 5, 12, 0, 0, 0, time.UTC))
	a.createCalendarWithColor("Projects", 0, "#0082c9") // test seam around the store call
	cal, ok := a.store.Calendar("Projects")
	if !ok {
		t.Fatal("calendar not created")
	}
	if cal.Color != "#0082c9" {
		t.Errorf("new calendar color = %q, want #0082c9", cal.Color)
	}
	if !cal.PendingCreate {
		t.Error("new calendar should be pending create (MKCALENDAR on next sync)")
	}
}

// TestCalendarCreateDefaultsColor: creating without a color falls back to the
// default (a palette color), so a new calendar/list is never colorless.
func TestCalendarCreateDefaultsColor(t *testing.T) {
	a := newRootedTestApp(t, time.Date(2026, 7, 5, 12, 0, 0, 0, time.UTC))
	if err := a.createCalendarWithColor("Uncolored", 1, ""); err != nil {
		t.Fatal(err)
	}
	cal, ok := a.store.Calendar("Uncolored")
	if !ok {
		t.Fatal("calendar not created")
	}
	if cal.Color != defaultCalendarColor {
		t.Errorf("new calendar color = %q, want the default %q", cal.Color, defaultCalendarColor)
	}
}

func TestCmdCalendarColorRoutes(t *testing.T) {
	a := newRootedTestApp(t, time.Date(2026, 7, 5, 12, 0, 0, 0, time.UTC))
	a.setMode(modeCalendar)
	id := a.currentCalendarID()
	if id == "" {
		t.Skip("no calendar highlighted")
	}

	// A hex argument sets the color directly (backward compatible).
	a.cmdCalendar("color #abcdef")
	if cal, _ := a.store.Calendar(id); cal.Color != "#abcdef" {
		t.Errorf("color = %q, want #abcdef", cal.Color)
	}
	// No argument opens the swatch picker.
	a.cmdCalendar("color")
	if !a.root.HasPage(pageColor) {
		t.Error(":calendar color with no arg should open the picker")
	}
}
