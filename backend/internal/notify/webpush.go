// Package notify sends Web Push notifications: it turns committed events
// into messages for each user whose preferences match, and pushes them to
// every device the user has subscribed.
package notify

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/SherClockHolmes/webpush-go"

	"github.com/farrellm/bbb/internal/store"
)

// Payload is the JSON a notification carries to the service worker, which
// shows it. backend/api/openapi.yaml describes it as PushPayload.
type Payload struct {
	Title string `json:"title"`
	Body  string `json:"body"`
	// URL is the app path a tap opens.
	URL string `json:"url"`
	// Tag replaces an earlier notification with the same tag instead of
	// stacking, so a later edit to an item supersedes the first.
	Tag string `json:"tag"`
	// Badge is the user's unread count, for the app icon.
	Badge int `json:"badge"`
}

// Outcome classes for one push attempt.
type result int

const (
	sent      result = iota
	gone             // 404 or 410: the subscription is dead; delete it
	retryable        // 429 or 5xx, or a network error
	failed           // anything else; retrying won't help
)

// pushHosts are the push services browsers subscribe through. The server
// POSTs to whatever endpoint a subscription names, so endpoints elsewhere
// are refused rather than letting a user point it at arbitrary hosts.
var pushHosts = []string{
	"push.apple.com",            // Safari, including iOS home-screen apps
	"fcm.googleapis.com",        // Chrome; Edge on Android
	"push.services.mozilla.com", // Firefox
	"notify.windows.com",        // Edge on Windows
}

// AllowedEndpoint reports whether a subscription endpoint is an https URL
// on a known push service.
func AllowedEndpoint(endpoint string) bool {
	u, err := url.Parse(endpoint)
	if err != nil || u.Scheme != "https" || u.User != nil || u.Port() != "" {
		return false
	}
	host := strings.ToLower(u.Hostname())
	for _, h := range pushHosts {
		if host == h || strings.HasSuffix(host, "."+h) {
			return true
		}
	}
	return false
}

// Sender pushes to one subscription at a time with the server's VAPID
// keys.
type Sender struct {
	PublicKey  string
	PrivateKey string
	// Subject identifies the sender to push services: a mailto: or https:
	// URL. Apple rejects pushes without a valid one.
	Subject string
	// Client defaults to one with a 20 s timeout.
	Client *http.Client
}

// GenerateKeys makes a VAPID key pair, base64url encoded.
func GenerateKeys() (public, private string, err error) {
	private, public, err = webpush.GenerateVAPIDKeys()
	return public, private, err
}

// pushTTL is how long a push service holds a message for an offline
// device. A day-old change is still news; a week-old one isn't.
const pushTTL = 24 * time.Hour

// send makes one attempt. retryAfter is the server's Retry-After, if it
// sent one.
func (s *Sender) send(ctx context.Context, sub store.Subscription, p Payload) (res result, retryAfter time.Duration, err error) {
	body, err := json.Marshal(p)
	if err != nil {
		return failed, 0, err
	}
	client := s.Client
	if client == nil {
		client = &http.Client{Timeout: 20 * time.Second}
	}
	resp, err := webpush.SendNotificationWithContext(ctx, body,
		&webpush.Subscription{Endpoint: sub.Endpoint, Keys: webpush.Keys{P256dh: sub.P256dh, Auth: sub.Auth}},
		&webpush.Options{
			HTTPClient: client,
			// webpush-go adds "mailto:" to anything that isn't https.
			Subscriber:      strings.TrimPrefix(s.Subject, "mailto:"),
			VAPIDPublicKey:  s.PublicKey,
			VAPIDPrivateKey: s.PrivateKey,
			TTL:             int(pushTTL.Seconds()),
			Urgency:         webpush.UrgencyNormal,
			// The Topic header collapses an undelivered message with a
			// newer one, as the tag does on the device.
			Topic: p.Tag,
		})
	if err != nil {
		if ctx.Err() != nil {
			return failed, 0, err
		}
		var urlErr *url.Error
		if errors.As(err, &urlErr) {
			return retryable, 0, err
		}
		// Bad keys in the subscription: it can never work.
		return gone, 0, err
	}
	defer resp.Body.Close()
	msg, _ := io.ReadAll(io.LimitReader(resp.Body, 512))

	switch code := resp.StatusCode; {
	case code >= 200 && code < 300:
		return sent, 0, nil
	case code == http.StatusNotFound || code == http.StatusGone:
		return gone, 0, statusError(code, msg)
	case code == http.StatusTooManyRequests || code >= 500:
		ra, _ := strconv.Atoi(resp.Header.Get("Retry-After"))
		return retryable, time.Duration(ra) * time.Second, statusError(code, msg)
	default:
		return failed, 0, statusError(code, msg)
	}
}

func statusError(code int, body []byte) error {
	return fmt.Errorf("push service: %d %s", code, strings.TrimSpace(string(body)))
}
