package ui

import (
	"fmt"
	"strings"

	"github.com/gdamore/tcell/v2"

	"github.com/littekge/LazyPlanner/internal/model"
)

// colorMode selects how a server calendar color is rendered.
type colorMode int

const (
	// colorAuto draws the exact server color as truecolor RGB; tcell downsamples
	// to 256 or 16 on terminals that can't do truecolor (including a bare TTY).
	colorAuto colorMode = iota
	// color16 maps to the nearest of the themed 16 ANSI colors (inherits the
	// terminal theme; the pre-truecolor behavior).
	color16
	// colorOff ignores server colors entirely (default event/task colors only).
	colorOff
)

// parseColorMode maps a config color_mode string to a colorMode. "truecolor" is
// treated as colorAuto here — main force-enables tcell truecolor for it; the
// rendering is identical. Unknown values fall back to auto.
func parseColorMode(s string) colorMode {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "16", "ansi", "themed":
		return color16
	case "off", "none", "mono":
		return colorOff
	default: // "auto", "truecolor", "" …
		return colorAuto
	}
}

// ansi16Colors maps the 16 ANSI palette indices returned by model.NearestANSI16
// to tcell's named colors, in the same order as model's RGB table. tcell
// downsamples these to the terminal's actual palette, so a server calendar color
// renders in the terminal theme rather than as a fixed RGB (see main.md's
// 16-color rule).
var ansi16Colors = [16]tcell.Color{
	tcell.ColorBlack, tcell.ColorMaroon, tcell.ColorGreen, tcell.ColorOlive,
	tcell.ColorNavy, tcell.ColorPurple, tcell.ColorTeal, tcell.ColorSilver,
	tcell.ColorGray, tcell.ColorRed, tcell.ColorLime, tcell.ColorYellow,
	tcell.ColorBlue, tcell.ColorFuchsia, tcell.ColorAqua, tcell.ColorWhite,
}

// ansi16Names are the tview color-tag names for each palette index, used to tint
// list items (which render through tview's style-tag parser).
var ansi16Names = [16]string{
	"black", "maroon", "green", "olive", "navy", "purple", "teal", "silver",
	"gray", "red", "lime", "yellow", "blue", "fuchsia", "aqua", "white",
}

// calColor is a calendar's server color resolved for the terminal.
type calColor struct {
	fg   tcell.Color // foreground accent (bullets, day-cell lines, agenda titles, all-day band); readability-adjusted
	fill tcell.Color // exact color for filled blocks (their own text supplies contrast)
	name string      // tview color tag matching fg (for list-item bullets)
	dark bool        // fill is a dark color → use white (not black) text over it
}

// resolveCalColor maps a server hex color (e.g. "#FF2968FF") to a calColor under
// the given mode. ok is false when the color is empty/unparseable or the mode is
// colorOff, so the caller keeps its default (event green / task aqua).
//
// In colorAuto the exact color is used as a truecolor fill, while the foreground
// variant is lifted to a readability floor (see model.ReadableFg) since it sits
// on the terminal's unknown default background. In color16 both collapse to the
// nearest themed ANSI color.
func resolveCalColor(hex string, mode colorMode) (calColor, bool) {
	switch mode {
	case colorOff:
		return calColor{}, false
	case color16:
		idx, ok := model.NearestANSI16(hex)
		if !ok {
			return calColor{}, false
		}
		c := ansi16Colors[idx]
		return calColor{fg: c, fill: c, name: ansi16Names[idx], dark: model.ANSI16IsDark(idx)}, true
	default: // colorAuto
		r, g, b, ok := model.ParseHexColor(hex)
		if !ok {
			return calColor{}, false
		}
		fr, fgc, fb := model.ReadableFg(r, g, b)
		return calColor{
			fg:   tcell.NewRGBColor(int32(fr), int32(fgc), int32(fb)),
			fill: tcell.NewRGBColor(int32(r), int32(g), int32(b)),
			name: fmt.Sprintf("#%02x%02x%02x", fr, fgc, fb),
			dark: model.Luminance(r, g, b) < 128,
		}, true
	}
}

// rebuildColorIndex recomputes the per-calendar and per-item color maps from the
// current store contents. It runs whenever the calendars are rebuilt (every
// refresh/reload), so a synced color change takes effect on the next redraw.
// Calendars with no (mappable) server color are simply absent, and their items
// fall back to the default event/task colors.
func (a *app) rebuildColorIndex() {
	calC := map[string]calColor{}
	itemC := map[string]calColor{}
	for _, cal := range a.store.Calendars() {
		cc, ok := resolveCalColor(cal.Color, a.colorMode)
		if !ok {
			continue
		}
		calC[cal.ID] = cc
		for _, r := range cal.Resources {
			for _, ev := range r.Object.Events {
				itemC[ev.UID] = cc
			}
			for _, td := range r.Object.Todos {
				itemC[td.UID] = cc
			}
		}
	}
	a.calColors = calC
	a.itemColors = itemC
}

// agendaItemColor resolves an agenda item to its calendar's color, by the item's
// UID. ok is false when the calendar has no mappable color, so the caller uses
// its default.
func (a *app) agendaItemColor(it model.AgendaItem) (calColor, bool) {
	var uid string
	switch {
	case it.Todo != nil:
		uid = it.Todo.UID
	case it.Event != nil:
		uid = it.Event.UID
	}
	cc, ok := a.itemColors[uid]
	return cc, ok
}

// occurrenceColor resolves an event occurrence to its calendar's color.
func (a *app) occurrenceColor(o model.Occurrence) (calColor, bool) {
	if o.Event == nil {
		return calColor{}, false
	}
	cc, ok := a.itemColors[o.Event.UID]
	return cc, ok
}

// todoColor resolves a task to its list's color, by UID.
func (a *app) todoColor(t *model.Todo) (calColor, bool) {
	if t == nil {
		return calColor{}, false
	}
	cc, ok := a.itemColors[t.UID]
	return cc, ok
}
