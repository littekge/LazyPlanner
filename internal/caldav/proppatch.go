package caldav

import (
	"bytes"
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
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
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("caldav: PROPPATCH %q: reading response: %w", path, err)
	}

	// A PROPPATCH reports per-property status inside a 207 Multi-Status; some
	// servers answer a plain 200. Treat other codes as failure with a hint.
	if resp.StatusCode != http.StatusMultiStatus && resp.StatusCode != http.StatusOK {
		return fmt.Errorf("caldav: PROPPATCH %q: %s%s", path, resp.Status, responseHint(bytes.NewReader(respBody)))
	}

	// The 207 HTTP line is 207 even when the property change was REJECTED — the
	// real outcome is the per-property <status> inside each propstat. Treating the
	// 207 alone as success silently drops a server-rejected rename/recolor while
	// the local offline edit is marked pushed (a lost update). Fail on any
	// positively-identified non-2xx propstat; a plain 200, or a 207 whose body has
	// no parseable status (a quirky but non-rejecting server), stays lenient so a
	// genuine success is never turned into a false failure.
	if resp.StatusCode == http.StatusMultiStatus {
		if rejected := proppatchRejection(respBody); rejected != "" {
			return fmt.Errorf("caldav: PROPPATCH %q rejected: %s", path, rejected)
		}
	}
	return nil
}

// proppatchRejection scans a 207 Multi-Status body and returns a description of
// the first propstat whose <status> is non-2xx (the property change the server
// refused), or "" when every parseable status is 2xx (or none is present). Per
// RFC 4918 §9.2 a PROPPATCH is atomic, so a single failed property means the
// whole rename/recolor did not take effect.
func proppatchRejection(body []byte) string {
	var ms proppatchMultistatus
	if err := xml.Unmarshal(body, &ms); err != nil {
		return "" // unparseable 207 — stay lenient rather than fail a real success
	}
	for _, r := range ms.Responses {
		for _, ps := range r.Propstats {
			code := statusCode(ps.Status)
			if code != 0 && (code < 200 || code >= 300) {
				return fmt.Sprintf("property %s: %s", ps.Prop.names(), strings.TrimSpace(ps.Status))
			}
		}
	}
	return ""
}

// statusCode extracts the numeric HTTP code from a WebDAV status line such as
// "HTTP/1.1 403 Forbidden". It returns 0 when no code can be read, which callers
// treat as "no verdict" rather than a failure.
func statusCode(status string) int {
	fields := strings.Fields(status)
	if len(fields) < 2 {
		return 0
	}
	code, err := strconv.Atoi(fields[1])
	if err != nil {
		return 0
	}
	return code
}

// --- PROPPATCH response body (RFC 4918 §9.2) ---

type proppatchMultistatus struct {
	XMLName   xml.Name            `xml:"DAV: multistatus"`
	Responses []proppatchResponse `xml:"DAV: response"`
}

type proppatchResponse struct {
	Propstats []proppatchPropstat `xml:"DAV: propstat"`
}

type proppatchPropstat struct {
	Prop   proppatchProp `xml:"DAV: prop"`
	Status string        `xml:"DAV: status"`
}

// proppatchProp captures the names of the properties a propstat's status applies
// to, whatever their namespace, so a rejection message can identify them.
type proppatchProp struct {
	Props []xml.Name `xml:",any"`
}

func (p proppatchProp) names() string {
	if len(p.Props) == 0 {
		return "(unspecified)"
	}
	parts := make([]string, 0, len(p.Props))
	for _, n := range p.Props {
		parts = append(parts, n.Local)
	}
	return strings.Join(parts, ", ")
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
