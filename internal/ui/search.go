package ui

import (
	"fmt"
	"slices"
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
			if !blankQuery(a.searchQuery) {
				// Land on the matched item, discarding the pre-search selection —
				// but still pop the captured focus so the stack stays balanced.
				if n := len(a.focusStack); n > 0 {
					a.focusStack = a.focusStack[:n-1]
				}
				a.setFocus(a.searchWidget())
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
	if blankQuery(q) {
		// Store "" rather than the raw text: matchIndices trims before comparing, so
		// a whitespace-only query left active would match every row on the next n.
		a.searchQuery = ""
		return
	}
	a.searchQuery = q
	labels, sel, _ := a.searchItems()
	matches := matchIndices(labels, q)
	if len(matches) == 0 {
		// Escaped: statusLeft has dynamic colors on, so a query containing a bracket
		// run ("[red]") would be eaten as a style tag instead of echoed back.
		a.flash("no match: " + tview.Escape(q))
		return
	}
	a.searchIdx = 0
	sel(matches[0])
	a.flash(fmt.Sprintf("/%s  (1/%d)", tview.Escape(q), len(matches)))
}

// searchNext moves to the next (dir=1) or previous (dir=-1) match. Matches are
// recomputed on every press, and so is the position to step from: it is derived
// from where the selection actually *is* (searchItems' cur, found by the task's
// UID in Tasks mode — the same identity idiom sync-highlight preservation uses).
// A remembered ordinal is only the fallback: an item added, deleted or synced
// away between presses shifts every index, which used to make n stall or skip.
func (a *app) searchNext(dir int) {
	if blankQuery(a.searchQuery) {
		a.flash("no active search (/ to search)")
		return
	}
	labels, sel, cur := a.searchItems()
	matches := matchIndices(labels, a.searchQuery)
	if len(matches) == 0 {
		a.flash("no match: " + tview.Escape(a.searchQuery))
		return
	}
	pos := slices.Index(matches, cur)
	if pos < 0 {
		// The selection is on no match at all (it was deleted, or the user moved
		// off it): resume from the nearest surviving ordinal rather than restart.
		pos = a.searchIdx
		if pos >= len(matches) {
			pos = len(matches) - 1
		}
		if pos < 0 {
			pos = 0
		}
	}
	a.searchIdx = (pos + dir + len(matches)) % len(matches)
	sel(matches[a.searchIdx])
	a.setFocus(a.searchWidget())
	a.flash(fmt.Sprintf("/%s  (%d/%d)", tview.Escape(a.searchQuery), a.searchIdx+1, len(matches)))
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

// searchItems returns the labels of the current mode's collection, a function
// that selects the item at a given index (selection only — no focus change), and
// cur: the index the selection is currently on, or -1. cur is re-derived from the
// live collection on every call, which is what lets n/N keep their place across a
// rebuild instead of trusting an ordinal that the rebuild invalidated.
func (a *app) searchItems() (labels []string, sel func(i int), cur int) {
	cur = -1
	switch a.mode {
	case modeTasks:
		var hits []treeHit
		collectTreeHits(a.tree.GetRoot(), nil, &hits)
		labels = make([]string, len(hits))
		// The tree is rebuilt from scratch on every reload, so the highlighted row is
		// located by UID rather than by node pointer or position.
		uid := a.currentTreeUID()
		for i, h := range hits {
			if t, ok := h.node.GetReference().(*model.Todo); ok {
				labels[i] = t.Summary
				if uid != "" && t.UID == uid {
					cur = i
				}
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
		cur = a.agendaList.GetCurrentItem()
		sel = func(i int) { a.agendaList.SetCurrentItem(i) }
	default: // calendar: search calendar names
		n := a.calendars.GetItemCount()
		labels = make([]string, n)
		for i := 0; i < n; i++ {
			labels[i], _ = a.calendars.GetItemText(i)
		}
		cur = a.calendars.GetCurrentItem()
		sel = func(i int) { a.calendars.SetCurrentItem(i) }
	}
	if cur >= len(labels) {
		cur = -1
	}
	return labels, sel, cur
}

// currentSelectionRestore captures the current selection so Esc can put it back.
func (a *app) currentSelectionRestore() func() {
	switch a.mode {
	case modeTasks:
		n := a.tree.GetCurrentNode()
		// Snapshot every node's expansion too, so Esc re-collapses folders that
		// incremental search auto-expanded to reveal a match.
		type nodeExp struct {
			node *tview.TreeNode
			open bool
		}
		var snap []nodeExp
		var walk func(*tview.TreeNode)
		walk = func(nd *tview.TreeNode) {
			if nd == nil {
				return
			}
			snap = append(snap, nodeExp{nd, nd.IsExpanded()})
			for _, c := range nd.GetChildren() {
				walk(c)
			}
		}
		walk(a.tree.GetRoot())
		return func() {
			for _, e := range snap {
				e.node.SetExpanded(e.open)
			}
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

// blankQuery reports whether a query is effectively empty — the single place that
// decides it, so the callers that gate on "is a search active" cannot drift apart
// from matchIndices, which trims before comparing (a whitespace-only query would
// otherwise match every row).
func blankQuery(q string) bool { return strings.TrimSpace(q) == "" }

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
