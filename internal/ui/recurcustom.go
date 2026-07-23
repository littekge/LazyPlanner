package ui

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/rivo/tview"

	"github.com/littekge/LazyPlanner/internal/model"
)

// mondayOrder lists the weekday checkboxes Monday-first, matching how most
// calendars present a week.
var mondayOrder = [7]time.Weekday{
	time.Monday, time.Tuesday, time.Wednesday, time.Thursday,
	time.Friday, time.Saturday, time.Sunday,
}

// monthlyOption is one entry of the Custom sub-form's "Monthly by" dropdown. nth
// is 0 for by-day-of-month, 1..4 for a numbered weekday, or -1 for the last.
type monthlyOption struct {
	label string
	nth   int
}

// monthlyOptions builds the "Monthly by" choices from the anchor date, so a
// monthly rule can never contradict its start date (Google parity): always
// by-day-of-month, plus the anchor's numbered weekday position (1st–4th) and/or
// "last" when the anchor is the last such weekday of its month.
func monthlyOptions(anchor time.Time) []monthlyOption {
	day := anchor.Day()
	wd := anchor.Weekday().String()
	nth := (day-1)/7 + 1
	daysIn := time.Date(anchor.Year(), anchor.Month()+1, 0, 0, 0, 0, 0, anchor.Location()).Day()
	isLast := day+7 > daysIn

	opts := []monthlyOption{{label: fmt.Sprintf("on day %d", day), nth: 0}}
	if nth <= 4 {
		opts = append(opts, monthlyOption{label: fmt.Sprintf("on the %s %s", ordinalWord(nth), wd), nth: nth})
	}
	if isLast {
		opts = append(opts, monthlyOption{label: "on the last " + wd, nth: -1})
	}
	return opts
}

// ordinalWord renders 1..4 as 1st–4th (the Custom sub-form never needs "last"
// here — that entry is labelled directly).
func ordinalWord(n int) string {
	switch n {
	case 1:
		return "1st"
	case 2:
		return "2nd"
	case 3:
		return "3rd"
	default:
		return fmt.Sprintf("%dth", n)
	}
}

// customRecurFields holds the Custom sub-form's inputs for direct read-back.
type customRecurFields struct {
	every       *tview.InputField
	unit        *tview.DropDown
	strip       *weekdayStrip
	monthly     *tview.DropDown
	monthlyOpts []monthlyOption
	ends        *tview.DropDown
	until       *tview.InputField
	count       *tview.InputField
}

// wireRepeatCustom makes a Repeat dropdown open the Custom… sub-form when its
// Custom… entry is chosen, seeded from the previously-selected option and the
// current anchor. On OK the built spec is written back as the dropdown's selected
// entry; Cancel restores the prior selection. It tracks the last non-Custom
// selection so a cancel (or the initial state) has something to return to.
func (a *app) wireRepeatCustom(dd *tview.DropDown, choices *model.RepeatChoices, anchorFn func() time.Time) {
	prev := choices.Selected()
	var handler func(string, int)
	handler = func(_ string, idx int) {
		if !choices.IsCustom(idx) {
			prev = idx
			return
		}
		anchor := anchorFn()
		seed, ok := choices.SeedSpec(prev, anchor)
		if !ok {
			seed = model.RecurSpec{Freq: model.FreqWeekly, Weekdays: []time.Weekday{anchor.Weekday()}}
		}
		a.openCustomRepeat(seed, anchor, func(spec model.RecurSpec) {
			i := choices.SetCustom(spec, anchor)
			dd.SetOptions(choices.Labels(), handler)
			dd.SetCurrentOption(i) // fires handler with a non-Custom index → no reopen
			prev = i
		}, func() {
			dd.SetCurrentOption(prev)
		})
	}
	dd.SetSelectedFunc(handler)
}

// newCustomRepeatForm builds the Custom repeat field set seeded from seed at
// anchor (no buttons/border — the caller adds those). Split out so it can be
// unit-tested and draw-stressed without the running app. All widgets are built
// once here; layoutCustomRepeat then re-adds only the subset relevant to the
// current Unit/Ends selection, so the widgets (and their values) persist across
// relayouts.
func (a *app) newCustomRepeatForm(seed model.RecurSpec, anchor time.Time) (*caretForm, *customRecurFields) {
	f := newCaretForm()
	cf := &customRecurFields{}

	interval := seed.Interval
	if interval < 1 {
		interval = 1
	}
	cf.every = tview.NewInputField().SetLabel(caretGutter + "Every").SetText(strconv.Itoa(interval)).SetFieldWidth(4)
	cf.unit = newFormDropDown("Unit", []string{"days", "weeks", "months", "years"}, int(seed.Freq))

	cf.strip = newWeekdayStrip("Repeat on")
	if len(seed.Weekdays) > 0 {
		cf.strip.setDays(seed.Weekdays)
	} else {
		cf.strip.setDays([]time.Weekday{anchor.Weekday()}) // same fallback as the read path
	}

	cf.monthlyOpts = monthlyOptions(anchor)
	monLabels := make([]string, len(cf.monthlyOpts))
	for i, o := range cf.monthlyOpts {
		monLabels[i] = o.label
	}
	cf.monthly = newFormDropDown("Monthly by", monLabels, monthlyInitIndex(cf.monthlyOpts, seed))

	endsInit, untilStr, countStr := 0, "", ""
	switch {
	case seed.Until != nil:
		endsInit, untilStr = 1, seed.Until.In(a.loc).Format("2006-01-02")
	case seed.Count > 0:
		endsInit, countStr = 2, strconv.Itoa(seed.Count)
	}
	cf.ends = newFormDropDown("Ends", []string{"Never", "On date", "After N times"}, endsInit)
	cf.until = tview.NewInputField().SetLabel(caretGutter + "Until (YYYY-MM-DD)").SetText(untilStr).SetFieldWidth(12)
	cf.count = tview.NewInputField().SetLabel(caretGutter + "Count").SetText(countStr).SetFieldWidth(6)

	f.stylePopup()

	// Relayout when the frequency or end condition changes; focus stays on the
	// dropdown that triggered it so the cursor doesn't jump. (Wired after the
	// initial options are set above, so this doesn't fire during construction.)
	cf.unit.SetSelectedFunc(func(string, int) { a.layoutCustomRepeat(f, cf, cf.unit) })
	cf.ends.SetSelectedFunc(func(string, int) { a.layoutCustomRepeat(f, cf, cf.ends) })

	a.layoutCustomRepeat(f, cf, nil) // initial layout
	return f, cf
}

