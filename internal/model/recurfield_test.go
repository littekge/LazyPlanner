package model

import (
	"testing"
	"time"

	"github.com/emersion/go-ical"
)

func compWithRule(t *testing.T, rule string, dtstart time.Time, allDay bool) *ical.Component {
	t.Helper()
	c := ical.NewComponent(ical.CompEvent)
	c.Props.SetText(ical.PropUID, "x")
	setDateOrTime(c, ical.PropDateTimeStart, dtstart, allDay)
	if rule != "" {
		p := ical.NewProp(ical.PropRecurrenceRule)
		p.Value = rule
		c.Props.Set(p)
	}
	return c
}

// TestRepeatChoicesSeeding covers the option list and initial selection the Repeat
// dropdown shows for each rule shape, seeded from the anchor.
func TestRepeatChoicesSeeding(t *testing.T) {
	anchor := time.Date(2026, 8, 25, 0, 0, 0, 0, time.UTC) // a Tuesday, the 25th
	if anchor.Weekday() != time.Tuesday {
		t.Fatal("test setup: anchor not Tuesday")
	}
	basePresets := []string{"None", "Daily", "Weekly on Tue", "Monthly on day 25", "Yearly on Aug 25"}

	t.Run("no rule", func(t *testing.T) {
		rc := NewRepeatChoices(compWithRule(t, "", anchor, false), anchor, time.UTC)
		want := append(append([]string{}, basePresets...), "Custom…")
		if got := rc.Labels(); !equalStrings(got, want) {
			t.Errorf("labels = %v, want %v", got, want)
		}
		if rc.Selected() != 0 {
			t.Errorf("selected = %d, want 0 (None)", rc.Selected())
		}
	})

	presetCases := []struct {
		name string
		rule string
		want int
	}{
		{"daily", "FREQ=DAILY", 1},
		{"weekly byday", "FREQ=WEEKLY;BYDAY=TU", 2},
		{"weekly bare normalizes to preset", "FREQ=WEEKLY", 2},
		{"monthly by day", "FREQ=MONTHLY", 3},
		{"yearly", "FREQ=YEARLY", 4},
	}
	for _, tc := range presetCases {
		t.Run("preset "+tc.name, func(t *testing.T) {
			rc := NewRepeatChoices(compWithRule(t, tc.rule, anchor, false), anchor, time.UTC)
			if rc.Selected() != tc.want {
				t.Errorf("selected = %d (%q), want %d", rc.Selected(), rc.Labels()[rc.Selected()], tc.want)
			}
			if len(rc.Labels()) != 6 { // 5 presets + Custom…
				t.Errorf("labels = %v, want 6 (no extra kept entry)", rc.Labels())
			}
		})
	}

	t.Run("representable non-preset adds a humanized kept entry", func(t *testing.T) {
		rc := NewRepeatChoices(compWithRule(t, "FREQ=WEEKLY;INTERVAL=2;BYDAY=TU,TH", anchor, false), anchor, time.UTC)
		labels := rc.Labels()
		if len(labels) != 7 {
			t.Fatalf("labels = %v, want 7 (kept entry + Custom…)", labels)
		}
		if labels[5] != "every 2 weeks on Tue, Thu" {
			t.Errorf("kept entry = %q, want %q", labels[5], "every 2 weeks on Tue, Thu")
		}
		if rc.Selected() != 5 {
			t.Errorf("selected = %d, want 5 (the kept entry)", rc.Selected())
		}
	})

	t.Run("unrepresentable adds Custom rule (kept)", func(t *testing.T) {
		rc := NewRepeatChoices(compWithRule(t, "FREQ=MONTHLY;BYSETPOS=-1;BYDAY=MO,TU,WE,TH,FR", anchor, false), anchor, time.UTC)
		labels := rc.Labels()
		if labels[len(labels)-2] != "Custom rule (kept)" {
			t.Errorf("kept-raw entry = %q, want %q", labels[len(labels)-2], "Custom rule (kept)")
		}
		if rc.Selected() != len(labels)-2 {
			t.Errorf("selected = %d, want the kept-raw entry", rc.Selected())
		}
	})
}

