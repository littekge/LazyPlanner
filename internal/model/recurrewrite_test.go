package model

import (
	"strings"
	"testing"
	"time"

	"github.com/emersion/go-ical"
)

// recurringEventWithOverrides builds a weekly event (from Tue Jan 6 2026) with an
// EXDATE, a VALARM, an X- property, and two RECURRENCE-ID overrides (Jan 13 and
// Jan 20). Used to exercise the rule-rewrite reconciliation.
func recurringEventWithOverrides(t *testing.T) *Parsed {
	t.Helper()
	ics := "BEGIN:VCALENDAR\r\nVERSION:2.0\r\nPRODID:-//x//x//EN\r\n" +
		"BEGIN:VEVENT\r\nUID:rw-1\r\nDTSTAMP:20260101T000000Z\r\n" +
		"DTSTART:20260106T090000Z\r\nDTEND:20260106T100000Z\r\n" +
		"RRULE:FREQ=WEEKLY\r\nEXDATE:20260127T090000Z\r\nX-FOO:bar\r\nSUMMARY:base\r\n" +
		"BEGIN:VALARM\r\nACTION:DISPLAY\r\nTRIGGER:-PT15M\r\nEND:VALARM\r\nEND:VEVENT\r\n" +
		"BEGIN:VEVENT\r\nUID:rw-1\r\nDTSTAMP:20260101T000000Z\r\nRECURRENCE-ID:20260113T090000Z\r\n" +
		"DTSTART:20260113T090000Z\r\nDTEND:20260113T100000Z\r\nSUMMARY:custom13\r\nEND:VEVENT\r\n" +
		"BEGIN:VEVENT\r\nUID:rw-1\r\nDTSTAMP:20260101T000000Z\r\nRECURRENCE-ID:20260120T090000Z\r\n" +
		"DTSTART:20260120T090000Z\r\nDTEND:20260120T100000Z\r\nSUMMARY:custom20\r\nEND:VEVENT\r\n" +
		"END:VCALENDAR\r\n"
	obj, err := Decode([]byte(ics), time.UTC)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	return obj
}

func overrideCount(cal *ical.Calendar, uid string) int {
	n := 0
	for _, c := range cal.Children {
		if text(c.Props, ical.PropUID) == uid && c.Props.Get(ical.PropRecurrenceID) != nil {
			n++
		}
	}
	return n
}

// TestRewriteEventRuleChange rewrites a recurring event's rule at scope All and
// verifies: the new RRULE is set, EXDATEs and unmodeled props (X-, VALARM)
// survive (iron rule), a still-valid override is kept, an orphaned override is
// dropped, and the dropped count is reported.
func TestRewriteEventRuleChange(t *testing.T) {
	obj := recurringEventWithOverrides(t)
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	d := EventDraft{
		Summary: "base",
		Start:   time.Date(2026, 1, 6, 9, 0, 0, 0, time.UTC),
		End:     time.Date(2026, 1, 6, 10, 0, 0, 0, time.UTC),
		Recur:   &RecurSpec{Freq: FreqWeekly, Interval: 2}, // Jan 6, 20, Feb 3 — Jan 13 no longer occurs
	}
	out, dropped, err := RewriteEventRule(obj, "rw-1", d, now, time.UTC)
	if err != nil {
		t.Fatalf("RewriteEventRule: %v", err)
	}
	if dropped != 1 {
		t.Errorf("dropped = %d, want 1 (the orphaned Jan 13 override)", dropped)
	}
	master := masterComponent(out.Calendar, "rw-1")
	if master == nil {
		t.Fatal("master gone")
	}
	if got := master.Props.Get(ical.PropRecurrenceRule).Value; got != "FREQ=WEEKLY;INTERVAL=2" {
		t.Errorf("RRULE = %q, want FREQ=WEEKLY;INTERVAL=2", got)
	}
	if len(master.Props.Values(ical.PropExceptionDates)) != 1 {
		t.Error("EXDATE not preserved on a rule change")
	}
	if got := text(master.Props, "X-FOO"); got != "bar" {
		t.Errorf("X-FOO = %q, want bar (iron rule)", got)
	}
	if len(master.Children) != 1 {
		t.Errorf("VALARM child lost: %d children", len(master.Children))
	}
	if n := overrideCount(out.Calendar, "rw-1"); n != 1 {
		t.Fatalf("override count = %d, want 1 (Jan 20 kept, Jan 13 dropped)", n)
	}
	// The surviving override must be the Jan 20 one.
	for _, c := range out.Calendar.Children {
		if c.Props.Get(ical.PropRecurrenceID) == nil {
			continue
		}
		if got := text(c.Props, ical.PropSummary); got != "custom20" {
			t.Errorf("surviving override SUMMARY = %q, want custom20", got)
		}
	}
}

