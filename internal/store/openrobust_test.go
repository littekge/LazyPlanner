package store_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/littekge/LazyPlanner/internal/store"
)

// TestOpenDegradesWhenRootIsFile guards M2: a cache root that exists as a regular
// file (or is otherwise unreadable as a directory) must not abort startup. Open
// returns an empty store with the failure recorded in LoadErrors, so the user
// still gets into the app.
func TestOpenDegradesWhenRootIsFile(t *testing.T) {
	dataDir := t.TempDir()
	// Create <dataDir>/calendars as a FILE, not a directory.
	if err := os.WriteFile(filepath.Join(dataDir, "calendars"), []byte("not a dir"), 0o600); err != nil {
		t.Fatal(err)
	}
	s, err := store.Open(context.Background(), dataDir)
	if err != nil {
		t.Fatalf("Open returned a fatal error, want graceful degradation: %v", err)
	}
	if s == nil {
		t.Fatal("Open returned nil store")
	}
	if len(s.Calendars()) != 0 {
		t.Errorf("expected an empty store, got %d calendars", len(s.Calendars()))
	}
	if len(s.LoadErrors()) == 0 {
		t.Error("expected the cache-root failure recorded in LoadErrors")
	}
}
