package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

type jobsCLIOptions struct {
	BaseURL  string
	Watch    bool
	Interval time.Duration
	JSON     bool
	Limit    int
}

func cmdJobs() {
	os.Exit(runJobsCommand(os.Args[2:], os.Stdout, os.Stderr, http.DefaultClient))
}

func runJobsCommand(args []string, out, errOut io.Writer, client *http.Client) int {
	subcmd := "notifications"
	if len(args) > 0 {
		subcmd = strings.ToLower(strings.TrimSpace(args[0]))
	}
	switch subcmd {
	case "notifications":
		return runJobsNotifications(args[1:], out, errOut, client)
	default:
		fmt.Fprintf(errOut, "unknown jobs subcommand: %s\n", subcmd)
		fmt.Fprintln(errOut, "usage: rencrow jobs notifications [--watch] [--json] [--url URL] [limit]")
		return 1
	}
}

func runJobsNotifications(args []string, out, errOut io.Writer, client *http.Client) int {
	opts, err := parseJobsNotificationsOptions(args)
	if err != nil {
		fmt.Fprintln(errOut, err)
		return 2
	}
	if client == nil {
		client = http.DefaultClient
	}
	seen := make(map[string]struct{})
	printOnce := func() error {
		items, err := fetchChatCLIJobNotifications(context.Background(), client, opts.BaseURL, opts.Limit)
		if err != nil {
			return err
		}
		if opts.JSON {
			writeJSONCLI(out, map[string]any{"items": items}, true)
			return nil
		}
		for i := len(items) - 1; i >= 0; i-- {
			item := items[i]
			key := chatCLIJobNotificationKey(item)
			if opts.Watch {
				if _, ok := seen[key]; ok {
					continue
				}
				seen[key] = struct{}{}
			}
			fmt.Fprint(out, formatChatCLIJobNotification(item))
		}
		return nil
	}
	if err := printOnce(); err != nil {
		fmt.Fprintf(errOut, "job notifications unavailable: %v\n", err)
		return 1
	}
	if !opts.Watch {
		return 0
	}
	ticker := time.NewTicker(opts.Interval)
	defer ticker.Stop()
	for range ticker.C {
		if err := printOnce(); err != nil {
			fmt.Fprintf(errOut, "job notifications unavailable: %v\n", err)
		}
	}
	return 0
}

func parseJobsNotificationsOptions(args []string) (jobsCLIOptions, error) {
	opts := jobsCLIOptions{
		BaseURL:  defaultChatCLIBaseURL,
		Interval: 3 * time.Second,
		Limit:    20,
	}
	if envURL := strings.TrimSpace(os.Getenv(chatCLIBaseURLEnv)); envURL != "" {
		opts.BaseURL = envURL
	}
	fs := flag.NewFlagSet("jobs notifications", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.StringVar(&opts.BaseURL, "url", opts.BaseURL, "RenCrow server base URL")
	fs.BoolVar(&opts.Watch, "watch", false, "watch job interrupt notifications")
	fs.DurationVar(&opts.Interval, "interval", opts.Interval, "watch polling interval")
	fs.BoolVar(&opts.JSON, "json", false, "output JSON")
	if err := fs.Parse(args); err != nil {
		return opts, fmt.Errorf("usage: rencrow jobs notifications [--watch] [--json] [--url URL] [limit]")
	}
	if opts.Interval <= 0 {
		return opts, fmt.Errorf("interval must be positive")
	}
	opts.BaseURL = strings.TrimRight(strings.TrimSpace(opts.BaseURL), "/")
	if opts.BaseURL == "" {
		return opts, fmt.Errorf("url is required")
	}
	for _, arg := range fs.Args() {
		n, err := strconv.Atoi(strings.TrimSpace(arg))
		if err != nil || n <= 0 {
			return opts, fmt.Errorf("invalid limit: %s", arg)
		}
		opts.Limit = n
	}
	return opts, nil
}
