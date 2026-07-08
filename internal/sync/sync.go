package sync

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/littekge/LazyPlanner/internal/caldav"
	"github.com/littekge/LazyPlanner/internal/model"
	"github.com/littekge/LazyPlanner/internal/store"
)

// Syncer is the server side that two-way Sync needs: discovery and bulk download
// (as Import uses) plus conditional writes. *caldav.Client satisfies it; tests
// provide fakes.
type Syncer interface {
	DiscoverCalendars(ctx context.Context) ([]caldav.Calendar, error)
	DownloadAll(ctx context.Context, calendarPath string) ([]caldav.Object, error)
	// PutObject writes an encoded resource with a conditional header and returns
	// the new bare ETag (see caldav.Client.PutObject).
	PutObject(ctx context.Context, href string, data []byte, ifMatch string, create bool) (string, error)
	// DeleteObject removes a resource, conditional on ifMatch when set.
	DeleteObject(ctx context.Context, href, ifMatch string) error
	// CalendarHomeSet returns the collection under which calendars live, used to
	// place a newly-created calendar.
	CalendarHomeSet(ctx context.Context) (string, error)
	// CreateCalendar issues MKCALENDAR for a locally-created calendar.
	CreateCalendar(ctx context.Context, path string, spec caldav.CalendarSpec) error
	// DeleteCalendar removes a calendar collection on the server.
	DeleteCalendar(ctx context.Context, path string) error
	// SetCalendarProps pushes a calendar's display name/color (PROPPATCH).
	SetCalendarProps(ctx context.Context, path, displayName, color string) error
}

// SyncError records one resource that could not be synced. The rest of the sync
// still proceeds; these are collected in the result.
type SyncError struct {
	Calendar string
	Ref      string // href or resource name
	Err      error
}

func (e SyncError) Error() string { return fmt.Sprintf("%s (%s): %v", e.Calendar, e.Ref, e.Err) }
func (e SyncError) Unwrap() error { return e.Err }

// SyncResult summarizes one two-way sync.
type SyncResult struct {
	Calendars        int         // calendars reconciled
	CalendarsCreated int         // local calendars created on the server (MKCALENDAR)
	CalendarsDeleted int         // local calendar deletions applied on the server
	CalendarsUpdated int         // local calendar metadata changes pushed (PROPPATCH)
	Pushed           int         // local creates/edits sent to the server
	Pulled           int         // remote creates/edits fetched into the cache
	PushedDeletes    int         // local deletions applied on the server
	PulledDeletes    int         // server deletions applied locally
	Conflicts        int         // conflicts detected this run
	Discarded        int         // local changes dropped because the calendar is read-only
	Skipped          []SyncError // per-resource failures (sync still completed)
}

