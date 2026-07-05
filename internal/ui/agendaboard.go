package ui

import (
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"

	"github.com/littekge/LazyPlanner/internal/model"
)

// agendaBoard is the full-detail day agenda (mode 3). It is custom-drawn so the
// selected item can be marked with a rounded outline box — the same cursor style
// as the calendar month view — instead of a filled highlight bar. Selection is
// driven by the left Agenda list (this widget takes no focus); it manages its
// own vertical scroll to keep the selected item in view.
type agendaBoard struct {
	*tview.Box

	date     time.Time
	items    []model.AgendaItem
	selected int
	scroll   int
}

func newAgendaBoard() *agendaBoard {
	return &agendaBoard{Box: tview.NewBox()}
}

func (b *agendaBoard) setData(date time.Time, items []model.AgendaItem) {
	b.date = date
	b.items = items
	b.clampSelected()
}

func (b *agendaBoard) setSelected(i int) {
	b.selected = i
	b.clampSelected()
}

func (b *agendaBoard) clampSelected() {
	if b.selected < 0 {
		b.selected = 0
	}
	if b.selected >= len(b.items) {
		b.selected = len(b.items) - 1
	}
}

type styledLine struct {
	text  string
	style tcell.Style
}

// agendaItemLines renders one item as its stacked detail lines (title, meta, and
// an optional description), matching the colors used elsewhere.
func agendaItemLines(it model.AgendaItem) []styledLine {
	gray := tcell.StyleDefault.Foreground(adjacentColor)
	plain := tcell.StyleDefault
	if it.Todo != nil {
		t := it.Todo
		lines := []styledLine{
			{whenLabel(it) + "  " + nonEmpty(t.Summary, "(untitled)"), tcell.StyleDefault.Foreground(tcell.ColorAqua)},
			{"task · " + statusText(t.Status) + " · priority " + priorityText(t.Priority), gray},
		}
		if t.Description != "" {
			lines = append(lines, styledLine{oneLine(t.Description), plain})
		}
		return lines
	}
	e := it.Event
	lines := []styledLine{
		{whenLabel(it) + "  " + nonEmpty(e.Summary, "(untitled)"), tcell.StyleDefault.Foreground(eventColor)},
	}
	if e.Location != "" {
		lines = append(lines, styledLine{"at " + e.Location, gray})
	}
	if e.Description != "" {
		lines = append(lines, styledLine{oneLine(e.Description), plain})
	}
	return lines
}

func (b *agendaBoard) Draw(screen tcell.Screen) {
	b.Box.DrawForSubclass(screen, b)
	x, y, w, h := b.GetInnerRect()
	if w < 6 || h < 2 {
		return
	}

	printStyled(screen, x, y, w, b.date.Format("Monday, January 2, 2006"),
		tcell.StyleDefault.Foreground(accentColor).Bold(true))

	contentTop := y + 2
	availH := h - 2
	if availH < 1 {
		return
	}
	if len(b.items) == 0 {
		printStyled(screen, x, contentTop, w, "No events or due tasks today.",
			tcell.StyleDefault.Foreground(adjacentColor))
		return
	}

	// Lay out blocks in a virtual coordinate space; row 0 is a leading gap so the
	// first item's top border has somewhere to draw.
	blocks := make([][]styledLine, len(b.items))
	starts := make([]int, len(b.items))
	line := 1
	for i, it := range b.items {
		blocks[i] = agendaItemLines(it)
		starts[i] = line
		line += len(blocks[i]) + 1 // block plus a one-row gap
	}
	total := line

	// Scroll minimally to keep the selected block (and its border rows) visible.
	selTop := starts[b.selected] - 1
	selBot := starts[b.selected] + len(blocks[b.selected])
	if selTop < b.scroll {
		b.scroll = selTop
	}
	if selBot >= b.scroll+availH {
		b.scroll = selBot - availH + 1
	}
	if b.scroll > total-availH {
		b.scroll = total - availH
	}
	if b.scroll < 0 {
		b.scroll = 0
	}

	// Text is inset (x+2 .. x+w-3) so the selection box borders at x and x+w-1
	// never overlap it.
	for i, blk := range blocks {
		for j, ln := range blk {
			sr := contentTop + starts[i] + j - b.scroll
			if sr < contentTop || sr >= contentTop+availH {
				continue
			}
			printStyled(screen, x+2, sr, w-4, ln.text, ln.style)
		}
	}

	b.drawSelBox(screen, x, contentTop, w, availH, starts[b.selected], len(blocks[b.selected]))
}

// drawSelBox draws a rounded outline around the selected item's block, clipped to
// the visible content rows. The top/bottom borders sit in the inter-item gaps.
func (b *agendaBoard) drawSelBox(screen tcell.Screen, x, contentTop, w, availH, start, height int) {
	style := tcell.StyleDefault.Foreground(borderFocused)
	top := contentTop + (start - 1) - b.scroll
	bottom := contentTop + (start + height) - b.scroll
	left, right := x, x+w-1
	lo, hi := contentTop, contentTop+availH-1

	for yy := top + 1; yy < bottom; yy++ {
		if yy < lo || yy > hi {
			continue
		}
		screen.SetContent(left, yy, tcell.RuneVLine, nil, style)
		screen.SetContent(right, yy, tcell.RuneVLine, nil, style)
	}
	if top >= lo && top <= hi {
		screen.SetContent(left, top, '╭', nil, style)
		screen.SetContent(right, top, '╮', nil, style)
		for xx := left + 1; xx < right; xx++ {
			screen.SetContent(xx, top, tcell.RuneHLine, nil, style)
		}
	}
	if bottom >= lo && bottom <= hi {
		screen.SetContent(left, bottom, '╰', nil, style)
		screen.SetContent(right, bottom, '╯', nil, style)
		for xx := left + 1; xx < right; xx++ {
			screen.SetContent(xx, bottom, tcell.RuneHLine, nil, style)
		}
	}
}
