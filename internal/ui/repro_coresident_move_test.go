package ui

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/littekge/LazyPlanner/internal/store"
)

// TestReproCoResidentMoveDragsBystander reproduces the HIGH defect: a single
// .ics resource that bundles two unrelated VTODOs (X and Y). Cutting only X and
// pasting into a different list must move X alone — but moveSubtree writes the
// whole resource object (X+Y) to the destination and deletes it from the source,
// so the unrelated Y silently migrates too.
func TestReproCoResidentMoveDragsBystander(t *testing.T) {
	now := time.Date(2026, 7, 5, 9, 0, 0, 0, time.UTC)
	dir := t.TempDir()
	copyTree(t, "../store/testdata/vdir", dir)

	// One file, two unrelated top-level VTODOs — a shape any CalDAV server or
	// hand-edited vdir can produce (store decodes both into one Resource).
	bundle := "BEGIN:VCALENDAR\r\n" +
		"VERSION:2.0\r\n" +
		"PRODID:-//LazyPlanner//Test//EN\r\n" +
		"BEGIN:VTODO\r\n" +
		"UID:mover@lazyplanner.test\r\n" +
		"DTSTAMP:20260701T120000Z\r\n" +
		"SUMMARY:Mover\r\n" +
		"STATUS:NEEDS-ACTION\r\n" +
		"END:VTODO\r\n" +
		"BEGIN:VTODO\r\n" +
		"UID:bystander@lazyplanner.test\r\n" +
		"DTSTAMP:20260701T120000Z\r\n" +
		"SUMMARY:Bystander\r\n" +
		"STATUS:NEEDS-ACTION\r\n" +
		"END:VTODO\r\n" +
		"END:VCALENDAR\r\n"
	bundlePath := filepath.Join(dir, "calendars", "personal", "bundle.ics")
	if err := os.WriteFile(bundlePath, []byte(bundle), 0o644); err != nil {
		t.Fatal(err)
	}

	s, err := store.Open(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	a := newApp(s, "test", now)
	a.build()
	a.reload()
	a.setMode(modeTasks)

	const moverUID = "mover@lazyplanner.test"
	const bystanderUID = "bystander@lazyplanner.test"

	// Sanity: both start co-resident in "personal".
	if loc, ok := s.Locate(moverUID); !ok || loc.CalID != "personal" || loc.Name != "bundle.ics" {
		t.Fatalf("Mover setup wrong: %+v ok=%v", loc, ok)
	}
	if loc, ok := s.Locate(bystanderUID); !ok || loc.CalID != "personal" || loc.Name != "bundle.ics" {
		t.Fatalf("Bystander setup wrong: %+v ok=%v", loc, ok)
	}

	// Cut only the Mover and paste into the "work" list.
	a.yankUID = moverUID
	a.moveSubtree(moverUID, "", "personal", "work")

	// The Bystander was never selected — it must stay put in "personal".
	loc, ok := s.Locate(bystanderUID)
	if !ok {
		t.Fatalf("Bystander vanished entirely after moving an unrelated task")
	}
	if loc.CalID != "personal" {
		t.Errorf("BUG: Bystander was dragged to %q; it must remain in \"personal\"", loc.CalID)
	}
}
