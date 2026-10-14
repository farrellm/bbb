package handler

import (
	"net/http"
	"slices"

	"github.com/farrellm/bbb/internal/store"
)

var divisions = []string{"all", "lower", "middle", "upper"}

// listItems serves the feed. Query parameters: division (one of all,
// lower, middle, upper), signup=1 for signup items only, include_removed=1.
func (a *API) listItems(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	f := store.ItemFilter{
		Division:       q.Get("division"),
		SignupsOnly:    queryBool(r, "signup"),
		IncludeRemoved: queryBool(r, "include_removed"),
	}
	if f.Division != "" && !slices.Contains(divisions, f.Division) {
		writeError(w, http.StatusBadRequest, "Unknown division")
		return
	}
	user := currentUser(r)
	items, err := a.Store.ListItems(r.Context(), user.ID, f)
	if err != nil {
		internalError(w, r, "Could not load items", err)
		return
	}
	lastRead, err := a.Store.LastReadAt(r.Context(), user.ID)
	if err != nil {
		internalError(w, r, "Could not load items", err)
		return
	}
	out := itemListJSON{Items: make([]itemJSON, len(items)), LastReadAt: lastRead}
	for i, it := range items {
		out.Items[i] = a.toItem(it)
	}
	writeJSON(w, http.StatusOK, out)
}

// getItem returns an item with its version list and event history.
func (a *API) getItem(w http.ResponseWriter, r *http.Request) {
	panelID, ok := pathID(r, "panelID")
	if !ok {
		writeError(w, http.StatusNotFound, "No such item")
		return
	}
	user := currentUser(r)
	it, err := a.Store.GetItem(r.Context(), user.ID, panelID)
	if err != nil {
		notFoundOr500(w, r, err, "No such item", "Could not load item")
		return
	}
	versions, err := a.Store.ListVersions(r.Context(), panelID)
	if err != nil {
		internalError(w, r, "Could not load item", err)
		return
	}
	events, err := a.Store.ListEvents(r.Context(), user.ID, store.EventFilter{PanelID: panelID, Limit: 500})
	if err != nil {
		internalError(w, r, "Could not load item", err)
		return
	}
	out := itemDetailJSON{
		Item:     a.toItem(it),
		Versions: make([]versionMetaJSON, len(versions)),
		Events:   toTimelineEvents(events),
	}
	for i, v := range versions {
		out.Versions[i] = versionMetaJSON{ID: v.ID, SeenAt: v.SeenAt, Title: v.Title}
	}
	writeJSON(w, http.StatusOK, out)
}

// getVersion returns the full text of one version, for diffs.
func (a *API) getVersion(w http.ResponseWriter, r *http.Request) {
	panelID, ok1 := pathID(r, "panelID")
	versionID, ok2 := pathID(r, "versionID")
	if !ok1 || !ok2 {
		writeError(w, http.StatusNotFound, "No such version")
		return
	}
	v, err := a.Store.GetVersion(r.Context(), panelID, versionID)
	if err != nil {
		notFoundOr500(w, r, err, "No such version", "Could not load version")
		return
	}
	writeJSON(w, http.StatusOK, versionJSON{
		ID:          v.ID,
		PanelID:     v.PanelID,
		SeenAt:      v.SeenAt,
		Title:       v.Title,
		Subheader:   v.Subheader,
		BodyText:    v.BodyText,
		BodyHTML:    a.cleaner.Clean(v.BodyHTML),
		SignupLinks: a.cleaner.signupLinks(v.BodyHTML, v.SignupLinks),
	})
}

func (a *API) toItem(it store.FeedItem) itemJSON {
	out := itemJSON{
		PanelID:      it.PanelID,
		Division:     it.Division,
		Position:     it.Position,
		VersionID:    it.VersionID,
		VersionCount: it.VersionCount,
		Title:        it.Title,
		Subheader:    it.Subheader,
		BodyHTML:     a.cleaner.Clean(it.BodyHTML),
		IsSignup:     it.IsSignup,
		SignupLinks:  a.cleaner.signupLinks(it.BodyHTML, it.SignupLinks),
		FirstSeenAt:  it.FirstSeenAt,
		LastSeenAt:   it.LastSeenAt,
		RemovedAt:    it.RemovedAt,
	}
	if it.Deadline != nil {
		d := it.Deadline.Format("2006-01-02")
		out.Deadline = &d
	}
	if it.LatestEvent != nil {
		e := toEvent(*it.LatestEvent)
		out.LatestEvent = &e
	}
	return out
}
