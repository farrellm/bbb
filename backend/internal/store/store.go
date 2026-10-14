// Package store holds the data-access layer: SQL over the pgx pool. Methods
// hang off Store so callers depend on one value rather than the raw pool.
package store

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5/pgxpool"
)

// ErrNotFound means the requested row does not exist.
var ErrNotFound = errors.New("not found")

// Store provides data-access methods backed by a pgx connection pool.
type Store struct {
	pool *pgxpool.Pool
}

// New returns a Store backed by the given pool.
func New(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool}
}

// Ping checks the database answers, for the health check.
func (s *Store) Ping(ctx context.Context) error {
	return s.pool.Ping(ctx)
}
