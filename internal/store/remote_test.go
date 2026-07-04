package store_test

import (
	"context"
	"testing"

	"github.com/littekge/LazyPlanner/internal/store"
)

func TestSetCalendarMetaAndPutRemote(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()

	s, err := store.Open(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}

	meta := store.CalendarMeta{
		DisplayName: "My Calendar",
		Color:       "#ff8800",
		Href:        "/dav/cal1/",
		SyncToken:   "tok-1",
	}
	if err := s.SetCalendarMeta(ctx, "cal1", meta); err != nil {
		t.Fatal(err)
	}

	obj := mustDecode(t, "remote@lazyplanner.test", "From server")
	res, err := s.PutRemote(ctx, "cal1", "remote.ics", obj, `"srv-etag"`, "/dav/cal1/remote.ics")
	if err != nil {
		t.Fatal(err)
	}
	if res.Dirty {
		t.Error("PutRemote resource should not be dirty")
	}
	if res.ETag != `"srv-etag"` || res.Href != "/dav/cal1/remote.ics" {
		t.Errorf("server identity = %q/%q", res.ETag, res.Href)
	}

	// Metadata and clean sync state persist across a reload.
	s2, err := store.Open(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}
	cal, ok := s2.Calendar("cal1")
	if !ok {
		t.Fatal("cal1 missing after reload")
	}
	if cal.DisplayName != "My Calendar" || cal.Color != "#ff8800" ||
		cal.Href != "/dav/cal1/" || cal.SyncToken != "tok-1" {
		t.Errorf("metadata not persisted: %+v", cal)
	}
	r := findResource(cal, "remote.ics")
	if r == nil || r.Dirty || r.ETag != `"srv-etag"` {
		t.Errorf("reloaded remote resource = %+v, want clean with etag", r)
	}
}
