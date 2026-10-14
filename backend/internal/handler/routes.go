// Package handler is the HTTP API under /api, plus the built frontend.
package handler

import (
	"context"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/httprate"

	"github.com/farrellm/bbb/internal/auth"
	"github.com/farrellm/bbb/internal/notify"
	"github.com/farrellm/bbb/internal/store"
)

// SignInProvider runs the OpenID Connect code flow. *auth.Google is the
// real one.
type SignInProvider interface {
	AuthCodeURL(fl auth.Flow, selectAccount bool) string
	Exchange(ctx context.Context, code string, fl auth.Flow) (auth.Identity, error)
}

// API holds what the handlers need.
type API struct {
	Store    *store.Store
	Sessions *auth.Sessions
	Flows    *auth.Flows
	// Google is nil when sign-in is not configured; the auth routes then
	// answer 503.
	Google SignInProvider
	// Notifier sends test pushes. When it is nil or has no sender, the
	// push routes that need keys answer 503.
	Notifier *notify.Notifier
	// PublicURL is the app's origin, for invite links.
	PublicURL string
	// AdminEmails are lowercased.
	AdminEmails []string
	// PageURL is the BBB page, for resolving relative links in item bodies.
	PageURL string
	// WebDir is the built frontend, served at "/" with an SPA fallback.
	// Empty leaves the tree API-only, as in development, where Vite serves
	// the app and proxies /api here.
	WebDir string

	cleaner *bodyCleaner
}

// Routes builds the handler tree.
func (a *API) Routes() http.Handler {
	a.cleaner = newBodyCleaner(a.PageURL)

	r := chi.NewRouter()
	// Production binds loopback and is reached only through Tailscale
	// Funnel, which proxies from loopback and sets X-Forwarded-For.
	r.Use(middleware.ClientIPFromXFF("127.0.0.1/32", "::1/128"))
	r.Use(logRequests)
	r.Use(middleware.Recoverer)
	r.Use(securityHeaders)

	r.Route("/api", func(r chi.Router) {
		// Its own 404, so an unknown /api path never falls through to the
		// SPA and answers a fetch with index.html.
		r.NotFound(func(w http.ResponseWriter, _ *http.Request) {
			writeError(w, http.StatusNotFound, "Not found")
		})
		r.Use(middleware.NoCache)

		r.Get("/healthz", a.health)
		r.Get("/push/vapid-public-key", a.vapidKey)

		r.Route("/auth", func(r chi.Router) {
			// These are reachable from the public internet without a
			// session.
			r.Group(func(r chi.Router) {
				r.Use(limitByClient(30))
				r.Get("/google/start", a.googleStart)
				r.Get("/google/callback", a.googleCallback)
				r.Post("/handoff", a.createHandoff)
				r.Get("/invites/{code}", a.checkInvite)
			})
			// The app polls this every couple of seconds while the user
			// signs in elsewhere.
			r.With(limitByClient(120)).Get("/handoff/{nonce}", a.claimHandoff)
			r.Post("/logout", a.logout)
		})

		r.Group(func(r chi.Router) {
			r.Use(a.requireUser)
			r.Get("/me", a.me)
			r.Get("/status", a.status)
			r.Get("/items", a.listItems)
			r.Get("/items/{panelID}", a.getItem)
			r.Get("/items/{panelID}/versions/{versionID}", a.getVersion)
			r.Get("/events", a.listEvents)
			r.Post("/events/read", a.markRead)
			r.Get("/preferences", a.getPreferences)
			r.Put("/preferences", a.putPreferences)

			r.Route("/push", func(r chi.Router) {
				r.Post("/subscribe", a.subscribe)
				r.Delete("/subscribe", a.unsubscribe)
				r.Get("/devices", a.listDevices)
				r.Delete("/devices/{id}", a.deleteDevice)
				// Each test pushes to every device the user has.
				r.With(limitByClient(10)).Post("/test", a.testPush)
			})

			r.Route("/admin", func(r chi.Router) {
				r.Use(requireAdmin)
				r.Post("/invites", a.createInvite)
				r.Get("/scrape-runs", a.listRuns)
			})
		})
	})

	if a.WebDir != "" {
		r.Handle("/*", SPA(a.WebDir))
	}
	return r
}

// limitByClient allows n requests a minute per client address. Funnel
// traffic can share a source address, so limits are generous.
func limitByClient(n int) func(http.Handler) http.Handler {
	return httprate.LimitBy(n, time.Minute, func(r *http.Request) (string, error) {
		ip := middleware.GetClientIP(r.Context())
		if ip == "" {
			// Local requests with no proxy in front, e.g. through Vite.
			return "local", nil
		}
		return httprate.CanonicalizeIP(ip), nil
	})
}

func (a *API) health(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()
	if err := a.Store.Ping(ctx); err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"status": "db unavailable"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (a *API) status(w http.ResponseWriter, r *http.Request) {
	st, err := a.Store.Status(r.Context())
	if err != nil {
		internalError(w, r, "Could not load status", err)
		return
	}
	unread, err := a.Store.UnreadCount(r.Context(), currentUser(r).ID)
	if err != nil {
		internalError(w, r, "Could not load status", err)
		return
	}
	writeJSON(w, http.StatusOK, statusJSON{CheckedAt: st.CheckedAt, OKAt: st.OKAt, Failing: st.Failing, Unread: unread})
}
