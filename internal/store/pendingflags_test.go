package store_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/littekge/LazyPlanner/internal/store"
)

// TestPendingNameDoesNotBlockColorPull: a pending local rename must not block
// adopting the server's color (the flags are independent), and vice-versa.
func TestPendingNameDoesNotBlockColorPull(t *testing.T) {
	ctx := context.Background()
	s, err := store.Open(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := s.SetCalendarMeta(ctx, "cal1", store.CalendarMeta{DisplayName: "Old", Href: "/dav/cal1/"}); err != nil {
		t.Fatal(err)
	}
	// A local RENAME pending (name only).
	if err := s.UpdateCalendarMeta(ctx, "cal1", "NewName", ""); err != nil {
		t.Fatal(err)
	}

	// The color pull is NOT blocked by the pending name.
	if err := s.SyncCalendarColor(ctx, "cal1", "#abcdefff"); err != nil {
		t.Fatal(err)
	}
	if cal, _ := s.Calendar("cal1"); cal.Color != "#abcdefff" {
		t.Errorf("color should adopt despite a pending name; got %q", cal.Color)
	}
	// But the name pull IS blocked — the pending local rename wins until pushed.
	if err := s.SyncCalendarName(ctx, "cal1", "ServerName"); err != nil {
		t.Fatal(err)
	}
	if cal, _ := s.Calendar("cal1"); cal.DisplayName != "NewName" {
		t.Errorf("pending local rename should win; got %q", cal.DisplayName)
	}
	// Only the name is pushed (the color wasn't locally edited).
	pend := s.PendingCalendarProps()
	if len(pend) != 1 || pend[0].DisplayName != "NewName" || pend[0].Color != "" {
		t.Errorf("pending props = %+v, want only the name", pend)
	}
}

// TestMarkPropsSyncedKeepsConcurrentRename locks Pass-3 #9: a rename that lands
// during the PROPPATCH round-trip must not be dropped. MarkCalendarPropsSynced
// clears the pending flag only if the value still matches what was pushed; a
// concurrent change leaves it pending (so it re-pushes and the server value
// can't overwrite it).
func TestMarkPropsSyncedKeepsConcurrentRename(t *testing.T) {
	ctx := context.Background()
	s, err := store.Open(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := s.SetCalendarMeta(ctx, "cal1", store.CalendarMeta{DisplayName: "A", Href: "/dav/cal1/"}); err != nil {
		t.Fatal(err)
	}
	// Rename to B → pending. Snapshot what the PROPPATCH would push.
	if err := s.UpdateCalendarMeta(ctx, "cal1", "B", ""); err != nil {
		t.Fatal(err)
	}
	pushed := s.PendingCalendarProps()
	if len(pushed) != 1 || pushed[0].DisplayName != "B" {
		t.Fatalf("pending = %+v, want name B", pushed)
	}
	// While the PROPPATCH(B) is in flight, the user renames again to C.
	if err := s.UpdateCalendarMeta(ctx, "cal1", "C", ""); err != nil {
		t.Fatal(err)
	}
	// The PROPPATCH(B) returns; mark it synced with the value that was pushed.
	if err := s.MarkCalendarPropsSynced(ctx, "cal1", pushed[0].DisplayName, pushed[0].Color); err != nil {
		t.Fatal(err)
	}

	// C must still be pending (not cleared), so it re-pushes next sync...
	pend := s.PendingCalendarProps()
	if len(pend) != 1 || pend[0].DisplayName != "C" {
		t.Errorf("pending after mark = %+v, want the concurrent rename C still pending", pend)
	}
	// ...and a discovery pull of the older server name B must not overwrite C.
	if err := s.SyncCalendarName(ctx, "cal1", "B"); err != nil {
		t.Fatal(err)
	}
	if cal, _ := s.Calendar("cal1"); cal.DisplayName != "C" {
		t.Errorf("display name = %q, want C (concurrent rename preserved)", cal.DisplayName)
	}
}

// TestLegacyPendingPropsMapsToBoth: an old sidecar with pending_props=true loads
// as both pending name and color (so a pre-upgrade edit isn't lost).
func TestLegacyPendingPropsMapsToBoth(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	caldir := filepath.Join(dir, "calendars", "cal1")
	if err := os.MkdirAll(caldir, 0o700); err != nil {
		t.Fatal(err)
	}
	legacy := `{"display_name":"Old","color":"#111111ff","href":"/dav/cal1/","pending_props":true}`
	if err := os.WriteFile(filepath.Join(caldir, ".lazyplanner.json"), []byte(legacy), 0o600); err != nil {
		t.Fatal(err)
	}
	s, err := store.Open(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}
	// Both pulls should be blocked (legacy flag → both pending).
	_ = s.SyncCalendarColor(ctx, "cal1", "#999999ff")
	_ = s.SyncCalendarName(ctx, "cal1", "ServerName")
	cal, _ := s.Calendar("cal1")
	if cal.Color != "#111111ff" || cal.DisplayName != "Old" {
		t.Errorf("legacy pending_props should block both pulls; got name=%q color=%q", cal.DisplayName, cal.Color)
	}
}
