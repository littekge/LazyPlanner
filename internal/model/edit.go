package model

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/emersion/go-ical"
)

// ProductID identifies calendar objects authored by LazyPlanner (the PRODID the
// encoder requires on every VCALENDAR).
const ProductID = "-//LazyPlanner//LazyPlanner//EN"

// icalVersion is the iCalendar spec version stamped on new objects.
const icalVersion = "2.0"

// NewUID returns a random, collision-resistant UID suitable for a new VEVENT or
// VTODO. The value follows the common "<random>@domain" shape.
func NewUID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		// A failing system CSPRNG is a broken machine; degrade to a timestamp so
		// an edit still produces a unique-enough id rather than failing.
		return fmt.Sprintf("%d@lazyplanner", time.Now().UnixNano())
	}
	return hex.EncodeToString(b) + "@lazyplanner"
}

// TodoDraft is the set of known VTODO fields the editor writes. Everything not
// listed here (X- props, VALARMs, other clients' metadata) is preserved when
// editing an existing todo — the property-preservation iron rule.
type TodoDraft struct {
	Summary     string
	Description string
	Location    string
	HasDue      bool
	Due         time.Time
	DueAllDay   bool
	Priority    int // 0 = none
	Categories  []string
	ParentUID   string // "" = root task
	Completed   bool
	// Recurrence control: nil Recur + !RecurRemove leaves an existing RRULE
	// untouched; a non-nil Recur (re)writes the rule; RecurRemove deletes the rule
	// and its EXDATE/RDATE. See applyTodo.
	Recur       *RecurSpec
	RecurRemove bool
}

// EventDraft is the set of known VEVENT fields the editor writes; all other
// properties on an edited event are preserved.
type EventDraft struct {
	Summary     string
	Description string
	Location    string
	Start       time.Time
	End         time.Time // exclusive end (iCal DTEND semantics)
	AllDay      bool
	// Recurrence control: nil Recur + !RecurRemove leaves an existing RRULE
	// untouched; a non-nil Recur (re)writes the rule; RecurRemove deletes the rule
	// and its EXDATE/RDATE. See applyEvent.
	Recur       *RecurSpec
	RecurRemove bool
}

// NewTodoObject builds a fresh single-VTODO calendar object from d.
func NewTodoObject(d TodoDraft, now time.Time) *Parsed {
	cal, comp := newObject(ical.CompToDo, now)
	setCompleted(comp, d.Completed, now)
	applyTodo(comp, d, now)
	ensureVTimezone(cal, now)
	// Built from known-valid parts, so Parse cannot fail here.
	p, _ := Parse(cal, time.Local)
	return p
}

// NewEventObject builds a fresh single-VEVENT calendar object from d.
func NewEventObject(d EventDraft, now time.Time) (*Parsed, error) {
	cal, comp := newObject(ical.CompEvent, now)
	applyEvent(comp, d, now)
	ensureVTimezone(cal, now)
	return Parse(cal, time.Local)
}

// EditTodo returns a clone of obj with the todo identified by uid updated to d,
// leaving every other property (and every other component) untouched.
func EditTodo(obj *Parsed, uid string, d TodoDraft, now time.Time, loc *time.Location) (*Parsed, error) {
	return editComponent(obj, uid, now, loc, func(comp *ical.Component) {
		// Only rewrite the completion trio when the completed-ness actually changes.
		// TodoDraft.Completed is a single bool, but VTODO STATUS is quad-state
		// (NEEDS-ACTION / IN-PROCESS / COMPLETED / CANCELLED). A quick field-set
		// (sp/sd) or any edit that doesn't touch completion carries Completed =
		// td.Completed() unchanged; calling setCompleted then would flatten a foreign
		// client's IN-PROCESS/CANCELLED status to NEEDS-ACTION (dropping
		// PERCENT-COMPLETE) or restamp COMPLETED to now — an iron-rule breach. Skip it
		// when nothing changed so the existing status/percent/timestamp are preserved.
		if d.Completed != isCompletedStatus(comp) {
			setCompleted(comp, d.Completed, now)
		}
		applyTodo(comp, d, now)
	})
}

// EditEvent returns a clone of obj with the event identified by uid updated to d.
func EditEvent(obj *Parsed, uid string, d EventDraft, now time.Time, loc *time.Location) (*Parsed, error) {
	return editComponent(obj, uid, now, loc, func(comp *ical.Component) {
		applyEvent(comp, d, now)
	})
}

