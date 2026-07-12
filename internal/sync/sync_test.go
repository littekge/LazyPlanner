package sync_test

import (
	"bytes"
	"context"
	"errors"
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
	cals         []caldav.Calendar
	data         map[string]caldav.Object // href -> object
	puts         int
	deletes      int
	failPut      map[string]error
	failDel      map[string]error
	failDownload map[string]error         // calendar path -> DownloadAll error
	downloads    int                      // count of DownloadAll calls (CTag short-circuit test)
	getData      map[string]caldav.Object // href -> version GetObject returns (else f.data)
	gets         int
	writable     map[string]bool  // path -> writable (missing = writable); the 403 re-check
	writableErr  map[string]error // path -> error from the writability re-check
	seq          int
	homeSet      string
	created      []caldav.CalendarSpec // MKCALENDAR calls, by path order
	createdPath  []string
	deletedCals  []string      // DeleteCalendar paths
	propPatched  []propPatchOp // PROPPATCH calls
	onPut        func()        // invoked inside PutObject to simulate a concurrent local edit mid-PUT
}

type propPatchOp struct {
	path, displayName, color string
}

func newFakeServer() *fakeServer {
	return &fakeServer{
		cals:         []caldav.Calendar{{Path: calPath, Name: "Personal"}},
		data:         map[string]caldav.Object{},
		failPut:      map[string]error{},
		failDel:      map[string]error{},
		failDownload: map[string]error{},
		getData:      map[string]caldav.Object{},
		writable:     map[string]bool{},
		writableErr:  map[string]error{},
		homeSet:      "/dav/cal/",
	}
}

// CalendarWritable is the reactive privilege re-check. Missing key = writable
// (fail-open), matching the real client.
func (f *fakeServer) CalendarWritable(_ context.Context, path string) (bool, error) {
	if err := f.writableErr[path]; err != nil {
		return false, err
	}
	if w, ok := f.writable[path]; ok {
		return w, nil
	}
	return true, nil
}

func (f *fakeServer) CalendarHomeSet(context.Context) (string, error) { return f.homeSet, nil }

func (f *fakeServer) SetCalendarProps(_ context.Context, path, displayName, color string) error {
	f.propPatched = append(f.propPatched, propPatchOp{path: path, displayName: displayName, color: color})
	for i, c := range f.cals {
		if c.Path == path {
			if displayName != "" {
				f.cals[i].Name = displayName
			}
			if color != "" {
				f.cals[i].Color = color
			}
			break
		}
	}
	return nil
}

func (f *fakeServer) CreateCalendar(_ context.Context, path string, spec caldav.CalendarSpec) error {
	f.created = append(f.created, spec)
	f.createdPath = append(f.createdPath, path)
	// The new calendar now appears in discovery.
	f.cals = append(f.cals, caldav.Calendar{Path: path, Name: spec.DisplayName})
	return nil
}

func (f *fakeServer) DeleteCalendar(_ context.Context, path string) error {
	f.deletedCals = append(f.deletedCals, path)
	for i, c := range f.cals {
		if c.Path == path {
			f.cals = append(f.cals[:i], f.cals[i+1:]...)
			break
		}
	}
	return nil
}

func (f *fakeServer) DiscoverCalendars(context.Context) ([]caldav.Calendar, error) {
	return f.cals, nil
}

// GetObject returns the getData override for an href when present (so a test can
// make the re-fetched version differ from the start-of-sync download), else the
// live data. Missing -> not found.
func (f *fakeServer) GetObject(_ context.Context, href string) (caldav.Object, error) {
	f.gets++
	if o, ok := f.getData[href]; ok {
		return o, nil
	}
	if o, ok := f.data[href]; ok {
		return o, nil
	}
	return caldav.Object{}, errors.New("not found")
}

