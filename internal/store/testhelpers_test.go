package store_test

import (
	"testing"
	"time"

	"github.com/littekge/LazyPlanner/internal/model"
	"github.com/littekge/LazyPlanner/internal/store"
)

// Shared builders and finders for the store test suite. Kept in one place so a new
// test reuses a canonical builder rather than adding a fourth near-identical one.

// mustDecode builds a single-VEVENT calendar object (with an X- prop to prove
// unknown-property preservation).
func mustDecode(t *testing.T, uid, summary string) *model.Parsed {
	t.Helper()
	ics := "BEGIN:VCALENDAR\r\nVERSION:2.0\r\nPRODID:-//LazyPlanner//Test//EN\r\n" +
		"BEGIN:VEVENT\r\nUID:" + uid + "\r\nDTSTAMP:20260701T120000Z\r\n" +
		"DTSTART:20260704T130000Z\r\nDTEND:20260704T133000Z\r\n" +
		"SUMMARY:" + summary + "\r\nX-CUSTOM:preserve-me\r\nEND:VEVENT\r\nEND:VCALENDAR\r\n"
	obj, err := model.Decode([]byte(ics), time.UTC)
	if err != nil {
		t.Fatalf("decoding test object: %v", err)
	}
	return obj
}

// todoWithDescICS builds a single-VTODO calendar object with a summary and a
// DESCRIPTION (a field completion/quick-set does not touch — stands in for a note
// edited on another device).
func todoWithDescICS(t *testing.T, uid, summary, description string) *model.Parsed {
	t.Helper()
	ics := "BEGIN:VCALENDAR\r\nVERSION:2.0\r\nPRODID:-//LazyPlanner//Test//EN\r\n" +
		"BEGIN:VTODO\r\nUID:" + uid + "\r\nDTSTAMP:20260701T120000Z\r\n" +
		"SUMMARY:" + summary + "\r\nDESCRIPTION:" + description + "\r\n" +
		"END:VTODO\r\nEND:VCALENDAR\r\n"
	obj, err := model.Decode([]byte(ics), time.UTC)
	if err != nil {
		t.Fatalf("decoding %q: %v", uid, err)
	}
	return obj
}

// todoICS builds a single-VTODO calendar object with a summary and optional priority.
func todoICS(t *testing.T, uid, summary string, priority int) *model.Parsed {
	t.Helper()
	ics := "BEGIN:VCALENDAR\r\nVERSION:2.0\r\nPRODID:-//LazyPlanner//Test//EN\r\n" +
		"BEGIN:VTODO\r\nUID:" + uid + "\r\nDTSTAMP:20260701T120000Z\r\n" +
		"SUMMARY:" + summary + "\r\n"
	if priority > 0 {
		ics += "PRIORITY:" + itoa(priority) + "\r\n"
	}
	ics += "END:VTODO\r\nEND:VCALENDAR\r\n"
	obj, err := model.Decode([]byte(ics), time.UTC)
	if err != nil {
		t.Fatalf("decoding %q: %v", uid, err)
	}
	return obj
}

func itoa(n int) string { return string(rune('0' + n)) }

// eventICS builds a single-VEVENT calendar object with a summary and start hour.
func eventICS(t *testing.T, uid, summary string, startHour int) *model.Parsed {
	t.Helper()
	start := time.Date(2026, 7, 4, startHour, 0, 0, 0, time.UTC).Format("20060102T150405Z")
	end := time.Date(2026, 7, 4, startHour+1, 0, 0, 0, time.UTC).Format("20060102T150405Z")
	ics := "BEGIN:VCALENDAR\r\nVERSION:2.0\r\nPRODID:-//LazyPlanner//Test//EN\r\n" +
		"BEGIN:VEVENT\r\nUID:" + uid + "\r\nDTSTAMP:20260701T120000Z\r\n" +
		"DTSTART:" + start + "\r\nDTEND:" + end + "\r\n" +
		"SUMMARY:" + summary + "\r\nEND:VEVENT\r\nEND:VCALENDAR\r\n"
	obj, err := model.Decode([]byte(ics), time.UTC)
	if err != nil {
		t.Fatalf("decoding %q: %v", uid, err)
	}
	return obj
}

// findResource returns the resource named name in cal, or nil.
func findResource(cal store.Calendar, name string) *store.Resource {
	for _, r := range cal.Resources {
		if r.Name == name {
			return r
		}
	}
	return nil
}

// findTd returns the todo with the given UID in obj, or nil.
func findTd(obj *model.Parsed, uid string) *model.Todo {
	for _, td := range obj.Todos {
		if td.UID == uid {
			return td
		}
	}
	return nil
}

// findEvt returns the event with the given UID in obj, or nil.
func findEvt(obj *model.Parsed, uid string) *model.Event {
	for _, e := range obj.Events {
		if e.UID == uid {
			return e
		}
	}
	return nil
}
