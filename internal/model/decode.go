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
	cal, err := decodeCalendar(data)
	if err != nil {
		return nil, err
	}
	return Parse(cal, loc)
}

// decodeCalendar runs go-ical's decoder, containing any panic it raises and
// returning it as an ordinary error. go-ical's line decoder indexes past the end
// of the buffer on some malformed inputs (e.g. a content line that ends
// mid-parameter, "PROP;X="), panicking rather than erroring. LazyPlanner must
// never crash on a bad .ics from disk or a hostile/buggy server response (the
// iron rule and the error-handling standard), and vendored code must not be
// hand-edited — so the panic is contained at this boundary and surfaced as the
// normal decode error that every caller already skips-and-continues on.
func decodeCalendar(data []byte) (cal *ical.Calendar, err error) {
	defer func() {
		if r := recover(); r != nil {
			cal, err = nil, fmt.Errorf("decoding icalendar: malformed data (%v)", r)
		}
	}()
	cal, err = ical.NewDecoder(bytes.NewReader(data)).Decode()
	if err != nil {
		return nil, fmt.Errorf("decoding icalendar: %w", err)
	}
	return cal, nil
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
	dedupeSingleValued(cal)
	stripForbiddenNesting(cal)
	dropEmptyTimezones(cal) // after strip, which can leave a VTIMEZONE childless
	healComponentConstraints(cal)
	ensureCalendarProps(cal)
	sanitizePropValues(cal)
	p := &Parsed{Calendar: cal}
	for _, child := range cal.Children {
		switch child.Name {
		case ical.CompEvent:
			ensureDTStamp(child)
			ev, err := ParseEvent(child, loc)
			if err != nil {
				return nil, err
			}
			p.Events = append(p.Events, ev)
		case ical.CompToDo:
			ensureDTStamp(child)
			td, err := ParseTodo(child, loc)
			if err != nil {
				return nil, err
			}
			p.Todos = append(p.Todos, td)
		}
	}
	return p, nil
}

// singleValuedProps mirrors go-ical's encoder cardinality checks (encoder.go
// validateComponent): per component type, the properties it requires to appear
// exactly once or at most once. A malformed object carrying duplicates decodes
// but fails to encode ("want exactly one/at most one … got N"), so the whole
// resource — not just the bad item — becomes unwritable. Only the component
// types LazyPlanner emits are listed; if go-ical's rules change on a dependency
// bump, re-check this table.
var singleValuedProps = map[string][]string{
	ical.CompCalendar: {
		ical.PropProductID, ical.PropVersion, ical.PropCalendarScale, ical.PropMethod,
		ical.PropUID, ical.PropLastModified, ical.PropURL, ical.PropRefreshInterval,
		ical.PropSource, ical.PropColor,
	},
	ical.CompEvent: {
		ical.PropDateTimeStamp, ical.PropUID, ical.PropDateTimeStart, ical.PropClass,
		ical.PropCreated, ical.PropDescription, ical.PropGeo, ical.PropLastModified,
		ical.PropLocation, ical.PropOrganizer, ical.PropPriority, ical.PropRecurrenceRule,
		ical.PropSequence, ical.PropStatus, ical.PropSummary, ical.PropTransparency,
		ical.PropURL, ical.PropRecurrenceID, ical.PropDateTimeEnd, ical.PropDuration,
		ical.PropColor,
	},
	ical.CompToDo: {
		ical.PropDateTimeStamp, ical.PropUID, ical.PropClass, ical.PropCompleted,
		ical.PropCreated, ical.PropDescription, ical.PropDateTimeStart, ical.PropGeo,
		ical.PropLastModified, ical.PropLocation, ical.PropOrganizer, ical.PropPercentComplete,
		ical.PropPriority, ical.PropRecurrenceID, ical.PropSequence, ical.PropStatus,
		ical.PropSummary, ical.PropURL, ical.PropDue, ical.PropDuration, ical.PropColor,
	},
	ical.CompTimezone: {
		ical.PropTimezoneID, ical.PropLastModified, ical.PropTimezoneURL,
	},
	ical.CompTimezoneStandard: {
		ical.PropDateTimeStart, ical.PropTimezoneOffsetTo, ical.PropTimezoneOffsetFrom,
	},
	ical.CompTimezoneDaylight: {
		ical.PropDateTimeStart, ical.PropTimezoneOffsetTo, ical.PropTimezoneOffsetFrom,
	},
}

