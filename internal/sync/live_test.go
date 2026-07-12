//go:build live

// Package live tests run against a real CalDAV server and are excluded from the
// normal build. Run them explicitly against a *test* account:
//
//	go test -tags live -run TestLive ./internal/sync/ -v
//
// Credentials come from the normal config (~/.config/lazyplanner/config.toml),
// loaded via config.Load, so no secret is passed on the command line. Every test
// operates only inside a throwaway calendar it creates and deletes; it never
// writes to a pre-existing calendar.
package sync_test

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/littekge/LazyPlanner/internal/caldav"
	"github.com/littekge/LazyPlanner/internal/config"
	"github.com/littekge/LazyPlanner/internal/model"
	"github.com/littekge/LazyPlanner/internal/store"
	"github.com/littekge/LazyPlanner/internal/sync"
)

// throwawayCalendar creates a uniquely-named calendar on the server and registers
// its deletion for after the test, so a live test never touches a pre-existing
// calendar. It returns the collection path.
func throwawayCalendar(t *testing.T, c *caldav.Client, ctx context.Context, comps ...string) string {
	t.Helper()
	homeSet, err := c.CalendarHomeSet(ctx)
	if err != nil {
		t.Fatalf("home set: %v", err)
	}
	seg := fmt.Sprintf("lazyplanner_livetest_%d", time.Now().UnixNano())
	path := strings.TrimRight(homeSet, "/") + "/" + seg + "/"
	if err := c.CreateCalendar(ctx, path, caldav.CalendarSpec{
		DisplayName: "LazyPlanner Live Test",
		Color:       "#3366cc",
		Components:  comps,
	}); err != nil {
		t.Fatalf("MKCALENDAR %q: %v", path, err)
	}
	t.Cleanup(func() {
		cctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if err := c.DeleteCalendar(cctx, path); err != nil {
			t.Errorf("CLEANUP FAILED: delete throwaway calendar %q manually: %v", path, err)
		}
	})
	return path
}

func findCalIDByHref(t *testing.T, st *store.Store, href string) string {
	t.Helper()
	want := strings.TrimRight(href, "/")
	for _, c := range st.Calendars() {
		if strings.TrimRight(c.Href, "/") == want {
			return c.ID
		}
	}
	t.Fatalf("throwaway calendar %q not found locally after sync", href)
	return ""
}

// serverSummaries returns the SUMMARY of every event in a server calendar,
// keyed by UID, by downloading and parsing it fresh.
func serverSummaries(t *testing.T, c *caldav.Client, ctx context.Context, path string) map[string]string {
	t.Helper()
	objs, err := c.DownloadAll(ctx, path)
	if err != nil {
		t.Fatalf("DownloadAll %q: %v", path, err)
	}
	out := map[string]string{}
	for _, o := range objs {
		p, err := model.Parse(o.Data, time.Local)
		if err != nil {
			t.Errorf("server object %q did not parse: %v", o.Path, err)
			continue
		}
		for _, ev := range p.Events {
			out[ev.UID] = ev.Summary
		}
	}
	return out
}

// liveClient builds a real CalDAV client from the configured account, skipping
// the test when no server is configured.
func liveClient(t *testing.T) *caldav.Client {
	t.Helper()
	cfg, configured, _, err := config.Load()
	if err != nil {
		t.Fatalf("loading config: %v", err)
	}
	if !configured || !cfg.Server.Configured() {
		t.Skip("no live server configured; set up ~/.config/lazyplanner/config.toml")
	}
	pw, err := cfg.Server.ResolvePassword(context.Background())
	if err != nil {
		t.Fatalf("resolving password: %v", err)
	}
	c, err := caldav.NewClient(caldav.Config{Endpoint: cfg.Server.URL, Username: cfg.Server.Username, Password: pw})
	if err != nil {
		t.Fatalf("building client: %v", err)
	}
	return c
}

