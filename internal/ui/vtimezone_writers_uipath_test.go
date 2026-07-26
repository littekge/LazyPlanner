package ui

import (
	"context"
	"strings"
	"testing"
	"time"
	// Embed the IANA database in the test binary. internal/ui does not import it
	// (only cmd/lazyplanner does), so on a host without system zoneinfo the zone
	// lookups below would fail — they are guarded with t.Fatalf, not t.Skip, so a
	// missing database fails loudly instead of turning these guards vacuous.
	_ "time/tzdata"

	"github.com/emersion/go-ical"
	"github.com/rivo/tview"

	"github.com/littekge/LazyPlanner/internal/model"
	"github.com/littekge/LazyPlanner/internal/store"
)

// assertEveryTZIDDefined fails when an item property references a TZID the object
// does not define with a VTIMEZONE. An undefined TZID is what Sabre/NextCloud
// validation rejects and what Apple/Thunderbird treat as floating — putting the
// series on the wrong hour in every other client.
func assertEveryTZIDDefined(t *testing.T, obj *model.Parsed, what string) {
	t.Helper()
	defined := map[string]bool{}
	for _, c := range obj.Calendar.Children {
		if c.Name == ical.CompTimezone {
			if p := c.Props.Get(ical.PropTimezoneID); p != nil {
				defined[p.Value] = true
			}
		}
	}
	referenced := 0
	for _, c := range obj.Calendar.Children {
		if c.Name != ical.CompEvent && c.Name != ical.CompToDo {
			continue
		}
		for name, props := range c.Props {
			for _, p := range props {
				tzid := p.Params.Get(ical.ParamTimezoneID)
				if tzid == "" {
					continue
				}
				referenced++
				if !defined[tzid] {
					t.Errorf("%s: %s carries TZID=%q with no VTIMEZONE defining it (defined: %v)",
						what, name, tzid, defined)
				}
			}
		}
	}
	if referenced == 0 {
		t.Fatalf("%s: no property carries a TZID — the guard is vacuous", what)
	}
}

// seedEvent stores ics under the "ev" calendar and returns its location.
func seedEvent(t *testing.T, a *app, calID, uid, ics string) store.Located {
	t.Helper()
	parsed, err := model.Decode([]byte(ics), a.loc)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := a.store.Put(context.Background(), calID, store.ResourceName(uid), parsed); err != nil {
		t.Fatal(err)
	}
	located, ok := a.store.Locate(uid)
	if !ok {
		t.Fatalf("seeded %q not found", uid)
	}
	return located
}

// repeatIndex is the Repeat dropdown index of the choice labelled with prefix.
// tview.DropDown exposes no option reader, so the index comes from the same
// model.RepeatChoices the form was built from.
func repeatIndex(t *testing.T, choices *model.RepeatChoices, prefix string) int {
	t.Helper()
	for i, l := range choices.Labels() {
		if strings.HasPrefix(l, prefix) {
			return i
		}
	}
	t.Fatalf("no Repeat choice starting %q; have %v", prefix, choices.Labels())
	return -1
}

// legacyWednesdayUTCSeries is the reviewer's repro fixture: a UTC-anchored weekly
// series whose UTC weekday (Wednesday) is not its New York weekday (Tuesday), so a
// rule change authored in the form re-anchors it with a TZID.
const legacyWednesdayUTCSeries = "BEGIN:VCALENDAR\r\nVERSION:2.0\r\nPRODID:-//t//t//EN\r\n" +
	"BEGIN:VEVENT\r\nUID:rewrite-1\r\nSUMMARY:Legacy series\r\nDTSTAMP:20260701T000000Z\r\n" +
	"DTSTART:20260729T000000Z\r\nDTEND:20260729T010000Z\r\n" +
	"RRULE:FREQ=WEEKLY;BYDAY=WE\r\nEND:VEVENT\r\nEND:VCALENDAR\r\n"

// RewriteEventRule is the PRIMARY scope=All rule-change path (itemforms.go's
// showEventForm). It re-anchors the master in the zone the new rule was authored
// in, so the object must gain the matching VTIMEZONE — it shipped emitting a TZID
// nothing defined.
//
// This drives the real entry point (form → Repeat dropdown → Save → the stored
// object), not model.RewriteEventRule in isolation: whether production reaches the
// rewrite path at all is UI-derived (`d.Recur != nil`).
func TestRewriteEventRuleDefinesTheZoneItAnchorsIn(t *testing.T) {
	ny, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Fatalf("America/New_York must load (tzdata is embedded): %v", err)
	}
	a := newRootedTestApp(t, time.Date(2026, 7, 1, 9, 0, 0, 0, time.UTC))
	a.loc = ny

	located := seedEvent(t, a, "ev", "rewrite-1", legacyWednesdayUTCSeries)
	a.showEventForm(located, "rewrite-1")
	name, front := a.root.GetFrontPage()
	if name != pageForm {
		t.Fatalf("front page = %q, want %q", name, pageForm)
	}
	f := findCaretFormIn(front)
	if f == nil {
		t.Fatal("event form not found")
	}
	// The stored start is Wednesday in UTC but Tuesday in New York, so the form's
	// weekly choice is "Weekly on Tue" — a genuine rule change against BYDAY=WE.
	// The choices are rebuilt exactly as showEventForm built them, so the index
	// lines up with the dropdown the form laid out.
	ev := located.Object.Events[0]
	choices := a.newEventRepeat(ev, ev.Start)
	formItemByLabel(t, f, "Repeat").(*tview.DropDown).
		SetCurrentOption(repeatIndex(t, choices, "Weekly on Tue"))
	pressButton(t, f, "Save")

	after, ok := a.store.Locate("rewrite-1")
	if !ok {
		t.Fatal("event gone after save")
	}
	dtstart := after.Object.Events[0].Raw.Props.Get(ical.PropDateTimeStart)
	if got := dtstart.Params.Get(ical.ParamTimezoneID); got != "America/New_York" {
		t.Fatalf("DTSTART TZID = %q (value %q); the rule-authoring re-anchor did not happen, "+
			"so the VTIMEZONE guard below would be vacuous", got, dtstart.Value)
	}
	assertEveryTZIDDefined(t, after.Object, "RewriteEventRule")
}

