package handler

import (
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5/middleware"

	"github.com/farrellm/bbb/internal/auth"
	"github.com/farrellm/bbb/internal/store"
)

// requireUser rejects requests without a valid session, or whose account
// has been deleted, with a 401. Otherwise it puts the user in the context
// for handlers to read with auth.UserFrom.
func (a *API) requireUser(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id, ok := a.Sessions.UserID(r)
		if !ok {
			writeError(w, http.StatusUnauthorized, "Not signed in")
			return
		}
		u, err := a.Store.GetUser(r.Context(), id)
		if errors.Is(err, store.ErrNotFound) {
			_ = a.Sessions.Clear(w, r)
			writeError(w, http.StatusUnauthorized, "Not signed in")
			return
		}
		if err != nil {
			internalError(w, r, "Could not check session", err)
			return
		}
		next.ServeHTTP(w, r.WithContext(auth.WithUser(r.Context(), auth.User{ID: u.ID, IsAdmin: u.IsAdmin})))
	})
}

// requireAdmin must run after requireUser.
func requireAdmin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if u, _ := auth.UserFrom(r.Context()); !u.IsAdmin {
			writeError(w, http.StatusForbidden, "Admins only")
			return
		}
		next.ServeHTTP(w, r)
	})
}

// currentUser returns the user requireUser stored.
func currentUser(r *http.Request) auth.User {
	u, _ := auth.UserFrom(r.Context())
	return u
}

// logRequests logs one line per request. It logs the path without the
// query string, which can carry invite codes, handoff nonces and OAuth
// codes.
func logRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
		next.ServeHTTP(ww, r)
		slog.Info("http", "method", r.Method, "path", r.URL.Path, "status", ww.Status(),
			"dur", time.Since(start).Round(time.Millisecond), "ip", middleware.GetClientIP(r.Context()))
	})
}

// securityHeaders sets headers every response gets. The CSP allows inline
// styles because MUI (Emotion) injects them; scripts are same-origin only.
// Item bodies may show images hosted on the school's site.
func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("Referrer-Policy", "same-origin")
		h.Set("Content-Security-Policy", "default-src 'self'; img-src 'self' https: data:; "+
			"style-src 'self' 'unsafe-inline'; frame-ancestors 'none'; base-uri 'self'; form-action 'self'")
		next.ServeHTTP(w, r)
	})
}
