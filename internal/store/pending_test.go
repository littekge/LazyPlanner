package store_test

import (
	"context"
	"testing"

	"github.com/littekge/LazyPlanner/internal/store"
)

func openTemp(t *testing.T) *store.Store {
	t.Helper()
	s, err := store.Open(context.Background(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return s
}

// TestHasPendingChanges verifies the store-wide pending check reports true for
// every kind of unpushed local change (dirty/never-pushed resource, tombstone,
// pending calendar create/delete/rename/recolor) and false when the cache is a
// clean mirror of the server — the signal the quit-flush uses to stay a no-op.
func TestHasPendingChanges(t *testing.T) {
	ctx := context.Background()

	t.Run("empty store is clean", func(t *testing.T) {
		s := openTemp(t)
		if s.HasPendingChanges() {
			t.Error("empty store reports pending changes")
		}
	})

	t.Run("clean synced resource is clean", func(t *testing.T) {
		s := openTemp(t)
		if _, err := s.PutRemote(ctx, "personal", "a.ics", mustDecode(t, "a@t", "A"), "etag-1", "/c/a.ics"); err != nil {
			t.Fatal(err)
		}
		if s.HasPendingChanges() {
			t.Error("a clean server-synced resource reports pending changes")
		}
	})

	t.Run("local edit is pending", func(t *testing.T) {
		s := openTemp(t)
		if _, err := s.Put(ctx, "personal", "a.ics", mustDecode(t, "a@t", "A")); err != nil {
			t.Fatal(err)
		}
		if !s.HasPendingChanges() {
			t.Error("a dirty local edit not reported as pending")
		}
	})

	t.Run("tombstone is pending", func(t *testing.T) {
		s := openTemp(t)
		if _, err := s.PutRemote(ctx, "personal", "a.ics", mustDecode(t, "a@t", "A"), "etag-1", "/c/a.ics"); err != nil {
			t.Fatal(err)
		}
		if err := s.Delete(ctx, "personal", "a.ics"); err != nil {
			t.Fatal(err)
		}
		if !s.HasPendingChanges() {
			t.Error("a pending deletion (tombstone) not reported as pending")
		}
	})

	t.Run("pending calendar create is pending", func(t *testing.T) {
		s := openTemp(t)
		if err := s.CreateCalendarLocal(ctx, "new", store.CalendarMeta{DisplayName: "New"}, []string{"VTODO"}); err != nil {
			t.Fatal(err)
		}
		if !s.HasPendingChanges() {
			t.Error("a locally-created calendar (pending MKCALENDAR) not reported as pending")
		}
	})

	t.Run("pending calendar delete is pending", func(t *testing.T) {
		s := openTemp(t)
		if err := s.SetCalendarMeta(ctx, "personal", store.CalendarMeta{DisplayName: "Personal", Href: "/c/personal/"}); err != nil {
			t.Fatal(err)
		}
		if err := s.MarkCalendarDeleted(ctx, "personal"); err != nil {
			t.Fatal(err)
		}
		if !s.HasPendingChanges() {
			t.Error("a calendar marked for deletion not reported as pending")
		}
	})

	t.Run("pending rename/recolor is pending", func(t *testing.T) {
		s := openTemp(t)
		if err := s.SetCalendarMeta(ctx, "personal", store.CalendarMeta{DisplayName: "Personal", Href: "/c/personal/"}); err != nil {
			t.Fatal(err)
		}
		if err := s.UpdateCalendarMeta(ctx, "personal", "Renamed", "#ff0000"); err != nil {
			t.Fatal(err)
		}
		if !s.HasPendingChanges() {
			t.Error("a local rename/recolor (pending PROPPATCH) not reported as pending")
		}
	})
}
