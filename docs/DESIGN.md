# Pingry BBB Watch: design

Status: 2026-10-14. All four milestones are implemented: 1 (scraper core), 2 (read-only web app), 3 (push) and 4 (hardening).

## Context

Pingry posts parent announcements on the "Big Blue Bulletin" (BBB) page at
https://www.pingry.org/experience/community-life/bbb. Items get added and quietly edited in place (a date changes, a signup link is added), and there is no feed or notification of any kind.
Pingry BBB Watch polls the page, keeps every version of every item, tells **new** items apart from **edits**, and sends iPhone push notifications through an installable PWA. Each parent chooses which divisions notify them.
Signup and RSVP items are highlighted so they're hard to miss.
Users are invited parents, not just one family, so the doc needs accounts and per-user preferences.

## Source page facts (verified 2026-10-14)

- Finalsite CMS behind Cloudflare. The page is **fully rendered on the server**, about 134 KB, with no JavaScript, iframe or RSS needed.
  The response sends `cache-control: s-maxage=300` but no ETag or Last-Modified, so the content has to be hashed to detect changes.
- There are four divisions, each marked by an anchor: `#allschool`, `#lowerschool`, `#middleschool`, `#upperschool`.
  Each anchor's `div.fsContent` is followed by a `div.fsPanelGroup.fsAccordion`.
- Every item is a `section.fsPanel#fsEl_<N>`, and **`N` is a stable item ID** (it also shows up as `#fs-panel-<N>` and `fsTab-<N>`). Inside each:
  - Title: `h2.fsElementTitle a`
  - Subheader, usually the date, time, place or "Deadline: …": `.fsElementHeaderContent`
  - Body HTML: `.fsElementContent .fsElement.fsContent .fsElementContent`
- There were 41 items on the snapshot date (17 All School, 10 Lower, 6 Middle, 8 Upper). Items have no author or published date.
- Signup links seen on the page: SignUpGenius, Google Forms, Veracross, Punchbowl, CampBrain, Helper Helper, Clarity and cur8.

## Stack (mirrors `../nisaba`)

| Layer | Choice | Reuse from nisaba |
|---|---|---|
| Backend | Go, chi, pgx, goquery (HTML parsing), `SherClockHolmes/webpush-go` | Layout of `backend/cmd/server`, `internal/{config,db,handler,store,auth}`; env-var config without `.env`; `-create-user` style CLI flags |
| DB | Postgres 17 via docker-compose, golang-migrate in `db/migrations/` | `docker-compose.yml`, Makefile targets (`db`, `migrate`, `backend`, `frontend`, `deploy`) |
| Frontend | React, TypeScript, Vite, MUI, react-router, plus `vite-plugin-pwa` (Workbox) | `theme.ts` fonts and palette (`palette.struck`), `wordDiff.ts`, the diff rendering from `BlockAttributeDiffPage.tsx` |
| Auth | **Sign in with Google** (OIDC via `golang.org/x/oauth2` and `coreos/go-oidc`), then a signed session cookie (gorilla/sessions) | `backend/internal/auth/session.go` (Save/UserID/Clear) and `handler/auth.go` (Logout, Me); session lifetime raised from 1 week to about 1 year (see iOS notes) |
| Deploy | One Go process serves the SPA (`WEB_DIR`) and `/api` on 127.0.0.1:8093, run as a systemd user unit and exposed through **Tailscale Funnel on HTTPS port 443** (`tailscale funnel --bg --https=443 8093`) | nisaba's `deploy` target and unit file. Public URL: `https://<host>.<tailnet>.ts.net/`, which is also the OAuth redirect origin |

## Architecture

```
          ┌──────────── Go server (single process) ────────────┐
pingry ──▶│ Poller (ticker, 5 min) → Parser → Differ → Events   │
 .org     │                                   │                 │
          │                          Notifier (fan-out, web push)──▶ Apple/FCM push svc ──▶ iPhone
          │ HTTP: /api/*  +  static SPA + sw.js + manifest      │
          └──────────────────────┬──────────────────────────────┘
                                 ▼
                             Postgres
```

### 1. Poller
- Polls every 5 minutes by default (`POLL_INTERVAL`), with ±30 s of jitter. It doesn't poll faster, because Cloudflare caches the page for 300 s anyway.
  An optional overnight back-off of 30 minutes between 11pm and 6am can be configured.
- Requests use an honest User-Agent: `BBBWatch/1.0 (+contact email)`. Timeout is 20 s, with exponential back-off when errors occur.
- The SHA-256 of the raw body is stored in `scrape_runs`. If it matches the previous run's hash, parsing is skipped.

