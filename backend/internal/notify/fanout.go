package notify

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/farrellm/bbb/internal/store"
)

const (
	// sweepInterval is how often the notifier checks for events without
	// being kicked, e.g. ones a -scrape-once run wrote.
	sweepInterval = time.Minute
	// maxAttempts is how many times a retryable push is tried.
	maxAttempts = 4
	// maxRetryAfter caps how long a push service's Retry-After can hold
	// up everyone else's notifications.
	maxRetryAfter = time.Minute
)

// Notifier pushes committed events to the users whose preferences match.
// Events are its queue: it handles each one once, then stamps it.
type Notifier struct {
	Store *store.Store
	// Sender is nil when VAPID keys aren't configured. Events are still
	// stamped, so turning push on later doesn't announce old news.
	Sender *Sender
	Log    *slog.Logger
	// Backoff is the first retry delay, doubling after each attempt.
	Backoff time.Duration

	kick chan struct{}
}

// New returns a Notifier; sender may be nil.
func New(st *store.Store, sender *Sender, log *slog.Logger) *Notifier {
	return &Notifier{Store: st, Sender: sender, Log: log, Backoff: 2 * time.Second, kick: make(chan struct{}, 1)}
}

// Enabled reports whether pushes can be sent.
func (n *Notifier) Enabled() bool {
	return n.Sender != nil
}

// Kick asks Run to look for new events now, e.g. after a run wrote some.
func (n *Notifier) Kick() {
	select {
	case n.kick <- struct{}{}:
	default: // one is already pending
	}
}

// Run handles events until ctx is done: at start, when kicked, and every
// minute in case something was missed.
func (n *Notifier) Run(ctx context.Context) {
	t := time.NewTicker(sweepInterval)
	defer t.Stop()
	for {
		if err := n.Flush(ctx); err != nil && ctx.Err() == nil {
			n.Log.Warn("notify failed", "err", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-n.kick:
		case <-t.C:
		}
	}
}

// Flush pushes every pending event to every matching user's devices, then
// stamps the events. Failures to individual devices are logged and
// recorded in deliveries; they don't hold the events back.
func (n *Notifier) Flush(ctx context.Context) error {
	evs, err := n.Store.PendingEvents(ctx)
	if err != nil || len(evs) == 0 {
		return err
	}
	if n.Sender != nil {
		recipients, err := n.Store.Recipients(ctx)
		if err != nil {
			return err
		}
		for _, r := range recipients {
			n.notifyUser(ctx, r, evs)
		}
	}
	ids := make([]int64, len(evs))
	for i, e := range evs {
		ids[i] = e.ID
	}
	return n.Store.MarkNotified(ctx, ids)
}

func (n *Notifier) notifyUser(ctx context.Context, r store.Recipient, evs []store.PendingEvent) {
	var mine []store.PendingEvent
	for _, e := range evs {
		if Matches(r.Prefs, e) {
			mine = append(mine, e)
		}
	}
	if len(mine) == 0 {
		return
	}
	badge, err := n.Store.UnreadCount(ctx, r.UserID)
	if err != nil {
		n.Log.Warn("unread count", "err", err, "user", r.UserID)
	}
	deadSubs := map[int64]bool{}
	for _, m := range plan(mine) {
		m.payload.Badge = badge
		for _, sub := range r.Subs {
			if !deadSubs[sub.ID] && n.deliver(ctx, sub, m.payload, m.events, maxAttempts) == gone {
				deadSubs[sub.ID] = true
			}
		}
	}
	n.Log.Info("notified", "user", r.UserID, "events", len(mine), "devices", len(r.Subs)-len(deadSubs))
}

// deliver pushes p to one device, retrying 429s, 5xx and network errors
// with back-off, and records each attempt against events (if any). A
// subscription the push service says is gone is deleted.
func (n *Notifier) deliver(ctx context.Context, sub store.Subscription, p Payload, events []int64, attempts int) result {
	delay := n.Backoff
	for attempt := 1; ; attempt++ {
		res, retryAfter, err := n.Sender.send(ctx, sub, p)
		status, errText := store.DeliverySent, ""
		switch res {
		case sent:
		case gone:
			status = store.DeliveryGone
		case retryable, failed:
			status = store.DeliveryFailed
		}
		if err != nil {
			errText = err.Error()
			n.Log.Warn("push failed", "err", err, "subscription", sub.ID, "attempt", attempt)
		}

		var rerr error
		if len(events) > 0 {
			rerr = n.Store.RecordDelivery(ctx, events, sub.ID, status, errText)
		} else {
			rerr = n.Store.TouchSubscription(ctx, sub.ID, status)
		}
		if res == gone {
			rerr = errors.Join(rerr, n.Store.DropSubscription(ctx, sub.ID))
		}
		if rerr != nil {
			n.Log.Warn("record delivery", "err", rerr, "subscription", sub.ID)
		}

		if res != retryable || attempt >= attempts {
			return res
		}
		wait := max(delay, min(retryAfter, maxRetryAfter))
		delay *= 2
		select {
		case <-ctx.Done():
			return res
		case <-time.After(wait):
		}
	}
}

// TestResult counts the outcome of a test push.
type TestResult struct {
	Sent   int
	Failed int
}

// Test pushes a test notification to each of the user's devices, once
// each, so the Settings button answers promptly. Gone devices are removed.
func (n *Notifier) Test(ctx context.Context, userID int64) (TestResult, error) {
	var out TestResult
	subs, err := n.Store.ListSubscriptions(ctx, userID)
	if err != nil {
		return out, err
	}
	badge, err := n.Store.UnreadCount(ctx, userID)
	if err != nil {
		return out, err
	}
	p := Payload{
		Title: "Notifications are on",
		Body:  "This is a test from BBB Watch. New and edited items will arrive like this.",
		URL:   "/settings",
		Tag:   "test",
		Badge: badge,
	}
	for _, sub := range subs {
		if n.deliver(ctx, sub, p, nil, 1) == sent {
			out.Sent++
		} else {
			out.Failed++
		}
	}
	return out, nil
}
