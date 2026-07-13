package sync

import "testing"

// TestCollectionIDNeutralizesTraversal guards the defense-in-depth part of the
// path-escape fix: a server collection path whose last segment is a traversal
// element must not become a ".." calendar id (which would escape the cache root).
func TestCollectionIDNeutralizesTraversal(t *testing.T) {
	cases := map[string]string{
		"/dav/calendars/user/..":    "calendar",
		"/dav/calendars/user/../..": "calendar",
		"/dav/calendars/user/.":     "calendar",
		"":                          "calendar",
	}
	for in, want := range cases {
		if got := collectionID(in); got != want {
			t.Errorf("collectionID(%q) = %q, want %q", in, got, want)
		}
	}
	// A normal path still yields its safe last segment.
	if got := collectionID("/dav/calendars/user/personal/"); got != "personal" {
		t.Errorf("collectionID(personal) = %q, want personal", got)
	}
}
