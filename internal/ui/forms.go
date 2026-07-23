package ui

import (
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// caretForm is a tview.Form that marks the focused field with a ▸ caret in a
// fixed two-column label gutter. tview reapplies one field style to every field
// each frame (so a per-field "focused" color isn't possible), and the FormItem
// interface has no SetLabel — so the caret is set in Draw via GetFocusedItemIndex,
// type-switching to the concrete item to update its label.
type caretForm struct {
	*tview.Form
	labels   []string                // base label per item index (without the gutter)
	drilled  bool                    // DRILL mode: keys reach the focused field (NORMAL otherwise)
	onDrill  func(bool)              // notified on every drilled-state change (app wires the mode badge)
	appFocus func(p tview.Primitive) // the Application's focus setter; keeps a.focus in sync as nav moves focus
}

func newCaretForm() *caretForm {
	f := &caretForm{Form: tview.NewForm()}
	f.SetInputCapture(f.navKey)
	return f
}

func (f *caretForm) addInput(label, value string, width int) *tview.InputField {
	in := tview.NewInputField().SetLabel(caretGutter + label).SetText(value).SetFieldWidth(width)
	f.AddFormItem(in)
	f.labels = append(f.labels, label)
	return in
}

func (f *caretForm) addDropDown(label string, options []string, initial int) *tview.DropDown {
	dd := tview.NewDropDown().SetLabel(caretGutter+label).SetOptions(options, nil).SetCurrentOption(initial)
	// The open list must set a theme-adaptive selected style for the same reason
	// every tview.List does (see selectionStyle / TestSelectionIsLegible): the
	// app's terminal-default background makes tview's default selected style
	// (light bar, terminal-default ink) render illegibly. tcell.StyleDefault keeps
	// unselected rows on the unified background.
	dd.SetListStyles(tcell.StyleDefault, selectionStyle)
	f.AddFormItem(dd)
	f.labels = append(f.labels, label)
	return dd
}

func (f *caretForm) addCheckbox(label string, checked bool) *tview.Checkbox {
	cb := tview.NewCheckbox().SetLabel(caretGutter + label).SetChecked(checked)
	f.AddFormItem(cb)
	f.labels = append(f.labels, label)
	return cb
}

const (
	caretGutter = "  " // two columns; replaced by "▸ " on the focused field
	caretMarker = "▸ "
)

func (f *caretForm) Draw(screen tcell.Screen) {
	focused, _ := f.GetFocusedItemIndex()
	for i, base := range f.labels {
		gutter := caretGutter
		if i == focused {
			gutter = caretMarker
		}
		switch it := f.GetFormItem(i).(type) {
		case *tview.InputField:
			it.SetLabel(gutter + base)
		case *tview.DropDown:
			it.SetLabel(gutter + base)
		case *tview.Checkbox:
			it.SetLabel(gutter + base)
		}
	}
	f.Form.Draw(screen)
}

// --- DRILL-mode navigation ---
//
// The caret forms use the app-wide NORMAL/DRILL model in place of tview's Tab-only
// field movement. NORMAL (the resting state) navigates: j/k/arrows step the fields
// and buttons, h/l move between the buttons, g/G jump to the first field / last
// element, and Enter acts on the focused element (drill a text field, open a
// dropdown, toggle+advance a checkbox, or activate a button). DRILL hands keys to
// the focused text field so it edits normally; Enter commits and advances (auto-
// drilling the next text field), Esc returns to NORMAL keeping the value. The state
// is surfaced through the interactionMode badge via onDrill.

// count is the number of navigable elements: form items followed by buttons.
func (f *caretForm) count() int { return f.GetFormItemCount() + f.GetButtonCount() }

// currentIndex is the linear index of the focused element (items first, buttons
// last), defaulting to 0 when nothing reports focus.
func (f *caretForm) currentIndex() int {
	item, button := f.GetFocusedItemIndex()
	if button >= 0 {
		return f.GetFormItemCount() + button
	}
	if item >= 0 {
		return item
	}
	return 0
}

// isTextField reports whether the element at linear index i is an editable text
// field (the only element type NORMAL drills into).
func (f *caretForm) isTextField(i int) bool {
	if i < 0 || i >= f.GetFormItemCount() {
		return false
	}
	_, ok := f.GetFormItem(i).(*tview.InputField)
	return ok
}

func (f *caretForm) setDrilled(d bool) {
	f.drilled = d
	if f.onDrill != nil {
		f.onDrill(d)
	}
}

// focusElement focuses the element at linear index i. When the Application's focus
// setter is wired (the running app) it routes through it so a.focus tracks the
// leaf item — otherwise a later captureFocus would record a stale primitive and
// restoreFocus would land on the wrong element. Bare-widget tests fall back to the
// form-internal SetFocus.
func (f *caretForm) focusElement(i int) {
	if f.appFocus == nil {
		f.SetFocus(i)
		return
	}
	if items := f.GetFormItemCount(); i < items {
		f.appFocus(f.GetFormItem(i))
	} else {
		f.appFocus(f.GetButton(i - items))
	}
}

// moveFocus shifts focus to the clamped linear index i, auto-drilling when it lands
// on a text field and autoDrill is set; any other landing is NORMAL.
func (f *caretForm) moveFocus(i int, autoDrill bool) {
	n := f.count()
	if n == 0 {
		return
	}
	if i < 0 {
		i = 0
	}
	if i >= n {
		i = n - 1
	}
	f.focusElement(i)
	f.setDrilled(autoDrill && f.isTextField(i))
}

// navKey is the form's input capture: it runs before tview's item delegation, so
// returning nil swallows the key and returning the event passes it to the focused
// element's own handler.
func (f *caretForm) navKey(ev *tcell.EventKey) *tcell.EventKey {
	if f.drilled {
		return f.drillKey(ev)
	}
	return f.normalKey(ev)
}

func (f *caretForm) normalKey(ev *tcell.EventKey) *tcell.EventKey {
	cur := f.currentIndex()
	switch ev.Key() {
	case tcell.KeyDown, tcell.KeyTab:
		f.moveFocus(cur+1, false)
		return nil
	case tcell.KeyUp, tcell.KeyBacktab:
		f.moveFocus(cur-1, false)
		return nil
	case tcell.KeyRight:
		f.moveButton(cur, +1)
		return nil
	case tcell.KeyLeft:
		f.moveButton(cur, -1)
		return nil
	case tcell.KeyEnter:
		return f.actNormal(cur, ev)
	case tcell.KeyEscape:
		return ev // let the focused item's finished-handler fire the form's cancel
	case tcell.KeyRune:
		switch ev.Rune() {
		case 'j':
			f.moveFocus(cur+1, false)
		case 'k':
			f.moveFocus(cur-1, false)
		case 'l':
			f.moveButton(cur, +1)
		case 'h':
			f.moveButton(cur, -1)
		case 'g':
			f.moveFocus(0, false)
		case 'G':
			f.moveFocus(f.count()-1, false)
		}
		return nil // every other rune is inert in NORMAL (no typing leaks through)
	}
	return nil // swallow everything else in NORMAL
}

// actNormal handles Enter in NORMAL by element type: drill a text field, open a
// dropdown (delegated to tview), toggle+advance a checkbox, or activate a button.
func (f *caretForm) actNormal(cur int, ev *tcell.EventKey) *tcell.EventKey {
	if cur >= f.GetFormItemCount() {
		return ev // a button: let its own handler run the action
	}
	switch it := f.GetFormItem(cur).(type) {
	case *tview.InputField:
		f.setDrilled(true)
		return nil
	case *tview.DropDown:
		return ev // tview opens the list; its native nav/selection takes over
	case *tview.Checkbox:
		it.SetChecked(!it.IsChecked())
		f.moveFocus(cur+1, false)
		return nil
	}
	return nil
}

// moveButton moves within the button row (h/l). It is inert on a field and clamps
// at the row's ends, so h/l never leak between the buttons and the fields.
func (f *caretForm) moveButton(cur, delta int) {
	items := f.GetFormItemCount()
	if cur < items {
		return // on a field: h/l are inert
	}
	target := cur + delta
	if target < items {
		target = items
	}
	if target > f.count()-1 {
		target = f.count() - 1
	}
	f.focusElement(target)
	f.setDrilled(false)
}

func (f *caretForm) drillKey(ev *tcell.EventKey) *tcell.EventKey {
	cur := f.currentIndex()
	switch ev.Key() {
	case tcell.KeyEscape:
		f.setDrilled(false) // back to NORMAL, keeping the field's current value
		return nil
	case tcell.KeyEnter, tcell.KeyTab:
		f.moveFocus(cur+1, true) // commit + advance, auto-drilling the next text field
		return nil
	case tcell.KeyBacktab:
		f.moveFocus(cur-1, true)
		return nil
	}
	return ev // typing, cursor motion, backspace, … all reach the focused field
}

// styleModal gives a tview.Modal the shared popup chrome — the modal twin of
// stylePopup: terminal-default (unified) background and text, a reverse-video
// active button, and the accent rounded border + accent title every other window
// carries. tview.Modal draws its Box (border + title) but sets neither the accent
// color nor a title by default, so a bare confirm reads as a plainer style than
// the forms; routing all confirms/pickers through here keeps the chrome uniform.
func styleModal(m *tview.Modal, title string) {
	m.SetBackgroundColor(tcell.ColorDefault)
	// Modal.SetBackgroundColor resets only the frame/form, leaving the embedded
	// Box at tview's default ContrastBackgroundColor — which fills the box and the
	// border's background, drawing a highlighted band around the content. Reset the
	// Box directly so the border sits on the unified terminal-default background.
	m.Box.SetBackgroundColor(tcell.ColorDefault)
	m.SetTextColor(tcell.ColorDefault)
	m.SetButtonBackgroundColor(tcell.ColorDefault)
	m.SetButtonTextColor(tcell.ColorDefault)
	m.SetButtonActivatedStyle(tcell.StyleDefault.Reverse(true))
	m.SetBorder(true)
	m.SetBorderColor(accentColor)
	m.SetTitle(title)
	m.SetTitleColor(accentColor)
}

// stylePopup gives a form the shared popup look: the terminal's default (unified)
// background, high-contrast default text, an accent rounded border/title, and the
// focused button reversed. Field focus is shown by the caret, not a field color.
func (f *caretForm) stylePopup() {
	f.SetBackgroundColor(tcell.ColorDefault)
	f.SetLabelColor(tcell.ColorDefault)
	f.SetFieldBackgroundColor(tcell.ColorDefault)
	f.SetFieldTextColor(tcell.ColorDefault)
	f.SetButtonBackgroundColor(tcell.ColorDefault)
	f.SetButtonTextColor(tcell.ColorDefault)
	f.SetButtonActivatedStyle(tcell.StyleDefault.Reverse(true))
	f.SetBorderColor(accentColor)
	f.SetTitleColor(accentColor)
}
