package ui

import (
	"context"
	"testing"
	"time"
	// Embed the IANA database in the test binary. internal/ui does not import it
	// (only cmd/lazyplanner does), so on a host without system zoneinfo every
	// non-UTC zone below would t.Skip its way to a vacuous green — see
	// tzanchor_uipath_test.go for the same guard on the Task 3 tests.
	_ "time/tzdata"

	"github.com/rivo/tview"

	"github.com/littekge/LazyPlanner/internal/model"
	"github.com/littekge/LazyPlanner/internal/store"
)

// Opening the edit form on a recurring event and pressing Save WITHOUT touching
// the Repeat dropdown must not rewrite the rule. The seed anchor (the raw UTC
// start) and the resolve anchor (the form's local start) disagreed on the
// weekday, so "unchanged" compared false and the series silently moved a day.
func TestUntouchedRepeatDropdownDoesNotRewriteRule(t *testing.T) {
	for _, zone := range []string{"UTC", "America/New_York", "Europe/Berlin", "Asia/Kolkata", "Pacific/Kiritimati"} {
		t.Run(zone, func(t *testing.T) {
			loc, err := time.LoadLocation(zone)
			if err != nil {
				t.Skipf("zone %q unavailable: %v", zone, err)
			}
			a := newRootedTestApp(t, time.Date(2026, 8, 1, 9, 0, 0, 0, time.UTC))
			a.loc = loc

			uid := "anchor-1"
			ics := "BEGIN:VCALENDAR\r\nVERSION:2.0\r\nPRODID:-//t//t//EN\r\nBEGIN:VEVENT\r\nUID:" + uid +
				"\r\nSUMMARY:Standup\r\nDTSTAMP:20260701T000000Z\r\n" +
				"DTSTART:20260824T230000Z\r\nDTEND:20260825T000000Z\r\n" +
				"RRULE:FREQ=WEEKLY;BYDAY=MO\r\nEND:VEVENT\r\nEND:VCALENDAR\r\n"
			parsed, err := model.Decode([]byte(ics), time.Local)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := a.store.Put(context.Background(), "ev", store.ResourceName(uid), parsed); err != nil {
				t.Fatal(err)
			}
			located, ok := a.store.Locate(uid)
			if !ok {
				t.Fatal("seeded event not found")
			}
			before := rruleValue(t, located.Object, uid)

			a.showEventForm(located, uid)
			name, front := a.root.GetFrontPage()
			if name != pageForm {
				t.Fatalf("front page = %q, want %q", name, pageForm)
			}
			f := findCaretFormIn(front)
			if f == nil {
				t.Fatal("event form not found")
			}
			pressButton(t, f, "Save")

			after, ok := a.store.Locate(uid)
			if !ok {
				t.Fatal("event gone after save")
			}
			if got := rruleValue(t, after.Object, uid); got != before {
				t.Errorf("untouched Repeat dropdown rewrote the rule: %q → %q", before, got)
			}
		})
	}
}

// The todo form shares the seed/resolve anchor and must share the guard.
func TestUntouchedRepeatDropdownDoesNotRewriteTodoRule(t *testing.T) {
	berlin, err := time.LoadLocation("Europe/Berlin")
	if err != nil {
		t.Skip(err)
	}
	a := newRootedTestApp(t, time.Date(2026, 8, 1, 9, 0, 0, 0, time.UTC))
	a.loc = berlin

	uid := "anchor-todo"
	ics := "BEGIN:VCALENDAR\r\nVERSION:2.0\r\nPRODID:-//t//t//EN\r\nBEGIN:VTODO\r\nUID:" + uid +
		"\r\nSUMMARY:Water plants\r\nDTSTAMP:20260701T000000Z\r\n" +
		"DUE:20260824T230000Z\r\nRRULE:FREQ=WEEKLY;BYDAY=MO\r\nEND:VTODO\r\nEND:VCALENDAR\r\n"
	parsed, err := model.Decode([]byte(ics), time.Local)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := a.store.Put(context.Background(), "tasks", store.ResourceName(uid), parsed); err != nil {
		t.Fatal(err)
	}
	located, _ := a.store.Locate(uid)
	before := rruleValue(t, located.Object, uid)

	a.showTodoForm(located, uid)
	_, front := a.root.GetFrontPage()
	pressButton(t, findCaretFormIn(front), "Save")

	after, _ := a.store.Locate(uid)
	if got := rruleValue(t, after.Object, uid); got != before {
		t.Errorf("untouched Repeat dropdown rewrote the todo rule: %q → %q", before, got)
	}
}

// A weekly event created at a local time whose UTC date differs must recur on the
// day the user picked — the dropdown said "Weekly on Tue", so Tuesday it is.
func TestCreatedWeeklyEventRecursOnThePickedDay(t *testing.T) {
	ny, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Skip(err)
	}
	a := newRootedTestApp(t, time.Date(2026, 8, 1, 9, 0, 0, 0, time.UTC))
	a.loc = ny

	base := time.Date(2026, 8, 25, 0, 0, 0, 0, ny) // Tuesday
	a.showCreateEventForm("ev", base)
	_, front := a.root.GetFrontPage()
	f := findCaretFormIn(front)
	formItemByLabel(t, f, "Summary").(*tview.InputField).SetText("Evening sync")
	formItemByLabel(t, f, "All day").(*tview.Checkbox).SetChecked(false)
	formItemByLabel(t, f, "Start date").(*tview.InputField).SetText("2026-08-25")
	formItemByLabel(t, f, "Start time").(*tview.InputField).SetText("20:00")
	formItemByLabel(t, f, "End time").(*tview.InputField).SetText("21:00")
	formItemByLabel(t, f, "Repeat").(*tview.DropDown).SetCurrentOption(2) // Weekly on Tue
	pressButton(t, f, "Create")

	cs, ok := a.store.Calendar("ev")
	if !ok {
		t.Fatal("calendar missing")
	}
	for _, r := range cs.Resources {
		for _, ev := range r.Object.Events {
			if ev.Summary != "Evening sync" {
				continue
			}
			occs, err := ev.Occurrences(
				time.Date(2026, 8, 20, 0, 0, 0, 0, ny), time.Date(2026, 11, 20, 0, 0, 0, 0, ny))
			if err != nil {
				t.Fatal(err)
			}
			if len(occs) == 0 {
				t.Fatal("no occurrences")
			}
			for _, o := range occs {
				local := o.Start.In(ny)
				if local.Weekday() != time.Tuesday {
					t.Errorf("occurrence %v is a %v, want Tuesday", local, local.Weekday())
				}
				if local.Hour() != 20 {
					t.Errorf("occurrence %v drifted off 20:00 (DST)", local)
				}
			}
			return
		}
	}
	t.Fatal("created event not found")
}

// rruleValue returns the RRULE of the component carrying uid, or "" when absent.
func rruleValue(t *testing.T, obj *model.Parsed, uid string) string {
	t.Helper()
	for _, c := range obj.Calendar.Children {
		if p := c.Props.Get("UID"); p != nil && p.Value == uid {
			if r := c.Props.Get("RRULE"); r != nil {
				return r.Value
			}
			return ""
		}
	}
	t.Fatalf("component %q not found", uid)
	return ""
}
