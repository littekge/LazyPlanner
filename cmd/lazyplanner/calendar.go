package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strings"

	"github.com/littekge/LazyPlanner/internal/caldav"
)

// runCalendar handles `lazyplanner calendar <list|create|delete>` — server-side
// calendar management (used first to verify MKCALENDAR works against the
// server, and the basis for in-app calendar creation later).
func runCalendar(args []string) error {
	if len(args) == 0 {
		return errors.New("usage: lazyplanner calendar <list|create|delete> [flags]")
	}
	switch args[0] {
	case "list":
		return runCalendarList(args[1:])
	case "create":
		return runCalendarCreate(args[1:])
	case "delete":
		return runCalendarDelete(args[1:])
	default:
		return fmt.Errorf("unknown calendar subcommand %q (want list, create, or delete)", args[0])
	}
}

func runCalendarList(args []string) error {
	fs := flag.NewFlagSet("calendar list", flag.ContinueOnError)
	conn := addConnFlags(fs)
	if err := fs.Parse(args); err != nil {
		return err
	}
	client, err := conn.client()
	if err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	cals, err := client.DiscoverCalendars(ctx)
	if err != nil {
		return err
	}
	if len(cals) == 0 {
		fmt.Println("No calendars found.")
		return nil
	}
	for _, cal := range cals {
		comps := strings.Join(cal.SupportedComponentSet, ",")
		if comps == "" {
			comps = "—"
		}
		fmt.Printf("%-28s [%s]\n    %s\n", cal.Name, comps, cal.Path)
	}
	return nil
}

func runCalendarCreate(args []string) error {
	fs := flag.NewFlagSet("calendar create", flag.ContinueOnError)
	conn := addConnFlags(fs)
	name := fs.String("name", "", "display name of the new calendar (required)")
	tasks := fs.Bool("tasks", false, "create a task list (VTODO only) instead of an event calendar")
	both := fs.Bool("both", false, "support both events and tasks (VEVENT and VTODO)")
	color := fs.String("color", "", "calendar color, e.g. #3366cc (optional)")
	desc := fs.String("desc", "", "calendar description (optional)")
	path := fs.String("path", "", "explicit collection path (default: home set + slug of name)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *name == "" {
		return errors.New("calendar create requires --name")
	}

	client, err := conn.client()
	if err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	target := *path
	if target == "" {
		homeSet, err := client.CalendarHomeSet(ctx)
		if err != nil {
			return err
		}
		target = strings.TrimRight(homeSet, "/") + "/" + slugify(*name) + "/"
	}

	spec := caldav.CalendarSpec{
		DisplayName: *name,
		Description: *desc,
		Color:       *color,
		Components:  components(*tasks, *both),
	}
	if err := client.CreateCalendar(ctx, target, spec); err != nil {
		return err
	}
	fmt.Printf("Created calendar %q at %s\n", *name, target)
	fmt.Println("Run `lazyplanner import` to pull it into the local cache.")
	return nil
}

func runCalendarDelete(args []string) error {
	fs := flag.NewFlagSet("calendar delete", flag.ContinueOnError)
	conn := addConnFlags(fs)
	path := fs.String("path", "", "collection path to delete (from `calendar list`) (required)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *path == "" {
		return errors.New("calendar delete requires --path (see `calendar list`)")
	}

	client, err := conn.client()
	if err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	if err := client.DeleteCalendar(ctx, *path); err != nil {
		return err
	}
	fmt.Printf("Deleted calendar at %s\n", *path)
	return nil
}

// components resolves the requested component set. Default is VEVENT only (a
// normal calendar); --tasks makes a VTODO-only task list; --both supports each.
func components(tasks, both bool) []string {
	switch {
	case both:
		return []string{"VEVENT", "VTODO"}
	case tasks:
		return []string{"VTODO"}
	default:
		return []string{"VEVENT"}
	}
}

// slugify turns a display name into a URL-safe collection path segment.
func slugify(name string) string {
	var b strings.Builder
	lastDash := false
	for _, r := range strings.ToLower(name) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
			lastDash = false
		default:
			if !lastDash && b.Len() > 0 {
				b.WriteByte('-')
				lastDash = true
			}
		}
	}
	slug := strings.Trim(b.String(), "-")
	if slug == "" {
		return "calendar"
	}
	return slug
}
