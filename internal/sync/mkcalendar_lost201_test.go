package sync_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/littekge/LazyPlanner/internal/caldav"
	"github.com/littekge/LazyPlanner/internal/store"
	"github.com/littekge/LazyPlanner/internal/sync"
)

// lost201Server models a MKCALENDAR whose 201 Created is lost in transit: the
// server actually creates the collection, but the first attempt returns a
// transport error. A retry against the now-existing collection succeeds
// idempotently — mirroring the fixed caldav.Client, which treats the server's 405
// (already mapped) as success (see caldav.TestCreateCalendarAlreadyExistsIsIdempotent).
type lost201Server struct {
	*fakeServer
	createCalls int
}

func (s *lost201Server) CreateCalendar(_ context.Context, path string, spec caldav.CalendarSpec) error {
	s.createCalls++
	for _, c := range s.fakeServer.cals {
		if c.Path == path {
			// Already exists (server answered 405); the fixed client maps that to nil.
			return nil
		}
	}
	// First attempt: the server creates it, but the 201 never reaches us.
	s.fakeServer.cals = append(s.fakeServer.cals, caldav.Calendar{Path: path, Name: spec.DisplayName})
	return errors.New("caldav: MKCALENDAR: connection reset by peer (201 lost)")
}

// TestMKCalendarLost201RecoversInsteadOfWedging guards pass-13 MED #5: when a
// MKCALENDAR's 201 is lost the collection exists on the server but the local
// calendar stays pending-create; the next sync retried MKCALENDAR and the 405 was
// recorded as a permanent skip, wedging it forever. With the idempotent client the
// retry succeeds and pending-create is cleared.
func TestMKCalendarLost201RecoversInsteadOfWedging(t *testing.T) {
	ctx := context.Background()
	st, err := store.Open(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	srv := &lost201Server{fakeServer: newFakeServer()}
	srv.cals = nil // server starts empty

	if err := st.CreateCalendarLocal(ctx, "mycal", store.CalendarMeta{DisplayName: "My Cal"}, []string{"VEVENT"}); err != nil {
		t.Fatal(err)
	}

	// --- First sync: the 201 is lost, so the create appears to fail. ---
	res1, err := sync.Sync(ctx, srv, st)
	if err != nil {
		t.Fatal(err)
	}
	if res1.CalendarsCreated != 0 {
		t.Fatalf("first sync: CalendarsCreated = %d, want 0 (create appeared to fail)", res1.CalendarsCreated)
	}
	cal, ok := st.Calendar("mycal")
	if !ok {
		t.Fatal("mycal missing after first sync")
	}
	if !cal.PendingCreate {
		t.Fatal("mycal should still be pending-create after the lost 201")
	}

	// --- Second sync: MKCALENDAR is retried against the existing collection. ---
	res2, err := sync.Sync(ctx, srv, st)
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range res2.Skipped {
		if s.Calendar == "mycal" && strings.Contains(s.Err.Error(), "405") {
			t.Errorf("second sync recorded a 405 permanent skip for mycal; the retry must succeed idempotently")
		}
	}
	cal2, _ := st.Calendar("mycal")
	if cal2.PendingCreate {
		t.Errorf("mycal still pending-create after the retry; it wedges and retries MKCALENDAR every sync (createCalls=%d)", srv.createCalls)
	}
	if res2.CalendarsCreated != 1 {
		t.Errorf("second sync: CalendarsCreated = %d, want 1 (the idempotent retry adopts the collection)", res2.CalendarsCreated)
	}
	if srv.createCalls != 2 {
		t.Errorf("expected exactly one retry (createCalls=2), got %d", srv.createCalls)
	}
}
