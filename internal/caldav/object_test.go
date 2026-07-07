package caldav_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/littekge/LazyPlanner/internal/caldav"
)

// sampleICS is an encoded iCalendar body; PutObject takes bytes so the caldav
// package stays free of iCalendar parsing on the write path.
var sampleICS = []byte(`BEGIN:VCALENDAR
VERSION:2.0
PRODID:-//test//EN
BEGIN:VEVENT
UID:e1@test
DTSTAMP:20260701T120000Z
DTSTART:20260704T130000Z
DTEND:20260704T133000Z
SUMMARY:Sample
END:VEVENT
END:VCALENDAR
`)

func TestPutObjectCreate(t *testing.T) {
	var gotMethod, gotIfNoneMatch, gotIfMatch string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotIfNoneMatch = r.Header.Get("If-None-Match")
		gotIfMatch = r.Header.Get("If-Match")
		w.Header().Set("ETag", `"srv-new"`)
		w.WriteHeader(http.StatusCreated)
	}))
	defer srv.Close()

	c, _ := caldav.NewClient(caldav.Config{Endpoint: srv.URL})
	etag, err := c.PutObject(context.Background(), "/dav/cal/e1.ics", sampleICS, "", true)
	if err != nil {
		t.Fatal(err)
	}
	if gotMethod != http.MethodPut {
		t.Errorf("method = %q, want PUT", gotMethod)
	}
	if gotIfNoneMatch != "*" {
		t.Errorf("If-None-Match = %q, want *", gotIfNoneMatch)
	}
	if gotIfMatch != "" {
		t.Errorf("If-Match = %q, want empty on create", gotIfMatch)
	}
	if etag != "srv-new" {
		t.Errorf("returned etag = %q, want bare srv-new", etag)
	}
}

func TestPutObjectUpdateSendsQuotedIfMatch(t *testing.T) {
	var gotIfMatch string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotIfMatch = r.Header.Get("If-Match")
		w.Header().Set("ETag", `"srv-2"`)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	c, _ := caldav.NewClient(caldav.Config{Endpoint: srv.URL})
	// The store holds a bare etag; PutObject must quote it for the header.
	etag, err := c.PutObject(context.Background(), "/dav/cal/e1.ics", sampleICS, "srv-1", false)
	if err != nil {
		t.Fatal(err)
	}
	if gotIfMatch != `"srv-1"` {
		t.Errorf("If-Match = %q, want quoted \"srv-1\"", gotIfMatch)
	}
	if etag != "srv-2" {
		t.Errorf("returned etag = %q, want srv-2", etag)
	}
}

func TestPutObjectPreconditionFailed(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusPreconditionFailed)
	}))
	defer srv.Close()

	c, _ := caldav.NewClient(caldav.Config{Endpoint: srv.URL})
	_, err := c.PutObject(context.Background(), "/dav/cal/e1.ics", sampleICS, "srv-1", false)
	if err != caldav.ErrPreconditionFailed {
		t.Errorf("err = %v, want ErrPreconditionFailed", err)
	}
}

func TestDeleteObject(t *testing.T) {
	var gotMethod, gotIfMatch string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotIfMatch = r.Header.Get("If-Match")
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	c, _ := caldav.NewClient(caldav.Config{Endpoint: srv.URL})
	if err := c.DeleteObject(context.Background(), "/dav/cal/e1.ics", "srv-1"); err != nil {
		t.Fatal(err)
	}
	if gotMethod != http.MethodDelete {
		t.Errorf("method = %q, want DELETE", gotMethod)
	}
	if gotIfMatch != `"srv-1"` {
		t.Errorf("If-Match = %q, want quoted \"srv-1\"", gotIfMatch)
	}
}

func TestDeleteObjectPreconditionFailed(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusPreconditionFailed)
	}))
	defer srv.Close()

	c, _ := caldav.NewClient(caldav.Config{Endpoint: srv.URL})
	if err := c.DeleteObject(context.Background(), "/dav/cal/e1.ics", "srv-1"); err != caldav.ErrPreconditionFailed {
		t.Errorf("err = %v, want ErrPreconditionFailed", err)
	}
}

func TestDeleteObjectNotFoundIsSuccess(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	c, _ := caldav.NewClient(caldav.Config{Endpoint: srv.URL})
	if err := c.DeleteObject(context.Background(), "/dav/cal/gone.ics", ""); err != nil {
		t.Errorf("404 on delete should be idempotent success, got %v", err)
	}
}
