package ui

import (
	"context"
	"strings"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"

	"github.com/littekge/LazyPlanner/internal/model"
)

const pageCommand = "command"
const pageAccount = "account"

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
	case "account", "acct":
		a.cmdAccount(args)
	case "config":
		a.cmdConfig()
	case "calendar", "cal":
		a.cmdCalendar(args)
	case "conflicts", "conflict":
		a.showConflicts()
	case "help", "h":
		a.showHelp()
		a.echo(":help")
	default:
		a.flash("unknown command: " + name)
	}
}

// cmdAccount handles ":account [name]": with a name it switches directly, bare it
// opens the picker. Switching tears the app down and reopens the named account
// (main's rebuild loop); the cache is per-account, so this is the only safe way.
func (a *app) cmdAccount(args string) {
	a.echo(":account")
	if len(a.accounts) == 0 {
		a.flash("no accounts configured")
		return
	}
	if args == "" {
		a.openAccountPicker()
		return
	}
	a.switchAccount(args)
}

// switchAccount validates a switch target against the configured names
// (case-insensitively) and, unless it's already active, records the request and
// winds the UI down so main reopens it. Shared by the command and the picker.
func (a *app) switchAccount(name string) {
	name = strings.TrimSpace(name)
	match := ""
	for _, n := range a.accounts {
		if strings.EqualFold(n, name) {
			match = n
			break
		}
	}
	if match == "" {
		a.flash("unknown account: " + name)
		return
	}
	if strings.EqualFold(match, a.activeAccount) {
		a.flash("already on " + match)
		return
	}
	a.requestSwitch(match)
}

// requestSwitch records the account to switch to and stops the event loop. Run's
// clean-exit path then cancels any in-flight sync and best-effort-flushes pending
// pushes (the same wind-down as quit) before returning the switch to main.
func (a *app) requestSwitch(name string) {
	a.switchTo = name
	a.tv.Stop()
}

// openAccountPicker shows the configured accounts in a modal list, the active one
// marked; Enter switches, Esc cancels.
func (a *app) openAccountPicker() {
	list := a.accountPickerList()
	a.openModal(pageAccount, list, 40, len(a.accounts)+2)
}

// accountPickerList builds the bordered list of accounts for the picker (split
// out so the display-stress test can draw it directly).
func (a *app) accountPickerList() *tview.List {
	list := tview.NewList().ShowSecondaryText(false)
	list.SetBackgroundColor(tcell.ColorDefault)
	list.SetMainTextColor(tcell.ColorDefault)
	// Reverse-video selection like the app's other lists; tview's default List
	// selected style is terminal-default text on a light bar (white-on-white) under
	// our terminal-default background — see selectionStyle and TestSelectionIsLegible.
	list.SetSelectedStyle(selectionStyle)
	list.SetBorder(true).SetBorderColor(accentColor)
	list.SetTitle(" account ").SetTitleColor(accentColor)
	active := -1
	for i, name := range a.accounts {
		label := name
		if strings.EqualFold(name, a.activeAccount) {
			label += "  (active)"
			active = i
		}
		n := name
		list.AddItem(label, "", 0, func() {
			a.closeModal(pageAccount)
			a.switchAccount(n)
		})
	}
	if active >= 0 {
		list.SetCurrentItem(active)
	}
	list.SetDoneFunc(func() { a.closeModal(pageAccount) }) // Esc cancels
	list.SetInputCapture(func(ev *tcell.EventKey) *tcell.EventKey {
		if ev.Key() == tcell.KeyEscape || ev.Rune() == 'q' {
			a.closeModal(pageAccount)
			return nil
		}
		return modalMotionKey(ev)
	})
	return list
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
		res, err := a.editConfig()
		a.applyConfigReload(res, err)
	})
}

