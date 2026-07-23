# Custom recurrence sub-form redesign — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make the Custom… repeat sub-form lighter — show only fields relevant to the current Unit/Ends selection, and replace the 7 weekday checkboxes with a single compact toggle strip.

**Architecture:** A new `weekdayStrip` custom widget implements `tview.FormItem` so it lives inside the existing `caretForm` and is drilled into via the app-wide NORMAL/DRILL model. The `caretForm` gains small helpers to support a persistent-widget set that is re-laid-out (Clear + re-add the relevant subset) when Unit or Ends changes. The recurrence model layer is untouched — this is a UI-layer redesign.

**Tech Stack:** Go, `rivo/tview` (`FormItem`, `Box`, `Form`, `DropDown`, `InputField`), `github.com/gdamore/tcell/v2`. Standard `testing`, table-driven.

## Global Constraints

- Only `internal/ui` imports tview/tcell. This work is entirely inside `internal/ui`; no `store`/`model` changes.
- `gofmt` is law; `goimports` ordering (stdlib → third-party → project). No new dependencies.
- Full gate green before every commit: `go test ./...`, `go vet ./...`, `staticcheck ./...` (at `~/go/bin` — `export PATH=$PATH:~/go/bin` if not found), `go build ./...`.
- Repro-first (TDD): failing test → minimal implementation → green; tests stay as regression guards.
- Comments explain *why*, not *what*. Check every error. No magic numbers (named constants for meaning).
- **selectionStyle guardrail:** every selectable widget must carry the theme-adaptive `selectionStyle` (`tcell.StyleDefault.Reverse(true)`) — this includes the strip's selected cells and every `tview.DropDown` (via `SetListStyles(tcell.StyleDefault, selectionStyle)`). This class has shipped-broken twice; a new selectable widget gets a reverse-video regression test.
- **Display-stress guardrail:** every custom `Draw` path is drawn across `stressGeoms` (1×1→400×150) under the panic-recover + watchdog harness (`drawGeom`), so a new freeze/panic is caught on the normal gate.
- **Nested-modal focus guardrail:** the sub-form opens over the item form via `pageRepeat`; OK/Cancel/Esc must restore focus to the item form, not the calendar. Do not change `captureFocus`/`restoreFocus`.
- Existing test helpers to reuse (do not redefine): `newTestApp(t, now)`, `newRootedTestApp(t, now)`, `runeKey(r rune)`, `keyEv(k tcell.Key)`, `drawGeom(t, label, prim, w, h)`, `stressGeoms`.
- Existing constants (verbatim): `caretGutter = "  "`, `caretMarker = "▸ "`, `selectionStyle = tcell.StyleDefault.Reverse(true)`, `accentColor = tcell.ColorTeal`. `mondayOrder` is `[7]time.Weekday{Mon..Sun}` in `recurcustom.go`.
- `tview.Print(screen, text, x, y, maxWidth, align, color) (int, int)` returns `(runeCount, drawnScreenWidth)` — use the **second** value to advance x.
- `tview.Form.Clear(includeButtons bool)`: `Clear(false)` clears form items, keeps buttons.

---

## Task 1: The weekdayStrip widget

**Files:**
- Create: `internal/ui/weekdaystrip.go`
- Test: `internal/ui/weekdaystrip_test.go`

**Interfaces:**
- Consumes: `mondayOrder` (`recurcustom.go`), `selectionStyle`, `accentColor`, `caretGutter` (`app.go`/`forms.go`).
- Produces:
  - `type weekdayStrip struct{ ... }` implementing `tview.FormItem`.
  - `func newWeekdayStrip(label string) *weekdayStrip`
  - `func (w *weekdayStrip) SetLabel(label string) *weekdayStrip`
  - `func (w *weekdayStrip) setDays(days []time.Weekday)` — seed selected set (Monday-first).
  - `func (w *weekdayStrip) days() []time.Weekday` — selected days, Monday-first.
  - FormItem methods: `GetLabel`, `SetFormAttributes`, `GetFieldWidth`, `GetFieldHeight`, `SetFinishedFunc`, `SetDisabled`, `Draw`, `InputHandler` (cursor `←`/`→`/`h`/`l`, Space toggles).

