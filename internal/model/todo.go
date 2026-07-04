package model

import (
	"fmt"
	"strings"
	"time"

	"github.com/emersion/go-ical"
)

// TodoStatus is the VTODO STATUS value. An empty status means the property was
// absent or unrecognized; callers treat that as not-yet-actioned.
type TodoStatus string

// The four RFC 5545 VTODO status values.
const (
	StatusNeedsAction TodoStatus = "NEEDS-ACTION"
	StatusInProcess   TodoStatus = "IN-PROCESS"
	StatusCompleted   TodoStatus = "COMPLETED"
	StatusCancelled   TodoStatus = "CANCELLED"
)

// PriorityUndefined is the iCal PRIORITY value for "no priority set". Defined
// priorities run 1 (highest) through 9 (lowest) per RFC 5545.
const PriorityUndefined = 0

// Todo is a task parsed from a VTODO. The deep subtask hierarchy — the
// centerpiece feature — is expressed by ParentUID (RELATED-TO;RELTYPE=PARENT);
// a "folder" is simply a task that other tasks point to.
//
// Raw is the source component, retained so that editing a known field leaves
// every other property untouched (the property-preservation iron rule).
type Todo struct {
	UID     string
	Summary string
	// Due is meaningful only when HasDue is true. DueAllDay marks a date-only
	// due date (no time of day).
	Due         time.Time
	HasDue      bool
	DueAllDay   bool
	Status      TodoStatus
	Priority    int
	Categories  []string
	Description string
	ParentUID   string
	Recurring   bool

	Raw *ical.Component
}

// Completed reports whether the task is done (STATUS:COMPLETED).
func (t *Todo) Completed() bool { return t.Status == StatusCompleted }

// ParseTodo builds a Todo from a VTODO component, interpreting floating times
// in loc (nil defaults to time.Local). A VTODO has no required dated field, so
// ParseTodo returns an error only when a DUE that is present is malformed; all
// other optional fields degrade to their zero value.
func ParseTodo(comp *ical.Component, loc *time.Location) (*Todo, error) {
	if loc == nil {
		loc = time.Local
	}

	td := &Todo{
		UID:         text(comp.Props, ical.PropUID),
		Summary:     text(comp.Props, ical.PropSummary),
		Status:      parseTodoStatus(text(comp.Props, ical.PropStatus)),
		Description: text(comp.Props, ical.PropDescription),
		ParentUID:   parentUID(comp),
		Categories:  categories(comp),
		Priority:    priority(comp),
		Recurring:   isRecurring(comp),
		Raw:         comp,
	}

	if dueProp := comp.Props.Get(ical.PropDue); dueProp != nil {
		due, err := dueProp.DateTime(loc)
		if err != nil {
			return nil, fmt.Errorf("VTODO %q: parsing DUE: %w", td.UID, err)
		}
		td.Due = due
		td.HasDue = true
		td.DueAllDay = isDateOnly(dueProp)
	}

	return td, nil
}

func parseTodoStatus(s string) TodoStatus {
	switch status := TodoStatus(strings.ToUpper(strings.TrimSpace(s))); status {
	case StatusNeedsAction, StatusInProcess, StatusCompleted, StatusCancelled:
		return status
	default:
		return ""
	}
}

// priority returns the iCal PRIORITY, or PriorityUndefined when it is absent or
// outside the valid 0–9 range.
func priority(comp *ical.Component) int {
	prop := comp.Props.Get(ical.PropPriority)
	if prop == nil {
		return PriorityUndefined
	}
	n, err := prop.Int()
	if err != nil || n < 0 || n > 9 {
		return PriorityUndefined
	}
	return n
}

// categories collects tags across all CATEGORIES properties, each of which may
// hold a comma-separated list. Empty entries are dropped.
func categories(comp *ical.Component) []string {
	var out []string
	for _, prop := range comp.Props.Values(ical.PropCategories) {
		list, err := prop.TextList()
		if err != nil {
			continue
		}
		for _, c := range list {
			if c = strings.TrimSpace(c); c != "" {
				out = append(out, c)
			}
		}
	}
	return out
}
