package ui

import (
	"context"
	"testing"
	"time"

	"github.com/gdamore/tcell/v2"

	"github.com/littekge/LazyPlanner/internal/store"
)

// TestTreeNodeAtYPastLastNode closes a Pass 18 canary hole: treeNodeAtY guards
// idx >= len(visible) before indexing. A mutation to idx > len would let a click
// exactly one row past the last node index into visible[len] and panic the TUI
// (e.g. a double-click just below the last task). This clicks that boundary row and
// asserts no panic and no selection.
func TestTreeNodeAtYPastLastNode(t *testing.T) {
	a := newRootedTestApp(t, time.Date(2026, 7, 5, 9, 0, 0, 0, time.UTC))
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
	a.createTask("zztasks", "", "Only task")
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

	_, top, _, height := a.tree.GetInnerRect()

	// Any treeNodeAtY call below (the scan or the boundary probe) that indexes out
	// of range is the mutation firing; report it as a clean failure, not a raw panic.
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("treeNodeAtY panicked at the idx==len boundary (a click just past the last node): %v", r)
		}
	}()

	// Find the last row that maps to a real node, then probe exactly one past it —
	// that row's idx equals len(visible), the boundary the guard protects.
	lastReal := top - 1
	for y := top; y < top+height; y++ {
		if a.treeNodeAtY(y) != nil {
			lastReal = y
		}
	}
	if lastReal < top {
		t.Fatal("no tree rows resolved to a node; test setup is wrong")
	}
	if n := a.treeNodeAtY(lastReal + 1); n != nil {
		t.Errorf("treeNodeAtY one row past the last node returned a node, want nil")
	}
}
