package ui

import (
	"fmt"
	"strings"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"

	"github.com/littekge/LazyPlanner/internal/model"
)

const pageSearch = "search"

// Search is incremental: `/` opens an input, and the selection follows the first
// match as you type; Enter keeps the match (focus moves to the view), Esc cancels
// and restores the prior selection. `n` / `N` cycle matches afterwards. The search
// targets the current mode's collection — the task tree, the agenda list, or the
// calendars list.

// openSearch shows the `/` search input near the top of the screen.
func (a *app) openSearch() {
	a.searchRestore = a.currentSelectionRestore()

	in := tview.NewInputField().SetLabel("/")
	in.SetFieldBackgroundColor(tcell.ColorDefault)
	in.SetFieldTextColor(tcell.ColorDefault)
	in.SetLabelColor(accentColor)
	in.SetBackgroundColor(tcell.ColorDefault)
	in.SetBorder(true).SetBorderColor(accentColor)
	in.SetTitle(" search ").SetTitleColor(accentColor)

	// Incremental: move the selection to the first match on every keystroke. The
	// input keeps focus (runSearch only changes the selection, never the focus).
	in.SetChangedFunc(func(text string) { a.runSearch(text) })
	in.SetDoneFunc(func(key tcell.Key) {
		switch key {
		case tcell.KeyEnter:
			a.root.RemovePage(pageSearch)
			if a.searchQuery != "" {
				a.setFocus(a.searchWidget()) // land on the matched item
			} else {
				a.restoreFocus()
			}
		case tcell.KeyEscape:
			a.searchQuery = ""
			a.root.RemovePage(pageSearch)
			if a.searchRestore != nil {
				a.searchRestore()
			}
			a.restoreFocus()
		}
	})

	a.captureFocus()
	a.root.AddPage(pageSearch, topLineWrap(in), true, true)
	a.tv.SetFocus(in)
}

// runSearch selects the first item matching q (case-insensitive substring). It
// changes only the selection, not the focus, so the search input keeps focus
// while typing.
func (a *app) runSearch(q string) {
	a.searchQuery = q
	if strings.TrimSpace(q) == "" {
		return
	}
	labels, sel := a.searchItems()
	matches := matchIndices(labels, q)
	if len(matches) == 0 {
		a.flash("no match: " + q)
		return
	}
	a.searchIdx = 0
	sel(matches[0])
	a.flash(fmt.Sprintf("/%s  (1/%d)", q, len(matches)))
}

// searchNext moves to the next (dir=1) or previous (dir=-1) match. Matches are
// recomputed so the cycle survives edits between presses.
func (a *app) searchNext(dir int) {
	if a.searchQuery == "" {
		a.flash("no active search (/ to search)")
		return
	}
	labels, sel := a.searchItems()
	matches := matchIndices(labels, a.searchQuery)
	if len(matches) == 0 {
		a.flash("no match: " + a.searchQuery)
		return
	}
	a.searchIdx = (a.searchIdx + dir + len(matches)) % len(matches)
	sel(matches[a.searchIdx])
	a.setFocus(a.searchWidget())
	a.flash(fmt.Sprintf("/%s  (%d/%d)", a.searchQuery, a.searchIdx+1, len(matches)))
}

// searchWidget is the primitive that owns the current mode's searchable list.
func (a *app) searchWidget() tview.Primitive {
	switch a.mode {
	case modeTasks:
		return a.tree
	case modeAgenda:
		return a.agendaList
	default:
		return a.calendars
	}
}

// searchItems returns the labels of the current mode's collection plus a function
// that selects the item at a given index (selection only — no focus change).
func (a *app) searchItems() (labels []string, sel func(i int)) {
	switch a.mode {
	case modeTasks:
		var hits []treeHit
		collectTreeHits(a.tree.GetRoot(), nil, &hits)
		labels = make([]string, len(hits))
		for i, h := range hits {
			if t, ok := h.node.GetReference().(*model.Todo); ok {
				labels[i] = t.Summary
			}
		}
		sel = func(i int) {
			for _, anc := range hits[i].ancestors {
				anc.SetExpanded(true) // reveal a match inside a collapsed folder
			}
			a.tree.SetCurrentNode(hits[i].node)
		}
	case modeAgenda:
		n := a.agendaList.GetItemCount()
		labels = make([]string, n)
		for i := 0; i < n; i++ {
			labels[i], _ = a.agendaList.GetItemText(i)
		}
		sel = func(i int) { a.agendaList.SetCurrentItem(i) }
	default: // calendar: search calendar names
		n := a.calendars.GetItemCount()
		labels = make([]string, n)
		for i := 0; i < n; i++ {
			labels[i], _ = a.calendars.GetItemText(i)
		}
		sel = func(i int) { a.calendars.SetCurrentItem(i) }
	}
	return labels, sel
}

// currentSelectionRestore captures the current selection so Esc can put it back.
func (a *app) currentSelectionRestore() func() {
	switch a.mode {
	case modeTasks:
		n := a.tree.GetCurrentNode()
		return func() {
			if n != nil {
				a.tree.SetCurrentNode(n)
			}
		}
	case modeAgenda:
		i := a.agendaList.GetCurrentItem()
		return func() { a.agendaList.SetCurrentItem(i) }
	default:
		i := a.calendars.GetCurrentItem()
		return func() { a.calendars.SetCurrentItem(i) }
	}
}

// treeHit is a matchable task node plus the ancestors that must be expanded to
// reveal it.
type treeHit struct {
	node      *tview.TreeNode
	ancestors []*tview.TreeNode
}

// collectTreeHits walks the task tree in display order, collecting every node
// that carries a *model.Todo (the root and any label-only nodes are skipped).
func collectTreeHits(node *tview.TreeNode, ancestors []*tview.TreeNode, out *[]treeHit) {
	if node == nil {
		return
	}
	for _, c := range node.GetChildren() {
		if _, ok := c.GetReference().(*model.Todo); ok {
			anc := make([]*tview.TreeNode, len(ancestors))
			copy(anc, ancestors)
			*out = append(*out, treeHit{node: c, ancestors: anc})
		}
		childAnc := make([]*tview.TreeNode, len(ancestors)+1)
		copy(childAnc, ancestors)
		childAnc[len(ancestors)] = c
		collectTreeHits(c, childAnc, out)
	}
}

// matchIndices returns the indices of labels containing q (case-insensitive).
func matchIndices(labels []string, q string) []int {
	q = strings.ToLower(strings.TrimSpace(q))
	var out []int
	for i, l := range labels {
		if strings.Contains(strings.ToLower(l), q) {
			out = append(out, i)
		}
	}
	return out
}