- [ ] **Step 1: Write the failing test**

Create `internal/ui/weekdaystrip_test.go`:

```go
package ui

import (
	"testing"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

func TestWeekdayStripSeedAndRead(t *testing.T) {
	w := newWeekdayStrip("Repeat on")
	w.setDays([]time.Weekday{time.Tuesday, time.Thursday})
	got := w.days()
	if len(got) != 2 || got[0] != time.Tuesday || got[1] != time.Thursday {
		t.Fatalf("days() = %v, want [Tue Thu] (Monday-first order)", got)
	}
}

func TestWeekdayStripToggleAndCursor(t *testing.T) {
	w := newWeekdayStrip("Repeat on")
	w.setDays(nil)
	handler := w.InputHandler()
	noFocus := func(tview.Primitive) {}

	// Cursor starts at 0 (Mon). Right twice → Wed (index 2), Space toggles it on.
	handler(keyEv(tcell.KeyRight), noFocus)
	handler(keyEv(tcell.KeyRight), noFocus)
	handler(runeKey(' '), noFocus)
	if got := w.days(); len(got) != 1 || got[0] != time.Wednesday {
		t.Fatalf("after Right,Right,Space days() = %v, want [Wed]", got)
	}

	// Space again toggles it back off.
	handler(runeKey(' '), noFocus)
	if got := w.days(); len(got) != 0 {
		t.Fatalf("after second Space days() = %v, want []", got)
	}

	// Cursor clamps at the left edge.
	handler(keyEv(tcell.KeyLeft), noFocus)
	handler(keyEv(tcell.KeyLeft), noFocus)
	handler(keyEv(tcell.KeyLeft), noFocus)
	handler(runeKey(' '), noFocus)
	if got := w.days(); len(got) != 1 || got[0] != time.Monday {
		t.Fatalf("after clamping left + Space days() = %v, want [Mon]", got)
	}
}

func TestWeekdayStripSelectionIsLegible(t *testing.T) {
	w := newWeekdayStrip("Repeat on")
	w.setDays([]time.Weekday{time.Tuesday})
	w.SetRect(0, 0, 40, 1)
	screen := tcell.NewSimulationScreen("")
	if err := screen.Init(); err != nil {
		t.Fatalf("init screen: %v", err)
	}
	defer screen.Fini()
	screen.SetSize(40, 1)
	w.Draw(screen)
	screen.Show()
	cells, _, _ := screen.GetContents()
	for _, c := range cells {
		if _, _, attr := c.Style.Decompose(); attr&tcell.AttrReverse != 0 {
			return // a reverse-video cell means the selected day is legible
		}
	}
	t.Error("no reverse-video cell — selected day is illegible on terminal-default themes")
}

func TestWeekdayStripDrawStress(t *testing.T) {
	w := newWeekdayStrip("Repeat on")
	w.setDays([]time.Weekday{time.Monday, time.Wednesday, time.Friday})
	for _, g := range stressGeoms {
		drawGeom(t, "weekday-strip", w, g.w, g.h)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/ui/ -run TestWeekdayStrip`
Expected: FAIL — build error `undefined: newWeekdayStrip`.

- [ ] **Step 3: Write the widget**

Create `internal/ui/weekdaystrip.go`:

