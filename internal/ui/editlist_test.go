package ui

import (
	"strings"
	"testing"
	"time"

	"github.com/rivo/tview"
)

// focusedFieldLabel returns the label of the InputField the app has focused (the
// form delegates focus into its first field), used to tell which form opened.
func focusedFieldLabel(t *testing.T, a *app) string {
	t.Helper()
	in, ok := a.tv.GetFocus().(*tview.InputField)
	if !ok {
		t.Fatalf("focused primitive is %T, want a form *tview.InputField", a.tv.GetFocus())
	}
	return in.GetLabel()
}

// TestEditFromTasksPaneEditsList locks M3: `e` with the Tasks overview panel
// focused edits the highlighted task *list* (the calendar form, first field
// "Name"), symmetric with `d` deleting it — previously `e` always edited the
// current tree *task* (first field "Summary") because currentTarget returns the
// tree node regardless of which pane holds focus.
func TestEditFromTasksPaneEditsList(t *testing.T) {
	now := time.Date(2026, 7, 5, 9, 0, 0, 0, time.UTC)
	a := newRootedTestApp(t, now)
	a.setMode(modeTasks)
	listID := a.selectedTasklistID()
	if listID == "" {
		t.Skip("no task list in the fixture")
	}
	// Give the tree a task, so a wrong target would open the task form instead —
	// making the assertion meaningful.
	a.createTask(listID, "", "Some Task")
	a.buildTree()

	// Tasks overview panel focused → edit opens the list (calendar) form.
	a.setFocus(a.tasklists)
	a.editSelected()
	if lbl := focusedFieldLabel(t, a); !strings.Contains(lbl, "Name") {
		t.Errorf("`e` on the Tasks pane opened a form whose first field is %q, want the list form (\"Name\")", lbl)
	}
	a.closeModal(pageForm)

	// Symmetry: with the tree focused, `e` still edits the task (summary form).
	a.setFocus(a.tree)
	a.editSelected()
	if lbl := focusedFieldLabel(t, a); !strings.Contains(lbl, "Summary") {
		t.Errorf("`e` on the tree opened a form whose first field is %q, want the task form (\"Summary\")", lbl)
	}
}
