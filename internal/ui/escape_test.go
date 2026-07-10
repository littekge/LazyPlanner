package ui

import (
	"strings"
	"testing"
	"time"

	"github.com/littekge/LazyPlanner/internal/model"
)

// TestDetailEscapesTagLikeText: user text containing [brackets] must render
// literally in the detail pane, not be swallowed by tview's style-tag parser.
func TestDetailEscapesTagLikeText(t *testing.T) {
	a := newTestApp(t, time.Date(2026, 7, 5, 12, 0, 0, 0, time.UTC))
	a.setTodoDetail(&model.Todo{UID: "x", Summary: "Meeting [urgent] notes"})
	out := renderPrimitive(t, a.detail, 50, 14)
	if !strings.Contains(out, "[urgent]") {
		t.Errorf("detail should show literal [urgent]; got:\n%s", out)
	}
}

// TestTreeLabelEscapesTagLikeText: a task named with [brackets] renders the
// bracketed text literally in the tree node label.
func TestTreeLabelEscapesTagLikeText(t *testing.T) {
	a := newTestApp(t, time.Date(2026, 7, 5, 12, 0, 0, 0, time.UTC))
	label := a.nodeLabel(&model.Todo{UID: "y", Summary: "Review [draft] copy"}, false)
	// tview.Escape turns "[draft]" into "[draft[]" so the parser emits it literally.
	if !strings.Contains(label, "[draft[]") {
		t.Errorf("nodeLabel should escape the bracketed text; got %q", label)
	}
}
