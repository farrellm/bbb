# BBB Watch

Watches Pingry's [Big Blue Bulletin](https://www.pingry.org/experience/community-life/bbb)
page, keeps every version of every item, tells new items apart from edits,
and shows invited parents what changed in an installable web app. Push
notifications are next. Unofficial; it reads
the public page.

See [docs/DESIGN.md](docs/DESIGN.md) for the design and milestones.

## Status

Milestones 1 to 3 are done:

- **Scraper core:** the poller fetches the page, parses the four divisions,
  normalizes each item, detects signups and deadlines, and records
  new/edit/moved/removed events in Postgres.
- **Read-only web app:** Google sign-in with invites (plus the iOS
  home-screen handoff), the feed with unread marks and signup flyers, item
  history, the timeline, and word-level diffs.
- **Push:** Web Push notifications through a service worker, filtered by
  each parent's preferences, batched when a run brings many changes, with a
  test button in Settings.

## Running locally

```sh
make tools        # pinned linters, npm packages and git hooks
make db migrate   # Postgres 17 in Docker on localhost:5435
make scrape-once  # poll the page once
make backend      # API on 127.0.0.1:8094, polling every 5 minutes
make frontend     # Vite on http://localhost:5173, proxying /api
```

Sign-in needs a Google OAuth client (see `.envrc.example`) with
`http://localhost:5173/api/auth/google/callback` as a redirect URI. New
accounts need an invite unless their email is in `ADMIN_EMAILS`:

```sh
cd backend && go run ./cmd/server -create-invite   # prints a /join link
go run ./cmd/server -list-users
```

Notifications need a VAPID key pair in `.envrc`; generate it once (a new
pair invalidates every device's subscription):

```sh
cd backend && go run ./cmd/server -gen-vapid
```

## Deployment

The deployed instance is a systemd user unit, `deploy/bbb.service`, running
one process for the poller, the API and the built app on `127.0.0.1:8093`.
The only way in is Tailscale Funnel, which publishes it at
`https://<host>.<tailnet>.ts.net/` (Funnel allows only ports 443, 8443
and 10000).

```sh
make install-service   # once: link and enable the unit, so it starts at boot
make deploy            # build backend and frontend, restart the unit
make logs              # follow its logs
tailscale funnel status
```

- `scripts/run-production` starts the server. It sources `.envrc` for the
  secrets (`SESSION_SECRET`, the Google client, `ADMIN_EMAILS`), then sets
  the production listen address, `WEB_DIR`, and `PUBLIC_URL` from the
  machine's tailnet name.
- Before each start, the unit brings up Postgres and runs migrations. It
  sets up the Funnel mapping after the server starts and removes it when
  the server stops. If Funnel isn't enabled for the tailnet, that step times
  out instead of blocking.
- Starting at boot without a login needs `loginctl enable-linger $USER`.
- Add `https://<host>.<tailnet>.ts.net/api/auth/google/callback` to the
  Google OAuth client's redirect URIs.
- The dev server listens on 8094, so `make backend` can run alongside the
  service. Both use the same database, though, so run the dev server with
  `-no-poll` while the service is up.

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
