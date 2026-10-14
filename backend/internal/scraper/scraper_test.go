package scraper

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/farrellm/bbb/internal/bbb"
	"github.com/farrellm/bbb/internal/dbtest"
	"github.com/farrellm/bbb/internal/store"
)

// fakeSite serves whatever page the test sets.
type fakeSite struct {
	mu     sync.Mutex
	page   string
	status int
	ua     string
}

func (f *fakeSite) set(status int, page string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.status, f.page = status, page
}

func (f *fakeSite) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.ua = r.UserAgent()
	w.WriteHeader(f.status)
	_, _ = io.WriteString(w, f.page)
}

func setup(t *testing.T) (*Scraper, *fakeSite, *pgxpool.Pool) {
	t.Helper()
	pool := dbtest.New(t)
	site := &fakeSite{status: http.StatusOK}
	srv := httptest.NewServer(site)
	t.Cleanup(srv.Close)

	clock := time.Date(2026, time.October, 14, 16, 0, 0, 0, time.UTC)
	return &Scraper{
		Fetcher:  bbb.NewFetcher(srv.URL, "BBBWatch/test"),
		Store:    store.New(pool),
		Signup:   bbb.SignupDetector{Hosts: bbb.DefaultSignupHosts},
		Location: time.UTC,
		Interval: 5 * time.Minute,
		Log:      slog.New(slog.DiscardHandler),
		Now: func() time.Time {
			clock = clock.Add(time.Minute)
			return clock
		},
	}, site, pool
}