// SplitEvent's future half (model.NewSeriesFrom) is a brand-new calendar object: it
// carries the master's TZID-anchored DTSTART but starts from a fresh
// ical.NewCalendar, so it must build its own VTIMEZONE.
func TestSplitEventFutureSeriesDefinesItsZone(t *testing.T) {
	ny, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Fatalf("America/New_York must load (tzdata is embedded): %v", err)
	}
	a := newRootedTestApp(t, time.Date(2026, 7, 1, 9, 0, 0, 0, time.UTC))
	a.loc = ny

	// A TZID-anchored series, as every rule authored since this arc is written.
	start := time.Date(2026, 8, 25, 20, 0, 0, 0, ny) // Tuesday
	obj, err := model.NewEventObject(model.EventDraft{
		Summary: "Evening sync", Start: start, End: start.Add(time.Hour),
		Recur: &model.RecurSpec{Freq: model.FreqWeekly, Weekdays: []time.Weekday{time.Tuesday}},
	}, start)
	if err != nil {
		t.Fatal(err)
	}
	uid := obj.Events[0].UID
	if _, err := a.store.Put(context.Background(), "ev", store.ResourceName(uid), obj); err != nil {
		t.Fatal(err)
	}
	located, ok := a.store.Locate(uid)
	if !ok {
		t.Fatal("seeded event not found")
	}

	occ := start.AddDate(0, 0, 14) // split at the third occurrence
	a.editEventScoped(located, editTarget{uid: uid, occStart: occ, recurring: true}, scopeFuture)
	_, front := a.root.GetFrontPage()
	f := findCaretFormIn(front)
	if f == nil {
		t.Fatal("this & future form not found")
	}
	formItemByLabel(t, f, "Summary").(*tview.InputField).SetText("Later sync")
	pressButton(t, f, "Save")

	var future store.Located
	for _, cal := range a.store.Calendars() {
		for _, res := range cal.Resources {
			if res.Object == nil || len(res.Object.Events) == 0 {
				continue
			}
			if res.Object.Events[0].Summary == "Later sync" {
				future = store.Located{CalID: cal.ID, Name: res.Name, Object: res.Object}
			}
		}
	}
	if future.Object == nil {
		t.Fatal("the split's future series was not written")
	}
	assertEveryTZIDDefined(t, future.Object, "NewSeriesFrom (split future half)")
}

// DetachTodoOccurrence builds the same fresh-calendar shape for a todo: the
// standalone one-off clones the series' TZID-anchored DUE into an object with no
// VTIMEZONE of its own.
func TestDetachTodoOccurrenceDefinesItsZone(t *testing.T) {
	ny, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Fatalf("America/New_York must load (tzdata is embedded): %v", err)
	}
	a := newRootedTestApp(t, time.Date(2026, 7, 1, 9, 0, 0, 0, time.UTC))
	a.loc = ny

	due := time.Date(2026, 8, 25, 20, 0, 0, 0, ny) // Tuesday
	obj := model.NewTodoObject(model.TodoDraft{
		Summary: "Water plants", HasDue: true, Due: due,
		Recur: &model.RecurSpec{Freq: model.FreqWeekly, Weekdays: []time.Weekday{time.Tuesday}},
	}, due)
	uid := obj.Todos[0].UID
	if _, err := a.store.Put(context.Background(), "tasks", store.ResourceName(uid), obj); err != nil {
		t.Fatal(err)
	}
	located, ok := a.store.Locate(uid)
	if !ok {
		t.Fatal("seeded todo not found")
	}

	// The confirm dialog is skipped deliberately; its "Detach" button calls exactly
	// this, and the dialog is not what the guard is about.
	a.editTodoDetachForm(located, uid, located.Object.Todos[0])
	_, front := a.root.GetFrontPage()
	f := findCaretFormIn(front)
	if f == nil {
		t.Fatal("detach form not found")
	}
	formItemByLabel(t, f, "Summary").(*tview.InputField).SetText("Water plants (once)")
	pressButton(t, f, "Save")

	var standalone *model.Parsed
	for _, cal := range a.store.Calendars() {
		for _, res := range cal.Resources {
			if res.Object == nil || len(res.Object.Todos) == 0 {
				continue
			}
			if res.Object.Todos[0].Summary == "Water plants (once)" {
				standalone = res.Object
			}
		}
	}
	if standalone == nil {
		t.Fatal("the detached one-off task was not written")
	}
	assertEveryTZIDDefined(t, standalone, "DetachTodoOccurrence")
}
