package model

import (
	"bytes"
	"fmt"
	"strings"
	"time"

	"github.com/emersion/go-ical"
)

// Parsed is the result of decoding one iCalendar object. It exposes the typed
// events and todos alongside the underlying calendar, which is retained
// verbatim so unknown properties survive a later re-encode — the
// property-preservation iron rule (see main.md). Each Event.Raw / Todo.Raw
// points into Calendar.Children, so edits made through those components are
// reflected when the calendar is re-encoded.
type Parsed struct {
	Calendar *ical.Calendar
	Events   []*Event
	Todos    []*Todo
}

// Encode serializes the underlying calendar back to iCalendar bytes. Because
// Decode retained the whole calendar, this round-trips every property the model
// does not itself understand — the property-preservation iron rule.
func (p *Parsed) Encode() ([]byte, error) {
	var buf bytes.Buffer
	if err := ical.NewEncoder(&buf).Encode(p.Calendar); err != nil {
		return nil, fmt.Errorf("encoding icalendar: %w", err)
	}
	return buf.Bytes(), nil
}

// Decode parses an iCalendar stream (a single CalDAV resource may hold several
// components) into typed events and todos. Floating times — those with neither
// a TZID parameter nor a UTC marker — are interpreted in loc; callers pass
// time.Local so items display in the system timezone. A nil loc defaults to
// time.Local.
//
// Decode fails on the first component with a malformed required field. Callers
// that need per-item resilience (skip the bad one, keep the rest) should
// iterate the calendar's children and call ParseEvent / ParseTodo directly.
func Decode(data []byte, loc *time.Location) (*Parsed, error) {
	cal, err := ical.NewDecoder(bytes.NewReader(data)).Decode()
	if err != nil {
		return nil, fmt.Errorf("decoding icalendar: %w", err)
	}
	return Parse(cal, loc)
}

// Parse builds typed events and todos from an already-decoded calendar,
// retaining it for re-encode. It is the shared core of Decode and is also used
// directly by the sync layer, which receives decoded calendars from the CalDAV
// client. loc and the error semantics match Decode; a nil loc defaults to
// time.Local.
func Parse(cal *ical.Calendar, loc *time.Location) (*Parsed, error) {
	if loc == nil {
		loc = time.Local
	}
	p := &Parsed{Calendar: cal}
	for _, child := range cal.Children {
		switch child.Name {
		case ical.CompEvent:
			ev, err := ParseEvent(child, loc)
			if err != nil {
				return nil, err
			}
			p.Events = append(p.Events, ev)
		case ical.CompToDo:
			td, err := ParseTodo(child, loc)
			if err != nil {
				return nil, err
			}
			p.Todos = append(p.Todos, td)
		}
	}
	return p, nil
}

// text returns the unescaped text value of the named property, or "" when the
// property is absent. A property whose escaping is malformed falls back to its
// raw value rather than being dropped.
func text(props ical.Props, name string) string {
	prop := props.Get(name)
	if prop == nil {
		return ""
	}
	if s, err := prop.Text(); err == nil {
		return s
	}
	return prop.Value
}

// isDateOnly reports whether prop holds a date with no time of day — an all-day
// value. Well-formed data carries VALUE=DATE; as a fallback a bare YYYYMMDD
// value with no explicit type is also treated as date-only.
func isDateOnly(prop *ical.Prop) bool {
	if prop == nil {
		return false
	}
	if prop.ValueType() == ical.ValueDate {
		return true
	}
	return prop.ValueType() == ical.ValueDefault && len(prop.Value) == len("20060102")
}

// hasAlarm reports whether comp contains a VALARM child. LazyPlanner surfaces
// that reminders exist but never fires them — the phone and NextCloud do that.
func hasAlarm(comp *ical.Component) bool {
	for _, child := range comp.Children {
		if child.Name == ical.CompAlarm {
			return true
		}
	}
	return false
}

// isRecurring reports whether comp defines a recurrence rule. Occurrence
// expansion is handled separately by the recurrence layer.
func isRecurring(comp *ical.Component) bool {
	return comp.Props.Get(ical.PropRecurrenceRule) != nil
}

// parentUID returns the UID of the item comp is nested under, via RELATED-TO.
// Per RFC 5545 the default relationship type is PARENT, so a RELATED-TO with no
// RELTYPE — or RELTYPE=PARENT — identifies the parent. An empty result means a
// root item. This is the mechanism behind the subtask tree.
func parentUID(comp *ical.Component) string {
	for _, prop := range comp.Props.Values(ical.PropRelatedTo) {
		reltype := prop.Params.Get(ical.ParamRelationshipType)
		if reltype == "" || strings.EqualFold(reltype, "PARENT") {
			return prop.Value
		}
	}
	return ""
}
