package handler

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/farrellm/bbb/internal/auth"
	"github.com/farrellm/bbb/internal/auth/authtest"
	"github.com/farrellm/bbb/internal/bbb"
	"github.com/farrellm/bbb/internal/dbtest"
	"github.com/farrellm/bbb/internal/notify"
	"github.com/farrellm/bbb/internal/notify/notifytest"
	"github.com/farrellm/bbb/internal/store"
)

func TestMain(m *testing.M) {
	slog.SetDefault(slog.New(slog.DiscardHandler))
	os.Exit(m.Run())
}

const pageURL = "https://www.pingry.org/experience/community-life/bbb"

type env struct {
	t     *testing.T
	srv   *httptest.Server
	store *store.Store
	idp   *authtest.Provider
	push  *notifytest.Service
}

func setup(t *testing.T) *env {
	t.Helper()
	pool := dbtest.New(t)
	e := &env{t: t, store: store.New(pool), idp: authtest.New(t), push: notifytest.New(t)}
	var h http.Handler
	e.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h.ServeHTTP(w, r)
	}))
	t.Cleanup(e.srv.Close)

	pub, priv, err := notify.GenerateKeys()
	if err != nil {
		t.Fatal(err)
	}
	notifier := notify.New(e.store, &notify.Sender{
		PublicKey: pub, PrivateKey: priv, Subject: "mailto:test@example.org", Client: e.push.Client(),
	}, slog.New(slog.DiscardHandler))

	secret := strings.Repeat("s", 32)
	api := &API{
		Store:       e.store,
		Sessions:    auth.NewSessions(secret, false),
		Flows:       auth.NewFlows(secret, false),
		Google:      auth.NewGoogle(context.Background(), e.idp.Config(), authtest.ClientID, "secret", e.srv.URL+"/api/auth/google/callback"),
		PublicURL:   "https://bbb.example",
		AdminEmails: []string{"admin@example.org"},
		PageURL:     pageURL,
		Notifier:    notifier,
	}
	h = api.Routes()
	return e
}

