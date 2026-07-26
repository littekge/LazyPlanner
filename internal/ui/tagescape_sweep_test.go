package ui

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"

	"github.com/littekge/LazyPlanner/internal/store"
)

// This file is the class-wide sweep behind statusbar_tagescape_test.go (which
// covers the five search.go/command.go sites found first). Root cause: the three
// status-bar TextViews run with SetDynamicColors(true), so tview parses any "[…]"
// run in the text as a style/region tag. Text the user typed, a summary they
// wrote, a calendar name the server owns, or an error string carrying a path all
// reach those widgets by plain concatenation — and a bracket run in any of them
// is swallowed (the words vanish) and repaints the rest of the line.
//
// GetText(true) strips style tags, i.e. it returns what the terminal actually
// renders — so "the hostile text is still in GetText(true)" is exactly the
// assertion "the user can still read what they typed".

// hostileTagTexts are the bracket runs tview would otherwise eat: a color tag, a
// foreground:background tag, a region tag, and the reset tag.
var hostileTagTexts = []string{"[red]", "[white:black]", `[""]`, "[-]"}

// tagEscapeSite is one production path that composes a status-bar message out of
// text it does not control.
type tagEscapeSite struct {
	name string
	site string // file:line of the concatenation, for the failure message
	// act drives the real entry point with hostile embedded, and returns the
	// substring that must survive verbatim in the rendered status bar.
	act    func(t *testing.T, a *app, hostile string) string
	widget func(a *app) string
}

func newTagEscapeApp(t *testing.T) *app {
	t.Helper()
	a := newRootedTestApp(t, time.Date(2026, 7, 5, 9, 0, 0, 0, time.UTC))
	a.loc = time.UTC
	return a
}

// makeCalendar creates a local collection whose display name is attacker text.
// components nil leaves the type unconfirmed (guardComponent's unknown branch).
func makeCalendar(t *testing.T, a *app, id, displayName string, components []string) store.Calendar {
	t.Helper()
	if err := a.store.CreateCalendarLocal(context.Background(), id,
		store.CalendarMeta{DisplayName: displayName, Color: "#3366cc"}, components); err != nil {
		t.Fatalf("create calendar %q: %v", id, err)
	}
	a.reload()
	cal, ok := a.store.Calendar(id)
	if !ok {
		t.Fatalf("calendar %q not in store after create", id)
	}
	return cal
}

// frontCaretForm returns the caretForm on the front page, failing if none is up.
func frontCaretForm(t *testing.T, a *app) *caretForm {
	t.Helper()
	_, front := a.root.GetFrontPage()
	f := findCaretFormIn(front)
	if f == nil {
		t.Fatal("no form on the front page")
	}
	return f
}

// submitPrompt types text into the focused one-line prompt and presses Enter.
func submitPrompt(t *testing.T, a *app, text string) {
	t.Helper()
	in, ok := a.tv.GetFocus().(*tview.InputField)
	if !ok {
		t.Fatalf("prompt input not focused; got %T", a.tv.GetFocus())
	}
	in.SetText(text)
	in.InputHandler()(tcell.NewEventKey(tcell.KeyEnter, 0, tcell.ModNone), func(tview.Primitive) {})
}

func statusLeftText(a *app) string { return a.statusLeft.GetText(true) }

