package caldav

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// A privilege multistatus with a writable "personal" calendar and a read-only
// "contact_birthdays" calendar (read, but no write privilege).
const privMultistatusXML = `<?xml version="1.0"?>
<d:multistatus xmlns:d="DAV:">
  <d:response>
    <d:href>/dav/cal/personal/</d:href>
    <d:propstat>
      <d:prop>
        <d:current-user-privilege-set>
          <d:privilege><d:read/></d:privilege>
          <d:privilege><d:write/></d:privilege>
          <d:privilege><d:write-content/></d:privilege>
        </d:current-user-privilege-set>
      </d:prop>
      <d:status>HTTP/1.1 200 OK</d:status>
    </d:propstat>
  </d:response>
  <d:response>
    <d:href>/dav/cal/contact_birthdays/</d:href>
    <d:propstat>
      <d:prop>
        <d:current-user-privilege-set>
          <d:privilege><d:read/></d:privilege>
        </d:current-user-privilege-set>
      </d:prop>
      <d:status>HTTP/1.1 200 OK</d:status>
    </d:propstat>
  </d:response>
</d:multistatus>`

func TestDiscoverWritable(t *testing.T) {
	var gotMethod, gotDepth string
	var askedPrivileges bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotDepth = r.Method, r.Header.Get("Depth")
		buf := make([]byte, r.ContentLength)
		r.Body.Read(buf)
		askedPrivileges = strings.Contains(string(buf), "current-user-privilege-set")
		w.Header().Set("Content-Type", "application/xml")
		w.WriteHeader(http.StatusMultiStatus)
		w.Write([]byte(privMultistatusXML))
	}))
	defer srv.Close()

	c, err := NewClient(Config{Endpoint: srv.URL})
	if err != nil {
		t.Fatal(err)
	}
	writable, err := c.discoverWritable(context.Background(), "/dav/cal/")
	if err != nil {
		t.Fatal(err)
	}
	if gotMethod != "PROPFIND" || gotDepth != "1" || !askedPrivileges {
		t.Errorf("request = %s depth=%q askedPrivileges=%v, want PROPFIND depth 1 for privileges", gotMethod, gotDepth, askedPrivileges)
	}
	// Keys are trailing-slash-trimmed.
	if w, ok := writable["/dav/cal/personal"]; !ok || !w {
		t.Errorf("personal writable = %v (present %v), want writable", w, ok)
	}
	if w, ok := writable["/dav/cal/contact_birthdays"]; !ok || w {
		t.Errorf("contact_birthdays writable = %v (present %v), want read-only", w, ok)
	}
}

func TestPutDeleteForbiddenIsReadOnly(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer srv.Close()
	c, _ := NewClient(Config{Endpoint: srv.URL})
	body := []byte("BEGIN:VCALENDAR\r\nEND:VCALENDAR\r\n")
	if _, err := c.PutObject(context.Background(), "/dav/cal/ro/x.ics", body, "", true); err != ErrReadOnly {
		t.Errorf("PutObject err = %v, want ErrReadOnly", err)
	}
	if err := c.DeleteObject(context.Background(), "/dav/cal/ro/x.ics", ""); err != ErrReadOnly {
		t.Errorf("DeleteObject err = %v, want ErrReadOnly", err)
	}
}
