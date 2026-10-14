package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// Preferences are what a user wants to be notified about.
type Preferences struct {
	// Divisions whose items notify; a subset of all, lower, middle, upper.
	Divisions     []string
	NotifyNew     bool
	NotifyEdit    bool
	NotifyRemoved bool
	// SignupsOnly limits notifications to signup items.
	SignupsOnly bool
	// SignupsAnyDivision lets signup items notify from any division.
	SignupsAnyDivision bool
	// MinEditWords is the smallest edit, in words added plus removed, that
	// notifies.
	MinEditWords int
	// QuietStart and QuietEnd ("21:30") bound the user's quiet hours in the
	// school's time zone; both are nil when there are none. Pushes due
	// inside them are held, then sent as one.
	QuietStart *string
	QuietEnd   *string
}

// prefCols reads a preferences row (or prefColsRaw's columns); quiet hours
// come back as "15:04".
const (
	prefColsRaw = `divisions, notify_new, notify_edit, notify_removed,
	signups_only, signups_any_division, min_edit_words, quiet_start, quiet_end`
	prefCols = `divisions, notify_new, notify_edit, notify_removed,
	signups_only, signups_any_division, min_edit_words,
	to_char(quiet_start, 'HH24:MI'), to_char(quiet_end, 'HH24:MI')`
)

func (p *Preferences) dest() []any {
	return []any{
		&p.Divisions, &p.NotifyNew, &p.NotifyEdit, &p.NotifyRemoved,
		&p.SignupsOnly, &p.SignupsAnyDivision, &p.MinEditWords,
		&p.QuietStart, &p.QuietEnd,
	}
}

// GetPreferences returns the user's preferences. Accounts get a row when
// they are created; one made before that (by an older server against a
// migrated database) gets the defaults here.
func (s *Store) GetPreferences(ctx context.Context, userID int64) (Preferences, error) {
	var p Preferences
	err := s.pool.QueryRow(ctx, `
		WITH made AS (
			INSERT INTO preferences (user_id)
			SELECT id FROM users WHERE id = $1
			ON CONFLICT DO NOTHING
			RETURNING `+prefColsRaw+`
		)
		SELECT `+prefCols+` FROM made
		UNION ALL
		SELECT `+prefCols+` FROM preferences WHERE user_id = $1`, userID).
		Scan(p.dest()...)
	if errors.Is(err, pgx.ErrNoRows) {
		return p, ErrNotFound
	}
	if err != nil {
		return p, fmt.Errorf("get preferences: %w", err)
	}
	return p, nil
}

// SetPreferences replaces the user's preferences.
func (s *Store) SetPreferences(ctx context.Context, userID int64, p Preferences) error {
	divs := p.Divisions
	if divs == nil {
		divs = []string{}
	}
	_, err := s.pool.Exec(ctx, `
		INSERT INTO preferences (user_id, divisions, notify_new, notify_edit, notify_removed,
			signups_only, signups_any_division, min_edit_words, quiet_start, quiet_end)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9::time, $10::time)
		ON CONFLICT (user_id) DO UPDATE SET
			divisions = excluded.divisions,
			notify_new = excluded.notify_new,
			notify_edit = excluded.notify_edit,
			notify_removed = excluded.notify_removed,
			signups_only = excluded.signups_only,
			signups_any_division = excluded.signups_any_division,
			min_edit_words = excluded.min_edit_words,
			quiet_start = excluded.quiet_start,
			quiet_end = excluded.quiet_end`,
		userID, divs, p.NotifyNew, p.NotifyEdit, p.NotifyRemoved,
		p.SignupsOnly, p.SignupsAnyDivision, p.MinEditWords, p.QuietStart, p.QuietEnd)
	if err != nil {
		return fmt.Errorf("set preferences: %w", err)
	}
	return nil
}

// Subscription is one device's Web Push subscription.
type Subscription struct {
	ID        int64
	UserID    int64
	Endpoint  string
	P256dh    string
	Auth      string
	UserAgent string
	CreatedAt time.Time
	LastOKAt  *time.Time
}

const subCols = `id, user_id, endpoint, p256dh, auth, user_agent, created_at, last_ok_at`

// SaveSubscription stores a device's subscription for the user. An
// endpoint is one browser on one device, so if it was already stored
// (perhaps for whoever signed in there before) it moves to this user with
// the new keys.
func (s *Store) SaveSubscription(ctx context.Context, sub Subscription) (Subscription, error) {
	rows, err := s.pool.Query(ctx, `
		INSERT INTO push_subscriptions (user_id, endpoint, p256dh, auth, user_agent)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (endpoint) DO UPDATE SET
			user_id = excluded.user_id,
			p256dh = excluded.p256dh,
			auth = excluded.auth,
			user_agent = excluded.user_agent
		RETURNING `+subCols,
		sub.UserID, sub.Endpoint, sub.P256dh, sub.Auth, sub.UserAgent)
	if err != nil {
		return sub, fmt.Errorf("save subscription: %w", err)
	}
	saved, err := pgx.CollectExactlyOneRow(rows, pgx.RowToStructByPos[Subscription])
	if err != nil {
		return sub, fmt.Errorf("save subscription: %w", err)
	}
	return saved, nil
}

