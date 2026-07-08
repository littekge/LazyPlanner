package caldav

import (
	"bytes"
	"context"
	"encoding/xml"
	"fmt"
	"net/http"
)

// SetCalendarProps changes a calendar's server-owned metadata via a WebDAV
// PROPPATCH (RFC 4918 §9.2): the display name (DAV:displayname) and/or the Apple
// calendar-color. An empty value leaves that property unchanged. go-webdav's
// client doesn't expose PROPPATCH, so — as with MKCALENDAR — we issue it over
// the authenticated HTTP client directly.
func (c *Client) SetCalendarProps(ctx context.Context, path, displayName, color string) error {
	if displayName == "" && color == "" {
		return nil // nothing to change
	}
	doc := propertyUpdate{Set: propPatchSet{Prop: propPatchProp{
		DisplayName: displayName,
		Color:       color,
	}}}
	body, err := xml.Marshal(doc)
	if err != nil {
		return fmt.Errorf("caldav: encoding PROPPATCH body: %w", err)
	}
	payload := append([]byte(xml.Header), body...)

	target, err := c.resolve(path)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, "PROPPATCH", target, bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("caldav: building PROPPATCH request: %w", err)
	}
	req.Header.Set("Content-Type", `application/xml; charset="utf-8"`)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("caldav: PROPPATCH %q: %w", path, err)
	}
	defer resp.Body.Close()

	// A PROPPATCH reports per-property status inside a 207 Multi-Status; some
	// servers answer a plain 200. Treat other codes as failure with a hint.
	if resp.StatusCode != http.StatusMultiStatus && resp.StatusCode != http.StatusOK {
		return fmt.Errorf("caldav: PROPPATCH %q: %s%s", path, resp.Status, responseHint(resp.Body))
	}
	return nil
}

// --- PROPPATCH request body (RFC 4918 §9.2) ---

type propertyUpdate struct {
	XMLName xml.Name     `xml:"DAV: propertyupdate"`
	Set     propPatchSet `xml:"DAV: set"`
}

type propPatchSet struct {
	Prop propPatchProp `xml:"DAV: prop"`
}

type propPatchProp struct {
	DisplayName string `xml:"DAV: displayname,omitempty"`
	Color       string `xml:"http://apple.com/ns/ical/ calendar-color,omitempty"`
}
