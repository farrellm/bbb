package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

// Event is one stored change to an item, with whether the viewing user has
// read it. Events from before the user's account existed count as read, so
// a new parent doesn't start with every past edit marked.
type Event struct {
	ID            int64
	PanelID       int64
	Kind          string
	CreatedAt     time.Time
	FromVersionID *int64
	ToVersionID   *int64
	RepostOf      *int64
	BecameSignup  bool
	WordsAdded    int
	WordsRemoved  int
	Unread        bool
}

// FeedItem is an item's current state for one user.
type FeedItem struct {
	PanelID      int64
	Division     string
	Position     int
	VersionID    int64
	VersionCount int
	Title        string
	Subheader    string
	BodyHTML     string
	IsSignup     bool
	SignupLinks  []string
	Deadline     *time.Time
	FirstSeenAt  time.Time
	LastSeenAt   time.Time
	RemovedAt    *time.Time
	// LatestEvent is the newest new, edit or removed event. Moves are left
	// out: they say nothing about the content.
	LatestEvent *Event
}

// ItemFilter narrows ListItems. Zero values mean no filtering, except that
// removed items are left out unless IncludeRemoved is set.
type ItemFilter struct {
	PanelID        int64
	Division       string
	SignupsOnly    bool
	IncludeRemoved bool
}

// unreadExpr is true for an event e the user u (with read mark r) hasn't
// read.
const unreadExpr = `(e.created_at >= u.created_at AND r.event_id IS NULL)`

const eventCols = `e.id, e.panel_id, e.kind, e.created_at, e.from_version_id, e.to_version_id,
	e.repost_of, e.became_signup, e.words_added, e.words_removed, ` + unreadExpr

// ListItems returns items in page order: by division, then position.
func (s *Store) ListItems(ctx context.Context, userID int64, f ItemFilter) (items []FeedItem, err error) {
	defer wrap(&err, "list items")

	rows, err := s.pool.Query(ctx, `
		SELECT i.panel_id, i.division, i.position, v.id,
			(SELECT count(*) FROM item_versions c WHERE c.panel_id = i.panel_id),
			v.title, v.subheader, v.body_html, i.is_signup, v.signup_links, i.deadline,
			i.first_seen_at, i.last_seen_at, i.removed_at,
			e.id, e.panel_id, e.kind, e.created_at, e.from_version_id, e.to_version_id,
			e.repost_of, e.became_signup, e.words_added, e.words_removed,
			coalesce(`+unreadExpr+`, false)
		FROM items i
		JOIN item_versions v ON v.id = i.current_version_id
		CROSS JOIN (SELECT created_at FROM users WHERE id = $1) u
		LEFT JOIN LATERAL (
			SELECT * FROM events e
			WHERE e.panel_id = i.panel_id AND e.kind <> 'moved'
			ORDER BY e.id DESC LIMIT 1
		) e ON true
		LEFT JOIN reads r ON r.user_id = $1 AND r.event_id = e.id
		WHERE ($2 = 0 OR i.panel_id = $2)
			AND ($3 = '' OR i.division = $3)
			AND (NOT $4 OR i.is_signup)
			AND ($5 OR i.removed_at IS NULL)
		ORDER BY array_position(ARRAY['all', 'lower', 'middle', 'upper'], i.division), i.position`,
		userID, f.PanelID, f.Division, f.SignupsOnly, f.IncludeRemoved || f.PanelID != 0)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(r pgx.CollectableRow) (FeedItem, error) {
		var it FeedItem
		var e struct {
			ID, PanelID              *int64
			Kind                     *string
			CreatedAt                *time.Time
			BecameSignup             *bool
			WordsAdded, WordsRemoved *int
		}
		var ev Event
		err := r.Scan(&it.PanelID, &it.Division, &it.Position, &it.VersionID, &it.VersionCount,
			&it.Title, &it.Subheader, &it.BodyHTML, &it.IsSignup, &it.SignupLinks, &it.Deadline,
			&it.FirstSeenAt, &it.LastSeenAt, &it.RemovedAt,
			&e.ID, &e.PanelID, &e.Kind, &e.CreatedAt, &ev.FromVersionID, &ev.ToVersionID,
			&ev.RepostOf, &e.BecameSignup, &e.WordsAdded, &e.WordsRemoved, &ev.Unread)
		if err != nil || e.ID == nil {
			return it, err
		}
		ev.ID, ev.PanelID, ev.Kind, ev.CreatedAt = *e.ID, *e.PanelID, *e.Kind, *e.CreatedAt
		ev.BecameSignup, ev.WordsAdded, ev.WordsRemoved = *e.BecameSignup, *e.WordsAdded, *e.WordsRemoved
		it.LatestEvent = &ev
		return it, nil
	})
}

