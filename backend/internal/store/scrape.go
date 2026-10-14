package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/farrellm/bbb/internal/bbb"
)

// ScrapeRun is one poll of the page, as recorded in scrape_runs.
type ScrapeRun struct {
	StartedAt  time.Time
	FinishedAt time.Time
	HTTPStatus int    // 0 when the request never got a response
	BodyHash   string // "" when there was no body
	Unchanged  bool   // body hash matched the previous run; parsing skipped
	ItemCount  *int   // nil when the page was not parsed
	Suspect    bool
	Error      string
}

// LoadState reads every known item with its current version, plus the item
// count of the last good run, for the differ.
func (s *Store) LoadState(ctx context.Context) (st bbb.State, err error) {
	defer wrap(&err, "load state")

	rows, err := s.pool.Query(ctx, `
		SELECT i.panel_id, i.division, i.position, v.id, v.content_hash,
		       v.title, v.subheader, v.body_text, v.signup_links, i.removed_at
		FROM items i
		JOIN item_versions v ON v.id = i.current_version_id`)
	if err != nil {
		return st, err
	}
	defer rows.Close()

	st.Items = map[int64]bbb.Known{}
	for rows.Next() {
		var k bbb.Known
		var div string
		if err := rows.Scan(&k.PanelID, &div, &k.Position, &k.VersionID, &k.ContentHash,
			&k.Title, &k.Subheader, &k.BodyText, &k.SignupLinks, &k.RemovedAt); err != nil {
			return st, err
		}
		k.Division = bbb.Division(div)
		st.Items[k.PanelID] = k
	}
	if err := rows.Err(); err != nil {
		return st, err
	}

	err = s.pool.QueryRow(ctx, `
		SELECT item_count FROM scrape_runs
		WHERE NOT suspect AND error IS NULL AND item_count IS NOT NULL
		ORDER BY id DESC LIMIT 1`).Scan(&st.LastGoodCount)
	if errors.Is(err, pgx.ErrNoRows) {
		err = nil
	}
	return st, err
}

// LastBodyHash returns the body hash of the most recent run that fetched the
// page, or "" if there is none.
func (s *Store) LastBodyHash(ctx context.Context) (string, error) {
	var hash string
	err := s.pool.QueryRow(ctx, `
		SELECT body_hash FROM scrape_runs
		WHERE body_hash IS NOT NULL AND error IS NULL
		ORDER BY id DESC LIMIT 1`).Scan(&hash)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("last body hash: %w", err)
	}
	return hash, nil
}

// SuspectStreak returns how many of the most recent runs in a row were
// suspect or failed.
func (s *Store) SuspectStreak(ctx context.Context) (int, error) {
	var n int
	err := s.pool.QueryRow(ctx, `
		SELECT count(*) FROM scrape_runs
		WHERE id > coalesce((
			SELECT max(id) FROM scrape_runs WHERE NOT suspect AND error IS NULL
		), 0)`).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("suspect streak: %w", err)
	}
	return n, nil
}

// RecordRun writes a run and, when plan is non-nil and not suspect, applies
// it: item upserts, new versions, removals and events, all in one
// transaction so the notifier only ever sees complete runs. Returns the run
// id.
func (s *Store) RecordRun(ctx context.Context, run ScrapeRun, plan *bbb.Plan) (runID int64, err error) {
	defer wrap(&err, "record run")

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback(ctx)
		}
	}()

	err = tx.QueryRow(ctx, `
		INSERT INTO scrape_runs
			(started_at, finished_at, http_status, body_hash, unchanged, item_count, suspect, error)
		VALUES ($1, $2, nullif($3, 0), nullif($4, ''), $5, $6, $7, nullif($8, ''))
		RETURNING id`,
		run.StartedAt, run.FinishedAt, run.HTTPStatus, run.BodyHash, run.Unchanged,
		run.ItemCount, run.Suspect, run.Error,
	).Scan(&runID)
	if err != nil {
		return 0, err
	}

	if plan != nil && !plan.Suspect {
		if err := applyPlan(ctx, tx, runID, run.StartedAt, plan); err != nil {
			return 0, err
		}
	}
	return runID, tx.Commit(ctx)
}

