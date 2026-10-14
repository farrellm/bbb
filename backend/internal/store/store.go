// Package store holds the data-access layer: SQL over the pgx pool. Methods
// hang off Store so callers depend on one value rather than the raw pool.
package store

import (
	"github.com/jackc/pgx/v5/pgxpool"
)

// Store provides data-access methods backed by a pgx connection pool.
type Store struct {
	pool *pgxpool.Pool
}

// New returns a Store backed by the given pool.
func New(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool}
}