// GetItem returns one item, removed or not.
func (s *Store) GetItem(ctx context.Context, userID, panelID int64) (FeedItem, error) {
	items, err := s.ListItems(ctx, userID, ItemFilter{PanelID: panelID})
	if err != nil {
		return FeedItem{}, err
	}
	if len(items) == 0 {
		return FeedItem{}, ErrNotFound
	}
	return items[0], nil
}

// VersionMeta identifies one version of an item.
type VersionMeta struct {
	ID     int64
	SeenAt time.Time
	Title  string
}

// ListVersions returns an item's versions, oldest first.
func (s *Store) ListVersions(ctx context.Context, panelID int64) ([]VersionMeta, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, seen_at, title FROM item_versions WHERE panel_id = $1 ORDER BY id`, panelID)
	if err != nil {
		return nil, fmt.Errorf("list versions: %w", err)
	}
	vs, err := pgx.CollectRows(rows, pgx.RowToStructByPos[VersionMeta])
	if err != nil {
		return nil, fmt.Errorf("list versions: %w", err)
	}
	return vs, nil
}

// Version is the full text of one version.
type Version struct {
	ID          int64
	PanelID     int64
	SeenAt      time.Time
	Title       string
	Subheader   string
	BodyHTML    string
	BodyText    string
	SignupLinks []string
}

// GetVersion returns a version of the given item.
func (s *Store) GetVersion(ctx context.Context, panelID, versionID int64) (Version, error) {
	var v Version
	err := s.pool.QueryRow(ctx, `
		SELECT id, panel_id, seen_at, title, subheader, body_html, body_text, signup_links
		FROM item_versions WHERE panel_id = $1 AND id = $2`, panelID, versionID,
	).Scan(&v.ID, &v.PanelID, &v.SeenAt, &v.Title, &v.Subheader, &v.BodyHTML, &v.BodyText, &v.SignupLinks)
	if errors.Is(err, pgx.ErrNoRows) {
		return v, ErrNotFound
	}
	if err != nil {
		return v, fmt.Errorf("get version: %w", err)
	}
	return v, nil
}

// TimelineEvent is an event with enough of its item to list it.
type TimelineEvent struct {
	Event
	Division string
	Title    string // the to-version's title, or the from-version's for removals
	IsSignup bool
}

// EventFilter narrows ListEvents. Before is an event id to page back from
// (0 for the newest); Limit caps the page.
type EventFilter struct {
	PanelID int64
	Before  int64
	Limit   int
}

// ListEvents returns events newest first.
func (s *Store) ListEvents(ctx context.Context, userID int64, f EventFilter) ([]TimelineEvent, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT `+eventCols+`, i.division, v.title, i.is_signup
		FROM events e
		JOIN items i ON i.panel_id = e.panel_id
		JOIN item_versions v ON v.id = coalesce(e.to_version_id, e.from_version_id)
		CROSS JOIN (SELECT created_at FROM users WHERE id = $1) u
		LEFT JOIN reads r ON r.user_id = $1 AND r.event_id = e.id
		WHERE ($2 = 0 OR e.panel_id = $2) AND ($3 = 0 OR e.id < $3)
		ORDER BY e.id DESC
		LIMIT $4`, userID, f.PanelID, f.Before, f.Limit)
	if err != nil {
		return nil, fmt.Errorf("list events: %w", err)
	}
	evs, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (TimelineEvent, error) {
		var t TimelineEvent
		err := r.Scan(&t.ID, &t.PanelID, &t.Kind, &t.CreatedAt, &t.FromVersionID, &t.ToVersionID,
			&t.RepostOf, &t.BecameSignup, &t.WordsAdded, &t.WordsRemoved, &t.Unread,
			&t.Division, &t.Title, &t.IsSignup)
		return t, err
	})
	if err != nil {
		return nil, fmt.Errorf("list events: %w", err)
	}
	return evs, nil
}

// MarkRead records that the user has seen the given events. Unknown ids
// are ignored.
func (s *Store) MarkRead(ctx context.Context, userID int64, eventIDs []int64) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO reads (user_id, event_id)
		SELECT $1, id FROM events WHERE id = ANY($2)
		ON CONFLICT DO NOTHING`, userID, eventIDs)
	if err != nil {
		return fmt.Errorf("mark read: %w", err)
	}
	return nil
}

// LastReadAt returns when the user last marked anything read, or nil.
func (s *Store) LastReadAt(ctx context.Context, userID int64) (*time.Time, error) {
	var at *time.Time
	err := s.pool.QueryRow(ctx, `SELECT max(read_at) FROM reads WHERE user_id = $1`, userID).Scan(&at)
	if err != nil {
		return nil, fmt.Errorf("last read: %w", err)
	}
	return at, nil
}
