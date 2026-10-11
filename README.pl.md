# 🐍 SerpentSeek — samodzielnie hostowana brama wyszukiwania dla Open WebUI, MCP i 40+ API

[English](README.md "Read in English") | [Русский](README.ru.md) | [Deutsch](README.de.md) | [Français](README.fr.md) | [日本語](README.ja.md) | [中文](README.zh.md)

> To skrócona wersja polska. Pełny, aktualny opis funkcji, konfiguracji i
> zastrzeżeń znajduje się w [README.md](README.md).

**SerpentSeek** to open-source'owa, **samodzielnie hostowana brama
wyszukiwania** — ujednolicone API wyszukiwania w sieci (proxy metasearch)
napisane w Go, z wielojęzycznym panelem administracyjnym, stworzone dla
**Open WebUI**. Każde zapytanie przechodzi przez konfigurowalne łańcuchy
**40 wbudowanych dostawców** (SearXNG, Brave, Google Vertex AI Search /
Grounding, Yandex, SerpApi, Serper, DataForSEO, Exa, Tavily, Kagi, Perplexity
i inne), budowane w wizualnym edytorze węzłów i krawędzi. W zestawie: historia
zapytań z podglądem na żywo (SSE), klucze API i passkeys (WebAuthn), **narzędzie
MCP `search`** (streamable HTTP) dla asystentów AI, endpoint `/search`
kompatybilny z Open WebUI, magazyn SQLite ⟷ PostgreSQL oraz zadania
porządkowe w tle. Wszystko w jednym kontenerze Docker.

Licencja projektu — **GNU GPLv3** (`LICENSE`). Atrybucje: `NOTICE`/`AUTHORS`;
licencje komponentów: `THIRD_PARTY_LICENSES.md`. Wersje: [CHANGELOG.md](CHANGELOG.md).

## Najważniejsze funkcje

* 🔎 **Jeden endpoint dla 40 dostawców** — web/SERP, AI/neural, akademickie i
  korporacyjne
* 🧩 **Wizualny edytor łańcuchów** — graf z blokami **Start** (rozgałęzienie),
  providerami i blokiem **Join** (scalanie + deduplikacja URL-i)
* 🔗 **Routing łańcucha na token** — każdy klucz API może być przypisany do
  własnego łańcucha, niezależnie od aktywnego
* 🔌 **Gotowe dla Open WebUI** — endpoint `/search`: HTTP 200 z
  `[{"link","title","snippet"}]`
* 🤖 **Serwer MCP** — narzędzie `search` po streamable HTTP dla Claude, Cursor,
  VS Code i innych klientów MCP
* 🔭 **Podgląd na żywo** — os czasu kroków po SSE, nagłówki `X-Serpent-Rid` /
  `X-Serpent-Api` oraz strona History
* 📊 **Metryki Prometheus** — `GET /metrics` (wyniki wyszukiwań, próby
  providerów, czasy, aktywni klienci SSE)
* 📋 **Dziennik audytu** — każda mutacja administracyjna z aktorem, IP i
  nie-tajnymi szczegółami
* 🔐 **Bezpieczeństwo** — klucze API hashowane argon2id, passkeys WebAuthn,
  sesje SameSite=Strict z CSRF, sekrety tylko do zapisu, opcjonalne szyfrowanie
  AES-GCM
* 💾 **SQLite ⟷ PostgreSQL** — SQLite (WAL) domyślnie; profil `extdb` dla
  wielu replik
* 🌍 **Wielojęzyczny interfejs** — English, Русский, Polski, Deutsch, Français, 日本語, 中文
* 🐳 **Jeden kontener Docker** — backend Go z osadzonym SPA SvelteKit

## Szybki start

```bash
cp .env.example .env
# ustaw klucze providerów, jeśli potrzebne (np. SEARXNG_URL)
docker compose up --build
```

Panel: <http://localhost:8080>. Health check: `curl http://localhost:8080/healthz`
(lub `/readyz`; `/livez` to czysty liveness).

Przy pierwszym starcie jednorazowy token pojawia się w logach:

```text
SETUP_TOKEN: ff1089a6
```

Otwórz `/setup`, wpisz token i utwórz administratora. **Klucz API pokazywany
jest tylko raz** — zapisz go, a następnie zaloguj się na `/login`.

### Open WebUI

*Ustawienia → Web Search → External*:

