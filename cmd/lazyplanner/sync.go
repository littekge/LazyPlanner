package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"

	"github.com/littekge/LazyPlanner/internal/config"
	"github.com/littekge/LazyPlanner/internal/store"
	"github.com/littekge/LazyPlanner/internal/sync"
)

// runSync performs a two-way CalDAV sync of the local cache against the server.
// It is the runnable path for validating the sync engine against a real
// NextCloud before the UI drives it. Credentials come from flags or
// LAZYPLANNER_CALDAV_* environment variables.
func runSync(args []string) error {
	fs := flag.NewFlagSet("sync", flag.ContinueOnError)
	conn := addConnFlags(fs)
	data := fs.String("data", "", "data directory (default: OS data dir)")
	if err := fs.Parse(args); err != nil {
		return err
	}

	client, err := conn.client()
	if err != nil {
		return err
	}

	accountID := config.AccountID(*conn.url, *conn.username)
	dataDir := *data
	if dataDir == "" {
		d, err := config.DataDir()
		if err != nil {
			return err
		}
		dataDir = d
	}
	dataDir = filepath.Join(dataDir, accountID)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	st, err := store.Open(ctx, dataDir)
	if err != nil {
		return err
	}

	res, err := sync.Sync(ctx, client, st)
	if err != nil {
		return err
	}

	fmt.Printf("Synced %d calendar(s): %d pushed, %d pulled, %d deleted on server, %d deleted locally\n",
		res.Calendars, res.Pushed, res.Pulled, res.PushedDeletes, res.PulledDeletes)
	if res.Conflicts > 0 {
		fmt.Printf("%d conflict(s) kept both versions — resolve them in-app\n", res.Conflicts)
	}
	if len(res.Skipped) > 0 {
		fmt.Printf("Skipped %d resource(s):\n", len(res.Skipped))
		for _, s := range res.Skipped {
			fmt.Printf("  - %s\n", s)
		}
	}
	return nil
}
