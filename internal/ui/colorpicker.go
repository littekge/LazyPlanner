package ui

import (
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"

	"github.com/littekge/LazyPlanner/internal/model"
)

// calendarPalette is the preset swatch set the color picker offers — a
// NextCloud-like spread of well-separated hues (including NextCloud's blue,
// #0082c9). The picker also offers a "Custom hex…" entry for any other color.
// Immutable lookup table (like collectionTypes); its length is a multiple of
// colorPickerCols so the grid is rectangular.
var calendarPalette = []string{
	"#c9302c", "#e8710a", "#f0a30a", "#c0ca33", "#6aa84f",
	"#009688", "#0082c9", "#1a73e8", "#3f51b5", "#7e57c2",
	"#a64d99", "#e0397a", "#795548", "#607d8b", "#9e9e9e",
}

// sameColor reports whether two hex colors are the same RGB, ignoring case, a
// leading '#', and any alpha channel — server colors often arrive as #RRGGBBFF,
// so a plain-#RRGGBB palette swatch must still match one for preselection.
func sameColor(a, b string) bool {
	ar, ag, ab, aok := model.ParseHexColor(a)
	br, bg, bb, bok := model.ParseHexColor(b)
	return aok && bok && ar == br && ag == bg && ab == bb
}

// defaultCalendarColor is the color a newly created calendar/list gets when the
// user doesn't pick one — NextCloud's blue (a palette swatch), so every created
// collection always has a color instead of rendering with the app default.
const defaultCalendarColor = "#0082c9"

const (
	colorPickerCols = 5 // swatches per row
	swatchW         = 4 // width of a swatch cell (drawn in the calendar's color)
	swatchCellW     = swatchW + 2
	swatchCellH     = 2 // swatch row + a gap row
)

// colorPicker is a modal swatch grid for choosing a calendar color. The grid
// holds calendarPalette; a trailing "Custom hex…" entry (index len(palette))
// opens a free hex input. hjkl/arrows move the cursor, Enter selects, Esc
// cancels. It carries no calendar identity — the app wires the callbacks.
type colorPicker struct {
	*tview.Box
	cursor   int    // 0..len(palette): the last index is the Custom entry
	current  string // the calendar's current color (marked on the matching swatch)
	onSelect func(hex string)
	onCustom func()
	onCancel func()
}

func newColorPicker() *colorPicker {
	p := &colorPicker{Box: tview.NewBox()}
	p.SetBorder(true).SetBorderColor(accentColor).SetTitleColor(accentColor)
	p.SetBackgroundColor(tcell.ColorDefault)
	return p
}

// customIndex is the cursor value of the "Custom hex…" entry.
func (p *colorPicker) customIndex() int { return len(calendarPalette) }

// preselect records current as the marked color and positions the cursor on the
// matching swatch (RGB-wise, so an alpha suffix still matches), the Custom entry
// when current is a non-palette color, or the first swatch when it's empty.
func (p *colorPicker) preselect(current string) {
	p.current = current
	if current == "" {
		p.cursor = 0
		return
	}
	p.cursor = p.customIndex()
	for i, hex := range calendarPalette {
		if sameColor(hex, current) {
			p.cursor = i
			return
		}
	}
}

func (p *colorPicker) Draw(screen tcell.Screen) {
	p.Box.DrawForSubclass(screen, p)
	x, y, w, h := p.GetInnerRect()
	if w < swatchCellW || h < 2 {
		return
	}
	startX, startY := x+1, y+1

	for i, hex := range calendarPalette {
		r, g, b, ok := model.ParseHexColor(hex)
		cx := startX + (i%colorPickerCols)*swatchCellW
		cy := startY + (i/colorPickerCols)*swatchCellH
		fill := tcell.StyleDefault
		if ok {
			fill = fill.Background(tcell.NewRGBColor(int32(r), int32(g), int32(b)))
		}
		for dx := 0; dx < swatchW; dx++ {
			screen.SetContent(cx+1+dx, cy, ' ', nil, fill)
		}
		// Mark the calendar's current color with a ✓ in a contrasting ink.
		if ok && sameColor(hex, p.current) {
			ink := tcell.ColorBlack
			if model.Luminance(r, g, b) < 128 {
				ink = tcell.ColorWhite
			}
			screen.SetContent(cx+1+swatchW/2, cy, '✓', nil, fill.Foreground(ink))
		}
		// The cursor is accent brackets around the swatch (a fill can't show a
		// reverse cursor legibly, and the brackets sit in the cell's margins).
		if i == p.cursor {
			bs := tcell.StyleDefault.Foreground(accentColor).Bold(true)
			screen.SetContent(cx, cy, '[', nil, bs)
			screen.SetContent(cx+1+swatchW, cy, ']', nil, bs)
		}
	}

	rows := (len(calendarPalette) + colorPickerCols - 1) / colorPickerCols
	custY := startY + rows*swatchCellH
	label, style := "  Custom hex… ", tcell.StyleDefault
	if p.cursor == p.customIndex() {
		label, style = "▸ Custom hex… ", tcell.StyleDefault.Foreground(accentColor).Bold(true)
	}
	printStyled(screen, startX, custY, w-2, label, style)
	if p.current != "" {
		printStyled(screen, startX, custY+1, w-2, "current: "+p.current,
			tcell.StyleDefault.Foreground(adjacentColor))
	}
}

func (p *colorPicker) InputHandler() func(*tcell.EventKey, func(tview.Primitive)) {
	return p.WrapInputHandler(func(ev *tcell.EventKey, _ func(tview.Primitive)) {
		n := len(calendarPalette)
		cols := colorPickerCols
		switch {
		case ev.Key() == tcell.KeyEscape:
			if p.onCancel != nil {
				p.onCancel()
			}
		case ev.Key() == tcell.KeyEnter:
			p.choose()
		case ev.Key() == tcell.KeyLeft || ev.Rune() == 'h':
			if p.cursor < n && p.cursor%cols > 0 {
				p.cursor--
			}
		case ev.Key() == tcell.KeyRight || ev.Rune() == 'l':
			if p.cursor < n && p.cursor%cols < cols-1 && p.cursor+1 < n {
				p.cursor++
			}
		case ev.Key() == tcell.KeyUp || ev.Rune() == 'k':
			if p.cursor == p.customIndex() {
				p.cursor = n - 1
			} else if p.cursor >= cols {
				p.cursor -= cols
			}
		case ev.Key() == tcell.KeyDown || ev.Rune() == 'j':
			if p.cursor < n {
				if p.cursor+cols < n {
					p.cursor += cols
				} else {
					p.cursor = p.customIndex() // drop to Custom from the bottom row
				}
			}
		}
	})
}

func (p *colorPicker) choose() {
	if p.cursor == p.customIndex() {
		if p.onCustom != nil {
			p.onCustom()
		}
		return
	}
	if p.onSelect != nil {
		p.onSelect(calendarPalette[p.cursor])
	}
}
