package ui

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/littekge/LazyPlanner/internal/store"
)

// TestCopyBundledSiblingRepro reproduces the reported defect: two todos X and Y
// live co-resident in ONE .ics. Copying only X and pasting must duplicate X
// alone. The bug: CopyTodo clones the whole object (X and Y), so the write
// carries Y along with its ORIGINAL UID — a phantom copy of Y the user never
// touched, and a duplicate-UID resource on the server.
func TestCopyBundledSibling(t *testing.T) {
	now := time.Date(2026, 7, 5, 9, 0, 0, 0, time.UTC)
	dir := t.TempDir()
	copyTree(t, "../store/testdata/vdir", dir)

	// A single resource bundling two independent top-level todos.
	bundle := "BEGIN:VCALENDAR\r\n" +
		"VERSION:2.0\r\n" +
		"PRODID:-//LazyPlanner//Test//EN\r\n" +
		"BEGIN:VTODO\r\n" +
		"UID:bundle-X@lazyplanner.test\r\n" +
		"DTSTAMP:20260701T120000Z\r\n" +
		"SUMMARY:TaskX\r\n" +
		"STATUS:NEEDS-ACTION\r\n" +
		"END:VTODO\r\n" +
		"BEGIN:VTODO\r\n" +
		"UID:bundle-Y@lazyplanner.test\r\n" +
		"DTSTAMP:20260701T120000Z\r\n" +
		"SUMMARY:TaskY\r\n" +
		"STATUS:NEEDS-ACTION\r\n" +
		"END:VTODO\r\n" +
		"END:VCALENDAR\r\n"
	if err := os.WriteFile(filepath.Join(dir, "calendars", "personal", "bundle.ics"), []byte(bundle), 0o644); err != nil {
		t.Fatalf("write bundle: %v", err)
	}

	s, err := store.Open(context.Background(), dir)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	a := newApp(s, "test", now)
	a.build()
	a.reload()
	a.setMode(modeTasks)

	// Sanity: exactly one TaskX and one TaskY at the start.
	if n := len(todosBySummary(a, "TaskX")); n != 1 {
		t.Fatalf("setup: want 1 TaskX, got %d", n)
	}
	if n := len(todosBySummary(a, "TaskY")); n != 1 {
		t.Fatalf("setup: want 1 TaskY, got %d", n)
	}

	// Copy only X, paste at top level.
	a.buildTree()
	x := todoBySummary(a.store, "TaskX")
	a.selectTreeByUID(x.UID)
	a.copyTask()
	a.pasteAtTop()

	// Expected: one extra TaskX (the copy), Y untouched (still exactly one).
	if n := len(todosBySummary(a, "TaskX")); n != 2 {
		t.Errorf("after copy of X: want 2 TaskX, got %d", n)
	}
	if n := len(todosBySummary(a, "TaskY")); n != 1 {
		t.Errorf("DEFECT: copying X duplicated co-resident Y — want 1 TaskY, got %d", n)
	}
}
