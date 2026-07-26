package config

import (
	"os"
	"path/filepath"
	"strings"
	"time"
)

// zoneInfoRoots are the directories a resolved /etc/localtime symlink is
// expected to point inside; the IANA name is the path relative to one of them.
var zoneInfoRoots = []string{"/usr/share/zoneinfo/", "/usr/lib/zoneinfo/", "/etc/zoneinfo/"}

// timezoneFilePath and localtimeLinkPath are the well-known paths the two
// system sources live at on a real host; production always resolves through
// these, tests substitute their own via zoneNameFromPaths.
const (
	timezoneFilePath  = "/etc/timezone"
	localtimeLinkPath = "/etc/localtime"
)

// LocalZone returns the host's local time zone, preferring a location whose
// name is a real IANA identifier.
//
// Go names time.Local "Local" unless $TZ is set, and an anchor written as
// DTSTART;TZID=Local:... is meaningless to every other CalDAV client — so the
// name matters, not just the offset. Falling back to time.Local unnamed is safe:
// the write path keeps a UTC anchor when it cannot name the zone.
func LocalZone() *time.Location {
	if tz, set := os.LookupEnv("TZ"); set {
		if loc, ok := zoneFromTZ(tz); ok {
			return loc
		}
	}
	if name := zoneNameFromPaths(timezoneFilePath, localtimeLinkPath, zoneInfoRoots); name != "" {
		if loc, err := time.LoadLocation(name); err == nil {
			return loc
		}
	}
	return time.Local
}

// zoneFromTZ resolves a $TZ value the way Go's own runtime resolves time.Local,
// so the two can never disagree about an explicitly-set TZ.
//
// A plain time.LoadLocation(tz) is not enough: Go accepts three further forms it
// rejects, and each was a real divergence. TZ set to the empty string means UTC;
// a leading colon is POSIX's implementation-defined escape and is simply
// stripped; and an absolute path names a zoneinfo file directly. In all three,
// LoadLocation failed, LocalZone fell through to /etc/timezone, and the app then
// authored and rendered in a zone the rest of the process did not share.
//
// ok is false for a value naming no loadable zone (a typo like TZ=Not/AZone), so
// the caller falls through to the system files rather than returning a location
// whose name no other CalDAV client could resolve as a TZID. An absolute path
// outside every known zoneinfo root is deliberately treated the same way: Go
// would load it, but it yields no IANA name, and an unnameable zone breaks TZID
// authoring — the documented cost is that such an exotic TZ is not honoured.
func zoneFromTZ(tz string) (*time.Location, bool) {
	if tz == "" {
		return time.UTC, true // Go reads an empty TZ as UTC
	}
	tz = strings.TrimPrefix(tz, ":")
	if loc, err := time.LoadLocation(tz); err == nil {
		return loc, true
	}
	if name := nameFromZoneinfoPath(tz, zoneInfoRoots); name != "" {
		if loc, err := time.LoadLocation(name); err == nil {
			return loc, true
		}
	}
	return nil, false
}

// nameFromZoneinfoPath returns the IANA identifier a zoneinfo file path encodes
// — the path relative to whichever known root contains it — or "" when it lies
// outside all of them. Shared by the $TZ path form and the /etc/localtime symlink
// so the two cannot strip prefixes differently.
func nameFromZoneinfoPath(path string, roots []string) string {
	p := filepath.ToSlash(path)
	for _, root := range roots {
		if strings.HasPrefix(p, root) {
			return strings.TrimPrefix(p, root)
		}
	}
	return ""
}

// zoneNameFromPaths reads the IANA name Debian/Ubuntu record at tzFile, or
// derives it from the symlink target at localtimeLink used by most other
// distros, trying the sources in that order and taking the paths as
// parameters so tests can supply deterministic fixtures instead of the real
// system paths.
//
// A tzFile candidate is used only if it actually loads via time.LoadLocation:
// a stale /etc/timezone left behind after /etc/localtime was repointed (e.g.
// in a container image) must not shadow a good name the symlink would have
// yielded — it must fall through to the symlink instead.
func zoneNameFromPaths(tzFile, localtimeLink string, roots []string) string {
	if b, err := os.ReadFile(tzFile); err == nil {
		if name := strings.TrimSpace(string(b)); name != "" {
			if _, err := time.LoadLocation(name); err == nil {
				return name
			}
		}
	}
	target, err := filepath.EvalSymlinks(localtimeLink)
	if err != nil {
		return ""
	}
	return nameFromZoneinfoPath(target, roots)
}
