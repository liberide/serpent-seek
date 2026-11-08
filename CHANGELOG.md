# Changelog

All notable changes to this project are documented here. The format is based on
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and this project
adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [1.1.4] - 2026-11-08

### Fixed
- **SerpBase Google link resolution**: the optional `/google/resolve` call can
  take about 25 seconds, but the 15-second provider-test timeout and the
  20-second default node timeout cancelled it and silently returned the
  original `google.com/goto` or `/url` wrappers. With `resolve_google_links`
  enabled, the engine now allows up to 60 seconds for the combined search and
  resolve flow. Longer configured timeouts are preserved; disabling the option
  keeps the previous timeout behaviour.

## [1.1.3] - 2026-11-03

### Added
- **Restart reconciliation**: requests (and their steps) left in the
  `running` state by a crashed/killed process are marked `failed` with the
  "interrupted by service restart" reason at boot — History no longer shows
  eternally running entries. New storage method `FailStaleRunningRequests`
  (SQLite + PostgreSQL).
- **Kubernetes Helm chart** (`deploy/helm/serpentseek`): Deployment + Service +
  PVC for SQLite or external PostgreSQL, optional Ingress and ServiceMonitor;
  `helm lint`/`template`-validated.
- **Grafana dashboard** (`deploy/grafana/dashboard.json`) covering every
  exported metric, importable via the Prometheus datasource variable.
- **OpenTelemetry spans in the engine**: a chain-level `engine.run` span (
  rid/chain/mode attributes, query text deliberately excluded) and one
  `provider.attempt` span per provider try (provider/node/step/attempt, rows,
  HTTP status, error status). No-op unless an OTLP endpoint is configured.
- **History export**: `GET /api/requests/export?format=jsonl|csv` streams the
  filtered history (keyset-paginated, 100k-row cap) with the list endpoint's
  scoping rules; export buttons on the History page (all 7 locales).
- **Optional `/metrics` protection**: `METRICS_AUTH_TOKEN` (Bearer token) and
  `METRICS_ALLOWED_CIDRS` (client IP/CIDR allowlist) lock down the Prometheus
  endpoint; both empty keeps it public as before. Wrong token answers 401 with
  a `WWW-Authenticate` challenge.
- **`serpentseek rotate-key`**: re-encrypts secrets at rest (settings, provider
  credentials, proxy passwords) from `ENCRYPTION_KEY` to `ENCRYPTION_KEY_NEW`
  (an empty new key disables at-rest encryption). The command aborts before
  writing anything when the old key does not match.
- New Prometheus counter `serpentseek_sse_dropped_events_total` — SSE events
  skipped for slow subscribers (they replay via `Last-Event-ID` on reconnect).
- `.env.example` documents `SQLITE_PATH` and `IGNORE_REQUEST_COUNT`; a config
  unit test now fails CI when a supported variable is missing from the example.

### Changed
- **Dependency security sweep** (govulncheck + npm audit):
  - Go toolchain baseline moved to **1.26.9** (13 stdlib advisories in
    `net/http`, `html/template`, `crypto/tls`, `crypto/x509`, `os`,
    `net/textproto` no longer apply to built binaries; govulncheck reports
    "No vulnerabilities found").
  - `golang.org/x/crypto` v0.57.0 → v0.58.0, `golang.org/x/net` v0.60.0 →
    v0.62.0, plus `chi` 5.5.0, `pgx` 5.12.0, `goose` 3.29.0,
    `go-webauthn` 0.18.2, `modernc.org/sqlite` 1.61.1 and their minor
    transitive bumps.
  - Web stack upgraded to **SvelteKit 3** (`@sveltejs/kit` 3.1.1,
    `adapter-static` 4.0.1, `vite-plugin-svelte` 7.3.1, TypeScript 6):
    config moved from `svelte.config.js` into the Vite plugin, `$lib` kept via
    the official alias escape hatch, `goto()` options migrated
    (`replaceState/noScroll/keepFocus` → `replace/reset`). Vite 8.3.5 fixes
    three high dev-server advisories.
- The asynchronous playground search (`/api/search/ui`) no longer hard-caps at
  10 minutes: a configured `CHAIN_TIMEOUT` above 10 minutes is honored.
- CI total coverage floor raised from 50% to 58%.

### Fixed
- `notify`: the per-target webhook throttle map grew unbounded over long
  uptimes (one entry per exhausted-quota user); stale entries are now pruned
  once the map exceeds 1024 keys.
- Metrics allowlist: IPv4-mapped IPv6 client addresses (`::ffff:a.b.c.d`) now
  match IPv4 CIDR entries in `metrics_allowed_cidrs` (dual-stack sockets).

### Tests
- Smoke tests for `cmd/serpentseek` (`seed()` bootstrap + `rotate-key`).
- Handler tests for the `/metrics` guard; hub test for the dropped-event counter.
- Critical auth branch tests: revoked key, developer token gated by mode on/off,
  expired session, key of a disabled user.
- Web unit tests for the REST client (`api.ts`): CSRF header, error envelope,
  401 redirect, raw mode.

## [1.1.2] - 2026-10-19

