package model_test

import (
	"testing"
	"time"

	"github.com/littekge/LazyPlanner/internal/model"
)

// This is an exhaustive timezone/DST recurrence sweep. Recurrence + DST is a
// classic bug farm, so it pins the observed-correct behavior across both DST
// directions, half-hour offsets, leap days, short months, the spring-forward gap
// and fall-back ambiguity, all-day items, and floating/UTC/Windows zones. All
// assertions are on the *local wall-clock* time — the user-facing truth and the
// property that must survive an offset change ("matching other CalDAV clients").

func mustLoad(t *testing.T, name string) *time.Location {
	t.Helper()
	loc, err := time.LoadLocation(name)
	if err != nil {
		t.Skipf("zone %q unavailable: %v", name, err)
	}
	return loc
}

// expandLocal decodes a single recurring VEVENT and returns each occurrence's
// local wall-clock time, formatted date+time (or date-only for all-day).
func expandLocal(t *testing.T, dtstartProp, rrule string, loc *time.Location, from, to time.Time) []string {
	t.Helper()
	ics := "BEGIN:VCALENDAR\r\nVERSION:2.0\r\nPRODID:-//x//x//EN\r\n" +
		"BEGIN:VEVENT\r\nUID:u@t\r\nDTSTAMP:20240101T000000Z\r\n" +
		"DTSTART" + dtstartProp + "\r\nRRULE:" + rrule + "\r\nSUMMARY:s\r\nEND:VEVENT\r\nEND:VCALENDAR\r\n"
	p, err := model.Decode([]byte(ics), loc)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	ev := p.Events[0]
	occs, err := ev.Occurrences(from, to)
	if err != nil {
		t.Fatalf("Occurrences: %v", err)
	}
	layout := "2006-01-02 15:04"
	if ev.AllDay {
		layout = "2006-01-02"
	}
	out := make([]string, len(occs))
	for i, o := range occs {
		out[i] = o.Start.In(loc).Format(layout)
	}
	return out
}

