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

// A multistatus whose hrefs are percent-encoded / absolute — the shapes a real
// server (Google's user%40gmail.com, a NextCloud %20, a proxy-rewritten absolute
// URL) returns. go-webdav decodes these into Calendar.Path, so the color map must
// be keyed by the decoded path or the lookup silently misses.
const colorEncodedMultistatusXML = `<?xml version="1.0"?>
<d:multistatus xmlns:d="DAV:" xmlns:x="http://apple.com/ns/ical/">
  <d:response>
    <d:href>/dav/cal/user%40gmail.com/events/</d:href>
    <d:propstat>
      <d:prop><x:calendar-color>#112233FF</x:calendar-color></d:prop>
      <d:status>HTTP/1.1 200 OK</d:status>
    </d:propstat>
  </d:response>
  <d:response>
    <d:href>https://host.example/dav/cal/work%20cal/</d:href>
    <d:propstat>
      <d:prop><x:calendar-color>#445566FF</x:calendar-color></d:prop>
      <d:status>HTTP/1.1 200 OK</d:status>
    </d:propstat>
  </d:response>
</d:multistatus>`

// TestDiscoverColorsDecodesHrefKey guards pass-12 MED #7: discoverColors keyed the
// map by the raw <href>, while DiscoverCalendars looks up by the URL-decoded
// dc.Path — so a percent-encoded or absolute-URL href produced a key that never
// matched and the color was silently dropped. The keys must be the decoded paths.
func TestDiscoverColorsDecodesHrefKey(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		w.WriteHeader(http.StatusMultiStatus)
		w.Write([]byte(colorEncodedMultistatusXML))
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
	// Keyed by the DECODED path (what go-webdav's Calendar.Path carries), not the
	// raw %40 / absolute-URL href.
	if got := colors["/dav/cal/user@gmail.com/events"]; got != "#112233FF" {
		t.Errorf("percent-encoded href: color = %q, want %q (map keyed by raw href instead of decoded path?)", got, "#112233FF")
	}
	if got := colors["/dav/cal/work cal"]; got != "#445566FF" {
		t.Errorf("absolute+encoded href: color = %q, want %q", got, "#445566FF")
	}
}

func TestHrefKey(t *testing.T) {
	cases := []struct{ in, want string }{
		{"/dav/cal/personal/", "/dav/cal/personal"},
		{"/dav/cal/user%40gmail.com/events/", "/dav/cal/user@gmail.com/events"},
		{"https://host.example/dav/cal/work%20cal/", "/dav/cal/work cal"},
		{"/dav/cal/plain", "/dav/cal/plain"},
	}
	for _, c := range cases {
		if got := hrefKey(c.in); got != c.want {
			t.Errorf("hrefKey(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}
