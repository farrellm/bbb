package handler

import (
	"net/http"

	"github.com/farrellm/bbb/internal/store"
)

// maxReadIDs caps one mark-read request.
const maxReadIDs = 1000

// listEvents serves the timeline, newest first. Query parameters: before
// (an event id, to page back from) and limit (default 50, at most 200).
func (a *API) listEvents(w http.ResponseWriter, r *http.Request) {
	before, ok1 := queryInt(r, "before", 0, 0, 1<<62)
	limit, ok2 := queryInt(r, "limit", 50, 1, 200)
	if !ok1 || !ok2 {
		writeError(w, http.StatusBadRequest, "Invalid before or limit")
		return
	}
	evs, err := a.Store.ListEvents(r.Context(), currentUser(r).ID,
		store.EventFilter{Before: before, Limit: int(limit)})
	if err != nil {
		internalError(w, r, "Could not load events", err)
		return
	}
	out := eventPageJSON{Events: toTimelineEvents(evs)}
	if len(evs) == int(limit) {
		next := evs[len(evs)-1].ID
		out.NextBefore = &next
	}
	writeJSON(w, http.StatusOK, out)
}

// markRead records events as read: {"ids": [...]}.
func (a *API) markRead(w http.ResponseWriter, r *http.Request) {
	var body struct {
		IDs []int64 `json:"ids"`
	}
	if err := decodeJSON(w, r, &body); err != nil || len(body.IDs) > maxReadIDs {
		writeError(w, http.StatusBadRequest, "Invalid request body")
		return
	}
	if len(body.IDs) > 0 {
		if err := a.Store.MarkRead(r.Context(), currentUser(r).ID, body.IDs); err != nil {
			internalError(w, r, "Could not mark read", err)
			return
		}
	}
	w.WriteHeader(http.StatusNoContent)
}
