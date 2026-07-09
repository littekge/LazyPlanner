package ui

import (
	"github.com/gdamore/tcell/v2"

	"github.com/littekge/LazyPlanner/internal/model"
)

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

// calColor is a calendar's server color resolved to the terminal palette.
type calColor struct {
	fg   tcell.Color // accent: list bullets, day-cell lines, agenda titles, all-day band
	name string      // tview color-tag name (for list-item bullets)
	dark bool        // fg is a dark color → use white (not black) text over it as a fill
}

// resolveCalColor maps a server hex color (e.g. "#FF2968FF") to the nearest
// terminal palette color. ok is false when hex is empty or unparseable, so the
// caller keeps its default (event green / task aqua).
func resolveCalColor(hex string) (calColor, bool) {
	idx, ok := model.NearestANSI16(hex)
	if !ok {
		return calColor{}, false
	}
	return calColor{fg: ansi16Colors[idx], name: ansi16Names[idx], dark: model.ANSI16IsDark(idx)}, true
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
		cc, ok := resolveCalColor(cal.Color)
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
