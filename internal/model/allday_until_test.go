package model

import (
	"testing"
	"time"

	"github.com/emersion/go-ical"
)

// "Ends on date D" on an all-day series must include D's occurrence, and the
// stored UNTIL must be D's own calendar date — not the date D's local midnight
// happens to land on in UTC. These guard both halves across zones east and west
// of UTC, since the defect's sign flips with the offset.

// allDayUntilZones are the locations these tests sweep: UTC, a west-of-UTC zone
// (where a UTC-midnight reading of a DATE UNTIL falls before the local-midnight
// occurrence on D) and two east-of-UTC zones (where converting the anchor to UTC
// before truncating writes the previous day into the .ics).
var allDayUntilZones = []string{"UTC", "America/New_York", "Asia/Kolkata", "Pacific/Kiritimati"}

func loadZone(t *testing.T, name string) *time.Location {
	t.Helper()
	loc, err := time.LoadLocation(name)
	if err != nil {
		t.Skipf("zone %q unavailable: %v", name, err)
	}
	return loc
}

// rruleOf returns the single component's RRULE value.
func rruleOf(t *testing.T, obj *Parsed) string {
	t.Helper()
	for _, c := range obj.Calendar.Children {
		if p := c.Props.Get(ical.PropRecurrenceRule); p != nil {
			return p.Value
		}
	}
	t.Fatal("no RRULE on object")
	return ""
}

