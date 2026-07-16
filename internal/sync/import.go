package sync

import (
	"context"
	"errors"
	"fmt"
	"path"
	"strings"
	"time"

	"github.com/littekge/LazyPlanner/internal/caldav"
	"github.com/littekge/LazyPlanner/internal/model"
	"github.com/littekge/LazyPlanner/internal/store"
)

// Source is the read side of a CalDAV server that Import needs. *caldav.Client
// satisfies it; tests provide fakes.
type Source interface {
	DiscoverCalendars(ctx context.Context) ([]caldav.Calendar, error)
	DownloadAll(ctx context.Context, calendarPath string) ([]caldav.Object, error)
	// ListObjectHrefs and GetObject back the per-resource download fallback when
	// DownloadAll aborts the whole batch on one unparseable resource.
	ListObjectHrefs(ctx context.Context, calendarPath string) ([]caldav.ObjectRef, error)
	GetObject(ctx context.Context, href string) (caldav.Object, error)
}

// ImportError records a single resource that could not be imported. The rest of
// the import still proceeds; these are collected in the result.
type ImportError struct {
	Calendar string
	Path     string
	Err      error
}

func (e ImportError) Error() string {
	return fmt.Sprintf("%s (%s): %v", e.Calendar, e.Path, e.Err)
}

func (e ImportError) Unwrap() error { return e.Err }

// ImportResult summarizes a one-way import.
type ImportResult struct {
	Calendars int
	Objects   int
	Skipped   []ImportError
}

// Import performs a one-way pull: it discovers the server's calendars and
// downloads every resource into dst, overwriting local copies (upsert). It does
// not delete local resources that are absent from the server, nor push local
// changes — that is the two-way sync step. Individual unparseable or unwritable
// resources are skipped and collected in the result; only discovery and
// per-calendar listing failures abort. A bulk-download failure (one resource the
// transport can't decode) falls back to per-resource fetches so the rest of the
// calendar still imports.
func Import(ctx context.Context, src Source, dst *store.Store) (ImportResult, error) {
	var res ImportResult

	cals, err := src.DiscoverCalendars(ctx)
	if err != nil {
		return res, fmt.Errorf("import: discovering calendars: %w", err)
	}

	for _, cal := range cals {
		if err := ctx.Err(); err != nil {
			return res, err
		}

		id := collectionID(cal.Path)
		if err := dst.SetCalendarMeta(ctx, id, store.CalendarMeta{
			DisplayName: cal.Name,
			Color:       cal.Color,
			Href:        cal.Path,
		}); err != nil {
			return res, fmt.Errorf("import: recording calendar %q: %w", id, err)
		}
		res.Calendars++

		// Import only ever adds resources (it never reconciles deletions), so the
		// listed-but-unfetched set is irrelevant here — a failed fetch is recorded
		// as a skip below and simply not imported.
		objs, skips, _, err := downloadResilient(ctx, src, cal.Path)
		if err != nil {
			return res, fmt.Errorf("import: downloading calendar %q: %w", id, err)
		}
		for _, s := range skips {
			res.Skipped = append(res.Skipped, ImportError{Calendar: id, Path: s.Ref, Err: s.Err})
		}

		// Batch the writes so importing a large calendar costs one sidecar write,
		// not one per resource (O(N) not O(N²)).
		var pulls []store.RemoteObject
		for _, obj := range objs {
			parsed, err := model.Parse(obj.Data, time.Local)
			if err != nil {
				res.Skipped = append(res.Skipped, ImportError{Calendar: id, Path: obj.Path, Err: err})
				continue
			}
			pulls = append(pulls, store.RemoteObject{Name: resourceFileName(obj.Path), Object: parsed, ETag: obj.ETag, Href: obj.Path})
		}
		results, err := dst.PullRemoteBatch(ctx, id, pulls)
		if err != nil {
			return res, fmt.Errorf("import: writing calendar %q: %w", id, err)
		}
		for i, e := range results {
			switch {
			case e == nil:
				res.Objects++
			case errors.Is(e, store.ErrKeptLocalEdit):
				// A concurrent local edit was preserved; not an import failure.
			default:
				res.Skipped = append(res.Skipped, ImportError{Calendar: id, Path: pulls[i].Href, Err: e})
			}
		}
	}
	return res, nil
}

// collectionID derives a filesystem-safe calendar id from a CalDAV collection
// path (its last path segment).
func collectionID(calPath string) string {
	base := path.Base(strings.Trim(calPath, "/"))
	if base == "" || base == "." || base == ".." {
		return "calendar"
	}
	return store.SafeName(base)
}

// resourceFileName derives the local .ics file name from a server resource
// path. The local↔server mapping is tracked by the stored Href, so the exact
// name only needs to be safe and stable; the .ics extension is ensured.
func resourceFileName(objPath string) string {
	base := path.Base(strings.TrimRight(objPath, "/"))
	if base == "" || base == "." {
		base = "resource"
	}
	if !strings.HasSuffix(strings.ToLower(base), ".ics") {
		base += ".ics"
	}
	return store.SafeName(base)
}