func (f *fakeServer) DownloadAll(_ context.Context, p string) ([]caldav.Object, error) {
	f.downloads++
	if err := f.failDownload[p]; err != nil {
		return nil, err
	}
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
	// Simulate a concurrent local edit landing while this PUT is "in flight",
	// after the server accepted the pushed content but before the writeback.
	if f.onPut != nil {
		f.onPut()
	}
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

// TestSyncPushDoesNotClobberConcurrentEdit locks Pass-3 #3: if a local edit
// lands while a resource's PUT is in flight, the post-PUT writeback must not
// revert the resource to the pushed (stale) snapshot and mark it clean —
// silently losing the edit. The newer edit must survive, stay dirty, and adopt
// the server's new ETag as its baseline so the next push is conditional on it.
func TestSyncPushDoesNotClobberConcurrentEdit(t *testing.T) {
	ctx := context.Background()
	st := newStore(t)
	srv := newFakeServer()

	name := store.ResourceName("e1@test")
	href := calPath + name
	if _, err := st.PutRemote(ctx, "personal", name, mkParsed(t, eventICS("e1@test", "Original")), "srv-1", href); err != nil {
		t.Fatal(err)
	}
	srv.data[href] = caldav.Object{Path: href, ETag: "srv-1", Data: mkICal(t, eventICS("e1@test", "Original"))}
	// Local edit v1 — the version sync will push.
	if _, err := st.Put(ctx, "personal", name, mkParsed(t, eventICS("e1@test", "EditV1"))); err != nil {
		t.Fatal(err)
	}
	// While the PUT is in flight, the user makes a second edit (v2).
	srv.onPut = func() {
		srv.onPut = nil // once
		if _, err := st.Put(ctx, "personal", name, mkParsed(t, eventICS("e1@test", "EditV2"))); err != nil {
			t.Fatal(err)
		}
	}

	res, err := sync.Sync(ctx, srv, st)
	if err != nil {
		t.Fatal(err)
	}
	if res.Pushed != 1 {
		t.Fatalf("res = %+v, want Pushed 1", res)
	}
	r := findRes(t, st, name)
	if r == nil {
		t.Fatal("resource gone after sync")
	}
	if got := r.Object.Events[0].Summary; got != "EditV2" {
		t.Errorf("resource summary = %q, want EditV2 (the concurrent edit preserved, not reverted)", got)
	}
	if !r.Dirty {
		t.Error("resource should stay dirty so the concurrent edit is pushed on the next sync")
	}
	if r.ETag != "new-1" {
		t.Errorf("resource ETag = %q, want the server's new-1 baseline for the next conditional push", r.ETag)
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

// TestSyncSkipsEmptyHrefServerObject locks Pass-3 #8: a server response whose
// href is empty must be skipped and recorded, not stored with Href=="" (which
// the next sync would mistake for a never-pushed local resource and re-upload
// as a server-side duplicate).
func TestSyncSkipsEmptyHrefServerObject(t *testing.T) {
	ctx := context.Background()
	st := newStore(t)
	srv := newFakeServer()
	// Keyed under the calendar so DownloadAll returns it, but the Object's own
	// Path is empty — the malformed <href/> case.
	srv.data[calPath+"ghost.ics"] = caldav.Object{Path: "", ETag: "srv-1", Data: mkICal(t, eventICS("ghost@test", "Ghost"))}

	res, err := sync.Sync(ctx, srv, st)
	if err != nil {
		t.Fatal(err)
	}
	if res.Pulled != 0 {
		t.Errorf("Pulled = %d, want 0 (empty-href object skipped)", res.Pulled)
	}
	if len(res.Skipped) != 1 || !strings.Contains(res.Skipped[0].Err.Error(), "empty href") {
		t.Errorf("Skipped = %+v, want one empty-href skip", res.Skipped)
	}
	if cal, _ := st.Calendar("personal"); len(cal.Resources) != 0 {
		t.Errorf("stored %d resources, want 0", len(cal.Resources))
	}
	if srv.puts != 0 {
		t.Errorf("puts = %d, want 0 (nothing to re-upload)", srv.puts)
	}
}

// TestSyncUnparseableServerConflictNotTreatedAsDeletion locks Pass-3 #4: when
// both sides edited and the server version ical-decodes but fails our stricter
// model.Parse (e.g. a VEVENT missing DTSTART written by another client), the
// conflict must stash the raw server version and NOT be flagged as a deletion —
// otherwise "keep server" would silently discard the local edit as if the
// server had deleted the resource.
func TestSyncUnparseableServerConflictNotTreatedAsDeletion(t *testing.T) {
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
	// Server version present (different etag) but model-unparseable: no DTSTART.
	const noStart = "BEGIN:VCALENDAR\r\nVERSION:2.0\r\nPRODID:-//t//EN\r\n" +
		"BEGIN:VEVENT\r\nUID:e1@test\r\nDTSTAMP:20260701T120000Z\r\nSUMMARY:ServerNoStart\r\nEND:VEVENT\r\nEND:VCALENDAR\r\n"
	srv.data[href] = caldav.Object{Path: href, ETag: "srv-2", Data: mkICal(t, noStart)}

	res, err := sync.Sync(ctx, srv, st)
	if err != nil {
		t.Fatal(err)
	}
	if res.Conflicts != 1 {
		t.Fatalf("res = %+v, want 1 conflict", res)
	}
	confs := st.Conflicts()
	if len(confs) != 1 {
		t.Fatalf("Conflicts() = %d, want 1", len(confs))
	}
	if confs[0].ServerDeleted {
		t.Error("conflict flagged as a server deletion; a present-but-unparseable version must not be")
	}
	if len(confs[0].ServerData) == 0 {
		t.Error("server version not stashed; both versions must survive a conflict")
	}
	// Keep-server must NOT silently discard the local edit: it errors (can't decode
	// the unparseable version) and the local resource stays.
	if err := st.ResolveKeepServer(ctx, "personal", name); err == nil {
		t.Error("keep-server on an unparseable server version should error, not silently succeed")
	}
	if r := findRes(t, st, name); r == nil {
		t.Error("local edit was discarded on keep-server of an unparseable server version (data loss)")
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

func TestSyncPullsCalendarColor(t *testing.T) {
	ctx := context.Background()
	st := newStore(t) // personal calendar, no local color
	srv := newFakeServer()
	srv.cals[0].Color = "#112233FF" // server sets a color

	if _, err := sync.Sync(ctx, srv, st); err != nil {
		t.Fatal(err)
	}
	cal, _ := st.Calendar("personal")
	if cal.Color != "#112233FF" {
		t.Errorf("color after sync = %q, want the server's %q", cal.Color, "#112233FF")
	}
}

func TestSyncDoesNotClobberPendingLocalColor(t *testing.T) {
	ctx := context.Background()
	st := newStore(t)
	srv := newFakeServer()
	srv.cals[0].Color = "#112233FF" // server's current color

	// A local recolor made offline, awaiting a PROPPATCH.
	if err := st.UpdateCalendarMeta(ctx, "personal", "", "#AABBCCFF"); err != nil {
		t.Fatal(err)
	}

	res, err := sync.Sync(ctx, srv, st)
	if err != nil {
		t.Fatal(err)
	}
	// The pending edit is pushed (PROPPATCH), not overwritten by the server's older
	// color: push runs before discovery, and the pull skips a still-pending color.
	if res.CalendarsUpdated != 1 {
		t.Errorf("CalendarsUpdated = %d, want 1 (the local recolor pushed)", res.CalendarsUpdated)
	}
	var pushedColor string
	for _, op := range srv.propPatched {
		if op.color != "" {
			pushedColor = op.color
		}
	}
	if pushedColor != "#AABBCCFF" {
		t.Errorf("pushed color = %q, want the local %q", pushedColor, "#AABBCCFF")
	}
	cal, _ := st.Calendar("personal")
	if cal.Color != "#AABBCCFF" {
		t.Errorf("local color after sync = %q, want the local edit %q preserved", cal.Color, "#AABBCCFF")
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

func TestSyncCreatesLocalCalendarAndPushesItsResources(t *testing.T) {
	ctx := context.Background()
	st, err := store.Open(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	srv := newFakeServer()
	srv.cals = nil // server starts with no calendars

	// A calendar created in-app (offline), holding one local task, awaiting push.
	if err := st.CreateCalendarLocal(ctx, "errands", store.CalendarMeta{DisplayName: "Errands"}, []string{"VTODO"}); err != nil {
		t.Fatal(err)
	}
	name := store.ResourceName("t1@test")
	if _, err := st.Put(ctx, "errands", name, mkParsed(t, eventICS("t1@test", "Buy milk"))); err != nil {
		t.Fatal(err)
	}

	res, err := sync.Sync(ctx, srv, st)
	if err != nil {
		t.Fatal(err)
	}
	if res.CalendarsCreated != 1 {
		t.Fatalf("CalendarsCreated = %d, want 1", res.CalendarsCreated)
	}
	if len(srv.created) != 1 || srv.created[0].DisplayName != "Errands" || srv.created[0].Components[0] != "VTODO" {
		t.Fatalf("MKCALENDAR spec = %+v", srv.created)
	}
	// The calendar is no longer pending, has a server href, and its resource was pushed.
	cal, _ := st.Calendar("errands")
	if cal.PendingCreate || cal.Href == "" {
		t.Errorf("calendar still pending after create: %+v", cal)
	}
	if res.Pushed != 1 {
		t.Errorf("Pushed = %d, want 1 (the task in the new calendar)", res.Pushed)
	}
	if r := findResByName(t, st, "errands", name); r == nil || r.Dirty || r.Href == "" {
		t.Errorf("task not pushed clean: %+v", r)
	}
}

func TestSyncDeletesLocalCalendarOnServer(t *testing.T) {
	ctx := context.Background()
	st := newStore(t) // has "personal" at calPath
	srv := newFakeServer()
	// Seed one synced resource so the calendar isn't empty.
	name := store.ResourceName("e1@test")
	href := calPath + name
	if _, err := st.PutRemote(ctx, "personal", name, mkParsed(t, eventICS("e1@test", "Base")), "srv-1", href); err != nil {
		t.Fatal(err)
	}
	srv.data[href] = caldav.Object{Path: href, ETag: "srv-1", Data: mkICal(t, eventICS("e1@test", "Base"))}

	// Delete the calendar in-app.
	if err := st.MarkCalendarDeleted(ctx, "personal"); err != nil {
		t.Fatal(err)
	}
	// It vanishes from the UI immediately.
	if len(st.Calendars()) != 0 {
		t.Fatalf("deleted calendar still listed: %+v", st.Calendars())
	}

	res, err := sync.Sync(ctx, srv, st)
	if err != nil {
		t.Fatal(err)
	}
	if res.CalendarsDeleted != 1 || len(srv.deletedCals) != 1 || srv.deletedCals[0] != calPath {
		t.Fatalf("DeleteCalendar not issued: res=%+v deleted=%+v", res, srv.deletedCals)
	}
	if _, ok := st.Calendar("personal"); ok {
		t.Error("calendar still present locally after delete+sync")
	}
	// It must not be re-imported by the same sync's discovery pass.
	if res.Calendars != 0 {
		t.Errorf("deleted calendar was reconciled: Calendars=%d", res.Calendars)
	}
}

func TestSyncDeleteNeverPushedCalendarSkipsServer(t *testing.T) {
	ctx := context.Background()
	st, err := store.Open(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	srv := newFakeServer()
	srv.cals = nil
	if err := st.CreateCalendarLocal(ctx, "temp", store.CalendarMeta{DisplayName: "Temp"}, nil); err != nil {
		t.Fatal(err)
	}
	// Delete it before it was ever synced → removed outright, no server call.
	if err := st.MarkCalendarDeleted(ctx, "temp"); err != nil {
		t.Fatal(err)
	}
	if _, ok := st.Calendar("temp"); ok {
		t.Fatal("never-pushed calendar should be removed immediately on delete")
	}
	res, err := sync.Sync(ctx, srv, st)
	if err != nil {
		t.Fatal(err)
	}
	if len(srv.deletedCals) != 0 || res.CalendarsDeleted != 0 || res.CalendarsCreated != 0 {
		t.Errorf("unexpected server calendar ops: deleted=%+v res=%+v", srv.deletedCals, res)
	}
}

func TestSyncReadOnlyDiscardsStuckAndMirrors(t *testing.T) {
	ctx := context.Background()
	st := newStore(t)
	srv := newFakeServer()
	srv.cals[0].ReadOnly = true // Personal is read-only for this test

	// A stuck local event the user added to the read-only calendar (never synced).
	stuck := store.ResourceName("stuck@test")
	if _, err := st.Put(ctx, "personal", stuck, mkParsed(t, eventICS("stuck@test", "Junk"))); err != nil {
		t.Fatal(err)
	}
	// A real server event that should be mirrored in.
	href := calPath + "bday.ics"
	srv.data[href] = caldav.Object{Path: href, ETag: "srv-1", Data: mkICal(t, eventICS("bday@test", "Alice's Birthday"))}

	res, err := sync.Sync(ctx, srv, st)
	if err != nil {
		t.Fatal(err)
	}
	if srv.puts != 0 || srv.deletes != 0 {
		t.Errorf("read-only calendar was written to: puts=%d deletes=%d", srv.puts, srv.deletes)
	}
	if res.Discarded != 1 {
		t.Errorf("Discarded = %d, want 1 (the stuck event)", res.Discarded)
	}
	if findResByName(t, st, "personal", stuck) != nil {
		t.Error("stuck local event was not discarded")
	}
	// The server's birthday event is pulled in.
	cal, _ := st.Calendar("personal")
	found := false
	for _, r := range cal.Resources {
		if len(r.Object.Events) > 0 && r.Object.Events[0].Summary == "Alice's Birthday" {
			found = true
		}
	}
	if !found {
		t.Errorf("server event not mirrored into the read-only calendar: %+v", cal.Resources)
	}
	// The calendar is flagged read-only locally for the UI.
	if !cal.ReadOnly {
		t.Error("calendar not flagged read-only in the store")
	}
}

func TestSyncReactiveReadOnlyOn403(t *testing.T) {
	ctx := context.Background()
	st := newStore(t)
	srv := newFakeServer() // Personal reports writable (privilege detection missed it)

	name := store.ResourceName("e1@test")
	if _, err := st.Put(ctx, "personal", name, mkParsed(t, eventICS("e1@test", "Local"))); err != nil {
		t.Fatal(err)
	}
	// The server refuses the write with 403, AND the privilege re-check confirms
	// the calendar is genuinely read-only.
	srv.failPut[calPath+name] = caldav.ErrReadOnly
	srv.writable[calPath] = false

	res, err := sync.Sync(ctx, srv, st)
	if err != nil {
		t.Fatal(err)
	}
	if res.Discarded != 1 {
		t.Fatalf("Discarded = %d, want 1", res.Discarded)
	}
	if findResByName(t, st, "personal", name) != nil {
		t.Error("stuck event not discarded after a confirmed-read-only 403")
	}
	// The calendar is now flagged read-only so future syncs won't retry.
	cal, _ := st.Calendar("personal")
	if !cal.ReadOnly {
		t.Error("calendar not marked read-only after a confirmed 403")
	}
}

// TestSyncTransient403KeepsEdit: a 403 whose privilege re-check still reports the
// calendar writable is treated as transient — the local edit is kept (not
// discarded) and the calendar is not flagged read-only, so it retries next sync.
func TestSyncTransient403KeepsEdit(t *testing.T) {
	ctx := context.Background()
	st := newStore(t)
	srv := newFakeServer()

	name := store.ResourceName("e1@test")
	if _, err := st.Put(ctx, "personal", name, mkParsed(t, eventICS("e1@test", "Local"))); err != nil {
		t.Fatal(err)
	}
	srv.failPut[calPath+name] = caldav.ErrReadOnly
	// Default writable = true (re-check says the calendar still grants write).

	res, err := sync.Sync(ctx, srv, st)
	if err != nil {
		t.Fatal(err)
	}
	if res.Discarded != 0 {
		t.Errorf("Discarded = %d, want 0 (transient 403 must not drop the edit)", res.Discarded)
	}
	if findResByName(t, st, "personal", name) == nil {
		t.Error("local edit was lost on a transient 403")
	}
	if cal, _ := st.Calendar("personal"); cal.ReadOnly {
		t.Error("calendar wrongly flagged read-only on a transient 403")
	}
}

func findResByName(t *testing.T, st *store.Store, calID, name string) *store.Resource {
	t.Helper()
	cal, _ := st.Calendar(calID)
	for _, r := range cal.Resources {
		if r.Name == name {
			return r
		}
	}
	return nil
}

func TestSyncPushesCalendarRename(t *testing.T) {
	ctx := context.Background()
	st, err := store.Open(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	srv := newFakeServer()
	href := calPath + "e1.ics"
	srv.data[href] = caldav.Object{Path: href, ETag: "srv-1", Data: mkICal(t, eventICS("e1@test", "Hello"))}

	// First sync imports the server calendar "personal" (with its href).
	if _, err := sync.Sync(ctx, srv, st); err != nil {
		t.Fatal(err)
	}
	// Rename + recolor it locally, then sync again to push the PROPPATCH.
	if err := st.UpdateCalendarMeta(ctx, "personal", "Renamed", "#123456"); err != nil {
		t.Fatal(err)
	}
	res, err := sync.Sync(ctx, srv, st)
	if err != nil {
		t.Fatal(err)
	}
	if res.CalendarsUpdated != 1 {
		t.Errorf("CalendarsUpdated = %d, want 1", res.CalendarsUpdated)
	}
	if len(srv.propPatched) != 1 {
		t.Fatalf("PROPPATCH calls = %d, want 1", len(srv.propPatched))
	}
	got := srv.propPatched[0]
	if got.path != calPath || got.displayName != "Renamed" || got.color != "#123456" {
		t.Errorf("PROPPATCH = %+v, want path=%s name=Renamed color=#123456", got, calPath)
	}
	// Idempotent: a third sync must not re-push (pending-props was cleared).
	srv.propPatched = nil
	if _, err := sync.Sync(ctx, srv, st); err != nil {
		t.Fatal(err)
	}
	if len(srv.propPatched) != 0 {
		t.Errorf("re-pushed PROPPATCH after it was synced: %d", len(srv.propPatched))
	}
}

func TestSyncRecordsComponentSet(t *testing.T) {
	ctx := context.Background()
	st, err := store.Open(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	srv := newFakeServer()
	// Server reports this calendar as a task list (VTODO), and it's empty.
	srv.cals = []caldav.Calendar{{Path: calPath, Name: "Personal", SupportedComponentSet: []string{"VTODO"}}}

	if _, err := sync.Sync(ctx, srv, st); err != nil {
		t.Fatal(err)
	}
	cal, ok := st.Calendar("personal")
	if !ok {
		t.Fatal("calendar not imported")
	}
	if len(cal.Components) != 1 || cal.Components[0] != "VTODO" {
		t.Errorf("Components = %v, want [VTODO] (so an empty task list is recognizable)", cal.Components)
	}
}

// TestSyncSkipsFailedCalendarContinuesRest: a calendar whose download fails is
// recorded as a skip and doesn't block the others — a healthy calendar listed
// after it still syncs (its pending local edit is pushed).
func TestSyncSkipsFailedCalendarContinuesRest(t *testing.T) {
	ctx := context.Background()
	st := newStore(t)
	srv := newFakeServer()

	// Two calendars, the failing one FIRST so we prove it doesn't block Personal.
	const badPath = "/dav/cal/work/"
	srv.cals = []caldav.Calendar{
		{Path: badPath, Name: "Work"},
		{Path: calPath, Name: "Personal"},
	}
	srv.failDownload[badPath] = errors.New("REPORT 500")

	// A pending local edit in the healthy Personal calendar.
	name := store.ResourceName("e1@test")
	if _, err := st.Put(ctx, "personal", name, mkParsed(t, eventICS("e1@test", "Local"))); err != nil {
		t.Fatal(err)
	}

	res, err := sync.Sync(ctx, srv, st)
	if err != nil {
		t.Fatalf("Sync should not abort on one calendar's failure: %v", err)
	}
	if res.Pushed != 1 {
		t.Errorf("Pushed = %d, want 1 (Personal synced despite Work failing first)", res.Pushed)
	}
	if len(res.Skipped) == 0 {
		t.Error("the failed calendar should be recorded in Skipped")
	}
}

// TestSyncPullsCalendarRename: a calendar renamed on the server is adopted locally.
func TestSyncPullsCalendarRename(t *testing.T) {
	ctx := context.Background()
	st := newStore(t) // "personal" → DisplayName "Personal"
	srv := newFakeServer()
	srv.cals[0].Name = "Home" // renamed on the server

	if _, err := sync.Sync(ctx, srv, st); err != nil {
		t.Fatal(err)
	}
	cal, _ := st.Calendar("personal")
	if cal.DisplayName != "Home" {
		t.Errorf("display name after sync = %q, want the server's Home", cal.DisplayName)
	}
}

// TestSyncDoesNotClobberPendingLocalRename: a local rename awaiting a PROPPATCH
// is pushed and kept, not overwritten by the server's older name.
func TestSyncDoesNotClobberPendingLocalRename(t *testing.T) {
	ctx := context.Background()
	st := newStore(t)
	srv := newFakeServer()
	srv.cals[0].Name = "Home" // server's current name

	if err := st.UpdateCalendarMeta(ctx, "personal", "MyStuff", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := sync.Sync(ctx, srv, st); err != nil {
		t.Fatal(err)
	}
	cal, _ := st.Calendar("personal")
	if cal.DisplayName != "MyStuff" {
		t.Errorf("pending local rename should win until pushed, got %q", cal.DisplayName)
	}
	// And it was pushed to the server (PROPPATCH), so both sides now agree.
	if len(srv.propPatched) == 0 || srv.propPatched[len(srv.propPatched)-1].displayName != "MyStuff" {
		t.Errorf("local rename not pushed via PROPPATCH: %+v", srv.propPatched)
	}
}

// TestSyncRefetchesOn412: a push that 412s re-fetches the current server version
// (not the stale start-of-sync one) and stashes it as the conflict.
func TestSyncRefetchesOn412(t *testing.T) {
	ctx := context.Background()
	st := newStore(t)
	srv := newFakeServer()
	name := store.ResourceName("e1@test")
	href := calPath + name

	// Synced clean at srv-1 on both sides.
	if _, err := st.PutRemote(ctx, "personal", name, mkParsed(t, eventICS("e1@test", "Base")), "srv-1", href); err != nil {
		t.Fatal(err)
	}
	srv.data[href] = caldav.Object{Path: href, ETag: "srv-1", Data: mkICal(t, eventICS("e1@test", "Base"))}
	// Local edit (dirty, still srv-1 on our side).
	if _, err := st.Put(ctx, "personal", name, mkParsed(t, eventICS("e1@test", "LocalEdit"))); err != nil {
		t.Fatal(err)
	}
	// The PUT 412s; the *current* server version (what a re-fetch returns) is srv-2.
	srv.failPut[href] = caldav.ErrPreconditionFailed
	srv.getData[href] = caldav.Object{Path: href, ETag: "srv-2", Data: mkICal(t, eventICS("e1@test", "ServerEdit"))}

	res, err := sync.Sync(ctx, srv, st)
	if err != nil {
		t.Fatal(err)
	}
	if res.Conflicts != 1 {
		t.Fatalf("Conflicts = %d, want 1", res.Conflicts)
	}
	if srv.gets == 0 {
		t.Error("GetObject was not called on the 412")
	}
	cons := st.Conflicts()
	if len(cons) != 1 {
		t.Fatalf("stored conflicts = %d, want 1", len(cons))
	}
	if cons[0].ServerETag != "srv-2" {
		t.Errorf("stashed conflict ETag = %q, want the fresh srv-2 (not the stale srv-1)", cons[0].ServerETag)
	}
}

// TestSyncCTagShortCircuit: an unchanged CTag with nothing local to push lets a
// second sync skip the full download; a changed CTag forces a re-download.
func TestSyncCTagShortCircuit(t *testing.T) {
	ctx := context.Background()
	st := newStore(t)
	srv := newFakeServer()
	srv.cals = []caldav.Calendar{{Path: calPath, Name: "Personal", CTag: "ctag-1"}}

	// First sync downloads and records the CTag.
	res1, err := sync.Sync(ctx, srv, st)
	if err != nil {
		t.Fatal(err)
	}
	if srv.downloads != 1 || res1.CalendarsUnchanged != 0 {
		t.Fatalf("first sync: downloads=%d unchanged=%d, want 1/0", srv.downloads, res1.CalendarsUnchanged)
	}

	// Second sync: same CTag, nothing local → skip the download.
	res2, err := sync.Sync(ctx, srv, st)
	if err != nil {
		t.Fatal(err)
	}
	if srv.downloads != 1 || res2.CalendarsUnchanged != 1 {
		t.Fatalf("second sync: downloads=%d unchanged=%d, want 1/1 (short-circuit)", srv.downloads, res2.CalendarsUnchanged)
	}

	// A changed CTag forces a fresh download.
	srv.cals[0].CTag = "ctag-2"
	res3, err := sync.Sync(ctx, srv, st)
	if err != nil {
		t.Fatal(err)
	}
	if srv.downloads != 2 || res3.CalendarsUnchanged != 0 {
		t.Fatalf("third sync: downloads=%d unchanged=%d, want 2/0 (CTag changed)", srv.downloads, res3.CalendarsUnchanged)
	}
}

// TestSyncCTagPushStillHappens: a pending local change is pushed even when the
// server CTag is unchanged (the short-circuit only applies when nothing is local).
func TestSyncCTagPushStillHappens(t *testing.T) {
	ctx := context.Background()
	st := newStore(t)
	srv := newFakeServer()
	srv.cals = []caldav.Calendar{{Path: calPath, Name: "Personal", CTag: "ctag-1"}}
	if _, err := sync.Sync(ctx, srv, st); err != nil { // record CTag
		t.Fatal(err)
	}

	// Create a local resource; the CTag is still "ctag-1" but we must push.
	name := store.ResourceName("e9@test")
	if _, err := st.Put(ctx, "personal", name, mkParsed(t, eventICS("e9@test", "Local"))); err != nil {
		t.Fatal(err)
	}
	res, err := sync.Sync(ctx, srv, st)
	if err != nil {
		t.Fatal(err)
	}
	if res.Pushed != 1 || res.CalendarsUnchanged != 0 {
		t.Fatalf("pushed=%d unchanged=%d, want 1/0 (local change must not be short-circuited)", res.Pushed, res.CalendarsUnchanged)
	}
}

// setupPendingDelete puts a synced resource on both sides, then deletes it locally
// (leaving a tombstone), and makes the server refuse the DELETE with 403.
func setupPendingDelete(t *testing.T) (*store.Store, *fakeServer, string) {
	t.Helper()
	ctx := context.Background()
	st := newStore(t)
	srv := newFakeServer()
	name := store.ResourceName("d1@test")
	href := calPath + name
	if _, err := st.PutRemote(ctx, "personal", name, mkParsed(t, eventICS("d1@test", "X")), "e1", href); err != nil {
		t.Fatal(err)
	}
	srv.data[href] = caldav.Object{Path: href, ETag: "e1", Data: mkICal(t, eventICS("d1@test", "X"))}
	if err := st.Delete(ctx, "personal", name); err != nil {
		t.Fatal(err)
	}
	srv.failDel[href] = caldav.ErrReadOnly // server refuses the delete with 403
	return st, srv, name
}

func tombstoneCount(st *store.Store, calID string) int {
	n := 0
	for _, tm := range st.Tombstones() {
		if tm.CalID == calID {
			n++
		}
	}
	return n
}

// TestSyncDeleteTransient403KeepsTombstone: a 403 on delete that the privilege
// re-check does NOT confirm read-only keeps the pending delete for retry (rather
// than resurrecting the item and dropping the tombstone).
func TestSyncDeleteTransient403KeepsTombstone(t *testing.T) {
	st, srv, name := setupPendingDelete(t)
	// writable map left empty → CalendarWritable returns true (transient 403).
	res, err := sync.Sync(context.Background(), srv, st)
	if err != nil {
		t.Fatal(err)
	}
	if res.Discarded != 0 {
		t.Errorf("Discarded=%d, want 0 (a transient 403 must not discard the delete)", res.Discarded)
	}
	if len(res.Skipped) == 0 {
		t.Error("expected the refused delete to be recorded as a skip (retry next sync)")
	}
	if tombstoneCount(st, "personal") != 1 {
		t.Errorf("tombstone dropped after a transient 403 (%d), want it kept for retry", tombstoneCount(st, "personal"))
	}
	_ = name
}

// TestSyncDeleteConfirmedReadOnlyDiscards: a 403 the re-check CONFIRMS read-only
// flags the calendar, resurrects the item, and drops the tombstone.
func TestSyncDeleteConfirmedReadOnlyDiscards(t *testing.T) {
	st, srv, _ := setupPendingDelete(t)
	srv.writable[calPath] = false // re-check confirms read-only
	res, err := sync.Sync(context.Background(), srv, st)
	if err != nil {
		t.Fatal(err)
	}
	if res.Discarded != 1 {
		t.Errorf("Discarded=%d, want 1 (a confirmed read-only calendar discards the stuck delete)", res.Discarded)
	}
	if tombstoneCount(st, "personal") != 0 {
		t.Error("confirmed read-only should drop the tombstone")
	}
}
