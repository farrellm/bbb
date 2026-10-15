package handler

import (
	"net/http"
	"net/url"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/farrellm/bbb/internal/auth"
	"github.com/farrellm/bbb/internal/store"
)

// Invite defaults: one parent, two weeks.
const (
	DefaultInviteUses = 1
	DefaultInviteDays = 14
)

// InviteURL is the join link for an invite code.
func InviteURL(publicURL, code string) string {
	return publicURL + "/join?invite=" + url.QueryEscape(code)
}

// createInvite makes an invite: {"maxUses": 1, "expiresInDays": 14}, both
// optional. expiresInDays 0 means it never expires.
func (a *API) createInvite(w http.ResponseWriter, r *http.Request) {
	body := struct {
		MaxUses       *int `json:"maxUses"`
		ExpiresInDays *int `json:"expiresInDays"`
	}{}
	if err := decodeJSON(w, r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body")
		return
	}
	uses, days := DefaultInviteUses, DefaultInviteDays
	if body.MaxUses != nil {
		uses = *body.MaxUses
	}
	if body.ExpiresInDays != nil {
		days = *body.ExpiresInDays
	}
	if uses < 1 || uses > 100 || days < 0 || days > 365 {
		writeError(w, http.StatusBadRequest, "maxUses must be 1-100 and expiresInDays 0-365")
		return
	}
	var expires *time.Time
	if days > 0 {
		t := time.Now().Add(time.Duration(days) * 24 * time.Hour)
		expires = &t
	}
	inv, err := a.Store.CreateInvite(r.Context(), auth.RandomToken(), currentUser(r).ID, uses, expires)
	if err != nil {
		internalError(w, r, "Could not create invite", err)
		return
	}
	writeJSON(w, http.StatusCreated, a.inviteJSON(inv))
}

// listInvites shows the invite links that still work, newest first.
func (a *API) listInvites(w http.ResponseWriter, r *http.Request) {
	invs, err := a.Store.OpenInvites(r.Context())
	if err != nil {
		internalError(w, r, "Could not load invites", err)
		return
	}
	out := make([]inviteJSON, len(invs))
	for i, inv := range invs {
		out[i] = a.inviteJSON(inv)
	}
	writeJSON(w, http.StatusOK, out)
}

// revokeInvite expires an invite link at once.
func (a *API) revokeInvite(w http.ResponseWriter, r *http.Request) {
	if err := a.Store.RevokeInvite(r.Context(), chi.URLParam(r, "code")); err != nil {
		notFoundOr500(w, r, err, "No such invite", "Could not expire invite")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *API) inviteJSON(inv store.Invite) inviteJSON {
	return inviteJSON{
		Code: inv.Code, URL: InviteURL(a.PublicURL, inv.Code), MaxUses: inv.MaxUses, Uses: inv.Uses,
		ExpiresAt: inv.ExpiresAt, CreatedAt: inv.CreatedAt,
	}
}

// listRuns shows recent polls: ?limit= (default 50, at most 500).
func (a *API) listRuns(w http.ResponseWriter, r *http.Request) {
	limit, ok := queryInt(r, "limit", 50, 1, 500)
	if !ok {
		writeError(w, http.StatusBadRequest, "Invalid limit")
		return
	}
	runs, err := a.Store.ListRuns(r.Context(), int(limit))
	if err != nil {
		internalError(w, r, "Could not load runs", err)
		return
	}
	out := make([]scrapeRunJSON, len(runs))
	for i, run := range runs {
		out[i] = scrapeRunJSON(run)
	}
	writeJSON(w, http.StatusOK, out)
}
