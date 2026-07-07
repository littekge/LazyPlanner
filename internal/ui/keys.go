package ui

import (
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// The keyboard interface is vim-flavored: navigation, panel focus, and toggles
// are single keys, while create actions are grouped under the `a` prefix pressed
// as a short chord (at/aT task, ae/aE event, as/aS subtask, ac calendar, al
// list). A which-key hint pops up after the prefix; the next key completes the
// chord and Esc cancels. This is the step-10 replacement for the interim
// standalone create keys.

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
	'a': {
		{'t', "task", (*app).addTaskQuick},
		{'T', "task (form)", (*app).addTaskFull},
		{'e', "event", (*app).addEventQuick},
		{'E', "event (form)", (*app).addEventFull},
		{'s', "subtask", (*app).addSubtaskQuick},
		{'S', "subtask (form)", (*app).addSubtaskFull},
		{'c', "calendar", func(a *app) { a.createCollection(0) }},
		{'l', "list", func(a *app) { a.createCollection(1) }},
	},
}

// prefixLabel names each prefix for the which-key title.
var prefixLabel = map[rune]string{'a': "add"}

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

// showWhichKey draws a transient hint listing a prefix's continuations. It is a
// non-focused overlay; the next keystroke is intercepted by globalKeys (which
// checks pendingPrefix before anything else), so the popup never needs focus.
func (a *app) showWhichKey(p rune) {
	entries := chords[p]
	line := "[::b]" + prefixLabel[p] + ":[::-]  "
	for _, e := range entries {
		line += "[yellow]" + string(e.key) + "[-] " + e.label + "   "
	}
	line += "  [gray](Shift = full form · Esc cancels)[-]"

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
