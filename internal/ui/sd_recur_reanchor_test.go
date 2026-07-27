package ui

import (
	"context"
	"testing"
	"time"

	"github.com/littekge/LazyPlanner/internal/model"
	"github.com/littekge/LazyPlanner/internal/store"
)

// Pass-24 MED: `sd` (setDuePrompt -> applyTodoField) moved a recurring todo's
// DUE without re-anchoring a day-pinning RRULE, so the next Space advance snapped
// back to the old weekday and the user's change silently undid itself.
//
// This is the same guardrail grab already satisfied ("Moving a recurring item's
// anchor must re-anchor its day-pinning BY*") reached through a different door —
// the guardrail's sweep listed single and bulk grab and missed the quick-set path.
// The fix lives in applyTodoField, not setDuePrompt, so every field mutation that
// shifts the due is covered.
func TestSdRecurringTodoReanchorsByday(t *testing.T) {
	now := time.Date(2026, 7, 6, 12, 0, 0, 0, time.Local) // Monday
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

	// Confirm the fixture: only DUE, no DTSTART, and a BYDAY=MO rule.
	loc0, _ := a.store.Locate(uid)
	td0 := findTodo(loc0.Object, uid)
	t.Logf("fixture raw: recurring=%v rrule=%v dtstart=%v due=%v",
		td0.Recurring,
		td0.Raw.Props.Get("RRULE"),
		td0.Raw.Props.Get("DTSTART"),
		td0.Raw.Props.Get("DUE"))

	wed := time.Date(2026, 7, 8, 12, 0, 0, 0, time.Local) // Wednesday
	a.applyTodoField(uid, "set due", func(d *model.TodoDraft) {
		d.HasDue, d.Due, d.DueAllDay = true, wed, false
	})

	loc, ok := a.store.Locate(uid)
	if !ok {
		t.Fatal("todo vanished")
	}
	td := findTodo(loc.Object, uid)
	if td == nil || !td.Due.Equal(wed) {
		t.Fatalf("DUE after sd = %v, want %v", td.Due, wed)
	}
	t.Logf("after sd: due=%v rrule=%v", td.Raw.Props.Get("DUE"), td.Raw.Props.Get("RRULE"))

	advanced, done, err := model.AdvanceRecurringTodo(loc.Object, uid, a.now, a.loc)
	if err != nil {
		t.Fatalf("AdvanceRecurringTodo: %v", err)
	}
	if done {
		t.Fatal("series exhausted unexpectedly")
	}
	next := findTodo(advanced, uid)
	t.Logf("next due after Space = %v (%s)", next.Due, next.Due.Weekday())
	if next.Due.Weekday() != time.Wednesday {
		t.Fatalf("BUG CONFIRMED: next due %v is a %s, want Wednesday — sd did not re-anchor BYDAY",
			next.Due, next.Due.Weekday())
	}
}

// The permissive side of the same gate. reanchoredRecurrence reports blocked for
// a rule outside the editable vocabulary whether or not the day moved, so the
// re-anchor must fire only when the calendar DAY actually changes — otherwise
// setting just the time on a "Custom rule (kept)" todo would be refused with
// "can't shift the day", a change that shifts no day at all.
//
// Without the day-change gate this test fails: the set is rejected and the DUE
// keeps its original time.
func TestSdTimeOnlyChangeIsNotBlockedOnCustomRule(t *testing.T) {
	now := time.Date(2026, 7, 6, 9, 0, 0, 0, time.Local) // Monday
	a := newRootedTestApp(t, now)
	a.setMode(modeCalendar)
	cal := testCalID(a)

	// BYSETPOS is outside the editable vocabulary, so RecurSpecFromRule reports
	// not-ok and any re-anchor attempt blocks.
	uid := "custom-rule@t"
	ics := "BEGIN:VCALENDAR\r\nVERSION:2.0\r\nPRODID:-//t//t//EN\r\nBEGIN:VTODO\r\nUID:" + uid +
		"\r\nSUMMARY:water plants\r\nDTSTAMP:20260701T000000Z" +
		"\r\nDUE:20260706T160000Z" +
		"\r\nRRULE:FREQ=MONTHLY;BYDAY=MO,TU,WE,TH,FR;BYSETPOS=-1\r\nEND:VTODO\r\nEND:VCALENDAR\r\n"
	parsed, err := model.Decode([]byte(ics), a.loc)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := a.store.Put(context.Background(), cal, store.ResourceName(uid), parsed); err != nil {
		t.Fatal(err)
	}
	a.reload()

	td := findTodo(mustLocate(t, a, uid).Object, uid)
	if td == nil || !td.Recurring {
		t.Fatal("setup: want a recurring todo")
	}
	sameDayLaterTime := time.Date(2026, 7, 6, 20, 0, 0, 0, a.loc)

	a.applyTodoField(uid, "set due", func(d *model.TodoDraft) {
		d.HasDue, d.Due, d.DueAllDay = true, sameDayLaterTime, false
	})

	got := findTodo(mustLocate(t, a, uid).Object, uid)
	if got == nil {
		t.Fatal("todo vanished")
	}
	if !got.Due.Equal(sameDayLaterTime) {
		t.Errorf("time-only sd on a custom-rule todo was refused: DUE = %s, want %s",
			got.Due.In(a.loc), sameDayLaterTime)
	}
}

func mustLocate(t *testing.T, a *app, uid string) store.Located {
	t.Helper()
	loc, ok := a.store.Locate(uid)
	if !ok {
		t.Fatalf("locate %q failed", uid)
	}
	return loc
}