// TestLiveDiscover is the connectivity smoke test: it lists the account's
// calendars (read-only) and prints their paths, component sets, and read-only
// flags so the round-trip test's assumptions can be eyeballed.
func TestLiveDiscover(t *testing.T) {
	c := liveClient(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	cals, err := c.DiscoverCalendars(ctx)
	if err != nil {
		t.Fatalf("discovering calendars: %v", err)
	}
	if len(cals) == 0 {
		t.Fatal("no calendars discovered; expected at least one")
	}
	for _, cal := range cals {
		t.Logf("calendar: path=%q name=%q components=%v color=%q readonly=%v hasctag=%v",
			cal.Path, cal.Name, cal.SupportedComponentSet, cal.Color, cal.ReadOnly, cal.CTag != "")
	}
}

// TestLiveRoundTrip exercises the full two-way sync against the real server in a
// throwaway calendar: create → push → verify on server, edit → push → verify,
// then delete → push → verify gone. It also checks the CTag incremental
// short-circuit (a repeat sync with nothing to do reports calendars unchanged).
func TestLiveRoundTrip(t *testing.T) {
	c := liveClient(t)
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	path := throwawayCalendar(t, c, ctx, "VEVENT")
	st, err := store.Open(ctx, t.TempDir())
	if err != nil {
		t.Fatalf("store open: %v", err)
	}

	if _, err := sync.Sync(ctx, c, st); err != nil {
		t.Fatalf("initial sync: %v", err)
	}
	calID := findCalIDByHref(t, st, path)

	uid := fmt.Sprintf("livetest-%d@evt", time.Now().UnixNano())
	name := store.ResourceName(uid)

	// create → push
	if _, err := st.Put(ctx, calID, name, mkParsed(t, eventICS(uid, "Live created"))); err != nil {
		t.Fatalf("local create: %v", err)
	}
	if _, err := sync.Sync(ctx, c, st); err != nil {
		t.Fatalf("push (create) sync: %v", err)
	}
	if got := serverSummaries(t, c, ctx, path); got[uid] != "Live created" {
		t.Fatalf("after create, server summary = %q, want %q", got[uid], "Live created")
	}

	// edit → push
	if _, err := st.Put(ctx, calID, name, mkParsed(t, eventICS(uid, "Live edited"))); err != nil {
		t.Fatalf("local edit: %v", err)
	}
	if _, err := sync.Sync(ctx, c, st); err != nil {
		t.Fatalf("push (edit) sync: %v", err)
	}
	if got := serverSummaries(t, c, ctx, path); got[uid] != "Live edited" {
		t.Fatalf("after edit, server summary = %q, want %q", got[uid], "Live edited")
	}

	// CTag short-circuit: nothing changed locally or on the server → a repeat sync
	// should skip re-downloading at least our calendar.
	res, err := sync.Sync(ctx, c, st)
	if err != nil {
		t.Fatalf("idle sync: %v", err)
	}
	if res.CalendarsUnchanged == 0 {
		t.Errorf("idle sync reported 0 unchanged calendars; CTag short-circuit did not engage (res=%+v)", res)
	}

	// delete → push
	if err := st.Delete(ctx, calID, name); err != nil {
		t.Fatalf("local delete: %v", err)
	}
	if _, err := sync.Sync(ctx, c, st); err != nil {
		t.Fatalf("push (delete) sync: %v", err)
	}
	if got := serverSummaries(t, c, ctx, path); len(got) != 0 {
		t.Fatalf("after delete, server still has %d event(s): %v", len(got), got)
	}
}

// TestLiveCalendarProps round-trips a calendar rename + recolor: it PROPPATCHes
// the throwaway calendar and confirms a fresh discovery reflects both.
func TestLiveCalendarProps(t *testing.T) {
	c := liveClient(t)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	path := throwawayCalendar(t, c, ctx, "VEVENT")

	const wantName, wantColor = "LazyPlanner Renamed", "#12ab34"
	if err := c.SetCalendarProps(ctx, path, wantName, wantColor); err != nil {
		t.Fatalf("PROPPATCH: %v", err)
	}
	cals, err := c.DiscoverCalendars(ctx)
	if err != nil {
		t.Fatalf("re-discover: %v", err)
	}
	want := strings.TrimRight(path, "/")
	var found *caldav.Calendar
	for i := range cals {
		if strings.TrimRight(cals[i].Path, "/") == want {
			found = &cals[i]
			break
		}
	}
	if found == nil {
		t.Fatal("renamed calendar not found in discovery")
	}
	if found.Name != wantName {
		t.Errorf("server name = %q, want %q", found.Name, wantName)
	}
	if !strings.EqualFold(strings.TrimRight(found.Color, "fF"), strings.TrimRight(wantColor, "fF")) {
		t.Errorf("server color = %q, want %q (ignoring alpha)", found.Color, wantColor)
	}
}

// TestLiveConflict verifies the keep-both conflict path against the real server:
// a resource edited both locally and directly on the server must sync to a
// recorded conflict, not a silent overwrite.
func TestLiveConflict(t *testing.T) {
	c := liveClient(t)
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	path := throwawayCalendar(t, c, ctx, "VEVENT")
	st, err := store.Open(ctx, t.TempDir())
	if err != nil {
		t.Fatalf("store open: %v", err)
	}
	if _, err := sync.Sync(ctx, c, st); err != nil {
		t.Fatalf("initial sync: %v", err)
	}
	calID := findCalIDByHref(t, st, path)

	uid := fmt.Sprintf("livetest-%d@conflict", time.Now().UnixNano())
	name := store.ResourceName(uid)
	if _, err := st.Put(ctx, calID, name, mkParsed(t, eventICS(uid, "Base"))); err != nil {
		t.Fatalf("local create: %v", err)
	}
	if _, err := sync.Sync(ctx, c, st); err != nil {
		t.Fatalf("push sync: %v", err)
	}

	// Server-side edit via a second, direct PUT (conditional on the current ETag).
	objs, err := c.DownloadAll(ctx, path)
	if err != nil || len(objs) != 1 {
		t.Fatalf("download after create: objs=%d err=%v", len(objs), err)
	}
	serverEdit, err := mkParsed(t, eventICS(uid, "Server edit")).Encode()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.PutObject(ctx, objs[0].Path, serverEdit, objs[0].ETag, false); err != nil {
		t.Fatalf("server-side PUT: %v", err)
	}

	// Local edit of the same resource (still at the old ETag), then sync.
	if _, err := st.Put(ctx, calID, name, mkParsed(t, eventICS(uid, "Local edit"))); err != nil {
		t.Fatalf("local edit: %v", err)
	}
	if _, err := sync.Sync(ctx, c, st); err != nil {
		t.Fatalf("conflict sync: %v", err)
	}

	confs := st.Conflicts()
	if len(confs) != 1 {
		t.Fatalf("Conflicts() = %d, want 1 (both-edited must keep both)", len(confs))
	}
	if confs[0].ServerDeleted {
		t.Error("conflict flagged as server deletion; the server version is present")
	}
	if len(confs[0].ServerData) == 0 {
		t.Error("conflict stashed no server version; both sides must be preserved")
	}
}