// TestRewriteEventRuleRemove implements Repeat → None: the RRULE, EXDATE, RDATE,
// and every override are removed, leaving one plain event.
func TestRewriteEventRuleRemove(t *testing.T) {
	obj := recurringEventWithOverrides(t)
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	d := EventDraft{
		Summary:     "base",
		Start:       time.Date(2026, 1, 6, 9, 0, 0, 0, time.UTC),
		End:         time.Date(2026, 1, 6, 10, 0, 0, 0, time.UTC),
		RecurRemove: true,
	}
	out, dropped, err := RewriteEventRule(obj, "rw-1", d, now, time.UTC)
	if err != nil {
		t.Fatalf("RewriteEventRule: %v", err)
	}
	if dropped != 2 {
		t.Errorf("dropped = %d, want 2 (both overrides)", dropped)
	}
	master := masterComponent(out.Calendar, "rw-1")
	if master == nil {
		t.Fatal("master gone")
	}
	if master.Props.Get(ical.PropRecurrenceRule) != nil {
		t.Error("RRULE not removed")
	}
	if len(master.Props.Values(ical.PropExceptionDates)) != 0 {
		t.Error("EXDATE not removed on Repeat→None")
	}
	if overrideCount(out.Calendar, "rw-1") != 0 {
		t.Error("overrides not removed on Repeat→None")
	}
	var vevents int
	for _, c := range out.Calendar.Children {
		if c.Name == ical.CompEvent {
			vevents++
		}
	}
	if vevents != 1 {
		t.Errorf("VEVENT count = %d, want 1 plain event", vevents)
	}
	// X-FOO / VALARM still preserved on the surviving plain event (iron rule).
	if text(master.Props, "X-FOO") != "bar" || len(master.Children) != 1 {
		t.Error("unmodeled data lost on Repeat→None")
	}
}

// TestRewriteAllDayUntilDateOnly verifies an all-day series rewritten with an
// UNTIL end gets a DATE-only UNTIL (RFC 5545 requires UNTIL's value type to match
// a VALUE=DATE DTSTART).
func TestRewriteAllDayUntilDateOnly(t *testing.T) {
	ics := "BEGIN:VCALENDAR\r\nVERSION:2.0\r\nPRODID:-//x//x//EN\r\n" +
		"BEGIN:VEVENT\r\nUID:ad-1\r\nDTSTAMP:20260101T000000Z\r\n" +
		"DTSTART;VALUE=DATE:20260106\r\nDTEND;VALUE=DATE:20260107\r\n" +
		"RRULE:FREQ=WEEKLY\r\nSUMMARY:allday\r\nEND:VEVENT\r\nEND:VCALENDAR\r\n"
	obj, err := Decode([]byte(ics), time.UTC)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	until := time.Date(2026, 1, 27, 0, 0, 0, 0, time.UTC)
	d := EventDraft{
		Summary: "allday",
		Start:   time.Date(2026, 1, 6, 0, 0, 0, 0, time.UTC),
		End:     time.Date(2026, 1, 7, 0, 0, 0, 0, time.UTC),
		AllDay:  true,
		Recur:   &RecurSpec{Freq: FreqWeekly, Until: &until},
	}
	out, _, err := RewriteEventRule(obj, "ad-1", d, time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), time.UTC)
	if err != nil {
		t.Fatalf("RewriteEventRule: %v", err)
	}
	rule := masterComponent(out.Calendar, "ad-1").Props.Get(ical.PropRecurrenceRule).Value
	if !strings.Contains(rule, "UNTIL=20260127") || strings.Contains(rule, "UNTIL=20260127T") {
		t.Errorf("RRULE = %q, want a DATE-only UNTIL=20260127", rule)
	}
}

