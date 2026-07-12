package model

import (
	"testing"
	"time"

	"github.com/emersion/go-ical"
)

const weeklyEvent = "BEGIN:VCALENDAR\r\nVERSION:2.0\r\nPRODID:-//t//t//EN\r\n" +
	"BEGIN:VEVENT\r\nUID:wk@t\r\nSUMMARY:Standup\r\nDTSTAMP:20260701T000000Z\r\n" +
	"DTSTART:20260706T090000Z\r\nDTEND:20260706T093000Z\r\nRRULE:FREQ=WEEKLY;COUNT=4\r\n" +
	"END:VEVENT\r\nEND:VCALENDAR\r\n"

func eventStarts(t *testing.T, obj *Parsed) []time.Time {
	t.Helper()
	occs, err := obj.EventOccurrences(
		time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	out := make([]time.Time, len(occs))
	for i, o := range occs {
		out[i] = o.Start.UTC()
	}
	return out
}

func d(y int, m time.Month, day, h int) time.Time {
	return time.Date(y, m, day, h, 0, 0, 0, time.UTC)
}

// TestAddOccurrenceOverride: editing one instance of a weekly series adds a
// RECURRENCE-ID override that replaces just that slot; the others are unchanged.
func TestAddOccurrenceOverride(t *testing.T) {
	obj := decodeForTest(t, weeklyEvent)
	now := d(2026, 7, 1, 0)
	occ := d(2026, 7, 13, 9) // the 2nd instance

	// Move this instance to 10:00 and rename it.
	out, err := AddOccurrenceOverride(obj, "wk@t", occ, false, func(c *ical.Component) {
		applyEvent(c, EventDraft{Summary: "Special", Start: d(2026, 7, 13, 10), End: d(2026, 7, 13, 11)}, now)
	}, now, time.UTC)
	if err != nil {
		t.Fatal(err)
	}

	occs, err := out.EventOccurrences(d(2026, 1, 1, 0), d(2027, 1, 1, 0))
	if err != nil {
		t.Fatal(err)
	}
	want := []time.Time{d(2026, 7, 6, 9), d(2026, 7, 13, 10), d(2026, 7, 20, 9), d(2026, 7, 27, 9)}
	if len(occs) != len(want) {
		t.Fatalf("got %d occurrences, want %d", len(occs), len(want))
	}
	for i := range want {
		if !occs[i].Start.UTC().Equal(want[i]) {
			t.Errorf("occurrence %d start = %s, want %s", i, occs[i].Start.UTC(), want[i])
		}
	}
	// The moved instance carries the edited summary.
	if got := occs[1].Event.Summary; got != "Special" {
		t.Errorf("overridden instance summary = %q, want Special", got)
	}
	// The others keep the master's summary.
	if got := occs[0].Event.Summary; got != "Standup" {
		t.Errorf("untouched instance summary = %q, want Standup", got)
	}
}

// TestAddException: deleting one instance suppresses just that slot.
func TestAddException(t *testing.T) {
	obj := decodeForTest(t, weeklyEvent)
	now := d(2026, 7, 1, 0)
	out, err := AddException(obj, "wk@t", d(2026, 7, 13, 9), false, now, time.UTC)
	if err != nil {
		t.Fatal(err)
	}
	got := eventStarts(t, out)
	want := []time.Time{d(2026, 7, 6, 9), d(2026, 7, 20, 9), d(2026, 7, 27, 9)}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if !got[i].Equal(want[i]) {
			t.Errorf("occ %d = %s, want %s", i, got[i], want[i])
		}
	}
}