var tagEscapeSites = []tagEscapeSite{
	{
		name: "unknown-type calendar guard",
		site: `calendar.go:74 — a.flash("\"" + cal.DisplayName + "\": unknown type …")`,
		act: func(t *testing.T, a *app, hostile string) string {
			name := "Cal " + hostile + " X"
			makeCalendar(t, a, "unknowntype", name, nil)
			a.guardComponent("unknowntype", compEvent)
			return name
		},
		widget: statusLeftText,
	},
	{
		name: "task-list guard rejects an event",
		site: `calendar.go:82 — a.flash("\"" + cal.DisplayName + "\" is a task list …")`,
		act: func(t *testing.T, a *app, hostile string) string {
			name := "Cal " + hostile + " X"
			makeCalendar(t, a, "tasksonly", name, []string{compTodo})
			a.guardComponent("tasksonly", compEvent)
			return name
		},
		widget: statusLeftText,
	},
	{
		name: "event-calendar guard rejects a task",
		site: `calendar.go:84 — a.flash("\"" + cal.DisplayName + "\" is an event calendar …")`,
		act: func(t *testing.T, a *app, hostile string) string {
			name := "Cal " + hostile + " X"
			makeCalendar(t, a, "eventsonly", name, []string{compEvent})
			a.guardComponent("eventsonly", compTodo)
			return name
		},
		widget: statusLeftText,
	},
	{
		name: "calendar created",
		site: `calendar.go:175 — a.flash(fmt.Sprintf("Created %q …", name))`,
		act: func(t *testing.T, a *app, hostile string) string {
			name := "Cal " + hostile + " X"
			a.showCalendarForm("", 0)
			f := frontCaretForm(t, a)
			formItemByLabel(t, f, "Name").(*tview.InputField).SetText(name)
			pressButton(t, f, "Create")
			return name
		},
		widget: statusLeftText,
	},
	{
		name: "calendar deleted",
		site: `calendar.go:341 — a.flash(fmt.Sprintf("Deleted %q", cal.DisplayName))`,
		act: func(t *testing.T, a *app, hostile string) string {
			name := "Cal " + hostile + " X"
			cal := makeCalendar(t, a, "doomed", name, []string{compTodo})
			f := a.promptDeleteCollection("doomed", cal)
			formItemByLabel(t, f, "Type name to confirm").(*tview.InputField).SetText(name)
			pressButton(t, f, "Delete")
			return name
		},
		widget: statusLeftText,
	},
	{
		name: "yank names the task on the clipboard",
		site: `yankpaste.go:49 — a.flash(verb + name + " — p paste under …")`,
		act: func(t *testing.T, a *app, hostile string) string {
			summary := "Buy " + hostile + " milk"
			calID := a.selectedTasklistID()
			a.createTask(calID, "", summary)
			td := todoBySummary(a.store, summary)
			if td == nil {
				t.Fatalf("task %q not created", summary)
			}
			a.refresh(td.UID)
			a.setClip(false)
			return summary
		},
		widget: statusLeftText,
	},
	{
		name: "flashErr funnel",
		site: `edit.go:1131 — a.flash(action + " failed: " + err.Error())`,
		act: func(t *testing.T, a *app, hostile string) string {
			msg := "open /home/u/" + hostile + "/cal.ics: permission denied"
			a.flashErr("Save", errors.New(msg))
			return msg
		},
		widget: statusLeftText,
	},
	{
		name: "sd rejects an unreadable date",
		site: `quickfield.go:143 — a.flash("due: couldn't read a date from " + text)`,
		act: func(t *testing.T, a *app, hostile string) string {
			calID := a.selectedTasklistID()
			a.createTask(calID, "", "Anchor task")
			td := todoBySummary(a.store, "Anchor task")
			if td == nil {
				t.Fatal("anchor task not created")
			}
			a.refresh(td.UID)
			a.setDuePrompt()
			typed := "zzz" + hostile
			submitPrompt(t, a, typed)
			return typed
		},
		widget: statusLeftText,
	},
	{
		name: "sd relays the quick-add parser warning",
		site: `quickfield.go:139 — a.flash("due: " + qa.Warnings[0])`,
		act: func(t *testing.T, a *app, hostile string) string {
			calID := a.selectedTasklistID()
			a.createTask(calID, "", "Anchor task")
			td := todoBySummary(a.store, "Anchor task")
			if td == nil {
				t.Fatal("anchor task not created")
			}
			a.refresh(td.UID)
			a.setDuePrompt()
			// An unclosed @"…" location is the parser warning that quotes the raw
			// token back at the user, so the hostile run reaches the flash.
			submitPrompt(t, a, `@"`+hostile)
			// The model formats the offending token with %q, which backslash-escapes
			// any quote inside it. That transformation is the parser's, not tview's,
			// and happens with or without tag escaping — so the property under test
			// is that the BRACKET RUN survives, and the expectation mirrors %q.
			return strings.ReplaceAll(hostile, `"`, `\"`)
		},
		widget: statusLeftText,
	},
	{
		name: "undo names the step",
		site: `edit.go:798 — a.flash("Undid " + step.label)`,
		act: func(t *testing.T, a *app, hostile string) string {
			label := "edit " + hostile + " task"
			calID := a.selectedTasklistID()
			a.createTask(calID, "", "Undo me")
			td := todoBySummary(a.store, "Undo me")
			if td == nil {
				t.Fatal("task not created")
			}
			loc, ok := a.store.Locate(td.UID)
			if !ok {
				t.Fatal("task not locatable")
			}
			a.undo = a.undo[:0]
			a.pushUndo(label, td.UID, undoOp{calID: loc.CalID, name: loc.Name, prev: loc.Prev})
			a.undoLast()
			return label
		},
		widget: statusLeftText,
	},
}

