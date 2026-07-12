package ui

import (
	"context"
	"testing"
	"time"

	"github.com/littekge/LazyPlanner/internal/model"
	"github.com/littekge/LazyPlanner/internal/store"
)

func putRecurringEvent(t *testing.T, a *app, calID, summary string, start time.Time, rrule string) string {
	t.Helper()
	uid := summary + "@rec"
	ics := "BEGIN:VCALENDAR\r\nVERSION:2.0\r\nPRODID:-//t//t//EN\r\nBEGIN:VEVENT\r\nUID:" + uid +
		"\r\nSUMMARY:" + summary + "\r\nDTSTAMP:20260701T000000Z\r\nDTSTART:" + start.UTC().Format("20060102T150405Z") +
		"\r\nDTEND:" + start.Add(time.Hour).UTC().Format("20060102T150405Z") + "\r\nRRULE:" + rrule +
		"\r\nEND:VEVENT\r\nEND:VCALENDAR\r\n"
	parsed, err := model.Decode([]byte(ics), time.Local)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := a.store.Put(context.Background(), calID, store.ResourceName(uid), parsed); err != nil {
		t.Fatal(err)
	}
	return uid
}

func putRecurringTodo(t *testing.T, a *app, calID, summary string, due time.Time, rrule string) string {
	t.Helper()
	uid := summary + "@rec"
	ics := "BEGIN:VCALENDAR\r\nVERSION:2.0\r\nPRODID:-//t//t//EN\r\nBEGIN:VTODO\r\nUID:" + uid +
		"\r\nSUMMARY:" + summary + "\r\nDTSTAMP:20260701T000000Z\r\nDTSTART:" + due.Add(-time.Hour).UTC().Format("20060102T150405Z") +
		"\r\nDUE:" + due.UTC().Format("20060102T150405Z") + "\r\nRRULE:" + rrule +
		"\r\nEND:VTODO\r\nEND:VCALENDAR\r\n"
	parsed, err := model.Decode([]byte(ics), time.Local)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := a.store.Put(context.Background(), calID, store.ResourceName(uid), parsed); err != nil {
		t.Fatal(err)
	}
	return uid
}

func todoDue(t *testing.T, a *app, uid string) *model.Todo {
	t.Helper()
	loc, ok := a.store.Locate(uid)
	if !ok {
		t.Fatalf("todo %q not found", uid)
	}
	return findTodo(loc.Object, uid)
}

// TestRecurringTodoSpaceAdvances: Space on a recurring todo rolls it to the next
// occurrence rather than marking it done.
func TestRecurringTodoSpaceAdvances(t *testing.T) {
	now := time.Date(2026, 7, 8, 0, 0, 0, 0, time.UTC)
	a := newRootedTestApp(t, now)
	if err := a.store.CreateCalendarLocal(context.Background(), "tl", store.CalendarMeta{DisplayName: "TL"}, []string{"VTODO"}); err != nil {
		t.Fatal(err)
	}
	uid := putRecurringTodo(t, a, "tl", "Water", time.Date(2026, 7, 6, 9, 0, 0, 0, time.UTC), "FREQ=WEEKLY;COUNT=3")
	a.reload()
	a.setMode(modeTasks)
	// select the recurring list's tree and the task
	a.treeListID = "tl"
	a.buildTreeForList("tl")
	a.selectTreeByUID(uid)

	a.toggleComplete()

	td := todoDue(t, a, uid)
	if td.Completed() {
		t.Error("a recurring todo should advance on Space, not complete")
	}
	if !td.Due.UTC().Equal(time.Date(2026, 7, 13, 9, 0, 0, 0, time.UTC)) {
		t.Errorf("DUE after advance = %s, want 2026-07-13 09:00Z", td.Due.UTC())
	}
}