// TestRepeatChoicesResolve covers mapping a selection + final anchor to a draft's
// Recur/RecurRemove, including rewrite-only-when-changed and preset re-derivation.
func TestRepeatChoicesResolve(t *testing.T) {
	anchor := time.Date(2026, 8, 25, 0, 0, 0, 0, time.UTC) // Tuesday

	noRule := func() *RepeatChoices { return NewRepeatChoices(compWithRule(t, "", anchor, false), anchor, time.UTC) }
	daily := func() *RepeatChoices { return NewRepeatChoices(compWithRule(t, "FREQ=DAILY", anchor, false), anchor, time.UTC) }

	t.Run("create None is inert", func(t *testing.T) {
		recur, remove := noRule().Resolve(0, anchor)
		if recur != nil || remove {
			t.Errorf("got (%v,%v), want (nil,false)", recur, remove)
		}
	})
	t.Run("create pick Daily makes recurring", func(t *testing.T) {
		recur, remove := noRule().Resolve(1, anchor)
		if recur == nil || recur.Freq != FreqDaily || remove {
			t.Errorf("got (%+v,%v), want a daily spec", recur, remove)
		}
	})
	t.Run("existing rule None removes", func(t *testing.T) {
		recur, remove := daily().Resolve(0, anchor)
		if recur != nil || !remove {
			t.Errorf("got (%v,%v), want (nil,true)", recur, remove)
		}
	})
	t.Run("unchanged preset leaves untouched", func(t *testing.T) {
		rc := daily()
		recur, remove := rc.Resolve(rc.Selected(), anchor)
		if recur != nil || remove {
			t.Errorf("got (%v,%v), want (nil,false) — unchanged rule untouched", recur, remove)
		}
	})
	t.Run("changed preset rewrites", func(t *testing.T) {
		recur, remove := daily().Resolve(2, anchor) // switch Daily → Weekly
		if recur == nil || recur.Freq != FreqWeekly || remove {
			t.Errorf("got (%+v,%v), want a weekly rewrite", recur, remove)
		}
	})
	t.Run("preset re-derives on a changed anchor", func(t *testing.T) {
		weekly := NewRepeatChoices(compWithRule(t, "FREQ=WEEKLY;BYDAY=TU", anchor, false), anchor, time.UTC)
		wed := time.Date(2026, 8, 26, 0, 0, 0, 0, time.UTC) // a Wednesday
		recur, _ := weekly.Resolve(weekly.Selected(), wed)  // same Weekly preset, new anchor
		if recur == nil || len(recur.Weekdays) != 1 || recur.Weekdays[0] != time.Wednesday {
			t.Errorf("got %+v, want a rewrite to Weekly on Wed (re-derived)", recur)
		}
	})
	t.Run("kept-raw preserved when left selected", func(t *testing.T) {
		rc := NewRepeatChoices(compWithRule(t, "FREQ=MONTHLY;BYSETPOS=-1;BYDAY=MO", anchor, false), anchor, time.UTC)
		recur, remove := rc.Resolve(rc.Selected(), anchor)
		if recur != nil || remove {
			t.Errorf("got (%v,%v), want (nil,false) — kept rule preserved", recur, remove)
		}
	})
}

// TestRepeatChoicesCustom covers the Custom… sub-form support: SetCustom adds a
// selectable humanized entry that Resolve treats as a rewrite, replacing any prior
// custom entry rather than duplicating; SeedSpec returns the spec to seed the
// sub-form from the current selection.
func TestRepeatChoicesCustom(t *testing.T) {
	anchor := time.Date(2026, 8, 25, 0, 0, 0, 0, time.UTC) // Tuesday

	t.Run("set custom on a fresh form rewrites", func(t *testing.T) {
		rc := NewRepeatChoices(compWithRule(t, "", anchor, false), anchor, time.UTC)
		spec := RecurSpec{Freq: FreqWeekly, Interval: 2, Weekdays: []time.Weekday{time.Tuesday, time.Thursday}}
		idx := rc.SetCustom(spec, anchor)
		if rc.Labels()[idx] != "every 2 weeks on Tue, Thu" {
			t.Errorf("label = %q", rc.Labels()[idx])
		}
		if rc.Selected() != idx {
			t.Errorf("selected = %d, want %d", rc.Selected(), idx)
		}
		recur, remove := rc.Resolve(idx, anchor)
		if recur == nil || recur.Interval != 2 || remove {
			t.Errorf("resolve = (%+v,%v), want the custom spec", recur, remove)
		}
	})

	t.Run("set custom twice replaces, no duplicate", func(t *testing.T) {
		rc := NewRepeatChoices(compWithRule(t, "", anchor, false), anchor, time.UTC)
		n0 := len(rc.Labels())
		rc.SetCustom(RecurSpec{Freq: FreqDaily, Interval: 3}, anchor)
		n1 := len(rc.Labels())
		rc.SetCustom(RecurSpec{Freq: FreqDaily, Interval: 4}, anchor)
		n2 := len(rc.Labels())
		if n1 != n0+1 || n2 != n1 {
			t.Errorf("lengths %d→%d→%d, want one added then replaced", n0, n1, n2)
		}
		if rc.Labels()[rc.Selected()] != "every 4 days" {
			t.Errorf("selected label = %q, want every 4 days", rc.Labels()[rc.Selected()])
		}
	})

	t.Run("custom equal to the seeded rule is untouched", func(t *testing.T) {
		rc := NewRepeatChoices(compWithRule(t, "FREQ=WEEKLY;INTERVAL=2;BYDAY=TU,TH", anchor, false), anchor, time.UTC)
		same := RecurSpec{Freq: FreqWeekly, Interval: 2, Weekdays: []time.Weekday{time.Tuesday, time.Thursday}}
		idx := rc.SetCustom(same, anchor)
		recur, remove := rc.Resolve(idx, anchor)
		if recur != nil || remove {
			t.Errorf("resolve = (%+v,%v), want untouched (unchanged rule)", recur, remove)
		}
	})

	t.Run("seed spec from a preset selection", func(t *testing.T) {
		rc := NewRepeatChoices(compWithRule(t, "FREQ=WEEKLY;BYDAY=TU", anchor, false), anchor, time.UTC)
		spec, ok := rc.SeedSpec(rc.Selected(), anchor)
		if !ok || spec.Freq != FreqWeekly {
			t.Errorf("SeedSpec = (%+v,%v), want a weekly spec", spec, ok)
		}
	})
	t.Run("seed spec from None is not ok", func(t *testing.T) {
		rc := NewRepeatChoices(compWithRule(t, "", anchor, false), anchor, time.UTC)
		if _, ok := rc.SeedSpec(0, anchor); ok {
			t.Error("SeedSpec(None) ok, want false")
		}
	})
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
