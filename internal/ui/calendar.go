package ui

import (
	"context"
	"fmt"
	"strings"

	"github.com/rivo/tview"

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

// showCalendarForm opens the create/edit form for a calendar or task list,
// offline-first (the collection appears immediately; the server MKCALENDAR /
// PROPPATCH happens on the next sync). editID is "" to create — the Type dropdown
// shows and defaultType preselects it (0 event calendar, 1 task list) — or names
// an existing calendar to edit, where the type is fixed and only Name and Color
// change. Color is a hex field with a "Pick color…" button that opens the swatch
// grid; the chosen color is set at creation (so a new calendar is colored from
// the start, carried in its MKCALENDAR — not left default).
func (a *app) showCalendarForm(editID string, defaultType int) {
	editing := editID != ""
	var cur store.Calendar
	if editing {
		c, ok := a.store.Calendar(editID)
		if !ok {
			a.flash("Calendar not found")
			return
		}
		if !a.guardWrite(editID) {
			return
		}
		cur = c
	}

	f := newCaretForm()
	nameField := f.addInput("Name", cur.DisplayName, 0)
	var typeField *tview.DropDown
	if !editing {
		typeField = f.addDropDown("Type", collectionTypes, defaultType)
	}
	// A new calendar is pre-seeded with the default color (so it always has one);
	// editing shows the existing color.
	initialColor := cur.Color
	if !editing {
		initialColor = defaultCalendarColor
	}
	colorField := f.addInput("Color", initialColor, 0) // #rrggbb; blank on create = default, on edit = unchanged
	f.stylePopup()

	// Pick color… opens the swatch grid over the form (nested modal); the pick is
	// written back into the Color field, so the form owns the value on submit.
	f.AddButton("Pick color…", func() {
		a.openColorPickerCallback(strings.TrimSpace(colorField.GetText()), " Pick a color ", func(hex string) {
			colorField.SetText(hex)
		})
	})

	submit := "Create"
	if editing {
		submit = "Save"
	}
	f.AddButton(submit, func() {
		name := strings.TrimSpace(nameField.GetText())
		if name == "" {
			a.flash("A name is required")
			return
		}
		color := strings.TrimSpace(colorField.GetText())
		if color != "" {
			c, ok := normalizeColor(color)
			if !ok {
				a.flash("Invalid color — use #rrggbb (or leave blank)")
				return
			}
			color = c
		}
		if editing {
			if err := a.store.UpdateCalendarMeta(context.Background(), editID, name, color); err != nil {
				a.flashErr("Calendar update", err)
				return
			}
			a.buildCalendars()
			a.buildTasklists()
			a.reloadCurrent()
			a.closeModal(pageForm)
			a.scheduleSyncDebounced()
			a.flash("Saved — pushes on next sync")
			return
		}
		typeIdx, _ := typeField.GetCurrentOption()
		if err := a.createCalendarWithColor(name, typeIdx, color); err != nil {
			a.flashErr("Create", err)
			return
		}
		a.refresh("")
		a.closeModal(pageForm)
		a.scheduleSyncDebounced()
		a.flash(fmt.Sprintf("Created %q — syncs on next sync", name))
	})
	f.AddButton("Cancel", func() { a.closeModal(pageForm) })
	f.SetCancelFunc(func() { a.closeModal(pageForm) })

	title, height := " New calendar / list ", 13
	if editing {
		title, height = " Edit "+cur.DisplayName+" ", 11
	}
	f.SetBorder(true).SetTitle(title)
	a.openModal(pageForm, f, 62, height)
}

// createCalendarWithColor creates a calendar/list locally with the chosen color
// (offline-first; the server MKCALENDAR carries the color on the next sync).
// typeIdx indexes collectionTypes. Extracted from the form's Create action so
// the create-with-color path is unit-testable without driving the tview form.
func (a *app) createCalendarWithColor(name string, typeIdx int, color string) error {
	if typeIdx < 0 || typeIdx >= len(collectionTypes) {
		typeIdx = 0
	}
	if color == "" {
		color = defaultCalendarColor // every created collection always has a color
	}
	id := store.SafeName(name)
	return a.store.CreateCalendarLocal(context.Background(), id,
		store.CalendarMeta{DisplayName: name, Color: color}, componentsForType(collectionTypes[typeIdx]))
}

// openColorPickerCallback shows the swatch grid seeded with current (a hex, or ""
// for none) and calls onPick with the chosen preset or a custom hex. Cancelling
// leaves the caller's value untouched. Reused by the calendar form and the
// direct `:calendar color` recolor.
func (a *app) openColorPickerCallback(current, title string, onPick func(hex string)) {
	p := newColorPicker()
	p.preselect(current)
	p.SetTitle(title)
	p.onSelect = func(hex string) {
		a.closeModal(pageColor)
		onPick(hex)
	}
	p.onCustom = func() {
		a.closeModal(pageColor)
		a.promptInput("Custom color (#rrggbb, blank cancels)", "Color: ", func(text string) {
			if strings.TrimSpace(text) == "" {
				return
			}
			hex, ok := normalizeColor(text)
			if !ok {
				a.flash("Invalid color — use #rrggbb")
				return
			}
			onPick(hex)
		})
	}
	p.onCancel = func() { a.closeModal(pageColor) }
	a.openModal(pageColor, p, 40, 12)
}

// openColorPicker recolors an existing calendar directly (from `:calendar color`
// with no hex): the swatch grid applied via applyCalendarColor.
func (a *app) openColorPicker(calID string) {
	cal, ok := a.store.Calendar(calID)
	if !ok {
		a.flash("Calendar not found")
		return
	}
	if !a.guardWrite(calID) {
		return
	}
	a.openColorPickerCallback(cal.Color, " Color · "+cal.DisplayName+" ", func(hex string) {
		a.applyCalendarColor(calID, hex)
	})
}

// applyCalendarColor sets a calendar's color offline-first (pushed as a CalDAV
// PROPPATCH on the next sync). Shared by the color picker and `:calendar color`.
func (a *app) applyCalendarColor(calID, hex string) {
	if !a.guardWrite(calID) {
		return
	}
	if err := a.store.UpdateCalendarMeta(context.Background(), calID, "", hex); err != nil {
		a.flashErr("Recolor", err)
		return
	}
	a.buildCalendars()
	a.scheduleSyncDebounced()
	a.flash("Color set (pushes on next sync)")
}

// collectionDeleteNameMatches reports whether typed confirms deletion of a
// collection named name. Trim + case-sensitive: both sides are trimmed so a
// stored stray space can't make a name impossible to type, but the match is
// otherwise exact — deleting a collection is not undoable, so the confirmation
// is deliberately strict.
func collectionDeleteNameMatches(typed, name string) bool {
	return strings.TrimSpace(typed) == strings.TrimSpace(name)
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
		a.flash("Switch to Calendars (c) or Tasks (t) to delete a list")
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

	a.promptDeleteCollection(id, cal)
}

// promptDeleteCollection opens the rigorous type-to-confirm dialog for deleting a
// calendar/list. Unlike an item delete, a collection delete is not undoable (it
// pushes no undo op), so the user must type the collection's exact name before
// Delete fires — a stray keystroke can't wipe a whole collection. Returns the form
// so tests can drive it; the production caller ignores the return.
func (a *app) promptDeleteCollection(id string, cal store.Calendar) *caretForm {
	noun := "calendar"
	if a.mode == modeTasks {
		noun = "list"
	}
	countClause := ""
	if n := len(cal.Resources); n > 0 {
		countClause = fmt.Sprintf(" (%d item(s))", n)
	}
	title := fmt.Sprintf(" ⚠ Delete %s %q%s — cannot be undone ", noun, cal.DisplayName, countClause)

	f := newCaretForm()
	nameField := f.addInput("Type name to confirm", "", 0)
	f.stylePopup()
	f.AddButton("Delete", func() {
		if !collectionDeleteNameMatches(nameField.GetText(), cal.DisplayName) {
			a.flash("Name doesn't match — type it exactly to delete")
			return // keep the dialog open for another attempt
		}
		if err := a.store.MarkCalendarDeleted(context.Background(), id); err != nil {
			a.flashErr("Delete", err)
			return
		}
		a.refresh("")
		a.closeModal(pageForm)
		a.scheduleSyncDebounced()
		a.flash(fmt.Sprintf("Deleted %q", cal.DisplayName))
	})
	f.AddButton("Cancel", func() { a.closeModal(pageForm) })
	f.SetCancelFunc(func() { a.closeModal(pageForm) })
	f.SetBorder(true).SetTitle(title)
	a.openModal(pageForm, f, 62, 7)
	return f
}
