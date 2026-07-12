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
