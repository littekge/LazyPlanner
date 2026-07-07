package model

import (
	"time"

	"github.com/emersion/go-ical"
)

// icalDateTimeLocal is the RFC 5545 "floating" date-time layout (no zone
// designator). It is used only on the recovery path below.
const icalDateTimeLocal = "20060102T150405"

// resolveDateTime parses an iCal date/date-time property into an absolute time,
// staying robust to time zones Go cannot load. It first defers to go-ical, which
// handles UTC, date-only, and IANA-TZID values. go-ical fails hard when a TZID
// is not an IANA zone (Outlook/Windows zone names like "Eastern Standard Time",
// or a custom VTIMEZONE label); rather than let that drop the whole item, this
// maps common Windows zone names to IANA and, failing that, interprets the value
// as floating time in loc. The item is thus never lost — at worst an unmapped
// exotic zone is off by its UTC offset until corrected.
func resolveDateTime(prop *ical.Prop, loc *time.Location) (time.Time, error) {
	if loc == nil {
		loc = time.Local
	}
	if t, err := prop.DateTime(loc); err == nil {
		return t, nil
	}

	// go-ical failed. If there is no TZID, the value itself is malformed — there
	// is nothing to recover, so report the original failure.
	tzid := prop.Params.Get(ical.ParamTimezoneID)
	if tzid == "" {
		_, err := prop.DateTime(loc)
		return time.Time{}, err
	}

	if iana := windowsToIANA(tzid); iana != "" {
		if z, err := time.LoadLocation(iana); err == nil {
			if t, err := time.ParseInLocation(icalDateTimeLocal, prop.Value, z); err == nil {
				return t, nil
			}
		}
	}

	// Last resort: keep the item by treating the wall-clock value as floating.
	if t, err := time.ParseInLocation(icalDateTimeLocal, prop.Value, loc); err == nil {
		return t, nil
	}

	_, err := prop.DateTime(loc)
	return time.Time{}, err
}
