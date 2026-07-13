package sync_test

import (
	"bytes"
	"context"
	"fmt"
	"testing"

	"github.com/emersion/go-ical"

	"github.com/littekge/LazyPlanner/internal/caldav"
	"github.com/littekge/LazyPlanner/internal/store"
	"github.com/littekge/LazyPlanner/internal/sync"
)

func icalFor(uid, summary string) *ical.Calendar {
	cal, err := ical.NewDecoder(bytes.NewReader([]byte(eventICS(uid, summary)))).Decode()
	if err != nil {
		panic(err)
	}
	return cal
}

// BenchmarkInitialSyncPull times a first-time pull of a calendar with N server
// resources into an empty store — the scenario that exposed the O(N²) sidecar
// rewrite (each pulled resource re-serialized the whole calendar's sidecar).
// Compare ns/op across N: quadratic before the batch-flush fix, linear after.
func BenchmarkInitialSyncPull(b *testing.B) {
	for _, n := range []int{100, 400, 1000} {
		b.Run(fmt.Sprintf("n=%d", n), func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				b.StopTimer()
				srv := newFakeServer()
				for j := 0; j < n; j++ {
					uid := fmt.Sprintf("e%d@test", j)
					href := calPath + store.ResourceName(uid)
					srv.data[href] = caldav.Object{Path: href, ETag: "srv-0", Data: icalFor(uid, "seed")}
				}
				st, err := store.Open(context.Background(), b.TempDir())
				if err != nil {
					b.Fatal(err)
				}
				if err := st.SetCalendarMeta(context.Background(), "personal",
					store.CalendarMeta{DisplayName: "Personal", Href: calPath}); err != nil {
					b.Fatal(err)
				}
				b.StartTimer()

				res, err := sync.Sync(context.Background(), srv, st)
				if err != nil {
					b.Fatal(err)
				}
				if res.Pulled != n {
					b.Fatalf("pulled %d, want %d", res.Pulled, n)
				}
			}
		})
	}
}
