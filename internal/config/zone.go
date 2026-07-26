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
	if tz := os.Getenv("TZ"); tz != "" {
		if loc, err := time.LoadLocation(tz); err == nil {
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
	target = filepath.ToSlash(target)
	for _, root := range roots {
		if strings.HasPrefix(target, root) {
			return strings.TrimPrefix(target, root)
		}
	}
	return ""
}