### Added
- **DeepSeek Search provider** (`deepseek_search`): calls DeepSeek's in-house
  search engine through `POST /v1/tools/search` with the regular DeepSeek API
  key. The model-oriented `summary` is used as the result snippet, the optional
  `need_content` flag returns the full page text, and `ori_question` /
  `localization` are passed through. `count` is capped at 50; `408`, `429` and
  `5xx` responses are retryable while `400`, `401`, `402`, `403` and `404` are
  permanent. The built-in provider count is now **41**.
- DeepSeek Search is documented in all localized README provider tables
  (English, Russian, German, French, Japanese and Chinese).

### Changed
- Project version is now **1.1.2**.

## [1.1.1] - 2026-10-11

### Added
- **Per-user analytics**: the Analytics sidebar group now offers two pages —
  **Overview** (the previous report) and **By users** (admins only; the
  sections are selected from the sidebar like the other pages). The By-users
  page lists every
  owner as a leaderboard for the selected period: request volume with its share
  of all traffic, success rate, average time, errors, last activity, active
  API-key count and the live quota status (🚦 remaining/limit, tying into the
  per-user quota feature; dormant users are listed too). Clicking a row opens a
  per-user report — KPIs, daily trend, hourly activity, status donut, top
  queries and a per-user provider comparison — plus a quota usage card with a
  progress bar. Requests made without a user are aggregated under an
  **Anonymous** bucket and are inspectable too. Served by the admin-only
  `GET /api/stats/users` and by `GET /api/stats/analytics?user_id=…`
  (`anonymous` selects the no-user bucket; non-admins may only query their own
  id). The selected user is mirrored in the URL
  (`/analytics/users?user=…`) so views are shareable.
- **Per-user search quotas**: admins can assign each user a request allowance
  with a daily / weekly / monthly / rolling reset cadence, from the Users page
  (🚦 badge) or via `GET`/`PUT`/`DELETE /api/users/{id}/quota`. The quota is
  enforced on `/search`, `/mcp` and the playground across all of the user's
  API keys and sessions; exhausted quotas return HTTP 429 with `Retry-After`
  and `X-Serpent-Quota-Reset-At`. Fully opt-in: users without a quota keep the
  global `SEARCH_RATE_LIMIT_PER_MINUTE` / `SEARCH_DAILY_QUOTA` behaviour.
  Rejections are exported as `serpentseek_quota_rejected_total{period}` and
  fire the `quota_exceeded` webhook.
- **Developer mode & developer tokens**: an admin-only **Settings → Developer**
  section with just the enable/disable switch (`DEVELOPER_MODE`, default off).
  Turning it on shows a **Developer** section in the sidebar where keys are
  issued, revoked and (after revoking) deleted permanently. Developer tokens
  (`seek_dev_…`) carry full admin scope for third-party/programmatic
  integrations: a token can provision providers, chains (and activate them),
  users, search API keys, proxies and settings entirely through the REST API.
  Turning the mode off immediately rejects every developer token without revoking
  it. Served by the admin-only `GET /api/developer`, `PUT /api/developer/mode`,
  `GET /api/developer/endpoints`, `POST /api/developer/tokens`,
  `PATCH /api/developer/tokens/{id}` and `DELETE /api/developer/tokens/{id}`
  (`?purge=true` deletes a token permanently).
