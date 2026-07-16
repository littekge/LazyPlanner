package caldav_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/littekge/LazyPlanner/internal/caldav"
)

func TestCreateCalendar(t *testing.T) {
	var gotMethod, gotPath, gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
		w.WriteHeader(http.StatusCreated)
	}))
	defer srv.Close()

	c, err := caldav.NewClient(caldav.Config{Endpoint: srv.URL})
	if err != nil {
		t.Fatal(err)
	}

	err = c.CreateCalendar(context.Background(), "/dav/cal/tasks/", caldav.CalendarSpec{
		DisplayName: "My Tasks",
		Color:       "#3366cc",
		Components:  []string{"VTODO"},
	})
	if err != nil {
		t.Fatal(err)
	}

	if gotMethod != "MKCALENDAR" {
		t.Errorf("method = %q, want MKCALENDAR", gotMethod)
	}
	if gotPath != "/dav/cal/tasks/" {
		t.Errorf("path = %q", gotPath)
	}
	for _, want := range []string{"mkcalendar", "My Tasks", "#3366cc", `name="VTODO"`, "supported-calendar-component-set"} {
		if !strings.Contains(gotBody, want) {
			t.Errorf("request body missing %q:\n%s", want, gotBody)
		}
	}
}

func TestCreateCalendarDefaultComponents(t *testing.T) {
	var gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
		w.WriteHeader(http.StatusCreated)
	}))
	defer srv.Close()

	c, _ := caldav.NewClient(caldav.Config{Endpoint: srv.URL})
	if err := c.CreateCalendar(context.Background(), "/dav/cal/x/", caldav.CalendarSpec{DisplayName: "X"}); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`name="VEVENT"`, `name="VTODO"`} {
		if !strings.Contains(gotBody, want) {
			t.Errorf("default components should include both; missing %q", want)
		}
	}
}

func TestCreateCalendarError(t *testing.T) {
	// A genuine failure status (not 405, which is now the idempotent already-exists
	// case) must still surface as an error with the server's hint.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInsufficientStorage)
		_, _ = io.WriteString(w, "quota exceeded")
	}))
	defer srv.Close()

	c, _ := caldav.NewClient(caldav.Config{Endpoint: srv.URL})
	err := c.CreateCalendar(context.Background(), "/dav/cal/dup/", caldav.CalendarSpec{DisplayName: "Dup"})
	if err == nil {
		t.Fatal("expected an error on a genuine failure status")
	}
	if !strings.Contains(err.Error(), "quota exceeded") {
		t.Errorf("error should include the server hint, got: %v", err)
	}
}

func TestCreateCalendarAlreadyExistsIsIdempotent(t *testing.T) {
	// MKCALENDAR on an already-mapped URL returns 405 (RFC 4791 §5.3.1). That must
	// be treated as success, else a lost-201 create wedges the calendar permanently
	// pending-create, retrying MKCALENDAR -> 405 every sync.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusMethodNotAllowed)
		_, _ = io.WriteString(w, "calendar already exists")
	}))
	defer srv.Close()

	c, _ := caldav.NewClient(caldav.Config{Endpoint: srv.URL})
	if err := c.CreateCalendar(context.Background(), "/dav/cal/dup/", caldav.CalendarSpec{DisplayName: "Dup"}); err != nil {
		t.Errorf("CreateCalendar returned 405 as error: %v; want nil (already exists == created)", err)
	}
}

func TestDeleteCalendarAlreadyGoneIsIdempotent(t *testing.T) {
	// A DELETE whose success response was lost (or a retry after a prior delete)
	// re-issues DELETE and the server answers 404/410. That must be treated as
	// success, else the calendar wedges permanently pending-delete.
	for _, code := range []int{http.StatusNotFound, http.StatusGone} {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(code)
		}))
		c, _ := caldav.NewClient(caldav.Config{Endpoint: srv.URL})
		err := c.DeleteCalendar(context.Background(), "/dav/cal/mycal/")
		srv.Close()
		if err != nil {
			t.Errorf("DeleteCalendar returned %d as error: %v; want nil (already gone == deleted)", code, err)
		}
	}
}

func TestDeleteCalendar(t *testing.T) {
	var gotMethod, gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	c, _ := caldav.NewClient(caldav.Config{Endpoint: srv.URL})
	if err := c.DeleteCalendar(context.Background(), "/dav/cal/tasks/"); err != nil {
		t.Fatal(err)
	}
	if gotMethod != http.MethodDelete {
		t.Errorf("method = %q, want DELETE", gotMethod)
	}
	if gotPath != "/dav/cal/tasks/" {
		t.Errorf("path = %q", gotPath)
	}
}
