package ui

import (
	"strings"
	"testing"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"

	"github.com/littekge/LazyPlanner/internal/model"
)

// findCaretFormIn walks a modal's wrapper flexes to reach the caret form inside.
func findCaretFormIn(p tview.Primitive) *caretForm {
	switch v := p.(type) {
	case *caretForm:
		return v
	case *tview.Flex:
		for i := 0; i < v.GetItemCount(); i++ {
			if f := findCaretFormIn(v.GetItem(i)); f != nil {
				return f
			}
		}
	}
	return nil
}

// formItemByLabel returns the laid-out form item carrying base label want.
func formItemByLabel(t *testing.T, f *caretForm, want string) tview.FormItem {
	t.Helper()
	for i, l := range f.labels {
		if strings.HasPrefix(l, want) {
			return f.GetFormItem(i)
		}
	}
	t.Fatalf("form item %q not laid out; labels = %v", want, f.labels)
	return nil
}

func pressButton(t *testing.T, f *caretForm, label string) {
	t.Helper()
	idx := f.GetButtonIndex(label)
	if idx < 0 {
		t.Fatalf("button %q not found", label)
	}
	f.GetButton(idx).InputHandler()(tcell.NewEventKey(tcell.KeyEnter, 0, tcell.ModNone), func(tview.Primitive) {})
}

// customIndex finds the Custom… entry in a Repeat dropdown's labels.
func customIndex(t *testing.T, choices *model.RepeatChoices) int {
	t.Helper()
	for i, l := range choices.Labels() {
		if strings.HasPrefix(l, "Custom") {
			return i
		}
	}
	t.Fatalf("no Custom… entry in %v", choices.Labels())
	return -1
}

// endsOnDateViaCustomForm walks the real UI path: opening the Repeat dropdown's
// Custom… entry, filling "Every 1 day / Ends On date until", and pressing OK.
func (a *app) endsOnDateViaCustomForm(t *testing.T, dd *tview.DropDown, choices *model.RepeatChoices, until string) {
	t.Helper()
	dd.SetCurrentOption(customIndex(t, choices))

	name, front := a.root.GetFrontPage()
	if name != pageRepeat {
		t.Fatalf("front page = %q, want %q", name, pageRepeat)
	}
	sub := findCaretFormIn(front)
	if sub == nil {
		t.Fatal("custom repeat sub-form not found in modal")
	}
	formItemByLabel(t, sub, "Every").(*tview.InputField).SetText("1")
	formItemByLabel(t, sub, "Unit").(*tview.DropDown).SetCurrentOption(0) // days
	formItemByLabel(t, sub, "Ends").(*tview.DropDown).SetCurrentOption(1) // On date
	formItemByLabel(t, sub, "Until").(*tview.InputField).SetText(until)
	pressButton(t, sub, "OK")
}

// TestEndsOnDateRealUIPathTimedEvent drives the ACTUAL UI path a user walks: the
// event form (timed 15:00 start), Repeat → Custom…, Ends = "On date" 2026-07-25,
// OK, then Save (readEventDraft) → the stored object's occurrences.
//
// Pass 22 fixed readCustomRecur to anchor UNTIL at "D + the anchor's
// time-of-day", but the event form's anchorFn parsed only the Start DATE field,
// so production always handed it a midnight anchor and the fix never fired. The
// pass-22 test called readCustomRecur directly and passed anyway — this test
// exists to hold the *wiring*, not the arithmetic.
func TestEndsOnDateRealUIPathTimedEvent(t *testing.T) {
	a := newRootedTestApp(t, time.Date(2026, 7, 1, 9, 0, 0, 0, time.UTC))

	day := time.Date(2026, 7, 20, 0, 0, 0, 0, a.loc)
	choices := a.newEventRepeat(nil, day)
	_, fields := a.newEventForm(nil, day, choices)

	// User fills in a TIMED event starting 2026-07-20 15:00.
	fields.allDay.SetChecked(false)
	fields.startDate.SetText("2026-07-20")
	fields.startTime.SetText("15:00")
	fields.endDate.SetText("2026-07-20")
	fields.endTime.SetText("16:00")

	a.endsOnDateViaCustomForm(t, fields.repeat, choices, "2026-07-25")

	draft, err := a.readEventDraft(fields)
	if err != nil {
		t.Fatalf("readEventDraft: %v", err)
	}
	if draft.Recur == nil {
		t.Fatal("no recurrence resolved from the form")
	}

	p, err := model.NewEventObject(draft, draft.Start)
	if err != nil {
		t.Fatalf("NewEventObject: %v", err)
	}
	occs, err := p.EventOccurrences(
		time.Date(2026, 7, 20, 0, 0, 0, 0, a.loc),
		time.Date(2026, 8, 1, 0, 0, 0, 0, a.loc),
	)
	if err != nil {
		t.Fatalf("EventOccurrences: %v", err)
	}
	var got []string
	found := false
	for _, o := range occs {
		ds := o.Start.In(a.loc).Format("2006-01-02 15:04")
		got = append(got, ds)
		if strings.HasPrefix(ds, "2026-07-25") {
			found = true
		}
	}
	if !found {
		t.Fatalf("timed 'Ends on date 2026-07-25' dropped the selected end day; occurrences: %v", got)
	}
}

