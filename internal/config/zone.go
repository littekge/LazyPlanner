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
	if name := zoneNameFromEtc(); name != "" {
		if loc, err := time.LoadLocation(name); err == nil {
			return loc
		}
	}
	return time.Local
}

// zoneNameFromEtc reads the IANA name Debian/Ubuntu record in /etc/timezone, or
// derives it from the /etc/localtime symlink target used by most other distros.
func zoneNameFromEtc() string {
	if b, err := os.ReadFile("/etc/timezone"); err == nil {
		if name := strings.TrimSpace(string(b)); name != "" {
			return name
		}
	}
	target, err := filepath.EvalSymlinks("/etc/localtime")
	if err != nil {
		return ""
	}
	target = filepath.ToSlash(target)
	for _, root := range zoneInfoRoots {
		if strings.HasPrefix(target, root) {
			return strings.TrimPrefix(target, root)
		}
	}
	return ""
}
