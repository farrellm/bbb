package handler

import (
	"encoding/base64"
	"net/http"
	"slices"

	"github.com/farrellm/bbb/internal/notify"
	"github.com/farrellm/bbb/internal/store"
)

const (
	maxEndpoint  = 2048
	maxUserAgent = 300
	// maxMinEditWords bounds the edit-size preference; anything bigger
	// would mean "never", which notify_edit already says.
	maxMinEditWords = 1000
)

// vapidKey gives the app the server's public key, which a browser needs
// to subscribe.
func (a *API) vapidKey(w http.ResponseWriter, _ *http.Request) {
	if !a.pushEnabled() {
		writeError(w, http.StatusServiceUnavailable, "Notifications aren't set up on this server")
		return
	}
	writeJSON(w, http.StatusOK, vapidKeyJSON{Key: a.Notifier.Sender.PublicKey})
}

func (a *API) pushEnabled() bool {
	return a.Notifier != nil && a.Notifier.Enabled()
}

// subscribe saves this device's push subscription for the signed-in user:
// the endpoint and keys from PushSubscription.toJSON().
func (a *API) subscribe(w http.ResponseWriter, r *http.Request) {
	var body subscriptionInJSON
	if err := decodeJSON(w, r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body")
		return
	}
	if len(body.Endpoint) > maxEndpoint || !notify.AllowedEndpoint(body.Endpoint) {
		writeError(w, http.StatusBadRequest, "Unsupported push service")
		return
	}
	if keyLen(body.Keys.P256dh) != 65 || keyLen(body.Keys.Auth) != 16 {
		writeError(w, http.StatusBadRequest, "Invalid subscription keys")
		return
	}
	ua := r.UserAgent()
	if len(ua) > maxUserAgent {
		ua = ua[:maxUserAgent]
	}
	sub, err := a.Store.SaveSubscription(r.Context(), store.Subscription{
		UserID: currentUser(r).ID, Endpoint: body.Endpoint,
		P256dh: body.Keys.P256dh, Auth: body.Keys.Auth, UserAgent: ua,
	})
	if err != nil {
		internalError(w, r, "Could not save subscription", err)
		return
	}
	writeJSON(w, http.StatusCreated, toDevice(sub))
}

// keyLen is the decoded length of a base64url subscription key, or -1.
func keyLen(s string) int {
	b, err := base64.RawURLEncoding.DecodeString(s)
	if err != nil {
		if b, err = base64.URLEncoding.DecodeString(s); err != nil {
			return -1
		}
	}
	return len(b)
}

// unsubscribe forgets this device's subscription: {"endpoint": ...}.
func (a *API) unsubscribe(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Endpoint string `json:"endpoint"`
	}
	if err := decodeJSON(w, r, &body); err != nil || body.Endpoint == "" {
		writeError(w, http.StatusBadRequest, "Invalid request body")
		return
	}
	if err := a.Store.DeleteSubscriptionByEndpoint(r.Context(), currentUser(r).ID, body.Endpoint); err != nil {
		internalError(w, r, "Could not remove subscription", err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// listDevices lists the signed-in user's subscribed devices.
func (a *API) listDevices(w http.ResponseWriter, r *http.Request) {
	subs, err := a.Store.ListSubscriptions(r.Context(), currentUser(r).ID)
	if err != nil {
		internalError(w, r, "Could not load devices", err)
		return
	}
	out := make([]deviceJSON, len(subs))
	for i, s := range subs {
		out[i] = toDevice(s)
	}
	writeJSON(w, http.StatusOK, out)
}

// deleteDevice stops notifications to one of the user's devices.
func (a *API) deleteDevice(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r, "id")
	if !ok {
		writeError(w, http.StatusNotFound, "No such device")
		return
	}
	if err := a.Store.DeleteSubscription(r.Context(), currentUser(r).ID, id); err != nil {
		notFoundOr500(w, r, err, "No such device", "Could not remove device")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// testPush sends a test notification to each of the user's devices.
func (a *API) testPush(w http.ResponseWriter, r *http.Request) {
	if !a.pushEnabled() {
		writeError(w, http.StatusServiceUnavailable, "Notifications aren't set up on this server")
		return
	}
	res, err := a.Notifier.Test(r.Context(), currentUser(r).ID)
	if err != nil {
		internalError(w, r, "Could not send test", err)
		return
	}
	writeJSON(w, http.StatusOK, testPushJSON{Sent: res.Sent, Failed: res.Failed})
}

func (a *API) getPreferences(w http.ResponseWriter, r *http.Request) {
	p, err := a.Store.GetPreferences(r.Context(), currentUser(r).ID)
	if err != nil {
		internalError(w, r, "Could not load preferences", err)
		return
	}
	writeJSON(w, http.StatusOK, preferencesJSON(p))
}

// putPreferences replaces the user's preferences and returns them as
// saved.
func (a *API) putPreferences(w http.ResponseWriter, r *http.Request) {
	var body preferencesJSON
	if err := decodeJSON(w, r, &body); err != nil || body.Divisions == nil {
		writeError(w, http.StatusBadRequest, "Invalid request body")
		return
	}
	if body.MinEditWords < 0 || body.MinEditWords > maxMinEditWords {
		writeError(w, http.StatusBadRequest, "minEditWords must be 0-1000")
		return
	}
	// Keep the page's order and drop repeats.
	divs := make([]string, 0, len(divisions))
	for _, d := range body.Divisions {
		if !slices.Contains(divisions, d) {
			writeError(w, http.StatusBadRequest, "Unknown division")
			return
		}
	}
	for _, d := range divisions {
		if slices.Contains(body.Divisions, d) {
			divs = append(divs, d)
		}
	}
	body.Divisions = divs

	p := store.Preferences(body)
	if err := a.Store.SetPreferences(r.Context(), currentUser(r).ID, p); err != nil {
		internalError(w, r, "Could not save preferences", err)
		return
	}
	writeJSON(w, http.StatusOK, body)
}
