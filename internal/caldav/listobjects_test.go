package caldav

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// A calendar listing multistatus: the collection itself (resourcetype
// collection, no etag) plus two member resources with ETags.
const objectListMultistatusXML = `<?xml version="1.0"?>
<d:multistatus xmlns:d="DAV:">
  <d:response>
    <d:href>/dav/cal/personal/</d:href>
    <d:propstat>
      <d:prop><d:resourcetype><d:collection/></d:resourcetype></d:prop>
      <d:status>HTTP/1.1 200 OK</d:status>
    </d:propstat>
  </d:response>
  <d:response>
    <d:href>/dav/cal/personal/a.ics</d:href>
    <d:propstat>
      <d:prop><d:getetag>"etag-a"</d:getetag><d:resourcetype/></d:prop>
      <d:status>HTTP/1.1 200 OK</d:status>
    </d:propstat>
  </d:response>
  <d:response>
    <d:href>/dav/cal/personal/b.ics</d:href>
    <d:propstat>
      <d:prop><d:getetag>"etag-b"</d:getetag><d:resourcetype/></d:prop>
      <d:status>HTTP/1.1 200 OK</d:status>
    </d:propstat>
  </d:response>
</d:multistatus>`

func TestListObjectHrefs(t *testing.T) {
	var gotMethod, gotDepth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotDepth = r.Method, r.Header.Get("Depth")
		w.Header().Set("Content-Type", "application/xml")
		w.WriteHeader(http.StatusMultiStatus)
		w.Write([]byte(objectListMultistatusXML))
	}))
	defer srv.Close()

	c, err := NewClient(Config{Endpoint: srv.URL})
	if err != nil {
		t.Fatal(err)
	}
	refs, err := c.ListObjectHrefs(context.Background(), "/dav/cal/personal/")
	if err != nil {
		t.Fatal(err)
	}
	if gotMethod != "PROPFIND" || gotDepth != "1" {
		t.Errorf("request = %s depth=%q, want PROPFIND depth 1", gotMethod, gotDepth)
	}
	// The collection itself is excluded; the two members come back with unquoted ETags.
	if len(refs) != 2 {
		t.Fatalf("refs = %+v, want 2 members (collection excluded)", refs)
	}
	got := map[string]string{}
	for _, r := range refs {
		if strings.HasSuffix(r.Href, "/") {
			t.Errorf("collection href leaked into refs: %q", r.Href)
		}
		got[r.Href] = r.ETag
	}
	if got["/dav/cal/personal/a.ics"] != "etag-a" || got["/dav/cal/personal/b.ics"] != "etag-b" {
		t.Errorf("refs = %+v, want a.ics=etag-a b.ics=etag-b", got)
	}
}

// A listing that includes a NESTED sub-collection whose href differs from the
// queried path (e.g. a scheduling inbox). Only the resourcetype=collection check
// excludes it — the path-equality check does not, since its href != the query.
const objectListNestedCollectionXML = `<?xml version="1.0"?>
<d:multistatus xmlns:d="DAV:">
  <d:response>
    <d:href>/dav/cal/personal/</d:href>
    <d:propstat>
      <d:prop><d:resourcetype><d:collection/></d:resourcetype></d:prop>
      <d:status>HTTP/1.1 200 OK</d:status>
    </d:propstat>
  </d:response>
  <d:response>
    <d:href>/dav/cal/personal/inbox/</d:href>
    <d:propstat>
      <d:prop><d:resourcetype><d:collection/></d:resourcetype></d:prop>
      <d:status>HTTP/1.1 200 OK</d:status>
    </d:propstat>
  </d:response>
  <d:response>
    <d:href>/dav/cal/personal/a.ics</d:href>
    <d:propstat>
      <d:prop><d:getetag>"etag-a"</d:getetag><d:resourcetype/></d:prop>
      <d:status>HTTP/1.1 200 OK</d:status>
    </d:propstat>
  </d:response>
</d:multistatus>`

// TestListObjectHrefsExcludesNestedCollection guards the resourcetype=collection
// member-filter: a nested sub-collection whose href is not the query path must be
// excluded, so it is never GET as an event object by the per-resource download
// fallback. The path-equality check alone cannot catch this (the href differs);
// only isCollection() does. Without this case the shared fixture's lone
// collection is masked by path-equality, so dropping the check escapes the suite.
func TestListObjectHrefsExcludesNestedCollection(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		w.WriteHeader(http.StatusMultiStatus)
		w.Write([]byte(objectListNestedCollectionXML))
	}))
	defer srv.Close()

	c, err := NewClient(Config{Endpoint: srv.URL})
	if err != nil {
		t.Fatal(err)
	}
	refs, err := c.ListObjectHrefs(context.Background(), "/dav/cal/personal/")
	if err != nil {
		t.Fatal(err)
	}
	if len(refs) != 1 {
		t.Fatalf("refs = %+v, want exactly 1 member (a.ics); the nested collection must be excluded", refs)
	}
	if refs[0].Href != "/dav/cal/personal/a.ics" {
		t.Errorf("ref = %q, want /dav/cal/personal/a.ics (nested collection leaked?)", refs[0].Href)
	}
}
