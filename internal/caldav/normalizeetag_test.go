package caldav

import "testing"

// TestNormalizeETag closes the Pass-22 canary escape: normalizeETag strips a W/
// weak-validator prefix before unquoting, but every other ETag test uses a strong
// double-quoted ETag, so dropping the W/ strip shipped undetected — a weak ETag
// would then be stored with its W/"…" wrapper intact and never match on a later
// If-Match, causing spurious 412s / lost writes. The W/ rows below fail if the
// TrimPrefix is removed.
func TestNormalizeETag(t *testing.T) {
	cases := []struct{ in, want string }{
		{`"abc123"`, "abc123"},   // strong, quoted
		{`W/"abc123"`, "abc123"}, // weak validator — the previously untested path
		{`  W/"xyz"  `, "xyz"},   // surrounding whitespace + weak validator
		{"bare", "bare"},         // already bare
		{"", ""},
	}
	for _, c := range cases {
		if got := normalizeETag(c.in); got != c.want {
			t.Errorf("normalizeETag(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}