// ListSubscriptions returns the user's devices, oldest first.
func (s *Store) ListSubscriptions(ctx context.Context, userID int64) ([]Subscription, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT `+subCols+` FROM push_subscriptions WHERE user_id = $1 ORDER BY id`, userID)
	if err != nil {
		return nil, fmt.Errorf("list subscriptions: %w", err)
	}
	subs, err := pgx.CollectRows(rows, pgx.RowToStructByPos[Subscription])
	if err != nil {
		return nil, fmt.Errorf("list subscriptions: %w", err)
	}
	return subs, nil
}

// DeleteSubscription removes one of the user's devices by id.
func (s *Store) DeleteSubscription(ctx context.Context, userID, id int64) error {
	tag, err := s.pool.Exec(ctx, `DELETE FROM push_subscriptions WHERE user_id = $1 AND id = $2`,
		userID, id)
	if err != nil {
		return fmt.Errorf("delete subscription: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// DeleteSubscriptionByEndpoint removes the user's subscription with this
// endpoint, if there is one.
func (s *Store) DeleteSubscriptionByEndpoint(ctx context.Context, userID int64, endpoint string) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM push_subscriptions WHERE user_id = $1 AND endpoint = $2`,
		userID, endpoint)
	if err != nil {
		return fmt.Errorf("delete subscription: %w", err)
	}
	return nil
}

// DropSubscription removes a subscription the push service says is gone.
func (s *Store) DropSubscription(ctx context.Context, id int64) error {
	if _, err := s.pool.Exec(ctx, `DELETE FROM push_subscriptions WHERE id = $1`, id); err != nil {
		return fmt.Errorf("drop subscription: %w", err)
	}
	return nil
}

// Recipient is a user with at least one device, and their preferences.
type Recipient struct {
	UserID int64
	Prefs  Preferences
	Subs   []Subscription
}

// Recipients returns every user who has a device to push to.
func (s *Store) Recipients(ctx context.Context) ([]Recipient, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT `+prefCols+`, s.id, s.user_id, s.endpoint, s.p256dh, s.auth, s.user_agent,
			s.created_at, s.last_ok_at
		FROM push_subscriptions s
		JOIN preferences p ON p.user_id = s.user_id
		ORDER BY s.user_id, s.id`)
	if err != nil {
		return nil, fmt.Errorf("recipients: %w", err)
	}
	defer rows.Close()

	var out []Recipient
	for rows.Next() {
		var p Preferences
		var sub Subscription
		dest := append(p.dest(), &sub.ID, &sub.UserID, &sub.Endpoint, &sub.P256dh, &sub.Auth,
			&sub.UserAgent, &sub.CreatedAt, &sub.LastOKAt)
		if err := rows.Scan(dest...); err != nil {
			return nil, fmt.Errorf("recipients: %w", err)
		}
		if n := len(out); n > 0 && out[n-1].UserID == sub.UserID {
			out[n-1].Subs = append(out[n-1].Subs, sub)
			continue
		}
		out = append(out, Recipient{UserID: sub.UserID, Prefs: p, Subs: []Subscription{sub}})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("recipients: %w", err)
	}
	return out, nil
}

// PendingEvent is an event the notifier hasn't handled yet, with what a
// notification about it says.
type PendingEvent struct {
	ID            int64
	RunID         int64
	PanelID       int64
	Kind          string
	FromVersionID *int64
	ToVersionID   *int64
	BecameSignup  bool
	WordsAdded    int
	WordsRemoved  int
	Division      string
	Title         string // the to-version's title, or the from-version's for removals
	IsSignup      bool
	Deadline      *time.Time
}

// pendingCols and pendingFrom select PendingEvents.
const (
	pendingCols = `e.id, e.run_id, e.panel_id, e.kind, e.from_version_id, e.to_version_id,
		e.became_signup, e.words_added, e.words_removed,
		i.division, v.title, i.is_signup, i.deadline`
	pendingFrom = `
		FROM events e
		JOIN items i ON i.panel_id = e.panel_id
		JOIN item_versions v ON v.id = coalesce(e.to_version_id, e.from_version_id)`
)

// PendingEvents returns the events not yet stamped notified, oldest first.
// Only committed runs' events are visible, so a run is never half-sent.
func (s *Store) PendingEvents(ctx context.Context) ([]PendingEvent, error) {
	rows, err := s.pool.Query(ctx, `SELECT `+pendingCols+pendingFrom+`
		WHERE e.notified_at IS NULL
		ORDER BY e.id`)
	if err != nil {
		return nil, fmt.Errorf("pending events: %w", err)
	}
	evs, err := pgx.CollectRows(rows, pgx.RowToStructByPos[PendingEvent])
	if err != nil {
		return nil, fmt.Errorf("pending events: %w", err)
	}
	return evs, nil
}

// MarkNotified stamps events as handled by the notifier.
func (s *Store) MarkNotified(ctx context.Context, eventIDs []int64) error {
	_, err := s.pool.Exec(ctx, `UPDATE events SET notified_at = now() WHERE id = ANY($1)`, eventIDs)
	if err != nil {
		return fmt.Errorf("mark notified: %w", err)
	}
	return nil
}

// HoldEvents keeps events for a user in quiet hours, to send when they end.
func (s *Store) HoldEvents(ctx context.Context, userID int64, eventIDs []int64) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO held_events (user_id, event_id)
		SELECT $1, id FROM unnest($2::bigint[]) AS id
		ON CONFLICT DO NOTHING`, userID, eventIDs)
	if err != nil {
		return fmt.Errorf("hold events: %w", err)
	}
	return nil
}

