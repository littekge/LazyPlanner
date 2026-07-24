package ui

import (
	"fmt"
	"testing"
	"time"

	"github.com/gdamore/tcell/v2"
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
