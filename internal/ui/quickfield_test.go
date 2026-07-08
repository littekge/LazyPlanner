package ui

import (
	"testing"
	"time"

	"github.com/littekge/LazyPlanner/internal/model"
)

func TestParseSetPriority(t *testing.T) {
	now := time.Date(2026, 7, 5, 9, 0, 0, 0, time.UTC)
	cases := []struct {
		in    string
		want  int
		wantK bool
	}{
		{"3", 3, true},
		{"!3", 3, true}, // a leading ! is tolerated
		{"high", 1, true},
		{"med", 5, true},
		{"low", 9, true},
		{"", 0, true},     // blank clears
		{"0", 0, true},    // explicit clear
		{"none", 0, true}, // word clear
		{"banana", 0, false},
		{"12", 0, false}, // out of 1-9 range
	}
	for _, c := range cases {
		got, ok := parseSetPriority(c.in, now, time.UTC)
		if got != c.want || ok != c.wantK {
			t.Errorf("parseSetPriority(%q) = (%d,%v), want (%d,%v)", c.in, got, ok, c.want, c.wantK)
		}
	}
}

func TestApplyTodoFieldPriorityAndUndo(t *testing.T) {
	now := time.Date(2026, 7, 5, 9, 0, 0, 0, time.UTC)
	a := newWritableTestApp(t, now)
	a.setMode(modeTasks)
	cal := a.selectedTasklistID()
	a.createTask(cal, "", "Task X")
	uid := todoBySummary(a.store, "Task X").UID
	if p := todoBySummary(a.store, "Task X").Priority; p != 0 {
		t.Fatalf("new task priority = %d, want 0", p)
	}

	a.applyTodoField(uid, "set priority", func(d *model.TodoDraft) { d.Priority = 7 })
	if p := todoBySummary(a.store, "Task X").Priority; p != 7 {
		t.Errorf("priority after set = %d, want 7", p)
	}

	a.undoLast()
	if p := todoBySummary(a.store, "Task X").Priority; p != 0 {
		t.Errorf("priority after undo = %d, want 0", p)
	}
}

func TestApplyTodoFieldDue(t *testing.T) {
	now := time.Date(2026, 7, 5, 9, 0, 0, 0, time.UTC)
	a := newWritableTestApp(t, now)
	a.setMode(modeTasks)
	cal := a.selectedTasklistID()
	a.createTask(cal, "", "Task Y")
	uid := todoBySummary(a.store, "Task Y").UID

	a.applyTodoField(uid, "set due", func(d *model.TodoDraft) {
		d.HasDue, d.DueAllDay = true, true
		d.Due = time.Date(2026, 7, 20, 0, 0, 0, 0, time.Local)
	})
	td := todoBySummary(a.store, "Task Y")
	if !td.HasDue {
		t.Fatal("task should have a due date after set")
	}
	if got := td.Due.In(time.Local).Format("2006-01-02"); got != "2026-07-20" {
		t.Errorf("due = %s, want 2026-07-20", got)
	}

	a.applyTodoField(uid, "clear due", func(d *model.TodoDraft) {
		d.HasDue, d.Due, d.DueAllDay = false, time.Time{}, false
	})
	if todoBySummary(a.store, "Task Y").HasDue {
		t.Error("due should be cleared")
	}
}
