package store_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/littekge/LazyPlanner/internal/model"
	"github.com/littekge/LazyPlanner/internal/store"
)

// todoICS builds a single-VTODO calendar object with a summary and a DESCRIPTION
// (a field completion does not touch — stands in for a note/priority edited on
// another device).
func todoWithDescICS(t *testing.T, uid, summary, description string) *model.Parsed {
	t.Helper()
	ics := "BEGIN:VCALENDAR\r\nVERSION:2.0\r\nPRODID:-//LazyPlanner//Test//EN\r\n" +
		"BEGIN:VTODO\r\nUID:" + uid + "\r\nDTSTAMP:20260701T120000Z\r\n" +
		"SUMMARY:" + summary + "\r\nDESCRIPTION:" + description + "\r\n" +
		"END:VTODO\r\nEND:VCALENDAR\r\n"
	obj, err := model.Decode([]byte(ics), time.UTC)
	if err != nil {
		t.Fatalf("decoding %q: %v", uid, err)
	}
	return obj
}

func findTdDesc(obj *model.Parsed, uid string) *model.Todo {
	for _, td := range obj.Todos {
		if td.UID == uid {
			return td
		}
	}
	return nil
}

// TestSpaceCompleteDoesNotClobberConcurrentPull guards pass-12 MED #5: the UI's
// toggleComplete (and advanceRecurringTodo) did Locate -> SetTodoCompleted(stale
// object) -> store.Put(unguarded), so a sync pull landing between Locate and Put
// was silently overwritten (the write adopted the pulled ETag while persisting
// stale content, and the next push's CAS matched the server). Both now commit via
// PutIfUnchanged(loc.Prev), which skips the write when the resource changed
// underneath — matching the grab path.
func TestSpaceCompleteDoesNotClobberConcurrentPull(t *testing.T) {
	ctx := context.Background()
	s, err := store.Open(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	const cal, uid = "personal", "task1"
	name := store.ResourceName(uid)
	href := "/dav/personal/task1.ics"

	// Seed a clean, synced todo (ETag etag-v1) as a normal sync would.
	base := todoWithDescICS(t, uid, "Buy groceries", "original note")
	if _, err := s.PullRemote(ctx, cal, name, base, "etag-v1", href, nil); err != nil {
		t.Fatalf("seed pull: %v", err)
	}

	// (1) Space pressed: toggleComplete's Locate captures the current object+snapshot.
	loc, ok := s.Locate(uid)
	if !ok {
		t.Fatal("Locate failed")
	}

	// (2) A background sync pull lands: the server changed the DESCRIPTION (a note
	// edited on another device — a field completion does not touch). expectedPrev
	// still equals loc.Prev, so the guarded pull APPLIES and swaps in etag-v2.
	serverEdit := todoWithDescICS(t, uid, "Buy groceries", "REMOTE EDIT from another device")
	applied, err := s.PullRemote(ctx, cal, name, serverEdit, "etag-v2", href, loc.Prev)
	if err != nil {
		t.Fatalf("concurrent pull: %v", err)
	}
	if !applied {
		t.Fatal("concurrent pull was not applied; interleaving precondition not met")
	}

	// (3) toggleComplete computes newObj from the STALE loc.Object: mark complete,
	// carrying loc.Object's (old) description.
	newObj, err := model.SetTodoCompleted(loc.Object, uid, true, time.Now(), time.UTC)
	if err != nil {
		t.Fatalf("SetTodoCompleted: %v", err)
	}

	// (4) toggleComplete now commits via the version-checked write. loc.Prev no
	// longer matches the pulled resource, so the write is SKIPPED.
	applied, err = s.PutIfUnchanged(ctx, cal, loc.Name, newObj, loc.Prev)
	if err != nil {
		t.Fatalf("PutIfUnchanged: %v", err)
	}
	if applied {
		t.Fatal("PutIfUnchanged applied over a concurrent pull — the version check did not fire")
	}

	// The pulled server edit must survive intact, at its ETag, and clean.
	cs, ok := s.Calendar(cal)
	if !ok {
		t.Fatal("Calendar missing")
	}
	res := findResource(cs, name)
	if res == nil {
		t.Fatal("resource missing")
	}
	got := findTdDesc(res.Object, uid)
	if got == nil {
		t.Fatal("todo missing")
	}
	t.Logf("after guarded Space-complete: applied=%v ETag=%q Dirty=%v Description=%q Completed=%v",
		applied, res.ETag, res.Dirty, got.Description, got.Completed())

	if !strings.Contains(got.Description, "REMOTE EDIT") {
		t.Errorf("server edit lost: description = %q, want the pulled %q",
			got.Description, "REMOTE EDIT from another device")
	}
	if res.ETag != "etag-v2" || res.Dirty {
		t.Errorf("pulled resource disturbed: ETag=%q Dirty=%v, want etag-v2 / clean", res.ETag, res.Dirty)
	}
}
