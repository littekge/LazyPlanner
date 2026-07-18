package caldav

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/emersion/go-ical"
	"github.com/emersion/go-webdav"
	dav "github.com/emersion/go-webdav/caldav"
)

// maxResponseBodyBytes bounds how much of any server response the client reads.
// A hostile or buggy CalDAV server that streams an unbounded (or enormous) body
// would otherwise hang a sync or exhaust memory — a real risk on the Raspberry
// Pi target. 64 MiB is far above any realistic calendar listing or bulk resource
// dump; exceeding it fails that request, which sync records as a skip (and a bulk
// download that trips it falls back to per-resource fetches).
const maxResponseBodyBytes = 64 << 20

// errResponseTooLarge is returned when a response body exceeds maxResponseBodyBytes.
var errResponseTooLarge = errors.New("caldav: server response exceeds size limit")

// cappingTransport bounds every response body at maxResponseBodyBytes. It wraps
// the underlying RoundTripper so the cap applies to both go-webdav's requests and
// our own PROPFINDs (they share one http.Client), as defense-in-depth against a
// hostile/oversized server response.
type cappingTransport struct{ rt http.RoundTripper }

func (t cappingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	resp, err := t.rt.RoundTrip(req)
	if err != nil || resp == nil || resp.Body == nil {
		return resp, err
	}
	resp.Body = &cappedBody{inner: resp.Body, remaining: maxResponseBodyBytes}
	return resp, nil
}

// cappedBody fails the read once more than maxResponseBodyBytes has been
// consumed, rather than silently truncating (a truncated XML/iCal body then
// surfaces as a decode error, never as a silently-accepted partial response).
type cappedBody struct {
	inner     io.ReadCloser
	remaining int64
}

func (b *cappedBody) Read(p []byte) (int, error) {
	if b.remaining < 0 {
		return 0, errResponseTooLarge
	}
	if int64(len(p)) > b.remaining+1 { // read one extra byte to detect overflow
		p = p[:b.remaining+1]
	}
	n, err := b.inner.Read(p)
	b.remaining -= int64(n)
	if b.remaining < 0 {
		return n, errResponseTooLarge
	}
	return n, err
}

func (b *cappedBody) Close() error { return b.inner.Close() }

// defaultTimeout bounds each HTTP request when the caller does not set one.
const defaultTimeout = 30 * time.Second

// Config holds the settings needed to reach a NextCloud CalDAV account.
type Config struct {
	// Endpoint is the CalDAV base URL. For NextCloud this is typically
	// https://host/remote.php/dav — discovery walks from there to the
	// principal, the calendar home set, and the calendars.
	Endpoint string
	Username string
	// Password is a NextCloud app password, never the account password.
	Password string
	// Timeout bounds each HTTP request; zero uses defaultTimeout.
	Timeout time.Duration
	// HTTPClient overrides the underlying transport (used in tests). When set,
	// Timeout is ignored.
	HTTPClient *http.Client
}

// Client is LazyPlanner's CalDAV client: server discovery plus bulk download,
// wrapping emersion/go-webdav. It is the only type in the application that
// speaks HTTP to the server. It also holds the authenticated HTTP client and
// endpoint so it can issue verbs go-webdav's client does not expose (e.g.
// MKCALENDAR for calendar creation).
type Client struct {
	dav        *dav.Client
	httpClient webdav.HTTPClient
	endpoint   *url.URL
}

// Calendar is a discovered CalDAV collection.
type Calendar struct {
	Path                  string
	Name                  string
	Description           string
	SupportedComponentSet []string
	// Color is the server's calendar-color (Apple ical property), e.g.
	// "#FF2968FF", or "" when the server sets none. LazyPlanner maps it to the
	// nearest terminal palette color for display.
	Color string
	// ReadOnly is true when the current user lacks write privileges on the
	// collection (e.g. NextCloud's generated "Contact Birthdays" calendar, or a
	// share mounted read-only). LazyPlanner never writes to such calendars.
	ReadOnly bool
	// CTag is the collection's getctag (CalendarServer extension): an opaque token
	// that changes whenever the collection's contents change. "" when the server
	// doesn't support it. Sync compares it to the last-synced CTag to skip a full
	// download of an unchanged calendar.
	CTag string
}

