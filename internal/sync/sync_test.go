package sync_test

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/emersion/go-ical"

	"github.com/littekge/LazyPlanner/internal/caldav"
	"github.com/littekge/LazyPlanner/internal/model"
	"github.com/littekge/LazyPlanner/internal/store"
	"github.com/littekge/LazyPlanner/internal/sync"
)

const calPath = "/dav/cal/personal/"

// eventICS builds a one-event calendar with the given UID and summary.
func eventICS(uid, summary string) string {
	return fmt.Sprintf(`BEGIN:VCALENDAR
VERSION:2.0
PRODID:-//test//EN
BEGIN:VEVENT
UID:%s
DTSTAMP:20260701T120000Z
DTSTART:20260704T130000Z
DTEND:20260704T133000Z
SUMMARY:%s
END:VEVENT
END:VCALENDAR
`, uid, summary)
}

func mkParsed(t *testing.T, ics string) *model.Parsed {
	t.Helper()
	p, err := model.Decode([]byte(ics), nil)
	if err != nil {
		t.Fatalf("decoding: %v", err)
	}
	return p
}

func mkICal(t *testing.T, ics string) *ical.Calendar {
	t.Helper()
	cal, err := ical.NewDecoder(strings.NewReader(ics)).Decode()
	if err != nil {
		t.Fatalf("decoding ical: %v", err)
	}
	return cal
}

// fakeServer is an in-memory CalDAV server implementing sync.Syncer. PutObject
// and DeleteObject mutate its state (so idempotency and round-trips can be
// checked), and failPut/failDel inject conditional-write failures.
type fakeServer struct {
	cals    []caldav.Calendar
	data    map[string]caldav.Object // href -> object
	puts    int
	deletes int
	failPut map[string]error
	failDel map[string]error
	seq     int
}

func newFakeServer() *fakeServer {
	return &fakeServer{
		cals:    []caldav.Calendar{{Path: calPath, Name: "Personal"}},
		data:    map[string]caldav.Object{},
		failPut: map[string]error{},
		failDel: map[string]error{},
	}
}

func (f *fakeServer) DiscoverCalendars(context.Context) ([]caldav.Calendar, error) {
	return f.cals, nil
}

func (f *fakeServer) DownloadAll(_ context.Context, p string) ([]caldav.Object, error) {
	var out []caldav.Object
	for href, o := range f.data {
		if strings.HasPrefix(href, p) {
			out = append(out, o)
		}
	}
	return out, nil
}

func (f *fakeServer) PutObject(_ context.Context, href string, data []byte, _ string, _ bool) (string, error) {
	f.puts++
	if err := f.failPut[href]; err != nil {
		return "", err
	}
	cal, err := ical.NewDecoder(bytes.NewReader(data)).Decode()
	if err != nil {
		return "", err
	}
	f.seq++
	etag := fmt.Sprintf("new-%d", f.seq)
	f.data[href] = caldav.Object{Path: href, ETag: etag, Data: cal}
	return etag, nil
}

func (f *fakeServer) DeleteObject(_ context.Context, href, _ string) error {
	f.deletes++
	if err := f.failDel[href]; err != nil {
		return err
	}
	delete(f.data, href)
	return nil
}

