package model

import (
	"strings"
	"testing"
	"time"
)

const todoWithExtras = `BEGIN:VCALENDAR
VERSION:2.0
PRODID:-//Other Client//EN
BEGIN:VTODO
UID:keep-me@example.com
DTSTAMP:20260101T000000Z
SUMMARY:Original summary
X-CUSTOM-FLAG:do-not-drop
CATEGORIES:work
BEGIN:VALARM
ACTION:DISPLAY
TRIGGER:-PT15M
END:VALARM
END:VTODO
END:VCALENDAR
`

func decodeForTest(t *testing.T, data string) *Parsed {
	t.Helper()
	obj, err := Decode([]byte(data), time.UTC)
	if err != nil {
		t.Fatalf("decode fixture: %v", err)
	}
	return obj
}

// TestEditPreservesUnknownProps is the iron-rule test: editing a known field
// must leave X- properties, VALARMs, and the source PRODID untouched.
func TestEditPreservesUnknownProps(t *testing.T) {
	obj := decodeForTest(t, todoWithExtras)
	now := time.Date(2026, 7, 5, 12, 0, 0, 0, time.UTC)

	edited, err := EditTodo(obj, "keep-me@example.com", TodoDraft{
		Summary:    "New summary",
		Categories: []string{"work"},
	}, now, time.UTC)
	if err != nil {
		t.Fatalf("EditTodo: %v", err)
	}

	data, err := edited.Encode()
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	out := string(data)

	for _, want := range []string{"X-CUSTOM-FLAG:do-not-drop", "BEGIN:VALARM", "TRIGGER:-PT15M", "SUMMARY:New summary"} {
		if !strings.Contains(out, want) {
			t.Errorf("edited output missing %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "Original summary") {
		t.Errorf("edited output still has the old summary:\n%s", out)
	}
}

// TestEditDoesNotMutateOriginal guards that editing returns an independent copy.
func TestEditDoesNotMutateOriginal(t *testing.T) {
	obj := decodeForTest(t, todoWithExtras)
	now := time.Date(2026, 7, 5, 12, 0, 0, 0, time.UTC)

	if _, err := EditTodo(obj, "keep-me@example.com", TodoDraft{Summary: "Changed"}, now, time.UTC); err != nil {
		t.Fatalf("EditTodo: %v", err)
	}
	if got := obj.Todos[0].Summary; got != "Original summary" {
		t.Errorf("original mutated: summary = %q, want %q", got, "Original summary")
	}
}

func TestSetTodoCompletedRoundTrip(t *testing.T) {
	obj := decodeForTest(t, todoWithExtras)
	now := time.Date(2026, 7, 5, 12, 0, 0, 0, time.UTC)

	done, err := SetTodoCompleted(obj, "keep-me@example.com", true, now, time.UTC)
	if err != nil {
		t.Fatalf("complete: %v", err)
	}
	if !done.Todos[0].Completed() {
		t.Fatal("todo not marked completed")
	}
	out, _ := done.Encode()
	for _, want := range []string{"STATUS:COMPLETED", "PERCENT-COMPLETE:100", "COMPLETED:20260705T120000Z"} {
		if !strings.Contains(string(out), want) {
			t.Errorf("completed output missing %q:\n%s", want, out)
		}
	}

	undone, err := SetTodoCompleted(done, "keep-me@example.com", false, now, time.UTC)
	if err != nil {
		t.Fatalf("uncomplete: %v", err)
	}
	if undone.Todos[0].Completed() {
		t.Error("todo still completed after uncomplete")
	}
	out, _ = undone.Encode()
	if strings.Contains(string(out), "PERCENT-COMPLETE") || strings.Contains(string(out), "COMPLETED:") {
		t.Errorf("uncompleted output kept completion props:\n%s", out)
	}
}

func TestSetTodoParentPreservesOtherRelations(t *testing.T) {
	const withSibling = `BEGIN:VCALENDAR
VERSION:2.0
PRODID:-//X//EN
BEGIN:VTODO
UID:child@example.com
DTSTAMP:20260101T000000Z
SUMMARY:Child
RELATED-TO;RELTYPE=SIBLING:sib@example.com
RELATED-TO:old-parent@example.com
END:VTODO
END:VCALENDAR
`
	obj := decodeForTest(t, withSibling)
	now := time.Date(2026, 7, 5, 12, 0, 0, 0, time.UTC)

	edited, err := SetTodoParent(obj, "child@example.com", "new-parent@example.com", now, time.UTC)
	if err != nil {
		t.Fatalf("SetTodoParent: %v", err)
	}
	if got := edited.Todos[0].ParentUID; got != "new-parent@example.com" {
		t.Errorf("ParentUID = %q, want new-parent@example.com", got)
	}
	out, _ := edited.Encode()
	if !strings.Contains(string(out), "RELTYPE=SIBLING:sib@example.com") {
		t.Errorf("sibling relation dropped:\n%s", out)
	}
	if strings.Contains(string(out), "old-parent@example.com") {
		t.Errorf("old parent relation not replaced:\n%s", out)
	}

	// Clearing the parent removes only the PARENT link.
	rooted, err := SetTodoParent(edited, "child@example.com", "", now, time.UTC)
	if err != nil {
		t.Fatalf("clear parent: %v", err)
	}
	if rooted.Todos[0].ParentUID != "" {
		t.Errorf("ParentUID not cleared: %q", rooted.Todos[0].ParentUID)
	}
	out, _ = rooted.Encode()
	if !strings.Contains(string(out), "RELTYPE=SIBLING:sib@example.com") {
		t.Errorf("sibling relation dropped when clearing parent:\n%s", out)
	}
}

func TestNewTodoObjectEncodable(t *testing.T) {
	now := time.Date(2026, 7, 5, 12, 0, 0, 0, time.UTC)
	due := time.Date(2026, 7, 20, 0, 0, 0, 0, time.UTC)
	obj := NewTodoObject(TodoDraft{
		Summary:   "Buy milk",
		HasDue:    true,
		Due:       due,
		DueAllDay: true,
		Priority:  1,
	}, now)

	if len(obj.Todos) != 1 {
		t.Fatalf("expected 1 todo, got %d", len(obj.Todos))
	}
	td := obj.Todos[0]
	if td.UID == "" {
		t.Error("new todo has no UID")
	}
	if td.Summary != "Buy milk" || td.Priority != 1 || !td.HasDue || !td.DueAllDay {
		t.Errorf("draft not applied: %+v", td)
	}
	out, err := obj.Encode()
	if err != nil {
		t.Fatalf("encode new todo: %v", err)
	}
	if !strings.Contains(string(out), "DUE;VALUE=DATE:20260720") {
		t.Errorf("all-day due not date-only:\n%s", out)
	}
}

func TestNewEventObjectTimedIsUTC(t *testing.T) {
	now := time.Date(2026, 7, 5, 12, 0, 0, 0, time.UTC)
	start := time.Date(2026, 7, 6, 15, 0, 0, 0, time.UTC)
	obj, err := NewEventObject(EventDraft{
		Summary: "Sync",
		Start:   start,
		End:     start.Add(time.Hour),
	}, now)
	if err != nil {
		t.Fatalf("NewEventObject: %v", err)
	}
	out, _ := obj.Encode()
	for _, want := range []string{"DTSTART:20260706T150000Z", "DTEND:20260706T160000Z", "UID:", "DTSTAMP:"} {
		if !strings.Contains(string(out), want) {
			t.Errorf("event output missing %q:\n%s", want, out)
		}
	}
}

// TestEditUnknownUID reports an error rather than silently succeeding.
func TestEditUnknownUID(t *testing.T) {
	obj := decodeForTest(t, todoWithExtras)
	now := time.Date(2026, 7, 5, 12, 0, 0, 0, time.UTC)
	if _, err := EditTodo(obj, "missing@example.com", TodoDraft{Summary: "x"}, now, time.UTC); err == nil {
		t.Fatal("expected error editing unknown UID")
	}
}

// TestEditEventClearsDTEND: editing an event with a zero End removes the existing
// DTEND (symmetric with clearing a todo's DUE).
func TestEditEventClearsDTEND(t *testing.T) {
	now := time.Date(2026, 7, 5, 12, 0, 0, 0, time.UTC)
	start := time.Date(2026, 7, 6, 15, 0, 0, 0, time.UTC)
	obj, err := NewEventObject(EventDraft{Summary: "Sync", Start: start, End: start.Add(time.Hour)}, now)
	if err != nil {
		t.Fatal(err)
	}
	uid := obj.Events[0].UID

	edited, err := EditEvent(obj, uid, EventDraft{Summary: "Sync", Start: start}, now, time.UTC)
	if err != nil {
		t.Fatal(err)
	}
	out, _ := edited.Encode()
	if strings.Contains(string(out), "DTEND") {
		t.Errorf("DTEND should be cleared when End is zero:\n%s", out)
	}
	if !strings.Contains(string(out), "DTSTART:20260706T150000Z") {
		t.Errorf("DTSTART should remain:\n%s", out)
	}
}

// TestCopyTodo: a copy gets a fresh UID + new parent but preserves every other
// property (summary, categories, unknown X-props, VALARM) — the iron rule.
func TestCopyTodo(t *testing.T) {
	obj := decodeForTest(t, todoWithExtras)
	now := time.Date(2026, 7, 5, 12, 0, 0, 0, time.UTC)
	copied, err := CopyTodo(obj, "keep-me@example.com", "fresh-uid@test", "parent-9@test", now, time.UTC)
	if err != nil {
		t.Fatal(err)
	}
	td := copied.Todos[0]
	if td.UID != "fresh-uid@test" {
		t.Errorf("UID = %q, want the fresh uid", td.UID)
	}
	if td.ParentUID != "parent-9@test" {
		t.Errorf("ParentUID = %q, want parent-9@test", td.ParentUID)
	}
	if td.Summary != "Original summary" {
		t.Errorf("Summary = %q, want it preserved", td.Summary)
	}
	out, _ := copied.Encode()
	for _, want := range []string{"X-CUSTOM-FLAG:do-not-drop", "CATEGORIES:work", "BEGIN:VALARM"} {
		if !strings.Contains(string(out), want) {
			t.Errorf("copy dropped %q:\n%s", want, out)
		}
	}
}
