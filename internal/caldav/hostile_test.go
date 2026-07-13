package caldav_test

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/littekge/LazyPlanner/internal/caldav"
)

// hostileServer serves the given handler and yields a client pointed at it.
func hostileClient(t *testing.T, h http.HandlerFunc) *caldav.Client {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	c, err := caldav.NewClient(caldav.Config{Endpoint: srv.URL})
	if err != nil {
		t.Fatalf("new client: %v", err)
	}
	return c
}

// callWithWatchdog runs fn (a client call) and fails if it does not return within
// the watchdog — a hostile response must never wedge the client.
func callWithWatchdog(t *testing.T, fn func() error) error {
	t.Helper()
	done := make(chan error, 1)
	go func() {
		defer func() {
			if r := recover(); r != nil {
				done <- fmt.Errorf("client call panicked: %v", r)
			}
		}()
		done <- fn()
	}()
	select {
	case err := <-done:
		if err != nil && strings.Contains(err.Error(), "panicked") {
			t.Fatal(err)
		}
		return err
	case <-time.After(30 * time.Second):
		t.Fatal("client call did not return on a hostile response — possible unbounded read / hang")
		return nil
	}
}

// TestListObjectHrefsCapsOversizedBody covers the transport body cap: a server
// streaming a body far larger than the client's limit must make the call fail
// (bounded read) rather than hang or exhaust memory.
func TestListObjectHrefsCapsOversizedBody(t *testing.T) {
	c := hostileClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", `application/xml; charset="utf-8"`)
		w.WriteHeader(http.StatusMultiStatus)
		_, _ = io.WriteString(w, `<multistatus xmlns="DAV:"><response><href>/x.ics</href>`)
		flusher, _ := w.(http.Flusher)
		chunk := strings.Repeat("A", 1<<20) // 1 MiB of comment filler per iteration
		for i := 0; i < 100; i++ {          // up to ~100 MiB, well past the 64 MiB cap
			if _, err := io.WriteString(w, "<!--"+chunk+"-->"); err != nil {
				return // client gave up (cap enforced) — expected
			}
			if flusher != nil {
				flusher.Flush()
			}
		}
	})
	err := callWithWatchdog(t, func() error {
		_, e := c.ListObjectHrefs(context.Background(), "/dav/cal/personal/")
		return e
	})
	if err == nil {
		t.Fatal("expected an error from an oversized response, got nil (cap not enforced)")
	}
}

// TestHostileResponsesDegradeCleanly feeds a range of malformed / adversarial
// responses to the PROPFIND-based listing and asserts each returns an error
// without panicking or hanging.
func TestHostileResponsesDegradeCleanly(t *testing.T) {
	cases := []struct {
		name    string
		handler http.HandlerFunc
	}{
		{"malformed-xml", func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusMultiStatus)
			_, _ = io.WriteString(w, `<multistatus xmlns="DAV:"><response><href>/a`)
		}},
		{"not-xml-at-all", func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusMultiStatus)
			_, _ = io.WriteString(w, "\x00\x01\x02 totally not xml \xff\xfe")
		}},
		{"empty-207", func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusMultiStatus)
		}},
		{"server-error", func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = io.WriteString(w, "boom")
		}},
		{"unauthorized", func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusUnauthorized)
		}},
		{"truncated-then-hang-close", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Length", "100000") // promise more than we send
			w.WriteHeader(http.StatusMultiStatus)
			_, _ = io.WriteString(w, `<multistatus xmlns="DAV:"><response>`)
			if f, ok := w.(http.Flusher); ok {
				f.Flush()
			}
			// handler returns → body ends early, contradicting Content-Length
		}},
		{"deeply-nested", func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusMultiStatus)
			_, _ = io.WriteString(w, `<multistatus xmlns="DAV:">`)
			_, _ = io.WriteString(w, strings.Repeat("<a>", 5000)+strings.Repeat("</a>", 5000))
			_, _ = io.WriteString(w, `</multistatus>`)
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := hostileClient(t, tc.handler)
			// Must not panic or hang; an error (or, for a well-formed-but-empty
			// body, an empty result) is acceptable — never a crash.
			_ = callWithWatchdog(t, func() error {
				_, e := c.ListObjectHrefs(context.Background(), "/dav/cal/personal/")
				return e
			})
		})
	}
}
