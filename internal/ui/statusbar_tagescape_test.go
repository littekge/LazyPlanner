package ui

import (
	"strings"
	"testing"
	"time"
)

// TestStatusBarDoesNotSwallowUserTextAsColorTag guards the status-bar tag
// injection defect: statusLeft/statusMid are dynamic-color TextViews, and the
// search/command paths used to concatenate raw user text into them. A query or
// argument containing a bracket run (e.g. "[white:black]") is parsed by tview as
// a style tag, so the text the user actually typed vanished from the rendered
// message and the rest of the status widget was repainted in the injected colors.
//
// GetText(true) strips color tags, i.e. it returns what the terminal renders.
func TestStatusBarDoesNotSwallowUserTextAsColorTag(t *testing.T) {
	now := time.Date(2026, 7, 5, 9, 0, 0, 0, time.UTC)

	cases := []struct {
		name    string
		typed   string // the literal text the user typed
		act     func(a *app, typed string)
		widget  func(a *app) string // rendered (tag-stripped) status text
		context string
	}{
		{
			name:    "search no-match flash",
			typed:   "[white:black]",
			act:     func(a *app, s string) { a.runSearch(s) },
			widget:  func(a *app) string { return a.statusLeft.GetText(true) },
			context: "search.go:76 — a.flash(\"no match: \" + q)",
		},
		{
			name:    "unknown command flash",
			typed:   "[red]bogus",
			act:     func(a *app, s string) { a.runCommand(s) },
			widget:  func(a *app) string { return a.statusLeft.GetText(true) },
			context: "command.go:86 — a.flash(\"unknown command: \" + name)",
		},
		{
			name:    "goto bad-date flash",
			typed:   "[red]nonsense",
			act:     func(a *app, s string) { a.runCommand("goto " + s) },
			widget:  func(a *app) string { return a.statusLeft.GetText(true) },
			context: "command.go:267 — a.flash(\"goto: couldn't read a date from \" + arg)",
		},
		{
			name:    "search command echo",
			typed:   "[red]zzz",
			act:     func(a *app, s string) { a.runCommand("search " + s) },
			widget:  func(a *app) string { return a.statusMid.GetText(true) },
			context: "command.go:73 — a.echo(\":search \" + args)",
		},
		{
			name:    "goto command echo",
			typed:   "tomorrow [red]",
			act:     func(a *app, s string) { a.runCommand("goto " + s) },
			widget:  func(a *app) string { return a.statusMid.GetText(true) },
			context: "command.go:279 — a.echo(\":goto \" + arg)",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			a := newWritableTestApp(t, now)
			a.setMode(modeCalendar)
			a.statusLeft.SetText("")
			a.statusMid.SetText("")

			tc.act(a, tc.typed)

			got := tc.widget(a)
			if !strings.Contains(got, tc.typed) {
				t.Errorf("status bar swallowed the user's text as a color tag\n  %s\n  typed:    %q\n  rendered: %q",
					tc.context, tc.typed, got)
			}
		})
	}
}

// TestStatusBarRendersOrdinaryTextUnchanged is the other half of the tag-escape
// class: escaping must be invisible for text that carries no bracket run. An
// ordinary query or argument renders exactly as typed, with no stray "[]".
func TestStatusBarRendersOrdinaryTextUnchanged(t *testing.T) {
	now := time.Date(2026, 7, 5, 9, 0, 0, 0, time.UTC)

	cases := []struct {
		name   string
		act    func(a *app)
		widget func(a *app) string
		want   string
	}{
		{
			name:   "search no-match flash",
			act:    func(a *app) { a.runSearch("zzznope") },
			widget: func(a *app) string { return a.statusLeft.GetText(true) },
			want:   "no match: zzznope",
		},
		{
			name:   "unknown command flash",
			act:    func(a *app) { a.runCommand("bogus") },
			widget: func(a *app) string { return a.statusLeft.GetText(true) },
			want:   "unknown command: bogus",
		},
		{
			name:   "goto bad-date flash",
			act:    func(a *app) { a.runCommand("goto nonsense") },
			widget: func(a *app) string { return a.statusLeft.GetText(true) },
			want:   "goto: couldn't read a date from nonsense",
		},
		{
			name:   "goto command echo",
			act:    func(a *app) { a.runCommand("goto tomorrow") },
			widget: func(a *app) string { return a.statusMid.GetText(true) },
			want:   ":goto tomorrow",
		},
		{
			name:   "view command echo",
			act:    func(a *app) { a.runCommand("view week") },
			widget: func(a *app) string { return a.statusMid.GetText(true) },
			want:   ":view week",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			a := newWritableTestApp(t, now)
			a.setMode(modeCalendar)
			a.statusLeft.SetText("")
			a.statusMid.SetText("")

			tc.act(a)

			if got := tc.widget(a); got != tc.want {
				t.Errorf("escaping altered ordinary status text\n  got:  %q\n  want: %q", got, tc.want)
			}
		})
	}
}
