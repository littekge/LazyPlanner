package model

import (
	"fmt"
	"time"

	"github.com/emersion/go-ical"
)

// Event is a calendar event parsed from a VEVENT.
//
// Raw is the source component, retained so that editing a known field can leave
// every other property untouched — the property-preservation iron rule. The
// typed fields are a read snapshot; until the editing layer lands they are only
// populated from Raw, so the two cannot drift.
type Event struct {
	UID     string
	Summary string
	// Start is inclusive. End is the exclusive end per iCal (DTEND), derived
	// from DURATION or, for an all-day item without DTEND, DTSTART plus one day.
	Start       time.Time
	End         time.Time
	AllDay      bool
	Location    string
	Description string
	HasAlarm    bool
	Recurring   bool

	Raw *ical.Component
}

// ParseEvent builds an Event from a VEVENT component, interpreting floating
// times in loc (nil defaults to time.Local). It returns an error only when the
// required DTSTART is missing or malformed; optional fields that fail to parse
// are left at their zero value so one bad property never discards the event.
func ParseEvent(comp *ical.Component, loc *time.Location) (*Event, error) {
	if loc == nil {
		loc = time.Local
	}

	uid := text(comp.Props, ical.PropUID)
	startProp := comp.Props.Get(ical.PropDateTimeStart)
	if startProp == nil {
		return nil, fmt.Errorf("VEVENT %q: missing DTSTART", uid)
	}
	start, err := resolveDateTime(startProp, loc)
	if err != nil {
		return nil, fmt.Errorf("VEVENT %q: parsing DTSTART: %w", uid, err)
	}

	ev := &Event{
		UID:         uid,
		Summary:     text(comp.Props, ical.PropSummary),
		Start:       start,
		AllDay:      isDateOnly(startProp),
		Location:    text(comp.Props, ical.PropLocation),
		Description: text(comp.Props, ical.PropDescription),
		HasAlarm:    hasAlarm(comp),
		Recurring:   isRecurring(comp),
		Raw:         comp,
	}

	// DTEND/DURATION are optional. Reuse go-ical's derivation, which handles
	// DTEND, DURATION, and the all-day one-day default. If that fails on an
	// explicit DTEND (e.g. an unloadable TZID), recover it the same way as
	// DTSTART. Leave End zero only if it truly can't be derived.
	if end, err := (&ical.Event{Component: comp}).DateTimeEnd(loc); err == nil {
		ev.End = end
	} else if endProp := comp.Props.Get(ical.PropDateTimeEnd); endProp != nil {
		if end, err := resolveDateTime(endProp, loc); err == nil {
			ev.End = end
		}
	}

	return ev, nil
}
