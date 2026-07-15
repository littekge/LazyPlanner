package ui

import (
	"context"
	"testing"
	"time"

	"github.com/littekge/LazyPlanner/internal/model"
	"github.com/littekge/LazyPlanner/internal/store"
)

// TestGrabResizeRejectsZeroDuration closes a pass-12 escaped-canary hole: the only
// resize test grows the end (J) and never shrinks (K) to the equal boundary, so
// weakening the min-duration guard from !End.After(Start) to End.Before(Start) —
// which lets a K-resize collapse an event to end==start (zero duration) — escaped
// the suite. Shrinking a 1-hour event by one hour must be rejected, leaving the
// end unchanged and strictly after the start.
func TestGrabResizeRejectsZeroDuration(t *testing.T) {
	a := newWritableTestApp(t, time.Date(2026, 7, 5, 9, 0, 0, 0, time.UTC))
	ctx := context.Background()
	cal := a.store.Calendars()[0].ID
	start0 := time.Date(2026, 7, 15, 14, 0, 0, 0, time.UTC)
	end0 := time.Date(2026, 7, 15, 15, 0, 0, 0, time.UTC) // a 1-hour event
	obj, err := model.NewEventObject(model.EventDraft{Summary: "Meet", Start: start0, End: end0}, a.now)
	if err != nil {
		t.Fatal(err)
	}
	evUID := obj.Events[0].UID
	name := store.ResourceName(evUID)
	if _, err := a.store.Put(ctx, cal, name, obj); err != nil {
		t.Fatal(err)
	}

	// Grab it in week view (timed resize only applies there).
	a.setMode(modeCalendar)
	a.viewMode = viewWeek
	a.grabbing = true
	a.grabUID = evUID
	a.grabIsEvent = true
	a.grabCalID, a.grabName = cal, name
	if loc, ok := a.store.Locate(evUID); ok {
		a.grabPrev = loc.Prev
	}

	ev := func() *model.Event {
		loc, _ := a.store.Locate(evUID)
		return findEvent(loc.Object, evUID)
	}

	// K shrinks the end by 1h: 15:00 -> 14:00 == start. Must be rejected.
	a.grabNudge('K')
	if got := ev(); !got.End.After(got.Start) {
		t.Errorf("K-resize to end==start was allowed: start=%v end=%v (want end strictly after start)", got.Start, got.End)
	}
	if got := ev().End; !got.Equal(end0) {
		t.Errorf("a rejected resize must leave the end unchanged at %v, got %v", end0, got)
	}
}
