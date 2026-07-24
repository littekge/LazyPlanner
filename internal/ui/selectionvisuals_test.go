package ui

import (
	"strings"
	"testing"
	"time"

	"github.com/gdamore/tcell/v2"

	"github.com/littekge/LazyPlanner/internal/model"
)

// TestTreeSelectionReverseVideo: rows inside the range carry the theme-adaptive
// selectionStyle (the legibility guardrail class); rows outside stay default;
// exiting SELECT restores every row.
func TestTreeSelectionReverseVideo(t *testing.T) {
	now := time.Date(2026, 7, 5, 9, 0, 0, 0, time.UTC)
	a := newRootedTestApp(t, now)
	a.setMode(modeTasks)
	var uids []string
	for _, s := range []string{"A", "B", "C"} {
		uids = append(uids, putTodo(t, a, testCalID(a), "", "task "+s, now, true))
	}
	a.refresh(uids[0])
	a.setFocus(a.tree)
	a.selectTreeByUID(uids[0])
	a.enterSelect()
	a.selectTreeByUID(uids[1])
	a.syncSelectionVisuals()

	styles := map[string]tcell.Style{}
	for _, n := range visibleTreeNodes(a.tree.GetRoot()) {
		if td, ok := n.GetReference().(*model.Todo); ok {
			styles[td.UID] = n.GetTextStyle()
		}
	}
	if styles[uids[0]] != selectionStyle || styles[uids[1]] != selectionStyle {
		t.Fatal("in-range rows must carry selectionStyle (reverse video)")
	}
	if styles[uids[2]] == selectionStyle {
		t.Fatal("out-of-range rows must stay default")
	}
	a.exitSelect()
	for _, n := range visibleTreeNodes(a.tree.GetRoot()) {
		if td, ok := n.GetReference().(*model.Todo); ok && n.GetTextStyle() == selectionStyle {
			t.Fatalf("row %s must be restored on exit", td.UID)
		}
	}
}

// TestMonthGridDayRangeBoxes: every day in the range draws an outline box (the
// cursor day keeps the focused style), verified by corner glyphs in the cells.
func TestMonthGridDayRangeBoxes(t *testing.T) {
	now := time.Date(2026, 7, 6, 9, 0, 0, 0, time.UTC)
	a := newRootedTestApp(t, now)
	a.setMode(modeCalendar)
	a.refresh("")
	a.month.selected = model.DayStart(now)
	a.enterSelect()
	a.month.selected = model.DayStart(now.AddDate(0, 0, 2))
	a.syncSelectionVisuals()

	cells, cw, ch := drawCells(t, a.month, 84, 30)
	var b strings.Builder
	for row := 0; row < ch; row++ {
		for col := 0; col < cw; col++ {
			if c := cells[row*cw+col]; len(c.Runes) > 0 {
				b.WriteRune(c.Runes[0])
			} else {
				b.WriteByte(' ')
			}
		}
	}
	// Three boxed days → at least three top-left rounded corners on the grid.
	if n := strings.Count(b.String(), "╭"); n < 3 {
		t.Fatalf("expected ≥3 day boxes for a 3-day range, found %d corners", n)
	}
	a.exitSelect()
}

// TestDrilledRangeMarksItems: in a drilled month cell, items between anchor and
// cursor draw reverse-video.
func TestDrilledRangeMarksItems(t *testing.T) {
	now := time.Date(2026, 7, 5, 9, 0, 0, 0, time.UTC)
	a := newRootedTestApp(t, now)
	a.setMode(modeCalendar)
	putEvent(t, a, testCalID(a), "eventone", now, false)
	putEvent(t, a, testCalID(a), "eventtwo", now.Add(time.Hour), false)
	a.refresh("")
	a.month.reDrill(model.DayStart(now), 0)
	a.enterSelect()
	a.month.eventIndex = 1
	a.syncSelectionVisuals()

	// The drilled day fixture-carries a third item (the "work" calendar's
	// 2026-07-05 "Project meeting"), so the cell needs enough vertical room for
	// all three lines — at 30 rows the month grid's 6-row layout gives the day
	// cell only one content line, which collapses everything to "+N more" and
	// never exercises the per-item highlight this test is asserting on.
	cells, cw, ch := drawCells(t, a.month, 84, 48)
	reversed := 0
	for row := 0; row < ch; row++ {
		for col := 0; col < cw; col++ {
			c := cells[row*cw+col]
			if _, _, attrs := c.Style.Decompose(); attrs&tcell.AttrReverse != 0 {
				reversed++
			}
		}
	}
	// Two item lines reversed: clearly more reversed cells than a single-item
	// drill (the pre-SELECT baseline is one line's worth).
	if reversed < len("eventone")+len("eventtwo") {
		t.Fatalf("expected both range items reversed, got %d reversed cells", reversed)
	}
	a.exitSelect()
}
