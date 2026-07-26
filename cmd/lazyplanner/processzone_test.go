package main

import (
	"testing"
	"time"

	_ "time/tzdata"

	"github.com/littekge/LazyPlanner/internal/config"
	"github.com/littekge/LazyPlanner/internal/model"
)

// otherZoneThan returns a loaded zone that is definitely not want, so the test
// can establish a real divergence rather than assume one.
func otherZoneThan(t *testing.T, want *time.Location) *time.Location {
	t.Helper()
	for _, name := range []string{"Asia/Tokyo", "Pacific/Kiritimati", "America/Havana", "UTC"} {
		loc, err := time.LoadLocation(name)
		if err != nil {
			t.Fatalf("zone %q must load (time/tzdata is imported): %v", name, err)
		}
		if loc.String() != want.String() {
			return loc
		}
	}
	t.Fatal("no candidate zone differs from the resolved app zone")
	return nil
}

// run must install the app's own zone as the process time.Local before it
// dispatches anything.
//
// Go derives time.Local from $TZ alone; config.LocalZone() also reads
// /etc/timezone and /etc/localtime to recover a real IANA name. They diverge on
// TZ= set-but-empty, TZ=:Asia/Tokyo, TZ=/abs/path, and a stale-but-loadable
// /etc/timezone. Nine decode sites across internal/{store,sync,model} resolve
// floating and date-only values against time.Local while the UI renders against
// config.LocalZone(), so a divergence shows up as an all-day event on two days or
// "today" landing on the wrong day.
//
// The zone is process-global, so this test cannot run in parallel; it restores
// time.Local on the way out.
func TestRunInstallsAppZoneAsProcessLocal(t *testing.T) {
	saved := time.Local
	t.Cleanup(func() { time.Local = saved })

	want := config.LocalZone()
	time.Local = otherZoneThan(t, want)
	if time.Local.String() == want.String() {
		t.Fatal("test setup failed to create a divergence")
	}

	// A harmless dispatch path: it must still install the zone before returning.
	if code := run([]string{"version"}); code != 0 {
		t.Fatalf("run(version) = %d, want 0", code)
	}

	if time.Local.String() != want.String() {
		t.Errorf("time.Local = %s after run, want the app zone %s — store/sync/model "+
			"decode against time.Local and would disagree with the UI", time.Local, want)
	}
}

// The symptom the invariant above prevents: a floating (zone-less) DATE-TIME is
// resolved against time.Local, so if time.Local is not the zone the UI renders in,
// the item lands on the wrong day. This pins the mechanism, so a future change
// that reintroduces a second zone has something concrete to fail against.
func TestFloatingTimeFollowsTheProcessZone(t *testing.T) {
	saved := time.Local
	t.Cleanup(func() { time.Local = saved })

	tokyo, err := time.LoadLocation("Asia/Tokyo")
	if err != nil {
		t.Fatalf("zone must load: %v", err)
	}
	honolulu, err := time.LoadLocation("Pacific/Honolulu")
	if err != nil {
		t.Fatalf("zone must load: %v", err)
	}

	// 23:30 floating, late enough that a wrong zone moves it across midnight.
	ics := "BEGIN:VCALENDAR\r\nVERSION:2.0\r\nPRODID:-//t//t//EN\r\nBEGIN:VEVENT\r\n" +
		"UID:floating\r\nDTSTAMP:20260301T000000Z\r\nSUMMARY:Late\r\n" +
		"DTSTART:20260308T233000\r\nDTEND:20260309T000000\r\nEND:VEVENT\r\nEND:VCALENDAR\r\n"

	// model.Decode with a nil location is the shape the store's decode reduces to:
	// it resolves the floating value against time.Local.
	for _, tc := range []struct {
		zone *time.Location
		day  string
	}{{tokyo, "2026-03-08"}, {honolulu, "2026-03-08"}} {
		time.Local = tc.zone
		obj, err := model.Decode([]byte(ics), nil)
		if err != nil {
			t.Fatal(err)
		}
		if len(obj.Events) != 1 {
			t.Fatalf("want 1 event, got %d", len(obj.Events))
		}
		// Read in the SAME zone the decode used: the day must be the one written.
		if got := obj.Events[0].Start.In(tc.zone).Format("2006-01-02"); got != tc.day {
			t.Errorf("%s: floating DTSTART renders on %s, want %s", tc.zone, got, tc.day)
		}
		// And the resolved instant must actually carry that zone's offset, which is
		// what fails when the decode zone and the display zone are different objects.
		_, off := obj.Events[0].Start.Zone()
		_, want := time.Date(2026, 3, 8, 23, 30, 0, 0, tc.zone).Zone()
		if off != want {
			t.Errorf("%s: offset %d, want %d", tc.zone, off, want)
		}
	}
}
