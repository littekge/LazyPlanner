package ui

import (
	"testing"
	"time"
)

func TestResizeLeftClampsAndPersists(t *testing.T) {
	a := newRootedTestApp(t, time.Date(2026, 7, 5, 12, 0, 0, 0, time.UTC))
	var saved int
	a.saveState = func(w int, _ []string, _ int) { saved = w }

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

func TestZoomHourAdjustsClampsAndPersists(t *testing.T) {
	a := newRootedTestApp(t, time.Date(2026, 7, 5, 12, 0, 0, 0, time.UTC))
	var savedRows int
	a.saveState = func(_ int, _ []string, rph int) { savedRows = rph }

	a.zoomHour(1) // from auto-fit (effective 1) → 2
	if a.hourRows != 2 || a.timegrid.rowsPerHour != 2 {
		t.Fatalf("after zoom in: hourRows=%d tg.rowsPerHour=%d, want 2/2", a.hourRows, a.timegrid.rowsPerHour)
	}
	if savedRows != 2 {
		t.Errorf("persisted rows/hour = %d, want 2", savedRows)
	}

	for i := 0; i < 50; i++ {
		a.zoomHour(1)
	}
	if a.hourRows != maxRowsPerHour {
		t.Errorf("zoom in should clamp to %d, got %d", maxRowsPerHour, a.hourRows)
	}
	for i := 0; i < 50; i++ {
		a.zoomHour(-1)
	}
	if a.hourRows != minRowsPerHour {
		t.Errorf("zoom out should clamp to %d, got %d", minRowsPerHour, a.hourRows)
	}
}
