package ui

import (
	"context"
	"testing"
	"time"

	"github.com/littekge/LazyPlanner/internal/model"
	"github.com/littekge/LazyPlanner/internal/store"
)

// occCountFor returns how many occurrences of uid fall in [from,to), and whether
// any lands on the given day.
func occCountFor(t *testing.T, a *app, uid string, from, to, day time.Time) (int, bool) {
	t.Helper()
	occs, err := a.store.EventOccurrencesVisible(from, to, a.hidden)
	if err != nil {
		t.Fatal(err)
	}
	n, onDay := 0, false
	ds := model.DayStart(day.In(time.UTC))
	for _, o := range occs {
		if o.Event.UID != uid {
			continue
		}
		n++
		if model.DayStart(o.Start.In(time.UTC)).Equal(ds) {
			onDay = true
		}
	}
	return n, onDay
}

// grabAllDayMove puts a recurring event, grabs it at scope-all, moves +1 day, and
// commits — the exact path of the reported disappearance bug.
func grabAllDayMove(t *testing.T, spec model.RecurSpec) (*app, string, time.Time) {
	t.Helper()
	a := newWritableTestApp(t, time.Date(2026, 7, 5, 9, 0, 0, 0, time.UTC))
	a.setMode(modeCalendar)
	cal := a.store.Calendars()[0].ID
	start := time.Date(2026, 7, 6, 9, 0, 0, 0, time.UTC) // Monday
	obj, err := model.NewEventObject(model.EventDraft{
		Summary: "Standup", Start: start, End: start.Add(30 * time.Minute), Recur: &spec,
	}, a.now)
	if err != nil {
		t.Fatal(err)
	}
	uid := obj.Events[0].UID
	if _, err := a.store.Put(context.Background(), cal, store.ResourceName(uid), obj); err != nil {
		t.Fatal(err)
	}
	a.reload()

	loc, ok := a.store.Locate(uid)
	if !ok {
		t.Fatal("locate failed")
	}
	a.beginGrab(loc, editTarget{uid: uid, occStart: start, recurring: true}, scopeAll)
	if !a.grabbing {
		t.Fatal("did not enter grab mode")
	}
	a.grabNudge('l') // +1 day → Tuesday
	a.commitGrab()
	return a, uid, start.AddDate(0, 0, 1) // expected new day: Tue 2026-07-07
}

// TestGrabAllReanchorsWeeklyByday is the regression for the reported bug: grabbing
// a "Weekly on Monday" (BYDAY=MO) event at scope-all and moving it +1 day must
// move the whole series onto Tuesday — not leave it on Mondays with the moved
// instance vanished.
func TestGrabAllReanchorsWeeklyByday(t *testing.T) {
	from := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)

	a, uid, newDay := grabAllDayMove(t, model.RecurSpec{
		Freq: model.FreqWeekly, Weekdays: []time.Weekday{time.Monday},
	})

	n, onNewDay := occCountFor(t, a, uid, from, to, newDay)
	if n == 0 {
		t.Fatal("event VANISHED from the calendar after grab-all +1 day move")
	}
	if !onNewDay {
		t.Errorf("event has %d July occurrences but none on the moved-to day %s — the series didn't follow the move", n, newDay.Format("Mon 01-02"))
	}
	// Every occurrence should now be a Tuesday.
	occs, _ := a.store.EventOccurrencesVisible(from, to, a.hidden)
	for _, o := range occs {
		if o.Event.UID == uid && o.Start.In(time.UTC).Weekday() != time.Tuesday {
			t.Errorf("occurrence on %s is not a Tuesday — BYDAY was not re-anchored", o.Start.In(time.UTC).Format("Mon 01-02"))
		}
	}
}

// TestGrabAllPlainWeeklyStillMoves guards the already-working case (no BYDAY): the
// series follows the moved DTSTART, and nothing regressed.
func TestGrabAllPlainWeeklyStillMoves(t *testing.T) {
	from := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	a, uid, newDay := grabAllDayMove(t, model.RecurSpec{Freq: model.FreqWeekly})
	if _, onNewDay := occCountFor(t, a, uid, from, to, newDay); !onNewDay {
		t.Error("plain weekly series did not land on the moved-to day")
	}
}
