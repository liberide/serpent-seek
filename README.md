# 🐍 SerpentSeek — self-hosted search gateway for Open WebUI, MCP & 37+ search APIs

**English** | [Русский](README.ru.md "README по-русски") | [Deutsch](README.de.md "Auf Deutsch lesen") | [Français](README.fr.md "Lire en français") | [中文](README.zh.md "阅读中文版")

[![License: GPL-3.0](https://img.shields.io/badge/license-GPL--3.0-blue.svg)](LICENSE "Project license — GNU GPL v3.0")
[![Go 1.26](https://img.shields.io/badge/Go-1.26-00ADD8?logo=go&logoColor=white)](go.mod "Backend written in Go")
[![CI](https://github.com/liberide/serpent-seek/actions/workflows/ci.yml/badge.svg)](https://github.com/liberide/serpent-seek/actions/workflows/ci.yml "CI: build, vet, tests")
[![Docker — single container](https://img.shields.io/badge/docker-single_container-2496ED?logo=docker&logoColor=white)](Dockerfile "Ships as a single Docker container")
[![OpenAPI 3.1](https://img.shields.io/badge/API-OpenAPI_3.1-6BA539)](api/openapi.yaml "OpenAPI 3.1 specification")
[![37 search providers](https://img.shields.io/badge/built--in_search_providers-37-orange)](#providers "37 built-in search provider drivers")

**SerpentSeek** is an open-source, **self-hosted search gateway** — a unified
web-search API (metasearch proxy) written in Go with a multilingual web admin
UI built for **Open WebUI**. Every query runs through configurable chains of
**37 built-in search providers** (SearXNG, Brave, Google Vertex AI Search /
Grounding, Yandex, SerpApi, Serper, DataForSEO, Exa, Tavily, Kagi, Perplexity
and more), designed in a visual node-and-edge editor. Built in: request history
with live tracing (SSE), API keys and Passkeys (WebAuthn), an **MCP search
tool** (streamable HTTP) for AI assistants (Claude, Cursor, VS Code…), an
Open WebUI-compatible `/search` endpoint, SQLite ⟷ PostgreSQL storage and
background cleanup jobs. Everything ships in a single Docker container.

Project license — **GNU 3.0** (GPL-3.0-only, see `LICENSE`). Attribution lives in
`NOTICE`/`AUTHORS`; the licenses of all third-party components compiled into the
binary and the web bundle are collected in `THIRD_PARTY_LICENSES.md`. Report
vulnerabilities privately (see `SECURITY.md`); contribution rules are in
`CONTRIBUTING.md` (DCO sign-off + [CLA](CLA.md)).

The software is provided **"as is", without warranties of any kind**. Requests go
to third-party search APIs using the keys of whoever deployed the service; any
charges are a matter between the operator and their provider. The authors are not
liable for such charges or any other material damage (GPLv3 §§15–16, details in
`NOTICE`).

## Key features

* 🔎 **One endpoint for 37 search providers** — web/SERP (SearXNG, Brave,
  Google, Yandex, SerpApi…), AI/neural (Exa, Tavily, Perplexity, Kagi…),
  academic (OpenAlex, PubMed, Crossref, Semantic Scholar…) and enterprise
  (Azure AI Search, Vertex AI Search, Vectara…)
* 🧩 **Visual chain editor** — fallback/retry provider chains as a
  node-and-edge graph; multiple named instances per driver
* 🔌 **Open WebUI ready** — drop-in `/search` endpoint: always HTTP 200,
  `[{"link","title","snippet"}]`
* 🤖 **MCP server** — `search` tool over streamable HTTP for Claude, Cursor, VS
  Code and any other MCP client; plain JSON-RPC 2.0 is accepted too
* 🔭 **Live request tracing** — per-step timeline over SSE plus `X-Serpent-Rid`
  / `X-Serpent-Api` diagnostics headers and a History page
* 🔐 **Security first** — argon2id-hashed API keys, WebAuthn passkeys,
  SameSite=Strict sessions with CSRF, write-only secrets, optional AES-GCM
  encryption at rest
* 💾 **SQLite ⟷ PostgreSQL** — WAL-mode SQLite by default; the `extdb` compose
  profile brings PostgreSQL for multi-replica setups
* 🌍 **Multilingual UI** — English, Русский, Polski, Deutsch, Français, 中文
* 🐳 **Single Docker container** — Go backend with the embedded SvelteKit SPA

## Contents

* [Quick start](#quick-start)
  * [Open WebUI integration](#open-webui-integration)
  * [MCP integration (Model Context Protocol)](#mcp-integration-model-context-protocol)
* [Providers](#providers)
  * [SearXNG: external instance only](#searxng-external-instance-only)
* [PostgreSQL (profile `extdb`)](#postgresql-profile-extdb)
* [Settings](#settings)
* [Interface and localization](#interface-and-localization)
* [Authentication](#authentication)
* [Background jobs](#background-jobs)
* [Development](#development)
* [Dependencies and licenses](#dependencies-and-licenses)
* [Risks and caveats](#risks-and-caveats)
* [Screenshots](#screenshots)
* [Structure](#structure)

## Quick start

```bash
cp .env.example .env
# open .env and set provider keys if needed (SEARXNG_URL etc.)
docker compose up --build
```

UI: <http://localhost:8080>. Health check: `curl http://localhost:8080/healthz`.

Running the prebuilt image from Docker Hub instead of building from source —
see [DEPLOY.md](DEPLOY.md).

On first start a one-time token is printed to the logs:

```text
SETUP_TOKEN: ff1089a6
```

Open `/setup`, enter the token, create an administrator. The **API key is shown
only once** — save it. Then log in at `/login` with that key.

### Open WebUI integration

In Open WebUI → *Settings → Web Search* select **External** (or SearXNG) and set:

* URL: `http://<host>:8080/search`
* API key: your `seek_ak_...` key

The `POST /search` endpoint is Open WebUI-compatible: body `{"query":"...","count":5}`,
response is always HTTP 200 with an array `[{"link","title","snippet"}]` (empty
`[]` on failure/empty results). Diagnostics — via the `X-Serpent-Rid` and
`X-Serpent-Api` headers and the History page.

```bash
curl -s http://localhost:8080/search \
  -H "Authorization: Bearer seek_ak_..." \
  -H "Content-Type: application/json" \
  -d '{"query":"hello world","count":3}'
```

### MCP integration (Model Context Protocol)

SerpentSeek exposes a `search` tool over MCP at `/mcp` (official **streamable HTTP** transport: GET for SSE stream, POST for JSON-RPC, DELETE to close the session). The same endpoint also works as a plain HTTP JSON-RPC 2.0 request/response URL for clients that do not support the full streamable transport. Pass your API key as a Bearer token.

For **native HTTP MCP clients** (Claude Desktop, some Cursor builds, Open WebUI MCP tools, etc.):

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

For **stdio-only MCP clients** (older Cursor, VS Code extensions, etc.) use the `mcp-remote` wrapper so the remote HTTP endpoint looks like a local stdio server:

```bash
npx mcp-remote http://<host>:8080/mcp --header "Authorization=Bearer seek_ak_..."
```

Then point the client at the wrapper command instead of a URL.

The only tool is `search`, taking `query` (required) and `count` (optional,
0 = all, default 5). Each call runs the active provider chain and returns
`title` / `url` / `snippet` per result. The endpoint is described (with
ready-to-copy config snippets and a live test) on the **MCP** page of the admin
UI.

## Providers

Full list of built-in search drivers. Credentials are write-only and set per instance on the **Providers** page (only the `google_vertex` seed reads `GOOGLE_*` from the environment).

### Web search / SERP

| Code | Purpose | Credentials | Notes |
| --- | --- | --- | --- |
| `apiserpent` | apiserpent.com quick search | `api_key` | engine rotation google/bing/yahoo/ddg/brave |
| `serpbase` | serpbase.dev | `api_key` | configurable fatal/retry status tables |
| `searxng` | SearXNG — **external instance** | `api_key` (optional, gate token) | URL set in UI/`SEARXNG_URL`; needs `format=json` and `limiter: false` |
| `yandex` | Yandex Search API v2 (Yandex Cloud) | `api_key`, `folder_id` | `/v2/web/search`, base64 XML in `rawData` |
| `google_vertex` | Gemini Grounding with Google Search / Vertex AI Search | `api_key`, `project_id`, `engine_id` | primary Google path; env `GOOGLE_API_KEY`, `GOOGLE_MODEL`, `GOOGLE_DRIVER` |
| `google` | Google (dispatcher) | `api_key`, `cx`, `project_id`, `engine_id` | `driver` param switches vertex/cse |
| `google_cse` | Google Custom Search JSON API | `api_key`, `cx` | **deprecated**, shutdown 2027-01-01 |
| `brave` | Brave Search | `api_key` | |
| `serper` | Serper | `api_key` | |
| `serpapi` | SerpApi | `api_key` | multi-engine (`engine` param) |
| `searchapi` | SearchApi.io | `api_key` | |
| `dataforseo` | DataForSEO | `login`, `password` | HTTP Basic auth |
| `youcom` | You.com | `api_key` | |
| `mojeek` | Mojeek | `api_key` | key sent in the query string |
| `marginalia` | Marginalia | `api_key` | public key is rate-limited (503 retryable) |
| `hn` | Hacker News (Algolia) | — | no key required |

### AI / neural search

| Code | Purpose | Credentials | Notes |
| --- | --- | --- | --- |
| `exa` | Exa | `api_key` | highlights requested by default |
| `tavily` | Tavily | `api_key` | advanced search costs 2 credits |
| `jina` | Jina AI Search | `api_key` | query goes in the URL path |
| `firecrawl` | Firecrawl | `api_key` | |
| `linkup` | Linkup | `api_key` | `searchResults` output |
| `perplexity_search` | Perplexity Search API | `api_key` | standalone `/search`, not Sonar |
| `valyu` | Valyu | `api_key` | proprietary search needs a subscription |
| `parallel` | Parallel | `api_key` | `search_queries` built from the query |
| `kagi` | Kagi | `api_key` | FastGPT/enrich endpoints |

### Academic & developer

| Code | Purpose | Credentials | Notes |
| --- | --- | --- | --- |
| `openalex` | OpenAlex | — | abstract reconstructed from the inverted index |
| `semanticscholar` | Semantic Scholar | `api_key` (optional) | ~1 rps without a key |
| `crossref` | Crossref | — | DOI links normalized to `https://doi.org/` |
| `github` | GitHub Search | `api_key` | repositories/code/issues/commits/users |
| `stackexchange` | Stack Exchange | `api_key` (optional) | honours the API `backoff` |
| `pubmed` | PubMed (NCBI) | `api_key` (optional), env `NCBI_API_KEY` | two-step esearch→esummary (+ optional efetch) |

### Corporate / enterprise

| Code | Purpose | Credentials | Notes |
| --- | --- | --- | --- |
| `azure_search` | Azure AI Search | `api_key` or Entra (`tenant_id`, `client_id`, `client_secret`) | configurable `title_field`/`url_field`/`snippet_field` |
| `vertex_search` | Vertex AI Search (Discovery Engine) | `service_account_json` | OAuth2 only, no API keys |
| `vectara` | Vectara | `api_key` | generation disabled (Row output) |
| `kendra` | Amazon Kendra | `access_key_id`, `secret_access_key` | **deprecated** — AWS SigV4 not implemented |

### Generative answer nodes

These drivers belong on a **`mode: answer`** chain node: the generated text goes
to the request's answer panel, and sources become sparse rows (link+title).

| Code | Purpose | Credentials | Notes |
| --- | --- | --- | --- |
| `anthropic` | Anthropic web search | `api_key` | `web_search` tool + citations |
| `yandex_gen` | Yandex AI Studio (generative answer) | `api_key`, `folder_id` | `/v2/gen/search` |

Each provider's `base_url` is editable in the UI (not a secret). Secrets are
write-only: the value is never returned, only a "set" marker. The "Test" button
runs a probe query `q=test` and shows `http/kind/ms` plus the first links.

**Providers are instances.** On the Providers page you can add any number of
providers, including **several of the same type (driver) with different names
and `api_key`** — e.g. "apiserpent work" and "apiserpent personal". Each instance
is toggled on/off with a switch; disabled ones do not appear in the chain
editor's palette. Deleting an instance that is referenced by a chain is rejected
with `409` and the list of chains. When upgrading from an old DB version, the
previous records automatically become instances (`id = code`) and chain
references are preserved.

**On first start** only `google_vertex` (Vertex/Grounding) is created and
enabled, and the default chain consists of a single `google_vertex` block. No
other instances are created — add the drivers you need on the Providers page.

### SearXNG: external instance only

SerpentSeek **does not run SearXNG itself** — there is no such service in
compose. Point it at a URL of your already running SearXNG:

* in the `base_url` field of the `searxng` provider (Providers page, no restart), or
* via `SEARXNG_URL` in `.env`.

Both a plain URL (`http://host:port/search`) and a template with
`{query}`/`<query>` are supported. Until the URL is set, the provider is
considered dead (`skip`) and the chain continues without it.

Requirements for your instance: `search.formats: [html, json]` and
`limiter: false` (otherwise you will get 403/429). Example configuration —
`deploy/searxng/settings.yml`; the instance owner is the one who runs it.

> **⚠️ SearXNG is licensed under AGPLv3.** It is an external service;
> SerpentSeek only talks to it over HTTP and links nothing. Responsibility for
> complying with the SearXNG license lies with the person deploying it.

## PostgreSQL (profile `extdb`)

```bash
# in .env: POSTGRES_PASSWORD=... STORAGE_DRIVER=postgres \
#          DATABASE_URL=postgres://serpent:...@postgres:5432/serpentseek?sslmode=disable
docker compose --profile extdb up -d --build
```

PostgreSQL is a separate container (PostgreSQL License), **not** a code
dependency of SerpentSeek. SQLite is the default: `journal_mode=WAL`,
`foreign_keys=ON`, `busy_timeout=5000`, `synchronous=NORMAL`, single writer.
Multi-replica deployments require PostgreSQL.

## Settings

All variables are listed in [`.env.example`](.env.example). Environment values
take precedence; such fields are shown in the UI with an `env` badge and are
read-only. All other settings can be changed on the Settings page without a
restart (except `STORAGE_DRIVER`, which the UI honestly warns about).

Provider keys (`api_key`, `folder_id`, `cx`, …) and `base_url` are **instance
properties**: they are set on the Providers page. The only environment
variables that seed a provider on first start are `GOOGLE_API_KEY`,
`GOOGLE_MODEL` and `GOOGLE_DRIVER` (for the default `google_vertex` instance);
all other credentials are per-instance and have no env defaults. On the Settings
page, providers keep only the **retry codes** (`RETRY_HTTP_CODES`,
`AP_RETRY_CODES`) — global defaults that every instance can override via its own
`params`. The fatal-code tables (`fatal_codes`, `fatal_http`) are per-instance
params and are edited on the Providers page.

Key variables: `PORT`, `HOST`, `DATA_DIR`/`SQLITE_PATH`, `STORAGE_DRIVER`,
`DATABASE_URL`, `AUTH_ENABLED`, `PUBLIC_ORIGIN`, `RP_ID`, `RP_NAME`,
`HISTORY_RETENTION_DAYS`, `LOGS_RETENTION_DAYS`, `MAX_CONCURRENCY`, `MAX_ATTEMPTS`,
`USER_AGENT`, `LOG_LEVEL`, `LOG_FORMAT`, `TREAT_EMPTY_AS_FAIL`.

`IGNORE_REQUEST_COUNT=true` makes the gateway ignore the request's `count` and
pass `num=0` to providers — return everything found without a limit. The same
value (`0`) can be set manually in the "Results" field on the **Playground**
page (a hint appears when hovering over the field).

## Interface and localization

The UI is translated into 6 languages:

| Code | Language |
| --- | --- |
| `ru` | Русский |
| `en` | English |
| `pl` | Polski |
| `de` | Deutsch |
| `fr` | Français |
| `zh` | 中文 |

The language switcher is in the top right corner, next to the "Quick search"
button. The selected language is stored in `localStorage`; on first open it is
detected from `navigator.language`, otherwise English is used. Dates are
formatted according to the rules of the selected locale (`Intl.DateTimeFormat`).

To add a new language: drop a dictionary into `web/src/lib/i18n/<code>.json`
(the key set must match `en.json`), add an entry to
`web/src/lib/i18n/languages.json` and the code to the `Locale` type in
`web/src/lib/i18n.svelte.ts`.

## Authentication

* **Keys:** `seek_ak_<prefix>_<secret>` (scope search) and `seek_admin_<prefix>_<secret>`
  (scope admin). Only an argon2id hash is stored; only the prefix is visible in
  the UI/API.
* **Sessions:** httpOnly + Secure + SameSite=Strict cookie, rolling 30 days, CSRF
  double-submit on all mutations.
* **Passkeys (WebAuthn):** register after signing in with a key, then sign in
  passwordless. Requires HTTPS + a domain (`PUBLIC_ORIGIN`/`RP_ID`); over plain
  HTTP/IP the button is hidden with a warning.
* **`AUTH_ENABLED=false`** — the entire admin API works without sign-in (a banner
  is shown in the UI).

## Background jobs

* `cleanup` (`HISTORY_CLEANUP_CRON`, default `0 4 * * *`) — deletes history and
  logs older than the retention period, expired sessions, and recomputes daily
  aggregates.
* `vacuum` (`VACUUM_CRON`, default `0 3 * * 0`) — SQLite only.

Both can be triggered manually from Settings → Maintenance (or via
`POST /api/maintenance/*`). A panicking job does not crash the process.

## Development

```bash
make build      # SPA + Go binary (embed)
make test       # go test ./... -cover
make check      # svelte-check
make lint       # go vet + golangci-lint
make dev        # Vite dev server proxied to :8080
```

The backend listens on `:8080`; the SPA is embedded via `go:embed web/build`.

## Dependencies and licenses

Only MIT / Apache-2.0 / BSD-2/3 / ISC are allowed. Audit:

```bash
go run github.com/google/go-licenses@latest report ./...
cd web && npx license-checker --production --excludePrivatePackages
```

| Dependency | Version | License |
| --- | --- | --- |
| github.com/go-chi/chi/v5 | v5.3.2 | MIT |
| modernc.org/sqlite | v1.59.0 | BSD-3-Clause |
| github.com/jackc/pgx/v5 | v5.11.0 | MIT |
| github.com/pressly/goose/v3 | v3.28.0 | MIT |
| github.com/go-webauthn/webauthn | v0.18.1 | BSD-3-Clause |
| golang.org/x/crypto | v0.57.0 | BSD-3-Clause |
| golang.org/x/time | v0.16.0 | BSD-3-Clause |
| github.com/robfig/cron/v3 | v3.0.1 | MIT |
| github.com/google/uuid | v1.6.0 | BSD-3-Clause |
| svelte | 5.57.0 | MIT |
| @sveltejs/kit | 2.70.3 | MIT |
| @sveltejs/adapter-static | 3.0.10 | MIT |
| vite | 8.3.0 | MIT |
| typescript | 5.9.3 | Apache-2.0 |
| svelte-check | 4.7.6 | MIT |
| @xyflow/svelte | 1.6.6 | MIT |
| tailwindcss | 4.3.3 | MIT |
| @tailwindcss/vite | 4.3.3 | MIT |
| @simplewebauthn/browser | 14.0.0 | MIT |
| uqr | 0.1.3 | MIT |

Satellite containers (not code dependencies): `postgres:17-alpine` (PostgreSQL
License) and external `searxng/searxng` (not part of this repository; **AGPLv3**
— a separate service, not a code dependency).

## Risks and caveats

* **Passkeys require HTTPS + a domain** (WebAuthn rpID). Over a bare IP/HTTP only
  keys are available; the UI warns about this.
* **Google Custom Search JSON API** is closed to new customers and shuts down on
  **2027-01-01** — the driver is marked deprecated, the default is
  Vertex/Grounding.
* **Providers are paid services.** All requests are made with the **API keys of
  whoever deployed the service** and are billed by the provider (Yandex Cloud,
  Google Cloud, apiserpent, SerpBase). Configure budgets, alerts and key quotas
  **on the provider side** and keep retries under control (max attempts, retry
  policy in the chain): the project authors are not liable for charges (see
  `NOTICE`).
* **Yandex Search API v2** is paid: an Api-Key + folderId (Yandex Cloud) are
  required.
* **SearXNG** is an external service: `search.formats: [html, json]` and
  `limiter: false` are required; the gate token (`X-API-Key`) is **not**
  `server.secret_key`. SearXNG itself is licensed under **AGPLv3** and is not
  part of this repository.
* **SQLite** is single-writer; WAL + busy_timeout is enough for one replica.
* **Provider credentials** are write-only and masked in the API and logs;
  `base_url` is not a secret. Optional `ENCRYPTION_KEY` (AES-GCM) for at-rest
  encryption.

## Screenshots

After startup you get the following screens: Dashboard, History (with a colored
chain graph and a step timeline), Chains (visual editor), Providers, Playground,
MCP, Logs, Users, Settings. Add your screenshots to `docs/screenshots/` (they are
not included in the repository to avoid bloating the image).

## Structure

```
cmd/serpentseek/      # wiring and seed
internal/
  config logging store providers engine sse auth httpd jobs version
api/openapi.yaml      # OpenAPI 3.1
web/                  # SvelteKit SPA (build is embedded into the binary)
deploy/
  searxng/settings.yml
  caddy.example
```

---

<sub>**Keywords:** self-hosted search gateway, unified search API, metasearch proxy, meta search engine, Open WebUI web search, Open WebUI search backend, SearXNG JSON API, MCP server search tool, Model Context Protocol web search, LLM web search API, RAG search backend, Brave Search API, Google Vertex AI Search, Grounding with Google Search, Yandex Search API, SerpApi, Serper, DataForSEO, Exa, Tavily, Kagi, Perplexity Search API, search provider fallback chain, search aggregator, OpenAPI 3.1, Go microservice, single Docker container.</sub>
