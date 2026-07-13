package main

import (
	"errors"
	"flag"
	"os"

	"github.com/littekge/LazyPlanner/internal/caldav"
)

// connFlags holds the shared CalDAV connection flags used by the server-facing
// subcommands. Values fall back to LAZYPLANNER_CALDAV_* environment variables.
type connFlags struct {
	url      *string
	username *string
	password *string
}

func addConnFlags(fs *flag.FlagSet) connFlags {
	return connFlags{
		url: fs.String("url", os.Getenv("LAZYPLANNER_CALDAV_URL"),
			"CalDAV base URL, e.g. https://host/remote.php/dav (or $LAZYPLANNER_CALDAV_URL)"),
		username: fs.String("username", os.Getenv("LAZYPLANNER_CALDAV_USERNAME"),
			"CalDAV username (or $LAZYPLANNER_CALDAV_USERNAME)"),
		password: fs.String("password", os.Getenv("LAZYPLANNER_CALDAV_PASSWORD"),
			"CalDAV app password; prefer $LAZYPLANNER_CALDAV_PASSWORD — a --password flag is visible in ps/shell history"),
	}
}

// client validates the flags and builds a CalDAV client.
func (cf connFlags) client() (*caldav.Client, error) {
	if *cf.url == "" || *cf.username == "" || *cf.password == "" {
		return nil, errors.New("url, username, and password are required (flags or LAZYPLANNER_CALDAV_* env vars)")
	}
	return caldav.NewClient(caldav.Config{Endpoint: *cf.url, Username: *cf.username, Password: *cf.password})
}