// HeldEvents returns each user's held events, oldest first, leaving out
// any the user has since read in the app.
func (s *Store) HeldEvents(ctx context.Context) (map[int64][]PendingEvent, error) {
	rows, err := s.pool.Query(ctx, `SELECT h.user_id, `+pendingCols+pendingFrom+`
		JOIN held_events h ON h.event_id = e.id
		LEFT JOIN reads r ON r.user_id = h.user_id AND r.event_id = e.id
		WHERE r.event_id IS NULL
		ORDER BY h.user_id, e.id`)
	if err != nil {
		return nil, fmt.Errorf("held events: %w", err)
	}
	defer rows.Close()

	out := map[int64][]PendingEvent{}
	for rows.Next() {
		var userID int64
		var e PendingEvent
		if err := rows.Scan(&userID, &e.ID, &e.RunID, &e.PanelID, &e.Kind, &e.FromVersionID,
			&e.ToVersionID, &e.BecameSignup, &e.WordsAdded, &e.WordsRemoved, &e.Division,
			&e.Title, &e.IsSignup, &e.Deadline); err != nil {
			return nil, fmt.Errorf("held events: %w", err)
		}
		out[userID] = append(out[userID], e)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("held events: %w", err)
	}
	return out, nil
}

// ReleaseHeld forgets the events held for everyone except the users in
// keep: once they are sent, read already, or have no device to go to.
func (s *Store) ReleaseHeld(ctx context.Context, keep []int64) error {
	if keep == nil {
		keep = []int64{}
	}
	if _, err := s.pool.Exec(ctx, `DELETE FROM held_events WHERE user_id <> ALL($1)`, keep); err != nil {
		return fmt.Errorf("release held: %w", err)
	}
	return nil
}

// AdminSubscriptions returns every admin's devices, for alerts about the
// poller.
func (s *Store) AdminSubscriptions(ctx context.Context) ([]Subscription, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT `+subCols+` FROM push_subscriptions
		WHERE user_id IN (SELECT id FROM users WHERE is_admin)
		ORDER BY id`)
	if err != nil {
		return nil, fmt.Errorf("admin subscriptions: %w", err)
	}
	subs, err := pgx.CollectRows(rows, pgx.RowToStructByPos[Subscription])
	if err != nil {
		return nil, fmt.Errorf("admin subscriptions: %w", err)
	}
	return subs, nil
}

// Delivery statuses.
const (
	DeliverySent   = "sent"
	DeliveryGone   = "gone"
	DeliveryFailed = "failed"
)

// RecordDelivery logs one attempt to push the given events to a device. A
// sent push also stamps the device's last_ok_at.
func (s *Store) RecordDelivery(ctx context.Context, eventIDs []int64, subID int64, status, errText string) (err error) {
	defer wrap(&err, "record delivery")
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	// A gone subscription is deleted right after, which nulls these rows'
	// subscription_id; the status still says what happened.
	if _, err := tx.Exec(ctx, `
		INSERT INTO deliveries (event_id, subscription_id, status, error)
		SELECT id, $2, $3, nullif($4, '') FROM unnest($1::bigint[]) AS id`,
		eventIDs, subID, status, errText); err != nil {
		return err
	}
	if err := touchSubscription(ctx, tx, subID, status); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// TouchSubscription stamps last_ok_at after a push that isn't tied to
// events, such as a test.
func (s *Store) TouchSubscription(ctx context.Context, subID int64, status string) error {
	return touchSubscription(ctx, s.pool, subID, status)
}

type execer interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
}

func touchSubscription(ctx context.Context, db execer, subID int64, status string) error {
	if status != DeliverySent {
		return nil
	}
	if _, err := db.Exec(ctx, `UPDATE push_subscriptions SET last_ok_at = now() WHERE id = $1`, subID); err != nil {
		return fmt.Errorf("touch subscription: %w", err)
	}
	return nil
}

// UnreadCount is how many events the user hasn't read, for the app badge.
// Moves say nothing about content and are left out, as in the feed.
func (s *Store) UnreadCount(ctx context.Context, userID int64) (int, error) {
	var n int
	err := s.pool.QueryRow(ctx, `
		SELECT count(*)
		FROM events e
		CROSS JOIN (SELECT created_at FROM users WHERE id = $1) u
		LEFT JOIN reads r ON r.user_id = $1 AND r.event_id = e.id
		WHERE e.kind <> 'moved' AND `+unreadExpr, userID).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("unread count: %w", err)
	}
	return n, nil
}