- **Per-endpoint developer-token permissions**: each developer token carries an
  explicit permission list of endpoint ids (`"METHOD /path"`) enforced after
  authentication and routing (`403 endpoint_forbidden` otherwise). Tokens stay
  **Full access** by default; the Developer page offers a grouped picker when
  creating a token and a per-token **Edit** dialog to change the name and
  permissions. New admin endpoints `GET /api/developer/endpoints` and
  `PATCH /api/developer/tokens/{id}` back it, and migration
  `00016_api_key_permissions` adds the `api_keys.permissions` column. A
  restricted token cannot be used for browser sign-in (which would otherwise
  escalate to the owner's full role).
- **OpenAPI spec served at runtime + in-app Swagger UI**: `GET /openapi.yaml`
  exposes the embedded OpenAPI 3.1 document (public, CORS-enabled), and the
  Developer section opens `/swagger` — a Swagger UI bundled locally with the
  frontend (no CDN or other external resource; the public validator is
  disabled). A download button is provided as well.
- **Collapsed sidebar footer**: when the rail is collapsed, the build version
  and the “Support the project” button stay at the bottom, the support entry
  as a slowly shimmering, dimmed heart.

### Changed
- **Collapsible sidebar**: the rail collapses to an icon-only strip from the
  button next to the logo, and the choice is remembered per browser. Hovering a
  collapsed icon shows its section name; grouped entries (Analytics) open a
  fly-out submenu.
- **Sidebar icons**: navigation entries and the collapse toggle now use inline
  SVG glyphs that inherit the active theme colour. The 🐍 brand mark is
  unchanged.

### Fixed
- **Settings form no longer fails with “some settings are read-only”**: the
  Save button used to PUT every field, including values pinned by an
  environment variable or marked environment-only, so unchanged read-only
  values tripped the whole request. Read-only values echoed back unchanged are
  now ignored; only an actual attempt to change them is rejected.

## [1.1.0] - 2026-10-09

### Added
- **Update notifications**: an optional daily check of the public GitHub release
  feed (`UPDATE_CHECK_ENABLED`, `UPDATE_CHECK_URL`, `UPDATE_CHECK_INTERVAL_HOURS`).
  A dismissable badge appears in the sidebar and full status on the
  Settings → About page; served by `GET /api/update`, forced with
  `POST /api/update/check`. The check never modifies the installation and can be
  disabled for air-gapped setups. `UPDATE_CHECK_URL` is **environment-only**
  (never editable in the UI); a non-default value marks the instance as
  **unofficial** in the sidebar and on the About page.
- **Settings → About** page (bottom of the menu, separated by a rule): a grouped,
  redesigned panel with **Build** (version, commit, build date, Go), **System**
  (storage driver, uptime, start time), **Updates** (status, check-now, release
  notes) and **Links** (source, docs, license), served by the read-scoped
  `GET /api/info`.
- **Provider cost tracking**: an optional per-instance `cost_per_request`
  param is snapshotted into `request_steps.cost`, summed per provider and as a
  `cost_total` in Analytics. When no provider declares a cost the cost UI is
  hidden and nothing is counted (`cost_configured=false`). Shown on the
  Providers list, the Analytics provider table/KPI, and per step + per request
  in History.
- **Config export/import** (`GET /api/config/export`, `POST /api/config/import`):
  a secret-free snapshot of providers, chains and non-secret settings for
  migrating between instances. Credentials are never exported and must be
  re-entered. Managed from **Settings → Configuration**.
- **Configurable retention** for search-quota counters (`USAGE_RETENTION_DAYS`,
  default 90) and the audit trail (`AUDIT_RETENTION_DAYS`, default 180); `0`
  disables pruning.
- **Hardened `docker-compose.yml`**: postgres healthcheck/`depends_on`,
  memory/CPU limits, log rotation, read-only rootfs + tmpfs.
- **Reliability controls**: `CHAIN_TIMEOUT` (hard per-search deadline) and a
  per-provider **circuit breaker** (`PROVIDER_FAILURE_THRESHOLD` /
  `PROVIDER_COOLDOWN_SECONDS`). Both default to disabled.
- **`/livez`** (liveness) and **`/readyz`** (database-aware readiness), plus
  `pprof` under `/debug/pprof` gated by `DEBUG`.
- **Audit log**: every admin mutation (settings, providers, proxies, chains,
  API keys, users, maintenance) is recorded with actor, IP and non-secret
  details. Exposed at `GET /api/audit` and a new **Audit** page; records are
  pruned after 180 days by default.
- **Prometheus metrics** at `GET /metrics` (text exposition format 0.0.4):
  build info, in-flight requests, SSE clients, search outcomes/durations and
  per-provider attempt counts/durations. Dependency-free (`internal/metrics`).
- **Search throttling**: `SEARCH_RATE_LIMIT_PER_MINUTE` and
  `SEARCH_DAILY_QUOTA`, enforced per API key (or user/IP) on `/search`, `/mcp`
  and the playground. Daily counters are stored in `usage_daily` and exposed as
  `used_today` in `GET /api/keys` (column on the Users page).
- Provider driver alignment with the current API contract and an Ollama web
  search driver.
- Web UI appearance/animation polish.
- **Japanese localization** (`ja`): a full 620-key UI dictionary, the
  `ja-JP` locale for `Intl` date/number formatting, and a complete
  `README.ja.md`. The language switcher now offers 7 languages, and
  `web/scripts/check-i18n.mjs` verifies key parity across all of them.
- Polish documentation ([README.pl.md](README.pl.md)).

### Changed
- **Project version is now `1.1.0`.**
- Docker builds bake the version/commit/date into the binary: `make up`,
  `make up-extdb` and `make image` pass `APP_VERSION`/`APP_COMMIT`/`APP_BUILD_DATE`
  automatically (a plain `docker compose up --build` falls back to `1.1.0`
  with `dev`/`unknown` metadata).
- `GET /version` returns just the version string (e.g. `1.1.0`); commit and
  build date moved to `GET /api/info`, shown on the Settings → Info page. The UI
  footer shows the version with `Liberide 2026` on the line below.
- `GET /version` is documented as `text/plain`; several OpenAPI schemas
  (`ApiKey`, `ApiKeyCreated`, `MeResponse`, `PasskeyCredential`) now match the
  actual API responses.

### Fixed
- **"Importing a module script failed" after a redeploy**: a missing build asset
  is no longer served as the HTML shell (the server returns `404` now), and the
  SPA reloads once when Vite reports a failed chunk preload, so an open tab picks
  up the new build instead of breaking.
- Duplicate `flow.start` i18n key in every locale (masked by `JSON.parse`);
  `check-i18n.mjs` now fails on duplicate sibling keys.

## Older releases

Per-version history before 1.1.0 is available in the git log and release tags
(`v1.0.0` … `v1.0.6`). Backup/restore/upgrade notes are in
[docs/operations.md](docs/operations.md).
