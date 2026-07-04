package model_test

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/emersion/go-ical"

	"github.com/littekge/LazyPlanner/internal/model"
)

// testLoc is the location used to interpret floating and date-only values in
// tests. A fixed non-UTC zone makes all-day parsing deterministic and distinct
// from UTC, so a bug that ignores loc is visible.
func testLoc(t *testing.T) *time.Location {
	t.Helper()
	loc, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Skipf("timezone database unavailable: %v", err)
	}
	return loc
}

func decode(t *testing.T, name string, loc *time.Location) *model.Parsed {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatalf("reading fixture %s: %v", name, err)
	}
	p, err := model.Decode(data, loc)
	if err != nil {
		t.Fatalf("decoding %s: %v", name, err)
	}
	return p
}

func onlyEvent(t *testing.T, p *model.Parsed) *model.Event {
	t.Helper()
	if len(p.Events) != 1 {
		t.Fatalf("expected exactly 1 event, got %d", len(p.Events))
	}
	return p.Events[0]
}

func onlyTodo(t *testing.T, p *model.Parsed) *model.Todo {
	t.Helper()
	if len(p.Todos) != 1 {
		t.Fatalf("expected exactly 1 todo, got %d", len(p.Todos))
	}
	return p.Todos[0]
}

func TestParseEvents(t *testing.T) {
	loc := testLoc(t)

	tests := []struct {
		name        string
		file        string
		uid         string
		summary     string
		start       time.Time
		end         time.Time
		allDay      bool
		location    string
		description string
		hasAlarm    bool
		recurring   bool
	}{
		{
			name:        "timed event with TZID, alarm, and location",
			file:        "event_timed.ics",
			uid:         "timed-1@lazyplanner.test",
			summary:     "Standup meeting",
			start:       time.Date(2026, 7, 4, 18, 30, 0, 0, time.UTC), // 14:30 EDT
			end:         time.Date(2026, 7, 4, 19, 30, 0, 0, time.UTC), // 15:30 EDT
			allDay:      false,
			location:    "Room 204",
			description: "Daily sync\nBring notes",
			hasAlarm:    true,
			recurring:   false,
		},
		{
			name:    "all-day multi-day event",
			file:    "event_allday.ics",
			uid:     "allday-1@lazyplanner.test",
			summary: "Conference",
			start:   time.Date(2026, 7, 4, 0, 0, 0, 0, loc),
			end:     time.Date(2026, 7, 6, 0, 0, 0, 0, loc), // DTEND is exclusive
			allDay:  true,
		},
		{
			name:      "UTC recurring event",
			file:      "event_utc_recurring.ics",
			uid:       "utc-1@lazyplanner.test",
			summary:   "Webinar",
			start:     time.Date(2026, 7, 4, 9, 0, 0, 0, time.UTC),
			end:       time.Date(2026, 7, 4, 10, 0, 0, 0, time.UTC),
			allDay:    false,
			recurring: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ev := onlyEvent(t, decode(t, tt.file, loc))

			if ev.UID != tt.uid {
				t.Errorf("UID = %q, want %q", ev.UID, tt.uid)
			}
			if ev.Summary != tt.summary {
				t.Errorf("Summary = %q, want %q", ev.Summary, tt.summary)
			}
			if !ev.Start.Equal(tt.start) {
				t.Errorf("Start = %s, want %s", ev.Start.UTC(), tt.start.UTC())
			}
			if !ev.End.Equal(tt.end) {
				t.Errorf("End = %s, want %s", ev.End.UTC(), tt.end.UTC())
			}
			if ev.AllDay != tt.allDay {
				t.Errorf("AllDay = %v, want %v", ev.AllDay, tt.allDay)
			}
			if ev.Location != tt.location {
				t.Errorf("Location = %q, want %q", ev.Location, tt.location)
			}
			if ev.Description != tt.description {
				t.Errorf("Description = %q, want %q", ev.Description, tt.description)
			}
			if ev.HasAlarm != tt.hasAlarm {
				t.Errorf("HasAlarm = %v, want %v", ev.HasAlarm, tt.hasAlarm)
			}
			if ev.Recurring != tt.recurring {
				t.Errorf("Recurring = %v, want %v", ev.Recurring, tt.recurring)
			}
		})
	}
}