// Object is a downloaded calendar resource with its server identity. Data is
// kept as the decoded calendar so it can be cached verbatim, preserving every
// property (the property-preservation iron rule).
type Object struct {
	Path string
	ETag string
	Data *ical.Calendar
}

// NewClient builds a CalDAV client for the given account. It does not perform
// any network I/O; the first request happens on discovery.
func NewClient(cfg Config) (*Client, error) {
	if cfg.Endpoint == "" {
		return nil, errors.New("caldav: endpoint is required")
	}
	endpoint, err := url.Parse(cfg.Endpoint)
	if err != nil {
		return nil, fmt.Errorf("caldav: invalid endpoint %q: %w", cfg.Endpoint, err)
	}

	base := cfg.HTTPClient
	if base == nil {
		timeout := cfg.Timeout
		if timeout == 0 {
			timeout = defaultTimeout
		}
		base = &http.Client{Timeout: timeout}
	}

	// Bound every response body against a hostile/oversized server. Wrapping the
	// shared client's transport covers both go-webdav's requests and our own.
	if _, already := base.Transport.(cappingTransport); !already {
		rt := base.Transport
		if rt == nil {
			rt = http.DefaultTransport
		}
		base.Transport = cappingTransport{rt: rt}
	}

	// A CalDAV write must land on the exact target href. Go's default redirect
	// policy follows a 3xx and downgrades PUT/DELETE to a bodyless GET, dropping
	// the body and the If-Match/If-None-Match conditionals — a 200 on the followed
	// GET would then masquerade as a successful write and silently lose the edit.
	// Stop redirects for write methods (the 3xx is returned as-is and the write
	// path treats it as an error); reads and discovery — GET/PROPFIND/REPORT,
	// including RFC 6764 .well-known bootstrapping — still follow redirects.
	if base.CheckRedirect == nil {
		base.CheckRedirect = func(_ *http.Request, via []*http.Request) error {
			if len(via) > 0 && isWriteMethod(via[0].Method) {
				return http.ErrUseLastResponse
			}
			return nil
		}
	}

	httpClient := webdav.HTTPClientWithBasicAuth(base, cfg.Username, cfg.Password)
	dc, err := dav.NewClient(httpClient, cfg.Endpoint)
	if err != nil {
		return nil, fmt.Errorf("caldav: creating client: %w", err)
	}
	return &Client{dav: dc, httpClient: httpClient, endpoint: endpoint}, nil
}

// isWriteMethod reports whether an HTTP method mutates server state and so must
// never be silently redirected to a different resource (see CheckRedirect above).
func isWriteMethod(method string) bool {
	switch method {
	case http.MethodPut, http.MethodDelete, http.MethodPost,
		"PROPPATCH", "MKCALENDAR", "MKCOL", "MOVE", "COPY":
		return true
	}
	return false
}

// CalendarHomeSet returns the account's calendar home set path (the collection
// under which calendars live), walking current-user-principal → home-set.
func (c *Client) CalendarHomeSet(ctx context.Context) (string, error) {
	principal, err := c.dav.FindCurrentUserPrincipal(ctx)
	if err != nil {
		return "", fmt.Errorf("caldav: finding current user principal: %w", err)
	}
	homeSet, err := c.dav.FindCalendarHomeSet(ctx, principal)
	if err != nil {
		return "", fmt.Errorf("caldav: finding calendar home set: %w", err)
	}
	return homeSet, nil
}

// hrefKey normalizes a WebDAV <href> from a multistatus response into the key
// space go-webdav's FindCalendars uses for Calendar.Path: the URL-decoded path,
// trailing slash trimmed. A response href may be percent-encoded (%40 for @, %20
// for a space) or an absolute URL (a proxy-rewritten host), while go-webdav
// derives Calendar.Path via url.Parse(href).Path. Keying our side-channel maps
// (color / privilege / CTag) by the raw href would then never match the decoded
// lookup key, silently dropping the value; mirror the same decode on both sides.
func hrefKey(href string) string {
	if u, err := url.Parse(href); err == nil && u.Path != "" {
		href = u.Path
	}
	return strings.TrimRight(href, "/")
}