// SetTodoCompleted flips just the completion state of the todo identified by
// uid, preserving all other fields — the target of the Space shortcut.
func SetTodoCompleted(obj *Parsed, uid string, completed bool, now time.Time, loc *time.Location) (*Parsed, error) {
	return editComponent(obj, uid, now, loc, func(comp *ical.Component) {
		setCompleted(comp, completed, now)
		touch(comp, now)
	})
}

// SetTodoParent sets (or clears, when parentUID is "") the PARENT relationship of
// the todo identified by uid, preserving any non-parent RELATED-TO links.
func SetTodoParent(obj *Parsed, uid, parentUID string, now time.Time, loc *time.Location) (*Parsed, error) {
	return editComponent(obj, uid, now, loc, func(comp *ical.Component) {
		setParent(comp, parentUID)
		touch(comp, now)
	})
}

// CopyTodo returns a duplicate of the todo carrying uid in obj, re-keyed to a
// fresh newUID and re-parented to newParentUID (empty = top level). Every other
// iCal property is preserved (property-preservation iron rule), so a copied task
// keeps its fields, tags, notes, and any unknown props. Used by yank/paste's copy
// mode; descendants are copied by the caller, remapping each child's parent link.
func CopyTodo(obj *Parsed, uid, newUID, newParentUID string, now time.Time, loc *time.Location) (*Parsed, error) {
	return editComponent(obj, uid, now, loc, func(comp *ical.Component) {
		comp.Props.SetText(ical.PropUID, newUID)
		setParent(comp, newParentUID)
		touch(comp, now)
	})
}

// isItemComponent reports whether c is a top-level schedulable item (a VEVENT or
// VTODO) — the components that carry a user-facing UID and can be co-resident in a
// bundled resource.
func isItemComponent(c *ical.Component) bool {
	return c.Name == ical.CompEvent || c.Name == ical.CompToDo
}

// IsolateComponent returns a copy of obj containing only the VEVENT/VTODO carrying
// uid, dropping any co-resident sibling *items* (non-item components like VTIMEZONE
// are kept). LazyPlanner writes one item per resource, but a foreign or hand-edited
// .ics can bundle several; move/copy must act on the selected item alone rather
// than dragging or duplicating its file-mates.
func IsolateComponent(obj *Parsed, uid string, loc *time.Location) (*Parsed, error) {
	if loc == nil {
		loc = time.Local
	}
	clone, err := obj.clone(loc)
	if err != nil {
		return nil, err
	}
	if findComponent(clone.Calendar, uid) == nil {
		return nil, fmt.Errorf("model: no event or todo with UID %q", uid)
	}
	kept := clone.Calendar.Children[:0]
	for _, c := range clone.Calendar.Children {
		if isItemComponent(c) && text(c.Props, ical.PropUID) != uid {
			continue
		}
		kept = append(kept, c)
	}
	clone.Calendar.Children = kept
	return Parse(clone.Calendar, loc)
}

// RemoveComponent returns a copy of obj with the VEVENT/VTODO carrying uid removed,
// and reports whether any item component remains. The caller rewrites the resource
// when items remain (a bundled file loses only the moved item) or deletes it when
// none do — so a cross-list move never drags a co-resident bystander.
func RemoveComponent(obj *Parsed, uid string, loc *time.Location) (result *Parsed, remaining bool, err error) {
	if loc == nil {
		loc = time.Local
	}
	clone, err := obj.clone(loc)
	if err != nil {
		return nil, false, err
	}
	if findComponent(clone.Calendar, uid) == nil {
		return nil, false, fmt.Errorf("model: no event or todo with UID %q", uid)
	}
	kept := clone.Calendar.Children[:0]
	for _, c := range clone.Calendar.Children {
		if isItemComponent(c) && text(c.Props, ical.PropUID) == uid {
			continue
		}
		kept = append(kept, c)
	}
	clone.Calendar.Children = kept
	out, err := Parse(clone.Calendar, loc)
	if err != nil {
		return nil, false, err
	}
	return out, len(out.Events) > 0 || len(out.Todos) > 0, nil
}

// newObject creates a VCALENDAR wrapping one empty component of compName, with
// the required VERSION/PRODID and the component's required UID/DTSTAMP/CREATED.
func newObject(compName string, now time.Time) (*ical.Calendar, *ical.Component) {
	cal := ical.NewCalendar()
	cal.Props.SetText(ical.PropVersion, icalVersion)
	cal.Props.SetText(ical.PropProductID, ProductID)

	comp := ical.NewComponent(compName)
	comp.Props.SetText(ical.PropUID, NewUID())
	setDateTimeUTC(comp, ical.PropCreated, now)
	cal.Children = append(cal.Children, comp)
	return cal, comp
}

