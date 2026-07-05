package store_test

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/littekge/LazyPlanner/internal/model"
	"github.com/littekge/LazyPlanner/internal/store"
)

func mustDecode(t *testing.T, uid, summary string) *model.Parsed {
	t.Helper()
	ics := "BEGIN:VCALENDAR\r\nVERSION:2.0\r\nPRODID:-//LazyPlanner//Test//EN\r\n" +
		"BEGIN:VEVENT\r\nUID:" + uid + "\r\nDTSTAMP:20260701T120000Z\r\n" +
		"DTSTART:20260704T130000Z\r\nDTEND:20260704T133000Z\r\n" +
		"SUMMARY:" + summary + "\r\nX-CUSTOM:preserve-me\r\nEND:VEVENT\r\nEND:VCALENDAR\r\n"
	obj, err := model.Decode([]byte(ics), time.UTC)
	if err != nil {
		t.Fatalf("decoding test object: %v", err)
	}
	return obj
}

func findResource(cal store.Calendar, name string) *store.Resource {
	for _, r := range cal.Resources {
		if r.Name == name {
			return r
		}
	}
	return nil
}

func TestOpenLoadsVdirTree(t *testing.T) {
	s, err := store.Open(context.Background(), "testdata/vdir")
	if err != nil {
		t.Fatal(err)
	}

	cals := s.Calendars()
	if len(cals) != 2 {
		t.Fatalf("got %d calendars, want 2 (%v)", len(cals), cals)
	}
	// Sorted by ID: personal, work.
	personal, work := cals[0], cals[1]

	if personal.ID != "personal" || personal.DisplayName != "Personal" {
		t.Errorf("personal metadata = %q/%q", personal.ID, personal.DisplayName)
	}
	if personal.Color != "#3366cc" || personal.SyncToken != "http://sabre.io/ns/sync/42" {
		t.Errorf("personal color/token = %q/%q", personal.Color, personal.SyncToken)
	}
	// broken.ics is skipped, so 2 valid resources remain.
	if len(personal.Resources) != 2 {
		t.Fatalf("personal has %d resources, want 2 (%v)", len(personal.Resources), personal.Resources)
	}

	if standup := findResource(personal, "standup.ics"); standup == nil {
		t.Error("standup.ics missing")
	} else if standup.ETag != `"etag-standup-1"` || standup.Dirty {
		t.Errorf("standup ETag=%q Dirty=%v, want tracked etag and not dirty", standup.ETag, standup.Dirty)
	}
	// grocery.ics exists on disk but not in the sidecar: tracked as untracked.
	if grocery := findResource(personal, "grocery.ics"); grocery == nil {
		t.Error("grocery.ics missing")
	} else if grocery.ETag != "" {
		t.Errorf("grocery ETag=%q, want empty (untracked)", grocery.ETag)
	}

	// work has no sidecar: DisplayName falls back to the id.
	if work.ID != "work" || work.DisplayName != "work" {
		t.Errorf("work metadata = %q/%q, want id fallback", work.ID, work.DisplayName)
	}
	if len(work.Resources) != 1 {
		t.Errorf("work has %d resources, want 1", len(work.Resources))
	}

	// broken.ics is surfaced as a load error, not a silent drop.
	le := s.LoadErrors()
	if len(le) != 1 || le[0].Calendar != "personal" || le[0].Name != "broken.ics" {
		t.Errorf("LoadErrors = %v, want one for personal/broken.ics", le)
	}
}

func TestQueries(t *testing.T) {
	s, err := store.Open(context.Background(), "testdata/vdir")
	if err != nil {
		t.Fatal(err)
	}

	todos := s.Todos()
	if len(todos) != 1 || todos[0].UID != "grocery@lazyplanner.test" {
		t.Fatalf("Todos() = %v, want the grocery todo", todos)
	}

	from := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, 7, 10, 0, 0, 0, 0, time.UTC)
	occs, err := s.EventOccurrences(from, to)
	if err != nil {
		t.Fatal(err)
	}
	if len(occs) != 2 {
		t.Fatalf("got %d occurrences, want 2 (standup + meeting)", len(occs))
	}
	// Sorted by start: standup (7/4) before meeting (7/5).
	if occs[0].Event.UID != "standup@lazyplanner.test" || occs[1].Event.UID != "meeting@lazyplanner.test" {
		t.Errorf("occurrence order = %q, %q", occs[0].Event.UID, occs[1].Event.UID)
	}
}

