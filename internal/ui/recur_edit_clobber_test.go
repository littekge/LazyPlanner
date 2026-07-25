package ui

import (
	"context"
	"testing"
	"time"

	"github.com/littekge/LazyPlanner/internal/model"
)

// recurringTodoObj builds a single-VTODO recurring object with a fixed UID/
// summary/due/RRULE, so a test can control the resource identity and simulate a
// server edit landing from another device.
func recurringTodoObj(t *testing.T, uid, summary string, due time.Time, rrule string) *model.Parsed {
	t.Helper()
	ics := "BEGIN:VCALENDAR\r\nVERSION:2.0\r\nPRODID:-//LazyPlanner//Test//EN\r\n" +
		"BEGIN:VTODO\r\nUID:" + uid + "\r\nDTSTAMP:20260701T120000Z\r\nSUMMARY:" + summary +
		"\r\nDTSTART:" + due.Add(-time.Hour).UTC().Format("20060102T150405Z") +
		"\r\nDUE:" + due.UTC().Format("20060102T150405Z") +
		"\r\nRRULE:" + rrule + "\r\nEND:VTODO\r\nEND:VCALENDAR\r\n"
	obj, err := model.Decode([]byte(ics), time.UTC)
	if err != nil {
		t.Fatalf("decode recurring todo: %v", err)
	}
	return obj
}

// TestCommitDetachDoesNotClobberConcurrentPull guards the sweep companion to
// finding #6: commitDetach's FIRST write — advancing the series past the
// detached occurrence — used a bare store.Put. A background sync pull landing
// between the detach's Locate and that write was silently overwritten: the
// advance was derived from the stale snapshot, so the write persisted pre-pull
// content while adopting the freshly-pulled ETag, and the next push's CAS would
// have clobbered the remote edit. commitDetach now commits the advance via
// PutIfUnchanged(loc.Prev), skipping the whole detach (including the standalone
// creation) rather than clobbering when the resource changed underneath.
func TestCommitDetachDoesNotClobberConcurrentPull(t *testing.T) {
	ctx := context.Background()
	when := time.Date(2026, 7, 8, 0, 0, 0, 0, time.UTC)
	a := newRootedTestApp(t, when)

	const cal, uid = "personal", "detach-clobber@rec"
	name := "detach-clobber.ics"
	href := "/dav/personal/" + name
	due := time.Date(2026, 7, 6, 9, 0, 0, 0, time.UTC)

	if _, err := a.store.PullRemote(ctx, cal, name, recurringTodoObj(t, uid, "Water", due, "FREQ=WEEKLY;COUNT=3"), "etag-v1", href, nil); err != nil {
		t.Fatalf("seed pull: %v", err)
	}
	a.reload()

	// The detach's internal Locate captures the current object + snapshot.
	loc, ok := a.store.Locate(uid)
	if !ok {
		t.Fatal("Locate failed")
	}

	// A background sync pull lands: the server renamed the task (a field the
	// detach's advance does not touch). expectedPrev == loc.Prev, so it applies.
	applied, err := a.store.PullRemote(ctx, cal, name, recurringTodoObj(t, uid, "SERVER RENAMED", due, "FREQ=WEEKLY;COUNT=3"), "etag-v2", href, loc.Prev)
	if err != nil {
		t.Fatalf("concurrent pull: %v", err)
	}
	if !applied {
		t.Fatal("concurrent pull not applied; interleaving precondition not met")
	}

	// The detach proceeds from the STALE loc snapshot, exactly as
	// editTodoDetachForm's save callback does.
	advanced, _, err := model.AdvanceRecurringTodo(loc.Object, uid, a.now, a.loc)
	if err != nil {
		t.Fatal(err)
	}
	d := model.TodoDraft{Summary: "Water (one-off)", HasDue: true, Due: due, DueAllDay: false}
	standalone, newUID, err := model.DetachTodoOccurrence(loc.Object, uid, d, a.now, a.loc)
	if err != nil {
		t.Fatal(err)
	}

	a.commitDetach(loc, uid, newUID, advanced, standalone)

	// The bare-Put bug would advance the series from the stale (pre-rename)
	// object and create the standalone, discarding the server's rename. The fix
	// must skip the whole detach instead.
	got, ok := a.store.Locate(uid)
	if !ok {
		t.Fatal("series vanished")
	}
	td := findTodo(got.Object, uid)
	if td == nil {
		t.Fatal("todo missing from its resource")
	}
	if td.Summary != "SERVER RENAMED" {
		t.Errorf("CLOBBER: commitDetach's advance overwrote the concurrent pull; summary reads %q, want %q", td.Summary, "SERVER RENAMED")
	}
	if _, exists := a.store.Locate(newUID); exists {
		t.Error("standalone should not have been created when the version check skipped the advance")
	}
}

// TestCommitSplitDoesNotClobberConcurrentPull guards the sweep companion to
// finding #6: commitSplit's FIRST write — capping the master before spawning
// the future series — used a bare store.Put. A background sync pull landing
// between the split's Locate and that write was silently overwritten. commitSplit
// now commits the cap via PutIfUnchanged(loc.Prev), skipping the whole split
// (including the new future series) rather than clobbering.
func TestCommitSplitDoesNotClobberConcurrentPull(t *testing.T) {
	ctx := context.Background()
	when := time.Date(2026, 7, 6, 12, 0, 0, 0, time.UTC)
	a := newRootedTestApp(t, when)

	const cal, uid = "personal", "split-clobber@rec"
	name := "split-clobber.ics"
	href := "/dav/personal/" + name
	start := time.Date(2026, 7, 6, 9, 0, 0, 0, time.UTC)

	if _, err := a.store.PullRemote(ctx, cal, name, weeklySeriesObj(t, uid, "Standup", start, 4), "etag-v1", href, nil); err != nil {
		t.Fatalf("seed pull: %v", err)
	}
	a.reload()

	loc, ok := a.store.Locate(uid)
	if !ok {
		t.Fatal("Locate failed")
	}

	applied, err := a.store.PullRemote(ctx, cal, name, weeklySeriesObj(t, uid, "SERVER RENAMED", start, 4), "etag-v2", href, loc.Prev)
	if err != nil {
		t.Fatalf("concurrent pull: %v", err)
	}
	if !applied {
		t.Fatal("concurrent pull not applied; interleaving precondition not met")
	}

	occ := time.Date(2026, 7, 20, 9, 0, 0, 0, time.UTC) // 3rd instance = split point
	d := draftFromEvent(findEvent(loc.Object, uid))
	d.Start = occ
	capped, future, err := model.SplitEvent(loc.Object, uid, occ, d, a.now, a.loc)
	if err != nil {
		t.Fatal(err)
	}
	futureUID := future.Events[0].UID

	a.commitSplit(loc, uid, futureUID, capped, future, "edit this & future", "Split series (u to undo)")

	got, ok := a.store.Locate(uid)
	if !ok {
		t.Fatal("series vanished")
	}
	ev := findEvent(got.Object, uid)
	if ev == nil {
		t.Fatal("event missing from its resource")
	}
	if ev.Summary != "SERVER RENAMED" {
		t.Errorf("CLOBBER: commitSplit's cap overwrote the concurrent pull; summary reads %q, want %q", ev.Summary, "SERVER RENAMED")
	}
	occs, err := got.Object.EventOccurrences(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if len(occs) != 4 {
		t.Errorf("series was capped despite the stale-write guard: %d occurrences, want 4", len(occs))
	}
	if _, exists := a.store.Locate(futureUID); exists {
		t.Error("future series should not have been created when the version check skipped the cap")
	}
}
