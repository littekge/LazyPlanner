package store_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/littekge/LazyPlanner/internal/store"
)

// TestSafeNameNeutralizesTraversal guards the path-escape fix: a calendar id or
// resource name that would sanitize to a "." or ".." traversal segment must be
// neutralized, since it is later joined onto the cache root.
func TestSafeNameNeutralizesTraversal(t *testing.T) {
	for _, in := range []string{"", ".", "..", "../..", "/"} {
		out := store.SafeName(in)
		if out == "." || out == ".." || out == "" {
			t.Errorf("SafeName(%q) = %q — still a traversal/empty segment", in, out)
		}
		if filepath.Base(out) != out || out != filepath.Clean(out) {
			t.Errorf("SafeName(%q) = %q — not a single clean path element", in, out)
		}
	}
}

// TestRemoveCalendarLocalRejectsTraversalID is the catastrophe guard: before the
// fix, RemoveCalendarLocal(".." ) resolved to os.RemoveAll(<dataDir>) and wiped
// the whole account. It must now refuse an unsafe id and leave the parent intact.
func TestRemoveCalendarLocalRejectsTraversalID(t *testing.T) {
	ctx := context.Background()
	dataDir := t.TempDir()
	// A sentinel that lives beside the calendars root (i.e. under dataDir) — the
	// exact thing a RemoveAll(root/..) would have destroyed.
	sentinel := filepath.Join(dataDir, "state.json")
	if err := os.WriteFile(sentinel, []byte("keep me"), 0o600); err != nil {
		t.Fatal(err)
	}

	s, err := store.Open(ctx, dataDir)
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"..", ".", "../..", "a/../.."} {
		if err := s.RemoveCalendarLocal(ctx, id); err == nil {
			t.Errorf("RemoveCalendarLocal(%q) = nil, want error", id)
		}
	}
	if _, err := os.Stat(sentinel); err != nil {
		t.Fatalf("sentinel destroyed by a traversal id: %v", err)
	}
}

// TestCreateCalendarLocalRejectsTraversalID ensures the create path can't seed an
// escaping calendar id either.
func TestCreateCalendarLocalRejectsTraversalID(t *testing.T) {
	ctx := context.Background()
	s, err := store.Open(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"..", ".", "foo/bar"} {
		if err := s.CreateCalendarLocal(ctx, id, store.CalendarMeta{DisplayName: "x"}, []string{"VEVENT"}); err == nil {
			t.Errorf("CreateCalendarLocal(%q) = nil, want error", id)
		}
	}
}
