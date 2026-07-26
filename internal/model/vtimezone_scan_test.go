package model

import (
	"testing"
	"time"
	// Embed the IANA database so a host without system zoneinfo cannot skip these
	// into a vacuous green.
	_ "time/tzdata"

	"github.com/emersion/go-ical"
)

// definedTZIDs is the set of zones an object defines with a VTIMEZONE.
func definedTZIDs(obj *Parsed) map[string]bool {
	out := map[string]bool{}
	for _, c := range obj.Calendar.Children {
		if c.Name == ical.CompTimezone {
			if p := c.Props.Get(ical.PropTimezoneID); p != nil {
				out[p.Value] = true
			}
		}
	}
	return out
}

// itemProp returns the first item component's property `name`, or nil.
func itemProp(obj *Parsed, name string) *ical.Prop {
	for _, c := range obj.Calendar.Children {
		if c.Name == ical.CompEvent || c.Name == ical.CompToDo {
			if p := c.Props.Get(name); p != nil {
				return p
			}
		}
	}
	return nil
}

// ensureVTimezone scanned only DTSTART/DTEND/DUE, so a zone referenced by a
// recurrence date alone was never defined. The reviewer's repro is the sharpest
// case: an Outlook-authored master keeps its Windows TZID spelling on DTSTART
// (which no IANA lookup resolves), while the EXDATE the app writes beside it
// carries the *resolved* IANA name — two spellings, and before the fix zero
// definitions.
func TestEnsureVTimezoneDefinesRecurrenceDateZones(t *testing.T) {
	ny, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Fatalf("America/New_York must load (tzdata is embedded): %v", err)
	}

	t.Run("EXDATE written beside a Windows-spelled anchor", func(t *testing.T) {
		ics := "BEGIN:VCALENDAR\r\nVERSION:2.0\r\nPRODID:-//Microsoft//Outlook//EN\r\n" +
			"BEGIN:VEVENT\r\nUID:win-1\r\nSUMMARY:Outlook series\r\nDTSTAMP:20260701T000000Z\r\n" +
			"DTSTART;TZID=Eastern Standard Time:20260825T200000\r\n" +
			"DTEND;TZID=Eastern Standard Time:20260825T210000\r\n" +
			"RRULE:FREQ=WEEKLY;BYDAY=TU\r\nEND:VEVENT\r\nEND:VCALENDAR\r\n"
		obj, err := Decode([]byte(ics), ny)
		if err != nil {
			t.Fatal(err)
		}
		start := time.Date(2026, 8, 25, 20, 0, 0, 0, ny)
		out, err := AddException(obj, "win-1", start.AddDate(0, 0, 7), false, start, ny)
		if err != nil {
			t.Fatal(err)
		}
		ex := itemProp(out, ical.PropExceptionDates)
		if ex == nil {
			t.Fatal("no EXDATE was written")
		}
		tzid := ex.Params.Get(ical.ParamTimezoneID)
		if tzid != "America/New_York" {
			t.Fatalf("EXDATE TZID = %q, want the resolved IANA name; the guard would be vacuous", tzid)
		}
		if !definedTZIDs(out)[tzid] {
			t.Errorf("EXDATE references TZID=%q with no VTIMEZONE defining it (defined: %v)",
				tzid, definedTZIDs(out))
		}
	})

	t.Run("RDATE carried by a foreign object", func(t *testing.T) {
		// A UTC-anchored foreign master whose only zoned value is an RDATE — so the
		// zone is reachable ONLY through the recurrence-date scan.
		ics := "BEGIN:VCALENDAR\r\nVERSION:2.0\r\nPRODID:-//foreign//EN\r\n" +
			"BEGIN:VEVENT\r\nUID:rdate-1\r\nSUMMARY:Series\r\nDTSTAMP:20260701T000000Z\r\n" +
			"DTSTART:20260825T180000Z\r\nDTEND:20260825T190000Z\r\n" +
			"RRULE:FREQ=WEEKLY\r\n" +
			"RDATE;TZID=Europe/Berlin:20260902T200000,20260909T200000\r\n" +
			"END:VEVENT\r\nEND:VCALENDAR\r\n"
		obj, err := Decode([]byte(ics), ny)
		if err != nil {
			t.Fatal(err)
		}
		start := time.Date(2026, 8, 25, 18, 0, 0, 0, time.UTC)
		out, err := AddException(obj, "rdate-1", start.AddDate(0, 0, 7), false, start, ny)
		if err != nil {
			t.Fatal(err)
		}
		if !definedTZIDs(out)["Europe/Berlin"] {
			t.Errorf("RDATE references TZID=Europe/Berlin with no VTIMEZONE defining it (defined: %v)",
				definedTZIDs(out))
		}
	})

	t.Run("RECURRENCE-ID on an override", func(t *testing.T) {
		ics := "BEGIN:VCALENDAR\r\nVERSION:2.0\r\nPRODID:-//Microsoft//Outlook//EN\r\n" +
			"BEGIN:VEVENT\r\nUID:win-2\r\nSUMMARY:Outlook series\r\nDTSTAMP:20260701T000000Z\r\n" +
			"DTSTART;TZID=Eastern Standard Time:20260825T200000\r\n" +
			"DTEND;TZID=Eastern Standard Time:20260825T210000\r\n" +
			"RRULE:FREQ=WEEKLY;BYDAY=TU\r\nEND:VEVENT\r\nEND:VCALENDAR\r\n"
		obj, err := Decode([]byte(ics), ny)
		if err != nil {
			t.Fatal(err)
		}
		start := time.Date(2026, 8, 25, 20, 0, 0, 0, ny)
		out, err := AddOccurrenceOverride(obj, "win-2", start.AddDate(0, 0, 7), false,
			func(c *ical.Component) { c.Props.SetText(ical.PropSummary, "moved") }, start, ny)
		if err != nil {
			t.Fatal(err)
		}
		var rid *ical.Prop
		for _, c := range out.Calendar.Children {
			if p := c.Props.Get(ical.PropRecurrenceID); p != nil {
				rid = p
			}
		}
		if rid == nil {
			t.Fatal("no override was added")
		}
		tzid := rid.Params.Get(ical.ParamTimezoneID)
		if tzid != "America/New_York" {
			t.Fatalf("RECURRENCE-ID TZID = %q, want the resolved IANA name; the guard would be vacuous", tzid)
		}
		if !definedTZIDs(out)[tzid] {
			t.Errorf("RECURRENCE-ID references TZID=%q with no VTIMEZONE defining it (defined: %v)",
				tzid, definedTZIDs(out))
		}
	})
}

