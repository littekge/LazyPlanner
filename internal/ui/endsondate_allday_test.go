package ui

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/rivo/tview"

	"github.com/littekge/LazyPlanner/internal/model"
	"github.com/littekge/LazyPlanner/internal/store"
)

// The all-day half of the "Ends on date D" contract, driven from the real entry
// point: the New event / New task form a user opens, its Repeat → Custom… →
// "Ends on date" sub-form, the Create button, and the object that actually lands
// in the store. Pass 22 fixed the timed case and recorded all-day as already
// correct; it was not — a DATE UNTIL was written from the UTC date and read back
// as UTC midnight, so D's local-midnight occurrence fell outside its own rule.
//
// The zone sweep matters: the defect's sign flips with the UTC offset (west of
// UTC drops D on read, east of UTC stores D-1), so a single-zone test would have
// passed on half the planet.

var endsOnDateZones = []string{"UTC", "America/New_York", "Asia/Kolkata", "Pacific/Kiritimati"}

func endsOnDateZone(t *testing.T, name string) *time.Location {
	t.Helper()
	loc, err := time.LoadLocation(name)
	if err != nil {
		t.Skipf("zone %q unavailable: %v", name, err)
	}
	return loc
}

// fillCustomEndsOnDate walks the Custom… sub-form the way a user does: pick
// Custom… in Repeat, set "Every 1 day", Ends = On date, type D, press OK.
//
// The create forms build their own RepeatChoices internally, so the entry is
// picked positionally: NewRepeatChoices always appends Custom… last. Picking the
// wrong option fails loudly on the next line rather than silently — only Custom…
// opens the sub-form page.
func (a *app) fillCustomEndsOnDate(t *testing.T, dd *tview.DropDown, until string) {
	t.Helper()
	dd.SetCurrentOption(dd.GetOptionCount() - 1)
	name, front := a.root.GetFrontPage()
	if name != pageRepeat {
		t.Fatalf("front page = %q, want %q", name, pageRepeat)
	}
	sub := findCaretFormIn(front)
	if sub == nil {
		t.Fatal("custom repeat sub-form not found")
	}
	formItemByLabel(t, sub, "Every").(*tview.InputField).SetText("1")
	formItemByLabel(t, sub, "Unit").(*tview.DropDown).SetCurrentOption(0) // days
	formItemByLabel(t, sub, "Ends").(*tview.DropDown).SetCurrentOption(1) // On date
	formItemByLabel(t, sub, "Until").(*tview.InputField).SetText(until)
	pressButton(t, sub, "OK")
}

// storedObject returns the single object committed to calID, re-decoded in loc
// from its own bytes — the store parses in time.Local, and the zone the series
// lives in is what the reading depends on.
func storedObject(t *testing.T, a *app, calID string, loc *time.Location) *model.Parsed {
	t.Helper()
	cs, ok := a.store.Calendar(calID)
	if !ok {
		t.Fatalf("calendar %q missing", calID)
	}
	if len(cs.Resources) != 1 {
		t.Fatalf("stored %d resources, want 1 (form Create did not commit?)", len(cs.Resources))
	}
	raw, err := cs.Resources[0].Object.Encode()
	if err != nil {
		t.Fatalf("encode stored object: %v", err)
	}
	obj, err := model.Decode(raw, loc)
	if err != nil {
		t.Fatalf("decode stored object: %v", err)
	}
	return obj
}

func storedUntil(t *testing.T, obj *model.Parsed) string {
	t.Helper()
	for _, c := range obj.Calendar.Children {
		if p := c.Props.Get("RRULE"); p != nil {
			for _, part := range strings.Split(p.Value, ";") {
				if strings.HasPrefix(part, "UNTIL=") {
					return part[len("UNTIL="):]
				}
			}
			t.Fatalf("stored RRULE %q has no UNTIL", p.Value)
		}
	}
	t.Fatal("no RRULE on the stored object")
	return ""
}

// TestEndsOnDateAllDayEventUIPathIncludesEndDay: New event form → All day →
// Repeat → Custom… → Ends on date 2026-07-25 → Create. The series stored in the
// calendar must occur ON 2026-07-25.
func TestEndsOnDateAllDayEventUIPathIncludesEndDay(t *testing.T) {
	for _, zone := range endsOnDateZones {
		t.Run(zone, func(t *testing.T) {
			loc := endsOnDateZone(t, zone)
			a := newRootedTestApp(t, time.Date(2026, 7, 1, 9, 0, 0, 0, time.UTC))
			a.loc = loc
			if err := a.store.CreateCalendarLocal(context.Background(), "ev",
				store.CalendarMeta{DisplayName: "EV"}, []string{"VEVENT"}); err != nil {
				t.Fatal(err)
			}
			a.reload()

			a.showCreateEventForm("ev", time.Date(2026, 7, 20, 0, 0, 0, 0, loc))
			pageName, front := a.root.GetFrontPage()
			if pageName != pageForm {
				t.Fatalf("front page = %q, want %q", pageName, pageForm)
			}
			f := findCaretFormIn(front)
			if f == nil {
				t.Fatal("event form not found")
			}
			formItemByLabel(t, f, "Summary").(*tview.InputField).SetText("All-day daily")
			formItemByLabel(t, f, "All day").(*tview.Checkbox).SetChecked(true)
			formItemByLabel(t, f, "Start date").(*tview.InputField).SetText("2026-07-20")

			a.fillCustomEndsOnDate(t, formItemByLabel(t, f, "Repeat").(*tview.DropDown), "2026-07-25")
			pressButton(t, f, "Create")

			obj := storedObject(t, a, "ev", loc)
			if got := storedUntil(t, obj); got != "20260725" {
				t.Errorf("stored UNTIL = %q, want the selected end day as a DATE, 20260725", got)
			}

			occs, err := obj.EventOccurrences(
				time.Date(2026, 7, 1, 0, 0, 0, 0, loc),
				time.Date(2026, 8, 1, 0, 0, 0, 0, loc),
			)
			if err != nil {
				t.Fatalf("EventOccurrences: %v", err)
			}
			var days []string
			for _, o := range occs {
				days = append(days, o.Start.In(loc).Format("2006-01-02"))
			}
			want := []string{"2026-07-20", "2026-07-21", "2026-07-22", "2026-07-23", "2026-07-24", "2026-07-25"}
			if strings.Join(days, ",") != strings.Join(want, ",") {
				t.Errorf("stored all-day series occurs on %v, want %v", days, want)
			}
		})
	}
}