func TestOpenMissingDirIsEmpty(t *testing.T) {
	s, err := store.Open(context.Background(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if len(s.Calendars()) != 0 {
		t.Errorf("expected empty store, got %d calendars", len(s.Calendars()))
	}
	if len(s.LoadErrors()) != 0 {
		t.Errorf("expected no load errors, got %v", s.LoadErrors())
	}
}

func TestPutAndReload(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()

	s, err := store.Open(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}

	obj := mustDecode(t, "new-event@lazyplanner.test", "Fresh event")
	name := store.ResourceName("new-event@lazyplanner.test")

	res, err := s.Put(ctx, "cal1", name, obj)
	if err != nil {
		t.Fatal(err)
	}
	if !res.Dirty || res.ETag != "" {
		t.Errorf("new resource Dirty=%v ETag=%q, want dirty and no etag", res.Dirty, res.ETag)
	}

	// The .ics file exists on disk under the expected path.
	path := filepath.Join(dir, "calendars", "cal1", name)
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected written file at %s: %v", path, err)
	}

	// No temp files were left behind.
	entries, _ := os.ReadDir(filepath.Dir(path))
	for _, e := range entries {
		if filepath.Ext(e.Name()) != ".ics" && e.Name() != ".lazyplanner.json" {
			t.Errorf("unexpected leftover file: %s", e.Name())
		}
	}

	// Reopening from disk reproduces the resource and its dirty state.
	s2, err := store.Open(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}
	cal, ok := s2.Calendar("cal1")
	if !ok {
		t.Fatal("cal1 missing after reload")
	}
	got := findResource(cal, name)
	if got == nil {
		t.Fatalf("resource %s missing after reload", name)
	}
	if !got.Dirty {
		t.Error("Dirty flag not persisted across reload")
	}
	if len(got.Object.Events) != 1 || got.Object.Events[0].Summary != "Fresh event" {
		t.Errorf("reloaded object mismatch: %+v", got.Object.Events)
	}

	// Property preservation: the unknown X- property survived the write.
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(raw, []byte("X-CUSTOM:preserve-me")) {
		t.Errorf("X-CUSTOM property was not preserved on write:\n%s", raw)
	}
}

// TestPutPreservesServerIdentity verifies that overwriting a synced resource
// keeps its ETag/Href (so sync can detect the local edit) while marking it dirty.
func TestPutPreservesServerIdentity(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()

	// Seed a calendar with one synced resource and a sidecar recording its etag.
	calDir := filepath.Join(dir, "calendars", "cal1")
	if err := os.MkdirAll(calDir, 0o700); err != nil {
		t.Fatal(err)
	}
	seed := mustDecode(t, "seed@lazyplanner.test", "Original")
	seedBytes, _ := seed.Encode()
	name := store.ResourceName("seed@lazyplanner.test")
	if err := os.WriteFile(filepath.Join(calDir, name), seedBytes, 0o600); err != nil {
		t.Fatal(err)
	}
	sidecar := `{"resources":{"` + name + `":{"etag":"\"srv-1\"","href":"/dav/cal1/` + name + `"}}}`
	if err := os.WriteFile(filepath.Join(calDir, ".lazyplanner.json"), []byte(sidecar), 0o600); err != nil {
		t.Fatal(err)
	}

	s, err := store.Open(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}

	// Overwrite it with an edited version.
	edited := mustDecode(t, "seed@lazyplanner.test", "Edited title")
	res, err := s.Put(ctx, "cal1", name, edited)
	if err != nil {
		t.Fatal(err)
	}
	if res.ETag != `"srv-1"` || res.Href != "/dav/cal1/"+name {
		t.Errorf("server identity lost on overwrite: ETag=%q Href=%q", res.ETag, res.Href)
	}
	if !res.Dirty {
		t.Error("overwritten resource should be marked dirty")
	}
}

func TestDelete(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	s, err := store.Open(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}

	obj := mustDecode(t, "todelete@lazyplanner.test", "Doomed")
	name := store.ResourceName("todelete@lazyplanner.test")
	if _, err := s.Put(ctx, "cal1", name, obj); err != nil {
		t.Fatal(err)
	}

	if err := s.Delete(ctx, "cal1", name); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "calendars", "cal1", name)); !os.IsNotExist(err) {
		t.Errorf("file still present after delete: %v", err)
	}
	cal, _ := s.Calendar("cal1")
	if findResource(cal, name) != nil {
		t.Error("resource still in index after delete")
	}

	// Deleting an unknown resource is an error.
	if err := s.Delete(ctx, "cal1", "nope.ics"); err == nil {
		t.Error("expected error deleting unknown resource")
	}

	// The deletion survives a reload.
	s2, _ := store.Open(ctx, dir)
	cal2, _ := s2.Calendar("cal1")
	if findResource(cal2, name) != nil {
		t.Error("deleted resource reappeared after reload")
	}
}

