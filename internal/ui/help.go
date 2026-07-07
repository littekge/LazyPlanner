package ui

import (
	"strings"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

const pageHelp = "help"

// helpSections is the cheat sheet shown by `?` / `:help`. Grouped for scanning;
// the create chords are also discoverable live via the which-key popup.
var helpSections = []struct {
	title string
	rows  [][2]string
}{
	{"Panels & navigation", [][2]string{
		{"1 2 3", "focus Calendars / Tasks / Agenda"},
		{"Tab / Shift-Tab", "cycle panels"},
		{"h j k l / arrows", "move; expand/collapse tree nodes"},
		{"Enter", "dive in / open; cycle a day's events"},
		{"Esc / q", "back out / quit"},
	}},
	{"Create (a prefix)", [][2]string{
		{"a t / a T", "add task — quick / full form"},
		{"a e / a E", "add event — quick / full form"},
		{"a s / a S", "add subtask — quick / full form"},
		{"a c / a l", "new calendar / task list"},
	}},
	{"Edit & organize", [][2]string{
		{"e", "edit selected (full form)"},
		{"d", "delete (item, or calendar/list when its panel is focused)"},
		{"Space", "toggle task done"},
		{"H / L", "outdent / indent task (re-parent)"},
		{"u", "undo last local change"},
		{".", "show/hide completed tasks"},
	}},
	{"Calendar", [][2]string{
		{"v", "cycle month / week / day"},
		{"[ / ]", "cycle highlighted calendar"},
		{"n / p", "next / previous period"},
		{"t", "jump to today"},
	}},
	{"Sync & commands", [][2]string{
		{"r", "sync now (= :sync)"},
		{": ", "command line — :sync :view :goto :conflicts :help :q"},
		{":conflicts", "resolve items that changed on both sides"},
		{"?", "this help"},
	}},
}

// showHelp opens the scrollable cheat-sheet overlay. Esc/q/? closes it.
func (a *app) showHelp() {
	var b strings.Builder
	for _, sec := range helpSections {
		b.WriteString("[::b]" + sec.title + "[::-]\n")
		for _, r := range sec.rows {
			b.WriteString("  [yellow]" + pad(r[0], 18) + "[-] " + r[1] + "\n")
		}
		b.WriteString("\n")
	}

	tv := tview.NewTextView().SetDynamicColors(true).SetScrollable(true).SetText(strings.TrimRight(b.String(), "\n"))
	tv.SetBackgroundColor(tcell.ColorDefault)
	tv.SetBorder(true).SetBorderColor(accentColor)
	tv.SetTitle(" Help — keys & commands (Esc to close) ").SetTitleColor(accentColor)
	tv.SetInputCapture(func(ev *tcell.EventKey) *tcell.EventKey {
		if ev.Key() == tcell.KeyEscape || ev.Rune() == 'q' || ev.Rune() == '?' {
			a.closeModal(pageHelp)
			return nil
		}
		return ev // let the TextView scroll (j/k, arrows, PgUp/PgDn)
	})

	a.captureFocus()
	a.root.AddPage(pageHelp, modalWrap(tv, 72, 24), true, true)
	a.tv.SetFocus(tv)
}

// pad right-pads s to width for column alignment in the help sheet.
func pad(s string, width int) string {
	if len(s) >= width {
		return s
	}
	return s + strings.Repeat(" ", width-len(s))
}