// dedupeSingleValued drops all but the first occurrence of each RFC 5545
// single-valued property (see singleValuedProps). The first is the one text()
// and the typed parsers already read, so this heals the encode-blocking
// duplicate without changing what the app displays. A duplicate of a required
// property is corruption, not data the iron rule protects, and keeping the
// resource writable preserves far more than the dropped duplicate.
func dedupeSingleValued(cal *ical.Calendar) {
	dedupeProps(cal.Props, singleValuedProps[ical.CompCalendar])
	for _, comp := range cal.Children {
		dedupeComponent(comp)
	}
}

func dedupeComponent(comp *ical.Component) {
	dedupeProps(comp.Props, singleValuedProps[comp.Name])
	for _, child := range comp.Children {
		dedupeComponent(child)
	}
}

func dedupeProps(props ical.Props, names []string) {
	for _, name := range names {
		if list := props[name]; len(list) > 1 {
			props[name] = list[:1]
		}
	}
}

// allowedChildren lists the only nested component types go-ical will encode
// under each parent (encoder.go): a VEVENT or VTODO admits only VALARM, a
// VTIMEZONE only STANDARD/DAYLIGHT. Any other nesting is malformed and, left in
// place, blocks encoding the entire resource.
var allowedChildren = map[string]map[string]bool{
	ical.CompEvent:    {ical.CompAlarm: true},
	ical.CompToDo:     {ical.CompAlarm: true},
	ical.CompTimezone: {ical.CompTimezoneStandard: true, ical.CompTimezoneDaylight: true},
	// go-ical forbids ANY nested component under VJOURNAL/VFREEBUSY (encoder.go),
	// so an empty allow-set strips whatever a foreign object nested there.
	ical.CompJournal:  {},
	ical.CompFreeBusy: {},
}

// stripForbiddenNesting removes illegally-nested child components so the object
// stays encodable. An event/todo can only ever contain a VALARM; anything else
// found nested there is corruption (never addressable as a real item, since Parse
// only walks the calendar's direct children), and dropping it keeps the parent
// item editable instead of making the whole resource unwritable.
func stripForbiddenNesting(cal *ical.Calendar) {
	for _, comp := range cal.Children {
		stripForbiddenChildren(comp)
	}
}

func stripForbiddenChildren(comp *ical.Component) {
	if allowed, ok := allowedChildren[comp.Name]; ok {
		kept := comp.Children[:0]
		for _, child := range comp.Children {
			if allowed[child.Name] {
				kept = append(kept, child)
			}
		}
		comp.Children = kept
	}
	for _, child := range comp.Children {
		stripForbiddenChildren(child)
	}
}

// dropEmptyTimezones removes any VTIMEZONE with no STANDARD/DAYLIGHT child.
// go-ical refuses to encode an empty VTIMEZONE, so such a component — present in a
// foreign object, or left childless after stripForbiddenNesting drops its only,
// illegally-nested child — would block encoding the whole resource (every sibling
// item too). An empty VTIMEZONE carries no offset data, and the app resolves zones
// via the embedded tz database rather than the object's VTIMEZONE, so dropping it
// loses nothing usable. Must run after stripForbiddenNesting.
func dropEmptyTimezones(cal *ical.Calendar) {
	kept := cal.Children[:0]
	for _, comp := range cal.Children {
		if comp.Name == ical.CompTimezone && len(comp.Children) == 0 {
			continue
		}
		kept = append(kept, comp)
	}
	cal.Children = kept
}

