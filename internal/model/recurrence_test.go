package model_test

import (
	"testing"
	"time"

	"github.com/littekge/LazyPlanner/internal/model"
)

// wide is a window large enough to contain every fixture's full series.
func wide() (time.Time, time.Time) {
	return time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC)
}

func starts(occs []model.Occurrence) []time.Time {
	out := make([]time.Time, len(occs))
	for i, o := range occs {
		out[i] = o.Start
	}
	return out
}

func assertInstants(t *testing.T, got []time.Time, want []time.Time) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("got %d occurrences, want %d\n got:  %v\n want: %v", len(got), len(want), utc(got), utc(want))
	}
	for i := range want {
		if !got[i].Equal(want[i]) {
			t.Errorf("occurrence %d = %s, want %s", i, got[i].UTC(), want[i].UTC())
		}
	}
}

func utc(ts []time.Time) []string {
	out := make([]string, len(ts))
	for i, t := range ts {
		out[i] = t.UTC().Format("2006-01-02T15:04Z")
	}
	return out
}

// TestOccurrencesWeeklyDST is the headline recurrence case: a weekly event that
// crosses the spring-forward boundary must keep its 09:00 wall-clock time while
// its UTC instant shifts by an hour.
func TestOccurrencesWeeklyDST(t *testing.T) {
	loc := testLoc(t)
	ev := onlyEvent(t, decode(t, "recur_weekly_dst.ics", loc))
	from, to := wide()

	occs, err := ev.Occurrences(from, to)
	if err != nil {
		t.Fatal(err)
	}

	want := []time.Time{
		time.Date(2026, 3, 4, 14, 0, 0, 0, time.UTC),  // 09:00 EST
		time.Date(2026, 3, 11, 13, 0, 0, 0, time.UTC), // 09:00 EDT (after DST)
		time.Date(2026, 3, 18, 13, 0, 0, 0, time.UTC),
		time.Date(2026, 3, 25, 13, 0, 0, 0, time.UTC),
		time.Date(2026, 4, 1, 13, 0, 0, 0, time.UTC),
		time.Date(2026, 4, 8, 13, 0, 0, 0, time.UTC), // UNTIL boundary (inclusive)
	}
	assertInstants(t, starts(occs), want)

	for i, o := range occs {
		local := o.Start.In(loc)
		if local.Hour() != 9 || local.Minute() != 0 {
			t.Errorf("occurrence %d local time = %s, want 09:00 wall-clock", i, local.Format("15:04"))
		}
		if got := o.End.Sub(o.Start); got != 30*time.Minute {
			t.Errorf("occurrence %d duration = %s, want 30m", i, got)
		}
		if o.Event != ev {
			t.Errorf("occurrence %d Event pointer does not reference the source event", i)
		}
	}
}

func TestOccurrencesExdate(t *testing.T) {
	loc := testLoc(t)
	ev := onlyEvent(t, decode(t, "recur_exdate.ics", loc))
	from, to := wide()

	occs, err := ev.Occurrences(from, to)
	if err != nil {
		t.Fatal(err)
	}
	// DAILY COUNT=4 from 3/4 is 3/4,3/5,3/6,3/7; EXDATE removes 3/6.
	want := []time.Time{
		time.Date(2026, 3, 4, 9, 0, 0, 0, loc),
		time.Date(2026, 3, 5, 9, 0, 0, 0, loc),
		time.Date(2026, 3, 7, 9, 0, 0, 0, loc),
	}
	assertInstants(t, starts(occs), want)
}

// TestOccurrencesExdateWindowsZone: an EXDATE carrying a Windows/Outlook TZID
// ("Eastern Standard Time") must resolve via the same resilient path as DTSTART,
// not error out and blank the whole event's expansion.
func TestOccurrencesExdateWindowsZone(t *testing.T) {
	loc := testLoc(t)
	ev := onlyEvent(t, decode(t, "recur_exdate_winzone.ics", loc))
	from, to := wide()

	occs, err := ev.Occurrences(from, to)
	if err != nil {
		t.Fatalf("Windows-zone EXDATE should expand, not error: %v", err)
	}
	// Same as recur_exdate: EXDATE (mapped to America/New_York) removes 3/6.
	want := []time.Time{
		time.Date(2026, 3, 4, 9, 0, 0, 0, loc),
		time.Date(2026, 3, 5, 9, 0, 0, 0, loc),
		time.Date(2026, 3, 7, 9, 0, 0, 0, loc),
	}
	assertInstants(t, starts(occs), want)
}

func TestOccurrencesAllDayRecurring(t *testing.T) {
	loc := testLoc(t)
	ev := onlyEvent(t, decode(t, "recur_allday.ics", loc))
	from := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, 7, 10, 0, 0, 0, 0, time.UTC)

	occs, err := ev.Occurrences(from, to)
	if err != nil {
		t.Fatal(err)
	}
	want := []time.Time{
		time.Date(2026, 7, 4, 0, 0, 0, 0, loc),
		time.Date(2026, 7, 5, 0, 0, 0, 0, loc),
		time.Date(2026, 7, 6, 0, 0, 0, 0, loc),
	}
	assertInstants(t, starts(occs), want)
	for i, o := range occs {
		if !o.Event.AllDay {
			t.Errorf("occurrence %d not marked all-day", i)
		}
		if got := o.End.Sub(o.Start); got != 24*time.Hour {
			t.Errorf("occurrence %d duration = %s, want 24h", i, got)
		}
	}
}

