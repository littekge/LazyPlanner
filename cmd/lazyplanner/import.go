package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"

	"github.com/littekge/LazyPlanner/internal/caldav"
	"github.com/littekge/LazyPlanner/internal/config"
	"github.com/littekge/LazyPlanner/internal/store"
	"github.com/littekge/LazyPlanner/internal/sync"
)

// runImport performs a one-way CalDAV import into the local cache. It is the
// first path to pull real NextCloud data in, so the model can be validated
// against it before the UI exists. Credentials come from flags or
// LAZYPLANNER_CALDAV_* environment variables.
func runImport(args []string) error {
	fs := flag.NewFlagSet("import", flag.ContinueOnError)
	url := fs.String("url", os.Getenv("LAZYPLANNER_CALDAV_URL"),
		"CalDAV base URL, e.g. https://host/remote.php/dav (or $LAZYPLANNER_CALDAV_URL)")
	user := fs.String("username", os.Getenv("LAZYPLANNER_CALDAV_USERNAME"),
		"CalDAV username (or $LAZYPLANNER_CALDAV_USERNAME)")
	pass := fs.String("password", os.Getenv("LAZYPLANNER_CALDAV_PASSWORD"),
		"CalDAV app password (or $LAZYPLANNER_CALDAV_PASSWORD)")
	data := fs.String("data", "", "data directory (default: OS data dir)")
	if err := fs.Parse(args); err != nil {
		return err
	}

	if *url == "" || *user == "" || *pass == "" {
		return errors.New("import requires url, username, and password (flags or LAZYPLANNER_CALDAV_* env vars)")
	}

	dataDir := *data
	if dataDir == "" {
		d, err := config.DataDir()
		if err != nil {
			return err
		}
		dataDir = d
	}

	// Cancel cleanly on Ctrl-C so a long import never blocks uninterruptibly.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	client, err := caldav.NewClient(caldav.Config{Endpoint: *url, Username: *user, Password: *pass})
	if err != nil {
		return err
	}
	st, err := store.Open(ctx, dataDir)
	if err != nil {
		return err
	}

	res, err := sync.Import(ctx, client, st)
	if err != nil {
		return err
	}

	fmt.Printf("Imported %d object(s) across %d calendar(s) into %s\n", res.Objects, res.Calendars, dataDir)
	if len(res.Skipped) > 0 {
		fmt.Printf("Skipped %d resource(s):\n", len(res.Skipped))
		for _, s := range res.Skipped {
			fmt.Printf("  - %s\n", s)
		}
	}
	return nil
}
