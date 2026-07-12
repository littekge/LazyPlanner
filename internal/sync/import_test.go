package sync_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/emersion/go-ical"

	"github.com/littekge/LazyPlanner/internal/caldav"
	"github.com/littekge/LazyPlanner/internal/store"
	"github.com/littekge/LazyPlanner/internal/sync"
)

const goodEvent1 = `BEGIN:VCALENDAR
VERSION:2.0
PRODID:-//test//EN
BEGIN:VEVENT
UID:e1@test
DTSTAMP:20260701T120000Z
DTSTART:20260704T130000Z
DTEND:20260704T133000Z
SUMMARY:Event one
END:VEVENT
END:VCALENDAR
`

const goodEvent2 = `BEGIN:VCALENDAR
VERSION:2.0
PRODID:-//test//EN
BEGIN:VEVENT
UID:e2@test
DTSTAMP:20260701T120000Z
DTSTART:20260705T150000Z
DTEND:20260705T160000Z
SUMMARY:Event two
END:VEVENT
END:VCALENDAR
`

// badEvent has no DTSTART, so model.Parse rejects it.
const badEvent = `BEGIN:VCALENDAR
VERSION:2.0
PRODID:-//test//EN
BEGIN:VEVENT
UID:bad@test
DTSTAMP:20260701T120000Z
SUMMARY:Broken
END:VEVENT
END:VCALENDAR
`

func mustCal(t *testing.T, ics string) *ical.Calendar {
	t.Helper()
	cal, err := ical.NewDecoder(strings.NewReader(ics)).Decode()
	if err != nil {
		t.Fatalf("decoding fixture: %v", err)
	}
	return cal
}

type fakeSource struct {
	cals    []caldav.Calendar
	objs    map[string][]caldav.Object
	discErr error
}

func (f *fakeSource) DiscoverCalendars(context.Context) ([]caldav.Calendar, error) {
	return f.cals, f.discErr
}

func (f *fakeSource) DownloadAll(_ context.Context, p string) ([]caldav.Object, error) {
	return f.objs[p], nil
}

// ListObjectHrefs/GetObject satisfy the Source interface for the resilient
// download fallback; the import tests exercise the happy bulk path, so these are
// simple pass-throughs over the same fixture data.
func (f *fakeSource) ListObjectHrefs(_ context.Context, p string) ([]caldav.ObjectRef, error) {
	var out []caldav.ObjectRef
	for _, o := range f.objs[p] {
		out = append(out, caldav.ObjectRef{Href: o.Path, ETag: o.ETag})
	}
	return out, nil
}

func (f *fakeSource) GetObject(_ context.Context, href string) (caldav.Object, error) {
	for _, objs := range f.objs {
		for _, o := range objs {
			if o.Path == href {
				return o, nil
			}
		}
	}
	return caldav.Object{}, errors.New("not found")
}

func findResource(cal store.Calendar, name string) *store.Resource {
	for _, r := range cal.Resources {
		if r.Name == name {
			return r
		}
	}
	return nil
}

func TestImport(t *testing.T) {
	ctx := context.Background()
	src := &fakeSource{
		cals: []caldav.Calendar{
			{Path: "/dav/cal/personal/", Name: "Personal"},
			{Path: "/dav/cal/work/", Name: "Work"},
		},
		objs: map[string][]caldav.Object{
			"/dav/cal/personal/": {
				{Path: "/dav/cal/personal/e1.ics", ETag: `"etag-e1"`, Data: mustCal(t, goodEvent1)},
				{Path: "/dav/cal/personal/bad.ics", ETag: `"etag-bad"`, Data: mustCal(t, badEvent)},
			},
			"/dav/cal/work/": {
				{Path: "/dav/cal/work/e2.ics", ETag: `"etag-e2"`, Data: mustCal(t, goodEvent2)},
			},
		},
	}

	dir := t.TempDir()
	st, err := store.Open(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}

	res, err := sync.Import(ctx, src, st)
	if err != nil {
		t.Fatal(err)
	}
	if res.Calendars != 2 {
		t.Errorf("Calendars = %d, want 2", res.Calendars)
	}
	if res.Objects != 2 {
		t.Errorf("Objects = %d, want 2 (bad one skipped)", res.Objects)
	}
	if len(res.Skipped) != 1 || res.Skipped[0].Path != "/dav/cal/personal/bad.ics" {
		t.Fatalf("Skipped = %v, want one for the bad resource", res.Skipped)
	}

	cal, ok := st.Calendar("personal")
	if !ok {
		t.Fatal("personal calendar missing")
	}
	if cal.DisplayName != "Personal" || cal.Href != "/dav/cal/personal/" {
		t.Errorf("personal metadata = %q/%q", cal.DisplayName, cal.Href)
	}
	r := findResource(cal, "e1.ics")
	if r == nil {
		t.Fatal("e1.ics missing")
	}
	if r.Dirty {
		t.Error("imported resource should not be dirty")
	}
	if r.ETag != `"etag-e1"` || r.Href != "/dav/cal/personal/e1.ics" {
		t.Errorf("server identity = %q/%q", r.ETag, r.Href)
	}

	// The clean state must survive a reload from disk.
	st2, err := store.Open(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}
	cal2, _ := st2.Calendar("personal")
	r2 := findResource(cal2, "e1.ics")
	if r2 == nil || r2.Dirty || r2.ETag != `"etag-e1"` {
		t.Errorf("reloaded resource = %+v, want clean with etag", r2)
	}
}

func TestImportDiscoveryError(t *testing.T) {
	ctx := context.Background()
	st, err := store.Open(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	src := &fakeSource{discErr: errors.New("boom")}
	if _, err := sync.Import(ctx, src, st); err == nil {
		t.Error("expected Import to fail when discovery fails")
	}
}
