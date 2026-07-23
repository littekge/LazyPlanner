package ui

import (
	"testing"
	"time"

	"github.com/littekge/LazyPlanner/internal/model"
)

var customAnchor = time.Date(2026, 8, 25, 0, 0, 0, 0, time.UTC) // Tuesday, 4th & last of Aug

// TestMonthlyOptions verifies the anchor-derived "Monthly by" choices: day-of-
// month always, the numbered weekday, and "last" when the anchor is the last.
func TestMonthlyOptions(t *testing.T) {
	opts := monthlyOptions(customAnchor)
	want := []string{"on day 25", "on the 4th Tuesday", "on the last Tuesday"}
	if len(opts) != len(want) {
		t.Fatalf("options = %v, want %v", opts, want)
	}
	for i, o := range opts {
		if o.label != want[i] {
			t.Errorf("option %d = %q, want %q", i, o.label, want[i])
		}
	}
	// A mid-month weekday (not last) offers only day + numbered.
	midTue := time.Date(2026, 12, 8, 0, 0, 0, 0, time.UTC) // 2nd Tuesday of Dec 2026
	if got := monthlyOptions(midTue); len(got) != 2 || got[1].nth != 2 {
		t.Errorf("mid-month options = %v, want [day, 2nd Tuesday]", got)
	}
}

// TestReadCustomRecur covers reading the sub-form into a RecurSpec per frequency
// and end condition.
func TestReadCustomRecur(t *testing.T) {
	a := newTestApp(t, time.Date(2026, 8, 1, 9, 0, 0, 0, time.UTC))

	t.Run("weekly interval + day set", func(t *testing.T) {
		_, cf := a.newCustomRepeatForm(model.RecurSpec{}, customAnchor)
		cf.every.SetText("2")
		cf.unit.SetCurrentOption(1) // weeks
		cf.strip.setDays([]time.Weekday{time.Tuesday, time.Thursday})
		spec, err := a.readCustomRecur(cf, customAnchor)
		if err != nil {
			t.Fatal(err)
		}
		if spec.Freq != model.FreqWeekly || spec.Interval != 2 {
			t.Errorf("got %+v, want weekly interval 2", spec)
		}
		if len(spec.Weekdays) != 2 || spec.Weekdays[0] != time.Tuesday || spec.Weekdays[1] != time.Thursday {
			t.Errorf("weekdays = %v, want [Tue Thu]", spec.Weekdays)
		}
	})

	t.Run("weekly no days falls back to anchor", func(t *testing.T) {
		_, cf := a.newCustomRepeatForm(model.RecurSpec{}, customAnchor)
		cf.every.SetText("1")
		cf.unit.SetCurrentOption(1)
		spec, err := a.readCustomRecur(cf, customAnchor)
		if err != nil {
			t.Fatal(err)
		}
		if len(spec.Weekdays) != 1 || spec.Weekdays[0] != time.Tuesday {
			t.Errorf("weekdays = %v, want [Tuesday] (anchor fallback)", spec.Weekdays)
		}
	})

	t.Run("monthly nth weekday", func(t *testing.T) {
		_, cf := a.newCustomRepeatForm(model.RecurSpec{}, customAnchor)
		cf.every.SetText("1")
		cf.unit.SetCurrentOption(2)    // months
		cf.monthly.SetCurrentOption(1) // "on the 4th Tuesday"
		spec, err := a.readCustomRecur(cf, customAnchor)
		if err != nil {
			t.Fatal(err)
		}
		if spec.MonthlyNth != 4 || spec.MonthlyWeekday != time.Tuesday {
			t.Errorf("got %+v, want the 4th Tuesday", spec)
		}
	})

	t.Run("ends on date", func(t *testing.T) {
		_, cf := a.newCustomRepeatForm(model.RecurSpec{}, customAnchor)
		cf.every.SetText("1")
		cf.unit.SetCurrentOption(0) // days
		cf.ends.SetCurrentOption(1) // On date
		cf.until.SetText("2026-12-12")
		spec, err := a.readCustomRecur(cf, customAnchor)
		if err != nil {
			t.Fatal(err)
		}
		if spec.Until == nil || spec.Until.Year() != 2026 || spec.Until.Month() != time.December {
			t.Errorf("Until = %v, want Dec 2026", spec.Until)
		}
	})

	t.Run("ends after N", func(t *testing.T) {
		_, cf := a.newCustomRepeatForm(model.RecurSpec{}, customAnchor)
		cf.every.SetText("1")
		cf.unit.SetCurrentOption(0)
		cf.ends.SetCurrentOption(2) // After N times
		cf.count.SetText("10")
		spec, err := a.readCustomRecur(cf, customAnchor)
		if err != nil {
			t.Fatal(err)
		}
		if spec.Count != 10 {
			t.Errorf("Count = %d, want 10", spec.Count)
		}
	})
}

// TestReadCustomRecurValidation covers the rejected inputs.
func TestReadCustomRecurValidation(t *testing.T) {
	a := newTestApp(t, time.Date(2026, 8, 1, 9, 0, 0, 0, time.UTC))
	cases := []struct {
		name  string
		setup func(cf *customRecurFields)
	}{
		{"every not a number", func(cf *customRecurFields) { cf.every.SetText("x") }},
		{"every zero", func(cf *customRecurFields) { cf.every.SetText("0") }},
		{"bad until", func(cf *customRecurFields) {
			cf.every.SetText("1")
			cf.ends.SetCurrentOption(1)
			cf.until.SetText("nope")
		}},
		{"count zero", func(cf *customRecurFields) {
			cf.every.SetText("1")
			cf.ends.SetCurrentOption(2)
			cf.count.SetText("0")
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, cf := a.newCustomRepeatForm(model.RecurSpec{}, customAnchor)
			cf.every.SetText("1")
			tc.setup(cf)
			if _, err := a.readCustomRecur(cf, customAnchor); err == nil {
				t.Error("want a validation error, got nil")
			}
		})
	}
}