// editComponent clones obj (via encode/decode, so the store's snapshot is never
// mutated), applies mutate to the child component with the given UID, and
// re-parses so the typed fields match the edited raw component.
func editComponent(obj *Parsed, uid string, now time.Time, loc *time.Location, mutate func(*ical.Component)) (*Parsed, error) {
	if loc == nil {
		loc = time.Local
	}
	clone, err := obj.clone(loc)
	if err != nil {
		return nil, err
	}
	comp := findComponent(clone.Calendar, uid)
	if comp == nil {
		return nil, fmt.Errorf("model: no event or todo with UID %q", uid)
	}
	mutate(comp)
	ensureVTimezone(clone.Calendar, now)
	return Parse(clone.Calendar, loc)
}

// clone deep-copies the parsed object by round-tripping through the encoder, so
// edits operate on an independent calendar and never touch a shared snapshot.
func (p *Parsed) clone(loc *time.Location) (*Parsed, error) {
	data, err := p.Encode()
	if err != nil {
		return nil, fmt.Errorf("cloning object: %w", err)
	}
	return Decode(data, loc)
}

// findComponent returns the VEVENT or VTODO child carrying uid, or nil.
func findComponent(cal *ical.Calendar, uid string) *ical.Component {
	for _, c := range cal.Children {
		if c.Name != ical.CompEvent && c.Name != ical.CompToDo {
			continue
		}
		if text(c.Props, ical.PropUID) == uid {
			return c
		}
	}
	return nil
}

// applyTodo writes d's known fields onto comp and stamps it modified.
func applyTodo(comp *ical.Component, d TodoDraft, now time.Time) {
	setTextOrDel(comp, ical.PropSummary, d.Summary)
	setTextOrDel(comp, ical.PropDescription, d.Description)
	setTextOrDel(comp, ical.PropLocation, d.Location)

	if d.Priority == PriorityUndefined {
		comp.Props.Del(ical.PropPriority)
	} else {
		setInt(comp, ical.PropPriority, d.Priority)
	}

	setCategories(comp, d.Categories)

	if d.HasDue {
		zone := anchorZone(comp, ical.PropDue, d.Recur, d.DueAllDay, d.Due)
		setAnchorDateOrTime(comp, ical.PropDue, d.Due, d.DueAllDay, zone)
	} else {
		comp.Props.Del(ical.PropDue)
	}

	setParent(comp, d.ParentUID)
	applyRecurrence(comp, d.Recur, d.RecurRemove)
	touch(comp, now)
}

// applyEvent writes d's known fields onto comp and stamps it modified.
func applyEvent(comp *ical.Component, d EventDraft, now time.Time) {
	setTextOrDel(comp, ical.PropSummary, d.Summary)
	setTextOrDel(comp, ical.PropDescription, d.Description)
	setTextOrDel(comp, ical.PropLocation, d.Location)

	// The zone is decided once, from DTSTART: it is the rule's anchor, and DTEND
	// must keep the same value type and zone as the DTSTART it bounds.
	zone := anchorZone(comp, ical.PropDateTimeStart, d.Recur, d.AllDay, d.Start)
	setAnchorDateOrTime(comp, ical.PropDateTimeStart, d.Start, d.AllDay, zone)
	// DTEND and DURATION are mutually exclusive; a set End writes DTEND (dropping
	// any inherited DURATION), a zero End clears both (zero-duration / point) —
	// symmetric with how applyTodo handles DUE.
	comp.Props.Del(ical.PropDuration)
	if !d.End.IsZero() {
		setAnchorDateOrTime(comp, ical.PropDateTimeEnd, d.End, d.AllDay, zone)
	} else {
		comp.Props.Del(ical.PropDateTimeEnd)
	}

	applyRecurrence(comp, d.Recur, d.RecurRemove)

	bumpSequence(comp)
	touch(comp, now)
}

