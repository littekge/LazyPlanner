package config

import (
	"os"
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
