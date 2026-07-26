package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLocalZonePrefersTZEnv(t *testing.T) {
	t.Setenv("TZ", "America/New_York")
	if got := LocalZone().String(); got != "America/New_York" {
		t.Errorf("LocalZone() = %q, want America/New_York", got)
	}
}

// With TZ unset the resolver falls back to the system zone files. On a host
// with neither, it must still return a usable location rather than nil.
func TestLocalZoneWithoutTZEnvIsUsable(t *testing.T) {
	t.Setenv("TZ", "")
	os.Unsetenv("TZ")
	loc := LocalZone()
	if loc == nil {
		t.Fatal("LocalZone() = nil")
	}
	// Whatever it resolves to must round-trip: a name we cannot load is worse
	// than no name at all, because the write path would emit an unresolvable TZID.
	if name := loc.String(); name != "Local" && name != "UTC" {
		if _, err := time.LoadLocation(name); err != nil {
			t.Errorf("LocalZone() = %q, which does not load: %v", name, err)
		}
	}
}

func TestLocalZoneIgnoresUnloadableTZ(t *testing.T) {
	t.Setenv("TZ", "Not/AZone")
	if loc := LocalZone(); loc == nil {
		t.Fatal("LocalZone() = nil for a bogus TZ")
	}
}

// LocalZone must resolve $TZ in every form Go's own runtime accepts for
// time.Local, or the two disagree and the app authors/renders in a zone the
// store, sync engine and model do not share (an all-day event on two days,
// floating times an offset out, "today" on the wrong day).
//
// Each form below was a confirmed divergence: LoadLocation rejected the value,
// LocalZone fell through to /etc/timezone, and time.Local kept Go's answer.
func TestLocalZoneHonoursEveryTZForm(t *testing.T) {
	tokyoPath := ""
	for _, root := range zoneInfoRoots {
		if _, err := os.Stat(filepath.Join(root, "Asia", "Tokyo")); err == nil {
			tokyoPath = filepath.Join(root, "Asia", "Tokyo")
			break
		}
	}

	tests := []struct {
		name, tz, want string
		needsPath      bool
	}{
		{name: "plain IANA name", tz: "Asia/Tokyo", want: "Asia/Tokyo"},
		// Go reads an empty (but set) TZ as UTC; LocalZone used to skip it entirely
		// because it tested tz != "" rather than whether TZ was set.
		{name: "empty TZ means UTC", tz: "", want: "UTC"},
		// POSIX's implementation-defined escape. LoadLocation rejects the colon.
		{name: "leading colon is stripped", tz: ":Asia/Tokyo", want: "Asia/Tokyo"},
		// An absolute zoneinfo path: Go loads it, LoadLocation does not.
		{name: "absolute zoneinfo path", tz: tokyoPath, want: "Asia/Tokyo", needsPath: true},
		{name: "colon plus absolute path", tz: ":" + tokyoPath, want: "Asia/Tokyo", needsPath: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.needsPath && tokyoPath == "" {
				// Not a skip of the property under test: the host simply ships no
				// zoneinfo tree to build an absolute-path fixture from. The colon and
				// empty-TZ cases above still run and cover the same code path.
				t.Skip("no on-disk zoneinfo root to build an absolute-path TZ from")
			}
			t.Setenv("TZ", tt.tz)
			if got := LocalZone().String(); got != tt.want {
				t.Errorf("LocalZone() with TZ=%q = %q, want %q", tt.tz, got, tt.want)
			}
		})
	}
}

// A TZ naming no loadable zone must fall through to the system files rather than
// produce a location whose name no CalDAV client could resolve as a TZID.
func TestLocalZoneFallsThroughOnUnresolvableTZ(t *testing.T) {
	for _, tz := range []string{"Not/AZone", ":Not/AZone", "/no/such/zoneinfo/Zone"} {
		t.Setenv("TZ", tz)
		loc := LocalZone()
		if loc == nil {
			t.Fatalf("LocalZone() = nil for TZ=%q", tz)
		}
		if name := loc.String(); name != "Local" && name != "UTC" {
			if _, err := time.LoadLocation(name); err != nil {
				t.Errorf("TZ=%q gave unloadable zone %q: %v", tz, name, err)
			}
		}
	}
}

// TestZoneNameFromPaths drives the unexported seam directly with t.TempDir()
// fixtures so precedence and prefix-stripping are deterministic on any host
// and in CI — no dependency on the real /etc/timezone or /etc/localtime, and
// no need for the fixture's derived name to exist in the host's tzdata.
func TestZoneNameFromPaths(t *testing.T) {
	tests := []struct {
		name  string
		setup func(t *testing.T) (tzFile, localtimeLink string, roots []string)
		want  string
	}{
		{
			name: "loadable /etc/timezone wins",
			setup: func(t *testing.T) (string, string, []string) {
				dir := t.TempDir()
				tzFile := filepath.Join(dir, "timezone")
				if err := os.WriteFile(tzFile, []byte("UTC\n"), 0o644); err != nil {
					t.Fatal(err)
				}
				return tzFile, filepath.Join(dir, "does-not-exist"), zoneInfoRoots
			},
			want: "UTC",
		},
		{
			// B1 regression guard: a stale /etc/timezone left behind after
			// /etc/localtime was repointed must not shadow the symlink's name.
			name: "garbage /etc/timezone falls through to a good symlink",
			setup: func(t *testing.T) (string, string, []string) {
				dir := t.TempDir()
				tzFile := filepath.Join(dir, "timezone")
				if err := os.WriteFile(tzFile, []byte("Not/AZone"), 0o644); err != nil {
					t.Fatal(err)
				}
				root, link := fakeZoneinfoSymlink(t, dir)
				return tzFile, link, []string{root}
			},
			want: "America/New_York",
		},
		{
			name: "no timezone file, symlink into a zoneinfo root",
			setup: func(t *testing.T) (string, string, []string) {
				dir := t.TempDir()
				root, link := fakeZoneinfoSymlink(t, dir)
				return filepath.Join(dir, "does-not-exist"), link, []string{root}
			},
			want: "America/New_York",
		},
		{
			name: "symlink outside every known root yields no name",
			setup: func(t *testing.T) (string, string, []string) {
				dir := t.TempDir()
				_, link := fakeZoneinfoSymlink(t, dir)
				return filepath.Join(dir, "does-not-exist"), link, []string{filepath.Join(dir, "other-root") + "/"}
			},
			want: "",
		},
		{
			name: "neither source available yields no name",
			setup: func(t *testing.T) (string, string, []string) {
				dir := t.TempDir()
				return filepath.Join(dir, "no-timezone"), filepath.Join(dir, "no-localtime"), zoneInfoRoots
			},
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tzFile, localtimeLink, roots := tt.setup(t)
			if got := zoneNameFromPaths(tzFile, localtimeLink, roots); got != tt.want {
				t.Errorf("zoneNameFromPaths() = %q, want %q", got, tt.want)
			}
		})
	}
}

// fakeZoneinfoSymlink builds <dir>/zoneinfo/America/New_York and a symlink at
// <dir>/localtime pointing to it, returning the zoneinfo root (with the
// trailing slash zoneInfoRoots' entries use) and the symlink path.
func fakeZoneinfoSymlink(t *testing.T, dir string) (root, link string) {
	t.Helper()
	root = filepath.Join(dir, "zoneinfo") + string(filepath.Separator)
	target := filepath.Join(root, "America", "New_York")
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	link = filepath.Join(dir, "localtime")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	return root, link
}
