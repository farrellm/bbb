package auth

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"net/http"
	"time"

	"github.com/gorilla/securecookie"
)

const (
	flowCookie = "bbb_oauth"
	flowPath   = "/api/auth/google"
	// FlowTTL bounds how long the user can spend on Google's pages.
	FlowTTL = 10 * time.Minute
)

// Flow is what one sign-in attempt carries from /start to /callback: the
// CSRF state, the PKCE verifier and the ID token nonce, plus the app's own
// context (invite code, iOS handoff nonce, where to land afterwards).
type Flow struct {
	State    string
	Verifier string
	Nonce    string
	Invite   string
	Handoff  string
	Next     string
}

// Flows stores a Flow in a signed, HttpOnly cookie scoped to the Google
// auth routes.
type Flows struct {
	codec  *securecookie.SecureCookie
	secure bool
}

// NewFlows builds a flow cookie codec. The secret is shared with Sessions;
// securecookie binds each value to its cookie name, so one can't stand in
// for the other.
func NewFlows(secret string, secure bool) *Flows {
	codec := securecookie.New([]byte(secret), nil)
	codec.MaxAge(int(FlowTTL.Seconds()))
	return &Flows{codec: codec, secure: secure}
}

// Start writes the flow cookie.
func (f *Flows) Start(w http.ResponseWriter, fl Flow) error {
	v, err := f.codec.Encode(flowCookie, fl)
	if err != nil {
		return err
	}
	http.SetCookie(w, f.cookie(v, int(FlowTTL.Seconds())))
	return nil
}

// Finish reads the flow cookie and deletes it, so each flow is used once.
func (f *Flows) Finish(w http.ResponseWriter, r *http.Request) (Flow, error) {
	var fl Flow
	c, err := r.Cookie(flowCookie)
	if err != nil {
		return fl, errors.New("no sign-in in progress")
	}
	http.SetCookie(w, f.cookie("", -1))
	if err := f.codec.Decode(flowCookie, c.Value, &fl); err != nil {
		return fl, err
	}
	return fl, nil
}

func (f *Flows) cookie(value string, maxAge int) *http.Cookie {
	return &http.Cookie{ //nolint:gosec // G124: Secure is off only for http://localhost
		Name:     flowCookie,
		Value:    value,
		Path:     flowPath,
		MaxAge:   maxAge,
		HttpOnly: true,
		Secure:   f.secure,
		// Lax still sends it on the top-level GET redirect back from Google.
		SameSite: http.SameSiteLaxMode,
	}
}

// RandomToken returns 128 random bits, base64url encoded.
func RandomToken() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b) // crypto/rand.Read never fails
	return base64.RawURLEncoding.EncodeToString(b)
}
