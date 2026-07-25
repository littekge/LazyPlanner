package ui

import (
	"context"
	"strconv"
	"testing"
	"time"

	"github.com/littekge/LazyPlanner/internal/model"
)

// TestGrabFutureCapDoesNotClobberConcurrentPull guards the sweep companion to
// finding #6 (reparent_clobber_test.go): beginGrabFuture's FIRST write — capping
// the master series before spawning the future tail — used a bare store.Put. A
// background sync pull landing between the grab's Locate and that Put was
// silently overwritten: the cap was derived from the stale snapshot, so the
// write persisted pre-pull content while adopting the freshly-pulled ETag, and
// the next push's CAS would have clobbered the remote edit. beginGrabFuture now
// commits the cap via PutIfUnchanged(loc.Prev), aborting the grab (not started)
// rather than clobbering when the resource changed underneath.
func TestGrabFutureCapDoesNotClobberConcurrentPull(t *testing.T) {
	ctx := context.Background()
	when := time.Date(2026, 7, 6, 12, 0, 0, 0, time.UTC)
	a := newWritableTestApp(t, when)
	a.mode = modeCalendar
	a.viewMode = viewWeek

	const cal, uid = "personal", "grab-future-clobber@rec"
	name := "grab-future-clobber.ics"
	href := "/dav/personal/" + name
	start := time.Date(2026, 7, 6, 9, 0, 0, 0, time.UTC)

	// Seed a clean, synced 4-week weekly series.
	if _, err := a.store.PullRemote(ctx, cal, name, weeklySeriesObj(t, uid, "Standup", start, 4), "etag-v1", href, nil); err != nil {
		t.Fatalf("seed pull: %v", err)
	}
	a.reload()

	// The grab's internal Locate captures the current object + snapshot.
	loc, ok := a.store.Locate(uid)
	if !ok {
		t.Fatal("Locate failed")
	}

	// A background sync pull lands: the server renamed the series (a field the
	// grab's cap does not touch). expectedPrev == loc.Prev, so it applies.
	applied, err := a.store.PullRemote(ctx, cal, name, weeklySeriesObj(t, uid, "SERVER RENAMED", start, 4), "etag-v2", href, loc.Prev)
	if err != nil {
		t.Fatalf("concurrent pull: %v", err)
	}
	if !applied {
		t.Fatal("concurrent pull not applied; interleaving precondition not met")
	}

	occ := time.Date(2026, 7, 20, 9, 0, 0, 0, time.UTC) // 3rd instance
	a.anchor = model.DayStart(occ)
	a.beginGrabFuture(loc, editTarget{uid: uid, occStart: occ, recurring: true})

	// The bare-Put bug would cap the master from the stale (pre-rename) object and
	// spawn a future series, discarding the server's rename. The fix must refuse
	// to start the grab instead.
	if a.grabbing {
		t.Error("grab should not have started over a stale snapshot")
	}

	got, ok := a.store.Locate(uid)
	if !ok {
		t.Fatal("series vanished")
	}
	ev := findEvent(got.Object, uid)
	if ev == nil {
		t.Fatal("event missing from its resource")
	}
	if ev.Summary != "SERVER RENAMED" {
		t.Errorf("CLOBBER: beginGrabFuture's cap overwrote the concurrent pull; summary reads %q, want %q", ev.Summary, "SERVER RENAMED")
	}
	occs, err := got.Object.EventOccurrences(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if len(occs) != 4 {
		t.Errorf("series was capped despite the stale-write guard: %d occurrences, want 4", len(occs))
	}
}

// weeklySeriesObj builds a single-VEVENT weekly-recurring object with a fixed
// UID/summary/count, so a test can control the resource identity and simulate a
// rename landing from another device.
func weeklySeriesObj(t *testing.T, uid, summary string, start time.Time, count int) *model.Parsed {
	t.Helper()
	ics := "BEGIN:VCALENDAR\r\nVERSION:2.0\r\nPRODID:-//LazyPlanner//Test//EN\r\n" +
		"BEGIN:VEVENT\r\nUID:" + uid + "\r\nDTSTAMP:20260701T120000Z\r\nSUMMARY:" + summary +
		"\r\nDTSTART:" + start.UTC().Format("20060102T150405Z") +
		"\r\nDTEND:" + start.Add(time.Hour).UTC().Format("20060102T150405Z") +
		"\r\nRRULE:FREQ=WEEKLY;COUNT=" + strconv.Itoa(count) + "\r\nEND:VEVENT\r\nEND:VCALENDAR\r\n"
	obj, err := model.Decode([]byte(ics), time.UTC)
	if err != nil {
		t.Fatalf("decode weekly series: %v", err)
	}
	return obj
}
