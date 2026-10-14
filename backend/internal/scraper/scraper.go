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
	// alertAfter is how many bad runs in a row alert the admins.
	alertAfter = 3
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
	// OnStuck, if set, is called once when runs in a row have failed or
	// been suspect, and OnRecovered at the next good run after that.
	OnStuck     func(ctx context.Context, runs int)
	OnRecovered func(ctx context.Context)

	// alerted is true from OnStuck until OnRecovered.
	alerted bool
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
		if ctx.Err() != nil {
			return
		}
		if err != nil {
			failures++
			s.Log.Warn("scrape failed", "err", err, "run", out.RunID, "failures", failures)
		} else {
			failures = 0
			s.Log.Info("scraped", "run", out.RunID, "unchanged", out.Unchanged,
				"baseline", out.Baseline, "items", out.Items, "events", out.Events)
			if out.Events > 0 && s.OnEvents != nil {
				s.OnEvents()
			}
		}
		s.checkStuck(ctx, err == nil)

		select {
		case <-ctx.Done():
			return
		case <-time.After(s.delay(failures)):
		}
	}
}

// checkStuck alerts once the stored runs show alertAfter bad ones in a
// row, and gives the all-clear at the next good run. The streak comes from
// the database, so a restart mid-outage doesn't reset it (though it does
// alert again).
func (s *Scraper) checkStuck(ctx context.Context, ok bool) {
	if ok {
		if s.alerted {
			s.alerted = false
			s.Log.Info("scraper recovered")
			if s.OnRecovered != nil {
				s.OnRecovered(ctx)
			}
		}
		return
	}
	if s.alerted {
		return
	}
	n, err := s.Store.SuspectStreak(ctx)
	if err != nil {
		s.Log.Warn("suspect streak", "err", err)
		return
	}
	if n >= alertAfter {
		s.alerted = true
		s.Log.Error("scraper has failed repeatedly; check the page structure", "runs", n)
		if s.OnStuck != nil {
			s.OnStuck(ctx, n)
		}
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
