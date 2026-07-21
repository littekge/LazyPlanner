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
		{"c t a", "focus Calendars / Tasks / Agenda"},
		{"Tab / Shift-Tab", "cycle panels"},
		{"[ / ]", "cycle highlighted calendar (any mode)"},
		{"{ / }", "cycle highlighted task list (any mode)"},
		{"h j k l / arrows", "move the highlight (Enter expands/collapses a tree node)"},
		{"3j 5k …", "count prefix — repeat a motion"},
		{"g g / G", "go to top / bottom"},
		{"/ then n / N", "search; next / prev match"},
		{"Enter", "dive in / open; cycle a day's events"},
		{"Esc / q", "back out / quit"},
		{"(mode badge)", "status-bar badge shows the input mode: NORMAL · DRILL (drilled into a calendar day) · GRAB · RESIZE"},
	}},
	{"Create (i prefix)", [][2]string{
		{"i t / i T", "add task — quick / full form"},
		{"i e / i E", "add event — quick / full form"},
		{"i s / i S", "add subtask — quick / full form"},
		{"i c / i l", "new calendar / task list"},
		{"i ! e / i ! t", "force-create on an unknown-type [?] calendar"},
	}},
	{"Edit & organize", [][2]string{
		{"e", "edit selected (full form); on the Calendars/Tasks pane, edit the calendar/list (name + color)"},
		{"s p / s d", "quick-set task priority / due date"},
		{"d", "delete (item, or calendar/list when its panel is focused)"},
		{"e / d on recurring", "prompts scope: this occurrence / this & future / all"},
		{"Space", "toggle task done (hide/show calendar in Calendar view)"},
		{"H / L", "outdent / indent task (re-parent)"},
		{"> / <", "zoom into / out of the selected task's subtree"},
		{"y / Y", "cut / copy a task (with its subtree) to the clipboard"},
		{"p / P", "paste under the selected task / at the list top level (clipboard persists)"},
		{"m", "grab: move an event (hjkl day/hour, J/K resize) or nudge a task's due date — Enter keep, Esc cancel; a recurring event prompts this / this & future / all"},
		{"Space on recurring task", "advances to its next occurrence (completes the last one)"},
		{"z R / z M / z a", "fold — expand all / collapse all / toggle"},
		{"u", "undo last local change"},
		{".", "show/hide completed tasks"},
	}},
	{"Calendar", [][2]string{
		{"v", "cycle month / week / day"},
		{"Space", "hide / show the highlighted calendar"},
		{"f / b", "forward / back one period"},
		{"g t", "jump to today"},
		{"g d", "go to date (smart-parsed)"},
	}},
	{"Layout", [][2]string{
		{"+ / - / 0", "week/day: zoom hour height in/out · 0 = auto-fit (remembered) · else: +/- collapse / restore the overview (accordion)"},
		{"Ctrl-← / Ctrl-→", "narrow / widen the overview column"},
		{"Ctrl-W", "resize sub-mode: ←/→ overview · H/L Detail · Enter keep · Esc cancel"},
	}},
	{"Sync & commands", [][2]string{
		{"r", "sync now (= :sync)"},
		{": ", "cmd — :sync :view :goto :search :config :calendar :account :conflicts :q"},
		{":config", "edit config in $EDITOR, reload on exit"},
		{":calendar", "new / rename / color / hide / show (`new` opens the create form; `color` with no hex opens the swatch picker)"},
		{":account", "switch account — `:account <name>`, or bare to pick from a list (multi-account)"},
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
