package ui

import (
	"context"
	"testing"
	"time"

	"github.com/littekge/LazyPlanner/internal/model"
	"github.com/littekge/LazyPlanner/internal/store"
)

// completeTask marks an existing task complete directly through the store (no
// stickyDone side effect), for tests that want a plainly-completed task.
func completeTask(t *testing.T, a *app, uid string) {
	t.Helper()
	loc, ok := a.store.Locate(uid)
	if !ok {
		t.Fatalf("task %q not found", uid)
	}
	newObj, err := model.SetTodoCompleted(loc.Object, uid, true, a.now, a.loc)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := a.store.Put(context.Background(), loc.CalID, loc.Name, newObj); err != nil {
		t.Fatal(err)
	}
}

// TestHideCompletedAppliesToCalendarAndAgenda locks H1's sibling H2: the `.`
// (show/hide completed) toggle must hide/show completed due tasks in the calendar
// and agenda, not only the tree — previously it was tree-only, so a completed task
// showed permanently on the month/week/day grids and agenda.
func TestHideCompletedAppliesToCalendarAndAgenda(t *testing.T) {
	when := time.Date(2026, 7, 20, 12, 0, 0, 0, time.UTC)
	a := newRootedTestApp(t, when)
	if err := a.store.CreateCalendarLocal(context.Background(), "tl", store.CalendarMeta{DisplayName: "TL"}, []string{"VTODO"}); err != nil {
		t.Fatal(err)
	}
	uid := putDueTask(t, a, "tl", "PayRent", time.Date(2026, 7, 20, 9, 0, 0, 0, time.Local))
	completeTask(t, a, uid)
	a.reload()

	day := model.DayStart(when)
	inAgenda := func() bool {
		for _, it := range a.dayItems(day) {
			if it.IsTodo() && it.Todo.UID == uid {
				return true
			}
		}
		return false
	}
	inTimeGrid := func() bool {
		for _, td := range a.dueTasksByDay([]time.Time{day})[dayKey(day)] {
			if td.UID == uid {
				return true
			}
		}
		return false
	}

	// Completed shown: the task appears in both calendar/agenda data builders.
	a.showCompleted = true
	a.stickyDone = map[string]bool{}
	if !inAgenda() || !inTimeGrid() {
		t.Errorf("with completed shown: agenda=%v timegrid=%v, want both true", inAgenda(), inTimeGrid())
	}

	// Completed hidden: the task disappears from both (the H2 fix).
	a.showCompleted = false
	a.stickyDone = map[string]bool{}
	if inAgenda() || inTimeGrid() {
		t.Errorf("with completed hidden: agenda=%v timegrid=%v, want both false (H2)", inAgenda(), inTimeGrid())
	}

	// A just-completed sticky pin keeps it visible even while hidden (F-sticky),
	// matching the tree's keep-visible-until-you-leave behavior.
	a.stickyDone = map[string]bool{uid: true}
	if !inAgenda() || !inTimeGrid() {
		t.Errorf("sticky-pinned completed task: agenda=%v timegrid=%v, want both true", inAgenda(), inTimeGrid())
	}
}

// TestCompleteWhileHiddenPinsStickyInAnyView locks F-sticky: completing a task via
// Space while completed are hidden pins it (stickyDone) regardless of view, so it
// doesn't vanish instantly from the calendar/agenda.
func TestCompleteWhileHiddenPinsStickyInAnyView(t *testing.T) {
	when := time.Date(2026, 7, 20, 12, 0, 0, 0, time.UTC)
	a := newRootedTestApp(t, when)
	if err := a.store.CreateCalendarLocal(context.Background(), "tl", store.CalendarMeta{DisplayName: "TL"}, []string{"VTODO"}); err != nil {
		t.Fatal(err)
	}
	uid := putDueTask(t, a, "tl", "PayRent", time.Date(2026, 7, 20, 9, 0, 0, 0, time.Local))
	a.reload()
	a.showCompleted = false
	a.stickyDone = map[string]bool{}

	// Drill onto the task in the agenda and check it off.
	a.setMode(modeAgenda)
	a.buildAgendaLeft()
	items := a.dayItems(model.DayStart(when))
	idx := todoIndexIn(items)
	if idx < 0 {
		t.Fatal("due task not present in today's agenda")
	}
	a.agendaList.SetCurrentItem(idx)
	a.agenda.setData(model.DayStart(when), items)
	a.agenda.setSelected(idx)
	a.toggleComplete()

	if !a.stickyDone[uid] {
		t.Error("completing a task in the agenda while hidden should pin it (stickyDone) — F-sticky")
	}
}
