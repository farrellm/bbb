# BBB Watch

Watches Pingry's [Big Blue Bulletin](https://www.pingry.org/experience/community-life/bbb)
page, keeps every version of every item, tells new items apart from edits,
and shows invited parents what changed in an installable web app. Push
notifications are next. Unofficial; it reads
the public page.

See [docs/DESIGN.md](docs/DESIGN.md) for the design and milestones.

## Status

Milestones 1 and 2 are done:

- **Scraper core:** the poller fetches the page, parses the four divisions,
  normalizes each item, detects signups and deadlines, and records
  new/edit/moved/removed events in Postgres.
- **Read-only web app:** Google sign-in with invites (plus the iOS
  home-screen handoff), the feed with unread marks and signup flyers, item
  history, the timeline, and word-level diffs.

## Running locally

```sh
make tools        # pinned linters, npm packages and git hooks
make db migrate   # Postgres 17 in Docker on localhost:5435
make scrape-once  # poll the page once
make backend      # API on 127.0.0.1:8093, polling every 5 minutes
make frontend     # Vite on http://localhost:5173, proxying /api
```

Sign-in needs a Google OAuth client (see `.envrc.example`) with
`http://localhost:5173/api/auth/google/callback` as a redirect URI. New
accounts need an invite unless their email is in `ADMIN_EMAILS`:

```sh
cd backend && go run ./cmd/server -create-invite   # prints a /join link
go run ./cmd/server -list-users
```

In production one process serves both: build with `make frontend-build` and
set `WEB_DIR=../frontend/dist`.

The first run records a baseline without events, so a fresh database doesn't
report every item as new.

## Checks

```sh
make fmt    # format Go, TS, JSON, YAML, Markdown, SQL and shell
make check  # format check, lint, tests, API types check, secret scan; CI runs exactly this
```

The API is described in `backend/api/openapi.yaml`. The handler tests check
every response against it, and the frontend's types are generated from it
with `make gen-api`; `make check` fails if they are stale.

Database tests run in a throwaway schema of the docker-compose database
(`BBB_TEST_DATABASE_URL` overrides it) and skip when it isn't set.

## Fixtures

Raw page snapshots contain names and email addresses, so they live only in
the gitignored `docs/fixtures/`. The committed test page,
`backend/internal/bbb/testdata/bbb-sample.html`, is generated from the newest
snapshot by `scripts/sanitize-fixture`, which replaces the text with
placeholder words. After regenerating it, update the golden file with
`cd backend && go test ./internal/bbb -update` and review the diff.
