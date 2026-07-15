package caldav

import (
	"bytes"
	"context"
	"encoding/xml"
	"fmt"
	"net/http"
	"strings"
)

// discoverColors issues a Depth-1 PROPFIND for the Apple calendar-color property
// under the calendar home set and returns, per calendar path, the server's color
// (e.g. "#FF2968FF"). go-webdav's FindCalendars does not surface calendar-color,
// so — as with MKCALENDAR and the privilege query — we issue the request over our
// own authenticated HTTP client.
//
// Keys are calendar paths with any trailing slash trimmed. A calendar missing
// from the result (or with no color set) is simply absent, and callers leave its
// local color untouched. Errors are best-effort: a failed or unsupported query
// must not break discovery, since color is cosmetic.
func (c *Client) discoverColors(ctx context.Context, homeSet string) (map[string]string, error) {
	body := []byte(xml.Header + `<d:propfind xmlns:d="DAV:" xmlns:x="http://apple.com/ns/ical/"><d:prop><x:calendar-color/></d:prop></d:propfind>`)

	target, err := c.resolve(homeSet)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, "PROPFIND", target, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("caldav: building PROPFIND request: %w", err)
	}
	req.Header.Set("Content-Type", `application/xml; charset="utf-8"`)
	req.Header.Set("Depth", "1")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("caldav: PROPFIND colors: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusMultiStatus {
		return nil, fmt.Errorf("caldav: PROPFIND colors: %s%s", resp.Status, responseHint(resp.Body))
	}

	var ms colorMultistatus
	if err := xml.NewDecoder(resp.Body).Decode(&ms); err != nil {
		return nil, fmt.Errorf("caldav: parsing color response: %w", err)
	}

	out := make(map[string]string, len(ms.Responses))
	for _, r := range ms.Responses {
		if color := r.color(); color != "" {
			out[hrefKey(r.Href)] = color
		}
	}
	return out, nil
}

// --- calendar-color response (Apple ical namespace / WebDAV) ---

type colorMultistatus struct {
	XMLName   xml.Name        `xml:"DAV: multistatus"`
	Responses []colorResponse `xml:"DAV: response"`
}

type colorResponse struct {
	Href      string          `xml:"DAV: href"`
	Propstats []colorPropstat `xml:"DAV: propstat"`
}

// color returns the first non-empty calendar-color value across the response's
// propstats, trimmed. NextCloud returns it in an HTTP 200 propstat; a calendar
// with no color set returns an empty value (or a 404 propstat), which yields "".
func (r colorResponse) color() string {
	for _, ps := range r.Propstats {
		if v := strings.TrimSpace(ps.Prop.Color); v != "" {
			return v
		}
	}
	return ""
}

type colorPropstat struct {
	Prop colorProp `xml:"DAV: prop"`
}

type colorProp struct {
	Color string `xml:"http://apple.com/ns/ical/ calendar-color"`
}