```go
package ui

import (
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// dayAbbrevs labels the strip cells Monday-first, matching mondayOrder.
var dayAbbrevs = [7]string{"Mon", "Tue", "Wed", "Thu", "Fri", "Sat", "Sun"}

// weekdayStrip is a single-row form item for selecting a set of weekdays: seven
// day cells navigated with ←/→ (or h/l) and toggled with Space. It implements
// tview.FormItem so it lives inside a caretForm like any field and is drilled
// into via the app-wide NORMAL/DRILL model (Enter drills, arrows/Space act while
// drilled, Esc leaves). It replaces the seven separate checkboxes the Custom
// repeat form used to show — one field to land on instead of seven.
type weekdayStrip struct {
	*tview.Box
	label      string
	labelWidth int
	labelColor tcell.Color
	selected   [7]bool // index 0..6 == mondayOrder (Monday..Sunday)
	cursor     int      // focused day cell, 0..6
	finished   func(tcell.Key)
	disabled   bool
}

func newWeekdayStrip(label string) *weekdayStrip {
	return &weekdayStrip{Box: tview.NewBox(), label: label, labelColor: tcell.ColorDefault}
}

// SetLabel sets the label text (the caretForm prepends the ▸ gutter in Draw).
func (w *weekdayStrip) SetLabel(label string) *weekdayStrip {
	w.label = label
	return w
}

// setDays seeds the selected set from a weekday list.
func (w *weekdayStrip) setDays(days []time.Weekday) {
	w.selected = [7]bool{}
	for _, d := range days {
		for i, wd := range mondayOrder {
			if wd == d {
				w.selected[i] = true
			}
		}
	}
}

// days returns the selected weekdays in Monday-first order.
func (w *weekdayStrip) days() []time.Weekday {
	var out []time.Weekday
	for i, on := range w.selected {
		if on {
			out = append(out, mondayOrder[i])
		}
	}
	return out
}

// --- tview.FormItem ---

func (w *weekdayStrip) GetLabel() string { return w.label }

func (w *weekdayStrip) SetFormAttributes(labelWidth int, labelColor, bgColor, _, _ tcell.Color) tview.FormItem {
	w.labelWidth = labelWidth
	w.labelColor = labelColor
	w.SetBackgroundColor(bgColor)
	return w
}

func (w *weekdayStrip) GetFieldWidth() int  { return 0 } // flexible: uses the available field area
func (w *weekdayStrip) GetFieldHeight() int { return 1 }

func (w *weekdayStrip) SetFinishedFunc(handler func(key tcell.Key)) tview.FormItem {
	w.finished = handler
	return w
}

func (w *weekdayStrip) SetDisabled(disabled bool) tview.FormItem {
	w.disabled = disabled
	return w
}

func (w *weekdayStrip) Draw(screen tcell.Screen) {
	w.Box.DrawForSubclass(screen, w)
	x, y, width, height := w.GetInnerRect()
	if height < 1 || width <= 0 {
		return
	}
	// Label (the caretForm's Draw has prepended the ▸/space gutter into w.label).
	if w.labelWidth > 0 {
		lw := w.labelWidth
		if lw > width {
			lw = width
		}
		tview.Print(screen, w.label, x, y, lw, tview.AlignLeft, w.labelColor)
		x += lw
		width -= lw
	} else {
		_, drawn := tview.Print(screen, w.label, x, y, width, tview.AlignLeft, w.labelColor)
		x += drawn
		width -= drawn
	}
	// Day cells. A selected day is reverse-video (selectionStyle — the theme-
	// adaptive legibility guardrail); the focused cell is underlined in the accent
	// color so "which day is focused" reads apart from "which days are on".
	focused := w.HasFocus()
	for i := 0; i < 7; i++ {
		style := tcell.StyleDefault
		if w.selected[i] {
			style = selectionStyle
		}
		if focused && i == w.cursor {
			style = style.Underline(true).Foreground(accentColor)
		}
		for _, r := range dayAbbrevs[i] + " " {
			if width <= 0 {
				return
			}
			screen.SetContent(x, y, r, nil, style)
			x++
			width--
		}
	}
}

func (w *weekdayStrip) InputHandler() func(*tcell.EventKey, func(tview.Primitive)) {
	return w.WrapInputHandler(func(ev *tcell.EventKey, _ func(tview.Primitive)) {
		if w.disabled {
			return
		}
		switch ev.Key() {
		case tcell.KeyLeft:
			w.moveCursor(-1)
		case tcell.KeyRight:
			w.moveCursor(+1)
		case tcell.KeyRune:
			switch ev.Rune() {
			case ' ':
				w.selected[w.cursor] = !w.selected[w.cursor]
			case 'h':
				w.moveCursor(-1)
			case 'l':
				w.moveCursor(+1)
			}
		}
	})
}

// moveCursor shifts the day cursor by delta, clamped to the seven cells.
func (w *weekdayStrip) moveCursor(delta int) {
	w.cursor += delta
	if w.cursor < 0 {
		w.cursor = 0
	}
	if w.cursor > 6 {
		w.cursor = 6
	}
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/ui/ -run TestWeekdayStrip -v`
Expected: PASS (all four tests).

