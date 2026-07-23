package ui

import (
	"testing"
	"time"

	"github.com/littekge/LazyPlanner/internal/model"
)

func recurringEvent(t *testing.T, rrule string) *model.Event {
	t.Helper()
	ics := "BEGIN:VCALENDAR\r\nVERSION:2.0\r\nPRODID:-//x//x//EN\r\n" +
		"BEGIN:VEVENT\r\nUID:re-1\r\nDTSTAMP:20260101T000000Z\r\n" +
		"DTSTART:20260825T090000Z\r\nDTEND:20260825T100000Z\r\n" +
		"RRULE:" + rrule + "\r\nSUMMARY:base\r\nEND:VEVENT\r\nEND:VCALENDAR\r\n"
	obj, err := model.Decode([]byte(ics), time.UTC)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	return obj.Events[0]
}

// TestEventFormRepeatReadCreate verifies a create form's Repeat dropdown flows a
// picked preset into the read draft, re-deriving the weekday from the start date.
func TestEventFormRepeatReadCreate(t *testing.T) {
	a := newTestApp(t, time.Date(2026, 8, 1, 9, 0, 0, 0, time.UTC))
	base := time.Date(2026, 8, 25, 0, 0, 0, 0, a.loc) // a Tuesday
	_, fields := a.newEventForm(nil, base, a.newEventRepeat(nil, base))

	fields.summary.SetText("Standup")
	fields.allDay.SetChecked(false)
	fields.startDate.SetText("2026-08-25")
	fields.startTime.SetText("09:00")
	fields.repeat.SetCurrentOption(2) // "Weekly on Tue"

	d, err := a.readEventDraft(fields)
	if err != nil {
		t.Fatalf("readEventDraft: %v", err)
	}
	if d.Recur == nil || d.Recur.Freq != model.FreqWeekly {
		t.Fatalf("Recur = %+v, want weekly", d.Recur)
	}
	if len(d.Recur.Weekdays) != 1 || d.Recur.Weekdays[0] != time.Tuesday {
		t.Errorf("Weekdays = %v, want [Tuesday]", d.Recur.Weekdays)
	}
}

// TestEventFormRepeatUntouchedAndRemove verifies seeding an existing rule: leaving
// the selection reads as untouched (nil/false, preserving bytes), and picking None
// reads as a removal.
func TestEventFormRepeatUntouchedAndRemove(t *testing.T) {
	a := newTestApp(t, time.Date(2026, 8, 1, 9, 0, 0, 0, time.UTC))
	ev := recurringEvent(t, "FREQ=WEEKLY;BYDAY=TU")

	// Seeded and left as-is → untouched.
	_, fields := a.newEventForm(ev, ev.Start, a.newEventRepeat(ev, ev.Start))
	fields.startDate.SetText(ev.Start.In(a.loc).Format("2006-01-02"))
	fields.startTime.SetText(ev.Start.In(a.loc).Format("15:04"))
	fields.allDay.SetChecked(false)
	d, err := a.readEventDraft(fields)
	if err != nil {
		t.Fatalf("readEventDraft: %v", err)
	}
	if d.Recur != nil || d.RecurRemove {
		t.Errorf("unchanged rule: got (%+v,%v), want untouched", d.Recur, d.RecurRemove)
	}

	// Pick None → remove.
	fields.repeat.SetCurrentOption(0)
	d, err = a.readEventDraft(fields)
	if err != nil {
		t.Fatalf("readEventDraft: %v", err)
	}
	if d.Recur != nil || !d.RecurRemove {
		t.Errorf("None: got (%+v,%v), want RecurRemove", d.Recur, d.RecurRemove)
	}
}

// TestEventFormRepeatHidden verifies the this-occurrence override edit hides the
// Repeat field (nil choices) so a read never touches the rule.
func TestEventFormRepeatHidden(t *testing.T) {
	a := newTestApp(t, time.Date(2026, 8, 1, 9, 0, 0, 0, time.UTC))
	ev := recurringEvent(t, "FREQ=WEEKLY;BYDAY=TU")
	_, fields := a.newEventForm(ev, ev.Start, nil)
	if fields.repeat != nil || fields.repeatChoices != nil {
		t.Fatal("Repeat field should be hidden for the occurrence edit")
	}
	fields.startDate.SetText(ev.Start.In(a.loc).Format("2006-01-02"))
	fields.startTime.SetText(ev.Start.In(a.loc).Format("15:04"))
	fields.allDay.SetChecked(false)
	d, err := a.readEventDraft(fields)
	if err != nil {
		t.Fatalf("readEventDraft: %v", err)
	}
	if d.Recur != nil || d.RecurRemove {
		t.Errorf("hidden field: got (%+v,%v), want untouched", d.Recur, d.RecurRemove)
	}
}

// TestTodoFormRepeatNeedsDue verifies a repeating task requires a due date to
// anchor its recurrence.
func TestTodoFormRepeatNeedsDue(t *testing.T) {
	a := newTestApp(t, time.Date(2026, 8, 1, 9, 0, 0, 0, time.UTC))
	_, fields := a.newTodoForm(nil, a.newTodoRepeat(nil))
	fields.summary.SetText("Chore")
	fields.repeat.SetCurrentOption(1) // Daily, but no due date set
	if _, err := a.readTodoDraft(fields); err == nil {
		t.Error("expected an error requiring a due date for a repeating task")
	}
}
