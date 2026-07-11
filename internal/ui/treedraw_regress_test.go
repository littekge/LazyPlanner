package ui

import (
	"testing"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// TestDeepTreeDrawTerminates guards the local patch to vendored tview
// (treeview.go: advance `ancestor` before `continue`). Upstream v0.42.0 spins
// forever in TreeView.Draw when a node's ancestor indent reaches the pane width,
// freezing the app. Drawing a deep tree in a narrow pane must return promptly.
func TestDeepTreeDrawTerminates(t *testing.T) {
	tree := tview.NewTreeView()
	root := tview.NewTreeNode("root")
	tree.SetRoot(root).SetCurrentNode(root)
	node := root
	for i := 0; i < 20; i++ { // 20 levels: indent far exceeds the narrow pane below
		child := tview.NewTreeNode("n")
		node.AddChild(child)
		node.SetExpanded(true)
		node = child
	}

	screen := tcell.NewSimulationScreen("")
	if err := screen.Init(); err != nil {
		t.Fatalf("init simulation screen: %v", err)
	}
	defer screen.Fini()
	screen.SetSize(8, 24) // width 8 < deepest indent → triggers the buggy branch
	tree.SetRect(0, 0, 8, 24)

	done := make(chan struct{})
	go func() {
		tree.Draw(screen)
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("TreeView.Draw hung on a deep tree in a narrow pane — the vendored " +
			"tview patch (treeview.go ancestor-advance) was likely lost in a re-vendor")
	}
}