func TestOccurrencesRDateOnly(t *testing.T) {
	loc := testLoc(t)
	ev := onlyEvent(t, decode(t, "recur_rdate.ics", loc))
	from, to := wide()

	occs, err := ev.Occurrences(from, to)
	if err != nil {
		t.Fatal(err)
	}
	// DTSTART plus one RDATE, no RRULE. DTSTART must be included per RFC 5545.
	want := []time.Time{
		time.Date(2026, 7, 4, 9, 0, 0, 0, time.UTC),
		time.Date(2026, 7, 6, 9, 0, 0, 0, time.UTC),
	}
	assertInstants(t, starts(occs), want)
}

func TestOccurrencesWindowing(t *testing.T) {
	loc := testLoc(t)
	ev := onlyEvent(t, decode(t, "recur_weekly_dst.ics", loc))

	t.Run("narrow window yields only the contained instance", func(t *testing.T) {
		from := time.Date(2026, 3, 11, 0, 0, 0, 0, time.UTC)
		to := time.Date(2026, 3, 12, 0, 0, 0, 0, time.UTC)
		occs, err := ev.Occurrences(from, to)
		if err != nil {
			t.Fatal(err)
		}
		assertInstants(t, starts(occs), []time.Time{time.Date(2026, 3, 11, 13, 0, 0, 0, time.UTC)})
	})

	t.Run("window between instances yields none", func(t *testing.T) {
		from := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
		to := time.Date(2026, 6, 2, 0, 0, 0, 0, time.UTC)
		occs, err := ev.Occurrences(from, to)
		if err != nil {
			t.Fatal(err)
		}
		if len(occs) != 0 {
			t.Errorf("got %d occurrences, want 0", len(occs))
		}
	})
}

// TestOccurrencesNonRecurringOverlap checks the window logic for a plain
// multi-day event: a window starting inside the event must still return it,
// even though the event started earlier.
func TestOccurrencesNonRecurringOverlap(t *testing.T) {
	loc := testLoc(t)
	ev := onlyEvent(t, decode(t, "event_allday.ics", loc)) // 7/4 .. 7/6 (exclusive)

	t.Run("window inside the span includes the event", func(t *testing.T) {
		from := time.Date(2026, 7, 5, 0, 0, 0, 0, loc)
		to := time.Date(2026, 7, 5, 12, 0, 0, 0, loc)
		occs, err := ev.Occurrences(from, to)
		if err != nil {
			t.Fatal(err)
		}
		if len(occs) != 1 || !occs[0].Start.Equal(time.Date(2026, 7, 4, 0, 0, 0, 0, loc)) {
			t.Fatalf("got %v, want single occurrence starting 7/4", utc(starts(occs)))
		}
	})

	t.Run("window before the span is empty", func(t *testing.T) {
		from := time.Date(2026, 7, 1, 0, 0, 0, 0, loc)
		to := time.Date(2026, 7, 2, 0, 0, 0, 0, loc)
		occs, err := ev.Occurrences(from, to)
		if err != nil {
			t.Fatal(err)
		}
		if len(occs) != 0 {
			t.Errorf("got %d occurrences, want 0", len(occs))
		}
	})
}

// TestEventOccurrencesOverride checks RECURRENCE-ID handling: the overridden
// instance is moved to its new time with its new details, and the original slot
// is suppressed.
func TestEventOccurrencesOverride(t *testing.T) {
	loc := testLoc(t)
	p := decode(t, "recur_override.ics", loc)
	from, to := wide()

	occs, err := p.EventOccurrences(from, to)
	if err != nil {
		t.Fatal(err)
	}

	want := []time.Time{
		time.Date(2026, 3, 4, 14, 0, 0, 0, time.UTC),  // master, 09:00 EST
		time.Date(2026, 3, 11, 18, 0, 0, 0, time.UTC), // override, moved to 14:00 EDT
		time.Date(2026, 3, 18, 13, 0, 0, 0, time.UTC), // master, 09:00 EDT
	}
	assertInstants(t, starts(occs), want)

	if occs[1].Event.Summary != "Team standup (moved to afternoon)" {
		t.Errorf("moved instance summary = %q, want the override's summary", occs[1].Event.Summary)
	}
	if occs[0].Event.Summary != "Team standup" || occs[2].Event.Summary != "Team standup" {
		t.Errorf("master instances should keep the master summary; got %q and %q",
			occs[0].Event.Summary, occs[2].Event.Summary)
	}
	// The original 3/11 09:00 slot must not also appear.
	for _, o := range occs {
		if o.Start.Equal(time.Date(2026, 3, 11, 13, 0, 0, 0, time.UTC)) {
			t.Error("original overridden slot (3/11 09:00) should be suppressed")
		}
	}
}
