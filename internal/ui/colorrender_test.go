package ui

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/gdamore/tcell/v2"

	"github.com/littekge/LazyPlanner/internal/model"
)

// rowContains reconstructs a screen row's visible text and reports whether it
// contains sub, returning the row index (or -1).
func rowFind(cells []tcell.SimCell, cw, ch int, sub string) int {
	for r := 0; r < ch; r++ {
		line := make([]rune, cw)
		for c := 0; c < cw; c++ {
			line[c] = ' '
			if rs := cells[r*cw+c].Runes; len(rs) > 0 {
				line[c] = rs[0]
			}
		}
		if strings.Contains(string(line), sub) {
			return r
		}
	}
	return -1
}

// glyphFg returns the foreground of the first cell holding rune want on row r.
func glyphFg(cells []tcell.SimCell, cw, r int, want rune) (tcell.Color, bool) {
	for c := 0; c < cw; c++ {
		if rs := cells[r*cw+c].Runes; len(rs) > 0 && rs[0] == want {
			fg, _, _ := cells[r*cw+c].Style.Decompose()
			return fg, true
		}
	}
	return 0, false
}

func TestResolveCalColor(t *testing.T) {
	// colorAuto: a bright color renders as exact truecolor (fill == fg, no floor).
	cc, ok := resolveCalColor("#3fb950", colorAuto)
	if !ok || cc.fill.Hex() != 0x3fb950 || cc.fg.Hex() != 0x3fb950 || cc.name != "#3fb950" {
		t.Errorf("resolveCalColor(#3fb950, auto) = %+v (ok=%v), want exact #3fb950", cc, ok)
	}

	// colorAuto: a dark color keeps the exact fill but lifts fg for readability.
	dark, ok := resolveCalColor("#000080", colorAuto) // navy
	if !ok || dark.fill.Hex() != 0x000080 {
		t.Fatalf("navy fill = %#06x (ok=%v), want exact #000080", dark.fill.Hex(), ok)
	}
	if !dark.dark {
		t.Error("navy fill should be flagged dark")
	}
	if dark.fg.Hex() == 0x000080 {
		t.Error("navy foreground should be lightened for readability, not left exact")
	}
	fr, fg, fb := int(dark.fg.Hex()>>16&0xff), int(dark.fg.Hex()>>8&0xff), int(dark.fg.Hex()&0xff)
	if model.Luminance(fr, fg, fb) < 90 {
		t.Errorf("lightened navy fg luminance = %d, want at/above the readability floor", model.Luminance(fr, fg, fb))
	}

	// color16: collapses to a themed named color.
	c16, ok := resolveCalColor("#ff0000ff", color16)
	if !ok || c16.fg != tcell.ColorRed || c16.name != "red" || !c16.dark {
		t.Errorf("resolveCalColor(#ff0000ff, 16) = %+v (ok=%v), want red/red/dark", c16, ok)
	}

	// colorOff and unparseable input don't resolve.
	if _, ok := resolveCalColor("#ff0000", colorOff); ok {
		t.Error("colorOff should not resolve")
	}
	if _, ok := resolveCalColor("", colorAuto); ok {
		t.Error("empty color should not resolve")
	}
}

func TestParseColorMode(t *testing.T) {
	cases := map[string]colorMode{
		"": colorAuto, "auto": colorAuto, "truecolor": colorAuto, "AUTO": colorAuto,
		"16": color16, "ansi": color16,
		"off": colorOff, "none": colorOff,
		"bogus": colorAuto,
	}
	for in, want := range cases {
		if got := parseColorMode(in); got != want {
			t.Errorf("parseColorMode(%q) = %d, want %d", in, got, want)
		}
	}
}