// TestCapSeries: capping ends the series at UNTIL (inclusive).
func TestCapSeries(t *testing.T) {
	obj := decodeForTest(t, weeklyEvent)
	now := d(2026, 7, 1, 0)
	// This-and-future delete from the 3rd instance (07-20): cap just before it.
	out, err := CapSeries(obj, "wk@t", d(2026, 7, 20, 9).Add(-time.Second), now, time.UTC)
	if err != nil {
		t.Fatal(err)
	}
	got := eventStarts(t, out)
	want := []time.Time{d(2026, 7, 6, 9), d(2026, 7, 13, 9)}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

// TestSplitSeries: cap the master + spawn a new series = this-and-future edit.
func TestSplitSeries(t *testing.T) {
	obj := decodeForTest(t, weeklyEvent)
	now := d(2026, 7, 1, 0)
	occ := d(2026, 7, 20, 9) // split at the 3rd instance

	capped, err := CapSeries(obj, "wk@t", occ.Add(-time.Second), now, time.UTC)
	if err != nil {
		t.Fatal(err)
	}
	if got := eventStarts(t, capped); len(got) != 2 {
		t.Fatalf("capped master has %d occurrences, want 2", len(got))
	}

	// New series from 07-20 at 11:00 with a new name.
	newObj, err := NewSeriesFrom(obj, "wk@t", func(c *ical.Component) {
		applyEvent(c, EventDraft{Summary: "Renamed", Start: d(2026, 7, 20, 11), End: d(2026, 7, 20, 12)}, now)
	}, now, time.UTC)
	if err != nil {
		t.Fatal(err)
	}
	occs, err := newObj.EventOccurrences(d(2026, 7, 1, 0), d(2026, 8, 20, 0))
	if err != nil {
		t.Fatal(err)
	}
	if len(occs) == 0 {
		t.Fatal("new series has no occurrences")
	}
	if !occs[0].Start.UTC().Equal(d(2026, 7, 20, 11)) {
		t.Errorf("new series first start = %s, want 2026-07-20 11:00Z", occs[0].Start.UTC())
	}
	if occs[0].Event.Summary != "Renamed" {
		t.Errorf("new series summary = %q, want Renamed", occs[0].Event.Summary)
	}
	if occs[0].Event.UID == "wk@t" {
		t.Error("new series must have a fresh UID, not the master's")
	}
}

const weeklyTodo = "BEGIN:VCALENDAR\r\nVERSION:2.0\r\nPRODID:-//t//t//EN\r\n" +
	"BEGIN:VTODO\r\nUID:tk@t\r\nSUMMARY:Water plants\r\nDTSTAMP:20260701T000000Z\r\n" +
	"DTSTART:20260706T080000Z\r\nDUE:20260706T090000Z\r\nRRULE:FREQ=WEEKLY;COUNT=3\r\n" +
	"END:VTODO\r\nEND:VCALENDAR\r\n"

// TestAdvanceRecurringTodo: completing an occurrence rolls DTSTART/DUE forward and
// keeps the offset; the final occurrence marks the todo done.
func TestAdvanceRecurringTodo(t *testing.T) {
	now := d(2026, 7, 8, 0)

	// First advance: 07-06 → 07-13, keeping the 1h DTSTART→DUE offset, not done.
	obj := decodeForTest(t, weeklyTodo)
	out, done, err := AdvanceRecurringTodo(obj, "tk@t", now, time.UTC)
	if err != nil {
		t.Fatal(err)
	}
	if done {
		t.Fatal("first advance of a 3-count series should not be done")
	}
	td := onlyTodoIn(t, out)
	if !td.Due.UTC().Equal(d(2026, 7, 13, 9)) {
		t.Errorf("advanced DUE = %s, want 2026-07-13 09:00Z", td.Due.UTC())
	}
	if td.Completed() {
		t.Error("advanced todo should be not-done")
	}

	// Advancing the last remaining occurrence marks it completed.
	single := decodeForTest(t, "BEGIN:VCALENDAR\r\nVERSION:2.0\r\nPRODID:-//t//t//EN\r\n"+
		"BEGIN:VTODO\r\nUID:one@t\r\nSUMMARY:Last\r\nDTSTAMP:20260701T000000Z\r\n"+
		"DTSTART:20260706T080000Z\r\nDUE:20260706T090000Z\r\nRRULE:FREQ=WEEKLY;COUNT=1\r\n"+
		"END:VTODO\r\nEND:VCALENDAR\r\n")
	out2, done2, err := AdvanceRecurringTodo(single, "one@t", now, time.UTC)
	if err != nil {
		t.Fatal(err)
	}
	if !done2 {
		t.Fatal("advancing the last occurrence of a COUNT=1 series should complete it")
	}
	if !onlyTodoIn(t, out2).Completed() {
		t.Error("exhausted recurring todo should be marked completed")
	}
}

func onlyTodoIn(t *testing.T, obj *Parsed) *Todo {
	t.Helper()
	if len(obj.Todos) != 1 {
		t.Fatalf("want 1 todo, got %d", len(obj.Todos))
	}
	return obj.Todos[0]
}
