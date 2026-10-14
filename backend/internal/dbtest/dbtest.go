// Package dbtest gives tests a migrated, throwaway Postgres schema.
package dbtest

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// EnvVar names the database tests run against. Tests that need it skip when
// it is unset; `make test` sets it to the docker-compose database.
const EnvVar = "BBB_TEST_DATABASE_URL"

// New creates a schema with a random name, applies every up migration in
// it, and returns a pool whose connections use only that schema. The schema
// is dropped when the test ends, so tests never touch real data.
func New(t *testing.T) *pgxpool.Pool {
	t.Helper()
	url := os.Getenv(EnvVar)
	if url == "" {
		t.Skipf("%s not set", EnvVar)
	}
	ctx := context.Background()

	buf := make([]byte, 6)
	_, _ = rand.Read(buf)
	schema := "test_" + hex.EncodeToString(buf)

	admin, err := pgx.Connect(ctx, url)
	if err != nil {
		t.Fatalf("connect %s: %v", EnvVar, err)
	}
	defer admin.Close(ctx)
	if _, err := admin.Exec(ctx, "CREATE SCHEMA "+schema); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		c, err := pgx.Connect(context.Background(), url)
		if err != nil {
			t.Errorf("cleanup connect: %v", err)
			return
		}
		defer c.Close(context.Background())
		if _, err := c.Exec(context.Background(), "DROP SCHEMA "+schema+" CASCADE"); err != nil {
			t.Errorf("drop schema: %v", err)
		}
	})

	cfg, err := pgxpool.ParseConfig(url)
	if err != nil {
		t.Fatal(err)
	}
	cfg.ConnConfig.RuntimeParams["search_path"] = schema
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)

	for _, f := range upMigrations(t) {
		sql, err := os.ReadFile(f) //nolint:gosec // G304: paths come from the repo's migrations dir
		if err != nil {
			t.Fatal(err)
		}
		if _, err := pool.Exec(ctx, string(sql)); err != nil {
			t.Fatalf("migrate %s: %v", filepath.Base(f), err)
		}
	}
	return pool
}

func upMigrations(t *testing.T) []string {
	t.Helper()
	_, here, _, _ := runtime.Caller(0)
	dir := filepath.Join(filepath.Dir(here), "..", "..", "..", "db", "migrations")
	files, err := filepath.Glob(filepath.Join(dir, "*.up.sql"))
	if err != nil || len(files) == 0 {
		t.Fatalf("no migrations in %s", dir)
	}
	sort.Strings(files)
	for _, f := range files {
		if !strings.HasSuffix(f, ".up.sql") {
			t.Fatalf("unexpected migration %s", f)
		}
	}
	return files
}
