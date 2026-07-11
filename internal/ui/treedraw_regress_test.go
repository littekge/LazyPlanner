package ui

import (
	"fmt"
	"testing"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// TestDeepTreeDrawTerminates guards that the task tree draws without hanging on a
// deep subtask chain in a narrow pane. Upstream tview v0.42.0 has a TreeView.Draw
// infinite loop when a node's ancestor indent reaches the pane width; we avoid it
// by disabling branch graphics (a.tree.SetGraphics(false)). If that call is
// removed (or graphics get re-enabled), this draw hangs and the watchdog fires.
func TestDeepTreeDrawTerminates(t *testing.T) {
	a := newRootedTestApp(t, time.Date(2026, 7, 5, 9, 0, 0, 0, time.UTC))
	a.setMode(modeTasks)
	cal := a.selectedTasklistID()

	parent := ""
	for i := 0; i < 20; i++ { // 20 levels: indent far exceeds the narrow pane below
		summary := fmt.Sprintf("Deep%02d", i)
		a.createTask(cal, parent, summary)
		parent = todoBySummary(a.store, summary).UID
	}
	a.buildTree()
	expandAllNodes(a.tree.GetRoot()) // every level must be visible to be drawn

	screen := tcell.NewSimulationScreen("")
	if err := screen.Init(); err != nil {
		t.Fatalf("init simulation screen: %v", err)
	}
	defer screen.Fini()
	screen.SetSize(8, 24) // width 8 < deepest indent → would trip the buggy branch
	a.tree.SetRect(0, 0, 8, 24)

	done := make(chan struct{})
	go func() {
		a.tree.Draw(screen)
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("task tree Draw hung on a deep tree in a narrow pane — a.tree.SetGraphics(false) " +
			"was likely dropped, re-exposing the upstream tview TreeView.Draw infinite loop")
	}
}

func expandAllNodes(node *tview.TreeNode) {
	if node == nil {
		return
	}
	node.SetExpanded(true)
	for _, c := range node.GetChildren() {
		expandAllNodes(c)
	}
}
