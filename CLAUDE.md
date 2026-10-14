# BBB Watch

Design and milestones: `docs/DESIGN.md`. Go backend in `backend/`, migrations
in `db/migrations/`, scripts in `scripts/`. The default branch is `master`.

## Before calling a change done

- `make fmt`, then `make check` (needs `make tools` once, and `make db migrate`
  for the database tests).
- Parser or normalization changes move the golden file: run
  `cd backend && go test ./internal/bbb -update` and check the diff is intended.

## Conventions

- Never commit anything from `docs/fixtures/`; raw snapshots contain personal
  data and the repo is public. Regenerate the sample with
  `scripts/sanitize-fixture`.
- `backend/internal/bbb/worddiff.go` ports nisaba's `wordDiff.ts`; the
  frontend will copy that file as-is. Keep the two in sync.
- Config is environment variables only (see `.envrc.example`), with local-dev
  defaults.