func sample(t *testing.T) string {
	t.Helper()
	b, err := os.ReadFile("../bbb/testdata/bbb-sample.html")
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestRunOnce(t *testing.T) {
	sc, site, pool := setup(t)
	ctx := context.Background()
	page := sample(t)

	// First run: baseline, no events.
	site.set(http.StatusOK, page)
	out, err := sc.RunOnce(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !out.Baseline || out.Items != 41 || out.Events != 0 {
		t.Errorf("first run: %+v", out)
	}
	if site.ua != "BBBWatch/test" {
		t.Errorf("User-Agent = %q", site.ua)
	}

	// Same body: skipped.
	out, err = sc.RunOnce(ctx)
	if err != nil || !out.Unchanged {
		t.Errorf("second run: %+v, %v", out, err)
	}

	// An edit and a removal.
	edited := strings.Replace(page, "November 1, 2026", "November 15, 2026", 1)
	at := strings.Index(edited, `id="fsEl_67762"`)
	start := strings.LastIndex(edited[:at], "<section")
	end := at + strings.Index(edited[at:], "</section>") + len("</section>")
	edited = edited[:start] + edited[end:]
	site.set(http.StatusOK, edited)
	out, err = sc.RunOnce(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if out.Events != 2 {
		t.Errorf("edit run: %+v", out)
	}

	type ev struct {
		panel        int64
		kind         string
		hasFrom      bool
		hasTo        bool
		added, remvd int
	}
	rows, err := pool.Query(ctx, `
		SELECT panel_id, kind::text, from_version_id IS NOT NULL, to_version_id IS NOT NULL,
		       words_added, words_removed
		FROM events WHERE run_id = $1 ORDER BY id`, out.RunID)
	if err != nil {
		t.Fatal(err)
	}
	var got []ev
	for rows.Next() {
		var e ev
		if err := rows.Scan(&e.panel, &e.kind, &e.hasFrom, &e.hasTo, &e.added, &e.remvd); err != nil {
			t.Fatal(err)
		}
		got = append(got, e)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	want := []ev{
		{67589, "edit", true, true, 1, 1},
		{67762, "removed", true, false, 0, 0},
	}
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Errorf("events = %+v, want %+v", got, want)
	}

	var versions int
	var removed bool
	if err := pool.QueryRow(ctx, `
		SELECT (SELECT count(*) FROM item_versions WHERE panel_id = 67589),
		       (SELECT removed_at IS NOT NULL FROM items WHERE panel_id = 67762)`).
		Scan(&versions, &removed); err != nil {
		t.Fatal(err)
	}
	if versions != 2 || !removed {
		t.Errorf("versions=%d removed=%v", versions, removed)
	}

	// The removed item comes back unchanged: no event, no longer removed.
	site.set(http.StatusOK, strings.Replace(page, "November 1, 2026", "November 15, 2026", 1))
	out, err = sc.RunOnce(ctx)
	if err != nil || out.Events != 0 {
		t.Errorf("reappear run: %+v, %v", out, err)
	}
	if err := pool.QueryRow(ctx, `SELECT removed_at IS NOT NULL FROM items WHERE panel_id = 67762`).Scan(&removed); err != nil {
		t.Fatal(err)
	}
	if removed {
		t.Error("67762 still marked removed")
	}
}

func TestRunOnceFailures(t *testing.T) {
	sc, site, pool := setup(t)
	ctx := context.Background()

	site.set(http.StatusOK, sample(t))
	if _, err := sc.RunOnce(ctx); err != nil {
		t.Fatal(err)
	}

	// A broken page is suspect: recorded, nothing removed.
	site.set(http.StatusOK, "<html><body>Maintenance</body></html>")
	out, err := sc.RunOnce(ctx)
	if !errors.Is(err, ErrSuspect) || !out.Suspect {
		t.Fatalf("suspect run: %+v, %v", out, err)
	}

	// An HTTP error is recorded with its status.
	site.set(http.StatusServiceUnavailable, "down")
	_, err = sc.RunOnce(ctx)
	var se *bbb.StatusError
	if !errors.As(err, &se) || se.Status != http.StatusServiceUnavailable {
		t.Fatalf("503 run: %v", err)
	}

	var removed, events int
	if err := pool.QueryRow(ctx, `
		SELECT (SELECT count(*) FROM items WHERE removed_at IS NOT NULL),
		       (SELECT count(*) FROM events)`).Scan(&removed, &events); err != nil {
		t.Fatal(err)
	}
	if removed != 0 || events != 0 {
		t.Errorf("bad runs changed state: removed=%d events=%d", removed, events)
	}

	streak, err := sc.Store.SuspectStreak(ctx)
	if err != nil || streak != 2 {
		t.Errorf("streak = %d, %v", streak, err)
	}

	var status int
	var msg string
	if err := pool.QueryRow(ctx, `SELECT http_status, error FROM scrape_runs ORDER BY id DESC LIMIT 1`).
		Scan(&status, &msg); err != nil {
		t.Fatal(err)
	}
	if status != http.StatusServiceUnavailable || msg == "" {
		t.Errorf("last run: status=%d error=%q", status, msg)
	}

	// Recovery resets the streak. Failed runs don't count as the previous
	// body, so the good page matches the last good run and is skipped.
	site.set(http.StatusOK, sample(t))
	out, err = sc.RunOnce(ctx)
	if err != nil || !out.Unchanged {
		t.Errorf("recovery: %+v, %v", out, err)
	}
	if streak, _ := sc.Store.SuspectStreak(ctx); streak != 0 {
		t.Errorf("streak after recovery = %d", streak)
	}
}

func TestCheckStuck(t *testing.T) {
	sc, site, _ := setup(t)
	ctx := context.Background()
	var calls []string
	sc.OnStuck = func(_ context.Context, runs int) { calls = append(calls, "stuck "+strconv.Itoa(runs)) }
	sc.OnRecovered = func(context.Context) { calls = append(calls, "recovered") }
	poll := func() {
		_, err := sc.RunOnce(ctx)
		sc.checkStuck(ctx, err == nil)
	}

	site.set(http.StatusOK, sample(t))
	poll()
	// Four bad runs alert once, at the third.
	site.set(http.StatusOK, "<html><body>Maintenance</body></html>")
	poll()
	poll()
	site.set(http.StatusBadGateway, "")
	poll()
	poll()
	site.set(http.StatusOK, sample(t))
	poll()
	poll()
	if got := strings.Join(calls, ", "); got != "stuck 3, recovered" {
		t.Errorf("calls = %s", got)
	}
}

func TestDelay(t *testing.T) {
	night := time.Date(2026, time.October, 14, 23, 30, 0, 0, time.UTC)
	day := time.Date(2026, time.October, 14, 12, 0, 0, 0, time.UTC)
	s := &Scraper{Location: time.UTC, Interval: 5 * time.Minute, NightInterval: 30 * time.Minute}

	tests := []struct {
		at       time.Time
		failures int
		want     time.Duration
	}{
		{day, 0, 5 * time.Minute},
		{night, 0, 30 * time.Minute},
		{day, 1, 10 * time.Minute},
		{day, 3, 40 * time.Minute},
		{day, 10, time.Hour},
	}
	for _, tt := range tests {
		s.Now = func() time.Time { return tt.at }
		for range 20 {
			d := s.delay(tt.failures)
			if d < tt.want-jitter || d > tt.want+jitter {
				t.Errorf("delay(%d) at %v = %v, want %v ± %v", tt.failures, tt.at, d, tt.want, jitter)
			}
		}
	}
}
