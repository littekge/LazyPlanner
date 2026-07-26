package store_test

import (
	"context"
	"fmt"
	"hash/fnv"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/littekge/LazyPlanner/internal/store"
)

const sidecarFile = ".lazyplanner.json"

// unsyncedICS is an event written locally and never pushed.
const unsyncedICS = "BEGIN:VCALENDAR\r\nVERSION:2.0\r\nPRODID:-//test//EN\r\n" +
	"BEGIN:VEVENT\r\nUID:u1@test\r\nDTSTAMP:20260701T000000Z\r\n" +
	"DTSTART:20260704T090000Z\r\nDTEND:20260704T100000Z\r\n" +
	"SUMMARY:LOCAL EDIT (unsynced)\r\nEND:VEVENT\r\nEND:VCALENDAR\r\n"

// writeCalDir lays out one calendar directory holding unsyncedICS as u1.ics
// plus the given sidecar bytes, and returns (root, calendar dir).
func writeCalDir(t *testing.T, sidecarJSON string) (string, string) {
	t.Helper()
	root := t.TempDir()
	dir := filepath.Join(root, "calendars", "personal")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "u1.ics"), []byte(unsyncedICS), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, sidecarFile), []byte(sidecarJSON), 0o644); err != nil {
		t.Fatal(err)
	}
	return root, dir
}

// contentHashOfUnsyncedICS mirrors the hash the store records for u1.ics, so a
// fixture sidecar can look exactly like one the app wrote (and the crash-window
// heal in loadResource cannot be what rescues the dirty flag).
func contentHashOfUnsyncedICS() string {
	h := fnv.New64a()
	_, _ = h.Write([]byte(unsyncedICS))
	return fmt.Sprintf("%016x", h.Sum64())
}

// syncedSidecar is a well-formed sidecar for the fixture above. badField makes
// one resource field carry the wrong JSON type; everything else stays valid.
func syncedSidecar(badField bool) string {
	dirty := "true"
	if badField {
		dirty = "1"
	}
	return fmt.Sprintf(`{
  "display_name": "Personal",
  "sync_token": "http://sabre.io/ns/sync/42",
  "href": "/dav/cal/personal/",
  "read_only": true,
  "resources": {
    "u1.ics": {
      "etag": "srv-1",
      "href": "/dav/cal/personal/u1.ics",
      "dirty": %s,
      "hash": "%s"
    }
  },
  "tombstones": {
    "u2.ics": { "href": "/dav/cal/personal/u2.ics", "etag": "srv-2" }
  }
}`, dirty, contentHashOfUnsyncedICS())
}

func resourceByName(t *testing.T, cal store.Calendar, name string) *store.Resource {
	t.Helper()
	for _, r := range cal.Resources {
		if r.Name == name {
			return r
		}
	}
	t.Fatalf("%s missing from calendar %q", name, cal.ID)
	return nil
}

// TestCorruptSidecarKeepsSyncState guards the data-loss class where a sidecar
// that fails to parse discards ALL of a calendar's sync metadata — dirty flags,
// ETags/hrefs, tombstones, read-only — and is then overwritten with the empty
// state. The next sync would clobber unsynced local edits and resurrect deleted
// items, with nothing on disk left to recover from.
func TestCorruptSidecarKeepsSyncState(t *testing.T) {
	ctx := context.Background()
	root, dir := writeCalDir(t, syncedSidecar(true))
	scPath := filepath.Join(dir, sidecarFile)

	s, err := store.Open(ctx, root)
	if err != nil {
		t.Fatal(err)
	}
	if len(s.LoadErrors()) == 0 {
		t.Fatal("expected the sidecar parse failure recorded in LoadErrors")
	}
	t.Logf("LoadErrors: %v", s.LoadErrors())

	cal, ok := s.Calendar("personal")
	if !ok {
		t.Fatal("personal calendar missing")
	}
	u1 := resourceByName(t, cal, "u1.ics")

	// (1) The unsynced local edit must not load clean, or the next pull overwrites it.
	if !u1.Dirty {
		t.Errorf("u1.ics Dirty=false, want true — the unsynced local edit will be silently overwritten by the next pull")
	}
	if u1.ETag != "srv-1" || u1.Href != "/dav/cal/personal/u1.ics" {
		t.Errorf("u1.ics ETag=%q Href=%q, want %q/%q — server identity lost", u1.ETag, u1.Href, "srv-1", "/dav/cal/personal/u1.ics")
	}

	// (2) The pending server-side deletion must survive, or u2.ics resurrects.
	if len(s.Tombstones()) != 1 {
		t.Errorf("Tombstones() = %v, want the pending u2.ics deletion — it will resurrect on the next sync", s.Tombstones())
	}

	// (3) The cached read-only flag must survive; the UI gates writes on it.
	if !cal.ReadOnly {
		t.Errorf("ReadOnly=false, want true — the UI will permit writes to a read-only calendar")
	}

	// (4) The next store write must not leave the recovered state unrecoverable.
	if err := s.SetCalendarCTag(ctx, "personal", "ctag-1"); err != nil {
		t.Fatal(err)
	}
	after, err := os.ReadFile(scPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(after), "srv-1") || !strings.Contains(string(after), "u2.ics") {
		t.Errorf("the rewritten sidecar dropped the salvaged sync state; file is now:\n%s", after)
	}
}

