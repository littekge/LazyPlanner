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
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusMethodNotAllowed)
		_, _ = io.WriteString(w, "calendar already exists")
	}))
	defer srv.Close()

	c, _ := caldav.NewClient(caldav.Config{Endpoint: srv.URL})
	err := c.CreateCalendar(context.Background(), "/dav/cal/dup/", caldav.CalendarSpec{DisplayName: "Dup"})
	if err == nil {
		t.Fatal("expected an error on non-201 status")
	}
	if !strings.Contains(err.Error(), "already exists") {
		t.Errorf("error should include the server hint, got: %v", err)
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
