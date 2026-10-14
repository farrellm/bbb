package store

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

// User is an account. Accounts come from Google sign-in; there are no
// passwords.
type User struct {
	ID          int64
	Email       string
	Name        string
	PictureURL  string
	IsAdmin     bool
	CreatedAt   time.Time
	LastLoginAt *time.Time
}

// Sign-in failures a user can fix with a (new) invite link.
var (
	ErrInviteRequired = errors.New("invite required")
	ErrInviteInvalid  = errors.New("invite invalid")
)

// SignIn is a verified Google identity arriving at the callback.
type SignIn struct {
	Subject string
	Email   string
	Name    string
	Picture string
	// Invite is the code the user arrived with, if any. Only a new account
	// uses it up.
	Invite string
	// Admin is true when the email is in ADMIN_EMAILS: the account becomes
	// an admin and needs no invite.
	Admin bool
}

const userCols = `id, email, name, picture_url, is_admin, created_at, last_login_at`

func scanUser(row pgx.Row) (User, error) {
	var u User
	err := row.Scan(&u.ID, &u.Email, &u.Name, &u.PictureURL, &u.IsAdmin, &u.CreatedAt, &u.LastLoginAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return u, ErrNotFound
	}
	return u, err
}

// GetUser returns the user with the given id.
func (s *Store) GetUser(ctx context.Context, id int64) (User, error) {
	return scanUser(s.pool.QueryRow(ctx, `SELECT `+userCols+` FROM users WHERE id = $1`, id))
}

// SignInUser finds the account for a Google subject, refreshing its profile,
// or creates one. Creating needs a usable invite, which is consumed in the
// same transaction, unless in.Admin is set.
func (s *Store) SignInUser(ctx context.Context, in SignIn) (u User, err error) {
	defer wrap(&err, "sign in")

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return u, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	u, err = scanUser(tx.QueryRow(ctx, `
		UPDATE users SET email = $2, name = $3, picture_url = $4,
			is_admin = is_admin OR $5, last_login_at = now()
		WHERE google_sub = $1
		RETURNING `+userCols,
		in.Subject, in.Email, in.Name, in.Picture, in.Admin))
	if errors.Is(err, ErrNotFound) {
		u, err = createUser(ctx, tx, in)
	}
	if err != nil {
		return u, err
	}
	return u, tx.Commit(ctx)
}

func createUser(ctx context.Context, tx pgx.Tx, in SignIn) (User, error) {
	var createdBy *int64
	if !in.Admin {
		if in.Invite == "" {
			return User{}, ErrInviteRequired
		}
		err := tx.QueryRow(ctx, `
			UPDATE invites SET uses = uses + 1
			WHERE code = $1 AND NOT revoked AND uses < max_uses
				AND (expires_at IS NULL OR expires_at > now())
			RETURNING created_by`, in.Invite).Scan(&createdBy)
		if errors.Is(err, pgx.ErrNoRows) {
			return User{}, ErrInviteInvalid
		}
		if err != nil {
			return User{}, fmt.Errorf("use invite: %w", err)
		}
	}
	u, err := scanUser(tx.QueryRow(ctx, `
		INSERT INTO users (google_sub, email, name, picture_url, is_admin, last_login_at)
		VALUES ($1, $2, $3, $4, $5, now())
		RETURNING `+userCols,
		in.Subject, in.Email, in.Name, in.Picture, in.Admin))
	if err != nil {
		return u, err
	}
	// Every account has a preferences row, starting from the defaults.
	if _, err := tx.Exec(ctx, `INSERT INTO preferences (user_id) VALUES ($1)`, u.ID); err != nil {
		return u, fmt.Errorf("create preferences: %w", err)
	}
	return u, nil
}

// ListUsers returns every account, oldest first.
func (s *Store) ListUsers(ctx context.Context) ([]User, error) {
	rows, err := s.pool.Query(ctx, `SELECT `+userCols+` FROM users ORDER BY id`)
	if err != nil {
		return nil, fmt.Errorf("list users: %w", err)
	}
	users, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (User, error) { return scanUser(r) })
	if err != nil {
		return nil, fmt.Errorf("list users: %w", err)
	}
	return users, nil
}

// DeleteUser deletes the account with the given email, with its
// subscriptions, preferences and read marks.
func (s *Store) DeleteUser(ctx context.Context, email string) error {
	tag, err := s.pool.Exec(ctx, `DELETE FROM users WHERE lower(email) = $1`, strings.ToLower(email))
	if err != nil {
		return fmt.Errorf("delete user: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// MakeAdmin grants admin to the account with the given email.
func (s *Store) MakeAdmin(ctx context.Context, email string) error {
	tag, err := s.pool.Exec(ctx, `UPDATE users SET is_admin = true WHERE lower(email) = $1`,
		strings.ToLower(email))
	if err != nil {
		return fmt.Errorf("make admin: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}
