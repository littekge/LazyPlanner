package caldav

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// A calendar-color multistatus: "personal" has a color, "plain" has none (an
// empty 404 propstat, as servers report an unset property).
const colorMultistatusXML = `<?xml version="1.0"?>
<d:multistatus xmlns:d="DAV:" xmlns:x="http://apple.com/ns/ical/">
  <d:response>
    <d:href>/dav/cal/personal/</d:href>
    <d:propstat>
      <d:prop><x:calendar-color>#FF2968FF</x:calendar-color></d:prop>
      <d:status>HTTP/1.1 200 OK</d:status>
    </d:propstat>
  </d:response>
  <d:response>
    <d:href>/dav/cal/plain/</d:href>
    <d:propstat>
      <d:prop><x:calendar-color/></d:prop>
      <d:status>HTTP/1.1 404 Not Found</d:status>
    </d:propstat>
  </d:response>
</d:multistatus>`

func TestDiscoverColors(t *testing.T) {
	var gotMethod, gotDepth string
	var askedColor bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotDepth = r.Method, r.Header.Get("Depth")
		buf := make([]byte, r.ContentLength)
		r.Body.Read(buf)
		askedColor = strings.Contains(string(buf), "calendar-color")
		w.Header().Set("Content-Type", "application/xml")
		w.WriteHeader(http.StatusMultiStatus)
		w.Write([]byte(colorMultistatusXML))
	}))
	defer srv.Close()

	c, err := NewClient(Config{Endpoint: srv.URL})
	if err != nil {
		t.Fatal(err)
	}
	colors, err := c.discoverColors(context.Background(), "/dav/cal/")
	if err != nil {
		t.Fatal(err)
	}
	if gotMethod != "PROPFIND" || gotDepth != "1" || !askedColor {
		t.Errorf("request = %s depth=%q askedColor=%v, want PROPFIND depth 1 for calendar-color", gotMethod, gotDepth, askedColor)
	}
	// Keys are trailing-slash-trimmed; a set color is returned verbatim.
	if got := colors["/dav/cal/personal"]; got != "#FF2968FF" {
		t.Errorf("personal color = %q, want %q", got, "#FF2968FF")
	}
	// A calendar with no color is absent, so callers leave its local color alone.
	if _, ok := colors["/dav/cal/plain"]; ok {
		t.Errorf("plain should have no color entry, got %q", colors["/dav/cal/plain"])
	}
}
