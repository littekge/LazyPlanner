package ui

import (
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// mouseCapture makes the mouse coherent with the mode model on top of tview's
// built-in click-to-select and wheel-scroll: clicking a left overview panel
// switches to that mode (so the center follows), and a double-click on the task
// tree or agenda opens the edit form for the item under the cursor. tview still
// receives the event afterward, so selection and scrolling keep working.
func (a *app) mouseCapture(ev *tcell.EventMouse, action tview.MouseAction) (*tcell.EventMouse, tview.MouseAction) {
	if ev == nil {
		return ev, action
	}
	// SELECT is keyboard-modal like grab: a click could switch panes or move
	// the context under the active range, so mouse input is inert until Esc.
	if a.selecting {
		return nil, action
	}
	// Don't reinterpret clicks while a modal/overlay is up — it owns the mouse.
	if a.modalOpen() {
		return ev, action
	}
	// Grab and the Ctrl-W resize sub-mode are modal flag-states with no overlay
	// page, so modalOpen() is false during them. Swallow the mouse entirely (not
	// just decline to reinterpret it), matching the keyboard gating in globalKeys:
	// otherwise a click could switch panes or open the edit form mid-grab/resize,
	// leaving two modal states coexisting and the mode state desynced.
	if a.grabbing || a.resizing {
		return nil, action
	}

	switch action {
	case tview.MouseLeftClick:
		x, y := ev.Position()
		switch {
		case a.calendars.InRect(x, y):
			if a.mode != modeCalendar {
				a.setMode(modeCalendar)
			}
		case a.tasklists.InRect(x, y):
			if a.mode != modeTasks {
				a.setMode(modeTasks)
			}
		case a.agendaList.InRect(x, y):
			if a.mode != modeAgenda {
				a.setMode(modeAgenda)
			}
		}
	case tview.MouseLeftDoubleClick:
		x, y := ev.Position()
		switch {
		case a.tree.InRect(x, y):
			// Select the node actually under the cursor before editing. The two
			// clicks of a double-click can land on different rows (the pointer moved
			// within the interval), and editSelected reads the *current* selection —
			// so without this it edits the row the first click selected, not the row
			// the user double-clicked. SetCurrentNode only sets the field (no
			// expand-toggle SetSelectedFunc fires), so this is a pure re-target.
			if node := a.treeNodeAtY(y); node != nil {
				a.tree.SetCurrentNode(node)
			}
			a.editSelected()
		case a.agenda.InRect(x, y):
			// The center agenda board has no click-to-select mapping (single clicks
			// there don't move the agenda selection either), so a double-click edits
			// the current agenda selection. Position-precise board editing would need
			// board-level hit-testing — see COVERAGE.md.
			a.editSelected()
		}
	}
	return ev, action
}

// treeNodeAtY maps a screen row to the task-tree node drawn there, mirroring
// TreeView's own mouse math (visible index = y − innerTop + scrollOffset over the
// pre-order walk of expanded nodes, root included). Returns nil when y is outside
// the pane or past the last node. Public-API only — it does not reach into
// TreeView's unexported node slice.
func (a *app) treeNodeAtY(y int) *tview.TreeNode {
	root := a.tree.GetRoot()
	if root == nil {
		return nil
	}
	_, top, _, height := a.tree.GetInnerRect()
	if y < top || y >= top+height {
		return nil
	}
	idx := y - top + a.tree.GetScrollOffset()
	if idx < 0 {
		return nil
	}
	var visible []*tview.TreeNode
	var walk func(n *tview.TreeNode)
	walk = func(n *tview.TreeNode) {
		visible = append(visible, n)
		if n.IsExpanded() {
			for _, c := range n.GetChildren() {
				walk(c)
			}
		}
	}
	walk(root)
	if idx >= len(visible) {
		return nil
	}
	return visible[idx]
}