// TestStatusTextSurvivesHostileTagRuns is the class guard: every production path
// that concatenates text it does not control into a status-bar message must
// escape it, so a bracket run renders as itself instead of being parsed away.
func TestStatusTextSurvivesHostileTagRuns(t *testing.T) {
	for _, site := range tagEscapeSites {
		for _, hostile := range hostileTagTexts {
			t.Run(site.name+"/"+hostile, func(t *testing.T) {
				a := newTagEscapeApp(t)
				a.statusLeft.SetText("")
				a.statusMid.SetText("")

				want := site.act(t, a, hostile)
				got := site.widget(a)

				if !strings.Contains(got, want) {
					t.Errorf("status bar swallowed uncontrolled text as a style tag\n  %s\n  want substring: %q\n  rendered:       %q",
						site.site, want, got)
				}
			})
		}
	}
}

// TestStatusTextUnchangedForOrdinaryMessages is the other half of the class:
// escaping must be invisible. The same paths, driven with bracket-free text,
// must render byte-identically to the message they always produced — no stray
// "[]" and no doubled bracket.
func TestStatusTextUnchangedForOrdinaryMessages(t *testing.T) {
	cases := []struct {
		name string
		act  func(t *testing.T, a *app)
		want string
	}{
		{
			name: "unknown-type calendar guard",
			act: func(t *testing.T, a *app) {
				makeCalendar(t, a, "unknowntype", "Home", nil)
				a.guardComponent("unknowntype", compEvent)
			},
			want: `"Home": unknown type — sync it first (i! to force)`,
		},
		{
			name: "task-list guard rejects an event",
			act: func(t *testing.T, a *app) {
				makeCalendar(t, a, "tasksonly", "Chores", []string{compTodo})
				a.guardComponent("tasksonly", compEvent)
			},
			want: `"Chores" is a task list — can't add events`,
		},
		{
			name: "event-calendar guard rejects a task",
			act: func(t *testing.T, a *app) {
				makeCalendar(t, a, "eventsonly", "Work", []string{compEvent})
				a.guardComponent("eventsonly", compTodo)
			},
			want: `"Work" is an event calendar — can't add tasks`,
		},
		{
			name: "calendar created",
			act: func(t *testing.T, a *app) {
				a.showCalendarForm("", 0)
				f := frontCaretForm(t, a)
				formItemByLabel(t, f, "Name").(*tview.InputField).SetText("Trips")
				pressButton(t, f, "Create")
			},
			want: `Created "Trips" — syncs on next sync`,
		},
		{
			name: "calendar deleted",
			act: func(t *testing.T, a *app) {
				cal := makeCalendar(t, a, "doomed", "Old list", []string{compTodo})
				f := a.promptDeleteCollection("doomed", cal)
				formItemByLabel(t, f, "Type name to confirm").(*tview.InputField).SetText("Old list")
				pressButton(t, f, "Delete")
			},
			want: `Deleted "Old list"`,
		},
		{
			name: "yank names the task on the clipboard",
			act: func(t *testing.T, a *app) {
				calID := a.selectedTasklistID()
				a.createTask(calID, "", "Buy milk")
				td := todoBySummary(a.store, "Buy milk")
				if td == nil {
					t.Fatal("task not created")
				}
				a.refresh(td.UID)
				a.setClip(false)
			},
			want: `Copied "Buy milk" — p paste under · P paste at top`,
		},
		{
			name: "flashErr funnel",
			act: func(t *testing.T, a *app) {
				a.flashErr("Save", errors.New("disk full"))
			},
			want: "Save failed: disk full",
		},
		{
			name: "sd rejects an unreadable date",
			act: func(t *testing.T, a *app) {
				calID := a.selectedTasklistID()
				a.createTask(calID, "", "Anchor task")
				td := todoBySummary(a.store, "Anchor task")
				if td == nil {
					t.Fatal("anchor task not created")
				}
				a.refresh(td.UID)
				a.setDuePrompt()
				submitPrompt(t, a, "zzznope")
			},
			want: "due: couldn't read a date from zzznope",
		},
		{
			name: "sd relays the quick-add parser warning",
			act: func(t *testing.T, a *app) {
				calID := a.selectedTasklistID()
				a.createTask(calID, "", "Anchor task")
				td := todoBySummary(a.store, "Anchor task")
				if td == nil {
					t.Fatal("anchor task not created")
				}
				a.refresh(td.UID)
				a.setDuePrompt()
				submitPrompt(t, a, "2026-13-45")
			},
			want: `due: "2026-13-45" is not a valid date`,
		},
		{
			name: "undo names the step",
			act: func(t *testing.T, a *app) {
				calID := a.selectedTasklistID()
				a.createTask(calID, "", "Undo me")
				td := todoBySummary(a.store, "Undo me")
				if td == nil {
					t.Fatal("task not created")
				}
				loc, ok := a.store.Locate(td.UID)
				if !ok {
					t.Fatal("task not locatable")
				}
				a.undo = a.undo[:0]
				a.pushUndo("edit task", td.UID, undoOp{calID: loc.CalID, name: loc.Name, prev: loc.Prev})
				a.undoLast()
			},
			want: "Undid edit task",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			a := newTagEscapeApp(t)
			a.statusLeft.SetText("")

			tc.act(t, a)

			if got := a.statusLeft.GetText(true); got != tc.want {
				t.Errorf("escaping altered an ordinary status message\n  got:  %q\n  want: %q", got, tc.want)
			}
		})
	}
}

