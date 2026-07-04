package model_test

import (
	"testing"
	"time"

	"github.com/littekge/LazyPlanner/internal/model"
)

func TestDayAgenda(t *testing.T) {
	loc := time.UTC
	dayStart := time.Date(2026, 7, 4, 0, 0, 0, 0, loc)
	dayEnd := dayStart.AddDate(0, 0, 1)

	allDay := &model.Event{Summary: "Holiday", AllDay: true, Start: dayStart}
	timed := &model.Event{Summary: "Afternoon sync", Start: time.Date(2026, 7, 4, 14, 0, 0, 0, loc)}
	occs := []model.Occurrence{
		{Start: timed.Start, Event: timed},
		{Start: allDay.Start, Event: allDay},
	}

	todos := []*model.Todo{
		{UID: "due-morning", Summary: "Morning task", HasDue: true, Due: time.Date(2026, 7, 4, 9, 0, 0, 0, loc)},
		{UID: "due-tomorrow", Summary: "Later", HasDue: true, Due: dayEnd.Add(time.Hour)},
		{UID: "no-due", Summary: "Someday"},
	}

	items := model.DayAgenda(occs, todos, dayStart, dayEnd)

	// Expect: all-day event, then 09:00 todo, then 14:00 event. Tomorrow's and
	// undated todos are excluded.
	if len(items) != 3 {
		t.Fatalf("got %d items, want 3: %+v", len(items), items)
	}
	if !items[0].AllDay || items[0].Title != "Holiday" {
		t.Errorf("item 0 = %+v, want the all-day holiday first", items[0])
	}
	if !items[1].IsTodo() || items[1].Title != "Morning task" {
		t.Errorf("item 1 = %+v, want the 09:00 todo", items[1])
	}
	if items[2].IsTodo() || items[2].Title != "Afternoon sync" {
		t.Errorf("item 2 = %+v, want the 14:00 event", items[2])
	}
}