// A recurring VTODO's anchor is DUE, not DTSTART. AddException/AddOccurrenceOverride
// hard-coded DTSTART as the master's anchor property, so a due-anchored series got a
// UTC exception written against a TZID anchor — a value naming a different instant
// than the rule generates, so the "deleted" occurrence comes straight back.
func TestRecurrenceExceptionsFollowADueAnchoredTodo(t *testing.T) {
	ny, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Fatalf("America/New_York must load (tzdata is embedded): %v", err)
	}
	due := time.Date(2026, 8, 25, 20, 0, 0, 0, ny) // Tuesday
	// A DUE-only recurring todo — no DTSTART at all, so DUE is the rule's anchor.
	obj := NewTodoObject(TodoDraft{
		Summary: "Water plants", HasDue: true, Due: due,
		Recur: &RecurSpec{Freq: FreqWeekly, Weekdays: []time.Weekday{time.Tuesday}},
	}, due)
	if itemProp(obj, ical.PropDateTimeStart) != nil {
		t.Fatal("fixture has a DTSTART; the DUE-anchored path would not be exercised")
	}
	if itemProp(obj, ical.PropDue).Params.Get(ical.ParamTimezoneID) == "" {
		t.Fatal("fixture is not TZID-anchored; the rest of the test would be vacuous")
	}
	uid := obj.Todos[0].UID
	victim := due.AddDate(0, 0, 7)

	out, err := AddException(obj, uid, victim, false, due, ny)
	if err != nil {
		t.Fatal(err)
	}
	ex := itemProp(out, ical.PropExceptionDates)
	if ex == nil {
		t.Fatal("no EXDATE was written")
	}
	if got := ex.Params.Get(ical.ParamTimezoneID); got != "America/New_York" {
		t.Errorf("EXDATE TZID = %q, want the DUE anchor's America/New_York (value %q)", got, ex.Value)
	}

	over, err := AddOccurrenceOverride(obj, uid, victim, false,
		func(c *ical.Component) { c.Props.SetText(ical.PropSummary, "moved") }, due, ny)
	if err != nil {
		t.Fatal(err)
	}
	var rid *ical.Prop
	for _, c := range over.Calendar.Children {
		if p := c.Props.Get(ical.PropRecurrenceID); p != nil {
			rid = p
		}
	}
	if rid == nil {
		t.Fatal("no override was added")
	}
	if got := rid.Params.Get(ical.ParamTimezoneID); got != "America/New_York" {
		t.Errorf("RECURRENCE-ID TZID = %q, want the DUE anchor's America/New_York (value %q)", got, rid.Value)
	}
}