func TestParseTodos(t *testing.T) {
	loc := testLoc(t)

	t.Run("todo with timed due, priority, categories", func(t *testing.T) {
		td := onlyTodo(t, decode(t, "todo_due.ics", loc))

		if td.UID != "todo-1@lazyplanner.test" {
			t.Errorf("UID = %q", td.UID)
		}
		if td.Summary != "Grade lab reports" {
			t.Errorf("Summary = %q", td.Summary)
		}
		if !td.HasDue {
			t.Fatal("HasDue = false, want true")
		}
		if td.DueAllDay {
			t.Error("DueAllDay = true, want false")
		}
		want := time.Date(2026, 7, 10, 21, 0, 0, 0, time.UTC) // 17:00 EDT
		if !td.Due.Equal(want) {
			t.Errorf("Due = %s, want %s", td.Due.UTC(), want)
		}
		if td.Status != model.StatusNeedsAction {
			t.Errorf("Status = %q, want %q", td.Status, model.StatusNeedsAction)
		}
		if td.Completed() {
			t.Error("Completed() = true, want false")
		}
		if td.Priority != 1 {
			t.Errorf("Priority = %d, want 1", td.Priority)
		}
		if got, want := td.Categories, []string{"school", "grading"}; !equalStrings(got, want) {
			t.Errorf("Categories = %v, want %v", got, want)
		}
		if td.Description != "ECE384 section A" {
			t.Errorf("Description = %q", td.Description)
		}
		if td.ParentUID != "" {
			t.Errorf("ParentUID = %q, want empty (root task)", td.ParentUID)
		}
	})

	t.Run("completed subtask with all-day due and explicit parent", func(t *testing.T) {
		td := onlyTodo(t, decode(t, "todo_subtask.ics", loc))

		if td.Status != model.StatusCompleted || !td.Completed() {
			t.Errorf("Status = %q, Completed() = %v", td.Status, td.Completed())
		}
		if !td.HasDue || !td.DueAllDay {
			t.Errorf("HasDue = %v, DueAllDay = %v, want both true", td.HasDue, td.DueAllDay)
		}
		want := time.Date(2026, 7, 12, 0, 0, 0, 0, loc)
		if !td.Due.Equal(want) {
			t.Errorf("Due = %s, want %s", td.Due, want)
		}
		if td.ParentUID != "todo-parent-1@lazyplanner.test" {
			t.Errorf("ParentUID = %q", td.ParentUID)
		}
		if td.Priority != 5 {
			t.Errorf("Priority = %d, want 5", td.Priority)
		}
	})

	t.Run("minimal todo: no due, default-parent RELATED-TO", func(t *testing.T) {
		td := onlyTodo(t, decode(t, "todo_minimal.ics", loc))

		if td.HasDue {
			t.Error("HasDue = true, want false")
		}
		if td.Status != "" {
			t.Errorf("Status = %q, want empty", td.Status)
		}
		if td.Priority != model.PriorityUndefined {
			t.Errorf("Priority = %d, want undefined", td.Priority)
		}
		// RELATED-TO with no RELTYPE defaults to PARENT per RFC 5545.
		if td.ParentUID != "todo-parent-2@lazyplanner.test" {
			t.Errorf("ParentUID = %q, want default-parent target", td.ParentUID)
		}
		if len(td.Categories) != 0 {
			t.Errorf("Categories = %v, want none", td.Categories)
		}
	})
}

// TestRoundTripPreservesUnknownData is the property-preservation iron rule in
// miniature: decoding then re-encoding must not drop properties the model does
// not model (an X- property) or nested components (a VALARM).
func TestRoundTripPreservesUnknownData(t *testing.T) {
	loc := testLoc(t)
	p := decode(t, "event_timed.ics", loc)
	ev := onlyEvent(t, p)

	if ev.Raw.Props.Get("X-CUSTOM-FLAG") == nil {
		t.Error("unknown X-CUSTOM-FLAG dropped from parsed component")
	}

	var buf bytes.Buffer
	if err := ical.NewEncoder(&buf).Encode(p.Calendar); err != nil {
		t.Fatalf("re-encoding calendar: %v", err)
	}
	out := buf.String()

	for _, want := range []string{"X-CUSTOM-FLAG:keep-me", "BEGIN:VALARM", "TRIGGER:-PT15M"} {
		if !bytes.Contains([]byte(out), []byte(want)) {
			t.Errorf("re-encoded output missing %q:\n%s", want, out)
		}
	}
}

func TestDecodeMalformedStreamErrors(t *testing.T) {
	if _, err := model.Decode([]byte("not an icalendar file"), time.UTC); err == nil {
		t.Error("expected an error decoding a non-iCalendar stream, got nil")
	}
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
