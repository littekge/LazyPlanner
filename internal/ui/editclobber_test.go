package ui

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/littekge/LazyPlanner/internal/model"
)

// TestApplyMutationDoesNotClobberConcurrentPull guards pass-13 MED #2: the edit
// form (and every recurrence-scoped save) commit through applyMutation, which used
// the UNGUARDED store.Put. A background sync pull landing while the form was open
// was silently overwritten — the stale Save adopted the pulled ETag while
// persisting the form's stale content, so the next push's CAS matched the server
// and the remote edit was lost with no conflict. applyMutation now version-checks
// an edit (prev != nil) via PutIfUnchanged and skips the write on a mismatch.
func TestApplyMutationDoesNotClobberConcurrentPull(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 7, 5, 9, 0, 0, 0, time.UTC)
	a := newWritableTestApp(t, now)

	const cal, uid = "personal", "clobbertask"
	name := "clobbertask.ics"
	href := "/dav/personal/clobbertask.ics"

	// Seed a clean, synced todo (ETag etag-v1) and reload so the app sees it.
	if _, err := a.store.PullRemote(ctx, cal, name, todoDescObj(t, uid, "Buy groceries", "original note"), "etag-v1", href, nil); err != nil {
		t.Fatalf("seed pull: %v", err)
	}
	a.reload()

	// `e` pressed: the form's Locate captures the current object + snapshot.
	loc, ok := a.store.Locate(uid)
	if !ok {
		t.Fatal("Locate failed")
	}

	// A background sync pull lands while the form is open: the DESCRIPTION changed
	// on another device. expectedPrev == loc.Prev so the guarded pull applies.
	applied, err := a.store.PullRemote(ctx, cal, name, todoDescObj(t, uid, "Buy groceries", "REMOTE EDIT from another device"), "etag-v2", href, loc.Prev)
	if err != nil {
		t.Fatalf("concurrent pull: %v", err)
	}
	if !applied {
		t.Fatal("concurrent pull not applied; interleaving precondition not met")
	}

	// The user hits Save. The draft is seeded from the STALE loc.Object, so it
	// carries the pre-pull "original note"; only the summary changed.
	d := model.TodoDraft{Summary: "Buy groceries and milk", Description: "original note"}
	newObj, err := model.EditTodo(loc.Object, uid, d, a.now, time.UTC)
	if err != nil {
		t.Fatalf("EditTodo: %v", err)
	}

	okApplied, stale := a.applyMutation(loc.CalID, loc.Name, newObj, loc.Prev, "edit task", uid)
	if okApplied {
		t.Error("applyMutation reported success on a stale edit; it must skip the version-checked write")
	}
	if !stale {
		t.Error("applyMutation did not report the write as stale")
	}

	// The remote edit must survive intact and clean (a fixed path skips the stale
	// write, keeping the pulled resource at etag-v2).
	cs, _ := a.store.Calendar(cal)
	var got *model.Todo
	for _, r := range cs.Resources {
		if r.Name == name {
			if r.Dirty {
				t.Errorf("resource marked Dirty after a skipped stale write; the pulled edit should stay clean (ETag=%q)", r.ETag)
			}
			got = findTdDesc(r.Object, uid)
		}
	}
	if got == nil {
		t.Fatal("todo missing after save")
	}
	if !strings.Contains(got.Description, "REMOTE EDIT") {
		t.Errorf("CLOBBER: remote DESCRIPTION overwritten by the stale edit-form Save (got %q)", got.Description)
	}
}