// Sync reconciles the local cache with the server in both directions, resource
// by resource, using ETags so it never silently overwrites either side:
//
//   - local create (no href)         → PUT If-None-Match:* (create on server)
//   - local edit, server unchanged   → PUT If-Match:etag   (update on server)
//   - server edit, local unchanged   → pull into the cache
//   - both edited                    → conflict: keep both, flag (no overwrite)
//   - local delete (tombstone)       → DELETE If-Match:etag on the server
//   - server delete, local unchanged → drop locally
//   - server delete, local edited    → conflict: keep the local edit, flag
//
// Discovery and per-calendar listing failures abort; individual resource
// failures are collected in the result and do not stop the sync. Calendars that
// exist only locally (e.g. offline-created, or removed on the server) are left
// untouched here — that is handled with in-app calendar management.
func Sync(ctx context.Context, client Syncer, st *store.Store) (SyncResult, error) {
	var res SyncResult

	// Push in-app calendar management first, so discovery reflects it: deletes
	// remove the collection server-side (and locally), creates issue MKCALENDAR
	// so the new calendar is then reconciled like any other.
	pendingDeleteHref := pushCalendarDeletes(ctx, client, st, &res)
	pushCalendarCreates(ctx, client, st, &res)
	pushCalendarProps(ctx, client, st, &res)

	serverCals, err := client.DiscoverCalendars(ctx)
	if err != nil {
		return res, fmt.Errorf("sync: discovering calendars: %w", err)
	}

	// Map local calendars by their server href so a server calendar can be
	// matched to the local cache (or recognized as new).
	localByHref := map[string]string{} // server href -> local calendar id
	for _, c := range st.Calendars() {
		if c.Href != "" {
			localByHref[c.Href] = c.ID
		}
	}

	for _, sc := range serverCals {
		if err := ctx.Err(); err != nil {
			return res, err
		}
		if pendingDeleteHref[sc.Path] {
			continue // a deletion is pending (or failed); don't re-import it
		}

		localID, known := localByHref[sc.Path]
		if !known {
			// A calendar new on the server: record its metadata (creates the
			// local collection), then pull everything below.
			localID = collectionID(sc.Path)
			if err := st.SetCalendarMeta(ctx, localID, store.CalendarMeta{DisplayName: sc.Name, Href: sc.Path}); err != nil {
				return res, fmt.Errorf("sync: recording calendar %q: %w", localID, err)
			}
		}
		// Keep the local read-only status in step with the server so the UI knows.
		if err := st.SetCalendarReadOnly(ctx, localID, sc.ReadOnly); err != nil {
			return res, fmt.Errorf("sync: recording read-only status for %q: %w", localID, err)
		}

		if err := reconcileCalendar(ctx, client, st, localID, sc, &res); err != nil {
			return res, err
		}
		res.Calendars++
	}
	return res, nil
}

// pushCalendarDeletes applies calendars marked for deletion: it deletes the
// collection on the server (when it was ever pushed) and removes it locally. It
// returns the set of server hrefs that are pending deletion, so a delete that
// fails is not re-imported by discovery. Per-calendar failures are recorded and
// leave the calendar pending for a later retry.
func pushCalendarDeletes(ctx context.Context, client Syncer, st *store.Store, res *SyncResult) map[string]bool {
	pending := map[string]bool{}
	for _, d := range st.PendingCalendarDeletes() {
		if d.Href != "" {
			pending[d.Href] = true
			if err := client.DeleteCalendar(ctx, d.Href); err != nil {
				recordSkip(res, d.ID, d.Href, err)
				continue // keep it pending; skip local removal so we retry next sync
			}
		}
		if err := st.RemoveCalendarLocal(ctx, d.ID); err != nil {
			recordSkip(res, d.ID, "calendar", err)
			continue
		}
		res.CalendarsDeleted++
	}
	return pending
}

// pushCalendarProps pushes locally-edited calendar metadata (display name/color)
// to the server with a PROPPATCH, before discovery so a routine pull doesn't race
// the change. A per-calendar failure is recorded and left pending for a retry.
func pushCalendarProps(ctx context.Context, client Syncer, st *store.Store, res *SyncResult) {
	for _, u := range st.PendingCalendarProps() {
		if err := client.SetCalendarProps(ctx, u.Href, u.DisplayName, u.Color); err != nil {
			recordSkip(res, u.ID, u.Href, err)
			continue
		}
		if err := st.MarkCalendarPropsSynced(ctx, u.ID); err != nil {
			recordSkip(res, u.ID, "calendar", err)
			continue
		}
		res.CalendarsUpdated++
	}
}

