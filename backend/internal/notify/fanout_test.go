package notify_test

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/farrellm/bbb/internal/bbb"
	"github.com/farrellm/bbb/internal/dbtest"
	"github.com/farrellm/bbb/internal/notify"
	"github.com/farrellm/bbb/internal/notify/notifytest"
	"github.com/farrellm/bbb/internal/store"
)

type fixture struct {
	t    *testing.T
	pool *pgxpool.Pool
	st   *store.Store
	push *notifytest.Service
	n    *notify.Notifier
	page *bbb.Page
	at   time.Time
}

func setup(t *testing.T) *fixture {
	t.Helper()
	pool := dbtest.New(t)
	f := &fixture{t: t, pool: pool, st: store.New(pool), push: notifytest.New(t), at: time.Now()}
	pub, priv, err := notify.GenerateKeys()
	if err != nil {
		t.Fatal(err)
	}
	f.n = notify.New(f.st, &notify.Sender{
		PublicKey: pub, PrivateKey: priv, Subject: "mailto:test@example.org", Client: f.push.Client(),
	}, slog.New(slog.DiscardHandler))
	f.n.Backoff = time.Millisecond

	raw, err := os.ReadFile("../bbb/testdata/bbb-sample.html")
	if err != nil {
		t.Fatal(err)
	}
	if f.page, err = bbb.Parse(strings.NewReader(string(raw))); err != nil {
		t.Fatal(err)
	}
	f.record() // the baseline: no events
	return f
}

// record runs the differ on the current page, as the poller would.
func (f *fixture) record() {
	f.t.Helper()
	ctx := context.Background()
	state, err := f.st.LoadState(ctx)
	if err != nil {
		f.t.Fatal(err)
	}
	f.at = f.at.Add(time.Minute)
	plan := bbb.Diff(state, f.page, bbb.SignupDetector{Hosts: bbb.DefaultSignupHosts}, f.at)
	n := len(f.page.Items)
	run := store.ScrapeRun{StartedAt: f.at, FinishedAt: f.at, HTTPStatus: 200, BodyHash: f.at.String(), ItemCount: &n}
	if _, err := f.st.RecordRun(ctx, run, &plan); err != nil {
		f.t.Fatal(err)
	}
}

// edit appends words to the titles of the first n items in division.
func (f *fixture) edit(division bbb.Division, n int) {
	for i := range f.page.Items {
		if it := &f.page.Items[i]; it.Division == division && n > 0 {
			it.Title += " now updated"
			n--
		}
	}
}

func (f *fixture) user(name string, prefs func(*store.Preferences)) int64 {
	f.t.Helper()
	ctx := context.Background()
	u, err := f.st.SignInUser(ctx, store.SignIn{Subject: name, Email: name + "@example.org", Admin: true})
	if err != nil {
		f.t.Fatal(err)
	}
	if prefs != nil {
		p, err := f.st.GetPreferences(ctx, u.ID)
		if err != nil {
			f.t.Fatal(err)
		}
		prefs(&p)
		if err := f.st.SetPreferences(ctx, u.ID, p); err != nil {
			f.t.Fatal(err)
		}
	}
	return u.ID
}

func (f *fixture) device(userID int64) notifytest.Subscription {
	f.t.Helper()
	sub := f.push.Subscribe()
	_, err := f.st.SaveSubscription(context.Background(), store.Subscription{
		UserID: userID, Endpoint: sub.Endpoint, P256dh: sub.Keys.P256dh, Auth: sub.Keys.Auth,
	})
	if err != nil {
		f.t.Fatal(err)
	}
	return sub
}

func (f *fixture) flush() []notifytest.Push {
	f.t.Helper()
	if err := f.n.Flush(context.Background()); err != nil {
		f.t.Fatal(err)
	}
	return f.push.Pushes()
}