* URL: `http://<host>:8080/search`
* Klucz API: `seek_ak_...`

### MCP

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
Dla klientów tylko stdio użyj `npx mcp-remote http://<host>:8080/mcp
--header "Authorization=Bearer seek_ak_..."`.

## Dostawcy

Pełna lista i wymagane poświadczenia — w [README.md](README.md#providers).
Poświadczenia są **tylko do zapisu** i ustawiane per instancja na stronie
**Providers**. SearXNG jest zawsze **zewnętrzną** usługą (własny URL, wymagane
`format=json` i `limiter: false`).

## Bloki łańcucha

Trzy rodzaje bloków:

* **Start** — logiczny punkt wejścia, rozgałęzia się do kilku providerów
  (dokładnie jeden na łańcuch).
* **Provider** — uruchamia instancję providera; tu ustawiane są timeouty,
  ponowienia, backoff i polityki wyników.
* **Join** — punkt scalania: czeka na wszystkie gałęzie, scala wyniki i usuwa
  duplikaty URL-i.

Tryb **first success** to liniowy fallback, **full chain** uruchamia równoległe
gałęzie po bloku Start. Blok Join z `ignore_count` ignoruje liczbę wyników.

## Ustawienia (skrót)

Wszystkie zmienne są w [`.env.example`](.env.example) i edytowalne w UI, o ile
nie są przypięte przez środowisko. Najważniejsze:

`PORT`, `HOST`, `DATA_DIR`/`SQLITE_PATH`, `STORAGE_DRIVER`, `DATABASE_URL`,
`AUTH_ENABLED`, `PUBLIC_ORIGIN`, `RP_ID`, `RP_NAME`, `HISTORY_RETENTION_DAYS`,
`LOGS_RETENTION_DAYS`, `MAX_CONCURRENCY`, `MAX_ATTEMPTS`, `USER_AGENT`,
`LOG_LEVEL`, `LOG_FORMAT`, `TREAT_EMPTY_AS_FAIL`, `SEARCH_RATE_LIMIT_PER_MINUTE`,
`SEARCH_DAILY_QUOTA`, `CHAIN_TIMEOUT`,
`PROVIDER_FAILURE_THRESHOLD`, `PROVIDER_COOLDOWN_SECONDS`, `DEBUG`.

## Uwierzytelnianie

* **Klucze:** `seek_ak_<prefix>_<secret>` (scope `search`) i
  `seek_admin_...` (scope `admin`); przechowywany jest tylko hash argon2id.
* **Sesje:** cookie httpOnly + Secure + SameSite=Strict, CSRF double-submit.
* **Passkeys (WebAuthn):** wymagają HTTPS i domeny (`PUBLIC_ORIGIN`/`RP_ID`).
* **`AUTH_ENABLED=false`** — całe API administracyjne działa bez logowania.

## Zadania w tle

* `cleanup` (`HISTORY_CLEANUP_CRON`, domyślnie `0 4 * * *`) — usuwa historię,
  logi, wygasłe sesje, liczniki limitów (`usage_daily`, 90 dni) oraz wpisy
  audytu (180 dni).
* `vacuum` (`VACUUM_CRON`, domyślnie `0 3 * * 0`) — tylko SQLite.

## Rozwój

```bash
make build      # SPA + binarka Go (embed)
make test       # go test ./...
make check      # svelte-check
make lint       # go vet + golangci-lint
make dev        # Vite proxy do :8080
```

## Operacje

Backup, odtwarzanie, aktualizacje i diagnostyka — [docs/operations.md](docs/operations.md).

## Ryzyka

* Passkeys wymagają HTTPS i domeny.
* Google Custom Search JSON API jest wycofywane (2027-01-01); domyślnie
  używane jest Vertex/Grounding.
* **Providerzy są płatni** — żądania idą z kluczami wdrożeniowca; ustaw budżety
  i limity po stronie providera. Autorzy nie odpowiadają za opłaty.
* SearXNG jest zewnętrzną usługą (AGPLv3), nie częścią tego repozytorium.
* SQLite ma jednego pisarza; WAL + busy_timeout wystarcza dla jednej repliki.
  Wiele replik wymaga PostgreSQL.
* `SEARCH_RATE_LIMIT_PER_MINUTE`, `SEARCH_DAILY_QUOTA`,
  `CHAIN_TIMEOUT` i circuit breaker są opcjonalne (0 = wyłączone).
