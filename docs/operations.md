# Operations: backup, restore, upgrades and diagnostics

## Backup

All state (database, history, keys, chains, audit trail) lives in the data
volume. Take a backup before every upgrade.

### SQLite (default)

The database is `${DATA_DIR}/serpentseek.db` with WAL side-files
(`-wal`, `-shm`). Copy all three together, or stop the container first for a
consistent snapshot:

```bash
docker compose stop serpentseek
docker run --rm \
  -v serpentseek_data:/data \
  -v "$PWD":/backup \
  alpine tar czf /backup/serpentseek-$(date +%F).tgz -C /data .
docker compose start serpentseek
```

If the `sqlite3` CLI is available on the host, an online-consistent copy can be
made without stopping the service:

```bash
sqlite3 serpentseek.db ".backup '/backup/serpentseek.db'"
# or: sqlite3 serpentseek.db "VACUUM INTO '/backup/serpentseek.db'"
```

### PostgreSQL (`extdb` profile)

```bash
docker compose --profile extdb exec -T postgres \
  pg_dump -U serpent serpentseek | gzip > serpentseek-$(date +%F).sql.gz
```

## Restore

### SQLite

```bash
docker compose stop serpentseek
docker run --rm \
  -v serpentseek_data:/data \
  -v "$PWD":/backup \
  alpine sh -c "rm -rf /data/* && tar xzf /backup/serpentseek-YYYY-MM-DD.tgz -C /data"
docker compose start serpentseek
```

### PostgreSQL

```bash
gunzip -c serpentseek-YYYY-MM-DD.sql.gz | \
  docker compose --profile extdb exec -T postgres psql -U serpent -d serpentseek
```

## Upgrading

```bash
docker compose pull && docker compose up -d
```

* Migrations run automatically on start (goose); the service becomes ready only
  after they complete — probe `/readyz`.
* Migrations are additive, but **take a backup first** and **do not downgrade**
  an image across a schema change.
* `STORAGE_DRIVER` and `DATABASE_URL` are environment-only; changing the driver
  requires a fresh database and a restart.
* `CHANGELOG.md` lists breaking changes per release; the HTTP API is additive
  between releases.

## Diagnostics

| Endpoint | Purpose |
| --- | --- |
| `/healthz`, `/readyz` | Readiness: database reachable (503 otherwise) |
| `/livez` | Liveness: process is answering |
| `/metrics` | Prometheus metrics (text exposition 0.0.4) |
| `/debug/pprof` | Go profiling, only when `DEBUG=true` |
| `X-Serpent-Rid` | Response header linking a search to History/logs/Audit |

The **Audit** page records every admin mutation (actor, IP, action, target,
non-secret details) and is kept for 180 days; search-quota counters
(`usage_daily`) are pruned after 90 days.

## Per-user search quotas

On top of the global `SEARCH_RATE_LIMIT_PER_MINUTE` / `SEARCH_DAILY_QUOTA`
settings, an admin can assign a **per-user** quota from the Users page
(🚦 badge next to each user) or via the API:

```
PUT    /api/users/{id}/quota   {"requests": 500, "period": "daily", "tz_offset": 480}
GET    /api/users/{id}/quota   -> usage, remaining, reset_at
DELETE /api/users/{id}/quota   -> user becomes unlimited
```

* `requests` — allowance per period; `0` = unlimited (default when no quota
  is set, fully backwards compatible).
* `period` — reset cadence:
  * `daily` — resets at midnight, shifted by `tz_offset` minutes
  * `weekly` — resets Monday 00:00 (tz-shifted)
  * `monthly` — resets on the 1st of the month 00:00 (tz-shifted)
  * `rolling` — sliding window of `window_hours` (1–720)
* `tz_offset` — fixed offset in minutes (-720..840); 0 = UTC. A fixed offset
  is used instead of a timezone name, so DST is not observed.

The quota applies to **all** of a user's API keys and sessions (counted on the
`user:<id>` subject); the global limits still apply on top. When exhausted,
search endpoints return HTTP 429 with `Retry-After` and
`X-Serpent-Quota-Reset-At` (calendar periods) headers. Rejections are counted
in the `serpentseek_quota_rejected_total{period}` metric and fire the
`quota_exceeded` webhook event. Quota mutations are recorded in the audit log
(`quota.set` / `quota.delete`).

Storage: per-user rows live in `user_quotas`; calendar-period usage is counted
in `usage_daily` under `"<start-date>#<period>"` keys, rolling-window hits in
`usage_rolling`. Both are pruned by the cleanup job after
`USAGE_RETENTION_DAYS` (default 90).

## API versioning

* The HTTP API is served under `/api` and documented in
  [`api/openapi.yaml`](../api/openapi.yaml). A parity test fails CI when routes
  and the spec drift apart.
* The contract is **additive**: new fields and endpoints may appear in a minor
  release; breaking changes are called out in `CHANGELOG.md`.
* `/search` (Open WebUI) and `/mcp` (Model Context Protocol) are the stable
  integration surfaces. `/search` always returns HTTP 200 with a rows array,
  except when the caller exceeds its own rate limit or daily quota (HTTP 429).