- [ ] **Step 5: Full gate + commit**

```bash
export PATH=$PATH:~/go/bin
go test ./... && go vet ./... && staticcheck ./... && go build ./...
git add internal/ui/weekdaystrip.go internal/ui/weekdaystrip_test.go
git commit -m "feat: weekdayStrip form-item widget (compact weekday toggle)

Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>"
```

---

## Task 2: caretForm support for the strip and dynamic relayout

**Files:**
- Modify: `internal/ui/forms.go`
- Test: `internal/ui/formnav_test.go` (append)

**Interfaces:**
- Consumes: `weekdayStrip` (Task 1), existing `caretForm` internals (`labels`, `isTextField`, `actNormal`, `moveFocus`, `Draw`, `addDropDown`).
- Produces:
  - `func newFormDropDown(label string, options []string, initial int) *tview.DropDown` — builds a dropdown already carrying `selectionStyle` (centralizes the legibility guardrail); `addDropDown` is refactored to use it.
  - `func (f *caretForm) clearItems()` — `Clear(false)` + reset the `labels` slice.
  - `func (f *caretForm) addExisting(item tview.FormItem, base string)` — re-add a pre-built item under base label (for relayout).
  - `func (f *caretForm) isDrillable(i int) bool` — text field OR weekdayStrip; `moveFocus` auto-drill now uses this.
  - `actNormal` gains a `*weekdayStrip` case (Enter → drill), and the Draw gutter type-switch gains a `*weekdayStrip` case.

- [ ] **Step 1: Write the failing test**

Append to `internal/ui/formnav_test.go`:

```go
// TestFormStripDrillsAndToggles: a weekdayStrip in a caretForm is drilled into by
// Enter in NORMAL, then Space toggles the focused day and Esc returns to NORMAL —
// proving the strip participates in the shared DRILL model.
func TestFormStripDrillsAndToggles(t *testing.T) {
	a := newRootedTestApp(t, time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC))
	f := newCaretForm()
	strip := newWeekdayStrip("Repeat on")
	strip.setDays(nil)
	f.AddFormItem(strip)
	f.labels = append(f.labels, "Repeat on")
	f.stylePopup()
	a.openModal(pageForm, f, 50, 6)

	var focus func(tview.Primitive)
	focus = func(p tview.Primitive) { p.Focus(focus) }
	focus(f)

	// NORMAL → Enter drills into the strip.
	f.InputHandler()(keyEv(tcell.KeyEnter), focus)
	if !f.drilled {
		t.Fatal("Enter did not drill into the strip")
	}
	// DRILL → Right moves the cursor to Tue, Space toggles it on.
	f.InputHandler()(keyEv(tcell.KeyRight), focus)
	f.InputHandler()(runeKey(' '), focus)
	if got := strip.days(); len(got) != 1 || got[0] != time.Tuesday {
		t.Fatalf("drilled Right+Space gave days() = %v, want [Tue]", got)
	}
	// Esc returns to NORMAL keeping the selection.
	f.InputHandler()(keyEv(tcell.KeyEscape), focus)
	if f.drilled {
		t.Error("Esc did not return to NORMAL")
	}
	if got := strip.days(); len(got) != 1 || got[0] != time.Tuesday {
		t.Errorf("selection lost on Esc: days() = %v, want [Tue]", got)
	}
	a.closeModal(pageForm)
}

// TestCaretFormClearAndReadd: clearItems empties the item list + labels, addExisting
// re-adds a pre-built item, keeping labels in sync (the relayout primitive).
func TestCaretFormClearAndReadd(t *testing.T) {
	f := newCaretForm()
	a := f.addInput("Alpha", "1", 4)
	f.addInput("Beta", "2", 4)
	if f.GetFormItemCount() != 2 || len(f.labels) != 2 {
		t.Fatalf("setup: items=%d labels=%d, want 2/2", f.GetFormItemCount(), len(f.labels))
	}
	f.clearItems()
	if f.GetFormItemCount() != 0 || len(f.labels) != 0 {
		t.Fatalf("after clearItems: items=%d labels=%d, want 0/0", f.GetFormItemCount(), len(f.labels))
	}
	f.addExisting(a, "Alpha")
	if f.GetFormItemCount() != 1 || len(f.labels) != 1 || f.labels[0] != "Alpha" {
		t.Fatalf("after addExisting: items=%d labels=%v, want 1/[Alpha]", f.GetFormItemCount(), f.labels)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/ui/ -run 'TestFormStripDrillsAndToggles|TestCaretFormClearAndReadd'`
