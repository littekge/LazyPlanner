package ui

import (
	"strings"
	"testing"
)

// TestCaretFormGutter verifies the caret form's Draw applies the fixed label
// gutter to every field without panicking (the ▸-on-focus placement is verified
// end-to-end via a running terminal, since tview focus needs the live app).
func TestCaretFormGutter(t *testing.T) {
	f := newCaretForm()
	f.addInput("Alpha", "", 0)
	f.addDropDown("Prio", []string{"none", "1"}, 0)
	f.addCheckbox("Done", false)
	f.stylePopup()

	drawCells(t, f, 40, 12) // exercises the Draw override

	for i, base := range []string{"Alpha", "Prio", "Done"} {
		got := f.GetFormItem(i).GetLabel()
		if !strings.HasSuffix(got, base) || len(got) != len(base)+len(caretGutter) {
			t.Errorf("item %d label = %q, want a %d-col gutter + %q", i, got, len(caretGutter), base)
		}
	}
}
