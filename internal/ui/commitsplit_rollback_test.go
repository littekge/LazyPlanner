package ui

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/littekge/LazyPlanner/internal/model"
	"github.com/littekge/LazyPlanner/internal/store"
)

// TestCommitSplitRollsBackMasterOnFutureWriteFailure guards pass-11 HIGH #2:
// commitSplit writes the capped master first and the new future series second.
// If the second Put fails, the split must be atomic — the master is rolled back
// to its full occurrence set rather than left permanently capped with the tail
// silently lost and no undo step to recover from.
func TestCommitSplitRollsBackMasterOnFutureWriteFailure(t *testing.T) {
	dataDir := t.TempDir()
	s, err := store.Open(context.Background(), dataDir)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	a := newApp(s, "test", time.Date(2026, 7, 6, 12, 0, 0, 0, time.UTC))
	a.build()

	if err := s.CreateCalendarLocal(context.Background(), "ev", store.CalendarMeta{DisplayName: "EV"}, []string{"VEVENT"}); err != nil {
		t.Fatal(err)
	}
	uid := putRecurringEvent(t, a, "ev", "Standup", time.Date(2026, 7, 6, 9, 0, 0, 0, time.UTC), "FREQ=WEEKLY;COUNT=4")
	a.reload()

	// Sanity: the master series expands to 4 occurrences before the split.
	if n := masterOccurrenceCount(t, a, uid); n != 4 {
		t.Fatalf("precondition: master has %d occurrences, want 4", n)
	}

	occ := time.Date(2026, 7, 20, 9, 0, 0, 0, time.UTC) // 3rd instance = split point
	loc, _ := a.store.Locate(uid)
	d := draftFromEvent(findEvent(loc.Object, uid))
	d.Start = occ
	capped, future, err := model.SplitEvent(loc.Object, uid, occ, d, a.now, a.loc)
	if err != nil {
		t.Fatal(err)
	}
	futureUID := future.Events[0].UID

	// Force the SECOND Put (the new future series) to fail deterministically:
	// pre-create a *directory* at the exact file path the store will try to write
	// the future resource to, so writeFileAtomic's rename-over-path fails. The
	// first Put (capping the master, an existing file) is unaffected.
	calDir := filepath.Join(dataDir, "calendars", "ev")
	futurePath := filepath.Join(calDir, store.ResourceName(futureUID))
	if err := os.Mkdir(futurePath, 0o755); err != nil {
		t.Fatalf("planting blocker dir: %v", err)
	}

	a.commitSplit(loc, futureUID, capped, future, "edit this & future", "Split series (u to undo)")

	// Correct behavior after the fix: because the second write (the future series)
	// failed, the whole split must be atomic — the master is rolled back to its
	// full 4 occurrences and the future tail is not silently dropped. The bug leaves
	// the master permanently capped to 2 and the future series absent.
	capN := masterOccurrenceCount(t, a, uid)
	_, futureExists := a.store.Locate(futureUID)
	t.Logf("after failed split: master occurrences=%d, futureExists=%v", capN, futureExists)

	if capN != 4 && !futureExists {
		t.Errorf("data loss: master truncated to %d occurrences and future series absent — the split half-completed with no rollback (want the master restored to 4, or the future series present)", capN)
	}
}