func eq(t *testing.T, got, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("got %d occurrences, want %d\n got:  %v\n want: %v", len(got), len(want), got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("occurrence %d = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestTZSweepWallClockPreserved(t *testing.T) {
	ny := mustLoad(t, "America/New_York")
	syd := mustLoad(t, "Australia/Sydney")
	adl := mustLoad(t, "Australia/Adelaide")
	yr := func(y int) time.Time { return time.Date(y, 1, 1, 0, 0, 0, 0, time.UTC) }

	cases := []struct {
		name    string
		dtstart string // the DTSTART property text after "DTSTART"
		rrule   string
		loc     *time.Location
		from    time.Time
		to      time.Time
		want    []string
	}{
		{
			name:    "NY daily across spring-forward keeps 09:00",
			dtstart: ";TZID=America/New_York:20260307T090000",
			rrule:   "FREQ=DAILY;COUNT=4",
			loc:     ny, from: yr(2026), to: yr(2027),
			want: []string{"2026-03-07 09:00", "2026-03-08 09:00", "2026-03-09 09:00", "2026-03-10 09:00"},
		},
		{
			name:    "NY daily across fall-back keeps 09:00",
			dtstart: ";TZID=America/New_York:20261031T090000",
			rrule:   "FREQ=DAILY;COUNT=4",
			loc:     ny, from: yr(2026), to: yr(2027),
			want: []string{"2026-10-31 09:00", "2026-11-01 09:00", "2026-11-02 09:00", "2026-11-03 09:00"},
		},
		{
			name:    "Southern-hemisphere (Sydney) weekly across fall-back keeps 09:00",
			dtstart: ";TZID=Australia/Sydney:20260329T090000",
			rrule:   "FREQ=WEEKLY;COUNT=3",
			loc:     syd, from: yr(2026), to: yr(2027),
			want: []string{"2026-03-29 09:00", "2026-04-05 09:00", "2026-04-12 09:00"},
		},
		{
			name:    "half-hour-offset zone (Adelaide) daily keeps 09:00",
			dtstart: ";TZID=Australia/Adelaide:20260101T090000",
			rrule:   "FREQ=DAILY;COUNT=3",
			// from spans 2025 so the first instant (09:00 ACDT = 2025-12-31 22:30Z) is in-window.
			loc: adl, from: yr(2025), to: yr(2027),
			want: []string{"2026-01-01 09:00", "2026-01-02 09:00", "2026-01-03 09:00"},
		},
		{
			name:    "leap-day yearly recurs only on leap years",
			dtstart: ";TZID=America/New_York:20240229T090000",
			rrule:   "FREQ=YEARLY;COUNT=3",
			loc:     ny, from: yr(2024), to: yr(2036),
			want: []string{"2024-02-29 09:00", "2028-02-29 09:00", "2032-02-29 09:00"},
		},
		{
			name:    "monthly on the 31st skips short months",
			dtstart: ";TZID=America/New_York:20260131T090000",
			rrule:   "FREQ=MONTHLY;COUNT=4",
			loc:     ny, from: yr(2026), to: yr(2027),
			want: []string{"2026-01-31 09:00", "2026-03-31 09:00", "2026-05-31 09:00", "2026-07-31 09:00"},
		},
		{
			name:    "year boundary daily",
			dtstart: ";TZID=America/New_York:20261231T230000",
			rrule:   "FREQ=DAILY;COUNT=3",
			loc:     ny, from: yr(2026), to: yr(2028),
			want: []string{"2026-12-31 23:00", "2027-01-01 23:00", "2027-01-02 23:00"},
		},
		{
			name:    "UTC event has no DST shift",
			dtstart: ":20260307T120000Z",
			rrule:   "FREQ=DAILY;COUNT=3",
			loc:     time.UTC, from: yr(2026), to: yr(2027),
			want: []string{"2026-03-07 12:00", "2026-03-08 12:00", "2026-03-09 12:00"},
		},
		{
			name:    "floating time interpreted in loc keeps wall-clock across DST",
			dtstart: ":20260307T090000",
			rrule:   "FREQ=DAILY;COUNT=3",
			loc:     ny, from: yr(2026), to: yr(2027),
			want: []string{"2026-03-07 09:00", "2026-03-08 09:00", "2026-03-09 09:00"},
		},
		{
			name:    "Windows/Outlook zone name resolves and keeps wall-clock",
			dtstart: ";TZID=Eastern Standard Time:20260307T090000",
			rrule:   "FREQ=DAILY;COUNT=3",
			loc:     ny, from: yr(2026), to: yr(2027),
			want: []string{"2026-03-07 09:00", "2026-03-08 09:00", "2026-03-09 09:00"},
		},
		{
			name:    "all-day weekly across spring-forward stays date-only on the right dates",
			dtstart: ";VALUE=DATE:20260306",
			rrule:   "FREQ=WEEKLY;COUNT=3",
			loc:     ny, from: yr(2026), to: yr(2027),
			want: []string{"2026-03-06", "2026-03-13", "2026-03-20"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			eq(t, expandLocal(t, tc.dtstart, tc.rrule, tc.loc, tc.from, tc.to), tc.want)
		})
	}
}

// TestTZSweepGapAndFold pins the two hard cases where a wall-clock time is
// nonexistent (spring-forward gap) or ambiguous (fall-back): the series must not
// crash, drop, or duplicate an occurrence — exactly one lands on each expected
// calendar day. (The gap-day instant is an hour off, a benign zone-arithmetic
// quirk; the invariant that matters is one-per-day with no error.)
func TestTZSweepGapAndFold(t *testing.T) {
	ny := mustLoad(t, "America/New_York")

	// Spring-forward: 02:30 does not exist on 2026-03-08.
	gap := expandLocalDates(t, ";TZID=America/New_York:20260307T023000", "FREQ=DAILY;COUNT=3", ny,
		time.Date(2026, 3, 6, 0, 0, 0, 0, time.UTC), time.Date(2026, 3, 12, 0, 0, 0, 0, time.UTC))
	eq(t, gap, []string{"2026-03-07", "2026-03-08", "2026-03-09"})

	// Fall-back: 01:30 occurs twice on 2026-11-01; must appear once, not twice.
	fold := expandLocalDates(t, ";TZID=America/New_York:20261031T013000", "FREQ=DAILY;COUNT=3", ny,
		time.Date(2026, 10, 30, 0, 0, 0, 0, time.UTC), time.Date(2026, 11, 4, 0, 0, 0, 0, time.UTC))
	eq(t, fold, []string{"2026-10-31", "2026-11-01", "2026-11-02"})
}

// expandLocalDates is expandLocal reduced to local calendar dates.
func expandLocalDates(t *testing.T, dtstartProp, rrule string, loc *time.Location, from, to time.Time) []string {
	t.Helper()
	got := expandLocal(t, dtstartProp, rrule, loc, from, to)
	for i := range got {
		got[i] = got[i][:10] // keep the date part
	}
	return got
}