// applyRecurrence writes the component's series-level recurrence per a draft's
// Recur/RecurRemove pair, on the master component only:
//   - remove: delete the RRULE and its EXDATE/RDATE (Repeat → None). Sibling
//     RECURRENCE-ID overrides live in other components and are pruned by the
//     object-level caller (RewriteEventRule).
//   - recur non-nil: (re)write the RRULE. rrule-go always renders UNTIL as a
//     DATE-TIME, but RFC 5545 §3.3.10 requires UNTIL's value type to match a
//     VALUE=DATE anchor, so an all-day series gets a DATE-only UNTIL.
//   - neither: leave any existing RRULE untouched (iron rule — a semantically
//     equal rewrite could still drop oddities like WKST; the UI rewrites only
//     when the rule actually changed).
func applyRecurrence(comp *ical.Component, recur *RecurSpec, remove bool) {
	if remove {
		comp.Props.Del(ical.PropRecurrenceRule)
		comp.Props.Del(ical.PropRecurrenceDates)
		comp.Props.Del(ical.PropExceptionDates)
		return
	}
	if recur == nil {
		return
	}
	comp.Props.SetRecurrenceRule(recur.ROption())
	if recur.Until != nil && anchorIsDateOnly(comp) {
		if rp := comp.Props.Get(ical.PropRecurrenceRule); rp != nil {
			// The end day is the one the user picked, i.e. the wall-clock date of
			// recur.Until in its own location — not the UTC date rrule-go rendered.
			rp.Value = dateOnlyUntil(rp.Value, *recur.Until)
		}
	}
}

// anchorIsDateOnly reports whether the component's recurrence anchor (DTSTART, or
// DUE for a VTODO with no DTSTART) is a VALUE=DATE (all-day) value — the case
// where UNTIL must also be date-only.
func anchorIsDateOnly(comp *ical.Component) bool {
	for _, n := range []string{ical.PropDateTimeStart, ical.PropDue} {
		if p := comp.Props.Get(n); p != nil {
			return isDateOnly(p)
		}
	}
	return false
}

// isCompletedStatus reports whether comp currently carries STATUS:COMPLETED — the
// completed-ness that TodoDraft.Completed round-trips. Any other status (including
// IN-PROCESS, CANCELLED, a missing STATUS, or NEEDS-ACTION) is not completed.
func isCompletedStatus(comp *ical.Component) bool {
	return strings.EqualFold(text(comp.Props, ical.PropStatus), string(StatusCompleted))
}

// setCompleted writes the RFC 5545 completion trio (STATUS/PERCENT-COMPLETE/
// COMPLETED) so NextCloud Tasks and other clients agree on the state.
func setCompleted(comp *ical.Component, completed bool, now time.Time) {
	if completed {
		comp.Props.SetText(ical.PropStatus, string(StatusCompleted))
		setInt(comp, ical.PropPercentComplete, 100)
		setDateTimeUTC(comp, ical.PropCompleted, now)
	} else {
		comp.Props.SetText(ical.PropStatus, string(StatusNeedsAction))
		comp.Props.Del(ical.PropPercentComplete)
		comp.Props.Del(ical.PropCompleted)
	}
}

// setParent replaces the PARENT RELATED-TO link (adding one when parentUID is
// non-empty), keeping any RELATED-TO of another relationship type intact.
func setParent(comp *ical.Component, parentUID string) {
	var kept []ical.Prop
	for _, p := range comp.Props.Values(ical.PropRelatedTo) {
		reltype := p.Params.Get(ical.ParamRelationshipType)
		if reltype == "" || strings.EqualFold(reltype, "PARENT") {
			continue // the default relationship is PARENT; drop existing parent links
		}
		kept = append(kept, p)
	}
	if parentUID != "" {
		pr := ical.NewProp(ical.PropRelatedTo)
		pr.Value = parentUID
		pr.Params.Set(ical.ParamRelationshipType, "PARENT")
		kept = append(kept, *pr)
	}
	if len(kept) == 0 {
		comp.Props.Del(ical.PropRelatedTo)
	} else {
		comp.Props[ical.PropRelatedTo] = kept
	}
}

// setCategories writes a single CATEGORIES property (or removes it when empty),
// collapsing the tags into one comma-separated value.
func setCategories(comp *ical.Component, tags []string) {
	if len(tags) == 0 {
		comp.Props.Del(ical.PropCategories)
		return
	}
	prop := ical.NewProp(ical.PropCategories)
	prop.SetTextList(tags)
	comp.Props.Set(prop)
}

