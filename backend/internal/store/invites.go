package store

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

// Invite lets someone without an account create one.
type Invite struct {
	Code      string
	MaxUses   int
	Uses      int
	ExpiresAt *time.Time
	CreatedAt time.Time
}

// CreateInvite stores a new invite. createdBy is 0 for invites made from
// the command line.
func (s *Store) CreateInvite(ctx context.Context, code string, createdBy int64, maxUses int, expiresAt *time.Time) (Invite, error) {
	var by *int64
	if createdBy != 0 {
		by = &createdBy
	}
	inv := Invite{Code: code}
	err := s.pool.QueryRow(ctx, `
		INSERT INTO invites (code, created_by, max_uses, expires_at)
		VALUES ($1, $2, $3, $4)
		RETURNING max_uses, uses, expires_at, created_at`,
		code, by, maxUses, expiresAt,
	).Scan(&inv.MaxUses, &inv.Uses, &inv.ExpiresAt, &inv.CreatedAt)
	if err != nil {
		return inv, fmt.Errorf("create invite: %w", err)
	}
	return inv, nil
}

// InviteUsable reports whether code can still create an account.
func (s *Store) InviteUsable(ctx context.Context, code string) (bool, error) {
	var ok bool
	err := s.pool.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM invites
			WHERE code = $1 AND NOT revoked AND uses < max_uses
				AND (expires_at IS NULL OR expires_at > now()))`, code).Scan(&ok)
	if err != nil {
		return false, fmt.Errorf("check invite: %w", err)
	}
	return ok, nil
}

// OpenInvites lists the invites that can still create an account, newest
// first.
func (s *Store) OpenInvites(ctx context.Context) ([]Invite, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT code, max_uses, uses, expires_at, created_at
		FROM invites
		WHERE NOT revoked AND uses < max_uses AND (expires_at IS NULL OR expires_at > now())
		ORDER BY created_at DESC, code`)
	if err != nil {
		return nil, fmt.Errorf("open invites: %w", err)
	}
	invs, err := pgx.CollectRows(rows, pgx.RowToStructByPos[Invite])
	if err != nil {
		return nil, fmt.Errorf("open invites: %w", err)
	}
	return invs, nil
}

// RevokeInvite stops an invite admitting anyone else. Accounts it already
// made are kept.
func (s *Store) RevokeInvite(ctx context.Context, code string) error {
	tag, err := s.pool.Exec(ctx, `UPDATE invites SET revoked = true WHERE code = $1`, code)
	if err != nil {
		return fmt.Errorf("revoke invite: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}
