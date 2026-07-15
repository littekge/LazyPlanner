package model

import (
	"strings"
	"testing"
	"time"
)

// Guards pass-11 MED #5: detaching a recurring-todo occurrence ("edit this
// occurrence") must preserve the original's unmodeled properties. The old UI path
// rebuilt the standalone from modeled fields only (model.NewTodoObject(draft)),
// dropping any VALARM and X- prop the original carried — an iron-rule violation.
// DetachTodoOccurrence clones the original component instead.
const recurTodoWithExtras = `BEGIN:VCALENDAR
VERSION:2.0
PRODID:-//Other Client//EN
BEGIN:VTODO
UID:recur-me@example.com
DTSTAMP:20260101T000000Z
SUMMARY:Water the plants
DUE:20260710T090000Z
RRULE:FREQ=WEEKLY
X-APPLE-SORT-ORDER:12345
BEGIN:VALARM
ACTION:DISPLAY
TRIGGER:-PT15M
END:VALARM
END:VTODO
END:VCALENDAR
`

func TestDetachOccurrencePreservesUnmodeledProps(t *testing.T) {
	obj := decodeForTest(t, recurTodoWithExtras)
	now := time.Date(2026, 7, 5, 12, 0, 0, 0, time.UTC)

	td := obj.Todos[0]

	// A draft built from the form (modeled fields only), as the UI does.
	d := TodoDraft{
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
	standalone, newUID, err := DetachTodoOccurrence(obj, td.UID, d, now, time.UTC)
	if err != nil {
		t.Fatalf("detach: %v", err)
	}
	if newUID == "" || newUID == td.UID {
		t.Fatalf("detached standalone must carry a fresh UID, got %q (original %q)", newUID, td.UID)
	}

	data, err := standalone.Encode()
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	out := string(data)

	// The unmodeled props must survive the detach (iron rule)...
	for _, want := range []string{"X-APPLE-SORT-ORDER:12345", "BEGIN:VALARM", "TRIGGER:-PT15M"} {
		if !strings.Contains(out, want) {
			t.Errorf("iron-rule violation: detached standalone dropped %q:\n%s", want, out)
		}
	}
	// ...and the recurrence must NOT (it is now a plain one-off task).
	if strings.Contains(out, "RRULE") {
		t.Errorf("detached standalone must not keep the RRULE:\n%s", out)
	}
}