### 2. Parser (`internal/bbb/parse.go`)
- Walks the four division anchors in document order. For each, it takes the next `fsPanelGroup` and reads every `section.fsPanel` in it.
- It produces one `Item` per panel: `{panelID, division, position, title, subheader, bodyHTML}`.
- **Normalization** produces the text that gets diffed and hashed:
  - Convert HTML to readable text, keeping links as `text <url>` so a link change counts as an edit.
  - Keep list bullets as `• `.
  - Map `&nbsp;` to a space, collapse runs of spaces and tabs, trim lines, and drop empty `<p>`.
  - Then `content_hash = sha256(title \n subheader \n body_text)`.
- **Sanity guard:** a run is marked `suspect` and does nothing (no removals, no events) if any of these is true:
  - zero items were parsed;
  - a division is missing;
  - the item count dropped by more than half compared with the last good run.
  If three suspect runs happen in a row, the admin gets a push.

### 3. Differ
The parsed items are compared with the current state in the DB, matched on `panel_id`:

| Case | Event |
|---|---|
| `panel_id` not seen before | `new` |
| Same `panel_id`, different `content_hash` | `edit` (from_version → to_version) |
| Same `panel_id`, different division | `moved` (stored but doesn't notify by default) |
| Seen before, absent now (only on a non-suspect run) | `removed` (sets `removed_at`; reappearing later with different content counts as an `edit` against the last version, and with the same content just clears `removed_at`) |
| Position change only | Ignored (position is updated but no event) |

- **Repost heuristic:** if a `new` item has the same normalized title as an item removed in the last 14 days, the event gets `repost_of`. The UI can then offer "compare with previous posting".
- Each run's events are written in one transaction along with the new `item_versions` rows. The notifier only reads events that have been committed.
- **Baseline:** the first run against an empty database stores items and versions but no events, so a fresh deploy doesn't announce all 41 items as new.

### 4. Signup detection (`internal/bbb/signup.go`)
- An item `is_signup` if either check matches:
  - **Link hosts:** a body link points to a known host: `signupgenius.com`, `docs.google.com/forms`, `forms.gle`, `punchbowl.com`, `events.veracross.com`, `campbrain.com`, `campbrainregistration.com`, `helperhelper.com`, `cur8`, `clarityapp.com`, `eventbrite`, `jotform`.
    This list lives in config so it can be extended (`SIGNUP_HOSTS_EXTRA`).
    Only `events.veracross.com` counts: `portals.veracross.com` links are archived emails, not forms.
  - **Keywords:** a regex over the title and subheader: `sign[\s-]?up|register|registration|rsvp|volunteer|tickets?|apply|application`.
- The matched URLs are saved as `signup_links[]`, so the UI can show a **Sign up** button that goes straight to the form.
- **Deadline extraction** (best effort): `Deadline:\s*(.+)`, or the first date in the subheader, parsed with the school-year context so the year can be inferred. It is stored as `deadline date NULL`.
- Detection is recomputed for every version. If an edit **adds** a signup link, the event gets `became_signup=true`, which counts as notification-worthy.

### 5. Notifier
- Uses Web Push with VAPID keys (`VAPID_PUBLIC`, `VAPID_PRIVATE`, `VAPID_SUBJECT=mailto:…`).
- For each committed event, it picks the users whose preferences match, then every push subscription each of those users has.
- **Batching:** if one run produces more than 3 matching events for a user, it sends one summary push, for example "5 BBB updates (3 new, 2 edited)", which opens the feed.
- **Payload:** `{title, body, url, tag}`, for example:
  - `title: "New in Lower School"`, `body: "Lunchroom Volunteer Program. Sign-up, due Oct 20"`
  - `title: "Edited in All School"`, `body: "Friday Night Lights: 12 words added, 4 removed"`
  - `tag = "item-<panelID>"`, so a later edit replaces the earlier notification instead of stacking.
- **Retries:** `404` or `410` deletes the subscription; `429` or `5xx` retries with back-off. Each attempt is recorded in `deliveries`.
- **Edit noise filter:** the server computes the word delta with a Go port of the token rule in `wordDiff`. Edits under `min_edit_words` (default 1) don't notify; this covers whitespace and punctuation-only changes, which normalization mostly removes already.

## Data model (Postgres)

```
users(id, google_sub unique, email, name, picture_url, is_admin, created_at, last_login_at)
invites(code pk, created_by, max_uses, uses, expires_at, revoked bool)
login_handoffs(nonce pk, user_id null, created_at, completed_at null)  -- iOS PWA OAuth handoff, 10-min TTL
push_subscriptions(id, user_id, endpoint unique, p256dh, auth, user_agent, created_at, last_ok_at)
preferences(user_id pk,
            divisions text[]  default '{all,lower,middle,upper}',
            notify_new bool default true, notify_edit bool default true,
            notify_removed bool default false,
            signups_only bool default false,     -- only notify for is_signup items
            signups_any_division bool default false, -- if true, signups notify even outside chosen divisions
            min_edit_words int default 1,
            quiet_start time null, quiet_end time null)  -- pushes held, then sent as summary
items(panel_id pk, division, position, current_version_id null, first_seen_at, last_seen_at,
      removed_at null, is_signup bool, deadline date null)
item_versions(id, panel_id, seen_at, title, subheader, body_html, body_text, content_hash,
              signup_links text[])
events(id, run_id, panel_id, kind enum(new,edit,moved,removed), from_version_id null, to_version_id null,
       repost_of null, became_signup bool, words_added int, words_removed int, created_at)
deliveries(event_id, subscription_id, status, attempted_at, error)
scrape_runs(id, started_at, finished_at, http_status, body_hash, unchanged bool, item_count, suspect bool, error)
reads(user_id, event_id, read_at)  -- unread badges
```

## API (all routes under `/api`; everything except auth, invite and VAPID routes needs a session)

- `GET /auth/google/start?invite=&handoff=`: sets a signed state cookie (state, PKCE verifier, nonce, invite, handoff), then redirects to Google.
- `GET /auth/google/callback`: exchanges the code and verifies the ID token (`aud`, `iss`, nonce, `email_verified`).
  - It finds the user by `sub`. If there is none, it requires a valid invite and creates the user, using up one invite use.
  - It sets the session and, if `handoff` is present, marks that handoff complete.
- `POST /auth/handoff` returns `{nonce}`. `GET /auth/handoff/{nonce}` sets the session cookie once the handoff is complete and is single-use.
- `POST /auth/logout`, `GET /me` (from nisaba).
- `GET /push/vapid-public-key`, `POST /push/subscribe`, `DELETE /push/subscribe`, `POST /push/test`.
- `GET|PUT /preferences`.
- `GET /items?division=&signup=&include_removed=`: current items with `latest_event` and `unread`.
- `GET /items/{panelID}`: the item plus all its versions (metadata only).
- `GET /items/{panelID}/versions/{id}`: the full text of one version.
- `GET /events?before=&limit=`: the timeline. `POST /events/read {ids}`.
- Admin only: `POST /admin/invites`, `GET /admin/scrape-runs`. There is also a CLI: `-create-invite`, `-list-users`, `-delete-user`, `-make-admin <email>`.
  The first admin is set with the `ADMIN_EMAILS` env var.

## Frontend (PWA)

**Routes**
- `/`: the **Feed** (see Visual design for the look).
  - Division tabs (All, Lower, Middle, Upper) plus a **Sign-ups** tab, which is sorted by deadline.
  - Each item shows the title, subheader, an expandable body and a relative "first seen" time.
  - Change marks: a solid margin bar for new, an outlined bar plus "See changes" for edited, and greyed-out removed items when "Show removed" is on.
  - **Signup highlight:** a flyer surface with tear-off tabs, one per signup link plus a deadline countdown ("Due in 3 days", Struck red at 2 days or fewer).
- `/timeline`: the event stream, read and unread, so you can see what changed since you last looked.
- `/items/:panelID`: item detail with version history; any two versions can be compared.
- `/items/:panelID/diff?from=&to=`: a deep link that notifications open. It renders the same `DiffView` component as a full page.
- `/settings`:
  - Division checkboxes, toggles for new, edit and removed, "signups only", "notify signups from any division" (off by default), the minimum edit size, and quiet hours.
  - An **Enable notifications on this device** button, a "send test" button, and the list of subscribed devices with a remove button for each.
- `/login`: a "Sign in with Google" button. `/join?invite=…` is the same button with the invite code carried along.

**Diff popout.** This reuses nisaba code.
- Copy `../nisaba/frontend/src/lib/wordDiff.ts` and `wordDiff.md` as they are; keep the two in sync, as the nisaba header asks.
  Add the vitest unit tests that nisaba doesn't have.
- Pull the body rendering out of `../nisaba/frontend/src/pages/BlockAttributeDiffPage.tsx` into `components/DiffView.tsx`. It takes `before` and `after` and renders:
  - `<ins>` in primary colour with an underline;
  - `<del>` in `palette.struck` with a line-through;
  - the `−N / +M words` summary using `wordCount`.
- **Popout behaviour:** a `DiffSheet` (MUI `Dialog`, full height and sliding up from the bottom on phones), with the URL synced through `?diff=from..to`.
  nisaba opens a new tab, but in an iOS standalone PWA that leaves the app for Safari, so the dialog plus deep link replaces it.
- The diff shows three sections, **Title / Details / Body**. Each section is diffed separately, and sections that didn't change are collapsed.
- A version selector in the dialog header (prev/next arrows) steps through the item's history.

**PWA and iOS specifics**
- `manifest.webmanifest`: `display: standalone`, name, icons (192, 512, maskable), `theme_color`, plus `apple-touch-icon` in `index.html`.
- **iOS 16.4+:** push only works after **Add to Home Screen**, and the permission request has to come from a user tap.
  When `navigator.standalone` is false on iOS, Settings shows an install guide.
- **The iOS home-screen app has a separate cookie store from Safari, and Google's page opens outside the app,** so a cookie set at the OAuth callback might never reach the PWA.
  The fix is a **handoff**, used whenever the app runs standalone:
  1. The PWA calls `POST /auth/handoff` to get a nonce.
  2. It opens `/auth/google/start?handoff=<nonce>`.
  3. Sign-in finishes wherever iOS showed it, and the callback page says "Done, return to the app".
  4. Meanwhile the PWA polls `GET /auth/handoff/{nonce}`, which sets the session cookie inside the PWA's own cookie store.

  In a normal browser tab it uses the plain redirect flow. Sessions last a long time (rolling, about 1 year), so users rarely have to sign in again.
- `sw.js`, built with vite-plugin-pwa `injectManifest`:
  - precaches the app shell;
  - on `push`, calls `showNotification(title, {body, tag, data:{url}, badge})`;
  - on `notificationclick`, focuses an existing client and navigates to `data.url`, or opens a new window;
  - on `pushsubscriptionchange`, subscribes again and POSTs the new subscription.
- Sets the app badge with `navigator.setAppBadge(unreadCount)` where supported.

## Visual design

**Subject, audience and job.** The audience is Pingry parents, standing in a school pickup line with one thumb free.
The app's main job is to answer two questions in under five seconds: what changed since I last looked, and what do I need to sign up for?
The visual vocabulary comes from a school bulletin board: flyers pinned up, flyers with tear-off tabs, and red-pen corrections.

**Tokens**

| Name | Light | Dark | Role |
|---|---|---|---|
| Paper | `#F6F8FB` (cool white) | `#0F1B2E` | Page background |
| Ink | `#0B2A5B` (deep school navy) | `#E4EAF5` | Text, rules, primary actions |
| Big Blue | `#1F5FD6` | `#7FA8F5` | "New" marks, inserted words in diffs |
| Struck | `#B23A2E` (nisaba's `palette.struck`) | `#F08A7E` | Deleted words, deadlines due within 2 days |
| Flyer | `#FFF1A8` (yellow flyer stock) | `#3A3410` | Fill for signup items only |
| Rule | `#C8D2E2` | `#2A3A55` | Hairline dividers |

- **Type:** a single family, **Atkinson Hyperlegible Next** (Google Fonts), chosen because it's designed for quick, low-effort reading on a small screen.
  - Weights: 400 for body, 700 for item titles, 800 for the masthead.
  - Tabular numerals for counts and dates.
  - Scale, mobile first: 15/17 body (1.5 line-height), 17/22 item title, 13/17 secondary, 26/30 masthead.
  - The diff body uses 1.7 line-height, as in nisaba.
- **Alignment:** everything is left aligned. Measure is capped at about 66ch on wider screens, centred in a single column with no sidebars.

**Layout concept.** The feed is a single list separated by hairline rules, not a stack of identical cards.
Only signup items get a different surface, so that difference itself carries meaning.

```
┌──────────────────────────────────┐
│ Pingry BBB Watch            ⚙    │
│ Checked 4 min ago                │
│ All  Lower  Middle  Upper  Sign-ups 3 │  ← segmented, scrolls sideways
├──────────────────────────────────┤
│ ── Since you looked, Tue 3:40pm ─│  ← "last read" marker
│█ Lunchroom Volunteer Program     │  █ = solid Big Blue margin bar: new
│█ Mondays, Oct 20 – Dec 15        │
│ ┌──────── yellow flyer ────────┐ │
│ │ Volunteers needed for lunch… │ │
│ ├╌╌╌╌╌╌╌╌╌ perforation ╌╌╌╌╌╌╌╌┤ │
│ │ [Sign up]  │ Due in 3 days   │ │  ← tear-off tabs
│ └──────────────────────────────┘ │
│┊ Friday Night Lights             │  ┊ = outlined margin bar: edited
│┊ Edited today, 12 words added    │
│┊ See changes                     │
├──────────────────────────────────┤
│  Senior Page Yearbook Materials  │  no bar: already read
└──────────────────────────────────┘
```

- **Change marks** borrow the revision bars from the margins of technical drawings:
  - a solid Big Blue bar for *new*;
  - an outlined bar for *edited*;
  - no bar once read.
  The "New" or "Edited" word sits next to the timestamp in sentence case, not as an all-caps pill.
- **Signups, the one bold element.** A signup item is drawn as a flyer: Flyer fill, with a perforated edge above a row of **tear-off tabs**.
  - There is one tab for each signup link (Sign up, RSVP, Buy tickets), each opening the form, plus a deadline tab.
  - The deadline tab turns Struck at 2 days or fewer.
  - The Sign-ups tab shows only flyers, sorted by deadline.
- **Diff sheet:**
  - A full-height sheet slides up from the bottom; this is the only motion besides the "since you looked" marker settling into place, and it is disabled under `prefers-reduced-motion`.
  - Title, Details and Body sections. Unchanged sections are collapsed into one line ("Body unchanged").
  - Inserted text is in Big Blue with an underline; deleted text is in Struck with a line-through, as in nisaba's `BlockAttributeDiffPage`.
  - The header reads "Oct 9 → Oct 14" with prev/next version arrows and a word-count summary.
- **Not used:** card grids, drop shadows, gradient washes, monospace data labels, all-caps eyebrows, "A · B" meta strings, "→" on buttons.
  External links get an icon, not a glyph.
- **Identity:** the app is unofficial, so it has its own name ("Pingry BBB Watch") and a plain pushpin icon.
  It uses no Pingry crest, logo or wordmark. The About screen says "Unofficial. Reads the public Big Blue Bulletin page."
- **Quality floor:**
  - WCAG AA contrast for all text in both themes; the Flyer fills are checked against Ink.
  - Touch targets of at least 44 px.
  - Visible focus rings in Big Blue, 2 px offset.
  - Dark mode follows the system setting.
  - The manifest's `theme_color` is Ink.

**Copy**
- Empty division: "Nothing new in Lower School since Monday. You'll get a notification when something is posted."
- Scrape failure banner: "Couldn't reach pingry.org at 8:05pm. Showing the page as of 7:55pm."
- Install guide (iOS, not yet installed): "To get notifications, add Pingry BBB Watch to your Home Screen: tap Share, then Add to Home Screen. Then open it from there."
- Push permission button: "Turn on notifications". Afterwards the toast reads "Notifications turned on for this iPhone".
- Invite missing: "You need an invite link to join. Ask the parent who shared Pingry BBB Watch with you."

**Review against generic defaults.** These were revised before finalizing:
- The first idea was a cork-board look with pins on every item. That was dropped as kitsch; the flyer treatment is kept for signups only, where it carries meaning.
- Small labels in monospace were planned, then removed; a template tell.
- Notification titles used "New · Lower School"; they were rewritten as plain sentences.
- Per-item rounded cards became a ruled list, with a distinct surface only for signups.

## Security, privacy and etiquette
- Access is by invitation only; the content is the school's and is shown only to invited parents. There are no public pages apart from the login and join pages and the static app shell.
- Google OIDC with PKCE, plus state and nonce checks; only verified emails are accepted. No passwords are stored.
  - A valid Google account alone doesn't grant access: a new account needs an invite. Invites can be revoked and have a usage cap.
  - The Google Cloud OAuth client needs an "External" consent screen and only the `openid email profile` scopes, so Google verification isn't needed.
    It stays in "Testing" mode until it is published, and Testing mode limits sign-in to listed test users, so it has to be published to "In production".
  - Env vars: `GOOGLE_CLIENT_ID`, `GOOGLE_CLIENT_SECRET`, `PUBLIC_URL=https://<host>.<tailnet>.ts.net`.
- The session cookie is HttpOnly, Secure and SameSite=Lax. Handoff nonces are 128-bit random values, single-use, with a 10-minute TTL. Auth endpoints are rate-limited.
- `body_html` is sanitized with bluemonday before it is sent to the client. Links get `rel=noopener` and `target=_blank`.
- Requests to the school site stay low (one every 5 minutes), carry an identifiable UA and back off on errors.
- Keeping data: all versions are kept, since the volume is tiny, roughly 50 items × a few versions per week.

## Tooling: lint, format, type-check

The goal is that every check runs from one command (`make check`) with pinned tool versions, so it gives the same result locally, in the pre-commit hook and in CI.
Each language gets one formatter and one linter.

### Go
- **golangci-lint v2** does both jobs, configured in `backend/.golangci.yml`.
  - **Formatters** (`golangci-lint fmt`): `gofumpt`, a stricter superset of gofmt, and `goimports` with `local-prefixes` set to the module path.
  - **Linters:** the `standard` set (`errcheck`, `govet`, `ineffassign`, `staticcheck`, `unused`), plus:
    - **HTTP client (poller and push sender):** `bodyclose`, `noctx`, `contextcheck`.
    - **pgx code:** `sqlclosecheck`, `rowserrcheck`.
    - **Errors:** `errorlint`, which requires `errors.Is`/`As` and `%w` wrapping, and `nilerr`.
    - **Security:** `gosec`, which covers the OAuth state, cookie flags and the HTTP timeouts.
    - **General:** `revive` (default rules), `misspell`, `usestdlibvars`, `perfsprint`, `copyloopvar`, `intrange`, `exhaustive` (for the event `kind` enum).
  - Test files are excluded from `gosec`.
- **`govulncheck ./...`** checks dependencies for known vulnerabilities. It matters most for the OAuth, session and web push packages.
- Tests run with **`go test -race -shuffle=on ./...`**. The poller and notifier run concurrently, which is why the race detector is on.
- Versions: golangci-lint is a pinned binary installed by `make tools`, as its maintainers recommend instead of a `go.mod` tool directive. `govulncheck` is a `tool` directive in `go.mod`.

### TypeScript, React and the service worker
- **oxlint** (`.oxlintrc.json`), run with **`--type-aware`**. Type information comes from `oxlint-tsgolint`, which is built on the Go port of TypeScript.
  - Categories: `correctness` and `suspicious` are set to error, `perf` to warn. `style` stays off, since Prettier owns formatting.
  - **Type-aware rules:**
    - `no-floating-promises` and `no-misused-promises` catch a missing `await`, and a missing `event.waitUntil(...)` in the service worker's `push` and `notificationclick` handlers. That bug fails silently on iOS, so the notification never appears.
    - `switch-exhaustiveness-check` covers the event `kind` union.
    - `await-thenable`, `no-unnecessary-type-assertion` and `restrict-template-expressions`.
    - `strict-boolean-expressions` is set to warn.
- **Built-in plugins:** `typescript`, `react`, `jsx-a11y`, `import`, `vitest`. Nothing extra has to be installed.
  - **react:** `rules-of-hooks`, `exhaustive-deps`, `only-export-components`, and the React Compiler rules (`purity`, `immutability`, `refs`, `set-state-in-render`, `static-components`, …), following nisaba's `.oxlintrc.json`.
  - **jsx-a11y:** the recommended rules. They enforce the accessibility standard set in Visual design: labels on the tear-off tab buttons, and focus handling in the diff sheet.
  - **typescript:** `no-explicit-any`, `consistent-type-imports`, and `no-unused-vars` that ignores names starting with `_`.
  - **import:** `no-cycle`.
  - **vitest:** applies to test files.
- `src/sw.ts` gets an `overrides` entry in `.oxlintrc.json` with `env.serviceworker: true` and `env.browser: false`, and its own `tsconfig.sw.json` (with `lib: ["ES2023", "WebWorker"]`), so DOM globals can't leak into the worker.
- **TypeScript:** `tsc -b --noEmit` with `strict`, `noUncheckedIndexedAccess`, `exactOptionalPropertyTypes`, `noImplicitOverride`, `noFallthroughCasesInSwitch` and `verbatimModuleSyntax`.
- **API types:** generated from an OpenAPI spec (`backend/api/openapi.yaml` → `openapi-typescript` → `frontend/src/api/schema.d.ts`). A CI step fails if the generated file is out of date, so the frontend and backend can't drift.
- **Prettier** is the only formatter for TS, TSX, CSS, JSON, YAML and Markdown, using its defaults: `printWidth` 80, double quotes and semicolons. Defaults avoid style debates, and long lines don't wrap well in phone-width diff reviews.
  Since oxlint's `style` category is off, the linter and Prettier never conflict.
- **knip** finds unused files, exports and dependencies.
- Install with `npm ci` from the committed lockfile. Dev tools are pinned to exact versions, and Renovate updates them in grouped weekly PRs.

### SQL migrations
- **squawk** lints each new migration for Postgres hazards: locking `ALTER`s, `NOT NULL` without a default, and indexes created without `CONCURRENTLY`. This is a migration-safety check that a style linter wouldn't do.
- **sqlfluff** (`dialect = postgres`, configured in `.sqlfluff`) handles both formatting and style.

### Everything else
- **shellcheck** and **shfmt** for `scripts/` and the deploy helpers.
- **gitleaks** scans for secrets in the pre-commit hook and in CI. This repo will have `GOOGLE_CLIENT_SECRET`, the VAPID private key and `SESSION_SECRET` sitting in `.envrc`, and `.envrc` is in `.gitignore`.
- **`.editorconfig`:** UTF-8, LF line endings, final newline, 2-space indent, tabs for Go and the Makefile.
- **Prettier** skips `docs/fixtures/`, `dist/`, generated `schema.d.ts` and `node_modules/`.

### Running the checks

```
make tools   # install pinned golangci-lint, squawk, gitleaks, shellcheck, shfmt into ./bin
make fmt     # golangci-lint fmt, prettier --write ., sqlfluff fix, shfmt -w
make lint    # golangci-lint run, govulncheck, oxlint --type-aware, tsc -b, knip, squawk, sqlfluff lint, shellcheck
make test    # go test -race -shuffle=on ./..., vitest run --coverage
make check   # formatters in check mode + lint + test + openapi drift check; CI runs exactly this
```

- **Git hooks:** managed with **lefthook** (`lefthook.yml`), installed by `make tools`.
  - **pre-commit:** formatters in check mode plus the linters, on staged files only (`{staged_files}` globs), with the jobs running in parallel. Also gitleaks.
  - **pre-push:** `make test`.
- **CI:** GitHub Actions runs `make check` on every push and PR, with Go and npm caches. Renovate keeps the Actions pinned by SHA and updates them.
- **`CLAUDE.md`:** lists `make fmt` and `make check` as the commands to run before calling a change done.

## Repo layout

```
bbb/
  docker-compose.yml  Makefile  .envrc.example  README.md  CLAUDE.md
  .editorconfig  .sqlfluff  .prettierignore  lefthook.yml  renovate.json  .github/workflows/check.yml
  backend/.golangci.yml  backend/api/openapi.yaml
  frontend/{.oxlintrc.json,tsconfig.json,tsconfig.sw.json,knip.json}
  db/migrations/000001_initial.{up,down}.sql
  backend/cmd/server/main.go
  backend/internal/{config,db,store,auth,handler}/      # nisaba-style
  backend/internal/bbb/{fetch,parse,normalize,signup,diff}.go  + testdata/bbb-sample.html
  backend/internal/notify/{webpush,fanout}.go
  frontend/src/{App.tsx,theme.ts,sw.ts}
  frontend/src/lib/wordDiff.{ts,md,test.ts}
  frontend/src/components/{ItemRow,SignupFlyer,DiffView,DiffSheet,InstallGuide}.tsx
  frontend/src/pages/{Feed,Timeline,ItemDetail,DiffPage,Settings,Login,Join}.tsx
  docs/DESIGN.md
  docs/fixtures/        # gitignored: raw page snapshots, never committed
```

## Milestones
1. **Scraper core:** parser, normalization, differ, migrations, and golden-file tests against the saved snapshot.
2. **Read-only web app:** Google sign-in with invites and the iOS handoff, feed, item detail, `DiffView` / `DiffSheet`, signup highlighting.
3. **Push:** VAPID, subscribe flow, service worker, fan-out against preferences, batching, the test-push button.
4. **Hardening:** suspect-run guard and admin alert, quiet hours, deploy unit, Funnel, install guide.

## Testing and verification
- **Fixtures:** raw snapshots of the page contain staff and parent email addresses and school content, so they are kept only locally in `docs/fixtures/`, which is gitignored. The repo is public.
  - The committed `testdata/bbb-sample.html` is produced by `scripts/sanitize-fixture`. It keeps the Finalsite markup, panel IDs and link hosts, but replaces the text with placeholder text and every `mailto:` with `example@example.org`.
  - The panel `fsEl_67589` is kept as-is, since its text is a generic school notice.
- **Parser:** a golden test on `testdata/bbb-sample.html`. It checks the item and division counts (41 items on 2026-10-14), and that `fsEl_67589` has the title "2027-28 Tuition Assistance Application" and the subheader "Deadline: Sunday, November 1".
  An optional local-only test runs against every file in `docs/fixtures/` when that directory exists.
- **Differ:** table tests on mutated copies of the snapshot:
  - an added panel gives `new`;
  - changed body text gives `edit` with the right word counts;
  - an `&nbsp;`-only change gives no event;
  - a removed panel gives `removed`;
  - an empty or truncated page gives `suspect`, with no events.
- **Signup:** a fixture of known items, for example "Friday Night Lights: Join Us and Sign Up to Help!" counts as a signup and "Field Hockey Lights Up Babbitt Field" does not.
- **wordDiff:** vitest cases, as listed in `wordDiff.md`.
- **End to end:**
  - Run a local fixture server that serves an edited snapshot, with `BBB_URL` pointed at it.
  - Check that the event appears in the timeline and that the diff dialog renders.
  - On a real iPhone using the Funnel URL: Add to Home Screen, sign in with Google (the handoff flow), enable notifications, press "send test", then edit the fixture and confirm the push arrives and opens the diff.

## Implementation notes (milestone 2)

Where the build differs from, or pins down, the design above:

- The feed's **All** tab shows every division, grouped under division headings; Lower, Middle and Upper show only that division.
- The diff sheet's URL is `?item=<panelID>&diff=<from>..<to>`; the API needs the panel id to fetch versions.
- Events from before a user's account existed count as read. The feed and timeline mark what they show as read after it has been on screen for 1.5 s, and keep showing the marks until the next load.
- The font is self-hosted from `@fontsource/atkinson-hyperlegible-next` instead of loaded from Google Fonts, so the installed app works offline and makes no third-party requests.
- `wordDiff.ts` is copied byte for byte and builds under the app's strict TypeScript and oxlint settings with no overrides. Its vitest cases match the Go port's tests.
- `openapi-typescript` needs the TypeScript 5 compiler API, which TypeScript 7 lacks, so it is installed separately in `frontend/tools/gen-api`.
- Handler tests validate every response against `openapi.yaml` (kin-openapi), so the spec can't drift from the server either.
- A handoff sign-in forces Google's account chooser (`prompt=select_account`). A handoff gives the session to whoever created the nonce, so this keeps a stray link from signing someone in silently.
- Admins can make invite links from Settings, as well as with `-create-invite`.

## Implementation notes (milestone 3)

- The notifier's queue is the `events` table: `events.notified_at` (migration 2) is stamped once an event is handled. The poller kicks the notifier after a run with events, and it also sweeps every minute, so events written by `-scrape-once` still go out. Migration 2 stamps all earlier events so turning push on doesn't announce old news; without VAPID keys events are stamped and nothing is sent, for the same reason.
- Keys: `VAPID_PUBLIC` and `VAPID_PRIVATE`, from `server -gen-vapid`. `VAPID_SUBJECT` defaults to `mailto:$CONTACT_EMAIL`; the server refuses to start with push on and a subject that isn't `mailto:` or `https:`, since Apple rejects those.
- Subscription endpoints must be https on a known push service (Apple, FCM, Mozilla, Windows), because the server POSTs to them. An endpoint already saved moves to whoever subscribes it, so a shared device follows the person signed in. Signing out turns off the device's notifications first.
- Batching is per run: more than 3 matching events from one run become one summary. Moves never notify; an edit that adds a signup link notifies whatever its size. Each push sets a `Topic` equal to its tag, so the push service also collapses undelivered ones.
- Retries: 429, 5xx and network errors are tried up to 4 times with doubling back-off (honouring `Retry-After`, capped at a minute); every attempt is a `deliveries` row. A test push is tried once so the button answers promptly; it records only `last_ok_at`.
- Each payload carries the user's unread count (`badge`), which the worker sets as the app badge; `/status` returns it too, and the app sets the badge on launch and return.
- Every account has a preferences row: created with the account, backfilled by migration 2, and created on first read for an account an older server made.
- The service worker (`src/sw.ts`, built by vite-plugin-pwa `injectManifest`) precaches the shell, serves `index.html` for navigations except `/api/`, and takes over at once (`autoUpdate`). It also runs under `make frontend` so push can be tried on localhost. `public/manifest.webmanifest` stays hand-written.
- Tests use `notify/notifytest`, a fake push service that decrypts what it receives (RFC 8291), so they check the notification text a phone would show.

## Implementation notes (milestone 4)

- **Admin alert:** when the stored runs show 3 failed or suspect runs in a row (counted from the last good run, so a restart doesn't reset the streak), every admin device gets "Pingry BBB Watch can't read the page", which opens Settings. The next good run sends "Pingry BBB Watch is reading the page again" with the same tag, replacing it. Alerts ignore quiet hours, since they are rare and need action. Fetch errors count as well as suspect pages, since either way nothing is being announced.
- Settings shows admins the last 12 checks ("Recent checks"), with failures in Struck, so an alert says why.
- **Quiet hours** are `"HH:MM"` in `SCHOOL_TZ`, may cross midnight, and are both set or both null (a check constraint, migration 3). A matching event for a user in quiet hours goes into `held_events` instead of being pushed; the first sweep after the hours end sends the held events as one push (the event itself if only one is left, otherwise a summary), re-checked against the current preferences. Events read in the app meanwhile are dropped, as are held events of a user with no devices left.
- Settings: a "Quiet hours" switch (default 9:30pm to 7:00am) with From and Until time fields, saved once both are whole and differ.
- The install guide (iOS, not yet on the Home Screen) is the one in Settings from milestone 3; the deploy unit and Funnel are under "Implementation notes (deploy)".

## Implementation notes (deploy)

- `deploy/bbb.service` is committed and linked into the user manager by `make install-service`. Its `ExecStartPost` sets up the Funnel mapping (`tailscale funnel --bg --https=443 http://127.0.0.1:8093`), and `ExecStopPost` removes it.
- `scripts/run-production` derives `PUBLIC_URL` from `tailscale status`, so the public repo doesn't name the tailnet.
- Development moved to `127.0.0.1:8094` so it doesn't collide with the deployed server on 8093.

## Decisions
- **Hosting:** home server, Tailscale Funnel on port 443.
- **Users:** invited parents.
- **Auth:** Google sign-in, plus an invite code for new accounts.
- **Signups outside chosen divisions:** these don't notify by default; there's a per-user toggle.