// applyConfigReload applies the reloaded settings (sync closure, color mode) or
// surfaces the reload error. Split out so it is testable without a running
// application.
func (a *app) applyConfigReload(res ConfigReload, err error) {
	if err != nil {
		a.flash("config: " + err.Error())
		return
	}
	// Adopt the reloaded account list so a :config-added/renamed account is visible
	// in the picker and status bar and reachable via :account, without a restart.
	// A reload can't hot-swap the *active* connection (the cache is account-keyed —
	// editConfigFn errors out above for that), but a rename keeps the cache id, so
	// the running account's label may still change.
	if res.Accounts != nil {
		a.accounts = res.Accounts
	}
	if res.ActiveAccount != "" {
		a.activeAccount = res.ActiveAccount
	}
	if res.Sync != nil {
		a.syncFn = res.Sync
	}
	if mode := parseColorMode(res.ColorMode); mode != a.colorMode {
		a.colorMode = mode
		// Rebuild the color index and the Calendars list, whose bullets bake in
		// the color tag; the center views read the index live and repaint on
		// resume. Preserve the highlighted row (a rebuild parks it at the top).
		calIdx := a.calendars.GetCurrentItem()
		a.buildCalendars()
		if calIdx >= 0 && calIdx < a.calendars.GetItemCount() {
			a.calendars.SetCurrentItem(calIdx)
		}
	}
	if res.Warning != "" {
		a.flash("config: " + res.Warning)
		return
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

// cmdCalendar handles ":calendar <sub>" — rename/color push server-owned
// metadata (offline-first, via PROPPATCH on the next sync); hide/show toggle the
// local visibility preference. It acts on the highlighted calendar/list.
func (a *app) cmdCalendar(args string) {
	sub, rest, _ := strings.Cut(args, " ")
	rest = strings.TrimSpace(rest)

	// `new` needs no highlighted calendar — it opens the create form (like `ic`).
	if strings.EqualFold(sub, "new") {
		a.echo(":calendar new")
		a.showCalendarForm("", 0)
		return
	}

	id := a.currentCalendarID()
	if id == "" {
		a.flash("select a calendar first")
		return
	}
	switch strings.ToLower(sub) {
	case "rename":
		if rest == "" {
			a.flash("calendar rename <new name>")
			return
		}
		if !a.guardWrite(id) {
			return
		}
		if err := a.store.UpdateCalendarMeta(context.Background(), id, rest, ""); err != nil {
			a.flashErr("Rename", err)
			return
		}
		a.buildCalendars()
		a.buildTasklists()
		a.scheduleSyncDebounced()
		a.echo(":calendar rename")
		a.flash("Renamed (pushes on next sync)")
	case "color":
		a.echo(":calendar color")
		if rest == "" {
			a.openColorPicker(id) // no hex → the swatch picker
			return
		}
		c, ok := normalizeColor(rest)
		if !ok {
			a.flash("calendar color <#rrggbb>")
			return
		}
		a.applyCalendarColor(id, c)
	case "hide":
		a.hidden[id] = true
		a.afterVisibilityChange()
		a.echo(":calendar hide")
	case "show":
		delete(a.hidden, id)
		a.afterVisibilityChange()
		a.echo(":calendar show")
	default:
		a.flash("calendar new|rename|color|hide|show")
	}
}

// currentCalendarID is the calendar the calendar-level commands act on: the
// selected task list in Tasks mode, otherwise the highlighted calendar.
func (a *app) currentCalendarID() string {
	if a.mode == modeTasks {
		return a.selectedTasklistID()
	}
	return a.selectedCalendarID()
}

// normalizeColor validates a hex color, adding a leading '#'. It accepts #rrggbb
// or #rrggbbaa (the Apple calendar-color forms).
func normalizeColor(s string) (string, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return "", false
	}
	if !strings.HasPrefix(s, "#") {
		s = "#" + s
	}
	if len(s) != 7 && len(s) != 9 {
		return "", false
	}
	for _, r := range s[1:] {
		if !((r >= '0' && r <= '9') || (r >= 'a' && r <= 'f') || (r >= 'A' && r <= 'F')) {
			return "", false
		}
	}
	return s, true
}

// topLineWrap pins a primitive to the top of the screen, full width.
func topLineWrap(p tview.Primitive) tview.Primitive {
	return tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(p, 3, 0, true).
		AddItem(nil, 0, 1, false)
}
