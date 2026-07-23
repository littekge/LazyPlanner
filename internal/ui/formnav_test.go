package ui

import (
	"testing"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// keyEv builds a bare key event (no rune) for the special keys the form nav uses.
func keyEv(k tcell.Key) *tcell.EventKey { return tcell.NewEventKey(k, 0, tcell.ModNone) }

// driveForm focuses f (delegating into its first element, as the running app does
// when a modal opens) and returns a send func that pushes a key event through the
// form's full input path — the DRILL-mode capture and, when it passes the event
// on, the focused item's own handler.
func driveForm(t *testing.T, f *caretForm) func(*tcell.EventKey) {
	t.Helper()
	var focus func(tview.Primitive)
	focus = func(p tview.Primitive) { p.Focus(focus) }
	focus(f)
	return func(ev *tcell.EventKey) {
		if h := f.InputHandler(); h != nil {
			h(ev, focus)
		}
	}
}

// TestFormOpensInNormalAndNavigatesWithoutTyping: a form opens in NORMAL with the
// first field active; j/k step fields and letters do not edit the field.
func TestFormOpensInNormalAndNavigatesWithoutTyping(t *testing.T) {
	f := newCaretForm()
	alpha := f.addInput("Alpha", "", 0)
	f.addInput("Beta", "", 0)
	f.addCheckbox("Done", false)
	f.stylePopup()
	send := driveForm(t, f)

	if f.drilled {
		t.Fatal("a form should open in NORMAL, not drilled")
	}
	if idx := f.currentIndex(); idx != 0 {
		t.Fatalf("focus starts at %d, want 0", idx)
	}

	send(runeKey('x')) // a letter in NORMAL must not reach the field
	if alpha.GetText() != "" {
		t.Errorf("typing in NORMAL edited the field: %q", alpha.GetText())
	}

	send(runeKey('j'))
	if idx := f.currentIndex(); idx != 1 {
		t.Errorf("after j, focus=%d want 1", idx)
	}
	send(runeKey('k'))
	if idx := f.currentIndex(); idx != 0 {
		t.Errorf("after k, focus=%d want 0", idx)
	}
}

// TestFormEnterDrillsTextFieldThenTypes: Enter on a text field drills it, and the
// then-typed keys (incl. h/j/k/l as letters) edit its value.
func TestFormEnterDrillsTextFieldThenTypes(t *testing.T) {
	f := newCaretForm()
	alpha := f.addInput("Alpha", "", 0)
	f.stylePopup()
	send := driveForm(t, f)

	send(keyEv(tcell.KeyEnter))
	if !f.drilled {
		t.Fatal("Enter on a text field should drill into it")
	}
	for _, r := range "hjkl" {
		send(runeKey(r))
	}
	if alpha.GetText() != "hjkl" {
		t.Errorf("drilled typing = %q, want hjkl (hjkl must be letters in DRILL)", alpha.GetText())
	}
}

// TestFormDrillEscReturnsToNormalKeepingValue: Esc in DRILL returns to NORMAL and
// keeps the edited value (it does not cancel the form).
func TestFormDrillEscReturnsToNormalKeepingValue(t *testing.T) {
	f := newCaretForm()
	alpha := f.addInput("Alpha", "seed", 0)
	f.stylePopup()
	cancelled := false
	f.SetCancelFunc(func() { cancelled = true })
	send := driveForm(t, f)

	send(keyEv(tcell.KeyEnter)) // drill
	send(runeKey('X'))          // append
	send(keyEv(tcell.KeyEscape))
	if f.drilled {
		t.Error("Esc in DRILL should return to NORMAL")
	}
	if cancelled {
		t.Error("Esc in DRILL must not cancel the form")
	}
	if got := alpha.GetText(); got != "seedX" {
		t.Errorf("value after Esc = %q, want seedX (kept)", got)
	}
}

// TestFormNormalEscTriggersCancel: Esc in NORMAL closes the form via its cancel.
func TestFormNormalEscTriggersCancel(t *testing.T) {
	f := newCaretForm()
	f.addInput("Alpha", "", 0)
	f.stylePopup()
	cancelled := false
	f.SetCancelFunc(func() { cancelled = true })
	send := driveForm(t, f)

	send(keyEv(tcell.KeyEscape))
	if !cancelled {
		t.Error("Esc in NORMAL should trigger the form's cancel")
	}
}

// TestFormDrillEnterAdvancesAutoDrillingNextTextField: Enter in DRILL commits and
// advances, auto-drilling the next field when it is also a text field.
func TestFormDrillEnterAdvancesAutoDrillingNextTextField(t *testing.T) {
	f := newCaretForm()
	f.addInput("Alpha", "", 0)
	beta := f.addInput("Beta", "", 0)
	f.stylePopup()
	send := driveForm(t, f)

	send(keyEv(tcell.KeyEnter)) // drill Alpha
	send(keyEv(tcell.KeyEnter)) // commit + advance to Beta
	if idx := f.currentIndex(); idx != 1 {
		t.Fatalf("focus=%d want 1 (Beta)", idx)
	}
	if !f.drilled {
		t.Error("advancing onto a text field should auto-drill it")
	}
	send(runeKey('y'))
	if beta.GetText() != "y" {
		t.Errorf("auto-drilled field text=%q want y", beta.GetText())
	}
}

// TestFormDrillEnterStopsNormalOnNonTextField: Enter in DRILL advancing onto a
// non-text field (a checkbox) lands in NORMAL, not drilled.
func TestFormDrillEnterStopsNormalOnNonTextField(t *testing.T) {
	f := newCaretForm()
	f.addInput("Alpha", "", 0)
	f.addCheckbox("Done", false)
	f.stylePopup()
	send := driveForm(t, f)

	send(keyEv(tcell.KeyEnter)) // drill Alpha
	send(keyEv(tcell.KeyEnter)) // advance to the checkbox
	if idx := f.currentIndex(); idx != 1 {
		t.Fatalf("focus=%d want 1 (checkbox)", idx)
	}
	if f.drilled {
		t.Error("advancing onto a checkbox must stop in NORMAL")
	}
}

// TestFormNormalEnterTogglesCheckboxAndAdvances: Enter on a checkbox in NORMAL
// toggles it and advances one element, staying in NORMAL.
func TestFormNormalEnterTogglesCheckboxAndAdvances(t *testing.T) {
	f := newCaretForm()
	f.addCheckbox("Done", false)
	f.addInput("Alpha", "", 0)
	f.stylePopup()
	cb := f.GetFormItem(0).(*tview.Checkbox)
	send := driveForm(t, f)

	send(keyEv(tcell.KeyEnter))
	if !cb.IsChecked() {
		t.Error("Enter on a checkbox should toggle it on")
	}
	if idx := f.currentIndex(); idx != 1 {
		t.Errorf("focus=%d want 1 (advanced past the checkbox)", idx)
	}
	if f.drilled {
		t.Error("a checkbox toggle+advance should land in NORMAL")
	}
}

// TestFormGGJumpFirstAndLast: g jumps to the first field, G to the last element
// (the last button).
func TestFormGGJumpFirstAndLast(t *testing.T) {
	f := newCaretForm()
	f.addInput("Alpha", "", 0)
	f.addInput("Beta", "", 0)
	f.AddButton("Save", func() {})
	f.AddButton("Cancel", func() {})
	f.stylePopup()
	send := driveForm(t, f)

	send(runeKey('G'))
	if idx, last := f.currentIndex(), f.count()-1; idx != last {
		t.Errorf("G focus=%d want last %d", idx, last)
	}
	send(runeKey('g'))
	if idx := f.currentIndex(); idx != 0 {
		t.Errorf("g focus=%d want 0", idx)
	}
}

// TestFormHLMoveBetweenButtonsOnly: h/l move between the buttons and clamp within
// the button row; on a field they are inert.
func TestFormHLMoveBetweenButtonsOnly(t *testing.T) {
	f := newCaretForm()
	f.addInput("Alpha", "", 0)
	f.AddButton("Save", func() {})
	f.AddButton("Cancel", func() {})
	f.stylePopup()
	send := driveForm(t, f)

	send(runeKey('l')) // on the field: inert
	if idx := f.currentIndex(); idx != 0 {
		t.Errorf("l on a field should be inert, idx=%d want 0", idx)
	}

	send(runeKey('G')) // Cancel (index 2)
	if idx := f.currentIndex(); idx != 2 {
		t.Fatalf("start button idx=%d want 2", idx)
	}
	send(runeKey('h')) // Save (index 1)
	if idx := f.currentIndex(); idx != 1 {
		t.Errorf("after h idx=%d want 1", idx)
	}
	send(runeKey('h')) // clamp at first button
	if idx := f.currentIndex(); idx != 1 {
		t.Errorf("h at the first button leaked into fields, idx=%d want 1", idx)
	}
	send(runeKey('l')) // Cancel (index 2)
	if idx := f.currentIndex(); idx != 2 {
		t.Errorf("after l idx=%d want 2", idx)
	}
}

// TestFormNormalEnterActivatesButton: Enter on a focused button runs its action.
func TestFormNormalEnterActivatesButton(t *testing.T) {
	f := newCaretForm()
	f.addInput("Alpha", "", 0)
	clicked := false
	f.AddButton("Save", func() { clicked = true })
	f.stylePopup()
	send := driveForm(t, f)

	send(runeKey('G')) // to the Save button
	send(keyEv(tcell.KeyEnter))
	if !clicked {
		t.Error("Enter on a button should activate it")
	}
}

// TestFormTabAliasesAdvance: Tab/Backtab remain advance/previous aliases in NORMAL.
func TestFormTabAliasesAdvance(t *testing.T) {
	f := newCaretForm()
	f.addInput("Alpha", "", 0)
	f.addInput("Beta", "", 0)
	f.stylePopup()
	send := driveForm(t, f)

	send(keyEv(tcell.KeyTab))
	if idx := f.currentIndex(); idx != 1 {
		t.Errorf("Tab should advance, idx=%d want 1", idx)
	}
	send(keyEv(tcell.KeyBacktab))
	if idx := f.currentIndex(); idx != 0 {
		t.Errorf("Backtab should retreat, idx=%d want 0", idx)
	}
}

// TestFormOpenDropdownReceivesArrowKeys: once a dropdown is open, the form's nav
// capture must step aside so the native list gets ↑/↓/Enter — otherwise NORMAL
// swallows the arrows as field navigation and the open list can't be steered.
func TestFormOpenDropdownReceivesArrowKeys(t *testing.T) {
	f := newCaretForm()
	dd := f.addDropDown("Pick", []string{"a", "b", "c"}, 0)
	f.stylePopup()
	send := driveForm(t, f)

	send(keyEv(tcell.KeyEnter)) // NORMAL Enter on a dropdown opens it
	if !dd.IsOpen() {
		t.Skip("could not open the dropdown in this build")
	}
	send(keyEv(tcell.KeyDown))  // move the open list's highlight to option 1
	send(keyEv(tcell.KeyEnter)) // select it
	if idx, _ := dd.GetCurrentOption(); idx != 1 {
		t.Errorf("arrow+Enter in the open dropdown selected option %d, want 1 (arrows not reaching the list)", idx)
	}
}

// TestFormNavKeepsAppFocusInSync: navigating with j must move the Application's
// tracked focus (GetFocus) too, not just the form-internal focus — otherwise a
// nested modal opened afterwards captures a stale primitive and restores focus to
// the wrong place on close (the softlock-adjacent focus bug).
func TestFormNavKeepsAppFocusInSync(t *testing.T) {
	a := newRootedTestApp(t, time.Date(2026, 7, 6, 12, 0, 0, 0, time.UTC))
	f := newCaretForm()
	f.addInput("Alpha", "", 0)
	beta := f.addInput("Beta", "", 0)
	f.stylePopup()
	a.openModal(pageForm, f, 40, 10)

	// After openModal focus is on Alpha; j should advance the app focus to Beta.
	setFocus := func(p tview.Primitive) { a.tv.SetFocus(p) }
	f.InputHandler()(runeKey('j'), setFocus)
	if got := a.tv.GetFocus(); got != beta {
		t.Errorf("app focus after j = %T (%p), want the Beta field (%p)", got, got, beta)
	}
	a.closeModal(pageForm)
}

// TestFormDrillSurfacesInModeBadge: a form opens NORMAL in the badge; drilling a
// field flips it to DRILL; closing the form resets it.
func TestFormDrillSurfacesInModeBadge(t *testing.T) {
	a := newRootedTestApp(t, time.Date(2026, 7, 6, 12, 0, 0, 0, time.UTC))
	f := newCaretForm()
	f.addInput("Alpha", "", 0)
	f.AddButton("Save", func() {})
	f.stylePopup()
	a.openModal(pageForm, f, 40, 10)

	if got := a.interactionMode(); got != modeNormal {
		t.Fatalf("a freshly opened form should read NORMAL, got %s", got)
	}

	var focus func(tview.Primitive)
	focus = func(p tview.Primitive) { p.Focus(focus) }
	focus(f)
	f.InputHandler()(keyEv(tcell.KeyEnter), focus) // drill the text field
	if !f.drilled {
		t.Fatal("Enter did not drill the field")
	}
	if got := a.interactionMode(); got != modeDrill {
		t.Errorf("badge=%s want DRILL while a field is drilled", got)
	}

	a.closeModal(pageForm)
	if a.formDrill {
		t.Error("closing the form should reset formDrill")
	}
}
