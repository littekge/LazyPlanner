package ui

import (
	"sort"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"

	"github.com/littekge/LazyPlanner/internal/model"
)

// The keyboard interface is vim-flavored: panel focus (c/t/a) and toggles are
// single keys, while multi-action groups live under a prefix pressed as a short
// chord: `i` create (it/iT task, ie/iE event, is/iS subtask, ic calendar, il
// list — i for "insert"), `g` go (gg top, gt today, gd go-to-date), and `z` fold
// (zR expand-all, zM collapse-all, za toggle). A which-key hint pops up after the
// prefix; the next key completes the chord and Esc cancels.

// chordEntry is one continuation under a prefix, used for dispatch and the
// which-key hint.
type chordEntry struct {
	key   rune
	label string
	run   func(a *app)
}

// chords maps each prefix to its continuations. Kept as data so the which-key
// popup and the help screen render from the same source as the dispatcher.
var chords = map[rune][]chordEntry{
	'i': {
		{'t', "task", (*app).addTaskQuick},
		{'T', "task (form)", (*app).addTaskFull},
		{'e', "event", (*app).addEventQuick},
		{'E', "event (form)", (*app).addEventFull},
		{'s', "subtask", (*app).addSubtaskQuick},
		{'S', "subtask (form)", (*app).addSubtaskFull},
		{'c', "calendar", func(a *app) { a.createCollection(0) }},
		{'l', "list", func(a *app) { a.createCollection(1) }},
	},
	'g': {
		{'g', "top", (*app).gotoTop},
		{'t', "today", (*app).gotoToday},
		{'d', "go to date", func(a *app) { a.openCommandLine("goto ") }},
	},
	'z': {
		{'R', "expand all", func(a *app) { a.setFoldAll(true) }},
		{'M', "collapse all", func(a *app) { a.setFoldAll(false) }},
		{'a', "toggle fold", (*app).toggleFold},
	},
	's': {
		{'p', "priority", (*app).setPriorityPrompt},
		{'d', "due date", (*app).setDuePrompt},
	},
}

// prefixLabel names each prefix for the which-key title.
var prefixLabel = map[rune]string{'i': "new", 'g': "go", 'z': "fold", 's': "set"}

// startPrefix enters a chord prefix and shows its which-key hint.
func (a *app) startPrefix(p rune) {
	if _, ok := chords[p]; !ok {
		return
	}
	a.pendingPrefix = p
	a.showWhichKey(p)
}

// clearPrefix leaves chord mode and removes the which-key hint.
func (a *app) clearPrefix() {
	if a.pendingPrefix == 0 {
		return
	}
	a.pendingPrefix = 0
	a.root.RemovePage(pageWhichKey)
}

// resolvePrefix completes the pending chord with ev, or cancels on Esc / an
// unknown continuation.
func (a *app) resolvePrefix(ev *tcell.EventKey) {
	p := a.pendingPrefix
	a.clearPrefix()
	if ev.Key() != tcell.KeyRune {
		return // Esc (or any non-rune) cancels
	}
	for _, e := range chords[p] {
		if e.key == ev.Rune() {
			e.run(a)
			a.echo(string(p) + string(e.key) + " " + e.label)
			return
		}
	}
	a.flash("no action for " + string(p) + string(ev.Rune()))
}

// deleteContextual deletes the calendar/list when an overview list is focused,
// otherwise the selected item — so a single `d` covers both.
func (a *app) deleteContextual() {
	switch a.tv.GetFocus() {
	case a.calendars, a.tasklists:
		a.deleteCollection()
	default:
		a.deleteSelected()
	}
}

// echo writes the last executed action, in command form, to the status bar's
// middle "command view" (lazygit-style).
func (a *app) echo(cmd string) { a.statusMid.SetText(cmd) }

// maxCount bounds an accumulated vim count so a fat-fingered "999999j" can't spin.
const maxCount = 999

// isMotion reports whether ev is a cursor-movement key (hjkl or an arrow) that a
// count prefix should repeat.
func isMotion(ev *tcell.EventKey) bool {
	switch ev.Key() {
	case tcell.KeyUp, tcell.KeyDown, tcell.KeyLeft, tcell.KeyRight:
		return true
	case tcell.KeyRune:
		switch ev.Rune() {
		case 'h', 'j', 'k', 'l':
			return true
		}
	}
	return false
}

// repeatKey feeds ev to the focused primitive n times. Counts and gg/G reuse the
// widgets' own navigation (tview's List/TreeView already handle arrows/Home/End),
// so movement stays consistent with a single keypress.
func (a *app) repeatKey(ev *tcell.EventKey, n int) {
	p := a.tv.GetFocus()
	if p == nil {
		return
	}
	handler := p.InputHandler()
	if handler == nil {
		return
	}
	setFocus := func(x tview.Primitive) { a.setFocus(x) }
	for i := 0; i < n; i++ {
		handler(ev, setFocus)
	}
}

// gotoTop moves the focused list/tree to its first item (vim gg). Home works for
// both tview.List and tview.TreeView.
func (a *app) gotoTop() {
	a.repeatKey(tcell.NewEventKey(tcell.KeyHome, 0, tcell.ModNone), 1)
}

// gotoToday re-anchors the calendar on today (gt), switching to Calendar mode
// first if needed — "go to today" implies the calendar.
func (a *app) gotoToday() {
	a.anchor = model.DayStart(a.now)
	if a.mode != modeCalendar {
		a.setMode(modeCalendar)
		return
	}
	a.buildCenterCalendar()
	a.refocusCalendar()
	a.updateStatus()
}

