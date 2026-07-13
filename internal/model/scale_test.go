package model_test

import (
	"fmt"
	"testing"
	"time"

	"github.com/littekge/LazyPlanner/internal/model"
)

// TestOccurrencesBoundedOnUnboundedRule covers the scale/robustness fix: a
// syntactically valid but pathological recurrence (FREQ=SECONDLY, no COUNT/UNTIL)
// must expand in bounded time and memory rather than materializing millions of
// instances and hanging the UI. The window is a full month, in which an
// unbounded per-second rule would otherwise yield ~2.6M instances.
func TestOccurrencesBoundedOnUnboundedRule(t *testing.T) {
	ics := "BEGIN:VCALENDAR\r\nVERSION:2.0\r\nPRODID:-//x//x//EN\r\n" +
		"BEGIN:VEVENT\r\nUID:flood\r\nDTSTART:20260101T000000Z\r\nRRULE:FREQ=SECONDLY\r\n" +
		"SUMMARY:Flood\r\nEND:VEVENT\r\nEND:VCALENDAR\r\n"
	p, err := model.Decode([]byte(ics), time.UTC)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	from := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)

	done := make(chan int, 1)
	go func() {
		occs, _ := p.Events[0].Occurrences(from, to)
		done <- len(occs)
	}()
	select {
	case n := <-done:
		if n > 10000 {
			t.Fatalf("expansion returned %d instances, want it capped at 10000", n)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("expansion did not complete in 10s — pathological rule is not bounded")
	}
}

// TestOccurrencesBoundedFarAnchor covers the other pathological shape: a rule
// anchored long before the query window, where the skip-forward loop (not the
// collection) is what would run away.
func TestOccurrencesBoundedFarAnchor(t *testing.T) {
	ics := "BEGIN:VCALENDAR\r\nVERSION:2.0\r\nPRODID:-//x//x//EN\r\n" +
		"BEGIN:VEVENT\r\nUID:old\r\nDTSTART:00010101T000000Z\r\nRRULE:FREQ=MINUTELY\r\n" +
		"SUMMARY:Ancient\r\nEND:VEVENT\r\nEND:VCALENDAR\r\n"
	p, err := model.Decode([]byte(ics), time.UTC)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	from := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)

	done := make(chan struct{})
	go func() {
		_, _ = p.Events[0].Occurrences(from, to)
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("expansion of a far-anchored unbounded rule did not complete in 10s")
	}
}

// buildTodos makes n todos in a shallow forest (each of the first level under a
// small set of roots), the realistic shape, for the BuildTree benchmark.
func buildTodos(n int) []*model.Todo {
	todos := make([]*model.Todo, 0, n)
	for i := 0; i < n; i++ {
		td := &model.Todo{UID: fmt.Sprintf("uid-%d", i), Summary: fmt.Sprintf("task %d", i)}
		if i >= 8 {
			td.ParentUID = fmt.Sprintf("uid-%d", i%8) // spread under 8 roots
		}
		todos = append(todos, td)
	}
	return todos
}

func BenchmarkBuildTree(b *testing.B) {
	for _, n := range []int{100, 1000, 5000} {
		todos := buildTodos(n)
		b.Run(fmt.Sprintf("n=%d", n), func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				model.BuildTree(todos, true)
			}
		})
	}
}