// TestDeleteRecurringEventOccurrence: deleting one occurrence adds an EXDATE so
// that instance disappears while the rest remain.
func TestDeleteRecurringEventOccurrence(t *testing.T) {
	when := time.Date(2026, 7, 6, 12, 0, 0, 0, time.UTC)
	a := newRootedTestApp(t, when)
	if err := a.store.CreateCalendarLocal(context.Background(), "ev", store.CalendarMeta{DisplayName: "EV"}, []string{"VEVENT"}); err != nil {
		t.Fatal(err)
	}
	uid := putRecurringEvent(t, a, "ev", "Standup", time.Date(2026, 7, 6, 9, 0, 0, 0, time.UTC), "FREQ=WEEKLY;COUNT=4")
	a.reload()

	occ := time.Date(2026, 7, 13, 9, 0, 0, 0, time.UTC) // 2nd occurrence
	loc, _ := a.store.Locate(uid)
	a.deleteOccurrence(loc, editTarget{uid: uid, occStart: occ})

	loc2, _ := a.store.Locate(uid)
	occs, err := loc2.Object.EventOccurrences(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if len(occs) != 3 {
		t.Fatalf("after deleting one occurrence, got %d, want 3", len(occs))
	}
	for _, o := range occs {
		if o.Start.UTC().Equal(occ) {
			t.Error("the deleted occurrence still appears")
		}
	}
}

// TestGrabRecurringEventThisOccurrence: a this-occurrence grab moves only that
// instance, via a RECURRENCE-ID override; the others stay put.
func TestGrabRecurringEventThisOccurrence(t *testing.T) {
	when := time.Date(2026, 7, 6, 12, 0, 0, 0, time.UTC)
	a := newRootedTestApp(t, when)
	if err := a.store.CreateCalendarLocal(context.Background(), "ev", store.CalendarMeta{DisplayName: "EV"}, []string{"VEVENT"}); err != nil {
		t.Fatal(err)
	}
	uid := putRecurringEvent(t, a, "ev", "Standup", time.Date(2026, 7, 6, 9, 0, 0, 0, time.UTC), "FREQ=WEEKLY;COUNT=4")
	a.reload()

	occ := time.Date(2026, 7, 13, 9, 0, 0, 0, time.UTC)
	loc, _ := a.store.Locate(uid)
	a.mode = modeCalendar
	a.viewMode = viewWeek
	a.beginGrab(loc, editTarget{uid: uid, occStart: occ}, scopeThis)
	a.grabNudge('l') // +1 day

	loc2, _ := a.store.Locate(uid)
	ov := loc2.Object.FindOverride(uid, occ)
	if ov == nil {
		t.Fatal("this-occurrence grab did not create a RECURRENCE-ID override")
	}
	if !ov.Start.UTC().Equal(time.Date(2026, 7, 14, 9, 0, 0, 0, time.UTC)) {
		t.Errorf("override start = %s, want 2026-07-14 09:00Z (moved +1 day)", ov.Start.UTC())
	}
	// The other instances are unchanged (still 4 total: 3 from master + 1 override).
	occs, _ := loc2.Object.EventOccurrences(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC))
	if len(occs) != 4 {
		t.Errorf("got %d occurrences after grab, want 4", len(occs))
	}
}

// TestEditRecurringShowsScopePicker: `e` on a recurring item opens the scope
// picker rather than the plain edit form.
func TestEditRecurringShowsScopePicker(t *testing.T) {
	when := time.Date(2026, 7, 6, 12, 0, 0, 0, time.UTC)
	a := newRootedTestApp(t, when)
	if err := a.store.CreateCalendarLocal(context.Background(), "ev", store.CalendarMeta{DisplayName: "EV"}, []string{"VEVENT"}); err != nil {
		t.Fatal(err)
	}
	uid := putRecurringEvent(t, a, "ev", "Standup", time.Date(2026, 7, 6, 9, 0, 0, 0, time.UTC), "FREQ=WEEKLY;COUNT=4")
	a.reload()

	loc, _ := a.store.Locate(uid)
	a.editRecurring(loc, editTarget{uid: uid, occStart: time.Date(2026, 7, 6, 9, 0, 0, 0, time.UTC), recurring: true})
	if !a.root.HasPage(pageConfirm) {
		t.Error("editing a recurring event should open the scope picker")
	}
}