// pushCalendarCreates issues MKCALENDAR for every locally-created calendar, then
// records its server href so the following reconcile pushes its resources.
func pushCalendarCreates(ctx context.Context, client Syncer, st *store.Store, res *SyncResult) {
	var homeSet string
	for _, c := range st.Calendars() {
		if !c.PendingCreate {
			continue
		}
		if homeSet == "" {
			hs, err := client.CalendarHomeSet(ctx)
			if err != nil {
				recordSkip(res, c.ID, "calendar", err)
				return // no home set → can't create any; discovery will surface the error
			}
			homeSet = hs
		}
		path := joinHref(homeSet, c.ID) + "/"
		spec := caldav.CalendarSpec{DisplayName: c.DisplayName, Color: c.Color, Components: c.Components}
		if err := client.CreateCalendar(ctx, path, spec); err != nil {
			recordSkip(res, c.ID, "calendar", err)
			continue
		}
		if err := st.MarkCalendarSynced(ctx, c.ID, path); err != nil {
			recordSkip(res, c.ID, "calendar", err)
			continue
		}
		res.CalendarsCreated++
	}
}

// reconcileCalendar performs the two-way merge for one calendar. It aborts only
// on a full-calendar download failure; per-resource problems are collected.
func reconcileCalendar(ctx context.Context, client Syncer, st *store.Store, calID string, sc caldav.Calendar, res *SyncResult) error {
	serverObjs, err := client.DownloadAll(ctx, sc.Path)
	if err != nil {
		return fmt.Errorf("sync: downloading calendar %q: %w", calID, err)
	}
	serverByHref := make(map[string]caldav.Object, len(serverObjs))
	for _, o := range serverObjs {
		serverByHref[o.Path] = o
	}

	if sc.ReadOnly {
		// Read-only calendar (e.g. NextCloud's generated birthdays): never write
		// to the server. Mirror it one-way and discard any stuck local changes.
		return reconcileReadOnly(ctx, st, calID, serverObjs, serverByHref, res)
	}

	cal, ok := st.Calendar(calID)
	if !ok {
		return nil // nothing local yet (metadata only); the pulls below still run
	}

	// Hrefs with a pending local deletion: don't re-pull them as "new on server".
	tombstonedHref := map[string]bool{}
	for _, t := range st.Tombstones() {
		if t.CalID == calID {
			tombstonedHref[t.Href] = true
		}
	}

	localByHref := map[string]bool{}

	// (A) Reconcile every locally-known resource.
	for _, r := range cal.Resources {
		if err := ctx.Err(); err != nil {
			return err
		}
		if r.Href != "" {
			localByHref[r.Href] = true
		}
		if r.Conflicted {
			continue // awaiting resolution; leave both sides as they are
		}

		switch {
		case r.Href == "":
			// New local resource, never pushed → create it on the server.
			pushCreate(ctx, client, st, calID, sc.Path, r, res)

		default:
			serverObj, onServer := serverByHref[r.Href]
			switch {
			case !onServer && r.Dirty:
				// Edited locally, deleted on the server → conflict; keep the
				// local edit (no server version survives to stash).
				markConflict(ctx, st, calID, r.Name, nil, "", res)
			case !onServer:
				// Clean and gone on the server → it was deleted remotely.
				if err := st.Forget(ctx, calID, r.Name); err != nil {
					recordSkip(res, calID, r.Name, err)
				} else {
					res.PulledDeletes++
				}
			case r.Dirty && serverObj.ETag != r.ETag:
				// Both sides changed → conflict; keep both.
				stashServerConflict(ctx, st, calID, r.Name, serverObj, res)
			case r.Dirty:
				// Local edit only → push it (conditional on the server ETag).
				pushUpdate(ctx, client, st, calID, r, serverObj, res)
			case serverObj.ETag != r.ETag:
				// Server edit only → pull it.
				pullInto(ctx, st, calID, r.Name, serverObj, res)
			}
		}
	}

	// (B) Pull resources that exist on the server but not locally (new remotely),
	// skipping any with a pending local deletion (handled in step C).
	for _, o := range serverObjs {
		if localByHref[o.Path] || tombstonedHref[o.Path] {
			continue
		}
		pullInto(ctx, st, calID, resourceFileName(o.Path), o, res)
	}

	// (C) Push local deletions (tombstones) to the server.
	for _, t := range st.Tombstones() {
		if t.CalID != calID {
			continue
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		pushDelete(ctx, client, st, calID, t, serverByHref, res)
	}
	return nil
}

// reconcileReadOnly mirrors a read-only calendar one-way (server → local). Any
// local change that can never be pushed is discarded: dirty or never-synced
// resources are dropped, and local deletions (tombstones) are reverted by
// re-pulling the server's copy. Clean resources track the server as usual.
func reconcileReadOnly(ctx context.Context, st *store.Store, calID string, serverObjs []caldav.Object, serverByHref map[string]caldav.Object, res *SyncResult) error {
	cal, ok := st.Calendar(calID)
	if !ok {
		return nil
	}
	localByHref := map[string]bool{}
	for _, r := range cal.Resources {
		if err := ctx.Err(); err != nil {
			return err
		}
		if r.Dirty || r.Href == "" {
			// A local add/edit on a read-only calendar can never sync → discard.
			if err := st.Forget(ctx, calID, r.Name); err != nil {
				recordSkip(res, calID, r.Name, err)
			} else {
				res.Discarded++
			}
			continue
		}
		localByHref[r.Href] = true
		serverObj, onServer := serverByHref[r.Href]
		switch {
		case !onServer:
			if err := st.Forget(ctx, calID, r.Name); err != nil {
				recordSkip(res, calID, r.Name, err)
			} else {
				res.PulledDeletes++
			}
		case serverObj.ETag != r.ETag:
			pullInto(ctx, st, calID, r.Name, serverObj, res)
		}
	}

	// A local deletion of a read-only item can't be pushed; drop the tombstone
	// and let the pull below restore the item.
	for _, t := range st.Tombstones() {
		if t.CalID == calID {
			_ = st.ClearTombstone(ctx, calID, t.Name)
			res.Discarded++
		}
	}

	for _, o := range serverObjs {
		if localByHref[o.Path] {
			continue
		}
		pullInto(ctx, st, calID, resourceFileName(o.Path), o, res)
	}
	return nil
}

// markReadOnlyDiscard is the reactive safety net: a write refused with 403
// (ErrReadOnly) means the calendar is read-only after all (privilege discovery
// missed it). Flag it and discard the stuck local change.
func markReadOnlyDiscard(ctx context.Context, st *store.Store, calID, name string, res *SyncResult) {
	_ = st.SetCalendarReadOnly(ctx, calID, true)
	if err := st.Forget(ctx, calID, name); err != nil {
		recordSkip(res, calID, name, err)
		return
	}
	res.Discarded++
}

func pushCreate(ctx context.Context, client Syncer, st *store.Store, calID, calPath string, r *store.Resource, res *SyncResult) {
	data, err := r.Object.Encode()
	if err != nil {
		recordSkip(res, calID, r.Name, err)
		return
	}
	href := joinHref(calPath, r.Name)
	etag, err := client.PutObject(ctx, href, data, "", true)
	if errors.Is(err, caldav.ErrReadOnly) {
		markReadOnlyDiscard(ctx, st, calID, r.Name, res)
		return
	}
	if errors.Is(err, caldav.ErrPreconditionFailed) {
		// Something already exists at that href (unexpected for a fresh UID).
		recordSkip(res, calID, r.Name, err)
		return
	}
	if err != nil {
		recordSkip(res, calID, r.Name, err)
		return
	}
	if _, err := st.PutRemote(ctx, calID, r.Name, r.Object, etag, href); err != nil {
		recordSkip(res, calID, r.Name, err)
		return
	}
	res.Pushed++
}

func pushUpdate(ctx context.Context, client Syncer, st *store.Store, calID string, r *store.Resource, serverObj caldav.Object, res *SyncResult) {
	data, err := r.Object.Encode()
	if err != nil {
		recordSkip(res, calID, r.Name, err)
		return
	}
	etag, err := client.PutObject(ctx, r.Href, data, r.ETag, false)
	if errors.Is(err, caldav.ErrReadOnly) {
		markReadOnlyDiscard(ctx, st, calID, r.Name, res)
		return
	}
	if errors.Is(err, caldav.ErrPreconditionFailed) {
		// The server changed between our download and this write → conflict.
		stashServerConflict(ctx, st, calID, r.Name, serverObj, res)
		return
	}
	if err != nil {
		recordSkip(res, calID, r.Name, err)
		return
	}
	if _, err := st.PutRemote(ctx, calID, r.Name, r.Object, etag, r.Href); err != nil {
		recordSkip(res, calID, r.Name, err)
		return
	}
	res.Pushed++
}

func pushDelete(ctx context.Context, client Syncer, st *store.Store, calID string, t store.Tombstone, serverByHref map[string]caldav.Object, res *SyncResult) {
	err := client.DeleteObject(ctx, t.Href, t.ETag)
	if errors.Is(err, caldav.ErrReadOnly) {
		// Can't delete on a read-only calendar; flag it and restore the item.
		_ = st.SetCalendarReadOnly(ctx, calID, true)
		if serverObj, ok := serverByHref[t.Href]; ok {
			pullInto(ctx, st, calID, t.Name, serverObj, res)
		}
		_ = st.ClearTombstone(ctx, calID, t.Name)
		res.Discarded++
		return
	}
	if errors.Is(err, caldav.ErrPreconditionFailed) {
		// Deleted locally but changed on the server → conflict. Resurrect the
		// server version so its change is not lost, flag it, and drop the
		// tombstone (the delete lost the race).
		if serverObj, ok := serverByHref[t.Href]; ok {
			if parsed, perr := model.Parse(serverObj.Data, time.Local); perr == nil {
				if _, werr := st.PutRemote(ctx, calID, t.Name, parsed, serverObj.ETag, t.Href); werr == nil {
					stashServerConflict(ctx, st, calID, t.Name, serverObj, res)
				}
			}
		}
		_ = st.ClearTombstone(ctx, calID, t.Name)
		return
	}
	if err != nil {
		recordSkip(res, calID, t.Href, err)
		return
	}
	if err := st.ClearTombstone(ctx, calID, t.Name); err != nil {
		recordSkip(res, calID, t.Name, err)
		return
	}
	res.PushedDeletes++
}

// pullInto writes a server object into the cache as a clean resource.
func pullInto(ctx context.Context, st *store.Store, calID, name string, o caldav.Object, res *SyncResult) {
	parsed, err := model.Parse(o.Data, time.Local)
	if err != nil {
		recordSkip(res, calID, o.Path, err)
		return
	}
	if _, err := st.PutRemote(ctx, calID, name, parsed, o.ETag, o.Path); err != nil {
		recordSkip(res, calID, name, err)
		return
	}
	res.Pulled++
}

// stashServerConflict records a conflict, stashing the server's version. It
// re-encodes the parsed server data (dropping it only if unparseable).
func stashServerConflict(ctx context.Context, st *store.Store, calID, name string, o caldav.Object, res *SyncResult) {
	var data []byte
	if parsed, err := model.Parse(o.Data, time.Local); err == nil {
		data, _ = parsed.Encode()
	}
	markConflict(ctx, st, calID, name, data, o.ETag, res)
}

func markConflict(ctx context.Context, st *store.Store, calID, name string, serverData []byte, serverETag string, res *SyncResult) {
	if err := st.MarkConflict(ctx, calID, name, serverData, serverETag); err != nil {
		recordSkip(res, calID, name, err)
		return
	}
	res.Conflicts++
}

func recordSkip(res *SyncResult, calID, ref string, err error) {
	res.Skipped = append(res.Skipped, SyncError{Calendar: calID, Ref: ref, Err: err})
}

// joinHref builds a resource href under a collection path.
func joinHref(calPath, name string) string {
	return strings.TrimRight(calPath, "/") + "/" + name
}
