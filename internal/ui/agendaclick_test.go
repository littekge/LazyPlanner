package ui

import (
	"fmt"
	"testing"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// drawnAgendaApp builds an app in Agenda mode with n timed events today, drawn
// once so the board has real rects and a settled scroll.
//
// The day (2026-07-10, midnight UTC) is deliberately clear of the store
// fixture's own events (`meeting`/`standup` fall on 2026-07-04/05 — see
// internal/store/testdata/vdir) so an n=0 board is genuinely empty, and starts
// at midnight so all n hourly events land on the same calendar day (avoiding
// the day-boundary rollover an afternoon start would cause).
func drawnAgendaApp(t *testing.T, n int) (*app, tcell.SimulationScreen) {
	t.Helper()
	now := time.Date(2026, 7, 10, 0, 0, 0, 0, time.UTC)
	a := newRootedTestApp(t, now)
	for i := 0; i < n; i++ {
		putTimedEvent(t, a, testCalID(a), fmt.Sprintf("event%d", i), now.Add(time.Duration(i)*time.Hour))
	}
	a.reload()
	a.setMode(modeAgenda)

	screen := tcell.NewSimulationScreen("")
	if err := screen.Init(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(screen.Fini)
	screen.SetSize(120, 40)
	a.root.SetRect(0, 0, 120, 40)
	a.root.Draw(screen)
	return a, screen
}

// TestAgendaBoardItemAtYBoundaries walks every screen row of the drawn board and
// asserts itemAtY agrees with the layout: each block's text rows map to its item,
// and both sides of every window — the header rows above the content, each
// gap/border row between blocks, and the first row past the last block — map to
// -1 (the audit guardrail: test both sides of every half-open window).
func TestAgendaBoardItemAtYBoundaries(t *testing.T) {
	a, _ := drawnAgendaApp(t, 3)
	b := a.agenda

	_, y, _, h := b.GetInnerRect()
	contentTop := y + 2
	blocks, starts, _ := b.layoutBlocks()

	want := make(map[int]int) // screen row -> item index
	for i := range blocks {
		for j := 0; j < len(blocks[i]); j++ {
			want[contentTop+starts[i]+j-b.scroll] = i
		}
	}
	for sy := y - 1; sy < y+h+1; sy++ {
		expect, ok := want[sy]
		if !ok {
			expect = -1
		}
		if sy < contentTop || sy >= y+h {
			expect = -1 // header rows, and rows outside the pane, never map
		}
		if got := b.itemAtY(sy); got != expect {
			t.Errorf("itemAtY(%d) = %d, want %d", sy, got, expect)
		}
	}
}

// TestAgendaBoardItemAtYEmptyAndScrolled covers the two remaining states: an
// empty board maps every row to -1, and a scrolled board (selection forced to
// the last of many items) still maps a visible block to the right index.
func TestAgendaBoardItemAtYEmptyAndScrolled(t *testing.T) {
	empty, _ := drawnAgendaApp(t, 0)
	_, ey, _, eh := empty.agenda.GetInnerRect()
	for sy := ey; sy < ey+eh; sy++ {
		if got := empty.agenda.itemAtY(sy); got != -1 {
			t.Errorf("empty board: itemAtY(%d) = %d, want -1", sy, got)
		}
	}

	a, screen := drawnAgendaApp(t, 20)
	last := len(a.agenda.items) - 1
	a.agendaList.SetCurrentItem(last)
	a.root.Draw(screen) // settle the scroll onto the selection
	b := a.agenda
	if b.scroll == 0 {
		t.Fatal("precondition: 20 items on a 40-row screen must scroll")
	}
	_, y, _, _ := b.GetInnerRect()
	contentTop := y + 2
	_, starts, _ := b.layoutBlocks()
	sy := contentTop + starts[last] - b.scroll // the last block's first text row
	if got := b.itemAtY(sy); got != last {
		t.Errorf("scrolled: itemAtY(%d) = %d, want %d", sy, got, last)
	}
}

// TestAgendaBoardClickSelects: a single left click on an item's text row moves
// the agenda selection to it (the board was the one surface where the mouse did
// nothing); a click on a gap row changes nothing.
func TestAgendaBoardClickSelects(t *testing.T) {
	a, _ := drawnAgendaApp(t, 3)
	b := a.agenda
	if got := a.agendaList.GetCurrentItem(); got != 0 {
		t.Fatalf("precondition: selection starts at 0, got %d", got)
	}

	_, y, _, _ := b.GetInnerRect()
	contentTop := y + 2
	blocks, starts, _ := b.layoutBlocks()
	// The board sits right of the always-visible left overview column
	// (calendars/tasklists/agendaList tile x 0..leftWidth-1 for the full pane
	// height), so the click's x must land inside the board's own rect — a
	// column within the left overview would hit one of those cases first.
	boardX, _, _, _ := b.GetRect()
	clickCol := boardX + 1

	clickRow := contentTop + starts[2] - b.scroll // item 2's first text row
	ev := tcell.NewEventMouse(clickCol, clickRow, tcell.Button1, 0)
	a.mouseCapture(ev, tview.MouseLeftClick)
	if got := a.agendaList.GetCurrentItem(); got != 2 {
		t.Errorf("click on item 2's row: selection = %d, want 2", got)
	}
	if b.selected != 2 {
		t.Errorf("board selection = %d, want 2 (SetCurrentItem must drive setSelected)", b.selected)
	}

	gapRow := contentTop + starts[0] + len(blocks[0]) - b.scroll // the gap under item 0
	a.mouseCapture(tcell.NewEventMouse(clickCol, gapRow, tcell.Button1, 0), tview.MouseLeftClick)
	if got := a.agendaList.GetCurrentItem(); got != 2 {
		t.Errorf("click on a gap row must not move the selection: got %d, want 2", got)
	}
}

// TestAgendaBoardDoubleClickEditsRowUnderCursor: the pass-16 known limitation —
// a double-click on the board edited whatever was already selected. It must now
// re-target to the item under the cursor first (the treeNodeAtY re-target
// precedent), so the edit form opens on the clicked item.
func TestAgendaBoardDoubleClickEditsRowUnderCursor(t *testing.T) {
	a, _ := drawnAgendaApp(t, 3)
	b := a.agenda

	_, y, _, _ := b.GetInnerRect()
	contentTop := y + 2
	_, starts, _ := b.layoutBlocks()
	// See TestAgendaBoardClickSelects: the click's x must land inside the
	// board's own rect, right of the always-visible left overview column.
	boardX, _, _, _ := b.GetRect()
	clickCol := boardX + 1

	clickRow := contentTop + starts[1] - b.scroll // item 1, while 0 is selected
	a.mouseCapture(tcell.NewEventMouse(clickCol, clickRow, tcell.Button1, 0), tview.MouseLeftDoubleClick)

	if got := a.agendaList.GetCurrentItem(); got != 1 {
		t.Errorf("double-click re-target: selection = %d, want 1", got)
	}
	if !a.modalOpen() {
		t.Error("double-click on an item must open the edit form")
	}
}
