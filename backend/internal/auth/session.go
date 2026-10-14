// Package auth handles sign-in: the signed session cookie, the short-lived
// cookie that carries an OAuth flow from start to callback, and the Google
// OpenID Connect client.
package auth

import (
	"net/http"
	"time"

	"github.com/gorilla/sessions"
)

const (
	sessionName = "bbb_session"
	userIDKey   = "uid"
	// SessionMaxAge is long because the iOS home-screen app has its own
	// cookie store and signing in again there means the handoff dance.
	// Sessions roll: every visit to /api/me re-issues the cookie.
	SessionMaxAge = 365 * 24 * time.Hour
)

// Sessions wraps a cookie-backed session store with helpers for the user
// id. Only the id is stored; the cookie is signed, not encrypted, and
// HttpOnly so page scripts can't read or forge it.
type Sessions struct {
	store *sessions.CookieStore
}

// NewSessions builds a session manager. secure sets the cookie's Secure
// flag; pass true whenever the app is served over https.
func NewSessions(secret string, secure bool) *Sessions {
	store := sessions.NewCookieStore([]byte(secret))
	store.Options = &sessions.Options{
		Path:     "/",
		MaxAge:   int(SessionMaxAge.Seconds()),
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteLaxMode,
	}
	// Also makes the signed timestamp expire with the cookie, so an old
	// cookie value replayed after a year is rejected.
	store.MaxAge(store.Options.MaxAge)
	return &Sessions{store: store}
}

// Save writes the user id into the session cookie.
func (s *Sessions) Save(w http.ResponseWriter, r *http.Request, userID int64) error {
	sess, _ := s.store.Get(r, sessionName)
	sess.Values[userIDKey] = userID
	return sess.Save(r, w)
}

// UserID returns the signed-in user id, or false if there is no valid
// session.
func (s *Sessions) UserID(r *http.Request) (int64, bool) {
	sess, err := s.store.Get(r, sessionName)
	if err != nil {
		return 0, false
	}
	id, ok := sess.Values[userIDKey].(int64)
	return id, ok
}

// Clear expires the session cookie.
func (s *Sessions) Clear(w http.ResponseWriter, r *http.Request) error {
	sess, _ := s.store.Get(r, sessionName)
	sess.Options.MaxAge = -1
	delete(sess.Values, userIDKey)
	return sess.Save(r, w)
}
