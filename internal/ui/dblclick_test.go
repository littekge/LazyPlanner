package ui

import (
	"testing"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/littekge/LazyPlanner/internal/model"
	"github.com/rivo/tview"
)

// TestDoubleClickEditsRowUnderCursor guards the fix: a double-click on row B (a
// different row than the currently selected row A) opens the edit form for B, the
// row under the cursor. Previously mouseCapture's double-click branch ran before
// tview moved the selection and called editSelected() on the stale selection;
// treeNodeAtY now re-targets the current node to the clicked row first.
func TestDoubleClickEditsRowUnderCursor(t *testing.T) {
	now := time.Date(2026, 7, 5, 9, 0, 0, 0, time.UTC)
	a := newRootedTestApp(t, now)
	a.setMode(modeTasks)

	listID := a.selectedTasklistID()
	if listID == "" {
		t.Fatal("no task list selected")
	}
	a.createTask(listID, "", "Alpha")
	a.createTask(listID, "", "Beta")
	a.buildTree()

	// Draw the full layout so panes get real rects for InRect/position math.
	screen := tcell.NewSimulationScreen("")
	if err := screen.Init(); err != nil {
		t.Fatal(err)
	}
	defer screen.Fini()
	screen.SetSize(120, 40)
	a.root.SetRect(0, 0, 120, 40)
	a.root.Draw(screen)

	nodes := visibleTreeNodes(a.tree.GetRoot())
	if len(nodes) < 2 {
		t.Fatalf("need >=2 visible tree nodes, have %d", len(nodes))
	}
	rowA, rowB := nodes[0], nodes[1]
	todoA, okA := rowA.GetReference().(*model.Todo)
	todoB, okB := rowB.GetReference().(*model.Todo)
	if !okA || !okB {
		t.Fatalf("tree nodes are not *model.Todo (A=%v B=%v)", okA, okB)
	}
	sumA, sumB := todoA.Summary, todoB.Summary
	t.Logf("rowA=%q rowB=%q", sumA, sumB)

	// User has row A selected.
	a.tree.SetCurrentNode(rowA)
	a.setFocus(a.tree)

	// Inner content of the (bordered) tree: root at row 0, node i at row i+1.
	ix, iy, _, _ := a.tree.GetInnerRect()
	betaY := iy + 2 // root(0), rowA(1), rowB(2)
	t.Logf("tree inner x=%d y=%d; double-click at (%d,%d)", ix, iy, ix+1, betaY)

	// Double-click on row B's position.
	a.mouseCapture(tcell.NewEventMouse(ix+1, betaY, tcell.Button1, tcell.ModNone), tview.MouseLeftDoubleClick)

	// The opened form's first field (Summary) should reflect the clicked row (B).
	in, ok := a.tv.GetFocus().(*tview.InputField)
	if !ok {
		t.Fatalf("no edit form opened; focus is %T", a.tv.GetFocus())
	}
	got := in.GetText()
	t.Logf("opened form Summary = %q (want %q, the clicked row B)", got, sumB)
	if got != sumB {
		t.Errorf("double-click on row B (%q) opened the edit form for %q instead", sumB, got)
	}
}

// TestAgendaBoardDoubleClickIgnoredOutsideAgendaMode: the agenda board's Box
// keeps its last-drawn rect after the app leaves Agenda mode — tview.Pages
// never resizes/redraws a hidden page (Pages.Draw skips !Visible pages) — so a
// stale rect can still InRect-match. The single-click case already guards on
// a.mode == modeAgenda; the double-click case must too, or a click that lands
// where the board used to be opens editSelected from the wrong mode.
func TestAgendaBoardDoubleClickIgnoredOutsideAgendaMode(t *testing.T) {
	a, _ := drawnAgendaApp(t, 3) // draws once while in Agenda mode, giving a.agenda a real rect
	b := a.agenda

	_, y, _, _ := b.GetInnerRect()
	contentTop := y + 2
	_, starts, _ := b.layoutBlocks()
	boardX, _, _, _ := b.GetRect()
	clickCol := boardX + 1
	clickRow := contentTop + starts[1] - b.scroll

	// Leave Agenda mode without redrawing, so a.agenda's rect is now stale.
	a.setMode(modeTasks)

	a.mouseCapture(tcell.NewEventMouse(clickCol, clickRow, tcell.Button1, 0), tview.MouseLeftDoubleClick)

	if a.modalOpen() {
		t.Error("double-click on the stale board rect opened the edit form outside Agenda mode")
	}
	if got := a.agendaList.GetCurrentItem(); got != 0 {
		t.Errorf("stale double-click moved the agenda selection to %d, want unchanged (0)", got)
	}
}

// TestTreeDoubleClickIgnoredOutsideTasksMode: the same staleness class as the
// agenda-board case above applies to the tree — it too is a hidden
// tview.Pages page outside Tasks mode, so its Box rect can equally go stale.
func TestTreeDoubleClickIgnoredOutsideTasksMode(t *testing.T) {
	now := time.Date(2026, 7, 5, 9, 0, 0, 0, time.UTC)
	a := newRootedTestApp(t, now) // starts in Tasks mode
	listID := a.selectedTasklistID()
	a.createTask(listID, "", "Alpha")
	a.buildTree()

	screen := tcell.NewSimulationScreen("")
	if err := screen.Init(); err != nil {
		t.Fatal(err)
	}
	defer screen.Fini()
	screen.SetSize(120, 40)
	a.root.SetRect(0, 0, 120, 40)
	a.root.Draw(screen) // draws once while in Tasks mode, giving a.tree a real rect

	ix, iy, _, _ := a.tree.GetInnerRect()
	clickY := iy + 1 // root node's row

	// Leave Tasks mode without redrawing, so a.tree's rect is now stale.
	a.setMode(modeCalendar)

	a.mouseCapture(tcell.NewEventMouse(ix+1, clickY, tcell.Button1, tcell.ModNone), tview.MouseLeftDoubleClick)

	if a.modalOpen() {
		t.Error("double-click on the stale tree rect opened the edit form outside Tasks mode")
	}
}
