package ui

import (
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// daysInWeek is the fixed cell count of the strip: one per weekday.
const daysInWeek = 7

// dayAbbrevs labels the strip cells Monday-first, matching mondayOrder.
var dayAbbrevs = [daysInWeek]string{"Mon", "Tue", "Wed", "Thu", "Fri", "Sat", "Sun"}

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
	selected   [daysInWeek]bool // index 0..daysInWeek-1 == mondayOrder (Monday..Sunday)
	cursor     int              // focused day cell, 0..daysInWeek-1
	// finished is wired via SetFinishedFunc to satisfy tview.FormItem, but never
	// invoked: the enclosing caretForm's drillKey intercepts Esc/Enter/Tab at the
	// form level before they reach a drilled field, so the strip itself never
	// needs to fire it. Kept for interface conformance, not a dropped wire.
	finished func(tcell.Key)
	disabled bool
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
	w.selected = [daysInWeek]bool{}
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
	for i := 0; i < daysInWeek; i++ {
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
	if w.cursor > daysInWeek-1 {
		w.cursor = daysInWeek - 1
	}
}
