package ui

import (
	"context"
	"testing"
	"time"

	"github.com/gdamore/tcell/v2"

	"github.com/littekge/LazyPlanner/internal/model"
	"github.com/littekge/LazyPlanner/internal/store"
)

// TestBulkGrabRecurringTodoReanchorsByday is the repro for the reported defect:
// bulk-grabbing a recurring todo whose RRULE pins a weekday (FREQ=WEEKLY;BYDAY=MO)
// and shifting its DUE by a day must move the series anchor onto the new day —
// not leave BYDAY=MO contradicting a Tuesday DUE. A LazyPlanner-native recurring
// todo carries only DUE (applyTodo never writes DTSTART), so DUE IS the recurrence
// anchor; shifting it without re-anchoring BYDAY makes AdvanceRecurringTodo compute
// the next occurrence from the stale rule.
func TestBulkGrabRecurringTodoReanchorsByday(t *testing.T) {
	now := time.Date(2026, 7, 6, 9, 30, 0, 0, time.UTC) // Monday
	a := newRootedTestApp(t, now)
	a.setMode(modeCalendar)
	cal := testCalID(a)

	// Native recurring todo: DUE only (no DTSTART), FREQ=WEEKLY;BYDAY=MO.
	obj := model.NewTodoObject(model.TodoDraft{
		Summary: "water plants",
		HasDue:  true,
		Due:     now,
		Recur:   &model.RecurSpec{Freq: model.FreqWeekly, Weekdays: []time.Weekday{time.Monday}},
	}, a.now)
	uid := obj.Todos[0].UID
	if _, err := a.store.Put(context.Background(), cal, store.ResourceName(uid), obj); err != nil {
		t.Fatal(err)
	}
	a.refresh("")
	a.month.reDrill(model.DayStart(now), 0)
	a.setFocus(a.calendarPrimitive())
	a.enterSelect()
	a.startBulkGrab()
	if !a.grabbing || len(a.bulkGrab) != 1 {
		t.Fatalf("grabbing=%v n=%d, want true/1", a.grabbing, len(a.bulkGrab))
	}

	// j = +1 day for a task (matches single-item grab's axis). DUE Mon -> Tue.
	a.handleBulkGrabKey(tcell.NewEventKey(tcell.KeyRune, 'j', tcell.ModNone))
	a.handleBulkGrabKey(tcell.NewEventKey(tcell.KeyEnter, 0, tcell.ModNone))

	loc, ok := a.store.Locate(uid)
	if !ok {
		t.Fatal("todo vanished after bulk grab")
	}
	td := findTodo(loc.Object, uid)
	wantDue := now.AddDate(0, 0, 1) // Tuesday 2026-07-07
	if td == nil || !td.Due.Equal(wantDue) {
		t.Fatalf("DUE after grab = %v, want %v", td.Due, wantDue)
	}

	// Now complete-and-advance the series. The user moved DUE to a Tuesday, so
	// they expect the next occurrence a week later, on the following Tuesday.
	advanced, done, err := model.AdvanceRecurringTodo(loc.Object, uid, a.now, a.loc)
	if err != nil {
		t.Fatalf("AdvanceRecurringTodo: %v", err)
	}
	if done {
		t.Fatal("series reported exhausted, want a next occurrence")
	}
	nextTd := findTodo(advanced, uid)
	if nextTd == nil {
		t.Fatal("advanced object has no todo")
	}
	wantNext := time.Date(2026, 7, 14, 9, 30, 0, 0, time.UTC) // next Tuesday
	if !nextTd.Due.Equal(wantNext) {
		t.Fatalf("next DUE after advance = %v (%s), want %v (%s) — BYDAY=MO was not re-anchored, so the series snapped back to Monday",
			nextTd.Due, nextTd.Due.Weekday(), wantNext, wantNext.Weekday())
	}
}