func TestFlush(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	everything := f.user("a", nil)
	phone := f.device(everything)
	lost := f.device(everything)
	f.push.Answer(lost.Endpoint, http.StatusGone)
	lowerOnly := f.user("b", func(p *store.Preferences) { p.Divisions = []string{"lower"} })
	lowerPhone := f.device(lowerOnly)
	f.user("no-devices", nil)

	if got := f.flush(); len(got) != 0 {
		t.Fatalf("baseline pushed %d", len(got))
	}

	// One All School edit: only the parent following All School hears.
	f.page.Items[0].Subheader = "Deadline: Monday, November 2"
	f.record()
	got := f.flush()
	if len(got) != 1 {
		t.Fatalf("got %d pushes, want 1: %+v", len(got), got)
	}
	p := got[0]
	if p.Endpoint != phone.Endpoint || p.Topic != "item-67589" || p.TTL != "86400" {
		t.Errorf("push = %+v", p)
	}
	if p.Payload.Title != "Edited in All School" ||
		p.Payload.Body != "2027-28 Tuition Assistance Application: 3 words added, 3 removed" ||
		!strings.HasPrefix(p.Payload.URL, "/items/67589/diff?from=") || p.Payload.Badge != 1 {
		t.Errorf("payload = %+v", p.Payload)
	}
	// The gone device was dropped.
	if subs, _ := f.st.ListSubscriptions(ctx, everything); len(subs) != 1 || subs[0].LastOKAt == nil {
		t.Errorf("subscriptions after 410: %+v", subs)
	}
	// Handled once.
	if got := f.flush(); len(got) != 0 {
		t.Errorf("second flush pushed %d", len(got))
	}

	// Four Lower School edits in one run arrive as one summary each.
	f.edit(bbb.DivisionLower, 4)
	f.record()
	got = f.flush()
	if len(got) != 2 {
		t.Fatalf("got %d pushes, want 2: %+v", len(got), got)
	}
	for _, p := range got {
		if p.Payload.Title != "4 BBB updates" || p.Payload.Body != "4 edited" || p.Payload.URL != "/" {
			t.Errorf("summary = %+v", p.Payload)
		}
	}
	if got[1].Endpoint != lowerPhone.Endpoint {
		t.Errorf("second summary went to %s", got[1].Endpoint)
	}
}

func TestFlushRetries(t *testing.T) {
	f := setup(t)
	phone := f.device(f.user("a", nil))
	f.push.Answer(phone.Endpoint, http.StatusTooManyRequests, http.StatusServiceUnavailable)

	f.edit(bbb.DivisionUpper, 1)
	f.record()
	if got := f.flush(); len(got) != 1 || got[0].Payload.Title != "Edited in Upper School" {
		t.Fatalf("pushes = %+v", got)
	}

	// Every failed attempt is in deliveries, then the success.
	rows, err := f.pool.Query(context.Background(), `SELECT status FROM deliveries ORDER BY id`)
	if err != nil {
		t.Fatal(err)
	}
	statuses, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil || strings.Join(statuses, ",") != "failed,failed,sent" {
		t.Errorf("deliveries = %v", statuses)
	}
}

func TestFlushWithoutSender(t *testing.T) {
	f := setup(t)
	f.device(f.user("a", nil))
	f.n.Sender = nil

	f.edit(bbb.DivisionUpper, 1)
	f.record()
	if got := f.flush(); len(got) != 0 {
		t.Fatalf("pushed %d with push off", len(got))
	}
	// The events were stamped anyway, so turning push on doesn't replay them.
	if evs, err := f.st.PendingEvents(context.Background()); err != nil || len(evs) != 0 {
		t.Errorf("pending after flush: %d, %v", len(evs), err)
	}
}

func TestTest(t *testing.T) {
	f := setup(t)
	u := f.user("a", nil)
	f.device(u)
	dead := f.device(u)
	f.push.Answer(dead.Endpoint, http.StatusNotFound)

	res, err := f.n.Test(context.Background(), u)
	if err != nil || res.Sent != 1 || res.Failed != 1 {
		t.Fatalf("Test = %+v, %v", res, err)
	}
	if got := f.push.Pushes(); len(got) != 1 || got[0].Payload.Tag != "test" || got[0].Payload.URL != "/settings" {
		t.Errorf("pushes = %+v", got)
	}
	if subs, _ := f.st.ListSubscriptions(context.Background(), u); len(subs) != 1 {
		t.Errorf("%d subscriptions left, want 1", len(subs))
	}
}

