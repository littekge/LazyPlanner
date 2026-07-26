package model

import (
	"fmt"
	"strings"
	"testing"
	"time"

	_ "time/tzdata"

	"github.com/emersion/go-ical"
)

// gapZone names a zone whose DST transition lands ON local midnight in the
// forward direction, so that day's midnight does not exist. gapDay is one such
// day: the day a recurring series must still fire on.
type gapZone struct {
	zone   string
	gapDay time.Time // UTC-dated calendar day; the zone's local midnight is missing
}

// dstGapZones is the set of zones where rrule-go's day enumeration collapses the
// gap day into the previous one (see the comment on resolveWallClock). Every zone
// listed here was found by sweeping the whole IANA database for a nonexistent
// local midnight and then checking whether the day survives expansion; these are
// the ones that lost it. Cairo/Beirut/Amman/Damascus/Gaza/Hebron/Tehran/Casey
// also have a nonexistent local midnight but normalize FORWARD to 01:00 of the
// same day, so their day is never lost and they are deliberately not listed.
var dstGapZones = []gapZone{
	{"America/Havana", time.Date(2026, 3, 8, 0, 0, 0, 0, time.UTC)},
	{"America/Santiago", time.Date(2026, 9, 6, 0, 0, 0, 0, time.UTC)},
	{"Atlantic/Azores", time.Date(2026, 3, 29, 0, 0, 0, 0, time.UTC)},
	{"America/Scoresbysund", time.Date(2023, 3, 26, 0, 0, 0, 0, time.UTC)},
	{"America/Sao_Paulo", time.Date(2018, 11, 4, 0, 0, 0, 0, time.UTC)},
	{"America/Asuncion", time.Date(2015, 10, 4, 0, 0, 0, 0, time.UTC)},
	{"Antarctica/Palmer", time.Date(2016, 8, 14, 0, 0, 0, 0, time.UTC)},
}

// gapZoneLoc loads the zone and fails hard when it is missing. A t.Skip here
// would make this guard a vacuous pass, which is exactly the failure mode the
// test-zone guardrail forbids; internal/model imports time/tzdata so every named
// zone loads regardless of the host's tzdata.
func gapZoneLoc(t *testing.T, name string) *time.Location {
	t.Helper()
	loc, err := time.LoadLocation(name)
	if err != nil {
		t.Fatalf("zone %q must load (time/tzdata is imported): %v", name, err)
	}
	return loc
}

// assertMidnightMissing pins the fixture's own premise: if a tzdata update ever
// moves one of these transitions off midnight, the subtest would still pass while
// testing nothing. Failing loudly beats silently going vacuous.
func assertMidnightMissing(t *testing.T, loc *time.Location, day time.Time) {
	t.Helper()
	y, m, d := day.Date()
	got := time.Date(y, m, d, 0, 0, 0, 0, loc)
	if got.Year() == y && got.Month() == m && got.Day() == d && got.Hour() == 0 {
		t.Fatalf("fixture premise broken: local midnight %04d-%02d-%02d exists in %s (got %s)",
			y, m, d, loc, got.Format(time.RFC3339))
	}
}

// A recurring todo completed the day before a DST-gap day must advance ONTO that
// day, not over it. This is the data-corruption half: the rolled-forward DUE is
// written to the .ics and pushed to the server, so a skipped day is a permanent
// hole in the user's series.
func TestAdvanceRecurringTodoDoesNotSkipDSTGapDay(t *testing.T) {
	for _, gz := range dstGapZones {
		t.Run(gz.zone, func(t *testing.T) {
			loc := gapZoneLoc(t, gz.zone)
			assertMidnightMissing(t, loc, gz.gapDay)

			prev := gz.gapDay.AddDate(0, 0, -1)
			ics := fmt.Sprintf(strings.Join([]string{
				"BEGIN:VCALENDAR",
				"VERSION:2.0",
				"PRODID:-//LazyPlanner//test//EN",
				"BEGIN:VTODO",
				"UID:gap-todo",
				"DTSTAMP:20200101T000000Z",
				"SUMMARY:Water the plants",
				"DUE;VALUE=DATE:%s",
				"RRULE:FREQ=DAILY",
				"END:VTODO",
				"END:VCALENDAR",
				"",
			}, "\r\n"), prev.Format("20060102"))

			obj, err := Decode([]byte(ics), loc)
			if err != nil {
				t.Fatal(err)
			}
			now := time.Date(prev.Year(), prev.Month(), prev.Day(), 12, 0, 0, 0, loc)
			out, done, err := AdvanceRecurringTodo(obj, "gap-todo", now, loc)
			if err != nil {
				t.Fatal(err)
			}
			if done {
				t.Fatal("an unbounded daily series must never report done")
			}
			due := propOf(t, out, ical.PropDue)
			want := gz.gapDay.Format("20060102")
			if due.Value != want {
				t.Errorf("advanced DUE = %q, want %q — the DST-gap day was skipped", due.Value, want)
			}
		})
	}
}