// layoutCustomRepeat re-adds only the fields relevant to the current Unit/Ends
// selection (Every, Unit and Ends always; the weekday strip only for weeks;
// "Monthly by" only for months; Until only for "On date"; Count only for
// "After N times"), then restores focus to focusOn (nil → the first field). The
// widgets persist across the clear, so values already entered survive.
func (a *app) layoutCustomRepeat(f *caretForm, cf *customRecurFields, focusOn tview.FormItem) {
	f.clearItems()
	f.addExisting(cf.every, "Every")
	f.addExisting(cf.unit, "Unit")
	switch unitIdx, _ := cf.unit.GetCurrentOption(); unitIdx {
	case 1: // weeks
		f.addExisting(cf.strip, "Repeat on")
	case 2: // months
		f.addExisting(cf.monthly, "Monthly by")
	}
	f.addExisting(cf.ends, "Ends")
	switch endsIdx, _ := cf.ends.GetCurrentOption(); endsIdx {
	case 1: // On date
		f.addExisting(cf.until, "Until (YYYY-MM-DD)")
	case 2: // After N times
		f.addExisting(cf.count, "Count")
	}
	// Restore focus to the field that triggered the relayout.
	for i := 0; i < f.GetFormItemCount(); i++ {
		if f.GetFormItem(i) == focusOn {
			f.focusElement(i)
			return
		}
	}
}

// openCustomRepeat presents the Custom repeat sub-form as a nested modal over the
// item form (the color-picker focus-stack precedent), seeded from seed at anchor.
// onApply fires with the built spec on OK; onCancel on Cancel/Esc.
func (a *app) openCustomRepeat(seed model.RecurSpec, anchor time.Time, onApply func(model.RecurSpec), onCancel func()) {
	f, cf := a.newCustomRepeatForm(seed, anchor)

	f.AddButton("OK", func() {
		spec, err := a.readCustomRecur(cf, anchor)
		if err != nil {
			a.flash(err.Error())
			return
		}
		a.closeModal(pageRepeat)
		onApply(spec)
	})
	f.AddButton("Cancel", func() {
		a.closeModal(pageRepeat)
		onCancel()
	})
	f.SetCancelFunc(func() {
		a.closeModal(pageRepeat)
		onCancel()
	})
	f.SetBorder(true).SetTitle(" Custom repeat ")
	a.openModal(pageRepeat, f, 54, 12)
}

// readCustomRecur reads the sub-form into a RecurSpec, validating the numeric and
// date fields. Inputs irrelevant to the chosen frequency (weekdays unless weekly,
// "Monthly by" unless monthly) are not read — the dynamic form only shows the
// relevant ones.
func (a *app) readCustomRecur(cf *customRecurFields, anchor time.Time) (model.RecurSpec, error) {
	n, err := strconv.Atoi(strings.TrimSpace(cf.every.GetText()))
	if err != nil || n < 1 {
		return model.RecurSpec{}, errFieldMsg("Every must be a whole number ≥ 1")
	}
	unitIdx, _ := cf.unit.GetCurrentOption()
	spec := model.RecurSpec{Freq: model.RecurFreq(unitIdx)}
	if n > 1 {
		spec.Interval = n
	}
	switch spec.Freq {
	case model.FreqWeekly:
		spec.Weekdays = cf.strip.days()
		if len(spec.Weekdays) == 0 {
			spec.Weekdays = []time.Weekday{anchor.Weekday()} // fall back to the start date's weekday
		}
	case model.FreqMonthly:
		if mi, _ := cf.monthly.GetCurrentOption(); mi >= 0 && mi < len(cf.monthlyOpts) {
			if o := cf.monthlyOpts[mi]; o.nth != 0 {
				spec.MonthlyNth, spec.MonthlyWeekday = o.nth, anchor.Weekday()
			}
		}
	}
	switch endsIdx, _ := cf.ends.GetCurrentOption(); endsIdx {
	case 1: // On date
		d, has, err := parseDateField(cf.until.GetText(), a.loc)
		if err != nil || !has {
			return model.RecurSpec{}, errFieldMsg("Ends on date: enter a valid YYYY-MM-DD")
		}
		spec.Until = &d
	case 2: // After N times
		c, err := strconv.Atoi(strings.TrimSpace(cf.count.GetText()))
		if err != nil || c < 1 {
			return model.RecurSpec{}, errFieldMsg("Ends after: count must be ≥ 1")
		}
		spec.Count = c
	}
	return spec, nil
}

// monthlyInitIndex finds the "Monthly by" option matching seed's monthly setting.
func monthlyInitIndex(opts []monthlyOption, seed model.RecurSpec) int {
	for i, o := range opts {
		if o.nth == seed.MonthlyNth {
			return i
		}
	}
	return 0
}
