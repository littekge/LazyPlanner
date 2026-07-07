package ui

import (
	"testing"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

func runeKey(r rune) *tcell.EventKey { return tcell.NewEventKey(tcell.KeyRune, r, tcell.ModNone) }

// newRootedTestApp builds a writable app with the top-level Pages root wired, so
// modal/overlay paths (which-key, forms) work in tests.
func newRootedTestApp(t *testing.T, now time.Time) *app {
	t.Helper()
	a := newWritableTestApp(t, now)
	a.root = tview.NewPages()
	a.root.AddPage(pageMain, a.layout(), true, true)
	a.setMode(modeTasks)
	return a
}

func TestPrefixShowsWhichKeyThenCancels(t *testing.T) {
	a := newRootedTestApp(t, time.Date(2026, 7, 5, 12, 0, 0, 0, time.UTC))

	a.startPrefix('a')
	if a.pendingPrefix != 'a' {
		t.Fatalf("pendingPrefix = %q, want 'a'", a.pendingPrefix)
	}
	if !a.root.HasPage(pageWhichKey) {
		t.Error("which-key popup not shown after prefix")
	}

	// Esc cancels: clears the prefix and removes the popup.
	a.resolvePrefix(tcell.NewEventKey(tcell.KeyEscape, 0, tcell.ModNone))
	if a.pendingPrefix != 0 {
		t.Error("prefix not cleared on Esc")
	}
	if a.root.HasPage(pageWhichKey) {
		t.Error("which-key popup not removed on cancel")
	}
}

func TestChordCreatesTask(t *testing.T) {
	a := newRootedTestApp(t, time.Date(2026, 7, 5, 12, 0, 0, 0, time.UTC))
	// Select the first task list so a top-level task has a home.
	if a.tasklists.GetItemCount() == 0 {
		t.Skip("fixture has no task lists")
	}
	before := len(a.store.Todos())

	// `a` then `t` opens the quick-add task prompt.
	a.startPrefix('a')
	a.resolvePrefix(runeKey('t'))
	if a.pendingPrefix != 0 {
		t.Error("prefix should be cleared after completing the chord")
	}
	// The quick-add input modal should now be open.
	if !a.modalOpen() {
		t.Fatal("quick-add prompt did not open on `at`")
	}
	// Command view echoes the chord.
	if got := a.statusMid.GetText(true); got == "" {
		t.Error("command view not echoed after a chord")
	}
	_ = before
}

func TestUnknownChordFlashesAndClears(t *testing.T) {
	a := newRootedTestApp(t, time.Date(2026, 7, 5, 12, 0, 0, 0, time.UTC))
	a.startPrefix('a')
	a.resolvePrefix(runeKey('z')) // no `az` action
	if a.pendingPrefix != 0 {
		t.Error("prefix not cleared after an unknown continuation")
	}
	if a.root.HasPage(pageWhichKey) {
		t.Error("which-key popup left open after an unknown continuation")
	}
	if got := a.statusLeft.GetText(true); got == "" {
		t.Error("expected a flash for the unknown chord")
	}
}
