package store_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/littekge/LazyPlanner/internal/model"
	"github.com/littekge/LazyPlanner/internal/store"
)

// eventICS builds a single-VEVENT calendar object with a summary and start.
func eventICS(t *testing.T, uid, summary string, startHour int) *model.Parsed {
	t.Helper()
	start := time.Date(2026, 7, 4, startHour, 0, 0, 0, time.UTC).Format("20060102T150405Z")
	end := time.Date(2026, 7, 4, startHour+1, 0, 0, 0, time.UTC).Format("20060102T150405Z")
	ics := "BEGIN:VCALENDAR\r\nVERSION:2.0\r\nPRODID:-//LazyPlanner//Test//EN\r\n" +
		"BEGIN:VEVENT\r\nUID:" + uid + "\r\nDTSTAMP:20260701T120000Z\r\n" +
		"DTSTART:" + start + "\r\nDTEND:" + end + "\r\n" +
		"SUMMARY:" + summary + "\r\nEND:VEVENT\r\nEND:VCALENDAR\r\n"
	obj, err := model.Decode([]byte(ics), time.UTC)
	if err != nil {
		t.Fatalf("decoding %q: %v", uid, err)
	}
	return obj
}

func findEvt(obj *model.Parsed, uid string) *model.Event {
	for _, e := range obj.Events {
		if e.UID == uid {
			return e
		}
	}
	return nil
}

// TestGrabNudgeDoesNotClobberConcurrentPull guards pass-11 LOW #6: the grab path's
// Locate-then-Put had no version check, so a sync pull that landed between Locate
// and Put was silently clobbered — the pulled ETag was adopted while the written
// content was the pre-pull object, so the next push's ETag CAS matched and
// overwrote the server's edit. grabNudge now commits via PutIfUnchanged(expected),
// which skips the write (applied=false) when the resource changed underneath.
//
// It replays the store-level sequence grabNudge performs:
//  1. Locate(uid) -> loc.Prev (the pre-grab snapshot) + loc.Object
//  2. (concurrently) sync PullRemote replaces the cached resource with a
//     server-updated version bearing a new ETag
//  3. newObj := model.EditEvent(loc.Object, ...) -- derived from the STALE object
//  4. store.PutIfUnchanged(newObj, loc.Prev) -- skipped because the resource moved
func TestGrabNudgeDoesNotClobberConcurrentPull(t *testing.T) {
	ctx := context.Background()
	s, err := store.Open(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	const cal, uid = "personal", "ev1"
	name := store.ResourceName(uid)
	href := "/dav/personal/ev1.ics"

	// Seed a clean, synced resource (ETag etag-v1) as a normal sync would.
	base := eventICS(t, uid, "Team Sync", 13)
	if _, err := s.PullRemote(ctx, cal, name, base, "etag-v1", href, nil); err != nil {
		t.Fatalf("seed pull: %v", err)
	}

	// (1) grab begins: Locate captures the current object + snapshot.
	loc, ok := s.Locate(uid)
	if !ok {
		t.Fatal("Locate failed")
	}
	if loc.Prev.ETag != "etag-v1" {
		t.Fatalf("located ETag = %q, want etag-v1", loc.Prev.ETag)
	}

	// (2) A background sync pull lands: the server changed the summary (a field the
	// nudge does not touch). expectedPrev is the snapshot the sync captured, which
	// still equals loc.Prev, so the guarded pull APPLIES and swaps in etag-v2.
	serverEdit := eventICS(t, uid, "Team Sync -- MOVED TO ROOM B", 13)
	applied, err := s.PullRemote(ctx, cal, name, serverEdit, "etag-v2", href, loc.Prev)
	if err != nil {
		t.Fatalf("concurrent pull: %v", err)
	}
	if !applied {
		t.Fatal("concurrent pull was not applied; interleaving precondition not met")
	}

	// (3) The nudge computes newObj from the STALE loc.Object: move the event +1 day,
	// carrying loc.Object's (old) summary. This is draftFromEvent + EditEvent.
	ev := findEvt(loc.Object, uid)
	if ev == nil {
		t.Fatal("event missing from located object")
	}
	d := model.EventDraft{
		Summary:     ev.Summary, // "Team Sync" -- the stale value
		Description: ev.Description,
		Location:    ev.Location,
		Start:       ev.Start.AddDate(0, 0, 1),
		End:         ev.End.AddDate(0, 0, 1),
		AllDay:      ev.AllDay,
	}
	newObj, err := model.EditEvent(loc.Object, uid, d, time.Now(), time.UTC)
	if err != nil {
		t.Fatalf("EditEvent: %v", err)
	}

	// (4) The nudge commits via the version-checked write with loc.Prev as the
	// expected snapshot. The concurrent pull replaced the resource, so the write
	// must be SKIPPED rather than clobber the server's edit.
	applied, err = s.PutIfUnchanged(ctx, cal, name, newObj, loc.Prev)
	if err != nil {
		t.Fatalf("PutIfUnchanged: %v", err)
	}
	if applied {
		t.Fatal("PutIfUnchanged applied over a concurrent pull — the version check did not fire")
	}

	// Inspect the resulting cached resource: the pulled server edit must survive
	// intact, at its ETag, and remain clean (nothing pending to clobber it).
	cs, ok := s.Calendar(cal)
	if !ok {
		t.Fatal("Calendar missing")
	}
	res := findResource(cs, name)
	if res == nil {
		t.Fatal("resource missing")
	}
	got := findEvt(res.Object, uid)
	if got == nil {
		t.Fatal("event missing")
	}
	t.Logf("after guarded nudge: applied=%v ETag=%q Dirty=%v Summary=%q", applied, res.ETag, res.Dirty, got.Summary)

	if !strings.Contains(got.Summary, "ROOM B") {
		t.Errorf("server edit lost: summary = %q, want the pulled %q", got.Summary, "Team Sync -- MOVED TO ROOM B")
	}
	if res.ETag != "etag-v2" || res.Dirty {
		t.Errorf("pulled resource disturbed: ETag=%q Dirty=%v, want etag-v2 / clean", res.ETag, res.Dirty)
	}
}
