package ui

import (
	"context"
	"testing"
	"time"
	// Embed the IANA database in the test binary. internal/ui does not import it
	// (only cmd/lazyplanner does), so on a host without system zoneinfo the zone
	// lookups below would fail — the loads are guarded with t.Fatalf, not t.Skip,
	// so dropping this import fails loudly instead of turning the guards vacuous.
	// See tzanchor_uipath_test.go for the same guard on the Task 3 tests.
	_ "time/tzdata"

	"github.com/rivo/tview"

	"github.com/littekge/LazyPlanner/internal/model"
	"github.com/littekge/LazyPlanner/internal/store"
)

// Opening the edit form on a recurring event and pressing Save WITHOUT touching
// the Repeat dropdown must not rewrite the rule. The seed anchor (the raw UTC
// start) and the resolve anchor (the form's local start) disagreed on the
// weekday, so "unchanged" compared false and the series silently moved a day.
//
// The UTC anchor must land on the OTHER local calendar day, in the direction
// this zone's offset pushes it, or the defect can't reproduce: an eastern
// (positive) offset needs a late-UTC-hour anchor to roll FORWARD into the next
// local day (23:00Z Monday -> local Tuesday); a western (negative) offset needs
// an early-UTC-hour anchor to roll BACK into the previous local day (01:00Z
// Tuesday -> local Monday). A single fixed anchor is structurally incapable of
// covering both directions — the original fixture's late-Monday anchor never
// crosses for a negative offset, which is why America/New_York (the zone the
// bug was originally reported from) passed even under the reverted fix.
func TestUntouchedRepeatDropdownDoesNotRewriteRule(t *testing.T) {
	for _, zone := range []string{"UTC", "America/New_York", "Europe/Berlin", "Asia/Kolkata", "Pacific/Kiritimati"} {
		t.Run(zone, func(t *testing.T) {
			loc, err := time.LoadLocation(zone)
			if err != nil {
				// Fatal, not Skip: tzdata is embedded above, so a failure here means
				// the guard is not running — which is exactly how this class hides.
				t.Fatalf("zone %q must load (tzdata is embedded): %v", zone, err)
			}
			a := newRootedTestApp(t, time.Date(2026, 8, 1, 9, 0, 0, 0, time.UTC))
			a.loc = loc

			// offsetSec's sign picks the anchor direction; UTC's offset is always
			// zero, so it is kept as an explicit non-failing control below rather
			// than accidentally sharing a branch with a real (crossing) case.
			_, offsetSec := time.Date(2026, 8, 24, 12, 0, 0, 0, time.UTC).In(loc).Zone()

			uid := "anchor-1"
			var ics string
			switch {
			case offsetSec > 0:
				// Eastern: 23:00Z Monday rolls forward to local Tuesday.
				ics = "BEGIN:VCALENDAR\r\nVERSION:2.0\r\nPRODID:-//t//t//EN\r\nBEGIN:VEVENT\r\nUID:" + uid +
					"\r\nSUMMARY:Standup\r\nDTSTAMP:20260701T000000Z\r\n" +
					"DTSTART:20260824T230000Z\r\nDTEND:20260825T000000Z\r\n" +
					"RRULE:FREQ=WEEKLY;BYDAY=MO\r\nEND:VEVENT\r\nEND:VCALENDAR\r\n"
			case offsetSec < 0:
				// Western: 01:00Z Tuesday rolls back to local Monday.
				ics = "BEGIN:VCALENDAR\r\nVERSION:2.0\r\nPRODID:-//t//t//EN\r\nBEGIN:VEVENT\r\nUID:" + uid +
					"\r\nSUMMARY:Standup\r\nDTSTAMP:20260701T000000Z\r\n" +
					"DTSTART:20260825T010000Z\r\nDTEND:20260825T020000Z\r\n" +
					"RRULE:FREQ=WEEKLY;BYDAY=TU\r\nEND:VEVENT\r\nEND:VCALENDAR\r\n"
			default:
				// UTC control: zero offset never crosses a day boundary, so this
				// subtest is deliberately expected to stay green even under the
				// reverted fix — it exercises the "genuinely unchanged" baseline,
				// not a failure case.
				ics = "BEGIN:VCALENDAR\r\nVERSION:2.0\r\nPRODID:-//t//t//EN\r\nBEGIN:VEVENT\r\nUID:" + uid +
					"\r\nSUMMARY:Standup\r\nDTSTAMP:20260701T000000Z\r\n" +
					"DTSTART:20260824T230000Z\r\nDTEND:20260825T000000Z\r\n" +
					"RRULE:FREQ=WEEKLY;BYDAY=MO\r\nEND:VEVENT\r\nEND:VCALENDAR\r\n"
			}
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
	// Fatal, not Skip: tzdata is embedded above, so a failure here means the
	// guard is not running — which is exactly how this class hides.
	berlin, err := time.LoadLocation("Europe/Berlin")
	if err != nil {
		t.Fatalf("Europe/Berlin must load (tzdata is embedded): %v", err)
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
	located, ok := a.store.Locate(uid)
	if !ok {
		t.Fatal("seeded todo not found")
	}
	before := rruleValue(t, located.Object, uid)

	a.showTodoForm(located, uid)
	_, front := a.root.GetFrontPage()
	pressButton(t, findCaretFormIn(front), "Save")

	after, ok := a.store.Locate(uid)
	if !ok {
		t.Fatal("todo gone after save")
	}
	if got := rruleValue(t, after.Object, uid); got != before {
		t.Errorf("untouched Repeat dropdown rewrote the todo rule: %q → %q", before, got)
	}
}

// A weekly event created at a local time whose UTC date differs must recur on the
// day the user picked — the dropdown said "Weekly on Tue", so Tuesday it is.
func TestCreatedWeeklyEventRecursOnThePickedDay(t *testing.T) {
	// Fatal, not Skip: tzdata is embedded above, so a failure here means the
	// guard is not running — which is exactly how this class hides.
	ny, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Fatalf("America/New_York must load (tzdata is embedded): %v", err)
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