// gotoBottom moves to the last item (G), or — with a count — to the count-th item
// of a list (vim NG).
func (a *app) gotoBottom(count int) {
	if count > 0 {
		if lst, ok := a.tv.GetFocus().(*tview.List); ok {
			idx := count - 1
			if last := lst.GetItemCount() - 1; idx > last {
				idx = last
			}
			if idx < 0 {
				idx = 0
			}
			lst.SetCurrentItem(idx)
			return
		}
	}
	a.repeatKey(tcell.NewEventKey(tcell.KeyEnd, 0, tcell.ModNone), 1)
}

// setFoldAll expands or collapses every folder in the task tree at once (zR/zM),
// keeping each folder's ▸/▾ disclosure marker in sync.
func (a *app) setFoldAll(expanded bool) {
	root := a.tree.GetRoot()
	if root == nil {
		return
	}
	for _, child := range root.GetChildren() {
		a.setFoldRec(child, expanded)
	}
}

func (a *app) setFoldRec(node *tview.TreeNode, expanded bool) {
	if len(node.GetChildren()) > 0 {
		node.SetExpanded(expanded)
		if t, ok := node.GetReference().(*model.Todo); ok {
			node.SetText(a.nodeLabel(t, expanded))
		}
	}
	for _, c := range node.GetChildren() {
		a.setFoldRec(c, expanded)
	}
}

// toggleFold flips the fold state of the current tree node (za).
func (a *app) toggleFold() {
	node := a.tree.GetCurrentNode()
	if node == nil || len(node.GetChildren()) == 0 {
		return
	}
	expanded := !node.IsExpanded()
	node.SetExpanded(expanded)
	if t, ok := node.GetReference().(*model.Todo); ok {
		node.SetText(a.nodeLabel(t, expanded))
	}
}

// resizeLeft grows/shrinks the left overview column by delta (clamped) and
// persists the new width. It is a no-op while the column is collapsed.
func (a *app) resizeLeft(delta int) {
	if a.accordion || a.leftCol == nil {
		return
	}
	w := clampLeftWidth(a.leftWidth + delta)
	if w == a.leftWidth {
		return
	}
	a.leftWidth = w
	a.body.ResizeItem(a.leftCol, a.leftWidth, 0)
	a.persistState()
}

// persistState saves the remembered UI prefs (pane width + hidden calendars) via
// the callback wired from main. No-op when persistence is disabled.
func (a *app) persistState() {
	if a.saveState == nil {
		return
	}
	hidden := make([]string, 0, len(a.hidden))
	for id, on := range a.hidden {
		if on {
			hidden = append(hidden, id)
		}
	}
	sort.Strings(hidden) // stable file output
	a.saveState(a.leftWidth, hidden)
}

// toggleCalendarVisibility hides or shows the highlighted calendar's items on the
// calendar and agenda views (Space in Calendar mode). The choice is remembered in
// the state file; the underlying data and server sync are untouched.
func (a *app) toggleCalendarVisibility() {
	id := a.selectedCalendarID()
	if id == "" {
		return
	}
	if a.hidden[id] {
		delete(a.hidden, id)
	} else {
		a.hidden[id] = true
	}
	a.persistState()
	a.buildCalendars()
	a.buildAgendaLeft()
	a.reloadCurrent()
}

// setAccordion collapses (on) or restores (off) the left overview column so the
// Main view can fill the width — the lazygit +/- idiom. Collapsing moves focus
// into the center so a hidden pane isn't focused. Not available in Agenda mode,
// whose center navigation is driven by the (left) agenda list.
func (a *app) setAccordion(on bool) {
	if a.leftCol == nil {
		return
	}
	if on && a.mode == modeAgenda {
		a.flash("Expand isn't available in Agenda")
		return
	}
	a.accordion = on
	if on {
		a.body.ResizeItem(a.leftCol, 0, 0)
		a.setFocus(a.mainPrimitive())
	} else {
		a.body.ResizeItem(a.leftCol, a.leftWidth, 0)
		a.setFocus(a.focusForMode())
	}
}

// mainPrimitive is the focusable center widget for the current mode (used when
// the overview is collapsed).
func (a *app) mainPrimitive() tview.Primitive {
	if a.mode == modeTasks {
		return a.tree
	}
	return a.calendarPrimitive()
}

// showWhichKey draws a transient hint listing a prefix's continuations. It is a
// non-focused overlay; the next keystroke is intercepted by globalKeys (which
// checks pendingPrefix before anything else), so the popup never needs focus.
func (a *app) showWhichKey(p rune) {
	entries := chords[p]
	line := "[::b]" + prefixLabel[p] + ":[::-]  "
	for _, e := range entries {
		line += "[yellow]" + string(e.key) + "[-] " + e.label + "   "
	}
	if p == 'i' {
		line += "  [gray](Shift = full form · Esc cancels)[-]"
	} else {
		line += "  [gray](Esc cancels)[-]"
	}

	tv := tview.NewTextView().SetDynamicColors(true).SetText(" " + line)
	tv.SetBackgroundColor(tcell.ColorDefault)
	tv.SetBorder(true).SetBorderColor(accentColor)
	tv.SetTitle(" which-key ").SetTitleColor(accentColor)

	a.root.AddPage(pageWhichKey, whichKeyWrap(tv), true, true)
}

// whichKeyWrap pins the hint to the bottom of the screen, full width.
func whichKeyWrap(p tview.Primitive) tview.Primitive {
	return tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(nil, 0, 1, false).
		AddItem(p, 3, 0, false)
}
