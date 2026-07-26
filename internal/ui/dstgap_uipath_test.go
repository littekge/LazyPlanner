package ui

import (
	"context"
	"testing"
	"time"

	_ "time/tzdata"

	"github.com/littekge/LazyPlanner/internal/model"
	"github.com/littekge/LazyPlanner/internal/store"
)

// gapUIZone loads a zone, failing hard rather than skipping: a t.Skip here would
// make this guard a vacuous pass on a host without tzdata files, and internal/ui
// imports time/tzdata precisely so that cannot happen.
func gapUIZone(t *testing.T, name string) *time.Location {
	t.Helper()
	loc, err := time.LoadLocation(name)
	if err != nil {
		t.Fatalf("zone %q must load (time/tzdata is imported): %v", name, err)
	}
	return loc
}

// Pressing Space on a recurring task the day before a DST-gap day must advance it
// ONTO that day. This is the end-to-end guard for the gap-safe recurrence fix: the
// model-level guards in internal/model prove the arithmetic, but only driving the
// real Space handler proves the app actually stores the corrected date — the value
// is persisted and pushed to the server, so a skipped day is permanent.
//
// America/Havana springs forward AT local midnight, so 2026-03-08 00:00 does not
// exist there. rrule-go's day enumeration collapsed that day into 03-07 and its
// own duplicate filter then dropped it, so the series jumped 03-07 -> 03-09.
func TestSpaceOnRecurringTaskDoesNotSkipDSTGapDay(t *testing.T) {
	havana := gapUIZone(t, "America/Havana")

	// Build the clock in the zone the app renders and writes in (a.loc), not UTC —
	// a UTC-built clock would disagree with the day the app buckets items into.
	now := time.Date(2026, 3, 7, 12, 0, 0, 0, havana)
	a := newRootedTestApp(t, now)
	a.loc = havana
	if err := a.store.CreateCalendarLocal(context.Background(), "tl",
		store.CalendarMeta{DisplayName: "TL"}, []string{"VTODO"}); err != nil {
		t.Fatal(err)
	}

	// An all-day recurring task due the day before the gap. Date-only DUE is the
	// shape a quick-added daily task takes, and its anchor IS local midnight, which
	// is exactly the reading the gap destroys.
	uid := "plants@gap"
	ics := "BEGIN:VCALENDAR\r\nVERSION:2.0\r\nPRODID:-//t//t//EN\r\nBEGIN:VTODO\r\nUID:" + uid +
		"\r\nSUMMARY:Water the plants\r\nDTSTAMP:20260301T000000Z" +
		"\r\nDUE;VALUE=DATE:20260307\r\nRRULE:FREQ=DAILY\r\nEND:VTODO\r\nEND:VCALENDAR\r\n"
	parsed, err := model.Decode([]byte(ics), havana)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := a.store.Put(context.Background(), "tl", store.ResourceName(uid), parsed); err != nil {
		t.Fatal(err)
	}
	a.reload()

	// Start where the user does: the Tasks tree with the task selected, then Space.
	a.setMode(modeTasks)
	a.treeListID = "tl"
	a.buildTreeForList("tl")
	a.selectTreeByUID(uid)
	a.toggleComplete()

	loc, ok := a.store.Locate(uid)
	if !ok {
		t.Fatal("task vanished after Space")
	}
	td := findTodo(loc.Object, uid)
	if td == nil {
		t.Fatal("todo missing from its own resource after Space")
	}
	if td.Completed() {
		t.Fatal("an unbounded daily series must advance, not complete")
	}
	if got := td.Due.In(havana).Format("2006-01-02"); got != "2026-03-08" {
		t.Errorf("DUE after Space = %s, want 2026-03-08 — the DST-gap day was skipped", got)
	}
}

// The recovered occurrence must be reachable in the calendar the same way any
// other occurrence is: a recurring event on the gap day has to appear in the day's
// agenda, or the user sees a hole in the series even though the data is right.
func TestRecurringEventOnDSTGapDayIsVisible(t *testing.T) {
	havana := gapUIZone(t, "America/Havana")
	now := time.Date(2026, 3, 8, 12, 0, 0, 0, havana)
	a := newRootedTestApp(t, now)
	a.loc = havana
	calID := testCalID(a)

	uid := "standup@gap"
	ics := "BEGIN:VCALENDAR\r\nVERSION:2.0\r\nPRODID:-//t//t//EN\r\nBEGIN:VEVENT\r\nUID:" + uid +
		"\r\nSUMMARY:Standup\r\nDTSTAMP:20260301T000000Z" +
		"\r\nDTSTART;TZID=America/Havana:20260306T000000" +
		"\r\nDTEND;TZID=America/Havana:20260306T003000" +
		"\r\nRRULE:FREQ=DAILY\r\nEND:VEVENT\r\nEND:VCALENDAR\r\n"
	parsed, err := model.Decode([]byte(ics), havana)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := a.store.Put(context.Background(), calID, store.ResourceName(uid), parsed); err != nil {
		t.Fatal(err)
	}
	a.reload()

	items := a.dayItems(time.Date(2026, 3, 8, 0, 0, 0, 0, havana))
	found := 0
	for _, it := range items {
		if it.Event != nil && it.Event.UID == uid {
			found++
		}
	}
	if found != 1 {
		t.Errorf("occurrences of the daily series on the gap day 2026-03-08 = %d, want 1", found)
	}
}
