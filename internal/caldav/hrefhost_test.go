package caldav_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/littekge/LazyPlanner/internal/caldav"
)

// These tests guard a HIGH defect found in pass 24: a server-supplied href of
// the form "//evil.host/x.ics" is a *protocol-relative URL*, not a path (RFC
// 3986 reads a leading "//" as the start of an authority). Client.resolve did
// endpoint.ResolveReference(url.Parse(ref)), which let that href replace the
// endpoint's authority — so an authenticated write (its Basic-auth app password
// and the full calendar body) was delivered to a host the endpoint never named,
// while the real server saw nothing and the app recorded success.
//
// The full chain: a multiget href "http://real.host//evil.host/x.ics" →
// go-webdav keeps url.Parse(href).Path == "//evil.host/x.ics" → sync stores it
// verbatim as Resource.Href (nothing in internal/store validates an href) →
// the next edit's PutObject resolves it to http://evil.host/x.ics.
//
// Two layers are guarded, and each kills a different subset of these tests:
// Client.resolve refuses to address anything off the endpoint's origin (covering
// hrefs already poisoned in a sidecar), and DownloadAll drops such an href at
// ingest so it is never persisted in the first place.
//
// TestResolveAcceptsSameOriginForms is the other side of the boundary: the check
// must reject a foreign origin WITHOUT rejecting the absolute same-origin URLs
// real servers legitimately return.

// hrefRecorder captures what a server received, so a test can prove where a
// write actually landed instead of trusting the returned error.
type hrefRecorder struct {
	mu     sync.Mutex
	hits   int
	method string
	auth   string
	body   string
}

func (r *hrefRecorder) record(req *http.Request, body string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.hits++
	r.method = req.Method
	r.auth = req.Header.Get("Authorization")
	r.body = body
}

func (r *hrefRecorder) snapshot() (hits int, method, auth, body string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.hits, r.method, r.auth, r.body
}

// hrefTestServers starts an "attacker" and an "honest" server, returning both
// recorders, a client pointed at the honest endpoint, and the attacker's
// authority (host:port) for building the hostile href.
func hrefTestServers(t *testing.T, handle func(*hrefRecorder) http.HandlerFunc) (attacker, honest *hrefRecorder, c *caldav.Client, attackerHost string) {
	t.Helper()
	attacker, honest = &hrefRecorder{}, &hrefRecorder{}

	attackerSrv := httptest.NewServer(handle(attacker))
	t.Cleanup(attackerSrv.Close)
	honestSrv := httptest.NewServer(handle(honest))
	t.Cleanup(honestSrv.Close)

	c, err := caldav.NewClient(caldav.Config{
		Endpoint: honestSrv.URL + "/dav",
		Username: "user",
		Password: "app-password",
	})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	return attacker, honest, c, strings.TrimPrefix(attackerSrv.URL, "http://")
}

// TestPutObjectRejectsAuthorityBearingHref: the PUT half — credential and
// calendar-body exfiltration, plus silent data loss (the app marks the resource
// clean/pushed although the real server never saw the write).
func TestPutObjectRejectsAuthorityBearingHref(t *testing.T) {
	attacker, honest, c, attackerHost := hrefTestServers(t, func(rec *hrefRecorder) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			buf := make([]byte, 4096)
			n, _ := r.Body.Read(buf)
			rec.record(r, string(buf[:n]))
			w.Header().Set("ETag", `"pwned"`)
			w.WriteHeader(http.StatusNoContent)
		}
	})

	href := "//" + attackerHost + "/x.ics"
	etag, putErr := c.PutObject(context.Background(), href, sampleICS, "srv-1", false)

	aHits, aMethod, aAuth, aBody := attacker.snapshot()
	hHits, _, _, _ := honest.snapshot()

	if aHits > 0 {
		t.Errorf("authority-bearing href %q sent the write to a foreign host:\n"+
			"  attacker hits=%d method=%s\n"+
			"  Authorization leaked: %q\n"+
			"  body leaked: %q\n"+
			"  honest endpoint hits=%d\n"+
			"  PutObject returned etag=%q err=%v (store marks the resource clean/pushed)",
			href, aHits, aMethod, aAuth, strings.ReplaceAll(aBody, "\n", "\\n"), hHits, etag, putErr)
	}
	if putErr == nil && hHits == 0 {
		t.Errorf("PutObject reported success (etag=%q) but the honest endpoint received 0 requests", etag)
	}
}

// TestDeleteObjectRejectsAuthorityBearingHref: the DELETE half — the
// authenticated DELETE goes to the attacker and its 204 convinces the app the
// resource is gone.
func TestDeleteObjectRejectsAuthorityBearingHref(t *testing.T) {
	attacker, honest, c, attackerHost := hrefTestServers(t, func(rec *hrefRecorder) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			rec.record(r, "")
			w.WriteHeader(http.StatusNoContent)
		}
	})

	href := "//" + attackerHost + "/x.ics"
	delErr := c.DeleteObject(context.Background(), href, "srv-1")

	aHits, aMethod, aAuth, _ := attacker.snapshot()
	hHits, _, _, _ := honest.snapshot()
	if aHits > 0 {
		t.Errorf("authority-bearing href %q sent the DELETE to a foreign host (hits=%d method=%s auth=%q); honest endpoint hits=%d; DeleteObject err=%v",
			href, aHits, aMethod, aAuth, hHits, delErr)
	}
}

