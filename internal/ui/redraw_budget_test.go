package ui

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/rivo/tview"

	"github.com/littekge/LazyPlanner/internal/model"
	"github.com/littekge/LazyPlanner/internal/store"
)

// vdirApp builds an app over a throwaway vdir holding the given raw .ics bodies
// (keyed by file name) in one calendar.
func vdirApp(t *testing.T, ics map[string]string, now time.Time) *app {
	t.Helper()
	dir := t.TempDir()
	calDir := filepath.Join(dir, "calendars", "flood")
	if err := os.MkdirAll(calDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	for name, body := range ics {
		if err := os.WriteFile(filepath.Join(calDir, name), []byte(body), 0o644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
	s, err := store.Open(context.Background(), dir)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	a := newApp(s, "test", now)
	a.build()
	return a
}

// recurringICS is a single-event .ics body: one recurring event anchored at
// dtstart with the given RRULE.
func recurringICS(uid, dtstart, rrule string) string {
	return "BEGIN:VCALENDAR\r\nVERSION:2.0\r\nPRODID:-//LazyPlanner//Test//EN\r\n" +
		"BEGIN:VEVENT\r\nUID:" + uid + "\r\nDTSTAMP:20260701T120000Z\r\n" +
		"DTSTART:" + dtstart + "\r\nDURATION:PT1H\r\nRRULE:" + rrule + "\r\n" +
		"SUMMARY:" + uid + "\r\nEND:VEVENT\r\nEND:VCALENDAR\r\n"
}

// floodVdirApp builds an app over a vdir holding n resources, each a single
// far-anchored FREQ=SECONDLY event — the pass-21 pathological shape, reachable
// from any foreign/hostile server or a hand-edited vdir.
func floodVdirApp(t *testing.T, n int, now time.Time) *app {
	t.Helper()
	ics := make(map[string]string, n)
	for i := 0; i < n; i++ {
		uid := "flood-" + fmt.Sprint(i)
		ics[fmt.Sprintf("flood-%03d.ics", i)] = recurringICS(uid, "19000101T000000Z", "FREQ=SECONDLY")
	}
	// One ordinary event inside the window, so a test can tell "materialized
	// nothing because the range collapsed to a no-op" from "materialized the
	// range cheaply".
	ics["ordinary.ics"] = "BEGIN:VCALENDAR\r\nVERSION:2.0\r\nPRODID:-//LazyPlanner//Test//EN\r\n" +
		"BEGIN:VEVENT\r\nUID:" + floodOrdinaryUID + "\r\nDTSTAMP:20260701T120000Z\r\n" +
		"DTSTART:20260708T090000Z\r\nDTEND:20260708T100000Z\r\nSUMMARY:Ordinary\r\nEND:VEVENT\r\nEND:VCALENDAR\r\n"
	return vdirApp(t, ics, now)
}

// floodOrdinaryUID is the one non-pathological event floodVdirApp plants inside
// the query window (2026-07-08).
const floodOrdinaryUID = "ordinary-event"

func daysFrom(start time.Time, n int) []time.Time {
	days := make([]time.Time, n)
	for i := range days {
		days[i] = start.AddDate(0, 0, i)
	}
	return days
}

// TestRedrawBudgetIsPerRedrawNotPerCall guards the pass-23 MED finding: the
// aggregate model.StepBudget is minted inside each store call, so any UI path
// that queries the store once per day (the week grid's drill list, a SELECT
// day-range materialization) paid the full aggregate ceiling once per day —
// multiplying the bound straight back into a UI-thread freeze (measured 1.7s for
// a week rebuild, ~94s extrapolated for a 366-day SELECT range).
//
// The bound asserted here is a GROWTH RATIO against one budgeted store query,
// not wall-clock: the invariant is that a redraw costs about one budgeted
// expansion regardless of how many days it spans. Ratios also survive the noisy
// machines this suite runs on, where an absolute threshold would flake.
func TestRedrawBudgetIsPerRedrawNotPerCall(t *testing.T) {
	now := time.Date(2026, 7, 5, 12, 0, 0, 0, time.UTC)
	a := floodVdirApp(t, 60, now)

	weekStart := time.Date(2026, 7, 5, 0, 0, 0, 0, time.UTC)

	start := time.Now()
	occs, _ := a.store.EventOccurrencesVisible(weekStart, weekStart.AddDate(0, 0, 7), a.hidden)
	oneQuery := time.Since(start)
	t.Logf("single budgeted 7-day store query: %v (%d occurrences)", oneQuery, len(occs))

	measure := func(name string, ratio float64, fn func()) {
		t.Helper()
		s := time.Now()
		fn()
		d := time.Since(s)
		t.Logf("%s: %v (%.2fx one budgeted query)", name, d, float64(d)/float64(oneQuery))
		if float64(d) > ratio*float64(oneQuery) {
			t.Errorf("%s took %v = %.1fx a single budgeted query (%v), want <= %.1fx — "+
				"the StepBudget is being minted per store call inside a per-day loop, not once per redraw",
				name, d, float64(d)/float64(oneQuery), oneQuery, ratio)
		}
	}

	// A week rebuild spans 7 days; a month grid 42. Per-day querying would cost
	// 7x and 42x one budgeted query respectively.
	measure("week-view drill list (dayItemsForDays, 7 days)", 2, func() {
		a.dayItemsForDays(daysFrom(weekStart, 7))
	})
	measure("month grid (calItems, 42 days)", 3, func() {
		a.calItems(model.MonthGrid(now, a.weekStartMonday))
	})
	// The worst case: a full maxSelectDays materialization. Per-day querying cost
	// ~366x here, which is the ~94s freeze the finding measured.
	measure(fmt.Sprintf("%d-day range (dayItemsForDays)", maxSelectDays), 4, func() {
		a.dayItemsForDays(daysFrom(weekStart, maxSelectDays))
	})
}

// TestSelectDayRangeMaterializationSharesOneBudget drives the real production
// path for the second call site in the finding (daysRange, selection.go): a
// SELECT day-range spanning the maxSelectDays cap must materialize for about the
// cost of one budgeted expansion, not one per day.
func TestSelectDayRangeMaterializationSharesOneBudget(t *testing.T) {
	now := time.Date(2026, 7, 5, 12, 0, 0, 0, time.UTC)
	a := floodVdirApp(t, 60, now)
	a.root = tview.NewPages()
	a.root.AddPage(pageMain, a.layout(), true, true)
	a.setMode(modeCalendar)
	a.setFocus(a.calendarPrimitive())

	anchor := model.DayStart(now)
	start := time.Now()
	occs, _ := a.store.EventOccurrencesVisible(anchor, anchor.AddDate(0, 0, 7), a.hidden)
	oneQuery := time.Since(start)
	t.Logf("single budgeted 7-day store query: %v (%d occurrences)", oneQuery, len(occs))

	a.month.selected = anchor
	a.enterSelect()
	// Push the far end past the cap so daysRange materializes the full
	// maxSelectDays interval.
	a.month.selected = anchor.AddDate(0, 0, maxSelectDays+10)

	s := time.Now()
	targets := a.daysRange()
	d := time.Since(s)
	t.Logf("daysRange over %d days: %v (%.2fx one budgeted query, %d targets)",
		maxSelectDays, d, float64(d)/float64(oneQuery), len(targets))
	if float64(d) > 4*float64(oneQuery) {
		t.Errorf("daysRange took %v = %.1fx a single budgeted query (%v) — it is expanding per day, "+
			"so each day mints a fresh StepBudget and the aggregate ceiling is multiplied by %d",
			d, float64(d)/float64(oneQuery), oneQuery, maxSelectDays)
	}
	// The timing above only means something if the range really materialized: the
	// ordinary event inside it must be among the targets.
	found := false
	for _, tg := range targets {
		if tg.uid == floodOrdinaryUID {
			found = true
		}
	}
	if !found {
		t.Fatalf("daysRange materialized %d targets, none the ordinary in-range event %q — "+
			"the selection collapsed to a no-op, so the timing above proves nothing",
			len(targets), floodOrdinaryUID)
	}
}

// TestLegitimateCalendarNotStarvedByRedrawBudget closes the class the other way:
// bounding the freeze must not be paid for by silently truncating a real user's
// calendar. A perfectly ordinary calendar — daily and weekly series anchored
// years before the window, the shape that makes a recurrence iterator do real
// catch-up work — must expand FULLY under the shared per-redraw budget, on every
// day of a month grid and across a full maxSelectDays range.
func TestLegitimateCalendarNotStarvedByRedrawBudget(t *testing.T) {
	const nDaily, nWeekly = 25, 25
	// Anchored 5 years before the window: every expansion pays ~1800 catch-up
	// iterations, so a budget spent on repeated per-day work would show up here.
	anchorUTC := time.Date(2021, 7, 5, 9, 0, 0, 0, time.UTC) // a Monday
	ics := map[string]string{}
	for i := 0; i < nDaily; i++ {
		ics[fmt.Sprintf("daily-%03d.ics", i)] = recurringICS(fmt.Sprintf("daily-%03d", i), "20210705T090000Z", "FREQ=DAILY")
	}
	for i := 0; i < nWeekly; i++ {
		ics[fmt.Sprintf("weekly-%03d.ics", i)] = recurringICS(fmt.Sprintf("weekly-%03d", i), "20210705T090000Z", "FREQ=WEEKLY")
	}
	now := time.Date(2026, 7, 5, 12, 0, 0, 0, time.UTC)
	a := vdirApp(t, ics, now)

	// Expected count for one day: every daily series fires, and every weekly
	// series fires only on days a whole number of weeks after the anchor.
	want := func(day time.Time) int {
		n := nDaily
		if int(model.DayStart(day).Sub(model.DayStart(anchorUTC)).Hours()/24)%7 == 0 {
			n += nWeekly
		}
		return n
	}

	check := func(label string, days []time.Time, got map[string][]model.AgendaItem) {
		t.Helper()
		total, wantTotal := 0, 0
		for _, day := range days {
			w := want(day)
			wantTotal += w
			g := len(got[dayKey(day)])
			total += g
			if g != w {
				t.Errorf("%s: %s expanded to %d occurrences, want %d — the shared budget is starving a legitimate calendar",
					label, day.Format("2006-01-02"), g, w)
			}
		}
		t.Logf("%s: %d occurrences over %d days (want %d)", label, total, len(days), wantTotal)
	}

	weeks := model.MonthGrid(now, a.weekStartMonday)
	var gridDays []time.Time
	for _, w := range weeks {
		gridDays = append(gridDays, w...)
	}
	check("month grid", gridDays, a.calItems(weeks))

	rangeDays := daysFrom(model.DayStart(now), maxSelectDays)
	check(fmt.Sprintf("%d-day range", maxSelectDays), rangeDays, a.dayItemsForDays(rangeDays))

	// The batched expansion must also be item-for-item identical to the per-day
	// query it replaces — same predicate, same merge, one expansion.
	batched := a.dayItemsForDays(gridDays)
	for _, day := range gridDays {
		perDay := a.dayItems(day)
		got := batched[dayKey(day)]
		if len(got) != len(perDay) {
			t.Fatalf("%s: batched agenda has %d items, per-day query has %d", day.Format("2006-01-02"), len(got), len(perDay))
		}
		for i := range perDay {
			if got[i].Title != perDay[i].Title || !got[i].Start.Equal(perDay[i].Start) {
				t.Fatalf("%s item %d: batched %q@%v != per-day %q@%v",
					day.Format("2006-01-02"), i, got[i].Title, got[i].Start, perDay[i].Title, perDay[i].Start)
			}
		}
	}
}
