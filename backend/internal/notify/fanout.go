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
	// Location is the school's time zone, which quiet hours are in.
	Location *time.Location
	// Now is the clock; nil means time.Now.
	Now func() time.Time

	kick chan struct{}
}

// New returns a Notifier; sender may be nil.
func New(st *store.Store, sender *Sender, log *slog.Logger) *Notifier {
	return &Notifier{
		Store: st, Sender: sender, Log: log, Backoff: 2 * time.Second, Location: time.Local,
		kick: make(chan struct{}, 1),
	}
}

func (n *Notifier) now() time.Time {
	if n.Now != nil {
		return n.Now().In(n.Location)
	}
	return time.Now().In(n.Location)
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
// stamps the events. A user in quiet hours has theirs held instead, and
// the first Flush after their quiet hours end sends what was held as one
// push. Failures to individual devices are logged and recorded in
// deliveries; they don't hold the events back.
func (n *Notifier) Flush(ctx context.Context) error {
	evs, err := n.Store.PendingEvents(ctx)
	if err != nil {
		return err
	}
	held, err := n.Store.HeldEvents(ctx)
	if err != nil {
		return err
	}
	if len(evs) == 0 && len(held) == 0 {
		return nil
	}

	var stillQuiet []int64
	if n.Sender != nil {
		recipients, err := n.Store.Recipients(ctx)
		if err != nil {
			return err
		}
		now := n.now()
		for _, r := range recipients {
			mine := matching(r.Prefs, evs)
			if quiet(r.Prefs, now) {
				stillQuiet = append(stillQuiet, r.UserID)
				if len(mine) > 0 {
					if err := n.Store.HoldEvents(ctx, r.UserID, ids(mine)); err != nil {
						return err
					}
				}
				continue
			}
			n.notifyUser(ctx, r, matching(r.Prefs, held[r.UserID]), mine)
		}
	}
	if len(evs) > 0 {
		if err := n.Store.MarkNotified(ctx, ids(evs)); err != nil {
			return err
		}
	}
	if len(held) > 0 {
		return n.Store.ReleaseHeld(ctx, stillQuiet)
	}
	return nil
}

// matching returns the events these preferences notify about.
func matching(p store.Preferences, evs []store.PendingEvent) []store.PendingEvent {
	var out []store.PendingEvent
	for _, e := range evs {
		if Matches(p, e) {
			out = append(out, e)
		}
	}
	return out
}

func ids(evs []store.PendingEvent) []int64 {
	out := make([]int64, len(evs))
	for i, e := range evs {
		out[i] = e.ID
	}
	return out
}

// notifyUser pushes to one user's devices: first what was held during
// their quiet hours, as one push, then the new events.
func (n *Notifier) notifyUser(ctx context.Context, r store.Recipient, held, mine []store.PendingEvent) {
	msgs := plan(mine)
	if len(held) > 0 {
		msgs = append([]message{released(held)}, msgs...)
	}
	if len(msgs) == 0 {
		return
	}
	badge, err := n.Store.UnreadCount(ctx, r.UserID)
	if err != nil {
		n.Log.Warn("unread count", "err", err, "user", r.UserID)
	}
	deadSubs := map[int64]bool{}
	for _, m := range msgs {
		m.payload.Badge = badge
		for _, sub := range r.Subs {
			if !deadSubs[sub.ID] && n.deliver(ctx, sub, m.payload, m.events, maxAttempts) == gone {
				deadSubs[sub.ID] = true
			}
		}
	}
	n.Log.Info("notified", "user", r.UserID, "events", len(mine), "held", len(held),
		"devices", len(r.Subs)-len(deadSubs))
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

// Alert pushes p to every admin's devices, e.g. when the poller keeps
// failing. Alerts are rare and need action, so they ignore quiet hours.
func (n *Notifier) Alert(ctx context.Context, p Payload) {
	if n.Sender == nil {
		return
	}
	subs, err := n.Store.AdminSubscriptions(ctx)
	if err != nil {
		n.Log.Warn("alert", "err", err)
		return
	}
	for _, sub := range subs {
		// The worker sets the app badge from every push, so each carries
		// its own user's count.
		if p.Badge, err = n.Store.UnreadCount(ctx, sub.UserID); err != nil {
			n.Log.Warn("unread count", "err", err, "user", sub.UserID)
		}
		n.deliver(ctx, sub, p, nil, maxAttempts)
	}
	n.Log.Info("alerted admins", "title", p.Title, "devices", len(subs))
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
		Body:  "This is a test from Pingry BBB Watch. New and edited items will arrive like this.",
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
