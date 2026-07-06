package ui

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"

	"github.com/littekge/LazyPlanner/internal/model"
	"github.com/littekge/LazyPlanner/internal/store"
)

// newTestApp builds an app over the store package's shared vdir fixture with a
// fixed clock, wired and loaded but not running the event loop.
func newTestApp(t *testing.T, now time.Time) *app {
	t.Helper()
	s, err := storeFixture(t)
	if err != nil {
		t.Fatalf("open store fixture: %v", err)
	}
	a := newApp(s, "test", now)
	a.build()
	a.reload()
	return a
}

// drawCells renders a primitive to an in-memory screen and returns its cells so
// tests can inspect styles (colors), not just text.
func drawCells(t *testing.T, p tview.Primitive, w, h int) ([]tcell.SimCell, int, int) {
	t.Helper()
	screen := tcell.NewSimulationScreen("")
	if err := screen.Init(); err != nil {
		t.Fatalf("init simulation screen: %v", err)
	}
	defer screen.Fini()
	screen.SetSize(w, h)
	p.SetRect(0, 0, w, h)
	p.Draw(screen)
	screen.Show()
	cells, cw, ch := screen.GetContents()
	return cells, cw, ch
}

// TestAgendaSelectedBlockOutlined guards that the selected agenda item is marked
// with an outline box (matching the month view) rather than a filled bar: the
// title keeps its own color and a rounded box corner is drawn.
func TestAgendaSelectedBlockOutlined(t *testing.T) {
	a := newTestApp(t, time.Date(2026, 7, 5, 12, 0, 0, 0, time.UTC))
	items := a.dayItems(model.DayStart(a.now))
	if len(items) == 0 {
		t.Fatal("fixture has no agenda items on 2026-07-05; test needs a today item")
	}
	title := nonEmpty(items[0].Title, "(untitled)")

	a.buildAgendaCenter() // selection defaults to the first item
	cells, cw, ch := drawCells(t, a.agenda, 80, 24)

	titleRow, hasCorner := -1, false
	for row := 0; row < ch; row++ {
		var line strings.Builder
		for col := 0; col < cw; col++ {
			if c := cells[row*cw+col]; len(c.Runes) > 0 {
				line.WriteRune(c.Runes[0])
			} else {
				line.WriteByte(' ')
			}
		}
		text := line.String()
		if titleRow < 0 && strings.Contains(text, title) {
			titleRow = row
			// The selected title keeps its own foreground (not inverted to black).
			idx := strings.Index(text, title)
			fg, _, _ := cells[row*cw+idx].Style.Decompose()
			if fg == tcell.ColorBlack {
				t.Errorf("selected title should keep its color, got black (inverted)")
			}
		}
		if strings.ContainsRune(text, '╭') || strings.ContainsRune(text, '╰') {
			hasCorner = true
		}
	}
	if titleRow < 0 {
		t.Fatalf("selected title %q not found in agenda render", title)
	}
	if !hasCorner {
		t.Error("selected item is not outlined (no rounded box corner drawn)")
	}
}

// TestNodeLabelCompletedGlyph locks the completed-task indicator to a filled box.
func TestNodeLabelCompletedGlyph(t *testing.T) {
	a := newTestApp(t, time.Date(2026, 7, 5, 12, 0, 0, 0, time.UTC))
	a.treeFolders = map[string]bool{}
	done := &model.Todo{Summary: "Done", Status: model.StatusCompleted}
	todo := &model.Todo{Summary: "Todo"}
	if got := a.nodeLabel(done, false); !strings.HasPrefix(got, "[■] ") {
		t.Errorf("completed label = %q, want a filled box [■]", got)
	}
	if got := a.nodeLabel(todo, false); !strings.HasPrefix(got, "[ ] ") {
		t.Errorf("incomplete label = %q, want an empty box [ ]", got)
	}
}

// TestSelectionIsLegible guards that the highlighted list row uses reverse video
// rather than tview's default selected style (which, with our terminal-default
// background, draws terminal-default text on a light bar — illegible).
func TestSelectionIsLegible(t *testing.T) {
	a := newTestApp(t, time.Date(2026, 7, 5, 12, 0, 0, 0, time.UTC))
	if a.calendars.GetItemCount() == 0 {
		t.Fatal("fixture has no calendars")
	}
	cells, _, _ := drawCells(t, a.calendars, 30, 12)

	reversed := false
	for _, c := range cells {
		if _, _, attr := c.Style.Decompose(); attr&tcell.AttrReverse != 0 {
			reversed = true
			break
		}
	}
	if !reversed {
		t.Error("selected row is not reverse-video — highlight would be illegible on some themes")
	}
}

// TestTextInheritsPaneBackground guards against the "text in a shaded box"
// artifact: a text cell must share the same background as the blank pane around
// it, so the app inherits the terminal's background on any color scheme.
func TestTextInheritsPaneBackground(t *testing.T) {
	a := newTestApp(t, time.Date(2026, 7, 5, 12, 0, 0, 0, time.UTC))
	a.buildAgendaCenter()
	cells, cw, ch := drawCells(t, a.agenda, 80, 24)

	row := -1
	for r := 0; r < ch; r++ {
		line := make([]rune, cw)
		for c := 0; c < cw; c++ {
			line[c] = ' '
			if rs := cells[r*cw+c].Runes; len(rs) > 0 {
				line[c] = rs[0]
			}
		}
		if strings.Contains(string(line), "2026") {
			row = r
			break
		}
	}
	if row < 0 {
		t.Fatal("date header not found")
	}

	var textBg, blankBg tcell.Color
	haveText, haveBlank := false, false
	for c := 1; c < cw-1; c++ { // skip border columns
		cell := cells[row*cw+c]
		_, bg, _ := cell.Style.Decompose()
		r := ' '
		if len(cell.Runes) > 0 {
			r = cell.Runes[0]
		}
		if r != ' ' && !haveText {
			textBg, haveText = bg, true
		} else if r == ' ' && !haveBlank {
			blankBg, haveBlank = bg, true
		}
	}
	if !haveText || !haveBlank {
		t.Fatal("need both a text cell and a blank cell on the header row")
	}
	if textBg != blankBg {
		t.Errorf("text background %v differs from pane background %v — renders as a shaded box", textBg, blankBg)
	}
	if textBg != tcell.ColorDefault {
		t.Errorf("background = %v, want terminal default so it inherits the theme", textBg)
	}
}

// TestTaskTreeRootIsListName guards that top-level tasks attach to the list's
// name (the tree root), instead of dangling from an empty root node.
func TestTaskTreeRootIsListName(t *testing.T) {
	a := newTestApp(t, time.Date(2026, 7, 5, 12, 0, 0, 0, time.UTC))
	if a.tasklists.GetItemCount() == 0 || len(a.tasklistIDs) == 0 {
		t.Fatal("fixture has no task lists")
	}
	a.tasklists.SetCurrentItem(0)
	a.buildTree()

	id := a.selectedTasklistID()
	cal, ok := a.store.Calendar(id)
	if !ok {
		t.Fatalf("selected task list %q not in store", id)
	}
	root := a.tree.GetRoot()
	if got := root.GetText(); got != cal.DisplayName {
		t.Errorf("tree root text = %q, want the list name %q", got, cal.DisplayName)
	}
	if len(root.GetChildren()) == 0 {
		t.Error("expected top-level tasks attached to the root")
	}
}

func storeFixture(t *testing.T) (*store.Store, error) {
	t.Helper()
	return store.Open(context.Background(), "../store/testdata/vdir")
}
