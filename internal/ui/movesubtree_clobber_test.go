package ui

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/littekge/LazyPlanner/internal/model"
)

// bundledTodosObj builds one resource holding two co-resident VTODOs: the item a
// test moves away, and a bystander whose DESCRIPTION carries a revision marker.
func bundledTodosObj(t *testing.T, moverUID, stayUID, stayDesc string) *model.Parsed {
	t.Helper()
	ics := "BEGIN:VCALENDAR\r\nVERSION:2.0\r\nPRODID:-//LazyPlanner//Test//EN\r\n" +
		"BEGIN:VTODO\r\nUID:" + moverUID + "\r\nDTSTAMP:20260701T120000Z\r\nSUMMARY:mover\r\nEND:VTODO\r\n" +
		"BEGIN:VTODO\r\nUID:" + stayUID + "\r\nDTSTAMP:20260701T120000Z\r\nSUMMARY:stay\r\nDESCRIPTION:" + stayDesc + "\r\nEND:VTODO\r\n" +
		"END:VCALENDAR\r\n"
	obj, err := model.Decode([]byte(ics), time.UTC)
	if err != nil {
		t.Fatalf("decode bundle: %v", err)
	}
	return obj
}

// TestMoveSubtreeSourceRewriteDoesNotClobberConcurrentPull guards the
// COVERAGE.md-flagged gap: moveSubtreeOps rewrote the source resource of a
// cross-list move with a bare Put. A sync pull updating a co-resident bystander
// between the move's Locate and that Put was silently overwritten — the
// bystander's remote edit vanished with no conflict. The rewrite must
// version-check via PutIfUnchanged (failing the move cleanly instead).
//
// The interleave is a real two-goroutine race (the Locate is internal to the
// loop, so no deterministic seam exists); each iteration races one pull against
// one move. One pull per iteration means no later pull can mask a clobber: if
// the pull applied and the move completed, the bystander must carry rev-1.
func TestMoveSubtreeSourceRewriteDoesNotClobberConcurrentPull(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 7, 5, 9, 0, 0, 0, time.UTC)
	a := newWritableTestApp(t, now)

	// "work" is already present in the store fixture (internal/store/testdata/vdir)
	// and already accepts VTODO writes as a move target — see
	// TestCoResidentMoveDragsBystander in coresident_move_test.go, which pastes into
	// it directly with no CreateCalendarLocal call. Creating it here would collide
	// with the fixture ("calendar already exists"), so we just reuse it.
	const srcCal = "personal"
	const iterations = 300
	for i := 0; i < iterations; i++ {
		mover := fmt.Sprintf("mover-%d", i)
		stay := fmt.Sprintf("stay-%d", i)
		name := fmt.Sprintf("bundle-%d.ics", i)
		href := "/dav/personal/" + name

		if _, err := a.store.PullRemote(ctx, srcCal, name, bundledTodosObj(t, mover, stay, "rev-0"), "etag-0", href, nil); err != nil {
			t.Fatalf("iteration %d: seed pull: %v", i, err)
		}
		a.reload()

		pullApplied := make(chan bool, 1)
		go func() {
			applied, err := a.store.PullRemote(ctx, srcCal, name, bundledTodosObj(t, mover, stay, "rev-1"), "etag-1", href, nil)
			pullApplied <- err == nil && applied
		}()

		a.moveSubtree(mover, "", srcCal, "work")
		applied := <-pullApplied

		moved := false
		if loc, ok := a.store.Locate(mover); ok && loc.CalID == "work" {
			moved = true
		}
		if applied && moved {
			loc, ok := a.store.Locate(stay)
			if !ok {
				t.Fatalf("iteration %d: bystander vanished from the source resource", i)
			}
			td := findTdDesc(loc.Object, stay)
			if td == nil {
				t.Fatalf("iteration %d: bystander todo missing from its resource", i)
			}
			if td.Description != "rev-1" {
				t.Fatalf("iteration %d: CLOBBER — the concurrent pull applied (rev-1) and the move completed, but the bystander reads %q: the stale source-side rewrite overwrote the pulled edit", i, td.Description)
			}
		}
	}
}
