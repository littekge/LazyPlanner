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
