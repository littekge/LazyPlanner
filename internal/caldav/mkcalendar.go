package caldav

import (
	"bytes"
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

// CalendarSpec describes a calendar collection to create.
type CalendarSpec struct {
	DisplayName string
	Description string
	// Color is an optional hex value like "#3366cc" (Apple calendar-color).
	Color string
	// Components are the iCalendar component types the calendar supports, e.g.
	// ["VEVENT", "VTODO"]. Empty defaults to both. A task list is ["VTODO"].
	Components []string
}

// CreateCalendar creates a calendar collection at path via an RFC 4791
// MKCALENDAR request. path is an absolute server path (resolved against the
// endpoint) and should end in '/'. go-webdav's client cannot do this, so we
// issue the request over the authenticated HTTP client directly.
func (c *Client) CreateCalendar(ctx context.Context, path string, spec CalendarSpec) error {
	comps := spec.Components
	if len(comps) == 0 {
		comps = []string{"VEVENT", "VTODO"}
	}

	doc := mkcalendarReq{
		Set: davSet{Prop: calProp{
			DisplayName: spec.DisplayName,
			Description: spec.Description,
			Color:       spec.Color,
		}},
	}
	for _, name := range comps {
		doc.Set.Prop.CompSet.Comps = append(doc.Set.Prop.CompSet.Comps, compElem{Name: name})
	}

	body, err := xml.Marshal(doc)
	if err != nil {
		return fmt.Errorf("caldav: encoding MKCALENDAR body: %w", err)
	}
	payload := append([]byte(xml.Header), body...)

	target, err := c.resolve(path)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, "MKCALENDAR", target, bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("caldav: building MKCALENDAR request: %w", err)
	}
	req.Header.Set("Content-Type", `application/xml; charset="utf-8"`)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("caldav: MKCALENDAR %q: %w", path, err)
	}
	defer resp.Body.Close()

	// RFC 4791: success is 201 Created.
	if resp.StatusCode != http.StatusCreated {
		return fmt.Errorf("caldav: MKCALENDAR %q: %s%s", path, resp.Status, responseHint(resp.Body))
	}
	return nil
}

// DeleteCalendar removes the calendar collection at path (a DELETE on the
// collection). This lets calendars be removed without the NextCloud web UI.
func (c *Client) DeleteCalendar(ctx context.Context, path string) error {
	target, err := c.resolve(path)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, target, nil)
	if err != nil {
		return fmt.Errorf("caldav: building DELETE request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("caldav: DELETE %q: %w", path, err)
	}
	defer resp.Body.Close()

	// 204 No Content is the usual success; 200 is also acceptable.
	if resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusOK {
		return fmt.Errorf("caldav: DELETE %q: %s%s", path, resp.Status, responseHint(resp.Body))
	}
	return nil
}

// resolve turns a server path (or absolute URL) into an absolute URL string
// against the client's endpoint.
func (c *Client) resolve(ref string) (string, error) {
	u, err := url.Parse(ref)
	if err != nil {
		return "", fmt.Errorf("caldav: invalid path %q: %w", ref, err)
	}
	return c.endpoint.ResolveReference(u).String(), nil
}

// responseHint returns a short, trimmed excerpt of an error response body to
// aid diagnosis, prefixed with a space, or "" if empty.
func responseHint(r io.Reader) string {
	b, _ := io.ReadAll(io.LimitReader(r, 2048))
	s := strings.TrimSpace(string(b))
	if s == "" {
		return ""
	}
	return ": " + s
}

// --- MKCALENDAR request body (RFC 4791 §5.3.1) ---

type mkcalendarReq struct {
	XMLName xml.Name `xml:"urn:ietf:params:xml:ns:caldav mkcalendar"`
	Set     davSet   `xml:"DAV: set"`
}

type davSet struct {
	Prop calProp `xml:"DAV: prop"`
}

type calProp struct {
	DisplayName string      `xml:"DAV: displayname"`
	Description string      `xml:"urn:ietf:params:xml:ns:caldav calendar-description,omitempty"`
	Color       string      `xml:"http://apple.com/ns/ical/ calendar-color,omitempty"`
	CompSet     compSetElem `xml:"urn:ietf:params:xml:ns:caldav supported-calendar-component-set"`
}

type compSetElem struct {
	Comps []compElem `xml:"urn:ietf:params:xml:ns:caldav comp"`
}

type compElem struct {
	Name string `xml:"name,attr"`
}
