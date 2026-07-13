package store_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/littekge/LazyPlanner/internal/store"
)

// TestCrashBetweenIcsAndSidecarLoadsDirty guards pass-10 MED #8: a crash between
// the .ics rename and the sidecar rename leaves a new .ics beside an old sidecar.
// Before the fix the resource reloaded clean-and-synced (Dirty=false), so the
// offline edit was never pushed. The content-hash reconcile must reload it Dirty.
func TestCrashBetweenIcsAndSidecarLoadsDirty(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	s, err := store.Open(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.CreateCalendarLocal(ctx, "cal1", store.CalendarMeta{DisplayName: "C"}, []string{"VTODO"}); err != nil {
		t.Fatal(err)
	}
	name := store.ResourceName("e@test")
	put, err := s.Put(ctx, "cal1", name, mustDecode(t, "e@test", "Base"))
	if err != nil {
		t.Fatal(err)
	}
	// Simulate a completed sync: clears Dirty and records the etag/href + hash.
	if _, err := s.CommitPush(ctx, "cal1", name, put, "etag-1", "/dav/cal1/"+name); err != nil {
		t.Fatal(err)
	}

	// Control: a clean reopen (the .ics untouched) must NOT be spuriously dirty.
	s2, err := store.Open(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}
	cal2, _ := s2.Calendar("cal1")
	if r := findResource(cal2, name); r == nil || r.Dirty {
		t.Fatalf("clean reopen: resource nil or spuriously dirty: %+v", r)
	}

	// Simulate the crash: the .ics gets the new edit, but the sidecar still holds
	// the pre-edit state (as if the process died between the two renames).
	icsPath := filepath.Join(dir, "calendars", "cal1", name)
	edited := "BEGIN:VCALENDAR\r\nVERSION:2.0\r\nPRODID:-//x//x//EN\r\n" +
		"BEGIN:VTODO\r\nUID:e@test\r\nDTSTAMP:20260101T000000Z\r\nSUMMARY:Edited offline\r\nEND:VTODO\r\n" +
		"END:VCALENDAR\r\n"
	if err := os.WriteFile(icsPath, []byte(edited), 0o600); err != nil {
		t.Fatal(err)
	}

	s3, err := store.Open(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}
	cal3, _ := s3.Calendar("cal1")
	r := findResource(cal3, name)
	if r == nil {
		t.Fatal("resource missing after crash reload")
	}
	if !r.Dirty {
		t.Error("resource reloaded clean after a crash between .ics and sidecar — the offline edit would never sync")
	}
	if !s3.HasPendingChanges() {
		t.Error("HasPendingChanges=false after the crash reload — sync would skip the stranded edit")
	}
}