Expected: FAIL — `f.clearItems undefined` / `f.addExisting undefined`, and the strip Enter case not drilling.

- [ ] **Step 3: Add the caretForm helpers and strip cases**

In `internal/ui/forms.go`:

(a) Add `newFormDropDown` and refactor `addDropDown` to use it. Replace the existing `addDropDown` body:

```go
// newFormDropDown builds a dropdown already carrying the theme-adaptive selected
// style. Centralized so every dropdown (created directly for relayout, or via
// addDropDown) is legible on the terminal-default background — the selectionStyle
// guardrail that has shipped broken twice.
func newFormDropDown(label string, options []string, initial int) *tview.DropDown {
	dd := tview.NewDropDown().SetLabel(caretGutter+label).SetOptions(options, nil).SetCurrentOption(initial)
	dd.SetListStyles(tcell.StyleDefault, selectionStyle)
	return dd
}

func (f *caretForm) addDropDown(label string, options []string, initial int) *tview.DropDown {
	dd := newFormDropDown(label, options, initial)
	f.AddFormItem(dd)
	f.labels = append(f.labels, label)
	return dd
}
```

(b) Add the relayout helpers (near the other `caretForm` methods):

```go
// clearItems removes all form items (keeping buttons) and resets the caret-gutter
// label slice, so a relayout can re-add a different subset of the same widgets.
func (f *caretForm) clearItems() {
	f.Clear(false)
	f.labels = f.labels[:0]
}

// addExisting re-adds a pre-built form item under base label (used for dynamic
// relayout, where widgets persist across clearItems). Dropdowns passed here must
// already carry selectionStyle (build them with newFormDropDown).
func (f *caretForm) addExisting(item tview.FormItem, base string) {
	f.AddFormItem(item)
	f.labels = append(f.labels, base)
}
```

(c) Add `isDrillable` and switch `moveFocus`'s auto-drill to it:

```go
// isDrillable reports whether NORMAL Enter (and Enter-advance auto-drill) should
// drill into the element at linear index i: a text field or the weekday strip.
func (f *caretForm) isDrillable(i int) bool {
	if f.isTextField(i) {
		return true
	}
	if i < 0 || i >= f.GetFormItemCount() {
		return false
	}
	_, ok := f.GetFormItem(i).(*weekdayStrip)
	return ok
}
```

In `moveFocus`, change the last line from `f.setDrilled(autoDrill && f.isTextField(i))` to:

```go
	f.setDrilled(autoDrill && f.isDrillable(i))
```

(d) In `actNormal`, add a `*weekdayStrip` case to the type-switch (drill like a text field):

```go
	case *weekdayStrip:
		f.setDrilled(true)
		return nil
```

