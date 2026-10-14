// Command server runs BBB Watch. For now that is the poller; the HTTP API
// and web app arrive in later milestones.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/farrellm/bbb/internal/bbb"
	"github.com/farrellm/bbb/internal/config"
	"github.com/farrellm/bbb/internal/db"
	"github.com/farrellm/bbb/internal/scraper"
	"github.com/farrellm/bbb/internal/store"
)

var once = flag.Bool("scrape-once", false, "poll the page once, print the outcome, then exit")

func main() {
	flag.Parse()
	if err := run(context.Background()); err != nil {
		slog.Error("server error", "err", err)
		os.Exit(1)
	}
}

func run(ctx context.Context) error {
	ctx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stop()

	cfg, err := config.Load()
	if err != nil {
		return err
	}

	pool, err := db.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		return fmt.Errorf("database connect: %w", err)
	}
	defer pool.Close()

	log := slog.Default()
	sc := &scraper.Scraper{
		Fetcher:       bbb.NewFetcher(cfg.BBBURL, cfg.UserAgent),
		Store:         store.New(pool),
		Signup:        bbb.SignupDetector{Hosts: cfg.SignupHosts},
		Location:      cfg.SchoolTZ,
		Interval:      cfg.PollInterval,
		NightInterval: cfg.NightPollInterval,
		Log:           log,
	}

	if *once {
		out, err := sc.RunOnce(ctx)
		fmt.Printf("run %d: unchanged=%v baseline=%v suspect=%v items=%d events=%d\n",
			out.RunID, out.Unchanged, out.Baseline, out.Suspect, out.Items, out.Events)
		if errors.Is(err, scraper.ErrSuspect) {
			return nil
		}
		return err
	}

	log.Info("poller started", "url", cfg.BBBURL, "interval", cfg.PollInterval)
	sc.Run(ctx)
	return nil
}
