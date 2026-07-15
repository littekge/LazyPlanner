package model

import (
	"strings"
	"testing"
	"time"
)

// TestReproQuickFieldStatusLoss reproduces the HIGH finding: a quick sp/sd on an
// IN-PROCESS task with PERCENT-COMPLETE:50 silently resets STATUS to
// NEEDS-ACTION and drops PERCENT-COMPLETE. Mirrors the UI flow:
// draftFromTodo -> EditTodo (which calls setCompleted with Completed()==false).
func TestReproQuickFieldStatusLoss(t *testing.T) {
	const inProcess = `BEGIN:VCALENDAR
VERSION:2.0
PRODID:-//NextCloud Tasks//EN
BEGIN:VTODO
UID:inproc@example.com
DTSTAMP:20260101T000000Z
SUMMARY:Half-done task
STATUS:IN-PROCESS
PERCENT-COMPLETE:50
END:VTODO
END:VCALENDAR
`
	obj := decodeForTest(t, inProcess)
	now := time.Date(2026, 7, 15, 12, 0, 0, 0, time.UTC)

	td := obj.Todos[0]
	// Sanity: the fixture parsed as intended.
	if td.Status != StatusInProcess {
		t.Fatalf("fixture status = %q, want IN-PROCESS", td.Status)
	}

	// Replicate the UI's draftFromTodo: Completed = td.Completed() (false here).
	draft := TodoDraft{
		Summary:     td.Summary,
		Description: td.Description,
		HasDue:      td.HasDue,
		Due:         td.Due,
		DueAllDay:   td.DueAllDay,
		Priority:    td.Priority,
		Categories:  td.Categories,
		ParentUID:   td.ParentUID,
		Completed:   td.Completed(),
	}
	// The quick sp change: bump priority. Everything else should be preserved.
	draft.Priority = 1

	edited, err := EditTodo(obj, "inproc@example.com", draft, now, time.UTC)
	if err != nil {
		t.Fatalf("EditTodo: %v", err)
	}
	data, err := edited.Encode()
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	out := string(data)

	if !strings.Contains(out, "STATUS:IN-PROCESS") {
		t.Errorf("STATUS:IN-PROCESS was lost:\n%s", out)
	}
	if !strings.Contains(out, "PERCENT-COMPLETE:50") {
		t.Errorf("PERCENT-COMPLETE:50 was dropped:\n%s", out)
	}
}
