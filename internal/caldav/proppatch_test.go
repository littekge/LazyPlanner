package caldav

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSetCalendarProps(t *testing.T) {
	var gotMethod, gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
		w.WriteHeader(http.StatusMultiStatus)
	}))
	defer srv.Close()

	c, err := NewClient(Config{Endpoint: srv.URL, Username: "u", Password: "p"})
	if err != nil {
		t.Fatal(err)
	}
	if err := c.SetCalendarProps(context.Background(), "/dav/cal/work/", "Work Stuff", "#ff8800"); err != nil {
		t.Fatalf("SetCalendarProps: %v", err)
	}
	if gotMethod != "PROPPATCH" {
		t.Errorf("method = %q, want PROPPATCH", gotMethod)
	}
	for _, want := range []string{"propertyupdate", "displayname", "Work Stuff", "calendar-color", "#ff8800"} {
		if !strings.Contains(gotBody, want) {
			t.Errorf("PROPPATCH body missing %q\nbody: %s", want, gotBody)
		}
	}
}

func TestSetCalendarPropsErrorSurfacesStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte("nope"))
	}))
	defer srv.Close()

	c, err := NewClient(Config{Endpoint: srv.URL, Username: "u", Password: "p"})
	if err != nil {
		t.Fatal(err)
	}
	err = c.SetCalendarProps(context.Background(), "/dav/cal/work/", "X", "")
	if err == nil || !strings.Contains(err.Error(), "403") {
		t.Errorf("expected a 403 error, got %v", err)
	}
}

// TestSetCalendarPropsRejected207 is the Pass-21 MED regression guard: a server
// can answer a 207 Multi-Status whose per-property <status> REJECTS the change
// (403/409/…). Treating the 207's HTTP line alone as success silently drops the
// rename/recolor while the local offline edit is marked pushed — a lost update.
// SetCalendarProps must inspect the propstat status and surface the rejection.
func TestSetCalendarPropsRejected207(t *testing.T) {
	const body = `<?xml version="1.0"?>
<D:multistatus xmlns:D="DAV:">
  <D:response>
    <D:href>/dav/cal/work/</D:href>
    <D:propstat>
      <D:prop><D:displayname/></D:prop>
      <D:status>HTTP/1.1 403 Forbidden</D:status>
    </D:propstat>
  </D:response>
</D:multistatus>`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", `application/xml; charset="utf-8"`)
		w.WriteHeader(http.StatusMultiStatus)
		_, _ = w.Write([]byte(body))
	}))
	defer srv.Close()

	c, err := NewClient(Config{Endpoint: srv.URL, Username: "u", Password: "p"})
	if err != nil {
		t.Fatal(err)
	}
	err = c.SetCalendarProps(context.Background(), "/dav/cal/work/", "Work Stuff", "")
	if err == nil {
		t.Fatal("a 207 rejecting the property change must return an error, got nil (silent lost update)")
	}
	if !strings.Contains(err.Error(), "403") {
		t.Errorf("error should surface the rejected status, got %v", err)
	}
}

// TestSetCalendarPropsAccepted207 pins the other half: a 207 whose propstats all
// report 2xx is a genuine success and must not be turned into a false failure.
func TestSetCalendarPropsAccepted207(t *testing.T) {
	const body = `<?xml version="1.0"?>
<D:multistatus xmlns:D="DAV:">
  <D:response>
    <D:href>/dav/cal/work/</D:href>
    <D:propstat>
      <D:prop><D:displayname/><x:calendar-color xmlns:x="http://apple.com/ns/ical/"/></D:prop>
      <D:status>HTTP/1.1 200 OK</D:status>
    </D:propstat>
  </D:response>
</D:multistatus>`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusMultiStatus)
		_, _ = w.Write([]byte(body))
	}))
	defer srv.Close()

	c, err := NewClient(Config{Endpoint: srv.URL, Username: "u", Password: "p"})
	if err != nil {
		t.Fatal(err)
	}
	if err := c.SetCalendarProps(context.Background(), "/dav/cal/work/", "Work Stuff", "#ff8800"); err != nil {
		t.Errorf("an all-2xx 207 is a success, got %v", err)
	}
}

func TestSetCalendarPropsNoChange(t *testing.T) {
	called := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { called = true }))
	defer srv.Close()
	c, err := NewClient(Config{Endpoint: srv.URL, Username: "u", Password: "p"})
	if err != nil {
		t.Fatal(err)
	}
	if err := c.SetCalendarProps(context.Background(), "/x/", "", ""); err != nil {
		t.Errorf("empty change should be a no-op, got %v", err)
	}
	if called {
		t.Error("no request should be sent when nothing changed")
	}
}
