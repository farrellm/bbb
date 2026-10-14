package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

// HandoffTTL is how long an iOS sign-in handoff stays claimable.
const HandoffTTL = 10 * time.Minute

// handoffTTLSeconds is HandoffTTL as a query parameter.
const handoffTTLSeconds = int(HandoffTTL / time.Second)

// HandoffState is where a handoff stands when the app polls it.
type HandoffState int

// The handoff states.
const (
	HandoffMissing HandoffState = iota // unknown, expired or already claimed
	HandoffPending                     // sign-in not finished yet
	HandoffDone                        // claimed now; the caller gets the session
)

// CreateHandoff stores a new, pending handoff and clears out expired ones.
func (s *Store) CreateHandoff(ctx context.Context, nonce string) error {
	if _, err := s.pool.Exec(ctx, `DELETE FROM login_handoffs WHERE created_at < now() - $1 * interval '1 second'`,
		handoffTTLSeconds); err != nil {
		return fmt.Errorf("expire handoffs: %w", err)
	}
	if _, err := s.pool.Exec(ctx, `INSERT INTO login_handoffs (nonce) VALUES ($1)`, nonce); err != nil {
		return fmt.Errorf("create handoff: %w", err)
	}
	return nil
}

// CompleteHandoff records that userID signed in for the handoff. It reports
// false when the handoff is unknown, expired or already complete.
func (s *Store) CompleteHandoff(ctx context.Context, nonce string, userID int64) (bool, error) {
	tag, err := s.pool.Exec(ctx, `
		UPDATE login_handoffs SET user_id = $2, completed_at = now()
		WHERE nonce = $1 AND completed_at IS NULL AND created_at >= now() - $3 * interval '1 second'`,
		nonce, userID, handoffTTLSeconds)
	if err != nil {
		return false, fmt.Errorf("complete handoff: %w", err)
	}
	return tag.RowsAffected() == 1, nil
}

// ClaimHandoff checks a handoff and, once it is complete, deletes it and
// returns its user, so it can only be claimed once.
func (s *Store) ClaimHandoff(ctx context.Context, nonce string) (int64, HandoffState, error) {
	var userID int64
	err := s.pool.QueryRow(ctx, `
		DELETE FROM login_handoffs
		WHERE nonce = $1 AND completed_at IS NOT NULL AND created_at >= now() - $2 * interval '1 second'
		RETURNING user_id`, nonce, handoffTTLSeconds).Scan(&userID)
	if err == nil {
		return userID, HandoffDone, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return 0, HandoffMissing, fmt.Errorf("claim handoff: %w", err)
	}

	var pending bool
	err = s.pool.QueryRow(ctx, `
		SELECT EXISTS (SELECT 1 FROM login_handoffs
			WHERE nonce = $1 AND completed_at IS NULL AND created_at >= now() - $2 * interval '1 second')`,
		nonce, handoffTTLSeconds).Scan(&pending)
	if err != nil {
		return 0, HandoffMissing, fmt.Errorf("check handoff: %w", err)
	}
	if pending {
		return 0, HandoffPending, nil
	}
	return 0, HandoffMissing, nil
}
