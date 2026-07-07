package ui

import (
	"context"
	"fmt"
	"strings"

	"github.com/littekge/LazyPlanner/internal/store"
)

// collectionTypes are the choices in the create-calendar form; the index maps to
// componentsForType.
var collectionTypes = []string{"Event calendar", "Task list", "Both"}

func componentsForType(label string) []string {
	switch label {
	case "Event calendar":
		return []string{"VEVENT"}
	case "Task list":
		return []string{"VTODO"}
	default:
		return nil // both (the server default)
	}
}

// createCollection (c) opens a form to create a calendar or task list locally.
// It is created offline-first: the collection appears immediately and the server
// MKCALENDAR happens on the next sync.
func (a *app) createCollection() {
	f := newCaretForm()
	nameField := f.addInput("Name", "", 0)
	// Default the type to match the active pane (a task list in Tasks mode).
	def := 0
	if a.mode == modeTasks {
		def = 1
	}
	typeField := f.addDropDown("Type", collectionTypes, def)
	f.stylePopup()

	f.AddButton("Create", func() {
		name := strings.TrimSpace(nameField.GetText())
		if name == "" {
			a.flash("A name is required")
			return
		}
		_, typeLabel := typeField.GetCurrentOption()
		id := store.SafeName(name)
		err := a.store.CreateCalendarLocal(context.Background(), id, store.CalendarMeta{DisplayName: name}, componentsForType(typeLabel))
		if err != nil {
			a.flash("Create failed: " + err.Error())
			return
		}
		a.refresh("")
		a.closeModal(pageForm)
		a.flash(fmt.Sprintf("Created %q — syncs on next sync", name))
	})
	f.AddButton("Cancel", func() { a.closeModal(pageForm) })
	f.SetCancelFunc(func() { a.closeModal(pageForm) })
	f.SetBorder(true).SetTitle(" New calendar / list ")
	a.openModal(pageForm, f, 62, 11)
}

// deleteCollection (D) deletes the highlighted calendar (Calendars) or task list
// (Tasks), with a confirm. The deletion is applied on the server on the next
// sync (or immediately locally if the calendar was never pushed).
func (a *app) deleteCollection() {
	var id string
	switch a.mode {
	case modeCalendar:
		id = a.selectedCalendarID()
	case modeTasks:
		id = a.selectedTasklistID()
	default:
		a.flash("Switch to Calendars (1) or Tasks (2) to delete a list")
		return
	}
	if id == "" {
		a.flash("No calendar selected")
		return
	}
	cal, ok := a.store.Calendar(id)
	if !ok {
		a.flash("Calendar not found")
		return
	}

	prompt := fmt.Sprintf("Delete calendar %q", cal.DisplayName)
	if n := len(cal.Resources); n > 0 {
		prompt += fmt.Sprintf(" and its %d item(s)", n)
	}
	prompt += "?\nThis also deletes it on the server on the next sync."

	a.confirm(prompt, func() {
		if err := a.store.MarkCalendarDeleted(context.Background(), id); err != nil {
			a.flash("Delete failed: " + err.Error())
			return
		}
		a.refresh("")
		a.flash(fmt.Sprintf("Deleted %q", cal.DisplayName))
	})
}
