package ui

import (
	"context"
	"fmt"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"

	"github.com/littekge/LazyPlanner/internal/model"
	"github.com/littekge/LazyPlanner/internal/store"
)

const (
	pageConflicts      = "conflicts"
	pageConflictChoose = "conflict-choose"
)

// showConflicts lists conflicted items (both sides kept by sync) and lets the
// user resolve each — keep the local edit or take the server version. It closes
// automatically once none remain.
func (a *app) showConflicts() {
	if len(a.store.Conflicts()) == 0 {
		a.flash("No conflicts to resolve")
		return
	}
	a.echo(":conflicts")

	list := tview.NewList().ShowSecondaryText(false)
	list.SetSelectedStyle(selectionStyle)
	list.SetBackgroundColor(tcell.ColorDefault)
	list.SetBorder(true).SetBorderColor(accentColor)
	list.SetTitle(" Conflicts — Enter to resolve · Esc to close ").SetTitleColor(accentColor)
	list.SetInputCapture(func(ev *tcell.EventKey) *tcell.EventKey {
		if ev.Key() == tcell.KeyEscape || ev.Rune() == 'q' {
			a.closeModal(pageConflicts)
			return nil
		}
		return modalMotionKey(ev)
	})

	a.populateConflicts(list)
	a.captureFocus()
	a.root.AddPage(pageConflicts, modalWrap(list, 74, 18), true, true)
	a.tv.SetFocus(list)
}

// populateConflicts (re)fills the conflicts list from the current store state.
func (a *app) populateConflicts(list *tview.List) {
	list.Clear()
	for _, c := range a.store.Conflicts() {
		cc := c // capture per iteration
		list.AddItem(a.conflictLabel(cc), "", 0, func() { a.chooseResolution(list, cc) })
	}
}

// chooseResolution asks how to resolve one conflict and applies it.
func (a *app) chooseResolution(list *tview.List, c store.Conflict) {
	modal := tview.NewModal().
		SetText(fmt.Sprintf("Resolve %q\nKeep your local edit, or take the server's version?", a.conflictSummary(c))).
		AddButtons([]string{"Keep local", "Keep server", "Cancel"}).
		SetDoneFunc(func(_ int, label string) {
			a.root.RemovePage(pageConflictChoose)
			a.tv.SetFocus(list)

			var err error
			switch label {
			case "Keep local":
				err = a.store.ResolveKeepLocal(context.Background(), c.CalID, c.Name)
			case "Keep server":
				err = a.store.ResolveKeepServer(context.Background(), c.CalID, c.Name)
			default:
				return
			}
			if err != nil {
				a.flash("Resolve failed: " + err.Error())
				return
			}
			a.refresh("")
			if len(a.store.Conflicts()) == 0 {
				a.closeModal(pageConflicts)
				a.flash("All conflicts resolved")
				return
			}
			a.populateConflicts(list)
		})
	styleModal(modal, " Resolve conflict ")

	a.root.AddPage(pageConflictChoose, modal, true, true)
	a.tv.SetFocus(modal)
}

// conflictLabel is the row shown in the conflicts list.
func (a *app) conflictLabel(c store.Conflict) string {
	cal, _ := a.store.Calendar(c.CalID)
	return tview.Escape(fmt.Sprintf("%s — %s", nonEmpty(cal.DisplayName, c.CalID), a.conflictSummary(c)))
}

// conflictSummary is the local item's title (falling back to the file name).
func (a *app) conflictSummary(c store.Conflict) string {
	cal, _ := a.store.Calendar(c.CalID)
	for _, r := range cal.Resources {
		if r.Name == c.Name {
			return firstSummary(r.Object)
		}
	}
	return c.Name
}

func firstSummary(obj *model.Parsed) string {
	if obj != nil {
		if len(obj.Events) > 0 {
			return nonEmpty(obj.Events[0].Summary, "(untitled)")
		}
		if len(obj.Todos) > 0 {
			return nonEmpty(obj.Todos[0].Summary, "(untitled)")
		}
	}
	return "(item)"
}