(e) In `caretForm.Draw`, add a `*weekdayStrip` case to the gutter type-switch (alongside InputField/DropDown/Checkbox):

```go
		case *weekdayStrip:
			it.SetLabel(gutter + base)
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/ui/ -run 'TestFormStripDrillsAndToggles|TestCaretFormClearAndReadd' -v`
Expected: PASS. Also run the existing form-nav suite to confirm no regression:
Run: `go test ./internal/ui/ -run 'TestForm|TestDropDownSelectionIsLegible'`
Expected: PASS.

- [ ] **Step 5: Full gate + commit**

```bash
export PATH=$PATH:~/go/bin
go test ./... && go vet ./... && staticcheck ./... && go build ./...
git add internal/ui/forms.go internal/ui/formnav_test.go
git commit -m "feat: caretForm supports weekdayStrip drill + relayout primitives

Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>"
```

---

## Task 3: Rework the Custom repeat form to be dynamic

**Files:**
- Modify: `internal/ui/recurcustom.go`
- Test: `internal/ui/recurcustom_test.go`

**Interfaces:**
- Consumes: `weekdayStrip`/`newWeekdayStrip`/`setDays`/`days` (Task 1); `newFormDropDown`, `clearItems`, `addExisting` (Task 2); existing `monthlyOptions`, `monthlyInitIndex`, `readCustomRecur`, `openCustomRepeat`, `wireRepeatCustom`.
- Produces:
  - `customRecurFields.days [7]*tview.Checkbox` replaced by `strip *weekdayStrip`.
  - `func (a *app) layoutCustomRepeat(f *caretForm, cf *customRecurFields, focusOn tview.FormItem)` — re-adds only the visible subset, then focuses `focusOn` (nil → first field).
  - `newCustomRepeatForm` builds all widgets once, wires Unit/Ends relayout, and calls `layoutCustomRepeat` for the initial layout.
  - `readCustomRecur`'s weekly branch reads `cf.strip.days()`.

- [ ] **Step 1: Write the failing tests**

In `internal/ui/recurcustom_test.go`: (a) update the two spots that use `cf.days[...]`, and (b) add relayout tests. Replace the `weekly interval + day set` subtest body's two `cf.days[...]` lines:

```go
		cf.strip.setDays([]time.Weekday{time.Tuesday, time.Thursday})
```
(delete the two `cf.days[1]`/`cf.days[3]` lines; the rest of that subtest is unchanged).

Then append two new tests:

```go
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
```

- [ ] **Step 2: Run to verify it fails**

Run: `go test ./internal/ui/ -run TestCustomRepeat`
Expected: FAIL — `cf.strip undefined` (struct still has `days`), and the relayout tests fail (form still static).

- [ ] **Step 3: Rework `customRecurFields` and `newCustomRepeatForm`**

In `internal/ui/recurcustom.go`, change the struct field:

```go
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
```