// The read-path twin: a daily event must still produce an occurrence on the gap
// day, so it does not silently vanish from the calendar views for one day a year.
func TestEventOccurrencesIncludeDSTGapDay(t *testing.T) {
	for _, gz := range dstGapZones {
		t.Run(gz.zone, func(t *testing.T) {
			loc := gapZoneLoc(t, gz.zone)
			assertMidnightMissing(t, loc, gz.gapDay)

			prev := gz.gapDay.AddDate(0, 0, -1)
			ics := fmt.Sprintf(strings.Join([]string{
				"BEGIN:VCALENDAR",
				"VERSION:2.0",
				"PRODID:-//LazyPlanner//test//EN",
				"BEGIN:VEVENT",
				"UID:gap-event",
				"DTSTAMP:20200101T000000Z",
				"SUMMARY:Standup",
				"DTSTART;TZID=%s:%sT090000",
				"DTEND;TZID=%s:%sT093000",
				"RRULE:FREQ=DAILY",
				"END:VEVENT",
				"END:VCALENDAR",
				"",
			}, "\r\n"), gz.zone, prev.Format("20060102"), gz.zone, prev.Format("20060102"))

			obj, err := Decode([]byte(ics), loc)
			if err != nil {
				t.Fatal(err)
			}
			from := time.Date(prev.Year(), prev.Month(), prev.Day(), 0, 0, 0, 0, loc)
			to := from.AddDate(0, 0, 4)
			occs, err := obj.EventOccurrences(from, to)
			if err != nil {
				t.Fatal(err)
			}
			days := map[string]int{}
			for _, o := range occs {
				days[o.Start.In(loc).Format("2006-01-02")]++
			}
			key := gz.gapDay.Format("2006-01-02")
			if days[key] != 1 {
				t.Errorf("occurrences on the gap day %s = %d, want 1 (all days: %v)", key, days[key], days)
			}
		})
	}
}

// The occurrence recovered on a gap day must land INSIDE that day — the first
// instant of it that exists. Landing on the previous day at 23:00 would move the
// item to the wrong day, which is the bug wearing a different hat.
func TestDSTGapOccurrenceLandsOnItsOwnDay(t *testing.T) {
	loc := gapZoneLoc(t, "America/Havana")
	gap := time.Date(2026, 3, 8, 0, 0, 0, 0, time.UTC)
	assertMidnightMissing(t, loc, gap)

	ics := strings.Join([]string{
		"BEGIN:VCALENDAR",
		"VERSION:2.0",
		"PRODID:-//LazyPlanner//test//EN",
		"BEGIN:VEVENT",
		"UID:gap-event",
		"DTSTAMP:20200101T000000Z",
		"SUMMARY:Midnight ritual",
		"DTSTART;TZID=America/Havana:20260306T000000",
		"DTEND;TZID=America/Havana:20260306T003000",
		"RRULE:FREQ=DAILY",
		"END:VEVENT",
		"END:VCALENDAR",
		"",
	}, "\r\n")

	obj, err := Decode([]byte(ics), loc)
	if err != nil {
		t.Fatal(err)
	}
	from := time.Date(2026, 3, 6, 0, 0, 0, 0, loc)
	occs, err := obj.EventOccurrences(from, from.AddDate(0, 0, 5))
	if err != nil {
		t.Fatal(err)
	}
	var onGapDay []time.Time
	for _, o := range occs {
		if o.Start.In(loc).Format("2006-01-02") == "2026-03-08" {
			onGapDay = append(onGapDay, o.Start.In(loc))
		}
	}
	if len(onGapDay) != 1 {
		t.Fatalf("want exactly 1 occurrence on 2026-03-08, got %d (%v)", len(onGapDay), onGapDay)
	}
	// Havana springs forward at 00:00 -> 01:00, so 01:00 is the day's first instant.
	if got := onGapDay[0].Format("15:04:05-0700"); got != "01:00:00-0400" {
		t.Errorf("gap-day occurrence at %s, want 01:00:00-0400 (the first instant of the day)", got)
	}
}

// A DATE value names a calendar day. Reading it back must land on THAT day even
// when the zone has no midnight to put it at — otherwise the recurrence fix writes
// the right day and the app immediately misreads it, which is how this defect
// survived its first fix: the .ics said 20260308 while the UI said 03/07.
func TestDateOnlyValueOnDSTGapDayNamesItsOwnDay(t *testing.T) {
	for _, gz := range dstGapZones {
		t.Run(gz.zone, func(t *testing.T) {
			loc := gapZoneLoc(t, gz.zone)
			assertMidnightMissing(t, loc, gz.gapDay)

			ics := fmt.Sprintf(strings.Join([]string{
				"BEGIN:VCALENDAR",
				"VERSION:2.0",
				"PRODID:-//LazyPlanner//test//EN",
				"BEGIN:VTODO",
				"UID:allday",
				"DTSTAMP:20200101T000000Z",
				"SUMMARY:All-day on the gap day",
				"DUE;VALUE=DATE:%s",
				"END:VTODO",
				"END:VCALENDAR",
				"",
			}, "\r\n"), gz.gapDay.Format("20060102"))

			obj, err := Decode([]byte(ics), loc)
			if err != nil {
				t.Fatal(err)
			}
			if len(obj.Todos) != 1 {
				t.Fatalf("want 1 todo, got %d", len(obj.Todos))
			}
			got := obj.Todos[0].Due.In(loc).Format("2006-01-02")
			if want := gz.gapDay.Format("2006-01-02"); got != want {
				t.Errorf("all-day DUE read back as %s, want %s", got, want)
			}
		})
	}
}

