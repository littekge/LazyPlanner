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
