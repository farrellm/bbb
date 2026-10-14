// Package scraper runs the poll loop: fetch the page, skip it if the body
// is unchanged, parse, diff against the stored state and record the run.
package scraper

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"math/rand/v2"
	"strings"
	"time"

	"github.com/farrellm/bbb/internal/bbb"
	"github.com/farrellm/bbb/internal/store"
)

const (
	jitter     = 30 * time.Second
	maxBackoff = time.Hour
	// suspectAlertAfter is how many bad runs in a row get logged as an alert.
	suspectAlertAfter = 3
)

// Scraper polls the page and records what changed.
type Scraper struct {
	Fetcher *bbb.Fetcher
	Store   *store.Store
	Signup  bbb.SignupDetector
	// Location is the school's time zone.
	Location      *time.Location
	Interval      time.Duration
	NightInterval time.Duration // 0 disables the overnight back-off
	Log           *slog.Logger
	Now           func() time.Time
	// OnEvents, if set, is called after a run that recorded events, once
	// they are committed.
	OnEvents func()
}

// Outcome summarizes one run for logging and tests.
type Outcome struct {
	RunID     int64
	Unchanged bool
	Suspect   bool
	Baseline  bool
	Items     int
	Events    int
}

// ErrSuspect is returned by RunOnce when the parsed page failed the sanity
// guard and nothing was applied.
var ErrSuspect = errors.New("scraper: suspect page")

func (s *Scraper) now() time.Time {
	if s.Now != nil {
		return s.Now().In(s.Location)
	}
	return time.Now().In(s.Location)
}

// RunOnce performs one poll. Failures are recorded in scrape_runs as well
// as returned.
func (s *Scraper) RunOnce(ctx context.Context) (Outcome, error) {
	run := store.ScrapeRun{StartedAt: s.now()}
	fail := func(err error) (Outcome, error) {
		run.FinishedAt = s.now()
		run.Error = err.Error()
		id, rerr := s.Store.RecordRun(ctx, run, nil)
		return Outcome{RunID: id}, errors.Join(err, rerr)
	}

	page, err := s.Fetcher.Fetch(ctx)
	run.HTTPStatus = page.Status
	if err != nil {
		return fail(err)
	}
	run.BodyHash = page.Hash

	last, err := s.Store.LastBodyHash(ctx)
	if err != nil {
		return fail(err)
	}
	if last == page.Hash {
		run.Unchanged = true
		run.FinishedAt = s.now()
		id, err := s.Store.RecordRun(ctx, run, nil)
		return Outcome{RunID: id, Unchanged: true}, err
	}

	parsed, err := bbb.Parse(bytes.NewReader(page.Body))
	if err != nil {
		return fail(err)
	}
	state, err := s.Store.LoadState(ctx)
	if err != nil {
		return fail(err)
	}
	plan := bbb.Diff(state, parsed, s.Signup, run.StartedAt)

	n := len(parsed.Items)
	run.ItemCount = &n
	run.Suspect = plan.Suspect
	if plan.Suspect {
		run.Error = "suspect: " + strings.Join(plan.SuspectReasons, "; ")
	}
	run.FinishedAt = s.now()
	id, err := s.Store.RecordRun(ctx, run, &plan)
	out := Outcome{
		RunID: id, Suspect: plan.Suspect, Baseline: plan.Baseline,
		Items: n, Events: len(plan.Events),
	}
	if err != nil {
		return out, err
	}
	if plan.Suspect {
		return out, ErrSuspect
	}
	return out, nil
}

// Run polls until ctx is done. Errors are logged and back off
// exponentially; they never stop the loop.
func (s *Scraper) Run(ctx context.Context) {
	failures := 0
	for {
		out, err := s.RunOnce(ctx)
		switch {
		case ctx.Err() != nil:
			return
		case err != nil:
			failures++
			s.Log.Warn("scrape failed", "err", err, "run", out.RunID, "failures", failures)
			s.alertIfStuck(ctx)
		default:
			failures = 0
			s.Log.Info("scraped", "run", out.RunID, "unchanged", out.Unchanged,
				"baseline", out.Baseline, "items", out.Items, "events", out.Events)
			if out.Events > 0 && s.OnEvents != nil {
				s.OnEvents()
			}
		}

		select {
		case <-ctx.Done():
			return
		case <-time.After(s.delay(failures)):
		}
	}
}

// alertIfStuck logs an error once several runs in a row have failed. The
// admin push for this arrives with notifications.
func (s *Scraper) alertIfStuck(ctx context.Context) {
	n, err := s.Store.SuspectStreak(ctx)
	if err != nil {
		s.Log.Warn("suspect streak", "err", err)
		return
	}
	if n == suspectAlertAfter {
		s.Log.Error("scraper has failed repeatedly; check the page structure", "runs", n)
	}
}

// delay is the wait before the next poll: the interval (or the overnight
// one), doubled for each consecutive failure up to maxBackoff, ±30 s.
func (s *Scraper) delay(failures int) time.Duration {
	d := s.Interval
	if h := s.now().Hour(); s.NightInterval > 0 && (h >= 23 || h < 6) {
		d = s.NightInterval
	}
	for range min(failures, 8) {
		d *= 2
	}
	d = min(d, maxBackoff)
	// Jitter only spreads requests out; it needs no cryptographic quality.
	d += time.Duration(rand.Int64N(int64(2*jitter))) - jitter //nolint:gosec // G404: not security-sensitive
	return max(d, time.Second)
}
