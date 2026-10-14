# BBB Watch

Watches Pingry's [Big Blue Bulletin](https://www.pingry.org/experience/community-life/bbb)
page, keeps every version of every item, tells new items apart from edits,
and (soon) sends push notifications to invited parents. Unofficial; it reads
the public page.

See [docs/DESIGN.md](docs/DESIGN.md) for the design and milestones.

## Status

Milestone 1, the scraper core, is done: the poller fetches the page, parses
the four divisions, normalizes each item, detects signups and deadlines, and
records new/edit/moved/removed events in Postgres. There is no web app yet.

## Running locally

```sh
make tools        # pinned linters and git hooks, into ./bin
make db migrate   # Postgres 17 in Docker on localhost:5435
make scrape-once  # poll the page once
make backend      # poll every 5 minutes
```

The first run records a baseline without events, so a fresh database doesn't
report every item as new.

## Checks

```sh
make fmt    # format Go, SQL and shell
make check  # format check, lint, tests, secret scan; CI runs exactly this
```

Database tests run in a throwaway schema of the docker-compose database
(`BBB_TEST_DATABASE_URL` overrides it) and skip when it isn't set.

## Fixtures

Raw page snapshots contain names and email addresses, so they live only in
the gitignored `docs/fixtures/`. The committed test page,
`backend/internal/bbb/testdata/bbb-sample.html`, is generated from the newest
snapshot by `scripts/sanitize-fixture`, which replaces the text with
placeholder words. After regenerating it, update the golden file with
`cd backend && go test ./internal/bbb -update` and review the diff.
