# Deploying SerpentSeek from Docker Hub

Prebuilt image: [`sallend/serpent-seek`](https://hub.docker.com/r/sallend/serpent-seek)
(`latest` tracks the newest version tag; fixed tags like `1.0.0` and `1.0` are available too).

## Requirements

* Docker 20.10+ with the `docker compose` plugin

## Option 1 — Docker Compose (recommended)

**1. Create `docker-compose.yml`:**

```yaml
services:
  serpentseek:
    image: sallend/serpent-seek:latest
    container_name: serpentseek
    ports: ["8080:8080"]
    env_file: [.env]
    volumes: [serpentseek_data:/app/data]
    restart: unless-stopped
    security_opt: [no-new-privileges:true]
    pids_limit: 512

volumes:
  serpentseek_data: {}
```

**2. Minimal `.env`** (everything else can be configured later in the UI):

```bash
PORT=8080
AUTH_ENABLED=true
# If you run your own SearXNG instance (always external):
# SEARXNG_URL=http://searxng.example.com/search
```

**3. Start and complete the first-run setup:**

```bash
docker compose up -d
docker compose logs -f serpentseek    # look for: SETUP_TOKEN: xxxxxxxx
```

1. Open `http://<host>:8080/setup` → enter the token from the logs → create
   an administrator.
2. **The API key (`seek_ak_...`) is shown only once** — save it. Then sign in
   at `/login` with that key.
3. Provider keys (Brave, Yandex, Tavily, …) are added in the UI on the
   **Providers** page — no restart required.

Health check: `curl http://localhost:8080/healthz`

## Option 2 — docker run

```bash
docker run -d --name serpentseek \
  -p 8080:8080 \
  -v serpentseek_data:/app/data \
  -e AUTH_ENABLED=true \
  --restart unless-stopped \
  sallend/serpent-seek:latest

docker logs -f serpentseek   # SETUP_TOKEN for /setup
```

## Option — PostgreSQL instead of SQLite

For multi-replica setups or heavier loads. The default WAL-mode SQLite is
sufficient for a single instance.

```yaml
  serpentseek:
    environment:
      STORAGE_DRIVER: postgres
      DATABASE_URL: postgres://serpent:secret@postgres:5432/serpentseek?sslmode=disable

  postgres:
    image: postgres:17-alpine
    container_name: serpentseek-postgres
    environment:
      POSTGRES_DB: serpentseek
      POSTGRES_USER: serpent
      POSTGRES_PASSWORD: secret
    volumes: [serpentseek_pg:/var/lib/postgresql/data]
    restart: unless-stopped

volumes:
  serpentseek_pg: {}
```

## Option — HTTPS + Passkeys (WebAuthn)

Passkeys require HTTPS and a domain. Put a reverse proxy (nginx / Caddy /
Traefik) in front of the service and add to `.env`:

```bash
PUBLIC_ORIGIN=https://seek.example.com
RP_ID=seek.example.com
```

## Open WebUI integration

*Settings → Web Search → External*: URL `http://<host>:8080/search`,
API key — your `seek_ak_...`.

## MCP integration

Point any MCP client at the streamable HTTP endpoint:

```json
{
  "mcpServers": {
    "serpentseek": {
      "url": "http://<host>:8080/mcp",
      "headers": { "Authorization": "Bearer seek_ak_..." }
    }
  }
}
```

## Updating

```bash
docker compose pull && docker compose up -d
```

All data (SQLite database, history, keys) lives in the `serpentseek_data`
volume and survives image updates.

## More configuration

Every setting (retention crons, retry codes, concurrency, encryption at rest
via `ENCRYPTION_KEY`, …) is documented in
[`.env.example`](.env.example) and editable in the admin UI unless pinned by an
environment variable.
