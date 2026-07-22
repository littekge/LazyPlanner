package model

import (
	"testing"
	"time"

	"github.com/emersion/go-ical"
)

// TestParseQuickAddLocation covers the v1.2.0 @location slot: a bare @word, a
// quoted @"multi word" span held together by the pre-lexer, a lone @ staying in
// the title, and first-match-wins.
func TestParseQuickAddLocation(t *testing.T) {
	loc := time.UTC
	now := time.Date(2026, 7, 5, 9, 0, 0, 0, loc)

	tests := []struct {
		name     string
		input    string
		title    string
		location string
	}{
		{name: "single word location", input: "Lunch @cafeteria", title: "Lunch", location: "cafeteria"},
		{name: "quoted multi-word location", input: "Class @\"room 204\" 9am", title: "Class", location: "room 204"},
		{name: "bare @ stays in title", input: "lunch @ noon", title: "lunch @ noon"},
		{name: "empty quotes are not a location", input: "x @\"\"", title: "x @\"\""},
		{name: "quoted span is not parsed as a date", input: "trip @\"jul 20\"", title: "trip", location: "jul 20"},
		{name: "first location wins", input: "meet @home @office", title: "meet @office", location: "home"},
		{name: "embedded @ is title text", input: "email bob@example.com", title: "email bob@example.com"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			qa := ParseQuickAdd(tc.input, now, loc)
			if qa.Title != tc.title {
				t.Errorf("Title = %q, want %q", qa.Title, tc.title)
			}
			if qa.Location != tc.location {
				t.Errorf("Location = %q, want %q", qa.Location, tc.location)
			}
			// A quoted date span must not have leaked into the date slot.
			if tc.name == "quoted span is not parsed as a date" && qa.HasDate {
				t.Errorf("HasDate = true, want false (the quoted span is a location)")
			}
		})
	}
}

// TestTodoParsesLocation verifies a VTODO's LOCATION round-trips into the model.
func TestTodoParsesLocation(t *testing.T) {
	td := NewTodoObject(TodoDraft{Summary: "Pickup", Location: "the depot"}, time.Date(2026, 7, 5, 9, 0, 0, 0, time.UTC))
	if got := td.Todos[0].Location; got != "the depot" {
		t.Errorf("Todo.Location = %q, want %q", got, "the depot")
	}
	if got := td.Todos[0].Raw.Props.Get(ical.PropLocation); got == nil || got.Value != "the depot" {
		t.Errorf("VTODO LOCATION = %v, want %q", got, "the depot")
	}
}
