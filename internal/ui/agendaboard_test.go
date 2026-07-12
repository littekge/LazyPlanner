package ui

import (
	"strings"
	"testing"

	"github.com/gdamore/tcell/v2"

	"github.com/littekge/LazyPlanner/internal/model"
)

// The full-detail agenda board must mark tasks with the same [ ]/[■]/▸ glyphs as
// the tree, month grid, and time-grid — the center board is the one view that
// historically dropped them (cross-view consistency: H1).
func TestAgendaBoardTaskGlyphs(t *testing.T) {
	tests := []struct {
		name   string
		todo   *model.Todo
		folder bool
		want   string
	}{
		{"incomplete", &model.Todo{Summary: "Grade labs"}, false, "[ ] "},
		{"completed", &model.Todo{Summary: "Grade labs", Status: model.StatusCompleted}, false, "[■] "},
		{"folder", &model.Todo{Summary: "Project"}, true, "▸ "},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			it := model.AgendaItem{Title: tt.todo.Summary, Todo: tt.todo}
			lines := agendaItemLines(it, tcell.ColorWhite, false, tt.folder)
			if len(lines) == 0 {
				t.Fatal("no lines rendered")
			}
			title := lines[0].text
			if !strings.Contains(title, tt.want) {
				t.Errorf("title line %q missing marker %q", title, tt.want)
			}
			if !strings.Contains(title, tt.todo.Summary) {
				t.Errorf("title line %q missing summary %q", title, tt.todo.Summary)
			}
		})
	}
}

// Events carry no task marker.
func TestAgendaBoardEventNoGlyph(t *testing.T) {
	it := model.AgendaItem{Title: "Standup", Event: &model.Event{Summary: "Standup"}}
	lines := agendaItemLines(it, tcell.ColorWhite, false, false)
	title := lines[0].text
	for _, mark := range []string{"[ ]", "[■]", "▸"} {
		if strings.Contains(title, mark) {
			t.Errorf("event title %q should carry no task marker, found %q", title, mark)
		}
	}
}
