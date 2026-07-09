package ui

import (
	"context"
	"strings"
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

// putChildTask writes an incomplete VTODO whose RELATED-TO parent is parentUID.
func putChildTask(t *testing.T, a *app, calID, summary, parentUID string) {
	t.Helper()
	uid := summary + "@child"
	ics := "BEGIN:VCALENDAR\r\nVERSION:2.0\r\nPRODID:-//t//t//EN\r\nBEGIN:VTODO\r\nUID:" + uid +
		"\r\nSUMMARY:" + summary + "\r\nDTSTAMP:20260701T000000Z\r\nRELATED-TO:" + parentUID +
		"\r\nEND:VTODO\r\nEND:VCALENDAR\r\n"
	parsed, err := model.Decode([]byte(ics), time.Local)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := a.store.Put(context.Background(), calID, store.ResourceName(uid), parsed); err != nil {
		t.Fatal(err)
	}
}

// TestFolderCaretInCalendarViews: a dated task that gains an incomplete child
// becomes a folder and renders with a ▸ caret (not [ ]) in the month grid and
// agenda, matching the tree — and still appears on the calendar (keeps its due).
func TestFolderCaretInCalendarViews(t *testing.T) {
	when := time.Date(2026, 7, 20, 12, 0, 0, 0, time.UTC)
	a := newRootedTestApp(t, when)
	if err := a.store.CreateCalendarLocal(context.Background(), "tl", store.CalendarMeta{DisplayName: "TL"}, []string{"VTODO"}); err != nil {
		t.Fatal(err)
	}
	parent := putDueTask(t, a, "tl", "Proj", time.Date(2026, 7, 20, 9, 0, 0, 0, time.Local))
	putChildTask(t, a, "tl", "Step", parent)
	a.reload()

	if !a.isFolder(parent) {
		t.Fatal("a dated task with an incomplete child should be a folder")
	}

	// Find the parent's agenda item for the day.
	var it model.AgendaItem
	for _, x := range a.dayItems(model.DayStart(when)) {
		if x.IsTodo() && x.Todo.UID == parent {
			it = x
		}
	}
	if it.Todo == nil {
		t.Fatal("dated folder missing from the day's items — it should still appear on the calendar")
	}
	if got := itemLabel(it, a.isFolder(parent)); !strings.HasPrefix(got, "▸ ") {
		t.Errorf("month-cell label = %q, want a ▸ folder caret", got)
	}
	if got := a.agendaLeftLabel(it); !strings.Contains(got, "▸ ") {
		t.Errorf("agenda label = %q, want a ▸ folder caret", got)
	}
	if got := taskMarkerLabel(it.Todo, a.isFolder(parent)); !strings.HasPrefix(got, "▸ ") {
		t.Errorf("week/day marker = %q, want a ▸ folder caret", got)
	}
}

// TestFBStaysDrilled: pressing f/b while drilled changes the period and re-enters
// the drill on the new day (per the "stay drilled" decision).
func TestFBStaysDrilled(t *testing.T) {
	day1 := time.Date(2026, 7, 20, 12, 0, 0, 0, time.UTC)
	a := newRootedTestApp(t, day1)
	if err := a.store.CreateCalendarLocal(context.Background(), "tl", store.CalendarMeta{DisplayName: "TL"}, []string{"VTODO"}); err != nil {
		t.Fatal(err)
	}
	putDueTask(t, a, "tl", "D1", time.Date(2026, 7, 20, 9, 0, 0, 0, time.Local))
	next := putDueTask(t, a, "tl", "D2", time.Date(2026, 7, 21, 9, 0, 0, 0, time.Local))
	a.reload()

	a.setMode(modeCalendar)
	a.viewMode = viewDay
	a.anchor = model.DayStart(day1)
	a.buildCenterCalendar()
	a.timegrid.eventMode = true // drilled on day1's task

	a.globalKeys(runeKey('f')) // → next day, should stay drilled
	if !a.timegrid.eventMode {
		t.Fatal("f un-drilled the view; expected to stay drilled")
	}
	if it := a.timegrid.selectedItem(); it == nil || it.Todo == nil || it.Todo.UID != next {
		t.Errorf("after f, drill = %v, want day 2's task %q", it, next)
	}
}