func TestLocate(t *testing.T) {
	ctx := context.Background()
	s, err := store.Open(ctx, "testdata/vdir")
	if err != nil {
		t.Fatal(err)
	}

	got, ok := s.Locate("grocery@lazyplanner.test")
	if !ok {
		t.Fatal("grocery todo not located")
	}
	if got.CalID != "personal" || got.Object == nil || got.Prev == nil {
		t.Errorf("Locate returned %+v", got)
	}

	if _, ok := s.Locate("does-not-exist@example.com"); ok {
		t.Error("Locate found a nonexistent UID")
	}
}

// TestRestoreUndoesEdit exercises the undo primitive: capture a snapshot, edit,
// then Restore returns the resource to the captured state exactly.
func TestRestoreUndoesEdit(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	s, err := store.Open(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}

	name := store.ResourceName("undo@lazyplanner.test")
	orig := mustDecode(t, "undo@lazyplanner.test", "Original")
	if _, err := s.Put(ctx, "cal1", name, orig); err != nil {
		t.Fatal(err)
	}

	// Capture the pre-edit snapshot, then overwrite with an edit.
	before, ok := s.Locate("undo@lazyplanner.test")
	if !ok {
		t.Fatal("resource not located")
	}
	snapshot := before.Prev

	edited := mustDecode(t, "undo@lazyplanner.test", "Edited")
	if _, err := s.Put(ctx, "cal1", name, edited); err != nil {
		t.Fatal(err)
	}

	// Undo by restoring the captured snapshot.
	if _, err := s.Restore(ctx, "cal1", name, snapshot); err != nil {
		t.Fatal(err)
	}
	cal, _ := s.Calendar("cal1")
	got := findResource(cal, name)
	if got == nil || len(got.Object.Events) != 1 || got.Object.Events[0].Summary != "Original" {
		t.Errorf("Restore did not bring back the original: %+v", got)
	}

	// The restored content is on disk too.
	raw, _ := os.ReadFile(filepath.Join(dir, "calendars", "cal1", name))
	if !bytes.Contains(raw, []byte("SUMMARY:Original")) {
		t.Errorf("restored file has wrong content:\n%s", raw)
	}
}

func TestPutCancelledContext(t *testing.T) {
	dir := t.TempDir()
	s, err := store.Open(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	obj := mustDecode(t, "x@lazyplanner.test", "Nope")
	if _, err := s.Put(ctx, "cal1", store.ResourceName("x@lazyplanner.test"), obj); err == nil {
		t.Error("expected Put to fail with a cancelled context")
	}
	if _, err := os.Stat(filepath.Join(dir, "calendars", "cal1")); !os.IsNotExist(err) {
		t.Error("Put wrote to disk despite cancelled context")
	}
}
