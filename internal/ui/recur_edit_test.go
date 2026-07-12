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
	// The flash must make clear the task advanced (not completed) — #1.
	if got := a.statusLeft.GetText(true); !strings.Contains(strings.ToLower(got), "advanced") {
		t.Errorf("advance flash = %q, want it to say the task advanced", got)
	}
}

// TestEditTodoThisOccurrenceConfirms locks #3: choosing to edit just this
// occurrence of a recurring todo (which detaches it as a duplicate) first asks
// for confirmation rather than silently splitting the task.
func TestEditTodoThisOccurrenceConfirms(t *testing.T) {
	now := time.Date(2026, 7, 8, 0, 0, 0, 0, time.UTC)
	a := newRootedTestApp(t, now)
	if err := a.store.CreateCalendarLocal(context.Background(), "tl", store.CalendarMeta{DisplayName: "TL"}, []string{"VTODO"}); err != nil {
		t.Fatal(err)
	}
	uid := putRecurringTodo(t, a, "tl", "Water", time.Date(2026, 7, 6, 9, 0, 0, 0, time.UTC), "FREQ=WEEKLY;COUNT=3")
	a.reload()

	loc, _ := a.store.Locate(uid)
	a.editTodoThisOccurrence(loc, uid)
	if !a.root.HasPage(pageConfirm) {
		t.Error("editing this occurrence of a recurring todo should confirm the detach first")
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

// TestEditOccurrenceReseedsFromOverride locks #5: re-editing an occurrence that
// already has an override pre-fills the form from the override, not the master.
func TestEditOccurrenceReseedsFromOverride(t *testing.T) {
	when := time.Date(2026, 7, 6, 12, 0, 0, 0, time.UTC)
	a := newRootedTestApp(t, when)
	if err := a.store.CreateCalendarLocal(context.Background(), "ev", store.CalendarMeta{DisplayName: "EV"}, []string{"VEVENT"}); err != nil {
		t.Fatal(err)
	}
	uid := putRecurringEvent(t, a, "ev", "Standup", time.Date(2026, 7, 6, 9, 0, 0, 0, time.UTC), "FREQ=WEEKLY;COUNT=4")
	a.reload()
	occ := time.Date(2026, 7, 13, 9, 0, 0, 0, time.UTC)

	// Create an override renaming this occurrence to "Overridden".
	loc, _ := a.store.Locate(uid)
	obj, err := model.EditEventOccurrence(loc.Object, uid, occ, false,
		model.EventDraft{Summary: "Overridden", Start: occ, End: occ.Add(time.Hour)}, a.now, a.loc)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := a.store.Put(context.Background(), loc.CalID, loc.Name, obj); err != nil {
		t.Fatal(err)
	}

	// Re-edit this occurrence: the form's summary field must show the override's value.
	loc2, _ := a.store.Locate(uid)
	a.mode = modeCalendar
	a.editEventScoped(loc2, editTarget{uid: uid, occStart: occ, recurring: true}, scopeThis)
	in, ok := a.tv.GetFocus().(*tview.InputField)
	if !ok {
		t.Fatalf("focused primitive is %T, want the form's summary field", a.tv.GetFocus())
	}
	if in.GetText() != "Overridden" {
		t.Errorf("re-edit form seeded summary %q, want %q (from the existing override)", in.GetText(), "Overridden")
	}
}

// masterOccurrenceCount expands the series carrying uid and returns how many
// instances it yields in a wide window.
func masterOccurrenceCount(t *testing.T, a *app, uid string) int {
	t.Helper()
	loc, ok := a.store.Locate(uid)
	if !ok {
		return 0
	}
	occs, err := loc.Object.EventOccurrences(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	return len(occs)
}

// grabFutureSetup puts a 4-week series, drills into the 3rd instance, and returns
// the app, the original UID, and the occurrence instant.
func grabFutureSetup(t *testing.T) (*app, string, time.Time) {
	t.Helper()
	when := time.Date(2026, 7, 6, 12, 0, 0, 0, time.UTC)
	a := newRootedTestApp(t, when)
	if err := a.store.CreateCalendarLocal(context.Background(), "ev", store.CalendarMeta{DisplayName: "EV"}, []string{"VEVENT"}); err != nil {
		t.Fatal(err)
	}
	uid := putRecurringEvent(t, a, "ev", "Standup", time.Date(2026, 7, 6, 9, 0, 0, 0, time.UTC), "FREQ=WEEKLY;COUNT=4")
	a.reload()
	a.mode = modeCalendar
	a.viewMode = viewWeek
	occ := time.Date(2026, 7, 20, 9, 0, 0, 0, time.UTC) // 3rd instance
	a.anchor = model.DayStart(occ)
	return a, uid, occ
}

// TestGrabFutureSplitsAndMoves locks #8: a this-and-future grab splits the series
// (capped master keeps the past instances, a new series holds the future ones) and
// commits both as one undo step.
func TestGrabFutureSplitsAndMoves(t *testing.T) {
	a, uid, occ := grabFutureSetup(t)
	loc, _ := a.store.Locate(uid)

	a.beginGrab(loc, editTarget{uid: uid, occStart: occ, recurring: true}, scopeFuture)

	// The original master is capped to the 2 past instances (07-06, 07-13).
	if n := masterOccurrenceCount(t, a, uid); n != 2 {
		t.Fatalf("capped master has %d occurrences, want 2", n)
	}
	// A new series was grabbed, starting at the occurrence.
	newUID := a.grabUID
	if newUID == uid || newUID == "" {
		t.Fatalf("grab target UID = %q, want a fresh series UID", newUID)
	}
	newLoc, ok := a.store.Locate(newUID)
	if !ok {
		t.Fatal("new future series not found in the store")
	}
	newOccs, _ := newLoc.Object.EventOccurrences(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC))
	if len(newOccs) != 2 || !newOccs[0].Start.UTC().Equal(occ) {
		t.Fatalf("new series occurrences = %d starting %v, want 2 starting %v", len(newOccs), newOccs[0].Start.UTC(), occ)
	}

	// Move the new series +1 day and keep it.
	a.grabNudge('l')
	a.commitGrab()
	moved, _ := a.store.Locate(newUID)
	mo, _ := moved.Object.EventOccurrences(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC))
	if !mo[0].Start.UTC().Equal(occ.AddDate(0, 0, 1)) {
		t.Errorf("moved new series first start = %v, want %v (+1 day)", mo[0].Start.UTC(), occ.AddDate(0, 0, 1))
	}
}

// TestGrabFutureCancelRestores locks #8's revert: cancelling a this-and-future
// grab deletes the new series and restores the original (uncapped) master.
func TestGrabFutureCancelRestores(t *testing.T) {
	a, uid, occ := grabFutureSetup(t)
	loc, _ := a.store.Locate(uid)

	a.beginGrab(loc, editTarget{uid: uid, occStart: occ, recurring: true}, scopeFuture)
	newUID := a.grabUID
	a.grabNudge('l')
	a.cancelGrab()

	// The new series is gone and the original master is back to all 4 occurrences.
	if _, ok := a.store.Locate(newUID); ok {
		t.Error("cancel should have deleted the new future series")
	}
	if n := masterOccurrenceCount(t, a, uid); n != 4 {
		t.Errorf("after cancel the master has %d occurrences, want 4 (restored)", n)
	}
}
