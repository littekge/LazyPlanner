package ui

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/rivo/tview"

	"github.com/littekge/LazyPlanner/internal/model"
	"github.com/littekge/LazyPlanner/internal/store"
)

// TestBulkDeleteDragsCoResidentBystander reproduces the HIGH defect: a single
// .ics resource bundles two unrelated top-level VTODOs (Mover and Bystander).
// The user selects and bulk-deletes ONLY the Mover — the Bystander is never
// selected, never a descendant of anything selected. bulkDelete calls
// store.Delete on the whole resource file, so the never-selected Bystander is
// silently erased locally and will be DELETE'd on the server at next sync.
func TestBulkDeleteDragsCoResidentBystander(t *testing.T) {
	now := time.Date(2026, 7, 5, 9, 0, 0, 0, time.UTC)
	dir := t.TempDir()
	copyTree(t, "../store/testdata/vdir", dir)

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
	a.root = tview.NewPages()
	a.root.AddPage(pageMain, a.layout(), true, true)
	a.setMode(modeTasks)

	const moverUID = "mover@lazyplanner.test"
	const bystanderUID = "bystander@lazyplanner.test"

	if loc, ok := s.Locate(bystanderUID); !ok || loc.Name != "bundle.ics" {
		t.Fatalf("Bystander setup wrong: %+v ok=%v", loc, ok)
	}

	// Enter SELECT anchored on the Mover row only (range of one). Locate its
	// tree node so the selection targets exactly the Mover.
	a.setFocus(a.tree)
	var moverNode *tview.TreeNode
	for _, n := range visibleTreeNodes(a.tree.GetRoot()) {
		if td, ok := n.GetReference().(*model.Todo); ok && td.UID == moverUID {
			moverNode = n
			break
		}
	}
	if moverNode == nil {
		t.Fatal("could not find Mover node in tree")
	}
	a.tree.SetCurrentNode(moverNode)
	a.enterSelect()
	// Cursor stays on Mover — the selection is exactly {Mover}.

	a.bulkDelete()
	// bulkDelete opens a confirm ("Delete 1 item(s)?"); accept it.
	confirmYes(t, a)

	// The Mover should be gone.
	if _, ok := s.Locate(moverUID); ok {
		t.Fatalf("Mover was not deleted")
	}
	// The Bystander was never selected — it must survive.
	if _, ok := s.Locate(bystanderUID); !ok {
		t.Errorf("BUG: Bystander (never selected) was silently deleted along with the co-resident Mover")
	}
}