// TestEndsOnDateAllDayTodoUIPathIncludesEndDay is the task twin, through the New
// task form. A recurring todo shows only its live due, so "includes D" means the
// series still advances onto D instead of reporting itself finished.
func TestEndsOnDateAllDayTodoUIPathIncludesEndDay(t *testing.T) {
	for _, zone := range endsOnDateZones {
		t.Run(zone, func(t *testing.T) {
			loc := endsOnDateZone(t, zone)
			now := time.Date(2026, 7, 24, 9, 0, 0, 0, loc)
			a := newRootedTestApp(t, now)
			a.loc = loc
			if err := a.store.CreateCalendarLocal(context.Background(), "td",
				store.CalendarMeta{DisplayName: "TD"}, []string{"VTODO"}); err != nil {
				t.Fatal(err)
			}
			a.reload()

			a.showCreateTodoForm("td", "")
			pageName, front := a.root.GetFrontPage()
			if pageName != pageForm {
				t.Fatalf("front page = %q, want %q", pageName, pageForm)
			}
			f := findCaretFormIn(front)
			if f == nil {
				t.Fatal("todo form not found")
			}
			formItemByLabel(t, f, "Summary").(*tview.InputField).SetText("All-day daily task")
			formItemByLabel(t, f, "Due date").(*tview.InputField).SetText("2026-07-24")
			formItemByLabel(t, f, "Due time").(*tview.InputField).SetText("") // untimed = all-day

			a.fillCustomEndsOnDate(t, formItemByLabel(t, f, "Repeat").(*tview.DropDown), "2026-07-25")
			pressButton(t, f, "Create")

			obj := storedObject(t, a, "td", loc)
			if got := storedUntil(t, obj); got != "20260725" {
				t.Errorf("stored UNTIL = %q, want the selected end day as a DATE, 20260725", got)
			}
			if len(obj.Todos) != 1 {
				t.Fatalf("stored %d todos, want 1", len(obj.Todos))
			}
			out, exhausted, err := model.AdvanceRecurringTodo(obj, obj.Todos[0].UID, now, loc)
			if err != nil {
				t.Fatalf("AdvanceRecurringTodo: %v", err)
			}
			if exhausted {
				t.Fatal("completing the 07-24 occurrence finished the series; 2026-07-25 was the selected end day")
			}
			if got := out.Todos[0].Due.In(loc).Format("2006-01-02"); got != "2026-07-25" {
				t.Errorf("advanced due = %s, want 2026-07-25", got)
			}
		})
	}
}

// TestEndsOnDateAllDayUIPathExcludesDayAfter closes the class on the other side:
// the same walk with D = 2026-07-24 must stop there, so the fix widens the series
// by the selected day and not by a blanket extra day.
func TestEndsOnDateAllDayUIPathExcludesDayAfter(t *testing.T) {
	for _, zone := range endsOnDateZones {
		t.Run(zone, func(t *testing.T) {
			loc := endsOnDateZone(t, zone)
			a := newRootedTestApp(t, time.Date(2026, 7, 1, 9, 0, 0, 0, time.UTC))
			a.loc = loc
			if err := a.store.CreateCalendarLocal(context.Background(), "ev",
				store.CalendarMeta{DisplayName: "EV"}, []string{"VEVENT"}); err != nil {
				t.Fatal(err)
			}
			a.reload()

			a.showCreateEventForm("ev", time.Date(2026, 7, 20, 0, 0, 0, 0, loc))
			_, front := a.root.GetFrontPage()
			f := findCaretFormIn(front)
			if f == nil {
				t.Fatal("event form not found")
			}
			formItemByLabel(t, f, "Summary").(*tview.InputField).SetText("All-day daily")
			formItemByLabel(t, f, "All day").(*tview.Checkbox).SetChecked(true)
			formItemByLabel(t, f, "Start date").(*tview.InputField).SetText("2026-07-20")

			a.fillCustomEndsOnDate(t, formItemByLabel(t, f, "Repeat").(*tview.DropDown), "2026-07-24")
			pressButton(t, f, "Create")

			obj := storedObject(t, a, "ev", loc)
			if got := storedUntil(t, obj); got != "20260724" {
				t.Errorf("stored UNTIL = %q, want 20260724", got)
			}
			occs, err := obj.EventOccurrences(
				time.Date(2026, 7, 1, 0, 0, 0, 0, loc),
				time.Date(2026, 8, 1, 0, 0, 0, 0, loc),
			)
			if err != nil {
				t.Fatalf("EventOccurrences: %v", err)
			}
			last := occs[len(occs)-1].Start.In(loc).Format("2006-01-02")
			if len(occs) != 5 || last != "2026-07-24" {
				t.Errorf("series ran to %s over %d occurrences, want 5 ending 2026-07-24", last, len(occs))
			}
		})
	}
}
