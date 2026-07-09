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
	cc, ok := resolveCalColor("#ff0000ff")
	if !ok || cc.fg != tcell.ColorRed || cc.name != "red" || !cc.dark {
		t.Errorf("resolveCalColor(#ff0000ff) = %+v (ok=%v), want red/red/dark", cc, ok)
	}
	if _, ok := resolveCalColor(""); ok {
		t.Error("empty color should not resolve")
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
	// "personal" is the fixture's [both] calendar.
	id := "personal"
	if err := a.store.SyncCalendarColor(context.Background(), id, "#ff0000ff"); err != nil {
		t.Fatal(err)
	}
	a.reload()

	if cc, ok := a.calColors[id]; !ok || cc.fg != tcell.ColorRed {
		t.Errorf("calColors[%q] = %+v (ok=%v), want red", id, cc, ok)
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
	if fg, ok := glyphFg(cells, cw, brow, '●'); !ok || fg != tcell.ColorRed {
		t.Errorf("bullet fg=%v (found=%v), want ● in red", fg, ok)
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
	// Color every calendar red so whichever one owns the item is colored.
	for _, cal := range a.store.Calendars() {
		if err := a.store.SyncCalendarColor(context.Background(), cal.ID, "#ff0000ff"); err != nil {
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
	if !ok || fg != tcell.ColorRed {
		t.Errorf("agenda title fg = %v (found=%v), want the calendar color red", fg, ok)
	}
}