// TestCorruptSidecarIsQuarantinedIntact checks the other half of the guarantee:
// whatever the salvage pass could not recover is still on disk byte-for-byte, so
// the rewrite that follows destroys nothing.
func TestCorruptSidecarIsQuarantinedIntact(t *testing.T) {
	ctx := context.Background()
	original := syncedSidecar(true)
	root, dir := writeCalDir(t, original)

	s, err := store.Open(ctx, root)
	if err != nil {
		t.Fatal(err)
	}
	// Force the rewrite that used to destroy the evidence.
	if err := s.SetCalendarCTag(ctx, "personal", "ctag-1"); err != nil {
		t.Fatal(err)
	}

	quarantined, err := os.ReadFile(filepath.Join(dir, sidecarFile+".corrupt"))
	if err != nil {
		t.Fatalf("no quarantine copy of the corrupt sidecar: %v", err)
	}
	if string(quarantined) != original {
		t.Errorf("quarantine copy differs from the original bytes:\ngot:\n%s\nwant:\n%s", quarantined, original)
	}
}

// TestValidSidecarLoadsFullState is the other side of the class: the salvage
// path must not quietly change how a healthy sidecar loads. A false "corrupt"
// verdict would mark every resource dirty (needless pushes) or lock a writable
// calendar read-only, so the normal path is asserted end to end.
func TestValidSidecarLoadsFullState(t *testing.T) {
	ctx := context.Background()
	root, dir := writeCalDir(t, syncedSidecar(false))

	s, err := store.Open(ctx, root)
	if err != nil {
		t.Fatal(err)
	}
	if errs := s.LoadErrors(); len(errs) != 0 {
		t.Fatalf("valid sidecar reported load errors: %v", errs)
	}
	if _, err := os.Stat(filepath.Join(dir, sidecarFile+".corrupt")); !os.IsNotExist(err) {
		t.Errorf("a valid sidecar must not be quarantined (stat err = %v)", err)
	}

	cal, ok := s.Calendar("personal")
	if !ok {
		t.Fatal("personal calendar missing")
	}
	if cal.DisplayName != "Personal" || cal.SyncToken != "http://sabre.io/ns/sync/42" || cal.Href != "/dav/cal/personal/" {
		t.Errorf("calendar metadata = %+v, want the sidecar's display name / sync token / href", cal)
	}
	if !cal.ReadOnly {
		t.Error("ReadOnly=false, want true")
	}
	u1 := resourceByName(t, cal, "u1.ics")
	if !u1.Dirty || u1.ETag != "srv-1" || u1.Href != "/dav/cal/personal/u1.ics" {
		t.Errorf("u1.ics = %+v, want the sidecar's dirty flag, ETag and href", u1)
	}
	if ts := s.Tombstones(); len(ts) != 1 || ts[0].Name != "u2.ics" || ts[0].ETag != "srv-2" {
		t.Errorf("Tombstones() = %v, want the pending u2.ics deletion", ts)
	}
}