// TestRecurringAdvanceFlashKeepsItsDeliberateColorTag is why the escape cannot
// live inside flash(): advanceRecurringTodo's two flashes wrap themselves in
// [yellow]…[-] on purpose (main.md: an advance is easy to mistake for a
// complete). A blanket escape in flash/echo would print those tags as literal
// text and lose the accent. The escape therefore belongs at each call site, and
// this site must keep rendering its color.
func TestRecurringAdvanceFlashKeepsItsDeliberateColorTag(t *testing.T) {
	a := newTagEscapeApp(t)
	calID := a.selectedTasklistID()
	a.createTask(calID, "", "water plants daily")

	td := todoBySummary(a.store, "water plants")
	if td == nil {
		t.Fatal("recurring task not created")
	}
	loc, ok := a.store.Locate(td.UID)
	if !ok {
		t.Fatal("recurring task not locatable")
	}
	a.advanceRecurringTodo(loc, td.UID)

	raw := a.statusLeft.GetText(false)
	rendered := a.statusLeft.GetText(true)

	if !strings.HasPrefix(raw, "[yellow]") {
		t.Fatalf("the advance flash lost its deliberate color tag; raw = %q", raw)
	}
	if strings.Contains(rendered, "[yellow]") || strings.Contains(rendered, "[-]") {
		t.Errorf("the advance flash's deliberate tags were escaped into visible text; rendered = %q", rendered)
	}
	if !strings.Contains(rendered, "Recurring task advanced") {
		t.Errorf("unexpected advance flash: %q", rendered)
	}
}
