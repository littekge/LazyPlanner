package ui

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/littekge/LazyPlanner/internal/model"
	"github.com/littekge/LazyPlanner/internal/store"
)

// TestTreeSubtreeZoom: `>` re-roots the tree at the selected task with a
// breadcrumb; `<` pops back to the list root.
func TestTreeSubtreeZoom(t *testing.T) {
	now := time.Date(2026, 7, 5, 9, 0, 0, 0, time.UTC)
	a := newWritableTestApp(t, now)
	a.setMode(modeTasks)
	cal := a.selectedTasklistID()
	a.createTask(cal, "", "ECE384")
	parent := todoBySummary(a.store, "ECE384")
	a.createTask(cal, parent.UID, "lab 3")
	a.buildTree()

	// Zoom in on ECE384.
	a.selectTreeByUID(parent.UID)
	a.zoomInTree()
	if a.zoomUID != parent.UID {
		t.Fatalf("zoomUID = %q, want %q", a.zoomUID, parent.UID)
	}
	root := a.tree.GetRoot()
	if got := root.GetText(); !strings.Contains(got, "ECE384") || !strings.Contains(got, "/") {
		t.Errorf("zoomed root breadcrumb = %q, want a 'List / ECE384' path", got)
	}
	kids := root.GetChildren()
	if len(kids) != 1 {
		t.Fatalf("zoomed root has %d children, want 1 (lab 3)", len(kids))
	}
	if td, _ := kids[0].GetReference().(*model.Todo); td == nil || td.Summary != "lab 3" {
		t.Errorf("zoomed child = %v, want lab 3", kids[0].GetReference())
	}

	// Zoom out → back to the list root, ECE384 a top-level child again.
	a.zoomOutTree()
	if a.zoomUID != "" {
		t.Errorf("zoomUID after zoom-out = %q, want empty", a.zoomUID)
	}
	found := false
	for _, k := range a.tree.GetRoot().GetChildren() {
		if td, _ := k.GetReference().(*model.Todo); td != nil && td.UID == parent.UID {
			found = true
		}
	}
	if !found {
		t.Error("ECE384 not back at top level after zoom-out")
	}

	// Switching to a different list clears the zoom.
	if err := a.store.CreateCalendarLocal(context.Background(), "zzlist2", store.CalendarMeta{DisplayName: "List2"}, []string{"VTODO"}); err != nil {
		t.Fatal(err)
	}
	a.reload()
	a.setMode(modeTasks)
	if len(a.tasklistIDs) < 2 {
		t.Skip("need two task lists to test the zoom reset")
	}
	a.tasklists.SetCurrentItem(0)
	a.zoomUID = parent.UID
	a.tasklists.SetCurrentItem(1) // fires the changed-callback → list change
	if a.zoomUID != "" {
		t.Errorf("zoom should reset on list change, zoomUID = %q", a.zoomUID)
	}
}
