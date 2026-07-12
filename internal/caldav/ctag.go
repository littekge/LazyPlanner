package caldav

import (
	"bytes"
	"context"
	"encoding/xml"
	"fmt"
	"net/http"
	"strings"
)

// discoverCTags issues a Depth-1 PROPFIND for the CalendarServer getctag property
// under the calendar home set and returns, per calendar path, the collection's
// CTag — an opaque token that changes whenever anything in the collection changes.
// The sync engine compares it to the last-synced CTag to skip the full download of
// a calendar nothing has touched (the incremental-sync short-circuit).
//
// go-webdav doesn't surface getctag, so — as with calendar-color and privileges —
// we issue the request over our own authenticated HTTP client. Keys are calendar
// paths with any trailing slash trimmed. Best-effort: a failed or unsupported
// query returns an error the caller ignores, falling back to a full sync.
func (c *Client) discoverCTags(ctx context.Context, homeSet string) (map[string]string, error) {
	body := []byte(xml.Header + `<d:propfind xmlns:d="DAV:" xmlns:cs="http://calendarserver.org/ns/"><d:prop><cs:getctag/></d:prop></d:propfind>`)

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
		return nil, fmt.Errorf("caldav: PROPFIND ctag: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusMultiStatus {
		return nil, fmt.Errorf("caldav: PROPFIND ctag: %s%s", resp.Status, responseHint(resp.Body))
	}

	var ms ctagMultistatus
	if err := xml.NewDecoder(resp.Body).Decode(&ms); err != nil {
		return nil, fmt.Errorf("caldav: parsing ctag response: %w", err)
	}

	out := make(map[string]string, len(ms.Responses))
	for _, r := range ms.Responses {
		if ct := r.ctag(); ct != "" {
			out[strings.TrimRight(r.Href, "/")] = ct
		}
	}
	return out, nil
}

// --- getctag response (CalendarServer namespace) ---

type ctagMultistatus struct {
	XMLName   xml.Name       `xml:"DAV: multistatus"`
	Responses []ctagResponse `xml:"DAV: response"`
}

type ctagResponse struct {
	Href      string         `xml:"DAV: href"`
	Propstats []ctagPropstat `xml:"DAV: propstat"`
}

func (r ctagResponse) ctag() string {
	for _, ps := range r.Propstats {
		if v := strings.TrimSpace(ps.Prop.CTag); v != "" {
			return v
		}
	}
	return ""
}

type ctagPropstat struct {
	Prop ctagProp `xml:"DAV: prop"`
}

type ctagProp struct {
	CTag string `xml:"http://calendarserver.org/ns/ getctag"`
}