// The same for a zone-less TZID DATE-TIME at midnight: a server-authored
// recurring item anchored at 00:00 in a midnight-gap zone must not read as the
// previous day.
func TestTZIDMidnightOnDSTGapDayNamesItsOwnDay(t *testing.T) {
	loc := gapZoneLoc(t, "America/Havana")
	gap := time.Date(2026, 3, 8, 0, 0, 0, 0, time.UTC)
	assertMidnightMissing(t, loc, gap)

	ics := strings.Join([]string{
		"BEGIN:VCALENDAR",
		"VERSION:2.0",
		"PRODID:-//LazyPlanner//test//EN",
		"BEGIN:VEVENT",
		"UID:midnight",
		"DTSTAMP:20200101T000000Z",
		"SUMMARY:Midnight",
		"DTSTART;TZID=America/Havana:20260308T000000",
		"DTEND;TZID=America/Havana:20260308T003000",
		"END:VEVENT",
		"END:VCALENDAR",
		"",
	}, "\r\n")

	obj, err := Decode([]byte(ics), loc)
	if err != nil {
		t.Fatal(err)
	}
	if len(obj.Events) != 1 {
		t.Fatalf("want 1 event, got %d", len(obj.Events))
	}
	if got := obj.Events[0].Start.In(loc).Format("2006-01-02"); got != "2026-03-08" {
		t.Errorf("DTSTART read back on %s, want 2026-03-08", got)
	}
}

// Zones whose midnight gap normalizes FORWARD already keep their day, and zones
// with no midnight gap at all must be untouched by the gap-safe path. This is the
// control: it fails if the fix starts moving occurrences it has no business
// moving.
func TestNonGapZonesKeepExistingOccurrenceInstants(t *testing.T) {
	cases := []struct {
		zone     string
		anchor   string // local wall clock, YYYYMMDDThhmmss
		wantDays []string
	}{
		// Forward-normalizing midnight gap: the day was never lost.
		{"Africa/Cairo", "20260423T000000", []string{"2026-04-23", "2026-04-24", "2026-04-25", "2026-04-26"}},
		// An ordinary 02:00 spring-forward zone, anchored across the transition.
		{"America/New_York", "20260306T000000", []string{"2026-03-06", "2026-03-07", "2026-03-08", "2026-03-09"}},
		// Southern-hemisphere fall-back.
		{"Australia/Sydney", "20260403T000000", []string{"2026-04-03", "2026-04-04", "2026-04-05", "2026-04-06"}},
		// Fixed offset, no transitions at all.
		{"Asia/Kolkata", "20260306T000000", []string{"2026-03-06", "2026-03-07", "2026-03-08", "2026-03-09"}},
	}
	for _, tc := range cases {
		t.Run(tc.zone, func(t *testing.T) {
			loc := gapZoneLoc(t, tc.zone)
			ics := fmt.Sprintf(strings.Join([]string{
				"BEGIN:VCALENDAR",
				"VERSION:2.0",
				"PRODID:-//LazyPlanner//test//EN",
				"BEGIN:VEVENT",
				"UID:ctrl",
				"DTSTAMP:20200101T000000Z",
				"SUMMARY:Control",
				"DTSTART;TZID=%s:%s",
				"DTEND;TZID=%s:%s",
				"RRULE:FREQ=DAILY",
				"END:VEVENT",
				"END:VCALENDAR",
				"",
			}, "\r\n"), tc.zone, tc.anchor, tc.zone, tc.anchor)

			obj, err := Decode([]byte(ics), loc)
			if err != nil {
				t.Fatal(err)
			}
			start, err := time.ParseInLocation("20060102T150405", tc.anchor, loc)
			if err != nil {
				t.Fatal(err)
			}
			occs, err := obj.EventOccurrences(start, start.AddDate(0, 0, 4))
			if err != nil {
				t.Fatal(err)
			}
			var got []string
			for _, o := range occs {
				got = append(got, o.Start.In(loc).Format("2006-01-02"))
			}
			if strings.Join(got, ",") != strings.Join(tc.wantDays, ",") {
				t.Errorf("days = %v, want %v", got, tc.wantDays)
			}
		})
	}
}