// healComponentConstraints drops properties that decode cleanly but violate
// go-ical's encoder mutual-exclusion / dependency rules (encoder.go), which would
// otherwise make the whole resource unwritable on the next edit. RFC 5545: a
// component carries a DTEND/DUE *or* a DURATION, never both, and a VTODO DURATION
// needs a DTSTART to anchor it. In every conflicting case the redundant DURATION
// is dropped (DTEND/DUE is the value the typed parser reads), so the heal only
// removes a duplicate/unanchored derived value — consistent with the iron rule and
// the other ingest healers (add-only-when-missing, never mangle real data).
func healComponentConstraints(cal *ical.Calendar) {
	for _, comp := range cal.Children {
		switch comp.Name {
		case ical.CompEvent:
			if comp.Props.Get(ical.PropDuration) != nil && comp.Props.Get(ical.PropDateTimeEnd) != nil {
				comp.Props.Del(ical.PropDuration)
			}
		case ical.CompToDo:
			hasDue := comp.Props.Get(ical.PropDue) != nil
			hasStart := comp.Props.Get(ical.PropDateTimeStart) != nil
			if comp.Props.Get(ical.PropDuration) != nil && (hasDue || !hasStart) {
				comp.Props.Del(ical.PropDuration)
			}
		}
	}
}

// sanitizePropValues strips raw CR and LF bytes from every property value in the
// calendar, including nested components (VALARM, VTIMEZONE). Per RFC 5545 a value
// never contains a raw control character — a real line break in text is the
// two-character escape "\n" (a backslash and an n, left intact here) — so a raw
// CR/LF is structural corruption that go-ical's decoder tolerates but its encoder
// rejects, which would make the containing item unwritable. Removing them keeps
// the item editable without altering any legitimate content.
func sanitizePropValues(cal *ical.Calendar) {
	sanitizeProps(cal.Props)
	for _, comp := range cal.Children {
		sanitizeComponent(comp)
	}
}

func sanitizeComponent(comp *ical.Component) {
	sanitizeProps(comp.Props)
	for _, child := range comp.Children {
		sanitizeComponent(child)
	}
}

func sanitizeProps(props ical.Props) {
	for _, list := range props {
		for i := range list {
			if strings.ContainsAny(list[i].Value, "\r\n") {
				list[i].Value = crlfStripper.Replace(list[i].Value)
			}
		}
	}
}

var crlfStripper = strings.NewReplacer("\r", "", "\n", "")

// ensureCalendarProps guarantees the VCALENDAR carries the RFC 5545-required
// VERSION and PRODID. go-ical's encoder refuses to serialize a calendar missing
// either, so a foreign or hand-edited object that omits them would decode yet
// become unwritable — blocking every edit of the items inside it. We add our own
// only when absent (an existing PRODID naming another producer is preserved, per
// the property-preservation iron rule), mirroring ensureDTStamp.
func ensureCalendarProps(cal *ical.Calendar) {
	if cal.Props.Get(ical.PropVersion) == nil {
		cal.Props.SetText(ical.PropVersion, icalVersion)
	}
	if cal.Props.Get(ical.PropProductID) == nil {
		cal.Props.SetText(ical.PropProductID, ProductID)
	}
}

// ensureDTStamp guarantees comp carries DTSTAMP, the RFC 5545-required object
// timestamp. Some tools and hand-edited vdir files omit it; go-ical's encoder
// then refuses to serialize the component, which would leave an otherwise-loaded
// item uneditable and unwritable — and block writing every sibling in the same
// resource. We heal it on ingest (mirroring how resolveDateTime recovers an
// unknown TZID) so the item degrades gracefully instead of hard-failing at the
// next edit. The value is taken from an existing timestamp (LAST-MODIFIED, then
// CREATED) so a re-decode of our own output is stable, with a fixed epoch only
// when the component carries no timestamp at all; a real edit overwrites DTSTAMP
// via touch(), so this placeholder rarely persists.
func ensureDTStamp(comp *ical.Component) {
	if comp.Props.Get(ical.PropDateTimeStamp) != nil {
		return
	}
	for _, src := range []string{ical.PropLastModified, ical.PropCreated} {
		if prop := comp.Props.Get(src); prop != nil {
			if t, err := prop.DateTime(time.UTC); err == nil {
				setDateTimeUTC(comp, ical.PropDateTimeStamp, t)
				return
			}
		}
	}
	setDateTimeUTC(comp, ical.PropDateTimeStamp, time.Date(1970, 1, 1, 0, 0, 0, 0, time.UTC))
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
