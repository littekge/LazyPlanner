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

// TestDayAgendaIncludesTodoDueAtMidnight pins the inclusive lower bound of the
// due-date window: a todo due exactly at dayStart (00:00) — the natural due time
// for a date-only / all-day todo — must appear on that day's agenda. Without a
// case sitting exactly on dayStart, weakening the bound from Due >= dayStart to
// Due > dayStart would silently drop such a todo (a pass-14 canary escape).
func TestDayAgendaIncludesTodoDueAtMidnight(t *testing.T) {
	loc := time.UTC
	dayStart := time.Date(2026, 7, 4, 0, 0, 0, 0, loc)
	dayEnd := dayStart.AddDate(0, 0, 1)

	todos := []*model.Todo{
		{UID: "due-midnight", Summary: "Date-only task", HasDue: true, Due: dayStart},
	}

	items := model.DayAgenda(nil, todos, dayStart, dayEnd)

	if len(items) != 1 {
		t.Fatalf("got %d items, want 1 (the midnight-due todo): %+v", len(items), items)
	}
	if !items[0].IsTodo() || items[0].Title != "Date-only task" {
		t.Errorf("item 0 = %+v, want the todo due exactly at dayStart", items[0])
	}
}
