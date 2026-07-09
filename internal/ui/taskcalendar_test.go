package ui

import (
	"context"
	"testing"
	"time"

	"github.com/littekge/LazyPlanner/internal/model"
	"github.com/littekge/LazyPlanner/internal/store"
)

// putDueTask writes a VTODO with a due datetime into calID and returns its UID.
func putDueTask(t *testing.T, a *app, calID, summary string, due time.Time) string {
	t.Helper()
	uid := summary + "@due"
	ics := "BEGIN:VCALENDAR\r\nVERSION:2.0\r\nPRODID:-//t//t//EN\r\nBEGIN:VTODO\r\nUID:" + uid +
		"\r\nSUMMARY:" + summary + "\r\nDTSTAMP:20260701T000000Z\r\nDUE:" + due.UTC().Format("20060102T150405Z") +
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

func isCompleted(t *testing.T, a *app, uid string) bool {
	t.Helper()
	loc, ok := a.store.Locate(uid)
	if !ok {
		t.Fatalf("task %q not found", uid)
	}
	return findTodo(loc.Object, uid).Completed()
}

func todoIndexIn(items []model.AgendaItem) int {
	for i, it := range items {
		if it.IsTodo() {
			return i
		}
	}
	return -1
}

// TestSpaceChecksOffDrilledTaskInMonthGrid: drilling onto a due task in the month
// grid and pressing Space checks it off (rather than toggling calendar visibility).
func TestSpaceChecksOffDrilledTaskInMonthGrid(t *testing.T) {
	when := time.Date(2026, 7, 20, 12, 0, 0, 0, time.UTC)
	a := newRootedTestApp(t, when)
	if err := a.store.CreateCalendarLocal(context.Background(), "tl", store.CalendarMeta{DisplayName: "TL"}, []string{"VTODO"}); err != nil {
		t.Fatal(err)
	}
	uid := putDueTask(t, a, "tl", "PayRent", time.Date(2026, 7, 20, 9, 0, 0, 0, time.Local))
	a.reload()

	a.setMode(modeCalendar)
	a.viewMode = viewMonth
	a.anchor = model.DayStart(when)
	a.buildCenterCalendar()
	a.month.selected = a.anchor

	items := a.month.selectedItems()
	idx := todoIndexIn(items)
	if idx < 0 {
		t.Fatalf("due task not present in the day's items: %+v", items)
	}
	a.month.eventMode = true
	a.month.eventIndex = idx

	if isCompleted(t, a, uid) {
		t.Fatal("precondition: task should start incomplete")
	}
	a.globalKeys(runeKey(' '))
	if !isCompleted(t, a, uid) {
		t.Error("Space on a drilled task in the month grid did not check it off")
	}
	// Completing must not undrill the day.
	if !a.month.eventMode {
		t.Error("completing undrilled the month grid")
	}
	if tt, ok := a.currentTarget(); !ok || !tt.isTodo || tt.uid != uid {
		t.Errorf("after complete, drill target = %+v, want still task %q", tt, uid)
	}
}

// TestSpaceChecksOffDrilledTaskInWeekGrid: same, in the week time-grid (tasks are
// now selectable in its drill).
func TestSpaceChecksOffDrilledTaskInWeekGrid(t *testing.T) {
	when := time.Date(2026, 7, 20, 12, 0, 0, 0, time.UTC)
	a := newRootedTestApp(t, when)
	if err := a.store.CreateCalendarLocal(context.Background(), "tl", store.CalendarMeta{DisplayName: "TL"}, []string{"VTODO"}); err != nil {
		t.Fatal(err)
	}
	uid := putDueTask(t, a, "tl", "PayRent", time.Date(2026, 7, 20, 9, 0, 0, 0, time.Local))
	a.reload()

	a.setMode(modeCalendar)
	a.viewMode = viewWeek
	a.anchor = model.DayStart(when)
	a.buildCenterCalendar()
	a.timegrid.selected = a.anchor

	idx := todoIndexIn(a.timegrid.daySelectables())
	if idx < 0 {
		t.Fatalf("due task not selectable in week drill: %+v", a.timegrid.daySelectables())
	}
	a.timegrid.eventMode = true
	a.timegrid.eventIndex = idx

	a.globalKeys(runeKey(' '))
	if !isCompleted(t, a, uid) {
		t.Error("Space on a drilled task in the week grid did not check it off")
	}
	if !a.timegrid.eventMode {
		t.Error("completing undrilled the week grid")
	}
	if it := a.timegrid.selectedItem(); it == nil || !it.IsTodo() || it.Todo.UID != uid {
		t.Errorf("after complete, week drill lost the task selection (%v)", it)
	}
}

// TestSubtaskUnderSelectedTaskInCalendar: with a task drilled in the calendar
// view, `is` creates a subtask under it, in the parent's own calendar.
func TestSubtaskUnderSelectedTaskInCalendar(t *testing.T) {
	when := time.Date(2026, 7, 20, 12, 0, 0, 0, time.UTC)
	a := newRootedTestApp(t, when)
	if err := a.store.CreateCalendarLocal(context.Background(), "tl", store.CalendarMeta{DisplayName: "TL"}, []string{"VTODO"}); err != nil {
		t.Fatal(err)
	}
	parentUID := putDueTask(t, a, "tl", "Parent", time.Date(2026, 7, 20, 9, 0, 0, 0, time.Local))
	a.reload()

	a.setMode(modeCalendar)
	a.viewMode = viewMonth
	a.anchor = model.DayStart(when)
	a.buildCenterCalendar()
	a.month.selected = a.anchor
	items := a.month.selectedItems()
	a.month.eventMode = true
	a.month.eventIndex = todoIndexIn(items)

	calID, gotParent, ok := a.subtaskContext()
	if !ok {
		t.Fatal("subtaskContext refused with a task drilled in the calendar")
	}
	if calID != "tl" {
		t.Errorf("subtask target calendar = %q, want the parent's list %q", calID, "tl")
	}
	if gotParent != parentUID {
		t.Errorf("subtask parent = %q, want the selected task %q", gotParent, parentUID)
	}
}