// TestCustomRepeatDrawStress draws the sub-form across hostile geometries — a
// panic or hang in any Draw path would crash the live app (the display-stress
// guardrail).
func TestCustomRepeatDrawStress(t *testing.T) {
	a := newTestApp(t, time.Date(2026, 8, 1, 9, 0, 0, 0, time.UTC))
	seeds := []model.RecurSpec{
		{Freq: model.FreqWeekly, Interval: 2, Weekdays: []time.Weekday{time.Tuesday, time.Thursday}},
		{Freq: model.FreqMonthly, MonthlyNth: -1, MonthlyWeekday: time.Tuesday},
		{Freq: model.FreqYearly},
	}
	for _, s := range seeds {
		f, _ := a.newCustomRepeatForm(s, customAnchor)
		for _, g := range stressGeoms {
			drawGeom(t, "custom-repeat", f, g.w, g.h)
		}
	}

	// Drawing after a relayout (Unit + Ends changes) must also be panic-free.
	f, cf := a.newCustomRepeatForm(model.RecurSpec{Freq: model.FreqWeekly}, customAnchor)
	cf.unit.SetCurrentOption(2) // → months (relayout)
	cf.ends.SetCurrentOption(1) // → On date (relayout)
	for _, g := range stressGeoms {
		drawGeom(t, "custom-repeat-relaid", f, g.w, g.h)
	}
}

// TestCustomRepeatFocusStack verifies the sub-form nests over the item form and
// unwinds the focus stack cleanly on close (the nested-modal invariant).
func TestCustomRepeatFocusStack(t *testing.T) {
	a := newRootedTestApp(t, time.Date(2026, 8, 1, 9, 0, 0, 0, time.UTC))
	before := len(a.focusStack)
	a.openCustomRepeat(model.RecurSpec{Freq: model.FreqWeekly, Weekdays: []time.Weekday{time.Tuesday}},
		customAnchor, func(model.RecurSpec) {}, func() {})
	if !a.modalOpen() {
		t.Fatal("sub-form did not open")
	}
	if name, _ := a.root.GetFrontPage(); name != pageRepeat {
		t.Errorf("front page = %q, want %q", name, pageRepeat)
	}
	if len(a.focusStack) != before+1 {
		t.Errorf("focus stack = %d, want %d (pushed)", len(a.focusStack), before+1)
	}
	a.closeModal(pageRepeat)
	if len(a.focusStack) != before {
		t.Errorf("focus stack = %d, want %d (popped)", len(a.focusStack), before)
	}
}

// TestCustomRepeatRelayout: changing Unit/Ends shows only the relevant fields and
// preserves values already entered in fields that stay visible.
func TestCustomRepeatRelayout(t *testing.T) {
	a := newTestApp(t, time.Date(2026, 8, 1, 9, 0, 0, 0, time.UTC))

	// labelsOf returns the base labels currently laid out in the form.
	labelsOf := func(f *caretForm) []string {
		out := make([]string, len(f.labels))
		copy(out, f.labels)
		return out
	}
	has := func(f *caretForm, want string) bool {
		for _, l := range f.labels {
			if l == want {
				return true
			}
		}
		return false
	}

	f, cf := a.newCustomRepeatForm(model.RecurSpec{Freq: model.FreqWeekly}, customAnchor)
	// Weekly: the strip shows, "Monthly by" does not.
	if !has(f, "Repeat on") || has(f, "Monthly by") {
		t.Fatalf("weekly layout = %v, want the strip and no Monthly by", labelsOf(f))
	}
	cf.every.SetText("3") // a value that must survive the relayout

	// Switch Unit → months: strip hides, "Monthly by" shows, Every preserved.
	cf.unit.SetCurrentOption(2)
	if has(f, "Repeat on") || !has(f, "Monthly by") {
		t.Fatalf("monthly layout = %v, want Monthly by and no strip", labelsOf(f))
	}
	if got := cf.every.GetText(); got != "3" {
		t.Errorf("Every = %q after relayout, want preserved %q", got, "3")
	}

	// Ends → On date reveals Until; → After N reveals Count.
	cf.ends.SetCurrentOption(1)
	if !has(f, "Until (YYYY-MM-DD)") || has(f, "Count") {
		t.Errorf("ends=on-date layout = %v, want Until and no Count", labelsOf(f))
	}
	cf.ends.SetCurrentOption(2)
	if !has(f, "Count") || has(f, "Until (YYYY-MM-DD)") {
		t.Errorf("ends=after-N layout = %v, want Count and no Until", labelsOf(f))
	}
}

// TestCustomRepeatDailyIsMinimal: a daily/never rule shows only Every, Unit, Ends.
func TestCustomRepeatDailyIsMinimal(t *testing.T) {
	a := newTestApp(t, time.Date(2026, 8, 1, 9, 0, 0, 0, time.UTC))
	f, _ := a.newCustomRepeatForm(model.RecurSpec{Freq: model.FreqDaily}, customAnchor)
	if f.GetFormItemCount() != 3 {
		t.Errorf("daily/never form has %d fields (%v), want 3 (Every, Unit, Ends)", f.GetFormItemCount(), f.labels)
	}
}
