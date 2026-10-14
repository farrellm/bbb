# Pingry BBB Watch

Design and milestones: `docs/DESIGN.md`. Go backend in `backend/`, React PWA
in `frontend/`, migrations in `db/migrations/`, scripts in `scripts/`. The
default branch is `master`.

## Before calling a change done

- `make fmt`, then `make check` (needs `make tools` once, and `make db migrate`
  for the database tests).
- API changes: edit `backend/api/openapi.yaml` and
  `backend/internal/handler/types.go` together, then `make gen-api`. Handler
  tests fail on responses that don't match the spec.
- Parser or normalization changes move the golden file: run
  `cd backend && go test ./internal/bbb -update` and check the diff is intended.

## Conventions

- Never commit anything from `docs/fixtures/`; raw snapshots contain personal
  data and the repo is public. Regenerate the sample with
  `scripts/sanitize-fixture`.
- `backend/internal/bbb/worddiff.go` ports nisaba's `wordDiff.ts`, and
  `frontend/src/lib/wordDiff.{ts,md}` are byte-for-byte copies of nisaba's
  (Prettier skips them). Keep all three in sync; the Go and vitest suites
  share cases.
- Config is environment variables only (see `.envrc.example`), with local-dev
  defaults.
