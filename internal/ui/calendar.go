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
		// "Both" is recorded explicitly (not left empty) so the calendar's type
		// is *known* — creation gating treats an empty set as unknown/blocked.
		return []string{"VEVENT", "VTODO"}
	}
}

// guardWrite flashes a hint and returns false when calID is a read-only
// calendar (e.g. NextCloud's generated birthdays) — writes there are refused by
// the server and discarded on sync, so the UI blocks them at the source.
func (a *app) guardWrite(calID string) bool {
	if cal, ok := a.store.Calendar(calID); ok && cal.ReadOnly {
		a.flash("That calendar is read-only")
		return false
	}
	return true
}

// Component names for the two kinds of item LazyPlanner creates.
const (
	compEvent = "VEVENT"
	compTodo  = "VTODO"
)

// hasComponent reports whether a calendar's (known) supported component set
// includes want.
func hasComponent(cal store.Calendar, want string) bool {
	for _, c := range cal.Components {
		if strings.EqualFold(c, want) {
			return true
		}
	}
	return false
}

// guardComponent refuses creation of the wrong kind of item for a calendar's
// type, keeping events on event calendars and tasks on task lists (a "both"
// calendar allows either). The component set must be *known*: an empty set means
// the type is unconfirmed (a foreign vdir, or not yet synced), and creation is
// blocked until a sync settles it — the owner's choice over guessing from
// contents.
func (a *app) guardComponent(calID, want string) bool {
	cal, ok := a.store.Calendar(calID)
	if !ok {
		a.flash("Calendar not found")
		return false
	}
	if len(cal.Components) == 0 {
		if a.forceCreate {
			return true // manual override (i!…) for an unconfirmed-type calendar
		}
		a.flash("\"" + cal.DisplayName + "\": unknown type — sync it first (i! to force)")
		return false
	}
	if hasComponent(cal, want) {
		return true
	}
	// A known but wrong type is a genuine mismatch — force does not override it.
	if want == compEvent {
		a.flash("\"" + cal.DisplayName + "\" is a task list — can't add events")
	} else {
		a.flash("\"" + cal.DisplayName + "\" is an event calendar — can't add tasks")
	}
	return false
}

// createCollection (ac/al) opens a form to create a calendar or task list
// locally, offline-first: the collection appears immediately and the server
// MKCALENDAR happens on the next sync. defaultType preselects the Type dropdown
// (index into collectionTypes: 0 event calendar, 1 task list).
func (a *app) createCollection(defaultType int) {
	f := newCaretForm()
	nameField := f.addInput("Name", "", 0)
	typeField := f.addDropDown("Type", collectionTypes, defaultType)
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
	if cal.ReadOnly {
		a.flash("That calendar is read-only and can't be deleted")
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
