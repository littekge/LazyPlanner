package store_test

import (
	"context"
	"testing"
	"time"

	"github.com/littekge/LazyPlanner/internal/model"
	"github.com/littekge/LazyPlanner/internal/store"
)

// todoICS builds a single-VTODO calendar object with a summary and priority.
func todoICS(t *testing.T, uid, summary string, priority int) *model.Parsed {
	t.Helper()
	ics := "BEGIN:VCALENDAR\r\nVERSION:2.0\r\nPRODID:-//LazyPlanner//Test//EN\r\n" +
		"BEGIN:VTODO\r\nUID:" + uid + "\r\nDTSTAMP:20260701T120000Z\r\n" +
		"SUMMARY:" + summary + "\r\n"
	if priority > 0 {
		ics += "PRIORITY:" + itoa(priority) + "\r\n"
	}
	ics += "END:VTODO\r\nEND:VCALENDAR\r\n"
	obj, err := model.Decode([]byte(ics), time.UTC)
	if err != nil {
		t.Fatalf("decoding %q: %v", uid, err)
	}
	return obj
}

func itoa(n int) string { return string(rune('0' + n)) }

func findTd(obj *model.Parsed, uid string) *model.Todo {
	for _, td := range obj.Todos {
		if td.UID == uid {
			return td
		}
	}
	return nil
}

// TestQuickFieldSetDoesNotClobberConcurrentPull guards pass-12 MED #4: the quick
// field-set path (ui.applyTodoField, sp/sd) committed via store.Put with NO
// version check, so a background sync PullRemote that landed between the UI's
// Locate and the Put was silently clobbered. applyTodoField now commits via
// PutIfUnchanged(loc.Prev), which skips the write (applied=false) when the
// resource changed underneath — matching grabNudge.
//
// This replays the store-level sequence applyTodoField performs:
//  1. Locate(uid) -> loc.Prev (pre-edit snapshot) + loc.Object
//  2. (concurrently) sync PullRemote replaces the cached resource with a
//     server-updated version bearing a new ETag (a field the sp/sd set doesn't touch)
//  3. newObj := model.EditTodo(loc.Object, ...) -- derived from the STALE object
//  4. store.PutIfUnchanged(newObj, loc.Prev) -- skipped; the pulled edit survives
func TestQuickFieldSetDoesNotClobberConcurrentPull(t *testing.T) {
	ctx := context.Background()
	s, err := store.Open(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	const cal, uid = "personal", "td1"
	name := store.ResourceName(uid)
	href := "/dav/personal/td1.ics"

	// Seed a clean, synced resource (ETag etag-v1) as a normal sync would.
	base := todoICS(t, uid, "Original summary", 0)
	if _, err := s.PullRemote(ctx, cal, name, base, "etag-v1", href, nil); err != nil {
		t.Fatalf("seed pull: %v", err)
	}

	// (1) sp/sd begins: applyTodoField calls Locate, capturing object + snapshot.
	loc, ok := s.Locate(uid)
	if !ok {
		t.Fatal("Locate failed")
	}
	if loc.Prev.ETag != "etag-v1" {
		t.Fatalf("located ETag = %q, want etag-v1", loc.Prev.ETag)
	}

	// (2) A background sync pull lands: the server changed the SUMMARY (a field the
	// priority-set does not touch). expectedPrev still equals loc.Prev, so the
	// guarded pull APPLIES and swaps in etag-v2.
	serverEdit := todoICS(t, uid, "SERVER EDITED THE TITLE", 0)
	applied, err := s.PullRemote(ctx, cal, name, serverEdit, "etag-v2", href, loc.Prev)
	if err != nil {
		t.Fatalf("concurrent pull: %v", err)
	}
	if !applied {
		t.Fatal("concurrent pull was not applied; interleaving precondition not met")
	}

	// (3) applyTodoField computes newObj from the STALE loc.Object: set priority=1,
	// carrying loc.Object's (old) summary. This is draftFromTodo + EditTodo.
	td := findTd(loc.Object, uid)
	if td == nil {
		t.Fatal("todo missing from located object")
	}
	d := model.TodoDraft{
		Summary:     td.Summary, // "Original summary" -- the STALE value
		Description: td.Description,
		HasDue:      td.HasDue,
		Due:         td.Due,
		DueAllDay:   td.DueAllDay,
		Priority:    1, // the sp change
		Categories:  td.Categories,
		ParentUID:   td.ParentUID,
		Completed:   td.Completed(),
	}
	newObj, err := model.EditTodo(loc.Object, uid, d, time.Now(), time.UTC)
	if err != nil {
		t.Fatalf("EditTodo: %v", err)
	}

	// (4) applyTodoField now commits with the version-checked write. loc.Prev no
	// longer matches the current (pulled) resource, so the write is SKIPPED.
	applied, err = s.PutIfUnchanged(ctx, cal, name, newObj, loc.Prev)
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
	got := findTd(res.Object, uid)
	if got == nil {
		t.Fatal("todo missing")
	}
	t.Logf("after guarded quick-set: applied=%v ETag=%q Dirty=%v Summary=%q Priority=%d",
		applied, res.ETag, res.Dirty, got.Summary, got.Priority)

	if got.Summary != "SERVER EDITED THE TITLE" {
		t.Errorf("SERVER EDIT LOST: summary = %q, want the pulled %q", got.Summary, "SERVER EDITED THE TITLE")
	}
	if res.ETag != "etag-v2" || res.Dirty {
		t.Errorf("pulled resource disturbed: ETag=%q Dirty=%v, want etag-v2 / clean", res.ETag, res.Dirty)
	}
}