// TestCreateCalendarRejectsAuthorityBearingHref: the collection half — a
// discovered Calendar.Path of the same shape redirects MKCALENDAR (and, via
// joinHref, every resource PUT under that calendar, since joinHref preserves
// the "//host" prefix).
func TestCreateCalendarRejectsAuthorityBearingHref(t *testing.T) {
	attacker, honest, c, attackerHost := hrefTestServers(t, func(rec *hrefRecorder) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			buf := make([]byte, 4096)
			n, _ := r.Body.Read(buf)
			rec.record(r, string(buf[:n]))
			w.WriteHeader(http.StatusCreated)
		}
	})

	path := "//" + attackerHost + "/newcal/"
	mkErr := c.CreateCalendar(context.Background(), path, caldav.CalendarSpec{DisplayName: "Work"})

	aHits, aMethod, aAuth, aBody := attacker.snapshot()
	hHits, _, _, _ := honest.snapshot()
	if aHits > 0 {
		t.Errorf("authority-bearing path %q sent MKCALENDAR to a foreign host (hits=%d method=%s auth=%q body=%q); honest endpoint hits=%d; CreateCalendar err=%v",
			path, aHits, aMethod, aAuth, aBody, hHits, mkErr)
	}
}

// TestDownloadAllRejectsAuthorityBearingHref covers the ingest end of the
// chain: the app must not surface (and sync must not store) an href that
// carries an authority, because every later write resolves against it.
func TestDownloadAllRejectsAuthorityBearingHref(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/xml; charset=utf-8")
		w.WriteHeader(http.StatusMultiStatus)
		fmt.Fprint(w, `<?xml version="1.0" encoding="utf-8"?>
<D:multistatus xmlns:D="DAV:" xmlns:C="urn:ietf:params:xml:ns:caldav">
 <D:response>
  <D:href>http://real.host//evil.host/x.ics</D:href>
  <D:propstat>
   <D:prop>
    <D:getetag>"srv-1"</D:getetag>
    <C:calendar-data>BEGIN:VCALENDAR&#13;
VERSION:2.0&#13;
PRODID:-//t//EN&#13;
BEGIN:VEVENT&#13;
UID:e1@test&#13;
DTSTAMP:20260701T120000Z&#13;
DTSTART:20260704T130000Z&#13;
SUMMARY:x&#13;
END:VEVENT&#13;
END:VCALENDAR&#13;
</C:calendar-data>
   </D:prop>
   <D:status>HTTP/1.1 200 OK</D:status>
  </D:propstat>
 </D:response>
</D:multistatus>`)
	}))
	defer srv.Close()

	c, err := caldav.NewClient(caldav.Config{Endpoint: srv.URL + "/dav"})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	objs, err := c.DownloadAll(context.Background(), "/dav/cal/")
	if err != nil {
		t.Fatalf("DownloadAll: %v", err)
	}
	for _, o := range objs {
		if strings.HasPrefix(o.Path, "//") {
			t.Errorf("DownloadAll surfaced authority-bearing href %q; sync stores it verbatim as Resource.Href and every later write resolves against that host", o.Path)
		}
	}
}

// TestResolveAcceptsSameOriginForms pins the permissive side of the origin
// check. A one-sided guard that only proves foreign hosts are rejected would go
// green if resolve started rejecting everything, which would break every real
// server that returns absolute hrefs (and every deployment on a default port).
func TestResolveAcceptsSameOriginForms(t *testing.T) {
	rec := &hrefRecorder{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rec.record(r, "")
		w.Header().Set("ETag", `"srv-1"`)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	c, err := caldav.NewClient(caldav.Config{Endpoint: srv.URL + "/dav/"})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	cases := []struct {
		name, href string
	}{
		{"absolute path", "/dav/cal/x.ics"},
		{"relative path", "cal/x.ics"},
		// The same origin spelled as a full absolute URL — what a server behind no
		// proxy commonly returns, and the form most at risk from an over-strict fix.
		{"absolute same-origin URL", srv.URL + "/dav/cal/x.ics"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			before, _, _, _ := rec.snapshot()
			if _, err := c.PutObject(context.Background(), tc.href, sampleICS, "", true); err != nil {
				t.Fatalf("PutObject(%q) rejected a same-origin href: %v", tc.href, err)
			}
			if after, _, _, _ := rec.snapshot(); after == before {
				t.Errorf("PutObject(%q) never reached the endpoint", tc.href)
			}
		})
	}
}