// browser is a client with its own cookie jar that doesn't follow
// redirects, so tests can check where each one goes.
func (e *env) browser() *http.Client {
	jar, _ := cookiejar.New(nil)
	return &http.Client{
		Jar: jar,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
}

// reply is a response already read, so tests needn't close bodies.
type reply struct {
	method, path string
	StatusCode   int
	Header       http.Header
	Body         []byte
}

func (e *env) do(c *http.Client, method, path, body string) *reply {
	e.t.Helper()
	var r io.Reader
	if body != "" {
		r = strings.NewReader(body)
	}
	req, err := http.NewRequestWithContext(context.Background(), method, e.srv.URL+path, r)
	if err != nil {
		e.t.Fatal(err)
	}
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	res, err := c.Do(req)
	if err != nil {
		e.t.Fatal(err)
	}
	defer res.Body.Close()
	b, err := io.ReadAll(res.Body)
	if err != nil {
		e.t.Fatal(err)
	}
	rep := &reply{method: method, path: req.URL.Path, StatusCode: res.StatusCode, Header: res.Header, Body: b}
	checkContract(e.t, e.srv.URL, req, rep)
	return rep
}

func (e *env) getJSON(c *http.Client, path string, want int, dst any) {
	e.t.Helper()
	res := e.do(c, http.MethodGet, path, "")
	decode(e.t, res, want, dst)
}

func decode(t *testing.T, res *reply, want int, dst any) {
	t.Helper()
	if res.StatusCode != want {
		t.Fatalf("%s %s: status %d, want %d: %s", res.method, res.path, res.StatusCode, want, res.Body)
	}
	if dst != nil {
		if err := json.Unmarshal(res.Body, dst); err != nil {
			t.Fatalf("decode %s: %v", res.Body, err)
		}
	}
}

// signIn runs the Google flow in c from /start with the given query and
// returns where the callback redirected.
func (e *env) signIn(c *http.Client, query string, acct authtest.Account) string {
	e.t.Helper()
	res := e.do(c, http.MethodGet, "/api/auth/google/start?"+query, "")
	if res.StatusCode != http.StatusFound {
		e.t.Fatalf("start: status %d", res.StatusCode)
	}
	back := e.idp.Consent(e.t, res.Header.Get("Location"), acct)
	cb, err := url.Parse(back)
	if err != nil {
		e.t.Fatal(err)
	}
	res = e.do(c, http.MethodGet, cb.RequestURI(), "")
	if res.StatusCode != http.StatusFound {
		e.t.Fatalf("callback: status %d", res.StatusCode)
	}
	return res.Header.Get("Location")
}

func (e *env) invite() string {
	e.t.Helper()
	inv, err := e.store.CreateInvite(context.Background(), auth.RandomToken(), 0, 1, nil)
	if err != nil {
		e.t.Fatal(err)
	}
	return inv.Code
}

func parent(n string) authtest.Account {
	return authtest.Account{Subject: "sub-" + n, Email: n + "@example.org", EmailVerified: true, Name: "Parent " + n}
}

func TestSignInNeedsInvite(t *testing.T) {
	e := setup(t)

	if got := e.signIn(e.browser(), "", parent("a")); got != "/join?error=invite_required" {
		t.Errorf("no invite: redirected to %q", got)
	}
	if got := e.signIn(e.browser(), "invite=bogus", parent("a")); got != "/join?error=invite_invalid" {
		t.Errorf("bad invite: redirected to %q", got)
	}

	code := e.invite()
	var check struct{ Usable bool }
	e.getJSON(e.browser(), "/api/auth/invites/"+code, http.StatusOK, &check)
	if !check.Usable {
		t.Error("fresh invite not usable")
	}

	c := e.browser()
	if got := e.signIn(c, "invite="+code+"&next=/items/5", parent("a")); got != "/items/5" {
		t.Errorf("with invite: redirected to %q", got)
	}
	var me userJSON
	e.getJSON(c, "/api/me", http.StatusOK, &me)
	if me.Email != "a@example.org" || me.Name != "Parent a" || me.IsAdmin {
		t.Errorf("me = %+v", me)
	}

	// The invite allowed one account; it is used up for the next parent,
	// but the first can sign in again without it.
	if got := e.signIn(e.browser(), "invite="+code, parent("b")); got != "/join?error=invite_invalid" {
		t.Errorf("used invite: redirected to %q", got)
	}
	if got := e.signIn(e.browser(), "", parent("a")); got != "/" {
		t.Errorf("returning user: redirected to %q", got)
	}

	// Signing out ends the session.
	e.do(c, http.MethodPost, "/api/auth/logout", "")
	e.getJSON(c, "/api/me", http.StatusUnauthorized, nil)
}

func TestSignInRejects(t *testing.T) {
	e := setup(t)

	unverified := parent("u")
	unverified.EmailVerified = false
	if got := e.signIn(e.browser(), "invite="+e.invite(), unverified); got != "/login?error=email_unverified" {
		t.Errorf("unverified email: redirected to %q", got)
	}

	// A callback whose state doesn't match the flow cookie.
	c := e.browser()
	res := e.do(c, http.MethodGet, "/api/auth/google/start", "")
	back := e.idp.Consent(t, res.Header.Get("Location"), parent("a"))
	back = strings.Replace(back, "state=", "state=x", 1)
	cb, _ := url.Parse(back)
	res = e.do(c, http.MethodGet, cb.RequestURI(), "")
	if got := res.Header.Get("Location"); got != "/login?error=expired" {
		t.Errorf("state mismatch: redirected to %q", got)
	}

	// An open redirect through next is not allowed.
	if got := e.signIn(e.browser(), "next=//evil.example&invite="+e.invite(), parent("c")); got != "/" {
		t.Errorf("next=//evil: redirected to %q", got)
	}
}

func TestAdminEmail(t *testing.T) {
	e := setup(t)
	admin := e.browser()
	if got := e.signIn(admin, "", authtest.Account{Subject: "s-admin", Email: "Admin@Example.org", EmailVerified: true}); got != "/" {
		t.Fatalf("admin without invite: redirected to %q", got)
	}
	var me userJSON
	e.getJSON(admin, "/api/me", http.StatusOK, &me)
	if !me.IsAdmin {
		t.Error("ADMIN_EMAILS user is not admin")
	}

	var inv inviteJSON
	decode(t, e.do(admin, http.MethodPost, "/api/admin/invites", `{"maxUses": 2}`), http.StatusCreated, &inv)
	if inv.MaxUses != 2 || inv.ExpiresAt == nil || !strings.HasPrefix(inv.URL, "https://bbb.example/join?invite=") {
		t.Errorf("invite = %+v", inv)
	}
	e.getJSON(admin, "/api/admin/scrape-runs", http.StatusOK, nil)

	user := e.browser()
	e.signIn(user, "invite="+inv.Code, parent("p"))
	decode(t, e.do(user, http.MethodPost, "/api/admin/invites", `{}`), http.StatusForbidden, nil)

	// Deleting an account ends its sessions.
	if err := e.store.DeleteUser(context.Background(), "P@example.org"); err != nil {
		t.Fatal(err)
	}
	e.getJSON(user, "/api/items", http.StatusUnauthorized, nil)
}

func TestHandoff(t *testing.T) {
	e := setup(t)
	app := e.browser()    // the iOS home-screen app
	safari := e.browser() // where sign-in actually happens

	var h struct{ Nonce string }
	decode(t, e.do(app, http.MethodPost, "/api/auth/handoff", ""), http.StatusOK, &h)
	e.getJSON(app, "/api/auth/handoff/"+h.Nonce, http.StatusAccepted, nil)

	start := e.do(safari, http.MethodGet, "/api/auth/google/start?handoff="+h.Nonce+"&invite="+e.invite(), "")
	if !strings.Contains(start.Header.Get("Location"), "prompt=select_account") {
		t.Error("handoff sign-in should force the account chooser")
	}
	back := e.idp.Consent(t, start.Header.Get("Location"), parent("i"))
	cb, _ := url.Parse(back)
	if got := e.do(safari, http.MethodGet, cb.RequestURI(), "").Header.Get("Location"); got != "/signed-in" {
		t.Fatalf("handoff callback: redirected to %q", got)
	}

	e.getJSON(app, "/api/me", http.StatusUnauthorized, nil)
	var u userJSON
	e.getJSON(app, "/api/auth/handoff/"+h.Nonce, http.StatusOK, &u)
	if u.Email != "i@example.org" {
		t.Errorf("handoff user = %+v", u)
	}
	e.getJSON(app, "/api/me", http.StatusOK, nil)
	// Single use.
	e.getJSON(e.browser(), "/api/auth/handoff/"+h.Nonce, http.StatusNotFound, nil)
}

func TestNeedsSession(t *testing.T) {
	e := setup(t)
	for _, p := range []string{
		"/api/me", "/api/items", "/api/events", "/api/status", "/api/items/1",
		"/api/preferences", "/api/push/devices",
	} {
		e.getJSON(e.browser(), p, http.StatusUnauthorized, nil)
	}
	e.getJSON(e.browser(), "/api/nope", http.StatusNotFound, nil)
	e.getJSON(e.browser(), "/api/healthz", http.StatusOK, nil)
}

// record runs the differ on page against the store, as the poller would.
func record(t *testing.T, st *store.Store, page string, at time.Time) {
	t.Helper()
	ctx := context.Background()
	p, err := bbb.Parse(strings.NewReader(page))
	if err != nil {
		t.Fatal(err)
	}
	state, err := st.LoadState(ctx)
	if err != nil {
		t.Fatal(err)
	}
	plan := bbb.Diff(state, p, bbb.SignupDetector{Hosts: bbb.DefaultSignupHosts}, at)
	n := len(p.Items)
	if _, err := st.RecordRun(ctx, store.ScrapeRun{StartedAt: at, FinishedAt: at, HTTPStatus: 200, BodyHash: at.String(), ItemCount: &n}, &plan); err != nil {
		t.Fatal(err)
	}
}

func TestFeed(t *testing.T) {
	e := setup(t)
	raw, err := os.ReadFile("../bbb/testdata/bbb-sample.html")
	if err != nil {
		t.Fatal(err)
	}
	page := string(raw)
	record(t, e.store, page, time.Now().Add(-time.Hour))

	c := e.browser()
	e.signIn(c, "invite="+e.invite(), parent("f"))

	edited := strings.Replace(page, "Deadline: Sunday, November 1", "Deadline: Monday, November 2", 1)
	if edited == page {
		t.Fatal("fixture no longer has the tuition deadline")
	}
	record(t, e.store, edited, time.Now())

	var list itemListJSON
	e.getJSON(c, "/api/items", http.StatusOK, &list)
	if len(list.Items) != 41 || list.LastReadAt != nil {
		t.Fatalf("items: %d, lastReadAt %v", len(list.Items), list.LastReadAt)
	}
	var tuition *itemJSON
	unread := 0
	for i, it := range list.Items {
		if it.PanelID == 67589 {
			tuition = &list.Items[i]
		}
		if it.LatestEvent != nil && it.LatestEvent.Unread {
			unread++
		}
	}
	if unread != 1 || tuition == nil || tuition.LatestEvent == nil {
		t.Fatalf("unread = %d, tuition = %+v", unread, tuition)
	}
	ev := tuition.LatestEvent
	if ev.Kind != "edit" || !ev.Unread || ev.WordsAdded != 3 || ev.WordsRemoved != 3 || tuition.VersionCount != 2 {
		t.Errorf("tuition event = %+v, versions %d", ev, tuition.VersionCount)
	}
	if tuition.Deadline == nil || !strings.HasSuffix(*tuition.Deadline, "-11-02") || !tuition.IsSignup {
		t.Errorf("tuition deadline %v, signup %v", tuition.Deadline, tuition.IsSignup)
	}

	var lower itemListJSON
	e.getJSON(c, "/api/items?division=lower", http.StatusOK, &lower)
	if len(lower.Items) != 10 {
		t.Errorf("lower items = %d", len(lower.Items))
	}
	e.getJSON(c, "/api/items?division=nope", http.StatusBadRequest, nil)

	var detail itemDetailJSON
	e.getJSON(c, "/api/items/67589", http.StatusOK, &detail)
	if len(detail.Versions) != 2 || len(detail.Events) != 1 {
		t.Fatalf("detail: %d versions, %d events", len(detail.Versions), len(detail.Events))
	}
	var from, to versionJSON
	e.getJSON(c, "/api/items/67589/versions/"+itoa(*ev.FromVersionID), http.StatusOK, &from)
	e.getJSON(c, "/api/items/67589/versions/"+itoa(*ev.ToVersionID), http.StatusOK, &to)
	if from.Subheader != "Deadline: Sunday, November 1" || to.Subheader != "Deadline: Monday, November 2" {
		t.Errorf("versions: %q -> %q", from.Subheader, to.Subheader)
	}
	e.getJSON(c, "/api/items/1/versions/"+itoa(to.ID), http.StatusNotFound, nil)
	e.getJSON(c, "/api/items/999", http.StatusNotFound, nil)

	var tl eventPageJSON
	e.getJSON(c, "/api/events?limit=1", http.StatusOK, &tl)
	if len(tl.Events) != 1 || tl.Events[0].Title != "2027-28 Tuition Assistance Application" || tl.NextBefore == nil {
		t.Fatalf("timeline = %+v", tl)
	}

	res := e.do(c, http.MethodPost, "/api/events/read", `{"ids": [`+itoa(ev.ID)+`]}`)
	if res.StatusCode != http.StatusNoContent {
		t.Fatalf("mark read: %d", res.StatusCode)
	}
	e.getJSON(c, "/api/items?signup=1", http.StatusOK, &list)
	if list.LastReadAt == nil {
		t.Error("lastReadAt not set after reading")
	}
	for _, it := range list.Items {
		if !it.IsSignup {
			t.Errorf("signup filter let through %d", it.PanelID)
		}
		if it.LatestEvent != nil && it.LatestEvent.Unread {
			t.Errorf("%d still unread", it.PanelID)
		}
	}

	var status statusJSON
	e.getJSON(c, "/api/status", http.StatusOK, &status)
	if status.CheckedAt == nil || status.OKAt == nil || status.Failing {
		t.Errorf("status = %+v", status)
	}
}

func TestEventsBeforeAccountAreRead(t *testing.T) {
	e := setup(t)
	raw, _ := os.ReadFile("../bbb/testdata/bbb-sample.html")
	page := string(raw)
	record(t, e.store, page, time.Now().Add(-2*time.Hour))
	record(t, e.store, strings.Replace(page, "Deadline: Sunday, November 1", "Deadline: Monday, November 2", 1),
		time.Now().Add(-time.Hour))

	c := e.browser()
	e.signIn(c, "invite="+e.invite(), parent("late"))
	var list itemListJSON
	e.getJSON(c, "/api/items", http.StatusOK, &list)
	for _, it := range list.Items {
		if it.LatestEvent != nil && it.LatestEvent.Unread {
			t.Errorf("%d unread for a parent who joined after the edit", it.PanelID)
		}
	}
}

func itoa(n int64) string {
	b, _ := json.Marshal(n)
	return string(b)
}
