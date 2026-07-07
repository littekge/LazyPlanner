package ui

import (
	"testing"
	"time"
)

func TestResizeLeftClampsAndPersists(t *testing.T) {
	a := newRootedTestApp(t, time.Date(2026, 7, 5, 12, 0, 0, 0, time.UTC))
	var saved int
	a.saveState = func(w int) { saved = w }

	start := a.leftWidth
	a.resizeLeft(leftWidthStep)
	if a.leftWidth != start+leftWidthStep {
		t.Errorf("leftWidth = %d, want %d", a.leftWidth, start+leftWidthStep)
	}
	if saved != a.leftWidth {
		t.Errorf("saveState got %d, want %d", saved, a.leftWidth)
	}

	for i := 0; i < 50; i++ {
		a.resizeLeft(leftWidthStep)
	}
	if a.leftWidth != maxLeftWidth {
		t.Errorf("leftWidth = %d, want clamped to max %d", a.leftWidth, maxLeftWidth)
	}
	for i := 0; i < 50; i++ {
		a.resizeLeft(-leftWidthStep)
	}
	if a.leftWidth != minLeftWidth {
		t.Errorf("leftWidth = %d, want clamped to min %d", a.leftWidth, minLeftWidth)
	}
}

func TestAccordionModeRestores(t *testing.T) {
	a := newRootedTestApp(t, time.Date(2026, 7, 5, 12, 0, 0, 0, time.UTC)) // tasks mode
	a.setAccordion(true)
	if !a.accordion {
		t.Fatal("accordion not set")
	}
	a.setMode(modeCalendar)
	if a.accordion {
		t.Error("switching panels should restore the collapsed overview")
	}
}

func TestAccordionBlockedInAgenda(t *testing.T) {
	a := newRootedTestApp(t, time.Date(2026, 7, 5, 12, 0, 0, 0, time.UTC))
	a.setMode(modeAgenda)
	a.setAccordion(true)
	if a.accordion {
		t.Error("accordion should be blocked in Agenda mode")
	}
}