// DiscoverCalendars walks current-user-principal → calendar-home-set →
// calendars and returns the account's collections.
func (c *Client) DiscoverCalendars(ctx context.Context) ([]Calendar, error) {
	homeSet, err := c.CalendarHomeSet(ctx)
	if err != nil {
		return nil, err
	}
	davCals, err := c.dav.FindCalendars(ctx, homeSet)
	if err != nil {
		return nil, fmt.Errorf("caldav: listing calendars: %w", err)
	}

	// Best-effort read-only detection: a failed or unsupported privilege query
	// must not break discovery — enforcement also has a reactive 403 safety net.
	writable, _ := c.discoverWritable(ctx, homeSet)
	// Best-effort color discovery: go-webdav doesn't surface calendar-color, and a
	// failed query must not break discovery — color is cosmetic.
	colors, _ := c.discoverColors(ctx, homeSet)
	// Best-effort CTag discovery for the incremental-sync short-circuit; a failed
	// or unsupported query just leaves CTags empty, so sync falls back to a full
	// download (correct, just not optimized).
	ctags, _ := c.discoverCTags(ctx, homeSet)

	cals := make([]Calendar, 0, len(davCals))
	for _, dc := range davCals {
		key := hrefKey(dc.Path)
		readOnly := false
		if writable != nil {
			if w, ok := writable[key]; ok && !w {
				readOnly = true
			}
		}
		cals = append(cals, Calendar{
			Path:                  dc.Path,
			Name:                  dc.Name,
			Description:           dc.Description,
			SupportedComponentSet: dc.SupportedComponentSet,
			ReadOnly:              readOnly,
			Color:                 colors[key],
			CTag:                  ctags[key],
		})
	}
	return cals, nil
}

// DownloadAll fetches every resource in the calendar at calendarPath in one
// calendar-query REPORT, returning full iCalendar data and ETags.
func (c *Client) DownloadAll(ctx context.Context, calendarPath string) ([]Object, error) {
	query := &dav.CalendarQuery{
		// Request the full VCALENDAR (all properties, all components).
		CompRequest: dav.CalendarCompRequest{
			Name:     ical.CompCalendar,
			AllProps: true,
			AllComps: true,
		},
		// Match every object in the collection.
		CompFilter: dav.CompFilter{Name: ical.CompCalendar},
	}

	var objs []dav.CalendarObject
	err := guardICalPanic(func() (e error) {
		objs, e = c.dav.QueryCalendar(ctx, calendarPath, query)
		return e
	})
	if err != nil {
		return nil, fmt.Errorf("caldav: querying calendar %q: %w", calendarPath, err)
	}

	out := make([]Object, 0, len(objs))
	for _, o := range objs {
		out = append(out, Object{Path: o.Path, ETag: o.ETag, Data: o.Data})
	}
	return out, nil
}

// GetObject fetches a single calendar resource fresh from the server. It is used
// to re-read the current server version when a conditional write returns 412, so
// the stashed conflict holds the up-to-date server copy (the version downloaded
// at the start of the sync is stale by definition of a 412).
func (c *Client) GetObject(ctx context.Context, href string) (Object, error) {
	var o *dav.CalendarObject
	err := guardICalPanic(func() (e error) {
		o, e = c.dav.GetCalendarObject(ctx, href)
		return e
	})
	if err != nil {
		return Object{}, fmt.Errorf("caldav: getting object %q: %w", href, err)
	}
	return Object{Path: o.Path, ETag: o.ETag, Data: o.Data}, nil
}

// guardICalPanic runs fn, converting a panic raised while decoding server
// iCalendar into an ordinary error. go-webdav decodes calendar-data with
// go-ical, whose line decoder panics on some malformed inputs (a content line
// ending mid-parameter); a hostile or buggy server must degrade to a
// skipped/errored resource, never crash the app (iron rule). A bulk-query panic
// surfaces as a DownloadAll error, which sync already handles by falling back to
// per-resource fetches, so one poison object no longer stalls a whole calendar.
func guardICalPanic(fn func() error) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("malformed iCalendar from server: %v", r)
		}
	}()
	return fn()
}
