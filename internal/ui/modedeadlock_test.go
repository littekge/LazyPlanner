package ui

import (
	"sync"
	"testing"
	"time"

	"github.com/gdamore/tcell/v2"
)

// TestModeIndicatorNoLiveDeadlock reproduces the "hangs the second I press t"
// freeze: the mode-indicator draw func must not call anything that takes the
// tview app lock (e.g. Application.GetFocus), because Application.draw() already
// holds that lock — a reentrant lock self-deadlocks. This runs the real event
// loop in Tasks mode (where the indicator exercises the focus check) against a
// simulation screen; a completed draw signals via SetAfterDrawFunc. If the draw
// deadlocks it never fires and the watchdog trips.
func TestModeIndicatorNoLiveDeadlock(t *testing.T) {
	a := newRootedTestApp(t, time.Date(2026, 7, 5, 9, 0, 0, 0, time.UTC)) // mode = Tasks

	screen := tcell.NewSimulationScreen("")
	if err := screen.Init(); err != nil {
		t.Fatalf("init screen: %v", err)
	}
	screen.SetSize(80, 24)
	a.tv.SetScreen(screen)
	a.tv.SetRoot(a.root, true).SetInputCapture(a.globalKeys)

	drawn := make(chan struct{})
	var once sync.Once
	a.tv.SetAfterDrawFunc(func(tcell.Screen) { once.Do(func() { close(drawn) }) })

	runErr := make(chan error, 1)
	go func() { runErr <- a.tv.Run() }()

	select {
	case <-drawn: // a full draw completed in Tasks mode → no deadlock
	case <-time.After(5 * time.Second):
		t.Fatal("DEADLOCK: draw did not complete in Tasks mode — the mode indicator " +
			"took the app lock during a draw (GetFocus under Application.draw's lock)")
	}

	a.tv.Stop()
	select {
	case <-runErr:
	case <-time.After(5 * time.Second):
		t.Fatal("app did not stop after Stop()")
	}
}
