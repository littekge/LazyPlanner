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
	HasDue      bool
	Due         time.Time
	DueAllDay   bool
	Priority    int // 0 = none
	Categories  []string
	ParentUID   string // "" = root task
	Completed   bool
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
}

// NewTodoObject builds a fresh single-VTODO calendar object from d.
func NewTodoObject(d TodoDraft, now time.Time) *Parsed {
	cal, comp := newObject(ical.CompToDo, now)
	setCompleted(comp, d.Completed, now)
	applyTodo(comp, d, now)
	// Built from known-valid parts, so Parse cannot fail here.
	p, _ := Parse(cal, time.Local)
	return p
}

// NewEventObject builds a fresh single-VEVENT calendar object from d.
func NewEventObject(d EventDraft, now time.Time) (*Parsed, error) {
	cal, comp := newObject(ical.CompEvent, now)
	applyEvent(comp, d, now)
	return Parse(cal, time.Local)
}

// EditTodo returns a clone of obj with the todo identified by uid updated to d,
// leaving every other property (and every other component) untouched.
func EditTodo(obj *Parsed, uid string, d TodoDraft, now time.Time, loc *time.Location) (*Parsed, error) {
	return editComponent(obj, uid, loc, func(comp *ical.Component) {
		setCompleted(comp, d.Completed, now)
		applyTodo(comp, d, now)
	})
}

// EditEvent returns a clone of obj with the event identified by uid updated to d.
func EditEvent(obj *Parsed, uid string, d EventDraft, now time.Time, loc *time.Location) (*Parsed, error) {
	return editComponent(obj, uid, loc, func(comp *ical.Component) {
		applyEvent(comp, d, now)
	})
}

// SetTodoCompleted flips just the completion state of the todo identified by
// uid, preserving all other fields — the target of the Space shortcut.
func SetTodoCompleted(obj *Parsed, uid string, completed bool, now time.Time, loc *time.Location) (*Parsed, error) {
	return editComponent(obj, uid, loc, func(comp *ical.Component) {
		setCompleted(comp, completed, now)
		touch(comp, now)
	})
}

// SetTodoParent sets (or clears, when parentUID is "") the PARENT relationship of
// the todo identified by uid, preserving any non-parent RELATED-TO links.
func SetTodoParent(obj *Parsed, uid, parentUID string, now time.Time, loc *time.Location) (*Parsed, error) {
	return editComponent(obj, uid, loc, func(comp *ical.Component) {
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
	return editComponent(obj, uid, loc, func(comp *ical.Component) {
		comp.Props.SetText(ical.PropUID, newUID)
		setParent(comp, newParentUID)
		touch(comp, now)
	})
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
func editComponent(obj *Parsed, uid string, loc *time.Location, mutate func(*ical.Component)) (*Parsed, error) {
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

	if d.Priority == PriorityUndefined {
		comp.Props.Del(ical.PropPriority)
	} else {
		setInt(comp, ical.PropPriority, d.Priority)
	}

	setCategories(comp, d.Categories)

	if d.HasDue {
		setDateOrTime(comp, ical.PropDue, d.Due, d.DueAllDay)
	} else {
		comp.Props.Del(ical.PropDue)
	}

	setParent(comp, d.ParentUID)
	touch(comp, now)
}

// applyEvent writes d's known fields onto comp and stamps it modified.
func applyEvent(comp *ical.Component, d EventDraft, now time.Time) {
	setTextOrDel(comp, ical.PropSummary, d.Summary)
	setTextOrDel(comp, ical.PropDescription, d.Description)
	setTextOrDel(comp, ical.PropLocation, d.Location)

	setDateOrTime(comp, ical.PropDateTimeStart, d.Start, d.AllDay)
	// DTEND and DURATION are mutually exclusive; a set End writes DTEND (dropping
	// any inherited DURATION), a zero End clears both (zero-duration / point) —
	// symmetric with how applyTodo handles DUE.
	comp.Props.Del(ical.PropDuration)
	if !d.End.IsZero() {
		setDateOrTime(comp, ical.PropDateTimeEnd, d.End, d.AllDay)
	} else {
		comp.Props.Del(ical.PropDateTimeEnd)
	}

	bumpSequence(comp)
	touch(comp, now)
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
	prop := ical.NewProp(name)
	if allDay {
		prop.SetDate(t)
	} else {
		prop.SetDateTime(t.UTC())
	}
	comp.Props.Set(prop)
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
