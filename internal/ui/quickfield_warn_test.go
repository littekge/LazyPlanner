package ui

import (
	"testing"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// TestSetPriorityFlashesParserWarning: like setDuePrompt, a failed sp parse
// that the quick-add parser recognizes as an obvious typo (e.g. "!hgh" for
// "!high") flashes that specific warning instead of the generic hint.
func TestSetPriorityFlashesParserWarning(t *testing.T) {
	now := time.Date(2026, 7, 5, 9, 0, 0, 0, time.UTC)
	a := newRootedTestApp(t, now)
	a.loc = time.UTC
	cal := a.selectedTasklistID()
	a.createTask(cal, "", "Task X")

	a.setPriorityPrompt()
	in, ok := a.tv.GetFocus().(*tview.InputField)
	if !ok {
		t.Fatalf("priority prompt input not focused; got %T", a.tv.GetFocus())
	}
	in.SetText("hgh")
	handle := in.InputHandler()
	handle(tcell.NewEventKey(tcell.KeyEnter, 0, tcell.ModNone), func(tview.Primitive) {})

	got := a.statusLeft.GetText(true)
	want := `priority: "!hgh" looks like a priority (use !1–!9 or !high/!med/!low)`
	if got != want {
		t.Errorf("flash = %q, want %q", got, want)
	}
}

// TestSetPriorityFlashesGenericFallback: a bare unparseable value (no
// obvious-typo warning from the quick-add parser) keeps the generic hint.
func TestSetPriorityFlashesGenericFallback(t *testing.T) {
	now := time.Date(2026, 7, 5, 9, 0, 0, 0, time.UTC)
	a := newRootedTestApp(t, now)
	a.loc = time.UTC
	cal := a.selectedTasklistID()
	a.createTask(cal, "", "Task X")

	a.setPriorityPrompt()
	in, ok := a.tv.GetFocus().(*tview.InputField)
	if !ok {
		t.Fatalf("priority prompt input not focused; got %T", a.tv.GetFocus())
	}
	in.SetText("!!!")
	handle := in.InputHandler()
	handle(tcell.NewEventKey(tcell.KeyEnter, 0, tcell.ModNone), func(tview.Primitive) {})

	got := a.statusLeft.GetText(true)
	want := "priority: 1-9 or high/med/low (blank clears)"
	if got != want {
		t.Errorf("flash = %q, want %q", got, want)
	}
}
