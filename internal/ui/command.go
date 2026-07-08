package ui

import (
	"context"
	"strings"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"

	"github.com/littekge/LazyPlanner/internal/model"
	"github.com/littekge/LazyPlanner/internal/sync"
)

const pageCommand = "command"

// openCommandLine shows the `:` command input near the top of the screen,
// optionally prefilled (e.g. "goto "). Enter runs the command, Esc cancels.
func (a *app) openCommandLine(prefill string) {
	in := tview.NewInputField().SetLabel(":")
	in.SetText(prefill)
	// Shared popup look: terminal-default background, accent border.
	in.SetFieldBackgroundColor(tcell.ColorDefault)
	in.SetFieldTextColor(tcell.ColorDefault)
	in.SetLabelColor(accentColor)
	in.SetBackgroundColor(tcell.ColorDefault)
	in.SetBorder(true).SetBorderColor(accentColor)
	in.SetTitle(" command ").SetTitleColor(accentColor)
	in.SetDoneFunc(func(key tcell.Key) {
		switch key {
		case tcell.KeyEnter:
			line := in.GetText()
			a.closeModal(pageCommand)
			a.runCommand(line)
		case tcell.KeyEscape:
			a.closeModal(pageCommand)
		}
	})

	a.captureFocus()
	a.root.AddPage(pageCommand, topLineWrap(in), true, true)
	a.tv.SetFocus(in)
}

// runCommand parses and dispatches a `:` command line.
func (a *app) runCommand(line string) {
	line = strings.TrimSpace(line)
	line = strings.TrimPrefix(line, ":") // tolerate a leading colon
	if line == "" {
		return
	}
	name, args, _ := strings.Cut(line, " ")
	args = strings.TrimSpace(args)

	switch name {
	case "sync":
		a.triggerSync()
		a.echo(":sync")
	case "q", "quit":
		a.tv.Stop()
	case "view":
		a.cmdView(args)
	case "goto":
		a.cmdGoto(args)
	case "search", "find":
		if args == "" {
			a.flash("search <text>")
			return
		}
		a.runSearch(args)
		if a.searchQuery != "" {
			a.setFocus(a.searchWidget())
		}
		a.echo(":search " + args)
	case "config":
		a.cmdConfig()
	case "conflicts", "conflict":
		a.showConflicts()
	case "help", "h":
		a.showHelp()
		a.echo(":help")
	default:
		a.flash("unknown command: " + name)
	}
}

// cmdConfig opens the config file in $EDITOR (via the callback wired from main),
// suspending the TUI so the editor owns the terminal, then reloads on exit.
func (a *app) cmdConfig() {
	a.echo(":config")
	if a.editConfig == nil {
		a.flash(":config unavailable (no config file)")
		return
	}
	// Suspend releases the screen for the editor; applyConfigReload runs inside so
	// the swap + flash happen before the TUI redraws on resume.
	a.tv.Suspend(func() {
		newSync, err := a.editConfig()
		a.applyConfigReload(newSync, err)
	})
}

// applyConfigReload swaps in a reloaded sync closure (if any) or surfaces the
// reload error. Split out so it is testable without a running application.
func (a *app) applyConfigReload(newSync func(context.Context) (sync.SyncResult, error), err error) {
	if err != nil {
		a.flash("config: " + err.Error())
		return
	}
	if newSync != nil {
		a.syncFn = newSync
	}
	a.flash("config reloaded")
}

// cmdView switches the calendar view (month|week|day).
func (a *app) cmdView(arg string) {
	views := map[string]int{"month": viewMonth, "week": viewWeek, "day": viewDay}
	v, ok := views[strings.ToLower(arg)]
	if !ok {
		a.flash("view: month | week | day")
		return
	}
	a.viewMode = v
	if a.mode != modeCalendar {
		a.setMode(modeCalendar)
	} else {
		a.buildCenterCalendar()
		a.refocusCalendar()
	}
	a.updateStatus()
	a.echo(":view " + arg)
}

// cmdGoto jumps the calendar to a smart-parsed date and shows it.
func (a *app) cmdGoto(arg string) {
	if arg == "" {
		a.flash("goto <date> (e.g. 'jul 20', 'tomorrow', 2026-07-20)")
		return
	}
	qa := model.ParseQuickAdd(arg, a.now, a.loc)
	if !qa.HasDate {
		a.flash("goto: couldn't read a date from " + arg)
		return
	}
	day, _ := qa.At(model.DayStart(a.now), a.loc)
	a.anchor = model.DayStart(day)
	if a.mode != modeCalendar {
		a.setMode(modeCalendar)
	} else {
		a.buildCenterCalendar()
		a.refocusCalendar()
	}
	a.updateStatus()
	a.echo(":goto " + arg)
}

// topLineWrap pins a primitive to the top of the screen, full width.
func topLineWrap(p tview.Primitive) tview.Primitive {
	return tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(p, 3, 0, true).
		AddItem(nil, 0, 1, false)
}