// TestEditTodoRewriteAndRemove drives a recurring todo's rule change and removal
// through EditTodo (todos have no RECURRENCE-ID overrides, so no reconciliation).
func TestEditTodoRewriteAndRemove(t *testing.T) {
	ics := "BEGIN:VCALENDAR\r\nVERSION:2.0\r\nPRODID:-//x//x//EN\r\n" +
		"BEGIN:VTODO\r\nUID:td-1\r\nDTSTAMP:20260101T000000Z\r\n" +
		"DUE:20260106T090000Z\r\nRRULE:FREQ=WEEKLY\r\nSUMMARY:chore\r\nEND:VTODO\r\nEND:VCALENDAR\r\n"
	obj, err := Decode([]byte(ics), time.UTC)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	base := TodoDraft{Summary: "chore", HasDue: true, Due: time.Date(2026, 1, 6, 9, 0, 0, 0, time.UTC)}

	changed := base
	changed.Recur = &RecurSpec{Freq: FreqDaily}
	out, err := EditTodo(obj, "td-1", changed, now, time.UTC)
	if err != nil {
		t.Fatalf("EditTodo change: %v", err)
	}
	if got := findComponent(out.Calendar, "td-1").Props.Get(ical.PropRecurrenceRule).Value; got != "FREQ=DAILY" {
		t.Errorf("todo RRULE = %q, want FREQ=DAILY", got)
	}

	removed := base
	removed.RecurRemove = true
	out2, err := EditTodo(obj, "td-1", removed, now, time.UTC)
	if err != nil {
		t.Fatalf("EditTodo remove: %v", err)
	}
	if findComponent(out2.Calendar, "td-1").Props.Get(ical.PropRecurrenceRule) != nil {
		t.Error("todo RRULE not removed on Repeat→None")
	}
	if out2.Todos[0].Recurring {
		t.Error("todo still flagged recurring after Repeat→None")
	}
}

// TestSplitEventWithNewRule verifies the this-and-future split takes a new rule
// from the draft (overwriting the COUNT-rebalance), and preserves the rebalance
// when the draft carries no rule.
func TestSplitEventWithNewRule(t *testing.T) {
	ics := "BEGIN:VCALENDAR\r\nVERSION:2.0\r\nPRODID:-//x//x//EN\r\n" +
		"BEGIN:VEVENT\r\nUID:sp-1\r\nDTSTAMP:20260101T000000Z\r\n" +
		"DTSTART:20260106T090000Z\r\nDTEND:20260106T100000Z\r\n" +
		"RRULE:FREQ=WEEKLY;COUNT=10\r\nSUMMARY:base\r\nEND:VEVENT\r\nEND:VCALENDAR\r\n"
	obj, err := Decode([]byte(ics), time.UTC)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	occ := time.Date(2026, 1, 20, 9, 0, 0, 0, time.UTC) // Jan 6, 13 stay with the past → 2 consumed

	// New rule: future series becomes daily with no count (its own end = never).
	dNew := EventDraft{
		Summary: "fut",
		Start:   occ,
		End:     occ.Add(time.Hour),
		Recur:   &RecurSpec{Freq: FreqDaily},
	}
	_, future, err := SplitEvent(obj, "sp-1", occ, dNew, now, time.UTC)
	if err != nil {
		t.Fatalf("split new rule: %v", err)
	}
	if got := future.Events[0].Raw.Props.Get(ical.PropRecurrenceRule).Value; got != "FREQ=DAILY" {
		t.Errorf("future RRULE = %q, want FREQ=DAILY (new rule)", got)
	}

	// No rule change: the future series keeps the COUNT-rebalanced weekly rule.
	dKeep := EventDraft{Summary: "fut", Start: occ, End: occ.Add(time.Hour)}
	_, future2, err := SplitEvent(obj, "sp-1", occ, dKeep, now, time.UTC)
	if err != nil {
		t.Fatalf("split keep rule: %v", err)
	}
	if got := future2.Events[0].Raw.Props.Get(ical.PropRecurrenceRule).Value; got != "FREQ=WEEKLY;COUNT=8" {
		t.Errorf("future RRULE = %q, want FREQ=WEEKLY;COUNT=8 (rebalanced)", got)
	}
}
