package caldav

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/emersion/go-ical"
)

// ErrPreconditionFailed is returned by PutObject and DeleteObject when the
// server rejects a conditional write with HTTP 412 — the resource changed on the
// server since our last sync (If-Match), or already exists (If-None-Match on a
// create). The sync layer turns this into a conflict rather than overwriting, so
// the app never silently discards either side's changes.
var ErrPreconditionFailed = errors.New("caldav: precondition failed (server copy changed)")

// PutObject writes a calendar resource at href with a conditional header so the
// app never blindly overwrites the server:
//   - ifMatch != "": sent as If-Match, so the write applies only while the
//     server's ETag still matches what we last synced.
//   - ifMatch == "" && create: sent as If-None-Match: *, so a create fails if a
//     resource already exists there.
//
// It returns the new server ETag in the store's bare (unquoted) form. Some
// servers omit the ETag on PUT; then it returns "" and the next pull reconciles
// it. go-webdav's PutCalendarObject cannot set these headers, so we issue the
// request over the authenticated HTTP client directly (as with MKCALENDAR).
func (c *Client) PutObject(ctx context.Context, href string, cal *ical.Calendar, ifMatch string, create bool) (string, error) {
	var buf bytes.Buffer
	if err := ical.NewEncoder(&buf).Encode(cal); err != nil {
		return "", fmt.Errorf("caldav: encoding object for %q: %w", href, err)
	}

	target, err := c.resolve(href)
	if err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, target, bytes.NewReader(buf.Bytes()))
	if err != nil {
		return "", fmt.Errorf("caldav: building PUT request: %w", err)
	}
	req.Header.Set("Content-Type", ical.MIMEType)
	switch {
	case ifMatch != "":
		req.Header.Set("If-Match", httpETag(ifMatch))
	case create:
		req.Header.Set("If-None-Match", "*")
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("caldav: PUT %q: %w", href, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusPreconditionFailed {
		return "", ErrPreconditionFailed
	}
	// 201 Created (new), 204 No Content (updated), or 200 OK are all success.
	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("caldav: PUT %q: %s%s", href, resp.Status, responseHint(resp.Body))
	}
	return normalizeETag(resp.Header.Get("ETag")), nil
}

// DeleteObject removes the resource at href. When ifMatch is set it is sent as
// If-Match, so a resource modified on the server since our last sync is not
// silently deleted (the server answers 412 → ErrPreconditionFailed). A resource
// already gone (404) is treated as success — deletion is idempotent.
func (c *Client) DeleteObject(ctx context.Context, href, ifMatch string) error {
	target, err := c.resolve(href)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, target, nil)
	if err != nil {
		return fmt.Errorf("caldav: building DELETE request: %w", err)
	}
	if ifMatch != "" {
		req.Header.Set("If-Match", httpETag(ifMatch))
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("caldav: DELETE %q: %w", href, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusPreconditionFailed {
		return ErrPreconditionFailed
	}
	if resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNotFound {
		return fmt.Errorf("caldav: DELETE %q: %s%s", href, resp.Status, responseHint(resp.Body))
	}
	return nil
}

// normalizeETag strips the weak-validator prefix and surrounding quotes so the
// store keeps a bare ETag value. go-webdav's download path already unquotes, so
// this keeps ETags from every code path in one comparable form.
func normalizeETag(etag string) string {
	etag = strings.TrimSpace(etag)
	etag = strings.TrimPrefix(etag, "W/")
	if len(etag) >= 2 && etag[0] == '"' && etag[len(etag)-1] == '"' {
		return etag[1 : len(etag)-1]
	}
	return etag
}

// httpETag formats a stored (bare) ETag for an If-Match header, quoting it as
// HTTP requires. An already-quoted or weak value is passed through unchanged.
func httpETag(etag string) string {
	if etag == "" {
		return ""
	}
	if strings.HasPrefix(etag, "W/") || (len(etag) >= 2 && etag[0] == '"') {
		return etag
	}
	return `"` + etag + `"`
}
