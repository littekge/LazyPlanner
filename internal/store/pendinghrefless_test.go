package store_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/littekge/LazyPlanner/internal/store"
)

// TestHasPendingChangesHreflessOrphan closes the pass-10 canary hole: no
// store-package test seeded a clean, href-less resource, so removing the
// `|| r.Href == ""` clause from HasPendingChanges/HasLocalChanges (which flags a
// pull orphan for reconcile) went undetected. A .ics with no recorded server
// identity is such an orphan and must count as a pending change.
func TestHasPendingChangesHreflessOrphan(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	calDir := filepath.Join(dir, "calendars", "cal1")
	if err := os.MkdirAll(calDir, 0o755); err != nil {
		t.Fatal(err)
	}
	ics := "BEGIN:VCALENDAR\r\nVERSION:2.0\r\nPRODID:-//x//x//EN\r\n" +
		"BEGIN:VTODO\r\nUID:orphan\r\nDTSTAMP:20260101T000000Z\r\nSUMMARY:x\r\n" +
		"END:VTODO\r\nEND:VCALENDAR\r\n"
	if err := os.WriteFile(filepath.Join(calDir, "orphan.ics"), []byte(ics), 0o600); err != nil {
		t.Fatal(err)
	}

	s, err := store.Open(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}
	// Precondition: the resource is clean (Dirty=false) AND href-less — the exact
	// pull-orphan state the clause covers.
	cal, _ := s.Calendar("cal1")
	r := findResource(cal, "orphan.ics")
	if r == nil || r.Dirty || r.Href != "" {
		t.Fatalf("setup: want a clean href-less resource, got %+v", r)
	}
	if !s.HasLocalChanges("cal1") {
		t.Error("HasLocalChanges=false for a clean href-less pull orphan")
	}
	if !s.HasPendingChanges() {
		t.Error("HasPendingChanges=false for a clean href-less pull orphan")
	}
}
