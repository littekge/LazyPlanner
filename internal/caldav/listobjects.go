package caldav

import (
	"bytes"
	"context"
	"encoding/xml"
	"fmt"
	"net/http"
	"strings"
)

// ObjectRef is a resource's href and ETag without its (potentially unparseable)
// calendar data. It backs the per-resource download fallback: enumerate refs
// cheaply, then GetObject each one so a single malformed resource can't abort
// the whole calendar.
type ObjectRef struct {
	Href string
	ETag string
}

// ListObjectHrefs issues a Depth-1 PROPFIND for getetag/resourcetype under a
// calendar collection and returns one ObjectRef per member resource (the
// collection itself is skipped). Unlike DownloadAll it never requests or decodes
// calendar-data, so a single malformed resource cannot make it fail — that is
// exactly what makes it a usable fallback when the bulk calendar-query aborts on
// one bad object. Issued over our own authenticated HTTP client because
// go-webdav offers no data-free listing.
func (c *Client) ListObjectHrefs(ctx context.Context, calendarPath string) ([]ObjectRef, error) {
	body := []byte(xml.Header + `<d:propfind xmlns:d="DAV:"><d:prop><d:getetag/><d:resourcetype/></d:prop></d:propfind>`)

	target, err := c.resolve(calendarPath)
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
		return nil, fmt.Errorf("caldav: PROPFIND object list: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusMultiStatus {
		return nil, fmt.Errorf("caldav: PROPFIND object list: %s%s", resp.Status, responseHint(resp.Body))
	}

	var ms objectListMultistatus
	if err := xml.NewDecoder(resp.Body).Decode(&ms); err != nil {
		return nil, fmt.Errorf("caldav: parsing object list response: %w", err)
	}

	collection := strings.TrimRight(calendarPath, "/")
	out := make([]ObjectRef, 0, len(ms.Responses))
	for _, r := range ms.Responses {
		href := strings.TrimSpace(r.Href)
		if href == "" || strings.TrimRight(href, "/") == collection || r.isCollection() {
			continue // the collection itself, not a member resource
		}
		out = append(out, ObjectRef{Href: href, ETag: r.etag()})
	}
	return out, nil
}

type objectListMultistatus struct {
	XMLName   xml.Name             `xml:"DAV: multistatus"`
	Responses []objectListResponse `xml:"DAV: response"`
}

type objectListResponse struct {
	Href      string               `xml:"DAV: href"`
	Propstats []objectListPropstat `xml:"DAV: propstat"`
}

func (r objectListResponse) etag() string {
	for _, ps := range r.Propstats {
		if v := strings.Trim(strings.TrimSpace(ps.Prop.ETag), `"`); v != "" {
			return v
		}
	}
	return ""
}

func (r objectListResponse) isCollection() bool {
	for _, ps := range r.Propstats {
		if ps.Prop.ResourceType.Collection != nil {
			return true
		}
	}
	return false
}

type objectListPropstat struct {
	Prop objectListProp `xml:"DAV: prop"`
}

type objectListProp struct {
	ETag         string                 `xml:"DAV: getetag"`
	ResourceType objectListResourceType `xml:"DAV: resourcetype"`
}

type objectListResourceType struct {
	Collection *struct{} `xml:"DAV: collection"`
}
