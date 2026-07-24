package model

import (
	"time"

	"github.com/emersion/go-ical"
	"github.com/teambition/rrule-go"
)

// The full-form "Repeat" dropdown state. RepeatChoices is pure and tview-free so
// the UI stays thin glue: it builds the ordered option labels and initial
// selection from an item's rule (seeding), and maps a final selection + anchor
// back to a draft's Recur/RecurRemove (reading). Keeping it here also keeps the
// rrule/ical dependencies out of internal/ui.

type repeatKind int

const (
	repeatNone      repeatKind = iota
	repeatDaily                // preset — anchor-derived on read
	repeatWeekly               // preset — "on <anchor weekday>"
	repeatMonthly              // preset — "on day <anchor day-of-month>"
	repeatYearly               // preset — "on <anchor month/day>"
	repeatKept                 // representable but not a plain preset; carries its spec
	repeatKeptRaw              // outside the vocabulary; the raw rule is preserved
	repeatCustomSet            // a spec built in the Custom… sub-form; carries its spec
	repeatCustom               // opens the Custom… sub-form
)

type repeatOption struct {
	label string
	kind  repeatKind
	spec  RecurSpec // set for repeatKept
}

// RepeatChoices is the state behind the full-form Repeat dropdown.
type RepeatChoices struct {
	options    []repeatOption
	selected   int
	hadRule    bool // the item was recurring when the form opened
	seeded     RecurSpec
	seededRepr bool // seeded is a valid decomposition (a representable original rule)
}

// NewRepeatChoices seeds the dropdown for the item in comp (nil = a create form),
// anchored at the item's start/due date. It always offers None, the four
// anchor-derived presets, and Custom…; a representable non-preset rule adds a
// humanized entry (selected), and an unrepresentable or RDATE-only rule adds a
// "Custom rule (kept)" entry (selected) whose bytes are preserved when left as-is.
func NewRepeatChoices(comp *ical.Component, anchor time.Time, loc *time.Location) *RepeatChoices {
	rc := &RepeatChoices{
		options: []repeatOption{
			{label: "None", kind: repeatNone},
			{label: presetSpec(repeatDaily, anchor).Humanize(anchor), kind: repeatDaily},
			{label: presetSpec(repeatWeekly, anchor).Humanize(anchor), kind: repeatWeekly},
			{label: presetSpec(repeatMonthly, anchor).Humanize(anchor), kind: repeatMonthly},
			{label: presetSpec(repeatYearly, anchor).Humanize(anchor), kind: repeatYearly},
		},
	}
	rule, recurring := recurrenceInfo(comp)
	if recurring {
		rc.hadRule = true
		if spec, ok := decomposeForSeed(rule, anchor); ok {
			rc.seeded, rc.seededRepr = spec, true
			if idx := rc.matchPreset(spec, anchor); idx >= 0 {
				rc.selected = idx
			} else {
				rc.options = append(rc.options, repeatOption{label: spec.Humanize(anchor), kind: repeatKept, spec: spec})
				rc.selected = len(rc.options) - 1
			}
		} else {
			rc.options = append(rc.options, repeatOption{label: "Custom rule (kept)", kind: repeatKeptRaw})
			rc.selected = len(rc.options) - 1
		}
	}
	rc.options = append(rc.options, repeatOption{label: "Custom…", kind: repeatCustom})
	return rc
}

// Labels returns the option labels in order, for the dropdown.
func (rc *RepeatChoices) Labels() []string {
	out := make([]string, len(rc.options))
	for i, o := range rc.options {
		out[i] = o.label
	}
	return out
}

// Selected returns the initially-selected option index.
func (rc *RepeatChoices) Selected() int { return rc.selected }

// IsCustom reports whether the option at idx is the Custom… entry (the trigger
// for the sub-form).
func (rc *RepeatChoices) IsCustom(idx int) bool {
	return idx >= 0 && idx < len(rc.options) && rc.options[idx].kind == repeatCustom
}

// Resolve maps the selected option and the final anchor (the form's saved
// start/due, so a preset re-derives its weekday/day from an edited date) to a
// draft's recurrence control:
//   - None: remove an existing rule, else no-op.
//   - a preset: rewrite unless it equals the untouched seeded rule.
//   - a kept / kept-raw / Custom… entry: leave the original rule untouched (the
//     Custom… sub-form supplies its own spec in the UI before saving).
func (rc *RepeatChoices) Resolve(selected int, anchor time.Time) (recur *RecurSpec, remove bool) {
	if selected < 0 || selected >= len(rc.options) {
		return nil, false
	}
	switch opt := rc.options[selected]; opt.kind {
	case repeatNone:
		return nil, rc.hadRule
	case repeatKept, repeatKeptRaw, repeatCustom:
		return nil, false
	case repeatCustomSet:
		// A spec built in the sub-form: a rewrite, unless it happens to equal the
		// untouched original (then preserve the original bytes — iron rule).
		spec := opt.spec
		if rc.hadRule && rc.seededRepr && recurSpecEqual(spec, rc.seeded) {
			return nil, false
		}
		return &spec, false
	default:
		spec := presetSpec(opt.kind, anchor)
		if rc.hadRule && rc.seededRepr && recurSpecEqual(spec, rc.seeded) {
			return nil, false // unchanged — preserve the original bytes (iron rule)
		}
		return &spec, false
	}
}

