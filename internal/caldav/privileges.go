package caldav

import (
	"bytes"
	"context"
	"encoding/xml"
	"fmt"
	"net/http"
	"strings"
)

// discoverWritable issues a Depth-1 PROPFIND for current-user-privilege-set
// (RFC 3744) under the calendar home set and returns, per calendar path, whether
// the current user may write to it. A calendar that grants read but not
// write/write-content/bind/all is read-only (e.g. NextCloud's generated
// "Contact Birthdays" calendar). go-webdav's client neither requests nor exposes
// this, so — as with MKCALENDAR — we issue the request ourselves.
//
// Keys are calendar paths with any trailing slash trimmed. A calendar missing
// from the result is treated by callers as writable (fail open): read-only
// enforcement also has a reactive 403 safety net, so a discovery gap never
// wrongly locks a calendar.
func (c *Client) discoverWritable(ctx context.Context, homeSet string) (map[string]bool, error) {
	body := []byte(xml.Header + `<d:propfind xmlns:d="DAV:"><d:prop><d:current-user-privilege-set/></d:prop></d:propfind>`)

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
		return nil, fmt.Errorf("caldav: PROPFIND privileges: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusMultiStatus {
		return nil, fmt.Errorf("caldav: PROPFIND privileges: %s%s", resp.Status, responseHint(resp.Body))
	}

	var ms privMultistatus
	if err := xml.NewDecoder(resp.Body).Decode(&ms); err != nil {
		return nil, fmt.Errorf("caldav: parsing privilege response: %w", err)
	}

	out := make(map[string]bool, len(ms.Responses))
	for _, r := range ms.Responses {
		key := strings.TrimRight(r.Href, "/")
		out[key] = r.writable()
	}
	return out, nil
}

// CalendarWritable reports whether the current user may write to the single
// calendar at calPath, via a Depth-0 current-user-privilege-set PROPFIND. It is
// the reactive confirmation used when a write is refused with 403: a bare 403 can
// be transient (auth blip, rate-limit, maintenance), so callers re-check here
// before treating the calendar as genuinely read-only. On an ambiguous answer
// (no matching response) it fails open (writable) so a local edit isn't discarded
// on uncertainty.
func (c *Client) CalendarWritable(ctx context.Context, calPath string) (bool, error) {
	body := []byte(xml.Header + `<d:propfind xmlns:d="DAV:"><d:prop><d:current-user-privilege-set/></d:prop></d:propfind>`)
	target, err := c.resolve(calPath)
	if err != nil {
		return false, err
	}
	req, err := http.NewRequestWithContext(ctx, "PROPFIND", target, bytes.NewReader(body))
	if err != nil {
		return false, fmt.Errorf("caldav: building PROPFIND request: %w", err)
	}
	req.Header.Set("Content-Type", `application/xml; charset="utf-8"`)
	req.Header.Set("Depth", "0")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return false, fmt.Errorf("caldav: PROPFIND privileges %q: %w", calPath, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusMultiStatus {
		return false, fmt.Errorf("caldav: PROPFIND privileges %q: %s%s", calPath, resp.Status, responseHint(resp.Body))
	}
	var ms privMultistatus
	if err := xml.NewDecoder(resp.Body).Decode(&ms); err != nil {
		return false, fmt.Errorf("caldav: parsing privilege response: %w", err)
	}
	if len(ms.Responses) == 0 {
		return true, nil // ambiguous → fail open (keep the edit)
	}
	want := strings.TrimRight(calPath, "/")
	for _, r := range ms.Responses {
		if strings.TrimRight(r.Href, "/") == want {
			return r.writable(), nil
		}
	}
	return ms.Responses[0].writable(), nil
}

// --- current-user-privilege-set response (RFC 3744 / WebDAV) ---

type privMultistatus struct {
	XMLName   xml.Name       `xml:"DAV: multistatus"`
	Responses []privResponse `xml:"DAV: response"`
}

type privResponse struct {
	Href      string         `xml:"DAV: href"`
	Propstats []privPropstat `xml:"DAV: propstat"`
}

// writable reports whether any propstat grants a write-ish privilege.
func (r privResponse) writable() bool {
	for _, ps := range r.Propstats {
		for _, p := range ps.Prop.PrivilegeSet.Privileges {
			if p.Write != nil || p.WriteContent != nil || p.Bind != nil || p.All != nil {
				return true
			}
		}
	}
	return false
}

type privPropstat struct {
	Prop privProp `xml:"DAV: prop"`
}

type privProp struct {
	PrivilegeSet privilegeSet `xml:"DAV: current-user-privilege-set"`
}

type privilegeSet struct {
	Privileges []privilege `xml:"DAV: privilege"`
}

// privilege captures the write-ish privilege names; read/others decode to nil.
type privilege struct {
	Write        *struct{} `xml:"DAV: write"`
	WriteContent *struct{} `xml:"DAV: write-content"`
	Bind         *struct{} `xml:"DAV: bind"`
	All          *struct{} `xml:"DAV: all"`
}