// TestCleanSidecarKeepsCleanResourcesClean pins the cost side of the fix: when a
// sidecar parses, an already-synced resource must still load clean. Marking
// everything dirty unconditionally would re-push the whole cache on every start.
func TestCleanSidecarKeepsCleanResourcesClean(t *testing.T) {
	ctx := context.Background()
	root, _ := writeCalDir(t, fmt.Sprintf(`{
  "resources": { "u1.ics": { "etag": "srv-1", "href": "/dav/cal/personal/u1.ics", "hash": "%s" } }
}`, contentHashOfUnsyncedICS()))

	s, err := store.Open(ctx, root)
	if err != nil {
		t.Fatal(err)
	}
	cal, ok := s.Calendar("personal")
	if !ok {
		t.Fatal("personal calendar missing")
	}
	if u1 := resourceByName(t, cal, "u1.ics"); u1.Dirty {
		t.Error("u1.ics Dirty=true, want false — a synced resource under a valid sidecar must stay clean")
	}
	if cal.ReadOnly {
		t.Error("ReadOnly=true, want false — a valid sidecar without read_only means writable")
	}
}

// TestUnsalvageableSidecarTreatsStateAsUnknown covers the worst case — bytes that
// aren't even a JSON object (a truncated or overwritten file). Nothing can be
// recovered, so every resource must load as a possibly-unsynced edit.
//
// The calendar deliberately stays WRITABLE: an unparseable sidecar is exactly when
// the user is most likely to be working offline from the cache, so locking the
// calendar would block editing in an offline-first app. Loading every resource
// Dirty is what protects the data — nothing is pushed over silently. Only the
// server's recorded privilege makes a calendar read-only.
func TestUnsalvageableSidecarTreatsStateAsUnknown(t *testing.T) {
	ctx := context.Background()
	original := "{\"display_name\": \"Personal\", \"resources\": {\"u1.ic"
	root, dir := writeCalDir(t, original)

	s, err := store.Open(ctx, root)
	if err != nil {
		t.Fatal(err)
	}
	if len(s.LoadErrors()) == 0 {
		t.Fatal("expected the sidecar parse failure recorded in LoadErrors")
	}
	cal, ok := s.Calendar("personal")
	if !ok {
		t.Fatal("personal calendar missing")
	}
	if u1 := resourceByName(t, cal, "u1.ics"); !u1.Dirty {
		t.Error("u1.ics Dirty=false, want true — with the sync state unknown the local .ics may be an unsynced edit")
	}
	if cal.ReadOnly {
		t.Error("ReadOnly=true, want false — a corrupt sidecar must not lock the user out of editing offline")
	}
	quarantined, err := os.ReadFile(filepath.Join(dir, sidecarFile+".corrupt"))
	if err != nil || string(quarantined) != original {
		t.Errorf("original bytes not preserved: %q, err=%v", quarantined, err)
	}
}

// TestPartialSidecarMarksUnrecoveredResourcesDirty checks the per-entry rule: a
// resource whose entry survived keeps its exact recorded state, while one whose
// entry was lost (or only partly readable) is treated as unsynced rather than as
// never-tracked.
func TestPartialSidecarMarksUnrecoveredResourcesDirty(t *testing.T) {
	ctx := context.Background()
	root, dir := writeCalDir(t, fmt.Sprintf(`{
  "read_only": "yes",
  "resources": {
    "u1.ics": { "etag": "srv-1", "href": "/dav/cal/personal/u1.ics", "hash": "%s" }
  }
}`, contentHashOfUnsyncedICS()))

	// A second, synced resource that the sidecar does not mention at all — the
	// shape a truncated resource map leaves behind.
	other := strings.Replace(unsyncedICS, "UID:u1@test", "UID:u2@test", 1)
	if err := os.WriteFile(filepath.Join(dir, "u2.ics"), []byte(other), 0o644); err != nil {
		t.Fatal(err)
	}

	s, err := store.Open(ctx, root)
	if err != nil {
		t.Fatal(err)
	}
	if len(s.LoadErrors()) == 0 {
		t.Fatal("expected the sidecar parse failure recorded in LoadErrors")
	}
	cal, ok := s.Calendar("personal")
	if !ok {
		t.Fatal("personal calendar missing")
	}
	u1 := resourceByName(t, cal, "u1.ics")
	if u1.Dirty {
		t.Error("u1.ics Dirty=true, want false — its entry was recovered in full, so its state is known")
	}
	if u1.ETag != "srv-1" {
		t.Errorf("u1.ics ETag=%q, want srv-1 — a readable entry must survive a corrupt sibling field", u1.ETag)
	}
	if u2 := resourceByName(t, cal, "u2.ics"); !u2.Dirty {
		t.Error("u2.ics Dirty=false, want true — no recovered entry means unknown state, not clean")
	}
}