// newStore opens an empty store and records the Personal calendar's server href.
func newStore(t *testing.T) *store.Store {
	t.Helper()
	st, err := store.Open(context.Background(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := st.SetCalendarMeta(context.Background(), "personal", store.CalendarMeta{DisplayName: "Personal", Href: calPath}); err != nil {
		t.Fatal(err)
	}
	return st
}

func findRes(t *testing.T, st *store.Store, name string) *store.Resource {
	t.Helper()
	cal, _ := st.Calendar("personal")
	for _, r := range cal.Resources {
		if r.Name == name {
			return r
		}
	}
	return nil
}

func TestSyncPushesNewLocalCreate(t *testing.T) {
	ctx := context.Background()
	st := newStore(t)
	srv := newFakeServer()

	name := store.ResourceName("e1@test")
	if _, err := st.Put(ctx, "personal", name, mkParsed(t, eventICS("e1@test", "Local"))); err != nil {
		t.Fatal(err)
	}

	res, err := sync.Sync(ctx, srv, st)
	if err != nil {
		t.Fatal(err)
	}
	if res.Pushed != 1 || srv.puts != 1 {
		t.Fatalf("Pushed=%d puts=%d, want 1/1", res.Pushed, srv.puts)
	}
	// The resource is now clean with the server's identity.
	r := findRes(t, st, name)
	if r == nil || r.Dirty || r.ETag == "" || r.Href == "" {
		t.Fatalf("resource after push = %+v, want clean with etag/href", r)
	}
	// A second sync is a no-op (nothing dirty, server matches).
	res2, _ := sync.Sync(ctx, srv, st)
	if res2.Pushed+res2.Pulled+res2.Conflicts != 0 {
		t.Errorf("second sync not idempotent: %+v", res2)
	}
}

func TestSyncPushesLocalEdit(t *testing.T) {
	ctx := context.Background()
	st := newStore(t)
	srv := newFakeServer()

	name := store.ResourceName("e1@test")
	href := calPath + name
	// Synced clean copy on both sides at etag srv-1.
	if _, err := st.PutRemote(ctx, "personal", name, mkParsed(t, eventICS("e1@test", "Original")), "srv-1", href); err != nil {
		t.Fatal(err)
	}
	srv.data[href] = caldav.Object{Path: href, ETag: "srv-1", Data: mkICal(t, eventICS("e1@test", "Original"))}
	// Local edit (still etag srv-1, now dirty).
	if _, err := st.Put(ctx, "personal", name, mkParsed(t, eventICS("e1@test", "Edited"))); err != nil {
		t.Fatal(err)
	}

	res, err := sync.Sync(ctx, srv, st)
	if err != nil {
		t.Fatal(err)
	}
	if res.Pushed != 1 || res.Conflicts != 0 {
		t.Fatalf("res = %+v, want Pushed 1 no conflict", res)
	}
	if r := findRes(t, st, name); r == nil || r.Dirty {
		t.Errorf("resource still dirty after push: %+v", r)
	}
	if got := srv.data[href].Data.Children[0].Props.Get("SUMMARY").Value; got != "Edited" {
		t.Errorf("server summary = %q, want Edited", got)
	}
}

func TestSyncPullsServerEdit(t *testing.T) {
	ctx := context.Background()
	st := newStore(t)
	srv := newFakeServer()

	name := store.ResourceName("e1@test")
	href := calPath + name
	if _, err := st.PutRemote(ctx, "personal", name, mkParsed(t, eventICS("e1@test", "Original")), "srv-1", href); err != nil {
		t.Fatal(err)
	}
	// Server has a newer version.
	srv.data[href] = caldav.Object{Path: href, ETag: "srv-2", Data: mkICal(t, eventICS("e1@test", "ServerUpdated"))}

	res, err := sync.Sync(ctx, srv, st)
	if err != nil {
		t.Fatal(err)
	}
	if res.Pulled != 1 || srv.puts != 0 {
		t.Fatalf("res = %+v puts=%d, want Pulled 1 puts 0", res, srv.puts)
	}
	r := findRes(t, st, name)
	if r == nil || r.ETag != "srv-2" || r.Dirty {
		t.Fatalf("resource after pull = %+v, want clean at srv-2", r)
	}
	if r.Object.Events[0].Summary != "ServerUpdated" {
		t.Errorf("summary = %q, want ServerUpdated", r.Object.Events[0].Summary)
	}
}

func TestSyncPullsNewServerObject(t *testing.T) {
	ctx := context.Background()
	st := newStore(t)
	srv := newFakeServer()
	href := calPath + "remote.ics"
	srv.data[href] = caldav.Object{Path: href, ETag: "srv-1", Data: mkICal(t, eventICS("remote@test", "FromServer"))}

	res, err := sync.Sync(ctx, srv, st)
	if err != nil {
		t.Fatal(err)
	}
	if res.Pulled != 1 {
		t.Fatalf("Pulled = %d, want 1", res.Pulled)
	}
	cal, _ := st.Calendar("personal")
	if len(cal.Resources) != 1 || cal.Resources[0].Object.Events[0].Summary != "FromServer" {
		t.Errorf("pulled resource missing/wrong: %+v", cal.Resources)
	}
}

func TestSyncConflictKeepsBoth(t *testing.T) {
	ctx := context.Background()
	st := newStore(t)
	srv := newFakeServer()

	name := store.ResourceName("e1@test")
	href := calPath + name
	if _, err := st.PutRemote(ctx, "personal", name, mkParsed(t, eventICS("e1@test", "Base")), "srv-1", href); err != nil {
		t.Fatal(err)
	}
	if _, err := st.Put(ctx, "personal", name, mkParsed(t, eventICS("e1@test", "LocalEdit"))); err != nil {
		t.Fatal(err)
	}
	// Server also changed (different etag).
	srv.data[href] = caldav.Object{Path: href, ETag: "srv-2", Data: mkICal(t, eventICS("e1@test", "ServerEdit"))}

	res, err := sync.Sync(ctx, srv, st)
	if err != nil {
		t.Fatal(err)
	}
	if res.Conflicts != 1 || res.Pushed != 0 || srv.puts != 0 {
		t.Fatalf("res = %+v puts=%d, want a single conflict and no push", res, srv.puts)
	}
	if cs := st.Conflicts(); len(cs) != 1 || len(cs[0].ServerData) == 0 {
		t.Fatalf("Conflicts = %+v, want one with stashed server data", cs)
	}
	// The local edit is preserved and flagged.
	r := findRes(t, st, name)
	if r == nil || !r.Conflicted || r.Object.Events[0].Summary != "LocalEdit" {
		t.Fatalf("local resource = %+v, want conflicted LocalEdit kept", r)
	}
	// A conflicted resource is skipped on the next sync (no repeated push/flag).
	res2, _ := sync.Sync(ctx, srv, st)
	if res2.Conflicts != 0 || res2.Pushed != 0 {
		t.Errorf("second sync touched a conflicted resource: %+v", res2)
	}
}

func TestSyncServerDeleteDropsCleanLocal(t *testing.T) {
	ctx := context.Background()
	st := newStore(t)
	srv := newFakeServer()

	name := store.ResourceName("e1@test")
	href := calPath + name
	if _, err := st.PutRemote(ctx, "personal", name, mkParsed(t, eventICS("e1@test", "Base")), "srv-1", href); err != nil {
		t.Fatal(err)
	}
	// Server no longer has it.

	res, err := sync.Sync(ctx, srv, st)
	if err != nil {
		t.Fatal(err)
	}
	if res.PulledDeletes != 1 {
		t.Fatalf("PulledDeletes = %d, want 1", res.PulledDeletes)
	}
	if findRes(t, st, name) != nil {
		t.Error("resource still present after remote deletion")
	}
	// Dropping a remotely-deleted resource must NOT create a tombstone.
	if ts := st.Tombstones(); len(ts) != 0 {
		t.Errorf("unexpected tombstone after remote-delete drop: %+v", ts)
	}
}

func TestSyncServerDeleteVsLocalEditIsConflict(t *testing.T) {
	ctx := context.Background()
	st := newStore(t)
	srv := newFakeServer()

	name := store.ResourceName("e1@test")
	href := calPath + name
	if _, err := st.PutRemote(ctx, "personal", name, mkParsed(t, eventICS("e1@test", "Base")), "srv-1", href); err != nil {
		t.Fatal(err)
	}
	if _, err := st.Put(ctx, "personal", name, mkParsed(t, eventICS("e1@test", "LocalEdit"))); err != nil {
		t.Fatal(err)
	}
	// Server deleted it (absent).

	res, err := sync.Sync(ctx, srv, st)
	if err != nil {
		t.Fatal(err)
	}
	if res.Conflicts != 1 {
		t.Fatalf("Conflicts = %d, want 1", res.Conflicts)
	}
	if r := findRes(t, st, name); r == nil || !r.Conflicted {
		t.Errorf("local edit not kept/flagged: %+v", r)
	}
}

func TestSyncPushesTombstoneDelete(t *testing.T) {
	ctx := context.Background()
	st := newStore(t)
	srv := newFakeServer()

	name := store.ResourceName("e1@test")
	href := calPath + name
	if _, err := st.PutRemote(ctx, "personal", name, mkParsed(t, eventICS("e1@test", "Base")), "srv-1", href); err != nil {
		t.Fatal(err)
	}
	srv.data[href] = caldav.Object{Path: href, ETag: "srv-1", Data: mkICal(t, eventICS("e1@test", "Base"))}
	// Delete locally → tombstone.
	if err := st.Delete(ctx, "personal", name); err != nil {
		t.Fatal(err)
	}

	res, err := sync.Sync(ctx, srv, st)
	if err != nil {
		t.Fatal(err)
	}
	if res.PushedDeletes != 1 || srv.deletes != 1 {
		t.Fatalf("res=%+v deletes=%d, want PushedDeletes 1 / deletes 1", res, srv.deletes)
	}
	if _, ok := srv.data[href]; ok {
		t.Error("object still on server after tombstone push")
	}
	if ts := st.Tombstones(); len(ts) != 0 {
		t.Errorf("tombstone not cleared: %+v", ts)
	}
}

func TestSyncTombstoneVsServerEditIsConflict(t *testing.T) {
	ctx := context.Background()
	st := newStore(t)
	srv := newFakeServer()

	name := store.ResourceName("e1@test")
	href := calPath + name
	if _, err := st.PutRemote(ctx, "personal", name, mkParsed(t, eventICS("e1@test", "Base")), "srv-1", href); err != nil {
		t.Fatal(err)
	}
	// Server has a changed version; the conditional DELETE will be refused.
	srv.data[href] = caldav.Object{Path: href, ETag: "srv-2", Data: mkICal(t, eventICS("e1@test", "ServerEdit"))}
	srv.failDel[href] = caldav.ErrPreconditionFailed
	if err := st.Delete(ctx, "personal", name); err != nil {
		t.Fatal(err)
	}

	res, err := sync.Sync(ctx, srv, st)
	if err != nil {
		t.Fatal(err)
	}
	if res.Conflicts != 1 {
		t.Fatalf("Conflicts = %d, want 1", res.Conflicts)
	}
	// The server version is resurrected locally so its edit is not lost...
	r := findRes(t, st, name)
	if r == nil || !r.Conflicted || r.Object.Events[0].Summary != "ServerEdit" {
		t.Fatalf("resurrected resource = %+v, want conflicted ServerEdit", r)
	}
	// ...and the tombstone is cleared (the delete lost the race).
	if ts := st.Tombstones(); len(ts) != 0 {
		t.Errorf("tombstone not cleared after conflict: %+v", ts)
	}
}

func TestSyncCreatesNewServerCalendar(t *testing.T) {
	ctx := context.Background()
	// A store that knows nothing yet.
	st, err := store.Open(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	srv := newFakeServer()
	href := calPath + "e1.ics"
	srv.data[href] = caldav.Object{Path: href, ETag: "srv-1", Data: mkICal(t, eventICS("e1@test", "Hello"))}

	res, err := sync.Sync(ctx, srv, st)
	if err != nil {
		t.Fatal(err)
	}
	if res.Calendars != 1 || res.Pulled != 1 {
		t.Fatalf("res = %+v, want 1 calendar / 1 pulled", res)
	}
	cal, ok := st.Calendar("personal")
	if !ok || cal.DisplayName != "Personal" || len(cal.Resources) != 1 {
		t.Errorf("calendar not created from server: %+v", cal)
	}
}
