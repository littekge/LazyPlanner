package caldav_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/littekge/LazyPlanner/internal/caldav"
)

// TestPutObjectRedirectMustNotReportSuccess reproduces the reported HIGH defect:
// a server/proxy that answers a write with a 301/302/303 redirect causes
// http.Client to re-issue the request as GET to Location (per RFC 7231 / Go's
// redirect policy for non-GET methods). The GET returns 200 + an ETag, so
// PutObject returns ("<etag>", nil) even though the PUT body was never stored.
// Sync then clears the dirty flag and the user's edit is silently lost.
func TestPutObjectRedirectMustNotReportSuccess(t *testing.T) {
	putBodyStored := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPut:
			// A reverse proxy normalizing http->https or a trailing slash: 301.
			http.Redirect(w, r, "/final.ics", http.StatusMovedPermanently)
		case r.Method == http.MethodGet && r.URL.Path == "/final.ics":
			// The redirect target answers the (method-downgraded) GET with an ETag.
			w.Header().Set("ETag", `"etag-from-get"`)
			w.WriteHeader(http.StatusOK)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	c, _ := caldav.NewClient(caldav.Config{Endpoint: srv.URL})
	etag, err := c.PutObject(context.Background(), "/dav/cal/e1.ics", sampleICS, "srv-1", false)

	if err == nil {
		t.Fatalf("PutObject reported success (etag=%q) on a 301-redirected write; the PUT body was never stored (putBodyStored=%v). The edit is silently lost.", etag, putBodyStored)
	}
	t.Logf("got expected error: %v", err)
}

// TestDeleteObjectRedirectMustNotReportSuccess reproduces the DELETE half: a 301
// on DELETE is re-issued as GET to Location; a 200 there makes DeleteObject
// return nil while the resource still exists server-side.
func TestDeleteObjectRedirectMustNotReportSuccess(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodDelete:
			http.Redirect(w, r, "/final.ics", http.StatusMovedPermanently)
		case r.Method == http.MethodGet && r.URL.Path == "/final.ics":
			w.Header().Set("ETag", `"etag-from-get"`)
			w.WriteHeader(http.StatusOK)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	c, _ := caldav.NewClient(caldav.Config{Endpoint: srv.URL})
	err := c.DeleteObject(context.Background(), "/dav/cal/e1.ics", "srv-1")

	if err == nil {
		t.Fatal("DeleteObject reported success on a 301-redirected delete; the resource was never deleted server-side.")
	}
	t.Logf("got expected error: %v", err)
}