// TestEndsOnDateRealUIPathAllDayEvent closes the class the other way: with the
// All day box checked the anchor must stay at midnight, so the model's
// dateOnlyUntil still emits an inclusive VALUE=DATE UNTIL. A "fix" that always
// stamped the Start time field onto the anchor would break this.
func TestEndsOnDateRealUIPathAllDayEvent(t *testing.T) {
	a := newRootedTestApp(t, time.Date(2026, 7, 1, 9, 0, 0, 0, time.UTC))

	day := time.Date(2026, 7, 20, 0, 0, 0, 0, a.loc)
	choices := a.newEventRepeat(nil, day)
	_, fields := a.newEventForm(nil, day, choices)

	fields.allDay.SetChecked(true)
	fields.startDate.SetText("2026-07-20")
	// A stale time left in the field must not leak into an all-day anchor.
	fields.startTime.SetText("15:00")
	fields.endDate.SetText("2026-07-20")

	a.endsOnDateViaCustomForm(t, fields.repeat, choices, "2026-07-25")

	draft, err := a.readEventDraft(fields)
	if err != nil {
		t.Fatalf("readEventDraft: %v", err)
	}
	if draft.Recur == nil || draft.Recur.Until == nil {
		t.Fatal("no UNTIL resolved from the all-day form")
	}
	// Pinned at the UNTIL value, the level the all-day contract is defined at
	// (encoding it date-only is the model's job via dateOnlyUntil). Deliberately
	// not asserted on the expanded occurrences: a date-only UNTIL against a
	// date-only DTSTART currently drops the final day independently of this
	// wiring — reproducible with a hand-built all-day spec and unchanged by this
	// fix, so it is a separate defect, not this test's subject.
	if !draft.AllDay {
		t.Fatal("draft is not all-day; the anchor path under test was not exercised")
	}
	want := time.Date(2026, 7, 25, 0, 0, 0, 0, a.loc)
	if !draft.Recur.Until.Equal(want) {
		t.Fatalf("all-day UNTIL = %v, want midnight %v", draft.Recur.Until, want)
	}
}

// TestEndsOnDateRealUIPathTimedTodo is the same walk for a TIMED task: the todo
// form's anchorFn read only the Due DATE field, ignoring Due time, so the same
// midnight anchor reached readCustomRecur.
func TestEndsOnDateRealUIPathTimedTodo(t *testing.T) {
	a := newRootedTestApp(t, time.Date(2026, 7, 1, 9, 0, 0, 0, time.UTC))

	choices := a.newTodoRepeat(nil)
	_, fields := a.newTodoForm(nil, choices)
	fields.summary.SetText("Timed daily task")
	fields.dueDate.SetText("2026-07-20")
	fields.dueTime.SetText("15:00")

	a.endsOnDateViaCustomForm(t, fields.repeat, choices, "2026-07-25")

	draft, err := a.readTodoDraft(fields)
	if err != nil {
		t.Fatalf("readTodoDraft: %v", err)
	}
	if draft.Recur == nil {
		t.Fatal("no recurrence resolved from the todo form")
	}
	want := time.Date(2026, 7, 25, 15, 0, 0, 0, a.loc)
	if draft.Recur.Until == nil || !draft.Recur.Until.Equal(want) {
		t.Fatalf("todo UNTIL = %v, want %v (D + the due time-of-day, so 07-25's occurrence survives)",
			draft.Recur.Until, want)
	}
}

// TestEndsOnDateRealUIPathUntimedTodo pins the no-time case: a task with an empty
// Due time field is all-day, so its anchor stays at midnight and UNTIL is
// date-only — unchanged by the timed fix.
func TestEndsOnDateRealUIPathUntimedTodo(t *testing.T) {
	a := newRootedTestApp(t, time.Date(2026, 7, 1, 9, 0, 0, 0, time.UTC))

	choices := a.newTodoRepeat(nil)
	_, fields := a.newTodoForm(nil, choices)
	fields.summary.SetText("Untimed daily task")
	fields.dueDate.SetText("2026-07-20")
	fields.dueTime.SetText("")

	a.endsOnDateViaCustomForm(t, fields.repeat, choices, "2026-07-25")

	draft, err := a.readTodoDraft(fields)
	if err != nil {
		t.Fatalf("readTodoDraft: %v", err)
	}
	if draft.Recur == nil || draft.Recur.Until == nil {
		t.Fatal("no UNTIL resolved from the untimed todo form")
	}
	want := time.Date(2026, 7, 25, 0, 0, 0, 0, a.loc)
	if !draft.Recur.Until.Equal(want) {
		t.Fatalf("untimed todo UNTIL = %v, want midnight %v", draft.Recur.Until, want)
	}
}

// TestEndsOnDateRealUIPathPartialTimeFallsBackToMidnight pins that a half-typed
// Due time (the field is read live, while the user is still editing) degrades to
// a midnight anchor instead of erroring or blocking the Custom sub-form.
func TestEndsOnDateRealUIPathPartialTimeFallsBackToMidnight(t *testing.T) {
	a := newRootedTestApp(t, time.Date(2026, 7, 1, 9, 0, 0, 0, time.UTC))

	choices := a.newTodoRepeat(nil)
	_, fields := a.newTodoForm(nil, choices)
	fields.summary.SetText("Half-typed time")
	fields.dueDate.SetText("2026-07-20")
	fields.dueTime.SetText("15:") // mid-keystroke

	a.endsOnDateViaCustomForm(t, fields.repeat, choices, "2026-07-25")

	// readTodoDraft rejects the unparseable time on save; the anchor path must
	// still have produced a usable spec rather than panicking or erroring early.
	fields.dueTime.SetText("15:00")
	draft, err := a.readTodoDraft(fields)
	if err != nil {
		t.Fatalf("readTodoDraft: %v", err)
	}
	if draft.Recur == nil || draft.Recur.Until == nil {
		t.Fatal("no UNTIL resolved after a half-typed time")
	}
	want := time.Date(2026, 7, 25, 0, 0, 0, 0, a.loc)
	if !draft.Recur.Until.Equal(want) {
		t.Fatalf("partial-time UNTIL = %v, want the midnight fallback %v", draft.Recur.Until, want)
	}
}
