package ui

import (
	"strings"
	"testing"
	"time"

	"github.com/littekge/LazyPlanner/internal/model"
)

func TestFormatHelpers(t *testing.T) {
	tm := time.Date(2026, 7, 4, 14, 32, 0, 0, time.UTC)
	if got := clockStr(tm, false); got != "2:32pm" {
		t.Errorf("clockStr 12h = %q, want 2:32pm", got)
	}
	if got := clockStr(tm, true); got != "14:32" {
		t.Errorf("clockStr 24h = %q, want 14:32", got)
	}
	if got := hourAxisLabel(14, false); got != "2pm" {
		t.Errorf("hourAxisLabel 12h = %q, want 2pm", got)
	}
	if got := hourAxisLabel(14, true); got != "14" {
		t.Errorf("hourAxisLabel 24h = %q, want 14", got)
	}
	if got := dateStr(tm, false); got != "07/04/2026" {
		t.Errorf("dateStr US = %q, want 07/04/2026", got)
	}
	if got := dateStr(tm, true); got != "2026-07-04" {
		t.Errorf("dateStr ISO = %q, want 2026-07-04", got)
	}
}

func TestParseAppearance(t *testing.T) {
	if !parseWeekStartMonday("") || !parseWeekStartMonday("monday") || parseWeekStartMonday("sunday") {
		t.Error("parseWeekStartMonday wrong")
	}
	if parseDefaultView("") != viewMonth || parseDefaultView("week") != viewWeek || parseDefaultView("day") != viewDay {
		t.Error("parseDefaultView wrong")
	}
}

// TestClockConfigAppliesToDetail: with 24h + ISO set, the detail pane renders
// times and dates accordingly.
func TestClockConfigAppliesToDetail(t *testing.T) {
	a := newTestApp(t, time.Date(2026, 7, 5, 12, 0, 0, 0, time.UTC))
	a.clock24 = true
	a.dateISO = true
	ev := &model.Event{Summary: "Sync", Start: time.Date(2026, 7, 4, 14, 30, 0, 0, time.Local)}
	a.setEventDetail(ev)
	out := renderPrimitive(t, a.detail, 60, 12)
	if !strings.Contains(out, "14:30") {
		t.Errorf("24h time not applied in detail:\n%s", out)
	}
	if !strings.Contains(out, "2026-07-04") {
		t.Errorf("ISO date not applied in detail:\n%s", out)
	}
}
