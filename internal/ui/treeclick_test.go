package ui

import (
	"context"
	"testing"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"

	"github.com/littekge/LazyPlanner/internal/store"
)

// TestTreeClickTogglesFolder: a left-click on a folder node in the task tree
// expands/collapses it (via the tree's SetSelectedFunc, which tview fires on
// click). Locks in the file-explorer click behavior.
func TestTreeClickTogglesFolder(t *testing.T) {
	a := newRootedTestApp(t, time.Date(2026, 7, 5, 9, 0, 0, 0, time.UTC))
	// A fresh list holding just one folder, so it lands on the first tree row.
	if err := a.store.CreateCalendarLocal(context.Background(), "zztasks", store.CalendarMeta{DisplayName: "ZZ"}, []string{"VTODO"}); err != nil {
		t.Fatal(err)
	}
	a.reload()
	a.setMode(modeTasks)
	for i, id := range a.tasklistIDs {
		if id == "zztasks" {
			a.tasklists.SetCurrentItem(i)
		}
	}
	a.createTask("zztasks", "", "Folder")
	p := todoBySummary(a.store, "Folder")
	a.createTask("zztasks", p.UID, "child")
	a.buildTreeForList("zztasks")

	// Draw so the tree gets real rects/offsets.
	screen := tcell.NewSimulationScreen("")
	if err := screen.Init(); err != nil {
		t.Fatal(err)
	}
	defer screen.Fini()
	screen.SetSize(120, 40)
	a.root.SetRect(0, 0, 120, 40)
	a.root.Draw(screen)

	folder := findTreeNode(a.tree.GetRoot(), p.UID)
	if folder == nil {
		t.Fatal("folder node not found in the tree")
	}
	before := folder.IsExpanded()

	ix, iy, _, _ := a.tree.GetInnerRect()
	h := a.tree.MouseHandler()
	// Row iy+1 is the first (and only) top-level node — the folder.
	h(tview.MouseLeftClick, tcell.NewEventMouse(ix+4, iy+1, tcell.Button1, 0), func(tview.Primitive) {})

	if folder.IsExpanded() == before {
		t.Errorf("clicking the folder did not toggle its expansion (still %v)", before)
	}
}
