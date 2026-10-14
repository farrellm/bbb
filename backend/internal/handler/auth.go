package handler

import (
	"errors"
	"log/slog"
	"net/http"
	"net/url"
	"slices"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/farrellm/bbb/internal/auth"
	"github.com/farrellm/bbb/internal/store"
)

// maxTokenLen bounds invite codes and handoff nonces read from requests.
const maxTokenLen = 64

// googleStart begins sign-in: it records the flow in a cookie and sends the
// browser to Google. Optional query parameters: invite (a code, for new
// accounts), handoff (an iOS home-screen app's nonce) and next (an app path
// to land on).
func (a *API) googleStart(w http.ResponseWriter, r *http.Request) {
	if a.Google == nil {
		writeError(w, http.StatusServiceUnavailable, "Google sign-in is not configured")
		return
	}
	q := r.URL.Query()
	fl := auth.Flow{
		State:    auth.RandomToken(),
		Verifier: auth.RandomToken() + auth.RandomToken(), // PKCE wants 43+ characters
		Nonce:    auth.RandomToken(),
		Invite:   token(q.Get("invite")),
		Handoff:  token(q.Get("handoff")),
		Next:     safeNext(q.Get("next")),
	}
	if err := a.Flows.Start(w, fl); err != nil {
		internalError(w, r, "Could not start sign-in", err)
		return
	}
	// A handoff hands a session to whoever started it, so make the user
	// pick the account on purpose rather than letting Google skip the
	// prompt.
	http.Redirect(w, r, a.Google.AuthCodeURL(fl, fl.Handoff != ""), http.StatusFound)
}

// googleCallback finishes sign-in and redirects back into the app. Failures
// land on /login or /join with an error code the page explains.
func (a *API) googleCallback(w http.ResponseWriter, r *http.Request) {
	if a.Google == nil {
		writeError(w, http.StatusServiceUnavailable, "Google sign-in is not configured")
		return
	}
	fail := func(page, code string, err error) {
		if err != nil {
			slog.Warn("sign-in failed", "reason", code, "err", err)
		}
		http.Redirect(w, r, page+"?error="+code, http.StatusFound)
	}

	fl, err := a.Flows.Finish(w, r)
	if err != nil {
		fail("/login", "expired", err)
		return
	}
	q := r.URL.Query()
	if q.Get("state") != fl.State {
		fail("/login", "expired", errors.New("state mismatch"))
		return
	}
	if q.Get("error") != "" {
		// access_denied when the user backs out on Google's page.
		fail("/login", "cancelled", nil)
		return
	}

	id, err := a.Google.Exchange(r.Context(), q.Get("code"), fl)
	if errors.Is(err, auth.ErrEmailUnverified) {
		fail("/login", "email_unverified", err)
		return
	}
	if err != nil {
		fail("/login", "failed", err)
		return
	}

	u, err := a.Store.SignInUser(r.Context(), store.SignIn{
		Subject: id.Subject,
		Email:   id.Email,
		Name:    id.Name,
		Picture: id.Picture,
		Invite:  fl.Invite,
		Admin:   slices.Contains(a.AdminEmails, strings.ToLower(id.Email)),
	})
	switch {
	case errors.Is(err, store.ErrInviteRequired):
		fail("/join", "invite_required", nil)
		return
	case errors.Is(err, store.ErrInviteInvalid):
		fail("/join", "invite_invalid", nil)
		return
	case err != nil:
		fail("/login", "failed", err)
		return
	}

	if err := a.Sessions.Save(w, r, u.ID); err != nil {
		fail("/login", "failed", err)
		return
	}
	if fl.Handoff != "" {
		ok, err := a.Store.CompleteHandoff(r.Context(), fl.Handoff, u.ID)
		if err != nil || !ok {
			fail("/login", "handoff_expired", err)
			return
		}
		http.Redirect(w, r, "/signed-in", http.StatusFound)
		return
	}
	http.Redirect(w, r, fl.Next, http.StatusFound)
}

// createHandoff starts an iOS home-screen sign-in: the app gets a nonce,
// opens sign-in with it outside the app, and polls claimHandoff.
func (a *API) createHandoff(w http.ResponseWriter, r *http.Request) {
	nonce := auth.RandomToken()
	if err := a.Store.CreateHandoff(r.Context(), nonce); err != nil {
		internalError(w, r, "Could not start sign-in", err)
		return
	}
	writeJSON(w, http.StatusOK, struct {
		Nonce string `json:"nonce"`
	}{nonce})
}

// claimHandoff answers 202 while sign-in is still in progress, then once
// with the user and a session cookie, then 404.
func (a *API) claimHandoff(w http.ResponseWriter, r *http.Request) {
	nonce := token(chi.URLParam(r, "nonce"))
	userID, state, err := a.Store.ClaimHandoff(r.Context(), nonce)
	if err != nil {
		internalError(w, r, "Could not check sign-in", err)
		return
	}
	switch state {
	case store.HandoffPending:
		writeJSON(w, http.StatusAccepted, struct {
			Status string `json:"status"`
		}{"pending"})
	case store.HandoffMissing:
		writeError(w, http.StatusNotFound, "Sign-in expired")
	case store.HandoffDone:
		u, err := a.Store.GetUser(r.Context(), userID)
		if err != nil {
			internalError(w, r, "Could not load account", err)
			return
		}
		if err := a.Sessions.Save(w, r, u.ID); err != nil {
			internalError(w, r, "Could not start session", err)
			return
		}
		writeJSON(w, http.StatusOK, toUser(u))
	}
}

// checkInvite lets the join page say up front that a link is stale.
func (a *API) checkInvite(w http.ResponseWriter, r *http.Request) {
	ok, err := a.Store.InviteUsable(r.Context(), token(chi.URLParam(r, "code")))
	if err != nil {
		internalError(w, r, "Could not check invite", err)
		return
	}
	writeJSON(w, http.StatusOK, struct {
		Usable bool `json:"usable"`
	}{ok})
}

func (a *API) logout(w http.ResponseWriter, r *http.Request) {
	if err := a.Sessions.Clear(w, r); err != nil {
		internalError(w, r, "Could not sign out", err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// me returns the signed-in user and re-issues the session cookie, which is
// what makes sessions roll: the app calls it on every launch.
func (a *API) me(w http.ResponseWriter, r *http.Request) {
	u, err := a.Store.GetUser(r.Context(), currentUser(r).ID)
	if err != nil {
		internalError(w, r, "Could not load account", err)
		return
	}
	if err := a.Sessions.Save(w, r, u.ID); err != nil {
		internalError(w, r, "Could not refresh session", err)
		return
	}
	writeJSON(w, http.StatusOK, toUser(u))
}

// token returns s if it looks like one of our random tokens, else "".
func token(s string) string {
	if len(s) > maxTokenLen {
		return ""
	}
	for _, c := range s {
		if (c < 'a' || c > 'z') && (c < 'A' || c > 'Z') && (c < '0' || c > '9') && c != '-' && c != '_' {
			return ""
		}
	}
	return s
}

// safeNext keeps a post-sign-in path only if it stays inside the app.
func safeNext(next string) string {
	u, err := url.Parse(next)
	if err != nil || next == "" || u.IsAbs() || u.Host != "" || !strings.HasPrefix(u.Path, "/") ||
		strings.HasPrefix(next, "//") || strings.Contains(next, `\`) {
		return "/"
	}
	return next
}
