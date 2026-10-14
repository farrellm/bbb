// Package db opens the Postgres connection pool.
package db

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Connect opens a pool and pings it to fail fast.
func Connect(ctx context.Context, url string) (*pgxpool.Pool, error) {
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		return nil, err
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, err
	}
	return pool, nil
}
