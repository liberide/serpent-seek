# 🐍 SerpentSeek — Self-hosted Search-Gateway für Open WebUI, MCP & 39+ Such-APIs

[English](README.md "Read in English") | [Русский](README.ru.md "README по-русски") | **Deutsch** | [Français](README.fr.md "Lire en français") | [中文](README.zh.md "阅读中文版")

[![License: GPL-3.0](https://img.shields.io/badge/license-GPL--3.0-blue.svg)](LICENSE "Projektlizenz — GNU GPL v3.0")
[![Go 1.26](https://img.shields.io/badge/Go-1.26-00ADD8?logo=go&logoColor=white)](go.mod "Backend in Go geschrieben")
[![CI](https://github.com/liberide/serpent-seek/actions/workflows/ci.yml/badge.svg)](https://github.com/liberide/serpent-seek/actions/workflows/ci.yml "CI: Build, Vet, Tests")
[![Docker — single container](https://img.shields.io/badge/docker-single_container-2496ED?logo=docker&logoColor=white)](Dockerfile "Alles in einem einzigen Docker-Container")
[![OpenAPI 3.1](https://img.shields.io/badge/API-OpenAPI_3.1-6BA539)](api/openapi.yaml "OpenAPI-3.1-Spezifikation")
[![39 search providers](https://img.shields.io/badge/built--in_search_providers-39-orange)](#suchanbieter "39 eingebaute Suchanbieter")

**SerpentSeek** ist ein Open-Source-**Search-Gateway** zum Selbsthosten — eine
vereinheitlichte Websuche-API (Metasuch-Proxy) in Go mit mehrsprachiger
Web-Admin-UI für **Open WebUI**. Jede Anfrage läuft über konfigurierbare Ketten
aus **39 eingebauten Suchanbietern** (SearXNG, Brave, Google Vertex AI Search /
Grounding, Yandex, SerpApi, Serper, DataForSEO, Exa, Tavily, Kagi, Perplexity
u. a.), die in einem visuellen Knoten-und-Kanten-Editor zusammengestellt
werden. An Bord: Anfrageverlauf mit Live-Tracing (SSE), API-Schlüssel und
Passkeys (WebAuthn), ein **MCP-Suchtool** (streamable HTTP) für KI-Assistenten
(Claude, Cursor, VS Code…), ein Open WebUI-kompatibler `/search`-Endpunkt,
SQLite ⟷ PostgreSQL und Hintergrundjobs für die Bereinigung. Alles ist in einem
einzigen Docker-Container verpackt.

Projektlizenz — **GNU 3.0** (GPL-3.0-only, siehe `LICENSE`). Die Urheberschaft
steht in `NOTICE`/`AUTHORS`; die Lizenzen aller in die Binärdatei und das
Web-Bundle eingebetteten Drittkomponenten sind in `THIRD_PARTY_LICENSES.md`
zusammengefasst. Sicherheitslücken bitte privat melden (siehe `SECURITY.md`);
die Beitragsregeln stehen in `CONTRIBUTING.md` (DCO-Sign-off + [CLA](CLA.md)).

Die Software wird **„wie besehen“ (AS IS), ohne jegliche Gewährleistung**
bereitgestellt. Anfragen gehen mit den Schlüsseln der Person an externe
Such-APIs, die den Dienst bereitgestellt hat; etwaige Abbuchungen sind eine
Angelegenheit zwischen dem Betreiber und seinem Anbieter. Die Autoren haften
nicht für solche Abbuchungen oder sonstige materielle Schäden (GPLv3 §§15–16,
Details in `NOTICE`).

## Funktionen

* 🔎 **Ein Endpunkt für 39 Suchanbieter** — Web/SERP (SearXNG, Brave, Google,
  Yandex, SerpApi…), KI/neural (Exa, Tavily, Perplexity, Kagi…),
  wissenschaftlich (OpenAlex, PubMed, Crossref, Semantic Scholar…) und
  Enterprise (Azure AI Search, Vertex AI Search, Vectara…)
* 🧩 **Visueller Ketten-Editor** — Fallback-/Retry-Ketten als
  Knoten-und-Kanten-Graph; mehrere benannte Instanzen pro Treiber
* 🔌 **Bereit für Open WebUI** — fertiger `/search`-Endpunkt: immer HTTP 200,
  `[{"link","title","snippet"}]`
* 🤖 **MCP-Server** — `search`-Tool über streamable HTTP für Claude, Cursor, VS
  Code und andere MCP-Clients; akzeptiert auch einfaches JSON-RPC 2.0
* 🔭 **Live-Tracing** — Schritt-Timeline pro Anfrage über SSE, Diagnose-Header
  `X-Serpent-Rid` / `X-Serpent-Api` und Verlaufsseite
* 🔐 **Sicherheit** — argon2id-gehashte API-Schlüssel, Passkeys (WebAuthn),
  SameSite=Strict-Sessions mit CSRF, Write-only-Secrets, optional AES-GCM
* 💾 **SQLite ⟷ PostgreSQL** — SQLite im WAL-Modus standardmäßig; das Profil
  `extdb` bringt PostgreSQL für Multi-Replika-Setups
* 🌍 **Mehrsprachige UI** — English, Русский, Polski, Deutsch, Français, 中文
* 🐳 **Ein Docker-Container** — Go-Backend mit eingebetteter SvelteKit-SPA

## Inhalt

* [Schnellstart](#schnellstart)
  * [Integration mit Open WebUI](#integration-mit-open-webui)
  * [MCP-Integration (Model Context Protocol)](#mcp-integration-model-context-protocol)
* [Suchanbieter](#suchanbieter)
  * [SearXNG: nur externe Instanz](#searxng-nur-externe-instanz)
* [PostgreSQL (Profil `extdb`)](#postgresql-profil-extdb)
* [Einstellungen](#einstellungen)
* [Oberfläche und Lokalisierung](#oberfläche-und-lokalisierung)
* [Authentifizierung](#authentifizierung)
* [Hintergrundjobs](#hintergrundjobs)
* [Entwicklung](#entwicklung)
* [Abhängigkeiten und Lizenzen](#abhängigkeiten-und-lizenzen)
* [Risiken und Vorbehalte](#risiken-und-vorbehalte)
* [Screenshots](#screenshots)
* [Struktur](#struktur)

## Schnellstart

```bash
cp .env.example .env
# .env öffnen und bei Bedarf Anbieter-Schlüssel setzen (SEARXNG_URL usw.)
docker compose up --build
```

UI: <http://localhost:8080>. Health-Check: `curl http://localhost:8080/healthz`.

Um das vorgefertigte Image von Docker Hub zu verwenden, statt aus dem Quellcode
zu bauen — siehe [DEPLOY.md](DEPLOY.md).

Beim ersten Start wird ein Einmal-Token in die Logs geschrieben:

```text
SETUP_TOKEN: ff1089a6
```

Öffne `/setup`, gib das Token ein und lege einen Administrator an. Der gezeigte
**API-Schlüssel wird nur einmal ausgegeben** — speichere ihn. Melde dich danach
unter `/login` mit diesem Schlüssel an.

### Integration mit Open WebUI

In Open WebUI → *Settings → Web Search* **External** (oder SearXNG) wählen und
eintragen:

* URL: `http://<host>:8080/search`
* API key: dein Schlüssel `seek_ak_...`

Der Endpunkt `POST /search` ist Open-WebUI-kompatibel: Body
`{"query":"...","count":5}`, Antwort ist immer HTTP 200 mit einem Array
`[{"link","title","snippet"}]` (leeres `[]` bei Fehler/leerer Trefferliste).
Diagnose — über die Header `X-Serpent-Rid` und `X-Serpent-Api` sowie die Seite
History.

```bash
curl -s http://localhost:8080/search \
  -H "Authorization: Bearer seek_ak_..." \
  -H "Content-Type: application/json" \
  -d '{"query":"hello world","count":3}'
```

### MCP-Integration (Model Context Protocol)

SerpentSeek stellt ein `search`-Tool über MCP unter `/mcp` bereit (offizieller
**streamable-HTTP**-Transport: GET für SSE-Stream, POST für JSON-RPC, DELETE
zum Schließen der Sitzung). Derselbe Endpunkt arbeitet auch als einfacher
HTTP-JSON-RPC-2.0-Request/Response-URL für Clients ohne vollständige
streamable-HTTP-Unterstützung. Übergeben Sie den API-Schlüssel als Bearer-Token.

Für **native HTTP-MCP-Clients** (Claude Desktop, neuere Cursor-Builds,
Open-WebUI-MCP-Tools usw.):

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

Für **nur-stdio-MCP-Clients** (älterer Cursor, VS-Code-Erweiterungen usw.)
verwenden Sie den Wrapper `mcp-remote`, damit der entfernte HTTP-Endpunkt wie ein
lokaler Stdio-Server aussieht:

```bash
npx mcp-remote http://<host>:8080/mcp --header "Authorization=Bearer seek_ak_..."
```

Geben Sie dem Client dann diesen Befehl statt einer URL an.

Das einzige Tool ist `search` mit den Argumenten `query` (erforderlich) und
`count` (optional, 0 = alle, Standard 5). Jeder Aufruf nutzt die aktive
Anbieter-Kette und liefert `title` / `url` / `snippet` je Ergebnis. Eine
Beschreibung des Endpunkts (fertige Konfig-Snippets und Live-Test) finden Sie auf
der Seite **MCP** in der Admin-Oberfläche.

## Suchanbieter

Vollständige Liste der eingebauten Treiber. Anmeldedaten sind write-only und
werden pro Instanz auf der Seite **Providers** gesetzt (nur `google_vertex` wird
beim ersten Start über `GOOGLE_*` aus der Umgebung befüllt).

### Websuche / SERP

| Code | Zweck | Anmeldedaten | Hinweise |
| --- | --- | --- | --- |
| `apiserpent` | apiserpent.com quick search | `api_key` | Engine-Rotation google/bing/yahoo/ddg/brave |
| `serpbase` | serpbase.dev | `api_key` | konfigurierbare Fatal-/Retry-Tabellen |
| `searxng` | SearXNG — **externe Instanz** | `api_key` (optional, Gate-Token) | URL in UI/`SEARXNG_URL`; benötigt `format=json` und `limiter: false` |
| `yandex` | Yandex Search API v2 (Yandex Cloud) | `api_key`, `folder_id` | `/v2/web/search`, Base64-XML in `rawData` |
| `google_vertex` | Gemini Grounding / Vertex AI Search | `api_key`, `project_id`, `engine_id` | primärer Google-Pfad; Env `GOOGLE_API_KEY`, `GOOGLE_MODEL`, `GOOGLE_DRIVER` |
| `google` | Google (Dispatcher) | `api_key`, `cx`, `project_id`, `engine_id` | `driver`-Parameter schaltet vertex/cse |
| `google_cse` | Google Custom Search JSON API | `api_key`, `cx` | **deprecated**, Abschaltung am 01.01.2027 |
| `brave` | Brave Search | `api_key` | |
| `serper` | Serper | `api_key` | |
| `serpapi` | SerpApi | `api_key` | Multi-Engine (`engine`) |
| `searchapi` | SearchApi.io | `api_key` | |
| `dataforseo` | DataForSEO | `login`, `password` | HTTP Basic Auth |
| `youcom` | You.com | `api_key` | |
| `mojeek` | Mojeek | `api_key` | Schlüssel in der Query |
| `marginalia` | Marginalia | `api_key` | öffentlicher Schlüssel ratenbegrenzt (503 wiederholbar) |
| `hn` | Hacker News (Algolia) | — | kein Schlüssel |

### KI / neuronale Suche

| Code | Zweck | Anmeldedaten | Hinweise |
| --- | --- | --- | --- |
| `exa` | Exa | `api_key` | standardmäßig Highlights |
| `tavily` | Tavily | `api_key` | Advanced-Suche = 2 Credits |
| `jina` | Jina AI Search | `api_key` | Query im URL-Pfad |
| `firecrawl` | Firecrawl | `api_key` | |
| `linkup` | Linkup | `api_key` | `searchResults`-Ausgabe |
| `perplexity_search` | Perplexity Search API | `api_key` | eigenständiges `/search`, nicht Sonar |
| `valyu` | Valyu | `api_key` | proprietäre Suche per Abo |
| `parallel` | Parallel | `api_key` | `search_queries` aus der Query |
| `kagi` | Kagi | `api_key` | FastGPT/Enrich |
| `kimi` | Kimi (Moonshot) Web Search | `api_key` | Titel/URL/Snippet; `include_content` lädt Seitentext |
| `kimi_pro` | Kimi (Moonshot) Web Search Pro | `api_key` | gerankte Content-Chunks; `sites`/`time_window`-Filter |

### Wissenschaft & Entwicklung

| Code | Zweck | Anmeldedaten | Hinweise |
| --- | --- | --- | --- |
| `openalex` | OpenAlex | — | Abstract aus dem Inverted Index |
| `semanticscholar` | Semantic Scholar | `api_key` (optional) | ~1 rps ohne Schlüssel |
| `crossref` | Crossref | — | DOI-Links normalisiert |
| `github` | GitHub Search | `api_key` | Repos/Code/Issues/Commits/Users |
| `stackexchange` | Stack Exchange | `api_key` (optional) | berücksichtigt `backoff` |
| `pubmed` | PubMed (NCBI) | `api_key` (optional), Env `NCBI_API_KEY` | zweistufig esearch→esummary (+ efetch) |

### Unternehmenssuche

| Code | Zweck | Anmeldedaten | Hinweise |
| --- | --- | --- | --- |
| `azure_search` | Azure AI Search | `api_key` oder Entra (`tenant_id`, `client_id`, `client_secret`) | konfigurierbare `title_field`/`url_field`/`snippet_field` |
| `vertex_search` | Vertex AI Search (Discovery Engine) | `service_account_json` | nur OAuth2, keine API-Keys |
| `vectara` | Vectara | `api_key` | Generation deaktiviert (Row) |
| `kendra` | Amazon Kendra | `access_key_id`, `secret_access_key` | **deprecated** — SigV4 nicht implementiert |

### Answer-Knoten (generative Antworten)

Diese Treiber gehören auf einen **`mode: answer`**-Knoten: Der generierte Text
landet im Antwort-Panel der Anfrage, Quellen werden zu sparse Rows (Link+Titel).

| Code | Zweck | Anmeldedaten | Hinweise |
| --- | --- | --- | --- |
| `anthropic` | Anthropic web search | `api_key` | `web_search`-Tool + Citations |
| `yandex_gen` | Yandex AI Studio (generative Antwort) | `api_key`, `folder_id` | `/v2/gen/search` |

Die `base_url` jedes Anbieters ist in der UI bearbeitbar (kein Geheimnis).
Geheimnisse sind write-only: Der Wert wird nie zurückgegeben, nur ein
„gesetzt“-Marker. Die Schaltfläche „Testen“ führt eine Probeanfrage `q=test`
aus und zeigt `http/kind/ms` sowie die ersten Links.

**Anbieter sind Instanzen.** Auf der Seite Providers lassen sich beliebig viele
Anbieter hinzufügen, auch **mehrere desselben Typs (Drivers) mit
unterschiedlichen Namen und `api_key`** — etwa „apiserpent work“ und „apiserpent
personal“. Jede Instanz wird per Schalter aktiviert/deaktiviert; deaktivierte
erscheinen nicht in der Palette des Chain-Editors. Das Löschen einer Instanz,
die in einer Chain verwendet wird, wird mit `409` und der Liste der Chains
abgelehnt. Beim Upgrade von einer alten DB-Version werden die bisherigen
Einträge automatisch zu Instanzen (`id = code`), die Referenzen in den Chains
bleiben erhalten.

**Beim ersten Start** wird nur `google_vertex` (Vertex/Grounding) angelegt und
aktiviert; die Standard-Chain besteht aus einem einzigen `google_vertex`-Block.
Weitere Instanzen werden nicht angelegt — füge die benötigten Treiber auf der
Seite „Providers“ hinzu.

### SearXNG: nur externe Instanz

SerpentSeek **startet SearXNG nicht selbst** — in compose gibt es keinen solchen
Dienst. Gib die URL deiner bereits laufenden SearXNG-Instanz an:

* im Feld `base_url` des Anbieters `searxng` (Seite Providers, ohne Neustart), oder
* über `SEARXNG_URL` in `.env`.

Unterstützt werden eine reine URL (`http://host:port/search`) und ein Template
mit `{query}`/`<query>`. Solange keine URL gesetzt ist, gilt der Anbieter als
ausgefallen (`skip`), und die Chain läuft weiter.

Anforderungen an deine Instanz: `search.formats: [html, json]` und
`limiter: false` (sonst gibt es 403/429). Eine Beispielkonfiguration liegt in
`deploy/searxng/settings.yml`; betrieben wird sie vom Instanz-Inhaber.

> **⚠️ Die Lizenz von SearXNG ist AGPLv3.** Es handelt sich um einen externen
> Dienst; SerpentSeek kommuniziert nur über HTTP und linkt nichts. Die
> Verantwortung für die Einhaltung der SearXNG-Lizenz liegt bei der Person,
> die ihn bereitstellt.

## PostgreSQL (Profil `extdb`)

```bash
# in .env: POSTGRES_PASSWORD=... STORAGE_DRIVER=postgres \
#          DATABASE_URL=postgres://serpent:...@postgres:5432/serpentseek?sslmode=disable
docker compose --profile extdb up -d --build
```

PostgreSQL ist ein separater Container (PostgreSQL License) und **keine**
Code-Abhängigkeit von SerpentSeek. SQLite ist der Standard: `journal_mode=WAL`,
`foreign_keys=ON`, `busy_timeout=5000`, `synchronous=NORMAL`, ein einziger
Writer. Multi-Replica-Betrieb ist nur mit PostgreSQL möglich.

## Einstellungen

Alle Variablen sind in [`.env.example`](.env.example) aufgelistet. Werte aus der
Umgebung haben Vorrang; solche Felder sind in der UI mit einem `env`-Badge
markiert und read-only. Alle übrigen Einstellungen lassen sich auf der Seite
Settings ohne Neustart ändern (außer `STORAGE_DRIVER` — darauf weist die UI
ehrlich hin).

Anbieter-Schlüssel (`api_key`, `folder_id`, `cx`, …) und `base_url` sind
**Eigenschaften der Instanz**: Sie werden auf der Seite Providers gesetzt. Die
einzigen Umgebungsvariablen, die beim ersten Start einen Anbieter befüllen, sind
`GOOGLE_API_KEY`, `GOOGLE_MODEL` und `GOOGLE_DRIVER` (für die Standard-Instanz
`google_vertex`); alle anderen Anmeldedaten sind instanzbezogen und haben keine
Env-Defaults. Auf der Seite Settings verbleiben bei den Anbietern nur die
**Wiederholungscodes** (`RETRY_HTTP_CODES`, `AP_RETRY_CODES`) — globale Defaults,
die jede Instanz über ihre eigenen `params` überschreiben kann. Die Tabellen der
fatalen Codes (`fatal_codes`, `fatal_http`) sind Instanz-Parameter und werden
auf der Seite Providers bearbeitet.

Wichtige Variablen: `PORT`, `HOST`, `DATA_DIR`/`SQLITE_PATH`, `STORAGE_DRIVER`,
`DATABASE_URL`, `AUTH_ENABLED`, `PUBLIC_ORIGIN`, `RP_ID`, `RP_NAME`,
`HISTORY_RETENTION_DAYS`, `LOGS_RETENTION_DAYS`, `MAX_CONCURRENCY`, `MAX_ATTEMPTS`,
`USER_AGENT`, `LOG_LEVEL`, `LOG_FORMAT`, `TREAT_EMPTY_AS_FAIL`.

`IGNORE_REQUEST_COUNT=true` lässt das Gateway das `count` der Anfrage ignorieren
und übergibt den Anbietern `num=0` — alle gefundenen Ergebnisse ohne Limit
zurückgeben. Derselbe Wert (`0`) lässt sich manuell im Feld „Ergebnisse“ auf der
Seite **Playground** eintragen (ein Hinweis erscheint beim Überfahren des
Feldes).

## Oberfläche und Lokalisierung

Die UI ist in 6 Sprachen übersetzt:

| Code | Sprache |
| --- | --- |
| `ru` | Русский |
| `en` | English |
| `pl` | Polski |
| `de` | Deutsch |
| `fr` | Français |
| `zh` | 中文 |

Der Sprachumschalter befindet sich oben rechts, neben der Schaltfläche
„Schnellsuche“. Die gewählte Sprache wird in `localStorage` gespeichert; beim
ersten Öffnen wird sie aus `navigator.language` ermittelt, sonst Englisch
verwendet. Datumsangaben werden nach den Regeln der gewählten Locale formatiert
(`Intl.DateTimeFormat`).

So fügst du eine neue Sprache hinzu: Lege ein Wörterbuch
`web/src/lib/i18n/<code>.json` ab (die Schlüsselmenge muss mit `en.json`
übereinstimmen), trage einen Eintrag in `web/src/lib/i18n/languages.json` ein
und ergänze den Code im Typ `Locale` in `web/src/lib/i18n.svelte.ts`.

## Authentifizierung

* **Schlüssel:** `seek_ak_<prefix>_<secret>` (Scope search) und
  `seek_admin_<prefix>_<secret>` (Scope admin). Gespeichert wird nur ein
  argon2id-Hash; in UI/API ist nur das Prefix sichtbar.
* **Sessions:** httpOnly + Secure + SameSite=Strict-Cookie, rolling 30 Tage,
  CSRF-Double-Submit bei allen Mutationen.
* **Passkeys (WebAuthn):** Registrierung nach der Anmeldung per Schlüssel,
  danach passwortlose Anmeldung. Erfordern HTTPS + Domain
  (`PUBLIC_ORIGIN`/`RP_ID`); über HTTP/IP wird die Schaltfläche mit einer
  Warnung ausgeblendet.
* **`AUTH_ENABLED=false`** — die gesamte Admin-API funktioniert ohne Anmeldung
  (Banner in der UI).

## Hintergrundjobs

* `cleanup` (`HISTORY_CLEANUP_CRON`, Standard `0 4 * * *`) — löscht Verlauf und
  Logs oberhalb der Retention, abgelaufene Sessions, und berechnet die
  Tagesaggregate neu.
* `vacuum` (`VACUUM_CRON`, Standard `0 3 * * 0`) — nur SQLite.

Beide lassen sich manuell unter Settings → Wartung starten (oder über
`POST /api/maintenance/*`). Eine Panik in einem Job bringt den Prozess nicht
zum Absturz.

## Entwicklung

```bash
make build      # SPA + Go-Binary (Embed)
make test       # go test ./... -cover
make check      # svelte-check
make lint       # go vet + golangci-lint
make dev        # Vite-Dev-Server mit Proxy auf :8080
```

Das Backend lauscht auf `:8080`; die SPA wird über `go:embed web/build`
eingebettet.

## Abhängigkeiten und Lizenzen

Erlaubt sind nur MIT / Apache-2.0 / BSD-2/3 / ISC. Audit:

```bash
go run github.com/google/go-licenses@latest report ./...
cd web && npx license-checker --production --excludePrivatePackages
```

| Abhängigkeit | Version | Lizenz |
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

Satelliten-Container (keine Code-Abhängigkeiten): `postgres:17-alpine`
(PostgreSQL License) und das externe `searxng/searxng` (nicht Teil dieses
Repositories; **AGPLv3** — ein separater Dienst, keine Code-Abhängigkeit).

## Risiken und Vorbehalte

* **Passkeys erfordern HTTPS + Domain** (WebAuthn-rpID). Über reine IP/HTTP sind
  nur Schlüssel verfügbar; die UI weist darauf hin.
* **Google Custom Search JSON API** ist für neue Kunden geschlossen und wird am
  **01.01.2027** abgeschaltet — der Treiber ist als deprecated markiert,
  Standard ist Vertex/Grounding.
* **Anbieter sind kostenpflichtig.** Alle Anfragen laufen mit den **API-Schlüsseln
  der Person, die den Dienst bereitgestellt hat**, und werden vom Anbieter
  abgerechnet (Yandex Cloud, Google Cloud, apiserpent, SerpBase). Richte
  Budgets, Alerts und Schlüssel-Quotas **auf Anbieterseite** ein und behalte
  Retries im Blick (max attempts, Retry-Policy in der Chain): Die
  Projektautoren haften nicht für Abbuchungen (siehe `NOTICE`).
* **Yandex Search API v2** ist kostenpflichtig: Api-Key + folderId (Yandex
  Cloud) erforderlich.
* **SearXNG** ist ein externer Dienst: `search.formats: [html, json]` und
  `limiter: false` werden benötigt; das Gate-Token (`X-API-Key`) ist **nicht**
  der `server.secret_key`. SearXNG selbst ist unter **AGPLv3** lizenziert und
  nicht Teil dieses Repositories.
* **SQLite** ist Single-Writer; WAL + busy_timeout reichen für eine Replik.
* **Anbieter-Credentials** sind write-only und werden in API und Logs maskiert;
  `base_url` ist kein Geheimnis. Optional `ENCRYPTION_KEY` (AES-GCM) für die
  Verschlüsselung auf der Platte.

## Screenshots

Nach dem Start stehen folgende Ansichten zur Verfügung: Dashboard, History (mit
farbiger Chain-Grafik und Schritt-Timeline), Chains (visueller Editor),
Providers, Playground, MCP, Logs, Users, Settings. Lege eigene Screenshots unter
`docs/screenshots/` ab (sie sind nicht im Repository enthalten, um das Image
nicht aufzublähen).

## Struktur

```
cmd/serpentseek/      # Wiring und Seed
internal/
  config logging store providers engine sse auth httpd jobs version
api/openapi.yaml      # OpenAPI 3.1
web/                  # SvelteKit SPA (Build wird ins Binary eingebettet)
deploy/
  searxng/settings.yml
  caddy.example
```

---

<sub>**Schlüsselwörter:** Search-Gateway, self-hosted, einheitliche Such-API, Metasuche, Metasuch-Proxy, Open WebUI Websuche, Such-Backend für Open WebUI, SearXNG, MCP-Server, MCP Suchtool, Model Context Protocol Websuche, Websuche für LLMs und KI-Assistenten, RAG-Suche, Brave Search API, Google Vertex AI Search, Grounding with Google Search, Yandex Search API, SerpApi, Serper, DataForSEO, Exa, Tavily, Kagi, Perplexity Search API, Fallback-Ketten für Suchanbieter, Such-Aggregator, OpenAPI 3.1, Go-Mikroservice, ein Docker-Container.</sub>