// setTextOrDel sets a text property, or removes it entirely when the value is
// empty, so editing never leaves an empty SUMMARY/DESCRIPTION behind.
func setTextOrDel(comp *ical.Component, name, value string) {
	if strings.TrimSpace(value) == "" {
		comp.Props.Del(name)
		return
	}
	comp.Props.SetText(name, value)
}

// setDateOrTime writes name as a date-only value (all-day) or a UTC date-time.
// Timed values are stored in UTC (Z form) so they are unambiguous; display
// converts back to local. All-day values stay date-only per the spec.
func setDateOrTime(comp *ical.Component, name string, t time.Time, allDay bool) {
	comp.Props.Set(newDateOrTimeProp(name, t, allDay))
}

// newDateOrTimeProp builds a date-or-time property applying the storage rule in
// one place: all-day → date-only, timed → UTC (Z form). Used by setDateOrTime
// (which replaces) and by the multi-valued EXDATE writer (which appends), so the
// rule has a single home.
func newDateOrTimeProp(name string, t time.Time, allDay bool) *ical.Prop {
	prop := ical.NewProp(name)
	if allDay {
		prop.SetDate(t)
	} else {
		prop.SetDateTime(t.UTC())
	}
	return prop
}

// setAnchorDateOrTime writes a value that a recurrence rule may be anchored to.
//
// It differs from setDateOrTime in one respect: when zone is non-nil the value is
// written as local wall clock + TZID rather than the UTC Z form. That matters
// because RFC 5545 evaluates a rule's BY* parts in its anchor's own zone, so a
// UTC-anchored rule authored from a local weekday fires on the wrong day whenever
// the local and UTC dates differ — and drifts an hour across DST.
//
// go-ical's SetDateTime already emits the TZID form for any non-UTC location, so
// the zone choice is expressed entirely by which location t carries.
func setAnchorDateOrTime(comp *ical.Component, name string, t time.Time, allDay bool, zone *time.Location) {
	comp.Props.Set(newAnchorDateOrTimeProp(name, t, allDay, zone))
}

// newAnchorDateOrTimeProp builds the property setAnchorDateOrTime stores. It is
// separate so the multi-valued EXDATE writer — which appends rather than replaces
// — can apply the same zone rule.
func newAnchorDateOrTimeProp(name string, t time.Time, allDay bool, zone *time.Location) *ical.Prop {
	prop := ical.NewProp(name)
	switch {
	case allDay:
		prop.SetDate(t)
	case zone != nil:
		prop.SetDateTime(t.In(zone))
	default:
		prop.SetDateTime(t.UTC())
	}
	return prop
}

// anchorZone returns the zone a recurrence anchor should be written in, or nil for
// the UTC form.
//
// Two cases produce a zoned anchor, and the order between them is the whole
// correctness argument:
//
//  1. This call is authoring the rule itself (recur != nil) and t is in a zone
//     another client can resolve — anchor in it. The BY* parts are being derived
//     from t's own weekday/day-of-month in this same call, so anchoring in t's zone
//     is the only way the rule and its anchor agree. This must be checked FIRST,
//     even against an existing TZID: a foreign DTSTART;TZID=Europe/Berlin edited by
//     a New York user who picks "Weekly on Tue" would otherwise keep the Berlin
//     Wednesday anchor while carrying a BYDAY=TU derived from the New York Tuesday
//     — reproducing, on the edit path, the very defect this function exists to fix.
//  2. Otherwise the component's existing anchor already carries a resolvable TZID —
//     keep writing in THAT zone. It may be a server's own, and re-expressing its
//     data in ours would churn it (iron rule); flattening it to UTC, which this code
//     did before, silently broke the rule the server authored. This still covers a
//     rule-authoring edit whose own zone cannot be named, where preserving the
//     server's zone beats destroying it for a UTC form that agrees with the new BY*
//     no better.
//
// Anything else keeps the UTC form, including every non-recurring value and every
// edit that leaves an existing rule alone. That last exclusion is deliberate:
// re-anchoring a series without re-deriving its BY* would move it.
func anchorZone(comp *ical.Component, name string, recur *RecurSpec, allDay bool, t time.Time) *time.Location {
	if allDay {
		return nil
	}
	if recur != nil && IsNamedZone(t.Location()) {
		return t.Location()
	}
	if existing := comp.Props.Get(name); existing != nil {
		if loc := zoneForTZID(existing.Params.Get(ical.ParamTimezoneID)); loc != nil {
			return loc
		}
	}
	return nil
}

