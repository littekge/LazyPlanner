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
	labels []string // base label per item index (without the gutter)
}

func newCaretForm() *caretForm {
	return &caretForm{Form: tview.NewForm()}
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

// styleModal gives a tview.Modal the shared popup chrome — the modal twin of
// stylePopup: terminal-default (unified) background and text, a reverse-video
// active button, and the accent rounded border + accent title every other window
// carries. tview.Modal draws its Box (border + title) but sets neither the accent
// color nor a title by default, so a bare confirm reads as a plainer style than
// the forms; routing all confirms/pickers through here keeps the chrome uniform.
func styleModal(m *tview.Modal, title string) {
	m.SetBackgroundColor(tcell.ColorDefault)
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
