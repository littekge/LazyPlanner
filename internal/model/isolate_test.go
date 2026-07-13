package model

import (
	"testing"
	"time"
)

const bundledTwoTodos = "BEGIN:VCALENDAR\r\nVERSION:2.0\r\nPRODID:-//t//t//EN\r\n" +
	"BEGIN:VTODO\r\nUID:x\r\nDTSTAMP:20260101T090000Z\r\nSUMMARY:X\r\nEND:VTODO\r\n" +
	"BEGIN:VTODO\r\nUID:y\r\nDTSTAMP:20260101T090000Z\r\nSUMMARY:Y\r\nEND:VTODO\r\n" +
	"END:VCALENDAR\r\n"

// TestIsolateComponentDropsSiblings: isolating one todo from a bundled resource
// yields an object with only that todo.
func TestIsolateComponentDropsSiblings(t *testing.T) {
	obj, err := Decode([]byte(bundledTwoTodos), time.UTC)
	if err != nil {
		t.Fatal(err)
	}
	if len(obj.Todos) != 2 {
		t.Fatalf("setup: want 2 todos, got %d", len(obj.Todos))
	}
	single, err := IsolateComponent(obj, "x", time.UTC)
	if err != nil {
		t.Fatal(err)
	}
	if len(single.Todos) != 1 || single.Todos[0].UID != "x" {
		t.Errorf("IsolateComponent kept the wrong set: %d todos", len(single.Todos))
	}
	// The source object is untouched (clone semantics).
	if len(obj.Todos) != 2 {
		t.Error("IsolateComponent mutated its input")
	}
	if _, err := single.Encode(); err != nil {
		t.Errorf("isolated object not encodable: %v", err)
	}
}

// TestRemoveComponentReportsRemaining: removing one todo from a bundle keeps the
// other and reports remaining=true; removing the last reports false.
func TestRemoveComponentReportsRemaining(t *testing.T) {
	obj, err := Decode([]byte(bundledTwoTodos), time.UTC)
	if err != nil {
		t.Fatal(err)
	}
	reduced, remaining, err := RemoveComponent(obj, "x", time.UTC)
	if err != nil {
		t.Fatal(err)
	}
	if !remaining {
		t.Error("remaining=false, want true (Y still present)")
	}
	if len(reduced.Todos) != 1 || reduced.Todos[0].UID != "y" {
		t.Errorf("reduced object wrong: %d todos", len(reduced.Todos))
	}
	// Removing the last item reports remaining=false.
	_, remaining2, err := RemoveComponent(reduced, "y", time.UTC)
	if err != nil {
		t.Fatal(err)
	}
	if remaining2 {
		t.Error("remaining=true after removing the last item, want false")
	}
}