func applyPlan(ctx context.Context, tx pgx.Tx, runID int64, now time.Time, plan *bbb.Plan) error {
	newVersion := map[int64]int64{}
	for _, u := range plan.Upserts {
		var deadline *time.Time
		if u.Version.Deadline != nil {
			d := u.Version.Deadline.Time()
			deadline = &d
		}
		_, err := tx.Exec(ctx, `
			INSERT INTO items (panel_id, division, position, first_seen_at, last_seen_at, is_signup, deadline)
			VALUES ($1, $2, $3, $4, $4, $5, $6)
			ON CONFLICT (panel_id) DO UPDATE SET
				division = excluded.division,
				position = excluded.position,
				last_seen_at = excluded.last_seen_at,
				removed_at = NULL,
				is_signup = excluded.is_signup,
				deadline = excluded.deadline`,
			u.Item.PanelID, string(u.Item.Division), u.Item.Position, now, u.Version.IsSignup, deadline)
		if err != nil {
			return fmt.Errorf("upsert item %d: %w", u.Item.PanelID, err)
		}
		if !u.NewVersion {
			continue
		}

		links := u.Version.SignupLinks
		if links == nil {
			links = []string{}
		}
		var vid int64
		err = tx.QueryRow(ctx, `
			INSERT INTO item_versions
				(panel_id, seen_at, title, subheader, body_html, body_text, content_hash, signup_links)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
			RETURNING id`,
			u.Item.PanelID, now, u.Version.Title, u.Version.Subheader, u.Version.BodyHTML,
			u.Version.BodyText, u.Version.ContentHash, links,
		).Scan(&vid)
		if err != nil {
			return fmt.Errorf("insert version of %d: %w", u.Item.PanelID, err)
		}
		if _, err := tx.Exec(ctx, `UPDATE items SET current_version_id = $2 WHERE panel_id = $1`,
			u.Item.PanelID, vid); err != nil {
			return fmt.Errorf("set current version of %d: %w", u.Item.PanelID, err)
		}
		newVersion[u.Item.PanelID] = vid
	}

	if len(plan.Removals) > 0 {
		if _, err := tx.Exec(ctx, `UPDATE items SET removed_at = $2 WHERE panel_id = ANY($1)`,
			plan.Removals, now); err != nil {
			return fmt.Errorf("mark removed: %w", err)
		}
	}

	for _, e := range plan.Events {
		var from, to *int64
		if e.FromVersionID != 0 {
			from = &e.FromVersionID
		}
		switch e.Kind {
		case bbb.EventNew, bbb.EventEdit:
			v := newVersion[e.PanelID]
			to = &v
		case bbb.EventMoved:
			v, ok := newVersion[e.PanelID]
			if !ok {
				v = e.FromVersionID
			}
			to = &v
		case bbb.EventRemoved:
		}
		var repost *int64
		if e.RepostOf != 0 {
			repost = &e.RepostOf
		}
		_, err := tx.Exec(ctx, `
			INSERT INTO events
				(run_id, panel_id, kind, from_version_id, to_version_id, repost_of,
				 became_signup, words_added, words_removed, created_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)`,
			runID, e.PanelID, string(e.Kind), from, to, repost,
			e.BecameSignup, e.WordsAdded, e.WordsRemoved, now)
		if err != nil {
			return fmt.Errorf("insert %s event for %d: %w", e.Kind, e.PanelID, err)
		}
	}
	return nil
}

// wrap annotates *errp with the operation name. Use as:
// defer wrap(&err, "load state").
func wrap(errp *error, op string) {
	if *errp != nil {
		*errp = fmt.Errorf("%s: %w", op, *errp)
	}
}
