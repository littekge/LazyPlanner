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
	// ListObjectHrefs enumerates a calendar's resource hrefs+ETags without
	// fetching calendar-data — the per-resource download fallback when DownloadAll
	// aborts the whole batch on one unparseable resource.
	ListObjectHrefs(ctx context.Context, calendarPath string) ([]caldav.ObjectRef, error)
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
	// CalendarWritable reports whether the current user may write to the calendar
	// at path — the reactive confirmation used when a write returns 403.
	CalendarWritable(ctx context.Context, path string) (bool, error)
	// GetObject fetches a single resource fresh — used to re-read the current
	// server version when a conditional write returns 412.
	GetObject(ctx context.Context, href string) (caldav.Object, error)
}

// errEmptyHref marks a server response whose resource href was empty — it can't
// be addressed for later writes, so it is skipped rather than stored.
var errEmptyHref = errors.New("server response had an empty href")

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
	Calendars          int         // calendars reconciled
	CalendarsUnchanged int         // calendars skipped via the CTag short-circuit (unchanged both ways)
	CalendarsCreated   int         // local calendars created on the server (MKCALENDAR)
	CalendarsDeleted   int         // local calendar deletions applied on the server
	CalendarsUpdated   int         // local calendar metadata changes pushed (PROPPATCH)
	Pushed             int         // local creates/edits sent to the server
	Pulled             int         // remote creates/edits fetched into the cache
	PushedDeletes      int         // local deletions applied on the server
	PulledDeletes      int         // server deletions applied locally
	Conflicts          int         // conflicts detected this run
	Discarded          int         // local changes dropped because the calendar is read-only
	Skipped            []SyncError // per-resource failures (sync still completed)
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
		// Per-calendar metadata bookkeeping (all local sidecar writes): a failure on
		// one calendar records a skip and moves on, so it can't abort syncing every
		// other calendar — matching the record-and-continue that reconcileCalendar
		// uses below. (Only discovery and context-cancellation abort the whole run.)
		if !known {
			// A calendar new on the server: record its metadata (creates the
			// local collection), then pull everything below.
			localID = collectionID(sc.Path)
			if err := st.SetCalendarMeta(ctx, localID, store.CalendarMeta{DisplayName: sc.Name, Href: sc.Path}); err != nil {
				recordSkip(&res, localID, "(calendar)", err)
				continue
			}
		}
		// Keep the local read-only status in step with the server so the UI knows.
		if err := st.SetCalendarReadOnly(ctx, localID, sc.ReadOnly); err != nil {
			recordSkip(&res, localID, "(calendar)", err)
			continue
		}
		// Record the supported component set so the UI can list an empty task
		// list (VTODO) even before it holds anything.
		if err := st.SetCalendarComponents(ctx, localID, sc.SupportedComponentSet); err != nil {
			recordSkip(&res, localID, "(calendar)", err)
			continue
		}
		// Adopt the server's calendar color (unless a local color edit is pending),
		// so the in-app palette stays consistent with NextCloud web.
		if err := st.SyncCalendarColor(ctx, localID, sc.Color); err != nil {
			recordSkip(&res, localID, "(calendar)", err)
			continue
		}
		// Adopt a server-side rename too (unless a local rename is pending), so
		// names stay server-authoritative like colors.
		if err := st.SyncCalendarName(ctx, localID, sc.Name); err != nil {
			recordSkip(&res, localID, "(calendar)", err)
			continue
		}

		// Incremental short-circuit: when the server's CTag matches the one recorded
		// at the last successful sync and there is nothing local to push, the
		// calendar is unchanged on both sides — skip its full download entirely.
		if sc.CTag != "" && sc.CTag == st.CalendarCTag(localID) && !st.HasLocalChanges(localID) {
			res.CalendarsUnchanged++
			continue
		}

		skipsBefore := len(res.Skipped)
		if err := reconcileCalendar(ctx, client, st, localID, sc, &res); err != nil {
			// One calendar's failure (e.g. its download/REPORT) shouldn't block the
			// rest — record it and move on, so healthy calendars still sync. A
			// cancelled context aborts the whole run, though.
			if ctx.Err() != nil {
				return res, err
			}
			recordSkip(&res, localID, "(calendar)", err)
			continue
		}
		// Cache the CTag only after a fully clean reconcile, so a calendar with any
		// per-resource failure re-syncs fully next time rather than being skipped.
		// (After a local push the server's CTag has already advanced past this one,
		// so the next sync re-downloads once — correct, if slightly less optimal.)
		if sc.CTag != "" && len(res.Skipped) == skipsBefore {
			if err := st.SetCalendarCTag(ctx, localID, sc.CTag); err != nil {
				recordSkip(&res, localID, "(ctag)", err)
			}
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
		if err := st.MarkCalendarPropsSynced(ctx, u.ID, u.DisplayName, u.Color); err != nil {
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
// downloader is the subset of the server API the resilient download needs; both
// Syncer (two-way sync) and Source (one-way import) satisfy it.
type downloader interface {
	DownloadAll(ctx context.Context, calendarPath string) ([]caldav.Object, error)
	ListObjectHrefs(ctx context.Context, calendarPath string) ([]caldav.ObjectRef, error)
	GetObject(ctx context.Context, href string) (caldav.Object, error)
}

// downloadSkip is one resource (or the bulk step) that could not be downloaded;
// callers fold these into their own result type (SyncResult/ImportResult).
type downloadSkip struct {
	Ref string
	Err error
}

// downloadResilient fetches every resource in a calendar. It first tries the bulk
// calendar-query (DownloadAll); if that fails — go-webdav aborts the whole batch
// on the first resource whose iCalendar won't decode — it falls back to
// enumerating hrefs (ListObjectHrefs never decodes data) and fetching each
// resource individually, so one malformed .ics can no longer silently disable a
// whole calendar (the resilience the docs promise). Returned skips include a
// note that the bulk path failed (so the slower degraded path isn't invisible)
// plus any per-resource fetch failures. A non-nil error means even the fallback
// listing failed — a genuine per-calendar abort.
//
// The third return is the set of hrefs the server LISTED but whose individual
// fetch failed (populated only on the degraded path). Reconcile must not mistake
// these for remote deletions: the listing proves the resource still exists on the
// server, so an absent-from-objs href that is in this set is unfetched, not gone.
func downloadResilient(ctx context.Context, d downloader, calPath string) ([]caldav.Object, []downloadSkip, map[string]bool, error) {
	objs, err := d.DownloadAll(ctx, calPath)
	if err == nil {
		return objs, nil, nil, nil // bulk succeeded → complete view, nothing unfetched
	}
	refs, lerr := d.ListObjectHrefs(ctx, calPath)
	if lerr != nil {
		return nil, nil, nil, fmt.Errorf("bulk download failed: %v; fallback listing failed: %w", err, lerr)
	}
	skips := []downloadSkip{{Ref: "(bulk download)", Err: fmt.Errorf("bulk download failed, fetching resources individually: %w", err)}}
	unfetched := map[string]bool{}
	out := make([]caldav.Object, 0, len(refs))
	for _, ref := range refs {
		if cerr := ctx.Err(); cerr != nil {
			return nil, nil, nil, cerr
		}
		o, gerr := d.GetObject(ctx, ref.Href)
		if gerr != nil {
			skips = append(skips, downloadSkip{Ref: ref.Href, Err: gerr}) // skip just this resource
			unfetched[ref.Href] = true                                    // listed on the server, just couldn't fetch it now
			continue
		}
		out = append(out, o)
	}
	return out, skips, unfetched, nil
}

func downloadCalendar(ctx context.Context, client Syncer, calID, calPath string, res *SyncResult) ([]caldav.Object, map[string]bool, error) {
	objs, skips, unfetched, err := downloadResilient(ctx, client, calPath)
	if err != nil {
		return nil, nil, fmt.Errorf("sync: downloading calendar %q: %w", calID, err)
	}
	for _, s := range skips {
		recordSkip(res, calID, s.Ref, s.Err)
	}
	return objs, unfetched, nil
}

func reconcileCalendar(ctx context.Context, client Syncer, st *store.Store, calID string, sc caldav.Calendar, res *SyncResult) error {
	serverObjs, unfetched, err := downloadCalendar(ctx, client, calID, sc.Path, res)
	if err != nil {
		return err
	}
	serverByHref := make(map[string]caldav.Object, len(serverObjs))
	for _, o := range serverObjs {
		serverByHref[o.Path] = o
	}

	if sc.ReadOnly {
		// Read-only calendar (e.g. NextCloud's generated birthdays): never write
		// to the server. Mirror it one-way and discard any stuck local changes.
		return reconcileReadOnly(ctx, st, calID, serverObjs, serverByHref, unfetched, res)
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
		case r.Href == "" && r.Dirty:
			// New local resource, never pushed → create it on the server.
			pushCreate(ctx, client, st, calID, sc.Path, r, res)

		case r.Href == "":
			// Clean yet never pushed: not a genuine local create (those are always
			// dirty) but a pull orphan — an interrupted bulk pull wrote this .ics
			// before its batched sidecar flush, so it reloaded without a server
			// identity. Don't upload it (that would create a server-side duplicate);
			// leave it for step (B) to overwrite by re-pulling the server's copy into
			// the same file. If the server no longer has it, it stays an inert
			// local-only item rather than being wrongly resurrected on the server.

		default:
			serverObj, onServer := serverByHref[r.Href]
			switch {
			case !onServer && unfetched[r.Href]:
				// Absent from the downloaded objects only because its individual
				// fetch failed this pass (degraded download); the server's listing
				// still includes it, so it was NOT deleted. Leave the local copy
				// untouched — treating an unfetchable-but-existing resource as a
				// deletion would Forget a clean item / raise a false ServerDeleted
				// conflict. The failed GET is already recorded as a skip upstream,
				// so the CTag isn't cached and the next sync retries it.
			case !onServer && r.Dirty:
				// Edited locally, deleted on the server → conflict; keep the
				// local edit (no server version survives to stash). Flag it as a
				// genuine server deletion so keep-server accepts the deletion.
				markConflict(ctx, st, calID, r.Name, nil, "", true, res)
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
				pushUpdate(ctx, client, st, calID, sc.Path, r, serverObj, res)
			case serverObj.ETag != r.ETag:
				// Server edit only → pull it, but not over a concurrent local edit.
				pullInto(ctx, st, calID, r.Name, serverObj, r, res)
			}
		}
	}

	// (B) Pull resources that exist on the server but not locally (new remotely),
	// skipping any with a pending local deletion (handled in step C). These are
	// applied in one batched write (PullRemoteBatch) so a first-time pull of a
	// large calendar costs one sidecar write, not one per resource (O(N) not
	// O(N²)). The pull list is built from the pre-lock snapshot, so an edit that
	// landed during step (A)'s network I/O may target a name we're about to pull
	// (notably a crash-orphan the user just re-edited); the batch guards against
	// that by skipping a Dirty resource (ErrKeptLocalEdit) rather than clobbering it.
	var pulls []store.RemoteObject
	for _, o := range serverObjs {
		if o.Path == "" {
			// A server response with an empty href can't be addressed for a later
			// update or delete, and storing it (with Href=="") would make the next
			// sync mistake it for a never-pushed local resource and re-upload it as
			// a server-side duplicate. Skip and record it instead.
			recordSkip(res, calID, "(empty href)", errEmptyHref)
			continue
		}
		if localByHref[o.Path] || tombstonedHref[o.Path] {
			continue
		}
		parsed, perr := model.Parse(o.Data, time.Local)
		if perr != nil {
			recordSkip(res, calID, o.Path, perr)
			continue
		}
		pulls = append(pulls, store.RemoteObject{Name: resourceFileName(o.Path), Object: parsed, ETag: o.ETag, Href: o.Path})
	}
	if len(pulls) > 0 {
		results, err := st.PullRemoteBatch(ctx, calID, pulls)
		if err != nil {
			return err
		}
		for i, e := range results {
			switch {
			case e == nil:
				res.Pulled++
			case errors.Is(e, store.ErrKeptLocalEdit):
				// A concurrent local edit was preserved instead of pulled; not a
				// failure — the next sync reconciles it. Don't count or flag it.
			default:
				recordSkip(res, calID, pulls[i].Name, e)
			}
		}
	}

	// (C) Push local deletions (tombstones) to the server.
	for _, t := range st.Tombstones() {
		if t.CalID != calID {
			continue
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		pushDelete(ctx, client, st, calID, sc.Path, t, serverByHref, res)
	}
	return nil
}

// reconcileReadOnly mirrors a read-only calendar one-way (server → local). Any
// local change that can never be pushed is discarded: dirty or never-synced
// resources are dropped, and local deletions (tombstones) are reverted by
// re-pulling the server's copy. Clean resources track the server as usual.
func reconcileReadOnly(ctx context.Context, st *store.Store, calID string, serverObjs []caldav.Object, serverByHref map[string]caldav.Object, unfetched map[string]bool, res *SyncResult) error {
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
		case !onServer && unfetched[r.Href]:
			// Listed on the server but its fetch failed this pass (degraded
			// download) — not a deletion. Leave the mirrored copy untouched;
			// the recorded skip keeps the CTag uncached so the next sync retries.
		case !onServer:
			if err := st.Forget(ctx, calID, r.Name); err != nil {
				recordSkip(res, calID, r.Name, err)
			} else {
				res.PulledDeletes++
			}
		case serverObj.ETag != r.ETag:
			pullInto(ctx, st, calID, r.Name, serverObj, nil, res)
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
		pullInto(ctx, st, calID, resourceFileName(o.Path), o, nil, res)
	}
	return nil
}

// handleWriteForbidden reacts to a write refused with 403. A bare 403 is not
// trusted — it can be transient (auth blip, rate-limit, WAF, maintenance) — so
// the calendar's privileges are re-checked: only a *confirmed* read-only calendar
// is flagged and its stuck local change discarded (the settled pull-only design).
// Otherwise the local edit is kept and surfaced, to retry on the next sync,
// rather than being silently lost on a spurious 403.
func handleWriteForbidden(ctx context.Context, client Syncer, st *store.Store, calID, calPath, name string, res *SyncResult) {
	writable, err := client.CalendarWritable(ctx, calPath)
	if err == nil && !writable {
		_ = st.SetCalendarReadOnly(ctx, calID, true)
		if ferr := st.Forget(ctx, calID, name); ferr != nil {
			recordSkip(res, calID, name, ferr)
			return
		}
		res.Discarded++
		return
	}
	// Not confirmed read-only (still writable, or the re-check itself failed):
	// keep the local edit and surface it; it retries next sync.
	if err != nil {
		recordSkip(res, calID, name, fmt.Errorf("write refused (403); privilege re-check failed (%w) — kept local change, will retry", err))
	} else {
		recordSkip(res, calID, name, fmt.Errorf("write refused (403) but calendar still grants write — kept local change, will retry"))
	}
}

// handleDeleteForbidden is the delete-side twin of handleWriteForbidden: a DELETE
// refused with 403 is not trusted on its own (it can be a transient auth/WAF blip)
// — the calendar's privileges are re-checked, and only a *confirmed* read-only
// calendar is flagged, its item resurrected, and its tombstone dropped. On a
// spurious 403 the tombstone is kept so the delete retries next sync, rather than
// being silently abandoned.
func handleDeleteForbidden(ctx context.Context, client Syncer, st *store.Store, calID, calPath string, t store.Tombstone, serverByHref map[string]caldav.Object, res *SyncResult) {
	writable, err := client.CalendarWritable(ctx, calPath)
	if err == nil && !writable {
		_ = st.SetCalendarReadOnly(ctx, calID, true)
		if serverObj, ok := serverByHref[t.Href]; ok {
			pullInto(ctx, st, calID, t.Name, serverObj, nil, res)
		}
		_ = st.ClearTombstone(ctx, calID, t.Name)
		res.Discarded++
		return
	}
	if err != nil {
		recordSkip(res, calID, t.Name, fmt.Errorf("delete refused (403); privilege re-check failed (%w) — kept the pending delete, will retry", err))
	} else {
		recordSkip(res, calID, t.Name, fmt.Errorf("delete refused (403) but calendar still grants write — kept the pending delete, will retry"))
	}
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
		handleWriteForbidden(ctx, client, st, calID, calPath, r.Name, res)
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
	if _, err := st.CommitPush(ctx, calID, r.Name, r, etag, href); err != nil {
		recordSkip(res, calID, r.Name, err)
		return
	}
	res.Pushed++
}

func pushUpdate(ctx context.Context, client Syncer, st *store.Store, calID, calPath string, r *store.Resource, serverObj caldav.Object, res *SyncResult) {
	data, err := r.Object.Encode()
	if err != nil {
		recordSkip(res, calID, r.Name, err)
		return
	}
	etag, err := client.PutObject(ctx, r.Href, data, r.ETag, false)
	if errors.Is(err, caldav.ErrReadOnly) {
		handleWriteForbidden(ctx, client, st, calID, calPath, r.Name, res)
		return
	}
	if errors.Is(err, caldav.ErrPreconditionFailed) {
		// The server changed between our download and this write → conflict. The
		// serverObj from the start of the sync is now stale (that's what the 412
		// means), so re-fetch the current server version to stash an accurate
		// conflict; fall back to serverObj if the re-fetch fails.
		if fresh, gerr := client.GetObject(ctx, r.Href); gerr == nil {
			serverObj = fresh
		}
		stashServerConflict(ctx, st, calID, r.Name, serverObj, res)
		return
	}
	if err != nil {
		recordSkip(res, calID, r.Name, err)
		return
	}
	if _, err := st.CommitPush(ctx, calID, r.Name, r, etag, r.Href); err != nil {
		recordSkip(res, calID, r.Name, err)
		return
	}
	res.Pushed++
}

func pushDelete(ctx context.Context, client Syncer, st *store.Store, calID, calPath string, t store.Tombstone, serverByHref map[string]caldav.Object, res *SyncResult) {
	err := client.DeleteObject(ctx, t.Href, t.ETag)
	if errors.Is(err, caldav.ErrReadOnly) {
		// A 403 on delete is not trusted outright (see handleWriteForbidden): re-check
		// privileges so a transient 403 doesn't wrongly abandon the delete.
		handleDeleteForbidden(ctx, client, st, calID, calPath, t, serverByHref, res)
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
// expectedPrev is the local snapshot the reconcile decision was based on (nil
// for a brand-new remote resource); the write is skipped if a concurrent local
// edit replaced it, so the edit is preserved and reconciled as a conflict next
// sync rather than being clobbered by the pulled server version.
func pullInto(ctx context.Context, st *store.Store, calID, name string, o caldav.Object, expectedPrev *store.Resource, res *SyncResult) {
	parsed, err := model.Parse(o.Data, time.Local)
	if err != nil {
		recordSkip(res, calID, o.Path, err)
		return
	}
	applied, err := st.PullRemote(ctx, calID, name, parsed, o.ETag, o.Path, expectedPrev)
	if err != nil {
		recordSkip(res, calID, name, err)
		return
	}
	if !applied {
		return // a concurrent local edit landed; leave it for the next sync to reconcile
	}
	res.Pulled++
}

// stashServerConflict records a conflict, stashing the server's version. It
// encodes the decoded server calendar directly (not via a stricter typed
// re-parse), so a server version our model rejects is still preserved rather
// than dropped to empty — empty would be misread by keep-server as a server
// deletion and used to silently discard the local edit. A parse failure is
// surfaced as a skip so keep-server later reports a decode error instead of
// applying it, but never treats the resource as deleted.
func stashServerConflict(ctx context.Context, st *store.Store, calID, name string, o caldav.Object, res *SyncResult) {
	data, err := (&model.Parsed{Calendar: o.Data}).Encode()
	if err != nil {
		recordSkip(res, calID, name, fmt.Errorf("encoding server conflict version: %w", err))
		data = nil
	} else if _, perr := model.Parse(o.Data, time.Local); perr != nil {
		recordSkip(res, calID, name, fmt.Errorf("server conflict version of %q does not parse: %w", name, perr))
	}
	markConflict(ctx, st, calID, name, data, o.ETag, false, res)
}

func markConflict(ctx context.Context, st *store.Store, calID, name string, serverData []byte, serverETag string, serverDeleted bool, res *SyncResult) {
	if err := st.MarkConflict(ctx, calID, name, serverData, serverETag, serverDeleted); err != nil {
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
