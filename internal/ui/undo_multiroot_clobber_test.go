package ui

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/littekge/LazyPlanner/internal/store"
)

// TestReproUndoMultiRootMoveLosesRoot reproduces the HIGH defect: two top-level
// VTODOs (r1, r2) co-reside in one .ics (personal/bundle.ics). Bulk-cut both and
// paste into "work"; both move and the source bundle is deleted. Undo must
// restore both roots — but moveSubtreeOps stacks two source-side Restore ops
// against the SAME resource (bundle.ics), and undoLast replays them in append
// order: the r2 op's Restore (bundle with r1 already removed) clobbers r1's
// Restore (full bundle), permanently losing r1.
func TestReproUndoMultiRootMoveLosesRoot(t *testing.T) {
	now := time.Date(2026, 7, 5, 9, 0, 0, 0, time.UTC)
	dir := t.TempDir()
	copyTree(t, "../store/testdata/vdir", dir)

	bundle := "BEGIN:VCALENDAR\r\n" +
		"VERSION:2.0\r\n" +
		"PRODID:-//LazyPlanner//Test//EN\r\n" +
		"BEGIN:VTODO\r\n" +
		"UID:r1@lazyplanner.test\r\n" +
		"DTSTAMP:20260701T120000Z\r\n" +
		"SUMMARY:Root One\r\n" +
		"STATUS:NEEDS-ACTION\r\n" +
		"END:VTODO\r\n" +
		"BEGIN:VTODO\r\n" +
		"UID:r2@lazyplanner.test\r\n" +
		"DTSTAMP:20260701T120000Z\r\n" +
		"SUMMARY:Root Two\r\n" +
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

	const r1 = "r1@lazyplanner.test"
	const r2 = "r2@lazyplanner.test"

	// Bulk-cut both roots and paste into "work".
	a.yankUIDs = []string{r1, r2}
	a.yankCut = true
	a.pasteMultiRoot("", "work")

	// After the paste both live in "work"; the source bundle is gone.
	if loc, ok := s.Locate(r1); !ok || loc.CalID != "work" {
		t.Fatalf("after paste: r1 loc=%+v ok=%v (want work)", loc, ok)
	}
	if loc, ok := s.Locate(r2); !ok || loc.CalID != "work" {
		t.Fatalf("after paste: r2 loc=%+v ok=%v (want work)", loc, ok)
	}

	// Undo the move.
	a.undoLast()

	_, r1Present := s.Locate(r1)
	_, r2Present := s.Locate(r2)
	t.Logf("after undo: r1 present=%v r2 present=%v", r1Present, r2Present)

	if !r1Present {
		t.Errorf("BUG: undo permanently lost r1 — it is gone from every list")
	}
	if !r2Present {
		t.Errorf("undo lost r2")
	}
}
