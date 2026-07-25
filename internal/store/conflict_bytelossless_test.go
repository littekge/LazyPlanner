package store_test

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/littekge/LazyPlanner/internal/store"
)

// The conflict stash is the ONLY copy of the server's diverging version, so it
// must survive the sidecar round-trip byte for byte (iron rule). Both sides of
// the class are covered: non-UTF-8 bytes, ordinary UTF-8, and a legacy sidecar
// written before the base64 field existed.

// latin1ServerICS is a server version whose SUMMARY carries a raw Latin-1 byte
// (0xE9, "é") — i.e. NOT valid UTF-8. Real CalDAV servers hand back whatever the
// producing client wrote; the stash must be byte-lossless.
func latin1ServerICS() []byte {
	return []byte("BEGIN:VCALENDAR\r\nVERSION:2.0\r\nPRODID:-//Other//Client//EN\r\n" +
		"BEGIN:VEVENT\r\nUID:e@test\r\nDTSTAMP:20260701T120000Z\r\n" +
		"DTSTART:20260704T130000Z\r\nDTEND:20260704T133000Z\r\n" +
		"SUMMARY:caf\xe9 meeting\r\nEND:VEVENT\r\nEND:VCALENDAR\r\n")
}

// utf8ServerICS is the same object in valid UTF-8 (plus an emoji), to prove the
// encoding change did not break the ordinary case.
func utf8ServerICS() []byte {
	return []byte("BEGIN:VCALENDAR\r\nVERSION:2.0\r\nPRODID:-//Other//Client//EN\r\n" +
		"BEGIN:VEVENT\r\nUID:e@test\r\nDTSTAMP:20260701T120000Z\r\n" +
		"DTSTART:20260704T130000Z\r\nDTEND:20260704T133000Z\r\n" +
		"SUMMARY:café meeting ☕\r\nEND:VEVENT\r\nEND:VCALENDAR\r\n")
}

// TestConflictStashIsByteLossless: MarkConflict promises to stash the server's
// diverging version "losslessly". Serialized as a plain JSON string it wasn't —
// encoding/json replaces every invalid UTF-8 byte with U+FFFD, so on reload the
// stash was corrupted and ResolveKeepServer wrote that mojibake as the local
// working copy.
func TestConflictStashIsByteLossless(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	name := seedSyncedResource(t, dir, "cal1", "e@test", "Base")

	s, err := store.Open(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Put(ctx, "cal1", name, mustDecode(t, "e@test", "Local edit")); err != nil {
		t.Fatal(err)
	}
	want := latin1ServerICS()
	if err := s.MarkConflict(ctx, "cal1", name, want, "srv-2", false); err != nil {
		t.Fatal(err)
	}

	// Reload from disk — the sidecar is the only place the stash lives.
	s2, err := store.Open(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}
	cs := s2.Conflicts()
	if len(cs) != 1 {
		t.Fatalf("conflicts after reload = %d, want 1", len(cs))
	}
	got := cs[0].ServerData
	if !bytes.Equal(got, want) {
		t.Errorf("stashed server data corrupted by the sidecar round-trip:\n"+
			" want %d bytes: %q\n  got %d bytes: %q", len(want), want, len(got), got)
	}

	// And the corruption would be durable: keep-server writes it as the local copy.
	if err := s2.ResolveKeepServer(ctx, "cal1", name); err != nil {
		t.Fatalf("ResolveKeepServer: %v", err)
	}
	local, err := os.ReadFile(filepath.Join(dir, "calendars", "cal1", name))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(local, []byte("caf\xe9 meeting")) {
		t.Errorf("local .ics after keep-server lost the server's bytes; SUMMARY line = %q",
			summaryLineOf(local))
	}
}

// TestConflictStashRoundTripsUTF8 is the other side of the class: the base64
// encoding must not disturb the ordinary valid-UTF-8 server version.
func TestConflictStashRoundTripsUTF8(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	name := seedSyncedResource(t, dir, "cal1", "e@test", "Base")

	s, err := store.Open(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}
	want := utf8ServerICS()
	if err := s.MarkConflict(ctx, "cal1", name, want, "srv-2", false); err != nil {
		t.Fatal(err)
	}
	s2, err := store.Open(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}
	cs := s2.Conflicts()
	if len(cs) != 1 {
		t.Fatalf("conflicts after reload = %d, want 1", len(cs))
	}
	if !bytes.Equal(cs[0].ServerData, want) {
		t.Errorf("valid-UTF-8 stash not round-tripped:\n want %q\n  got %q", want, cs[0].ServerData)
	}
	if cs[0].ServerETag != "srv-2" {
		t.Errorf("ServerETag = %q, want srv-2", cs[0].ServerETag)
	}
}

// TestConflictStashReadsLegacyPlainSidecar: sidecars already in users' caches
// carry the server version in the pre-base64 "server_data" string field. The
// upgrade must keep reading them, or an existing unresolved conflict loses the
// server's side entirely (keep-server would then refuse, with nothing to show).
func TestConflictStashReadsLegacyPlainSidecar(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	name := seedSyncedResource(t, dir, "cal1", "e@test", "Base")

	legacyServer := "BEGIN:VCALENDAR\r\nVERSION:2.0\r\nPRODID:-//Legacy//EN\r\n" +
		"BEGIN:VEVENT\r\nUID:e@test\r\nDTSTAMP:20260701T120000Z\r\n" +
		"DTSTART:20260704T130000Z\r\nDTEND:20260704T133000Z\r\n" +
		"SUMMARY:Legacy server edit\r\nEND:VEVENT\r\nEND:VCALENDAR\r\n"
	legacySidecar := map[string]any{
		"resources": map[string]any{
			name: map[string]any{
				"etag":  `"srv-1"`,
				"href":  "/dav/cal1/" + name,
				"dirty": true,
				"conflict": map[string]any{
					"server_etag": `"srv-2"`,
					"server_data": legacyServer,
				},
			},
		},
	}
	raw, err := json.Marshal(legacySidecar)
	if err != nil {
		t.Fatal(err)
	}
	scPath := filepath.Join(dir, "calendars", "cal1", ".lazyplanner.json")
	if err := os.WriteFile(scPath, raw, 0o600); err != nil {
		t.Fatal(err)
	}

	s, err := store.Open(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}
	cs := s.Conflicts()
	if len(cs) != 1 {
		t.Fatalf("legacy sidecar: conflicts = %d, want 1", len(cs))
	}
	if string(cs[0].ServerData) != legacyServer {
		t.Errorf("legacy stash not read back:\n want %q\n  got %q", legacyServer, cs[0].ServerData)
	}
	if cs[0].ServerETag != `"srv-2"` {
		t.Errorf("legacy ServerETag = %q", cs[0].ServerETag)
	}

	// And it is rewritten in the new byte-exact form, still readable afterwards.
	if err := s.MarkConflict(ctx, "cal1", name, []byte(legacyServer), `"srv-2"`, false); err != nil {
		t.Fatal(err)
	}
	after, err := os.ReadFile(scPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(after, []byte("server_data_b64")) {
		t.Error("rewritten sidecar did not adopt the byte-exact base64 field")
	}
	s2, err := store.Open(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}
	if cs2 := s2.Conflicts(); len(cs2) != 1 || string(cs2[0].ServerData) != legacyServer {
		t.Errorf("migrated stash lost on the next reload: %+v", cs2)
	}
}

func summaryLineOf(b []byte) string {
	for _, line := range bytes.Split(b, []byte("\r\n")) {
		if bytes.HasPrefix(line, []byte("SUMMARY")) {
			return string(line)
		}
	}
	return "<no SUMMARY>"
}
