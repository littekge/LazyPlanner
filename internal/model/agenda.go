package model

import (
	"sort"
	"strings"
	"time"
)

// AgendaItem is one entry in a day's agenda: either an event occurrence or a
// todo due that day. Exactly one of Event or Todo is set.
type AgendaItem struct {
	Start time.Time
	// End is the occurrence's end instant for events (zero for todos); it lets a
	// day-cell label distinguish a multi-day event's start/continuation/final day.
	End    time.Time
	AllDay bool
	Title  string
	Event  *Event
	Todo   *Todo
}

// IsTodo reports whether this agenda item is a due todo rather than an event.
func (a AgendaItem) IsTodo() bool { return a.Todo != nil }

// DayAgenda merges event occurrences with todos due within [dayStart, dayEnd)
// into one time-ordered list: all-day items first, then timed items by start,
// ties broken by title. occs should already be limited to the day (as returned
// by a store range query); todos are filtered here by their due time.
func DayAgenda(occs []Occurrence, todos []*Todo, dayStart, dayEnd time.Time) []AgendaItem {
	items := make([]AgendaItem, 0, len(occs))
	for _, o := range occs {
		items = append(items, AgendaItem{
			Start:  o.Start,
			End:    o.End,
			AllDay: o.Event.AllDay,
			Title:  o.Event.Summary,
			Event:  o.Event,
		})
	}
	for _, t := range todos {
		// A recurring todo shows only its current occurrence (its live due); it
		// advances forward on completion rather than painting future occurrences.
		if t.HasDue && !t.Due.Before(dayStart) && t.Due.Before(dayEnd) {
			items = append(items, AgendaItem{
				Start:  t.Due,
				AllDay: t.DueAllDay,
				Title:  t.Summary,
				Todo:   t,
			})
		}
	}

	sort.SliceStable(items, func(i, j int) bool {
		if items[i].AllDay != items[j].AllDay {
			return items[i].AllDay // all-day / untimed first
		}
		if !items[i].Start.Equal(items[j].Start) {
			return items[i].Start.Before(items[j].Start)
		}
		return strings.ToLower(items[i].Title) < strings.ToLower(items[j].Title)
	})
	return items
}
