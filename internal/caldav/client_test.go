package caldav_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/littekge/LazyPlanner/internal/caldav"
)

// multiStatus is a canned calendar-query REPORT response containing one event.
// The iCalendar payload is flush-left: a leading space would be read as an
// iCal line-folding continuation and corrupt the data.
const multiStatus = `<?xml version="1.0" encoding="utf-8"?>
<multistatus xmlns="DAV:" xmlns:cal="urn:ietf:params:xml:ns:caldav">
  <response>
    <href>/dav/cal/personal/abc.ics</href>
    <propstat>
      <prop>
        <getetag>"etag-abc"</getetag>
        <cal:calendar-data>BEGIN:VCALENDAR
VERSION:2.0
PRODID:-//test//EN
BEGIN:VEVENT
UID:abc@test
DTSTAMP:20260701T120000Z
DTSTART:20260704T130000Z
DTEND:20260704T133000Z
SUMMARY:Imported event
END:VEVENT
END:VCALENDAR
</cal:calendar-data>
      </prop>
      <status>HTTP/1.1 200 OK</status>
    </propstat>
  </response>
</multistatus>
`

func TestDownloadAll(t *testing.T) {
	var gotMethod, gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		w.Header().Set("Content-Type", `application/xml; charset="utf-8"`)
		w.WriteHeader(http.StatusMultiStatus)
		_, _ = io.WriteString(w, multiStatus)
	}))
	defer srv.Close()

	c, err := caldav.NewClient(caldav.Config{Endpoint: srv.URL})
	if err != nil {
		t.Fatal(err)
	}

	objs, err := c.DownloadAll(context.Background(), "/dav/cal/personal/")
	if err != nil {
		t.Fatal(err)
	}

	if gotMethod != "REPORT" {
		t.Errorf("server saw method %q, want REPORT", gotMethod)
	}
	if gotPath != "/dav/cal/personal/" {
		t.Errorf("server saw path %q", gotPath)
	}

	if len(objs) != 1 {
		t.Fatalf("got %d objects, want 1", len(objs))
	}
	o := objs[0]
	if o.Path != "/dav/cal/personal/abc.ics" {
		t.Errorf("Path = %q", o.Path)
	}
	// go-webdav unquotes ETags (strconv.Unquote), so the surrounding quotes
	// from the server's getetag are stripped.
	if o.ETag != "etag-abc" {
		t.Errorf("ETag = %q, want etag-abc", o.ETag)
	}
	if o.Data == nil {
		t.Fatal("Data is nil")
	}
	events := o.Data.Events()
	if len(events) != 1 {
		t.Fatalf("got %d events in data, want 1", len(events))
	}
	if summary, _ := events[0].Props.Text("SUMMARY"); summary != "Imported event" {
		t.Errorf("event summary = %q", summary)
	}
}
