package ui

import (
	"testing"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

func TestQuickAddShouldReprompt(t *testing.T) {
	tests := []struct {
		name        string
		warnings    []string
		text        string
		warnedText  string
		haveWarned  bool
		wantRepromp bool
	}{
		{"no warnings creates", nil, "buy milk", "", false, false},
		{"first warning re-prompts", []string{"w"}, "task !hgh", "", false, true},
		{"identical resubmit accepts", []string{"w"}, "task !hgh", "task !hgh", true, false},
		{"edited text re-prompts again", []string{"w"}, "task !hg", "task !hgh", true, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := quickAddShouldReprompt(tc.warnings, tc.text, tc.warnedText, tc.haveWarned); got != tc.wantRepromp {
				t.Errorf("quickAddShouldReprompt = %v, want %v", got, tc.wantRepromp)
			}
		})
	}
}

// TestQuickAddRepromptFlow drives the real quick-add input: a warned entry must
// not create on the first Enter (modal stays open), but an identical second
// Enter accepts it, while an edit to clean text creates the edited item.
func TestQuickAddRepromptFlow(t *testing.T) {
	now := time.Date(2026, 7, 5, 9, 0, 0, 0, time.UTC)

	t.Run("identical resubmit keeps the warned text", func(t *testing.T) {
		a := newRootedTestApp(t, now)
		a.loc = time.UTC
		a.addTaskQuick()

		in, ok := a.tv.GetFocus().(*tview.InputField)
		if !ok {
			t.Fatalf("quick-add input not focused; got %T", a.tv.GetFocus())
		}
		handle := in.InputHandler()
		in.SetText("call next tuedsay")
		handle(tcell.NewEventKey(tcell.KeyEnter, 0, tcell.ModNone), func(tview.Primitive) {})
		if todoBySummary(a.store, "call next tuedsay") != nil {
			t.Fatal("created despite the warning on first submit")
		}
		// Identical resubmit accepts as-is (failed tokens stay in the title).
		handle(tcell.NewEventKey(tcell.KeyEnter, 0, tcell.ModNone), func(tview.Primitive) {})
		if todoBySummary(a.store, "call next tuedsay") == nil {
			t.Fatal("identical resubmit did not create the task")
		}
	})

	t.Run("editing to clean text creates the edited item", func(t *testing.T) {
		a := newRootedTestApp(t, now)
		a.loc = time.UTC
		a.addTaskQuick()

		in := a.tv.GetFocus().(*tview.InputField)
		handle := in.InputHandler()
		in.SetText("call next tuedsay")
		handle(tcell.NewEventKey(tcell.KeyEnter, 0, tcell.ModNone), func(tview.Primitive) {})
		// Edit to a clean value; it has no warning, so it creates immediately.
		in.SetText("call mom fri")
		handle(tcell.NewEventKey(tcell.KeyEnter, 0, tcell.ModNone), func(tview.Primitive) {})
		if todoBySummary(a.store, "call mom") == nil {
			t.Fatal("edited clean text was not created")
		}
		if todoBySummary(a.store, "call next tuedsay") != nil {
			t.Fatal("the warned text should not have been created")
		}
	})
}