// SetCustom records a spec built in the Custom… sub-form as a selectable,
// humanized entry (replacing any prior custom entry so the list never grows
// without bound) and selects it. Returns the entry's index.
func (rc *RepeatChoices) SetCustom(spec RecurSpec, anchor time.Time) int {
	opt := repeatOption{label: spec.Humanize(anchor), kind: repeatCustomSet, spec: spec}
	for i := range rc.options {
		if rc.options[i].kind == repeatCustomSet {
			rc.options[i] = opt
			rc.selected = i
			return i
		}
	}
	// Insert just before the trailing Custom… entry.
	at := len(rc.options) - 1
	rc.options = append(rc.options[:at], append([]repeatOption{opt}, rc.options[at:]...)...)
	rc.selected = at
	return at
}

// SeedSpec returns the spec to seed the Custom… sub-form from the option at idx,
// and whether it carried a real spec (false for None / kept-raw / Custom… — the
// caller supplies a plain default).
func (rc *RepeatChoices) SeedSpec(idx int, anchor time.Time) (RecurSpec, bool) {
	if idx < 0 || idx >= len(rc.options) {
		return RecurSpec{}, false
	}
	switch opt := rc.options[idx]; opt.kind {
	case repeatDaily, repeatWeekly, repeatMonthly, repeatYearly:
		return presetSpec(opt.kind, anchor), true
	case repeatKept, repeatCustomSet:
		return opt.spec, true
	default:
		return RecurSpec{}, false
	}
}

// matchPreset returns the index of the preset option equal to spec at anchor, or
// -1 when spec is representable but not a plain preset.
func (rc *RepeatChoices) matchPreset(spec RecurSpec, anchor time.Time) int {
	for i, opt := range rc.options {
		switch opt.kind {
		case repeatDaily, repeatWeekly, repeatMonthly, repeatYearly:
			if recurSpecEqual(spec, presetSpec(opt.kind, anchor)) {
				return i
			}
		}
	}
	return -1
}

// presetSpec is the RecurSpec a preset produces at the given anchor. Weekly is
// "on the anchor's weekday"; monthly (by day-of-month) and yearly carry no stored
// day/month — the anchored DTSTART supplies them.
func presetSpec(kind repeatKind, anchor time.Time) RecurSpec {
	switch kind {
	case repeatDaily:
		return RecurSpec{Freq: FreqDaily}
	case repeatWeekly:
		return RecurSpec{Freq: FreqWeekly, Weekdays: []time.Weekday{anchor.Weekday()}}
	case repeatMonthly:
		return RecurSpec{Freq: FreqMonthly}
	case repeatYearly:
		return RecurSpec{Freq: FreqYearly}
	}
	return RecurSpec{}
}

// RecurrenceSummary renders comp's recurrence for the Detail pane: the humanized
// rule, "custom (FREQ=…)" for a kept rule outside the vocabulary, "yes" for
// RDATE-only recurrence or an unparseable rule, and "" when not recurring.
func RecurrenceSummary(comp *ical.Component, anchor time.Time, loc *time.Location) string {
	rule, recurring := recurrenceInfo(comp)
	if !recurring {
		return ""
	}
	if rule == nil {
		return "yes" // RDATE-only recurrence
	}
	if spec, ok := decomposeForSeed(rule, anchor); ok {
		return spec.Humanize(anchor)
	}
	return "custom (" + rule.RRuleString() + ")"
}

// decomposeForSeed decomposes rule at anchor and normalizes a bare weekly rule
// (no BYDAY) to "weekly on the anchor's weekday", so it matches the Weekly preset
// and humanizes with a weekday.
func decomposeForSeed(rule *rrule.ROption, anchor time.Time) (RecurSpec, bool) {
	if rule == nil {
		return RecurSpec{}, false
	}
	spec, ok := RecurSpecFromRule(rule, anchor)
	if !ok {
		return RecurSpec{}, false
	}
	if spec.Freq == FreqWeekly && len(spec.Weekdays) == 0 {
		spec.Weekdays = []time.Weekday{anchor.Weekday()}
	}
	return spec, true
}

// recurrenceInfo returns comp's parsed RRULE (nil when absent/unparseable) and
// whether it recurs at all (RRULE or RDATE present).
func recurrenceInfo(comp *ical.Component) (rule *rrule.ROption, recurring bool) {
	if comp == nil {
		return nil, false
	}
	if r, err := comp.Props.RecurrenceRule(); err == nil {
		rule = r
	}
	hasRDate := len(comp.Props.Values(ical.PropRecurrenceDates)) > 0
	return rule, rule != nil || hasRDate
}

// recurSpecEqual reports whether two specs describe the same rule (the fields
// ROption serializes; the quick-add-only Month/Day anchor is ignored).
func recurSpecEqual(a, b RecurSpec) bool {
	if a.Freq != b.Freq || a.Interval != b.Interval || a.Count != b.Count {
		return false
	}
	if (a.Until == nil) != (b.Until == nil) || (a.Until != nil && !a.Until.Equal(*b.Until)) {
		return false
	}
	if len(a.Weekdays) != len(b.Weekdays) {
		return false
	}
	for i := range a.Weekdays {
		if a.Weekdays[i] != b.Weekdays[i] {
			return false
		}
	}
	if a.MonthlyNth != b.MonthlyNth {
		return false
	}
	return a.MonthlyNth == 0 || a.MonthlyWeekday == b.MonthlyWeekday
}