// zoneForTZID resolves a TZID parameter to a location, or nil when the name is
// one no client could resolve.
//
// A Windows/Outlook name ("Eastern Standard Time") is mapped to its IANA
// equivalent, mirroring what resolveDateTime already does on the read side.
// Without that mapping an Outlook-authored series falls through to the UTC form
// on every edit, which moves a day-pinned rule (TZID=Eastern Standard Time 20:00
// Tuesday flattens to Wednesday 00:00Z while BYDAY=TU stays put) — the exact
// breakage the anchor rules exist to prevent. Rewriting the param to the IANA name
// is the point, not a side effect: it is the same zone, expressed in the form every
// other client can resolve and the form ensureVTimezone can define.
func zoneForTZID(tzid string) *time.Location {
	if tzid == "" {
		return nil
	}
	if iana := windowsToIANA(tzid); iana != "" {
		tzid = iana
	}
	loc, err := time.LoadLocation(tzid)
	if err != nil {
		return nil
	}
	return loc
}

// ensureVTimezone adds a VTIMEZONE for every TZID the object's items reference
// and that the object does not already define. Additive only: an existing
// VTIMEZONE (ours or a foreign server's) is never replaced or removed — per the
// iron rule, and because a server's own definition is the authority for its data.
//
// The defined-set is why this is cheap enough to sit on every write path:
// BuildVTimezone probes the zone database ~1100 times, so it must run once per
// newly-referenced TZID and never for one the object already carries.
func ensureVTimezone(cal *ical.Calendar, around time.Time) {
	defined := map[string]bool{}
	for _, c := range cal.Children {
		if c.Name == ical.CompTimezone {
			if p := c.Props.Get(ical.PropTimezoneID); p != nil {
				defined[p.Value] = true
			}
		}
	}
	// The item properties whose value may carry a TZID needing a definition. The
	// recurrence set's own dates belong here as much as the anchor does: an
	// EXDATE/RDATE/RECURRENCE-ID is written in the master's anchor zone, and that zone
	// is the *resolved IANA* one — so a master whose DTSTART still carries a Windows
	// spelling ("Eastern Standard Time") ends up beside an EXDATE;TZID=America/New_York
	// naming a zone no DTSTART/DTEND/DUE mentions.
	anchorProps := []string{
		ical.PropDateTimeStart, ical.PropDateTimeEnd, ical.PropDue,
		ical.PropExceptionDates, ical.PropRecurrenceDates, ical.PropRecurrenceID,
	}
	for _, c := range cal.Children {
		if !isItemComponent(c) {
			continue
		}
		for _, name := range anchorProps {
			// EXDATE and RDATE may repeat, each line with its own TZID, so every
			// occurrence of the property is scanned rather than just the first.
			for _, p := range c.Props.Values(name) {
				tzid := p.Params.Get(ical.ParamTimezoneID)
				if tzid == "" || defined[tzid] {
					continue
				}
				loc, err := time.LoadLocation(tzid)
				if err != nil {
					continue
				}
				if tz := BuildVTimezone(loc, around); tz != nil {
					// VTIMEZONE must precede the components referencing it.
					cal.Children = append([]*ical.Component{tz}, cal.Children...)
					defined[tzid] = true
				}
			}
		}
	}
}

func setDateTimeUTC(comp *ical.Component, name string, t time.Time) {
	prop := ical.NewProp(name)
	prop.SetDateTime(t.UTC())
	comp.Props.Set(prop)
}

// touch stamps DTSTAMP and LAST-MODIFIED to now (UTC), marking the most recent
// edit for the server and other clients.
func touch(comp *ical.Component, now time.Time) {
	setDateTimeUTC(comp, ical.PropDateTimeStamp, now)
	setDateTimeUTC(comp, ical.PropLastModified, now)
}

// bumpSequence increments SEQUENCE (starting at 0) so revisions are ordered for
// clients that track it.
func bumpSequence(comp *ical.Component) {
	seq := 0
	if prop := comp.Props.Get(ical.PropSequence); prop != nil {
		if n, err := prop.Int(); err == nil && n >= 0 {
			seq = n
		}
	}
	setInt(comp, ical.PropSequence, seq+1)
}

// setInt writes an integer-valued property without a VALUE parameter, so it
// round-trips through Prop.Int (SetText would tag it VALUE=TEXT and break that).
func setInt(comp *ical.Component, name string, n int) {
	prop := ical.NewProp(name)
	prop.Value = strconv.Itoa(n)
	comp.Props.Set(prop)
}