// TestCalendarColorIndexAndBullet: a synced calendar color populates the color
// index and draws a bullet in that color on the Calendars row, and the type
// markers now render literally (escaped) rather than being swallowed as tags.
func TestCalendarColorIndexAndBullet(t *testing.T) {
	a := newWritableTestApp(t, time.Date(2026, 7, 5, 12, 0, 0, 0, time.UTC))
	cals := a.store.Calendars()
	if len(cals) == 0 {
		t.Skip("fixture has no calendars")
	}
	// "personal" is the fixture's [both] calendar. Use a bright color that isn't
	// lifted by the readability floor, so fg is the exact hex.
	id := "personal"
	if err := a.store.SyncCalendarColor(context.Background(), id, "#3fb950"); err != nil {
		t.Fatal(err)
	}
	a.reload()

	if cc, ok := a.calColors[id]; !ok || cc.fg.Hex() != 0x3fb950 {
		t.Errorf("calColors[%q] = %+v (ok=%v), want #3fb950", id, cc, ok)
	}
	// Items of that calendar inherit the color in the item index.
	cal, _ := a.store.Calendar(id)
	for _, r := range cal.Resources {
		for _, ev := range r.Object.Events {
			if _, ok := a.itemColors[ev.UID]; !ok {
				t.Errorf("event %q missing from item color index", ev.UID)
			}
		}
	}

	cells, cw, ch := drawCells(t, a.calendars, 40, 10)
	// The bullet renders in the calendar color.
	brow := rowFind(cells, cw, ch, "●")
	if brow < 0 {
		t.Fatal("no color bullet drawn in the Calendars list")
	}
	if fg, ok := glyphFg(cells, cw, brow, '●'); !ok || fg.Hex() != 0x3fb950 {
		t.Errorf("bullet fg=%v (found=%v), want ● in #3fb950", fg, ok)
	}
	// The [both] marker now renders literally (previously swallowed by the tag parser).
	if rowFind(cells, cw, ch, "[both]") < 0 {
		t.Error("[both] marker not rendered literally in the Calendars list")
	}
}

// TestAgendaItemUsesCalendarColor: the agenda title line is drawn in the item's
// calendar color.
func TestAgendaItemUsesCalendarColor(t *testing.T) {
	a := newWritableTestApp(t, time.Date(2026, 7, 5, 12, 0, 0, 0, time.UTC))
	items := a.dayItems(model.DayStart(a.now))
	if len(items) == 0 {
		t.Skip("fixture has no agenda items on 2026-07-05")
	}
	// Color every calendar a bright green so whichever one owns the item is colored.
	for _, cal := range a.store.Calendars() {
		if err := a.store.SyncCalendarColor(context.Background(), cal.ID, "#3fb950"); err != nil {
			t.Fatal(err)
		}
	}
	a.reload()
	a.buildAgendaCenter()

	title := nonEmpty(items[0].Title, "(untitled)")
	cells, cw, ch := drawCells(t, a.agenda, 80, 24)
	trow := rowFind(cells, cw, ch, title)
	if trow < 0 {
		t.Fatalf("agenda title %q not found on screen", title)
	}
	// The whole title line (time label + summary) is drawn in the calendar color;
	// read the color at the summary's first character.
	fg, ok := glyphFg(cells, cw, trow, []rune(title)[0])
	if !ok || fg.Hex() != 0x3fb950 {
		t.Errorf("agenda title fg = %v (found=%v), want the calendar color #3fb950", fg, ok)
	}
}

// TestHiddenCalendarDropsColorBullet: hiding a calendar removes its color bullet
// from the Calendars list (a clearer at-a-glance "hidden" cue).
func TestHiddenCalendarDropsColorBullet(t *testing.T) {
	a := newWritableTestApp(t, time.Date(2026, 7, 5, 12, 0, 0, 0, time.UTC))
	cals := a.store.Calendars()
	if len(cals) == 0 {
		t.Skip("fixture has no calendars")
	}
	id := cals[0].ID
	if err := a.store.SyncCalendarColor(context.Background(), id, "#3fb950"); err != nil {
		t.Fatal(err)
	}

	// Visible: the bullet is present.
	a.buildCalendars()
	cells, cw, ch := drawCells(t, a.calendars, 40, 10)
	if rowFind(cells, cw, ch, "●") < 0 {
		t.Fatal("visible calendar should show the color bullet")
	}

	// Hidden: the bullet is gone (and the (hidden) marker shows).
	a.hidden[id] = true
	a.buildCalendars()
	cells, cw, ch = drawCells(t, a.calendars, 40, 10)
	if rowFind(cells, cw, ch, "●") >= 0 {
		t.Error("hidden calendar should not show the color bullet")
	}
	if rowFind(cells, cw, ch, "(hidden)") < 0 {
		t.Error("hidden calendar should show the (hidden) marker")
	}
}