func TestQuietHours(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	clock := time.Date(2026, time.October, 14, 22, 0, 0, 0, time.UTC)
	f.n.Location = time.UTC
	f.n.Now = func() time.Time { return clock }

	night := f.user("night", func(p *store.Preferences) {
		start, end := "21:30", "07:00"
		p.QuietStart, p.QuietEnd = &start, &end
	})
	nightPhone := f.device(night)
	dayPhone := f.device(f.user("day", nil))

	// Three edits in three runs during quiet hours: only the parent
	// without quiet hours hears, one push each.
	for _, d := range []bbb.Division{bbb.DivisionUpper, bbb.DivisionLower, bbb.DivisionMiddle} {
		f.edit(d, 1)
		f.record()
		got := f.flush()
		if len(got) != 1 || got[0].Endpoint != dayPhone.Endpoint {
			t.Fatalf("quiet-hours pushes = %+v", got)
		}
	}
	clock = time.Date(2026, time.October, 15, 6, 59, 0, 0, time.UTC)
	if got := f.flush(); len(got) != 0 {
		t.Fatalf("pushed %d before quiet hours ended", len(got))
	}

	// When they end, what was held arrives as one summary, once.
	clock = clock.Add(time.Minute)
	got := f.flush()
	if len(got) != 1 || got[0].Endpoint != nightPhone.Endpoint {
		t.Fatalf("release pushes = %+v", got)
	}
	if p := got[0].Payload; p.Title != "3 BBB updates" || p.Body != "3 edited" || p.URL != "/" || p.Badge != 3 {
		t.Errorf("release = %+v", p)
	}
	if got := f.flush(); len(got) != 0 {
		t.Errorf("released twice: %+v", got)
	}

	// Held events read in the app meanwhile are left out: of two, the
	// one left arrives as itself.
	clock = time.Date(2026, time.October, 15, 23, 0, 0, 0, time.UTC)
	f.edit(bbb.DivisionUpper, 1)
	f.record()
	f.page.Items[0].Subheader = "Deadline: Monday, November 2"
	f.record()
	f.flush()
	var upper int64
	if err := f.pool.QueryRow(ctx, `
		SELECT e.id FROM events e JOIN items i USING (panel_id)
		WHERE i.division = 'upper' ORDER BY e.id DESC LIMIT 1`).Scan(&upper); err != nil {
		t.Fatal(err)
	}
	if err := f.st.MarkRead(ctx, night, []int64{upper}); err != nil {
		t.Fatal(err)
	}
	clock = clock.Add(8 * time.Hour)
	got = f.flush()
	if len(got) != 1 || got[0].Endpoint != nightPhone.Endpoint || got[0].Payload.Title != "Edited in All School" {
		t.Fatalf("release after read = %+v", got)
	}
	var held int
	if err := f.pool.QueryRow(ctx, `SELECT count(*) FROM held_events`).Scan(&held); err != nil || held != 0 {
		t.Errorf("held_events left: %d, %v", held, err)
	}
}

func TestAlert(t *testing.T) {
	f := setup(t)
	admin := f.device(f.user("admin", nil)) // the fixture's users are admins
	ctx := context.Background()
	if _, err := f.st.CreateInvite(ctx, "code", 0, 1, nil); err != nil {
		t.Fatal(err)
	}
	parent, err := f.st.SignInUser(ctx, store.SignIn{Subject: "p", Email: "p@example.org", Invite: "code"})
	if err != nil {
		t.Fatal(err)
	}
	f.device(parent.ID)

	f.n.Alert(ctx, notify.ScraperStuck(3))
	got := f.push.Pushes()
	if len(got) != 1 || got[0].Endpoint != admin.Endpoint || got[0].Payload.Tag != "scraper" ||
		got[0].Payload.Title != "BBB Watch can't read the page" {
		t.Errorf("alert pushes = %+v", got)
	}
}