Replace `newCustomRepeatForm` with a version that builds all widgets once, then lays out (note dropdowns are built with `newFormDropDown` so they keep `selectionStyle`, and Unit/Ends get their `SetSelectedFunc` **after** the initial option is set, so construction doesn't fire a relayout):

```go
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
```

- [ ] **Step 4: Update `readCustomRecur`'s weekly branch**

In `readCustomRecur`, replace the weekly loop over `cf.days[i]` with the strip read:

```go
	case model.FreqWeekly:
		spec.Weekdays = cf.strip.days()
		if len(spec.Weekdays) == 0 {
			spec.Weekdays = []time.Weekday{anchor.Weekday()} // fall back to the start date's weekday
		}
```

- [ ] **Step 5: Shrink the modal height**

In `openCustomRepeat`, change the modal height from 22 to 12 (worst case — weekly + On-date is 5 fields + 2 buttons + border/title). Find `a.openModal(pageRepeat, f, 54, 22)` and change `22` to `12`.

- [ ] **Step 6: Run the tests**

Run: `go test ./internal/ui/ -run TestCustomRepeat -v`
Expected: PASS (relayout, daily-minimal, draw-stress, focus-stack). Then the read/validation tests:
Run: `go test ./internal/ui/ -run 'TestReadCustomRecur|TestMonthlyOptions'`
Expected: PASS.

- [ ] **Step 7: Extend the draw-stress to exercise relayout**

Append to `TestCustomRepeatDrawStress` (after the existing loop) a relayout draw pass, so a rebuild mid-draw can't panic:

```go
	// Drawing after a relayout (Unit + Ends changes) must also be panic-free.
	f, cf := a.newCustomRepeatForm(model.RecurSpec{Freq: model.FreqWeekly}, customAnchor)
	cf.unit.SetCurrentOption(2) // → months (relayout)
	cf.ends.SetCurrentOption(1) // → On date (relayout)
	for _, g := range stressGeoms {
		drawGeom(t, "custom-repeat-relaid", f, g.w, g.h)
	}
```

- [ ] **Step 8: Full gate + commit**

```bash
export PATH=$PATH:~/go/bin
go test ./... && go vet ./... && staticcheck ./... && go build ./...
git add internal/ui/recurcustom.go internal/ui/recurcustom_test.go
git commit -m "feat: dynamic Custom repeat form (relevant fields only, weekday strip)

Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>"
```

**If tview fights the relayout-from-callback** (focus panic, or items not clearing): STOP and report BLOCKED with the failing output rather than hacking around it — the relayout-from-dropdown-callback is the one genuinely tricky part of this plan.

---

## Task 4: Docs ripple

**Files:**
- Modify: `main.md`, `README.md` (only if wording changes), `internal/ui/help.go` (only if a key note is needed), `log.md`

**Interfaces:** none (documentation only).

- [ ] **Step 1: Update `main.md` — mark the item shipped, in place**

In the v1.3.0 "Post-Build Incremental Changes" section, **remove** the "Custom-recurrence form redesign" bullet from the **"Planned before release"** group (which should then be empty — remove the now-empty "Planned before release" heading and its intro sentence too, since both pre-release items have now shipped) and add a shipped bullet to the shipped list:

```markdown
- **Custom repeat sub-form redesign.** The Custom… repeat sub-form now shows only
  the fields relevant to the current selection — Every, Unit and Ends always, plus
  the weekday strip only for a weekly rule, "Monthly by" only for monthly, and
  Until / Count only for the matching "Ends" choice — re-laid-out live as Unit or
  Ends changes (values preserved). Weekday selection is a single compact toggle
  strip (a `tview.FormItem` drilled into like any field: `←`/`→` move, Space
  toggles) in place of seven checkboxes.
```

Also update the Creation-section sentence describing the Custom… sub-form (search for `Custom…` / "nested sub-form") so it reflects the dynamic field set and the strip, in place.

- [ ] **Step 2: Update `README.md` if needed**

The README's Recurring-items prose mentions the Repeat field + Custom…; if it describes the Custom form's fields, adjust to the dynamic behavior. If it only says "Custom… for any in-vocabulary rule" without field detail, no change is needed. Do not re-narrate keybindings the table owns.

- [ ] **Step 3: Update `:help` if needed**

`internal/ui/help.go` — the forms section describes DRILL nav generally; the strip uses the same drill model, so a change is only needed if a form-specific key note would help (e.g. "weekday strip: Space toggles"). Add one concise line under the forms section if it clarifies; otherwise skip.

- [ ] **Step 4: Add the `log.md` entry (top, below the intro blockquote, above the newest entry — leave that entry byte-identical)**

```markdown
## 2026-07-23 — v1.3.0: Custom repeat sub-form redesign (dynamic fields + weekday strip)

- Reworked the Custom… repeat sub-form (`internal/ui/recurcustom.go`) from a static 13-field wall into a dynamic form that shows only the fields relevant to the current selection: Every, Unit, Ends always; the weekday strip only for weeks; "Monthly by" only for months; Until/Count only for the matching Ends choice. Unit/Ends changes re-lay-out the form live (`layoutCustomRepeat`, via `caretForm.clearItems`/`addExisting`), preserving values in fields that stay visible. Modal height 22→12.
- New `weekdayStrip` widget (`internal/ui/weekdaystrip.go`) — a single-row `tview.FormItem` replacing the 7 weekday checkboxes: drilled into via the app-wide NORMAL/DRILL model (Enter drills; `←`/`→` or `h`/`l` move the day cursor; Space toggles; Esc leaves), selected days reverse-video via `selectionStyle`, the focused cell underlined in the accent color.
- caretForm gained `newFormDropDown` (centralizes the dropdown `selectionStyle` guardrail), `clearItems`/`addExisting` (relayout primitives), `isDrillable` (auto-drill includes the strip), and a `*weekdayStrip` case in `actNormal` + the Draw gutter.
- **Repro-first**: `weekdaystrip_test.go` (seed/read, cursor+toggle, reverse-video legibility, draw-stress), `formnav_test.go` (strip drills+toggles in a caretForm; clearItems/addExisting), `recurcustom_test.go` (relayout hides/shows the right fields + preserves values; daily is 3 fields; relaid-out draw-stress). Existing read/validation tests updated to the strip API.
- Docs rippled: `main.md` (item → shipped, both pre-release items now done), `log.md`.
- Full gate green (`go test ./...`, `go vet ./...`, `staticcheck ./...`, `go build ./...`).
- Files: `internal/ui/weekdaystrip.go`, `internal/ui/weekdaystrip_test.go`, `internal/ui/forms.go`, `internal/ui/formnav_test.go`, `internal/ui/recurcustom.go`, `internal/ui/recurcustom_test.go`, `main.md`, `log.md`.
```

- [ ] **Step 5: Verify + commit**

```bash
go build ./... && grep -c '^## ' log.md   # heading count = previous + 1
git add main.md README.md internal/ui/help.go log.md
git commit -m "docs: record Custom repeat sub-form redesign (v1.3.0)

Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>"
```

(If README/help.go were not changed, omit them from the `git add`.)

---

## Self-Review

**1. Spec coverage** (against `docs/superpowers/specs/2026-07-23-custom-recur-form-redesign-design.md`):
- Dynamic visibility (3 always + conditional) → Task 3 `layoutCustomRepeat`. ✓
- Relayout on Unit/Ends change, values preserved → Task 3 `SetSelectedFunc` + persistent widgets; `TestCustomRepeatRelayout`. ✓
- Weekday strip as a drillable `tview.FormItem` (←/→ move, Space toggle, reverse-video selection, accent cursor, anchor fallback) → Task 1. ✓
- caretForm integration (Enter drills, gutter caret, auto-drill) → Task 2. ✓
- Fixed height worst-case (~12) → Task 3 Step 5. ✓
- Preserved: monthlyOptions, validation, seeding/write-back (`wireRepeatCustom` untouched), model layer untouched, focus-stack (`TestCustomRepeatFocusStack` unchanged and must stay green). ✓
- Guardrails: selectionStyle (strip legibility test + `newFormDropDown`), display-stress (strip + relaid-out form) → Tasks 1–3. ✓
- Docs ripple → Task 4. ✓

**2. Placeholder scan:** No TBD/TODO; every code step shows complete code; commands have expected output. Task 4 Steps 2–3 are conditional ("if wording changes") — acceptable because they name the exact file/section to inspect and the decision rule, not vague work.

**3. Type consistency:** `weekdayStrip` methods (`setDays`, `days`, `SetLabel`, `moveCursor`) are defined in Task 1 and used identically in Tasks 2–3 and the tests. `customRecurFields.strip *weekdayStrip` (Task 3) matches. `newFormDropDown`/`clearItems`/`addExisting`/`isDrillable` defined in Task 2, used in Task 3. `layoutCustomRepeat(f, cf, focusOn)` signature consistent between definition and both call sites (`cf.unit`, `cf.ends`, `nil`). `tview.Print` second return used for width. `Clear(false)` keeps buttons. ✓
