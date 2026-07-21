package model

import "time"

// DayStart returns midnight (in t's own location) of the day containing t.
func DayStart(t time.Time) time.Time {
	y, m, d := t.Date()
	return time.Date(y, m, d, 0, 0, 0, 0, t.Location())
}

// StartOfWeek returns midnight of the first day of the week containing t.
// mondayFirst selects Monday (true) or Sunday (false) as the first day.
func StartOfWeek(t time.Time, mondayFirst bool) time.Time {
	d := DayStart(t)
	wd := int(d.Weekday()) // Sunday=0 .. Saturday=6
	var offset int
	if mondayFirst {
		offset = (wd + 6) % 7 // Monday=0
	} else {
		offset = wd
	}
	return d.AddDate(0, 0, -offset)
}

// Week returns the seven days (each at midnight) of the week containing t.
func Week(t time.Time, mondayFirst bool) []time.Time {
	start := StartOfWeek(t, mondayFirst)
	days := make([]time.Time, 7)
	for i := range days {
		days[i] = start.AddDate(0, 0, i)
	}
	return days
}

// MonthGrid returns a 6×7 grid of days (each at midnight) covering the month of
// t, padded with leading and trailing days from adjacent months so every row is
// a full week. Six rows always, which is enough for any month. Date arithmetic
// keeps every entry at local midnight across DST transitions.
func MonthGrid(t time.Time, mondayFirst bool) [][]time.Time {
	first := time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, t.Location())
	start := StartOfWeek(first, mondayFirst)

	grid := make([][]time.Time, 6)
	for r := 0; r < 6; r++ {
		row := make([]time.Time, 7)
		for c := 0; c < 7; c++ {
			row[c] = start.AddDate(0, 0, r*7+c)
		}
		grid[r] = row
	}
	return grid
}

// OccurrencesOn returns the occurrences overlapping the single day starting at
// day (midnight); day is treated as [day, day+24h) using calendar arithmetic.
func OccurrencesOn(occs []Occurrence, day time.Time) []Occurrence {
	start := DayStart(day)
	end := start.AddDate(0, 0, 1)
	var out []Occurrence
	for _, o := range occs {
		if overlaps(o.Start, o.End, start, end) {
			out = append(out, o)
		}
	}
	return out
}

// OverlapsDay reports whether this occurrence overlaps the calendar day
// beginning at day (in day's location) — the half-open span [DayStart(day),
// +24h). It lets the time-grid place a multi-day timed event on every day it
// covers, not only the day it starts.
func (o Occurrence) OverlapsDay(day time.Time) bool {
	ds := DayStart(day)
	return overlaps(o.Start, o.End, ds, ds.AddDate(0, 0, 1))
}

// SameDay reports whether a and b fall on the same calendar day in a's location.
func SameDay(a, b time.Time) bool {
	ay, am, ad := a.Date()
	by, bm, bd := b.In(a.Location()).Date()
	return ay == by && am == bm && ad == bd
}
