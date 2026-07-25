package ui

import (
	"context"
	"testing"
	"time"

	"github.com/littekge/LazyPlanner/internal/model"
	"github.com/littekge/LazyPlanner/internal/store"
)

// TestSingleGrabRecurringTodoReanchorsByday guards the single-item-grab twin of the
// bulk-grab reanchor fix (Pass-20 LOW): grabbing a recurring todo whose RRULE pins a
// weekday (FREQ=WEEKLY;BYDAY=MO) and nudging its DUE +1 day must move the series
// anchor onto the new day, not leave BYDAY=MO contradicting a Tuesday DUE and snap
// back to Monday on the next advance. A native recurring todo carries only DUE, so
// DUE is the recurrence anchor.
func TestSingleGrabRecurringTodoReanchorsByday(t *testing.T) {
	now := time.Date(2026, 7, 6, 9, 30, 0, 0, time.UTC) // Monday
	a := newRootedTestApp(t, now)
	a.setMode(modeCalendar)
	cal := testCalID(a)

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
	a.reload()

	loc, ok := a.store.Locate(uid)
	if !ok {
		t.Fatal("locate failed")
	}
	a.beginGrab(loc, editTarget{isTodo: true, uid: uid, occStart: now, recurring: true}, scopeAll)
	if !a.grabbing {
		t.Fatal("did not enter grab mode")
	}
	a.grabNudge('j') // +1 day for a task → Tuesday
	a.commitGrab()

	loc, ok = a.store.Locate(uid)
	if !ok {
		t.Fatal("todo vanished after grab")
	}
	td := findTodo(loc.Object, uid)
	wantDue := now.AddDate(0, 0, 1) // Tuesday 2026-07-07
	if td == nil || !td.Due.Equal(wantDue) {
		t.Fatalf("DUE after grab = %v, want %v", td.Due, wantDue)
	}

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
		t.Fatalf("next DUE after advance = %v (%s), want %v (%s) — BYDAY=MO was not re-anchored on single-item grab",
			nextTd.Due, nextTd.Due.Weekday(), wantNext, wantNext.Weekday())
	}
}
