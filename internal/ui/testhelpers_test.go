package ui

import (
	"testing"
	"time"

	"github.com/littekge/LazyPlanner/internal/model"
	"github.com/littekge/LazyPlanner/internal/store"
)

// Shared builders and finders for the ui test suite. Kept in one place so a new
// test reuses a canonical helper rather than adding another near-identical one.
// (App-harness helpers — newTestApp/newWritableTestApp/storeFixture/drawCells —
// live with the harness in app_test.go/edit_test.go.)

// todoDescObj builds a single-VTODO parsed object with a fixed UID + DESCRIPTION,
// so a test can control the resource identity and simulate a note edited on
// another device (a field a summary edit does not touch).
func todoDescObj(t *testing.T, uid, summary, description string) *model.Parsed {
	t.Helper()
	ics := "BEGIN:VCALENDAR\r\nVERSION:2.0\r\nPRODID:-//LazyPlanner//Test//EN\r\n" +
		"BEGIN:VTODO\r\nUID:" + uid + "\r\nDTSTAMP:20260701T120000Z\r\n" +
		"SUMMARY:" + summary + "\r\nDESCRIPTION:" + description + "\r\n" +
		"END:VTODO\r\nEND:VCALENDAR\r\n"
	obj, err := model.Decode([]byte(ics), time.UTC)
	if err != nil {
		t.Fatalf("decode %q: %v", uid, err)
	}
	return obj
}

// findTdDesc returns the todo with the given UID from a parsed object.
func findTdDesc(obj *model.Parsed, uid string) *model.Todo {
	for _, td := range obj.Todos {
		if td.UID == uid {
			return td
		}
	}
	return nil
}

// todoBySummary returns the first todo in the store with the given summary, or nil.
func todoBySummary(s *store.Store, summary string) *model.Todo {
	for _, td := range s.Todos() {
		if td.Summary == summary {
			return td
		}
	}
	return nil
}

// todosBySummary returns every todo in the app's store with the given summary.
func todosBySummary(a *app, summary string) []*model.Todo {
	var out []*model.Todo
	for _, t := range a.store.Todos() {
		if t.Summary == summary {
			out = append(out, t)
		}
	}
	return out
}
