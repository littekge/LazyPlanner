package caldav

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/emersion/go-ical"
	"github.com/emersion/go-webdav"
	dav "github.com/emersion/go-webdav/caldav"
)

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
// speaks HTTP to the server.
type Client struct {
	dav *dav.Client
}

// Calendar is a discovered CalDAV collection.
type Calendar struct {
	Path                  string
	Name                  string
	Description           string
	SupportedComponentSet []string
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

	base := cfg.HTTPClient
	if base == nil {
		timeout := cfg.Timeout
		if timeout == 0 {
			timeout = defaultTimeout
		}
		base = &http.Client{Timeout: timeout}
	}

	httpClient := webdav.HTTPClientWithBasicAuth(base, cfg.Username, cfg.Password)
	dc, err := dav.NewClient(httpClient, cfg.Endpoint)
	if err != nil {
		return nil, fmt.Errorf("caldav: creating client: %w", err)
	}
	return &Client{dav: dc}, nil
}

// DiscoverCalendars walks current-user-principal → calendar-home-set →
// calendars and returns the account's collections.
func (c *Client) DiscoverCalendars(ctx context.Context) ([]Calendar, error) {
	principal, err := c.dav.FindCurrentUserPrincipal(ctx)
	if err != nil {
		return nil, fmt.Errorf("caldav: finding current user principal: %w", err)
	}
	homeSet, err := c.dav.FindCalendarHomeSet(ctx, principal)
	if err != nil {
		return nil, fmt.Errorf("caldav: finding calendar home set: %w", err)
	}
	davCals, err := c.dav.FindCalendars(ctx, homeSet)
	if err != nil {
		return nil, fmt.Errorf("caldav: listing calendars: %w", err)
	}

	cals := make([]Calendar, 0, len(davCals))
	for _, dc := range davCals {
		cals = append(cals, Calendar{
			Path:                  dc.Path,
			Name:                  dc.Name,
			Description:           dc.Description,
			SupportedComponentSet: dc.SupportedComponentSet,
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

	objs, err := c.dav.QueryCalendar(ctx, calendarPath, query)
	if err != nil {
		return nil, fmt.Errorf("caldav: querying calendar %q: %w", calendarPath, err)
	}

	out := make([]Object, 0, len(objs))
	for _, o := range objs {
		out = append(out, Object{Path: o.Path, ETag: o.ETag, Data: o.Data})
	}
	return out, nil
}