// TestAllDayEndsOnDateIncludesEndDay is the model-layer repro: a daily all-day
// series starting 2026-07-20 with "Ends on date 2026-07-25" must occur ON
// 2026-07-25.
func TestAllDayEndsOnDateIncludesEndDay(t *testing.T) {
	for _, zone := range allDayUntilZones {
		t.Run(zone, func(t *testing.T) {
			loc := loadZone(t, zone)
			start := time.Date(2026, 7, 20, 0, 0, 0, 0, loc)
			until := time.Date(2026, 7, 25, 0, 0, 0, 0, loc)
			obj, err := NewEventObject(EventDraft{
				Summary: "all-day daily",
				Start:   start,
				AllDay:  true,
				Recur:   &RecurSpec{Freq: FreqDaily, Until: &until},
			}, time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
			if err != nil {
				t.Fatalf("NewEventObject: %v", err)
			}

			if got := untilValue(rruleOf(t, obj)); got != "20260725" {
				t.Errorf("stored UNTIL = %q, want DATE 20260725 (rule %q)", got, rruleOf(t, obj))
			}

			got := occurrenceDays(t, roundTrip(t, obj, loc), loc)
			want := []string{"2026-07-20", "2026-07-21", "2026-07-22", "2026-07-23", "2026-07-24", "2026-07-25"}
			assertDays(t, got, want)
		})
	}
}

// TestAllDayEndsOnDateExcludesDayAfter closes the class on the other side: an
// UNTIL one day earlier must still stop at 07-24 — the fix must not simply
// widen every all-day series by a day.
func TestAllDayEndsOnDateExcludesDayAfter(t *testing.T) {
	for _, zone := range allDayUntilZones {
		t.Run(zone, func(t *testing.T) {
			loc := loadZone(t, zone)
			start := time.Date(2026, 7, 20, 0, 0, 0, 0, loc)
			until := time.Date(2026, 7, 24, 0, 0, 0, 0, loc)
			obj, err := NewEventObject(EventDraft{
				Summary: "all-day daily",
				Start:   start,
				AllDay:  true,
				Recur:   &RecurSpec{Freq: FreqDaily, Until: &until},
			}, time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
			if err != nil {
				t.Fatalf("NewEventObject: %v", err)
			}
			if got := untilValue(rruleOf(t, obj)); got != "20260724" {
				t.Errorf("stored UNTIL = %q, want DATE 20260724", got)
			}
			assertDays(t, occurrenceDays(t, roundTrip(t, obj, loc), loc), []string{
				"2026-07-20", "2026-07-21", "2026-07-22", "2026-07-23", "2026-07-24",
			})
		})
	}
}

// roundTrip re-encodes obj and decodes it in loc — the .ics is the source of
// truth, and NewEventObject parses in time.Local, so expanding in a chosen zone
// means going back through the bytes the way a reload does.
func roundTrip(t *testing.T, obj *Parsed, loc *time.Location) *Parsed {
	t.Helper()
	raw, err := obj.Encode()
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	out, err := Decode(raw, loc)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	return out
}

// TestAllDayEndsOnDateForeignClientReadback feeds the DATE UNTIL back in as raw
// bytes — what a foreign client (NextCloud web, a phone) actually stores and
// what LazyPlanner re-reads on the next sync — and asserts the same inclusive
// reading. A fix that only patched the in-memory spec would pass the tests above
// and still drop D after a round trip through the .ics.
func TestAllDayEndsOnDateForeignClientReadback(t *testing.T) {
	ics := "BEGIN:VCALENDAR\r\nVERSION:2.0\r\nPRODID:-//x//x//EN\r\n" +
		"BEGIN:VEVENT\r\nUID:foreign-1\r\nDTSTAMP:20260101T000000Z\r\n" +
		"DTSTART;VALUE=DATE:20260720\r\nRRULE:FREQ=DAILY;UNTIL=20260725\r\n" +
		"SUMMARY:foreign all-day\r\nEND:VEVENT\r\nEND:VCALENDAR\r\n"
	for _, zone := range allDayUntilZones {
		t.Run(zone, func(t *testing.T) {
			loc := loadZone(t, zone)
			obj, err := Decode([]byte(ics), loc)
			if err != nil {
				t.Fatalf("decode: %v", err)
			}
			assertDays(t, occurrenceDays(t, obj, loc), []string{
				"2026-07-20", "2026-07-21", "2026-07-22", "2026-07-23", "2026-07-24", "2026-07-25",
			})
		})
	}
}

// TestAllDayRecurringTodoEndsOnDateAdvancesToEndDay is the VTODO twin: an
// all-day recurring task due the day before D must advance to D on complete,
// not report the series exhausted.
func TestAllDayRecurringTodoEndsOnDateAdvancesToEndDay(t *testing.T) {
	for _, zone := range allDayUntilZones {
		t.Run(zone, func(t *testing.T) {
			loc := loadZone(t, zone)
			due := time.Date(2026, 7, 24, 0, 0, 0, 0, loc)
			until := time.Date(2026, 7, 25, 0, 0, 0, 0, loc)
			now := time.Date(2026, 7, 24, 12, 0, 0, 0, loc)
			obj := NewTodoObject(TodoDraft{
				Summary:   "all-day daily task",
				HasDue:    true,
				Due:       due,
				DueAllDay: true,
				Recur:     &RecurSpec{Freq: FreqDaily, Until: &until},
			}, now)

			if got := untilValue(rruleOf(t, obj)); got != "20260725" {
				t.Errorf("stored UNTIL = %q, want DATE 20260725", got)
			}

			uid := obj.Todos[0].UID
			out, exhausted, err := AdvanceRecurringTodo(obj, uid, now, loc)
			if err != nil {
				t.Fatalf("advance: %v", err)
			}
			if exhausted {
				t.Fatalf("series reported exhausted; 2026-07-25 is still due (UNTIL=20260725)")
			}
			if got := out.Todos[0].Due.In(loc).Format("2006-01-02"); got != "2026-07-25" {
				t.Errorf("advanced due = %s, want 2026-07-25", got)
			}
		})
	}
}

// TestAllDaySplitPreservesEndDayExactlyOnce guards the other side of the same
// bound: a this-and-future split of an all-day series must partition the days —
// no day lost at the cut, no day emitted by both halves, and the final day D
// still present. The capped half's UNTIL and the future half's inherited UNTIL
// are both DATE values, so both go through the corrected reading.
func TestAllDaySplitPreservesEndDayExactlyOnce(t *testing.T) {
	for _, zone := range allDayUntilZones {
		t.Run(zone, func(t *testing.T) {
			loc := loadZone(t, zone)
			ics := "BEGIN:VCALENDAR\r\nVERSION:2.0\r\nPRODID:-//x//x//EN\r\n" +
				"BEGIN:VEVENT\r\nUID:split-1\r\nDTSTAMP:20260101T000000Z\r\n" +
				"DTSTART;VALUE=DATE:20260720\r\nRRULE:FREQ=DAILY;UNTIL=20260725\r\n" +
				"SUMMARY:all-day\r\nEND:VEVENT\r\nEND:VCALENDAR\r\n"
			obj, err := Decode([]byte(ics), loc)
			if err != nil {
				t.Fatalf("decode: %v", err)
			}
			occ := time.Date(2026, 7, 23, 0, 0, 0, 0, loc)
			now := time.Date(2026, 7, 1, 0, 0, 0, 0, loc)
			capped, future, err := SplitEvent(obj, "split-1", occ, EventDraft{
				Summary: "moved", Start: occ, AllDay: true,
			}, now, loc)
			if err != nil {
				t.Fatalf("split: %v", err)
			}
			assertDays(t, occurrenceDays(t, roundTrip(t, capped, loc), loc), []string{
				"2026-07-20", "2026-07-21", "2026-07-22",
			})
			assertDays(t, occurrenceDays(t, roundTrip(t, future, loc), loc), []string{
				"2026-07-23", "2026-07-24", "2026-07-25",
			})
		})
	}
}

// occurrenceDays expands obj over July 2026 and returns each occurrence's local
// calendar date.
func occurrenceDays(t *testing.T, obj *Parsed, loc *time.Location) []string {
	t.Helper()
	from := time.Date(2026, 7, 1, 0, 0, 0, 0, loc)
	to := time.Date(2026, 8, 1, 0, 0, 0, 0, loc)
	occs, err := obj.EventOccurrences(from, to)
	if err != nil {
		t.Fatalf("EventOccurrences: %v", err)
	}
	days := make([]string, 0, len(occs))
	for _, o := range occs {
		days = append(days, o.Start.In(loc).Format("2006-01-02"))
	}
	return days
}

func assertDays(t *testing.T, got, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("occurrences = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("occurrences = %v, want %v", got, want)
		}
	}
}
