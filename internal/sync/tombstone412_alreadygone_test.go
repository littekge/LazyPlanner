package sync_test

import (
	"context"
	"testing"

	"github.com/littekge/LazyPlanner/internal/caldav"
	"github.com/littekge/LazyPlanner/internal/store"
	"github.com/littekge/LazyPlanner/internal/sync"
)

// Pass-24 MED. TestTombstone412WhenAlreadyGoneConverges guards the wedged-tombstone bug:
// the resource is ALREADY GONE on the server (deleted from NextCloud web / a
// phone, or our own DELETE landed but its response was lost), then deleted
// locally. RFC 7232 says a conditional request whose If-Match cannot match a
// non-existent resource gets 412, and real servers do exactly that.
//
// The download is complete and healthy (bulk succeeded, nothing unfetched), so
// the href's absence from serverByHref means "gone", not "couldn't fetch". But
// pushDelete only sees serverByHref and treats both the same: it records a skip
// and keeps the tombstone. The pending delete therefore never converges, and
// because the skip suppresses the CTag cache the calendar re-downloads in full
// on every sync forever.
func TestTombstone412WhenAlreadyGoneConverges(t *testing.T) {
	ctx := context.Background()
	st := newStore(t)
	srv := newFakeServer()
	srv.cals[0].CTag = "ctag-1"

	name := store.ResourceName("e1@test")
	href := calPath + name
	// Local cache has the resource as previously synced...
	if _, err := st.PutRemote(ctx, "personal", name, mkParsed(t, eventICS("e1@test", "Base")), "srv-1", href); err != nil {
		t.Fatal(err)
	}
	// ...but the server no longer has it (srv.data stays empty), and answers the
	// conditional DELETE with 412 rather than 404.
	srv.failDel[href] = caldav.ErrPreconditionFailed

	if err := st.Delete(ctx, "personal", name); err != nil {
		t.Fatal(err)
	}

	for pass := 1; pass <= 3; pass++ {
		res, err := sync.Sync(ctx, srv, st)
		if err != nil {
			t.Fatalf("pass %d: %v", pass, err)
		}
		t.Logf("pass %d: deletes=%d tombstones=%d Conflicts=%d PushedDeletes=%d Skipped=%d CTag=%q HasLocalChanges=%v",
			pass, srv.deletes, len(st.Tombstones()), res.Conflicts, res.PushedDeletes,
			len(res.Skipped), st.CalendarCTag("personal"), st.HasLocalChanges("personal"))
		for _, s := range res.Skipped {
			t.Logf("  pass %d skip: %v", pass, s.Err)
		}
	}

	if n := len(st.Tombstones()); n != 0 {
		t.Errorf("BUG: tombstone still pending after 3 syncs (%d left) — the delete never converges", n)
	}
	if got := st.CalendarCTag("personal"); got != "ctag-1" {
		t.Errorf("BUG: CTag never cached (%q) — the calendar re-downloads in full on every sync", got)
	}
	if st.HasLocalChanges("personal") {
		t.Errorf("BUG: calendar still reports local changes — the CTag short-circuit stays disabled")
	}
}
