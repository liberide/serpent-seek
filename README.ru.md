# 🐍 SerpentSeek — поисковый шлюз (self-hosted) для Open WebUI, MCP и 39+ поисковых API

[English](README.md "Read in English") | **Русский** | [Deutsch](README.de.md "Auf Deutsch lesen") | [Français](README.fr.md "Lire en français") | [中文](README.zh.md "阅读中文版")

[![License: GPL-3.0](https://img.shields.io/badge/license-GPL--3.0-blue.svg)](LICENSE "Лицензия проекта — GNU GPL v3.0")
[![Go 1.26](https://img.shields.io/badge/Go-1.26-00ADD8?logo=go&logoColor=white)](go.mod "Бэкенд написан на Go")
[![CI](https://github.com/liberide/serpent-seek/actions/workflows/ci.yml/badge.svg)](https://github.com/liberide/serpent-seek/actions/workflows/ci.yml "CI: сборка, vet, тесты")
[![Docker — single container](https://img.shields.io/badge/docker-single_container-2496ED?logo=docker&logoColor=white)](Dockerfile "Всё в одном Docker-контейнере")
[![OpenAPI 3.1](https://img.shields.io/badge/API-OpenAPI_3.1-6BA539)](api/openapi.yaml "Спецификация OpenAPI 3.1")
[![39 search providers](https://img.shields.io/badge/built--in_search_providers-39-orange)](#провайдеры "39 встроенных поисковых провайдеров")

**SerpentSeek** — open-source «**поисковый шлюз**» (self-hosted): единый
API веб-поиска (метапоисковый прокси) на Go с мультиязычной веб-админкой для
**Open WebUI**. Каждый запрос проходит через настраиваемые цепочки из **39
встроенных поисковых провайдеров** (SearXNG, Brave, Google Vertex AI Search /
Grounding, Yandex, SerpApi, Serper, DataForSEO, Exa, Tavily, Kagi, Perplexity и
др.), собранные в визуальном редакторе нод и стрелок. Внутри: история запросов
с живой трассировкой (SSE), API-ключи и Passkeys (WebAuthn), **MCP-инструмент
поиска** (streamable HTTP) для ИИ-ассистентов (Claude, Cursor, VS Code…),
совместимый с Open WebUI эндпоинт `/search`, SQLite ⟷ PostgreSQL и фоновые
джобы очистки. Всё упаковано в один Docker-контейнер.

Лицензия проекта — **GNU 3.0** (GPL-3.0-only, см. `LICENSE`). Авторство — в
`NOTICE`/`AUTHORS`; лицензии всех встроенных в бинарь и веб-сборку сторонних
компонентов — в `THIRD_PARTY_LICENSES.md`. Об уязвимостях сообщайте приватно
(см. `SECURITY.md`), правила участия — `CONTRIBUTING.md` (DCO sign-off + [CLA](CLA.md)).

Программа предоставляется **«как есть» (AS IS), без гарантий**. Запросы уходят в
сторонние поисковые API с ключами того, кто развернул сервис; списания за них —
между оператором и его провайдером. Авторы не несут ответственности за такие
начисления и иной материальный вред (GPLv3 §§15–16, подробнее — `NOTICE`).

## Возможности

* 🔎 **Один эндпоинт для 39 провайдеров** — веб/SERP (SearXNG, Brave, Google,
  Yandex, SerpApi…), нейропоиск (Exa, Tavily, Perplexity, Kagi…), академический
  (OpenAlex, PubMed, Crossref, Semantic Scholar…) и корпоративный (Azure AI
  Search, Vertex AI Search, Vectara…)
* 🧩 **Визуальный редактор цепочек** — fallback и ретраи как граф из нод и
  стрелок; несколько именованных инстансов одного драйвера
* 🔌 **Готов для Open WebUI** — эндпоинт `/search` из коробки: всегда HTTP 200,
  `[{"link","title","snippet"}]`
* 🤖 **MCP-сервер** — инструмент `search` по streamable HTTP для Claude, Cursor,
  VS Code и других MCP-клиентов; принимает и plain JSON-RPC 2.0
* 🔭 **Живая трассировка** — таймлайн шагов запроса по SSE, диагностические
  заголовки `X-Serpent-Rid` / `X-Serpent-Api` и страница History
* 🔐 **Безопасность** — argon2id-хэши ключей, Passkeys (WebAuthn), сессии
  SameSite=Strict с CSRF, write-only секреты, опциональный AES-GCM
* 💾 **SQLite ⟷ PostgreSQL** — SQLite в WAL по умолчанию; профиль `extdb`
  поднимает PostgreSQL для мультиреплик
* 🌍 **Мультиязычный UI** — English, Русский, Polski, Deutsch, Français, 中文
* 🐳 **Один Docker-контейнер** — Go-бэкенд со встроенной SvelteKit SPA

## Содержание

* [Быстрый старт](#быстрый-старт)
  * [Интеграция с Open WebUI](#интеграция-с-open-webui)
  * [Интеграция по MCP (Model Context Protocol)](#интеграция-по-mcp-model-context-protocol)
* [Провайдеры](#провайдеры)
  * [SearXNG: только внешний инстанс](#searxng-только-внешний-инстанс)
* [PostgreSQL (профиль `extdb`)](#postgresql-профиль-extdb)
* [Настройки](#настройки)
* [Интерфейс и локализация](#интерфейс-и-локализация)
* [Аутентификация](#аутентификация)
* [Фоновые джобы](#фоновые-джобы)
* [Разработка](#разработка)
* [Зависимости и лицензии](#зависимости-и-лицензии)
* [Риски и оговорки](#риски-и-оговорки)
* [Скриншоты](#скриншоты)
* [Структура](#структура)

## Быстрый старт

```bash
cp .env.example .env
# откройте .env и при необходимости задайте ключи провайдеров (SEARXNG_URL и др.)
docker compose up --build
```

UI: <http://localhost:8080>. Проверка здоровья: `curl http://localhost:8080/healthz`.

Чтобы использовать готовый образ из Docker Hub вместо сборки из исходников —
см. [DEPLOY.md](DEPLOY.md).

При первом запуске в логах печатается одноразовый токен:

```text
SETUP_TOKEN: ff1089a6
```

Откройте `/setup`, введите токен, создайте администратора. Показанный **API-ключ
выдаётся один раз** — сохраните его. Затем войдите на `/login` этим ключом.

### Интеграция с Open WebUI

В Open WebUI → *Settings → Web Search* выберите **External** (или SearXNG) и укажите:

* URL: `http://<host>:8080/search`
* API key: ваш ключ `seek_ak_...`

Эндпоинт `POST /search` совместим с Open WebUI: тело `{"query":"...","count":5}`,
ответ всегда HTTP 200 с массивом `[{"link","title","snippet"}]` (пустой `[]` при
фейле/пустой выдаче). Диагностика — в заголовках `X-Serpent-Rid` и `X-Serpent-Api`
и на странице History.

```bash
curl -s http://localhost:8080/search \
  -H "Authorization: Bearer seek_ak_..." \
  -H "Content-Type: application/json" \
  -d '{"query":"hello world","count":3}'
```

### Интеграция по MCP (Model Context Protocol)

SerpentSeek отдаёт инструмент `search` по MCP на `/mcp` (официальный транспорт **streamable HTTP**: GET открывает SSE-поток, POST передаёт JSON-RPC, DELETE закрывает сессию). Тот же эндпоинт работает и как обычный HTTP JSON-RPC 2.0 request/response для клиентов без поддержки streamable HTTP. API-ключ передаётся как Bearer-токен.

Для **нативных HTTP MCP-клиентов** (Claude Desktop, некоторые сборки Cursor, MCP tools в Open WebUI и др.):

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

Для **stdio-only MCP-клиентов** (старый Cursor, расширения VS Code и др.) используйте обёртку `mcp-remote`, чтобы удалённый HTTP endpoint выглядел как локальный stdio-сервер:

```bash
npx mcp-remote http://<host>:8080/mcp --header "Authorization=Bearer seek_ak_..."
```

Затем укажите клиенту эту команду вместо URL.

Единственный инструмент — `search` с аргументами `query` (обязательный) и
`count` (опционально, 0 = все, по умолчанию 5). Каждый вызов запускает активную
цепочку провайдеров и возвращает `title` / `url` / `snippet` для каждого
результата. Описание эндпоинта (готовые конфиги для вставки и живой тест) — на
странице **MCP** в админке.

## Провайдеры

Полный список встроенных драйверов. Креды — write-only, задаются на инстанс на странице **Провайдеры** (из окружения сидится только `google_vertex` через `GOOGLE_*`).

### Веб-поиск / SERP

| Код | Назначение | Креды | Заметки |
| --- | --- | --- | --- |
| `apiserpent` | apiserpent.com quick search | `api_key` | ротация движков google/bing/yahoo/ddg/brave |
| `serpbase` | serpbase.dev | `api_key` | настраиваемая таблица fatal/retry |
| `searxng` | SearXNG — **внешний инстанс** | `api_key` (опц., токен гейта) | URL в UI/`SEARXNG_URL`; нужен `format=json` и `limiter: false` |
| `yandex` | Yandex Search API v2 (Yandex Cloud) | `api_key`, `folder_id` | `/v2/web/search`, base64-XML в `rawData` |
| `google_vertex` | Gemini Grounding / Vertex AI Search | `api_key`, `project_id`, `engine_id` | основной путь Google; env `GOOGLE_API_KEY`, `GOOGLE_MODEL`, `GOOGLE_DRIVER` |
| `google` | Google (диспетчер) | `api_key`, `cx`, `project_id`, `engine_id` | параметр `driver` переключает vertex/cse |
| `google_cse` | Google Custom Search JSON API | `api_key`, `cx` | **deprecated**, выключение 01.01.2027 |
| `brave` | Brave Search | `api_key` | |
| `serper` | Serper | `api_key` | |
| `serpapi` | SerpApi | `api_key` | мультидвижок (`engine`) |
| `searchapi` | SearchApi.io | `api_key` | |
| `dataforseo` | DataForSEO | `login`, `password` | HTTP Basic auth |
| `youcom` | You.com | `api_key` | |
| `mojeek` | Mojeek | `api_key` | ключ в query-строке |
| `marginalia` | Marginalia | `api_key` | публичный ключ с лимитом (503 повторяем) |
| `hn` | Hacker News (Algolia) | — | без ключа |

### AI / нейропоиск

| Код | Назначение | Креды | Заметки |
| --- | --- | --- | --- |
| `exa` | Exa | `api_key` | по умолчанию highlights |
| `tavily` | Tavily | `api_key` | advanced-поиск = 2 кредита |
| `jina` | Jina AI Search | `api_key` | запрос в пути URL |
| `firecrawl` | Firecrawl | `api_key` | |
| `linkup` | Linkup | `api_key` | вывод `searchResults` |
| `perplexity_search` | Perplexity Search API | `api_key` | отдельный `/search`, не Sonar |
| `valyu` | Valyu | `api_key` | proprietary-поиск по подписке |
| `parallel` | Parallel | `api_key` | `search_queries` из запроса |
| `kagi` | Kagi | `api_key` | FastGPT/enrich |
| `kimi` | Kimi (Moonshot) Web Search | `api_key` | title/url/snippet; `include_content` — текст страницы |
| `kimi_pro` | Kimi (Moonshot) Web Search Pro | `api_key` | ранжированные chunks; фильтры `sites`/`time_window` |

### Академический и разработческий поиск

| Код | Назначение | Креды | Заметки |
| --- | --- | --- | --- |
| `openalex` | OpenAlex | — | абстракт из inverted index |
| `semanticscholar` | Semantic Scholar | `api_key` (опц.) | ~1 rps без ключа |
| `crossref` | Crossref | — | DOI нормализуется в `https://doi.org/` |
| `github` | GitHub Search | `api_key` | репозитории/код/issues/commits/users |
| `stackexchange` | Stack Exchange | `api_key` (опц.) | учитывает `backoff` |
| `pubmed` | PubMed (NCBI) | `api_key` (опц.), env `NCBI_API_KEY` | двухшаговый esearch→esummary (+ efetch) |

### Корпоративный поиск

| Код | Назначение | Креды | Заметки |
| --- | --- | --- | --- |
| `azure_search` | Azure AI Search | `api_key` или Entra (`tenant_id`, `client_id`, `client_secret`) | настраиваемые `title_field`/`url_field`/`snippet_field` |
| `vertex_search` | Vertex AI Search (Discovery Engine) | `service_account_json` | только OAuth2, без API-ключей |
| `vectara` | Vectara | `api_key` | generation выключен (Row) |
| `kendra` | Amazon Kendra | `access_key_id`, `secret_access_key` | **deprecated** — SigV4 не реализован |

### Answer-узлы (генеративные ответы)

Эти драйверы ставятся на узел **`mode: answer`**: сгенерированный текст попадает
в панель ответа запроса, а источники — sparse-Row (link+title).

| Код | Назначение | Креды | Заметки |
| --- | --- | --- | --- |
| `anthropic` | Anthropic web search | `api_key` | инструмент `web_search` + citations |
| `yandex_gen` | Yandex AI Studio (генеративный ответ) | `api_key`, `folder_id` | `/v2/gen/search` |

`base_url` каждого провайдера редактируется в UI (не секрет). Секреты — write-only:
значение никогда не отдаётся обратно, только отметка «задано». Кнопка «Проверить»
делает пробный запрос `q=test` и показывает `http/kind/ms` и первые ссылки.

**Провайдеры — это инстансы.** На странице Providers можно добавить сколько угодно
провайдеров, в том числе **несколько одного типа (драйвера) с разными именами и
`api_key`** — например, «apiserpent work» и «apiserpent personal». Каждый инстанс
включается/выключается переключателем; отключённые не попадают в палитру редактора
маршрутов. Удаление инстанса, который используется маршрутом, отклоняется с `409` и
списком маршрутов. При обновлении со старой версии БД прежние записи автоматически
становятся инстансами (`id = code`), ссылки в маршрутах сохраняются.

**При первом старте** создаётся и включается только `google_vertex`
(Vertex/Grounding), а дефолтный маршрут состоит из одного блока `google_vertex`.
Остальные инстансы не создаются — добавляйте нужные драйверы на странице
«Провайдеры».

### SearXNG: только внешний инстанс

SerpentSeek **не запускает SearXNG сам** — в compose нет такого сервиса. Укажите
URL вашего уже работающего SearXNG:

* в поле `base_url` провайдера `searxng` (страница Providers, без рестарта), либо
* через `SEARXNG_URL` в `.env`.

Поддерживается чистый URL (`http://host:port/search`) и шаблон с
`{query}`/`<query>`. Пока URL не задан, провайдер считается выбывшим (`skip`), и
цепочка идёт дальше.

Требования к вашему инстансу: `search.formats: [html, json]` и `limiter: false`
(иначе будут 403/429). Пример конфигурации — `deploy/searxng/settings.yml`;
запускать её владелец инстанса.

> **⚠️ Лицензия SearXNG — AGPLv3.** Это внешний сервис, SerpentSeek общается с
> ним только по HTTP и ничего не линкует; ответственность за соблюдение
> лицензии SearXNG лежит на том, кто его разворачивает.

## PostgreSQL (профиль `extdb`)

```bash
# в .env: POSTGRES_PASSWORD=... STORAGE_DRIVER=postgres \
#         DATABASE_URL=postgres://serpent:...@postgres:5432/serpentseek?sslmode=disable
docker compose --profile extdb up -d --build
```

PostgreSQL — отдельный контейнер (PostgreSQL License), **не** зависимость кода
SerpentSeek. SQLite по умолчанию: `journal_mode=WAL`, `foreign_keys=ON`,
`busy_timeout=5000`, `synchronous=NORMAL`, один писатель. Multi-replica — только с
PostgreSQL.

## Настройки

Все переменные перечислены в [`.env.example`](.env.example). Значения из env имеют
приоритет; в UI такие поля помечены бейджем `env` и read-only. Остальные настройки
можно менять на странице Settings без перезапуска (кроме `STORAGE_DRIVER`, о чём UI
честно предупреждает).

Ключи провайдеров (`api_key`, `folder_id`, `cx`, …) и `base_url` — это **свойства
инстанса**: они задаются на странице Providers. Единственные переменные
окружения, которые сидят провайдера при первом старте, — `GOOGLE_API_KEY`,
`GOOGLE_MODEL` и `GOOGLE_DRIVER` (для дефолтного `google_vertex`); все остальные
креды задаются на инстанс и env-дефолтов не имеют. На странице Settings у
провайдеров остались только **коды повтора** (`RETRY_HTTP_CODES`, `AP_RETRY_CODES`) —
глобальные дефолты, которые каждый инстанс может переопределить через свой
`params`. Фатальные коды (`fatal_codes`, `fatal_http`) стали параметрами инстанса
и редактируются на странице Providers.

Ключевые переменные: `PORT`, `HOST`, `DATA_DIR`/`SQLITE_PATH`, `STORAGE_DRIVER`,
`DATABASE_URL`, `AUTH_ENABLED`, `PUBLIC_ORIGIN`, `RP_ID`, `RP_NAME`,
`HISTORY_RETENTION_DAYS`, `LOGS_RETENTION_DAYS`, `MAX_CONCURRENCY`, `MAX_ATTEMPTS`,
`USER_AGENT`, `LOG_LEVEL`, `LOG_FORMAT`, `TREAT_EMPTY_AS_FAIL`.

`IGNORE_REQUEST_COUNT=true` заставляет шлюз игнорировать `count` из запроса и
передавать провайдерам `num=0` — вернуть всё найденное без лимита. Это же значение
(`0`) можно задать вручную в поле «Результатов» на странице **Playground**
(подсказка появляется при наведении на поле).

## Интерфейс и локализация

UI переведён на 6 языков:

| Код | Язык |
| --- | --- |
| `ru` | Русский |
| `en` | English |
| `pl` | Polski |
| `de` | Deutsch |
| `fr` | Français |
| `zh` | 中文 |

Переключатель языка — в правом верхнем углу, рядом с кнопкой «Быстрый поиск».
Выбранный язык сохраняется в `localStorage`; при первом открытии он подбирается по
`navigator.language`, иначе используется английский. Даты форматируются по правилам
выбранной локали (`Intl.DateTimeFormat`).

Чтобы добавить новый язык: положите словарь `web/src/lib/i18n/<code>.json` (набор
ключей должен совпадать с `en.json`), добавьте запись в
`web/src/lib/i18n/languages.json` и код в тип `Locale` в
`web/src/lib/i18n.svelte.ts`.

## Аутентификация

* **Ключи:** `seek_ak_<prefix>_<secret>` (scope search) и `seek_admin_<prefix>_<secret>`
  (scope admin). Хранится только argon2id-хеш; в UI/API виден лишь prefix.
* **Сессии:** httpOnly + Secure + SameSite=Strict cookie, rolling 30 дней, CSRF
  double-submit на все мутации.
* **Passkeys (WebAuthn):** регистрация после входа ключом, вход без пароля. Требуют
  HTTPS + домен (`PUBLIC_ORIGIN`/`RP_ID`); по HTTP/IP кнопка скрыта с предупреждением.
* **`AUTH_ENABLED=false`** — весь админ-API без входа (в UI баннер).

## Фоновые джобы

* `cleanup` (`HISTORY_CLEANUP_CRON`, по умолчанию `0 4 * * *`) — удаляет историю и
  логи старше retention, истёкшие сессии, пересчитывает дневные агрегаты.
* `vacuum` (`VACUUM_CRON`, по умолчанию `0 3 * * 0`) — только SQLite.

Обе запускаются вручную из Settings → Обслуживание (или через
`POST /api/maintenance/*`). Паника джобы не роняет процесс.

## Разработка

```bash
make build      # SPA + Go-бинарь (embed)
make test       # go test ./... -cover
make check      # svelte-check
make lint       # go vet + golangci-lint
make dev        # Vite dev-сервер с прокси на :8080
```

Backend слушает `:8080`, SPA встроена через `go:embed web/build`.

## Зависимости и лицензии

Разрешены только MIT / Apache-2.0 / BSD-2/3 / ISC. Аудит:

```bash
go run github.com/google/go-licenses@latest report ./...
cd web && npx license-checker --production --excludePrivatePackages
```

| Зависимость | Версия | Лицензия |
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

Сателлитные контейнеры (не зависимости кода): `postgres:17-alpine` (PostgreSQL
License) и внешний `searxng/searxng` (не входит в этот репозиторий; **AGPLv3**
— отдельный сервис, не зависимость кода).

## Риски и оговорки

* **Passkeys требуют HTTPS + домен** (WebAuthn rpID). По голому IP/HTTP доступны
  только ключи; UI об этом предупреждает.
* **Google Custom Search JSON API** закрыт для новых клиентов и отключается
  **01.01.2027** — драйвер помечен deprecated, дефолт — Vertex/Grounding.
* **Провайдеры платные.** Все запросы выполняются со **своими** API-ключами
  того, кто развернул сервис, и тарифицируются провайдером (Yandex Cloud,
  Google Cloud, apiserpent, SerpBase). Настройте бюджеты, алерты и квоты ключей
  **на стороне провайдера** и контролируйте ретраи (max attempts, политика
  повторов в маршруте): авторы проекта не отвечают за списания (см. `NOTICE`).
* **Yandex Search API v2** платный: нужен Api-Key + folderId (Yandex Cloud).
* **SearXNG** — внешний сервис: нужны `search.formats: [html, json]` и
  `limiter: false`; гейт-токен (`X-API-Key`) — это **не** `server.secret_key`. Сам
  SearXNG лицензирован под **AGPLv3** и не входит в репозиторий.
* **SQLite** — один писатель; WAL + busy_timeout достаточно для одной реплики.
* **Кредаеншелы провайдеров** write-only, маскируются в API и логах; `base_url` —
  не секрет. Опционально `ENCRYPTION_KEY` (AES-GCM) для шифрования на диске.

## Скриншоты

После запуска доступны экраны: Dashboard, History (с цветным графом цепочки и
таймлайном шагов), Chains (визуальный редактор), Providers, Playground, MCP,
Logs, Users, Settings. Добавьте свои скриншоты в `docs/screenshots/` (в репозиторий
они не включены, чтобы не раздувать образ).

## Структура

```
cmd/serpentseek/      # wiring и seed
internal/
  config logging store providers engine sse auth httpd jobs version
api/openapi.yaml      # OpenAPI 3.1
web/                  # SvelteKit SPA (build встраивается в бинарь)
deploy/
  searxng/settings.yml
  caddy.example
```

---

<sub>**Ключевые слова:** поисковый шлюз, self-hosted search gateway, единый поисковый API, метапоиск, метапоисковый прокси, Open WebUI веб-поиск, поисковый бэкенд для Open WebUI, SearXNG, MCP-сервер, MCP search tool, Model Context Protocol веб-поиск, поиск для LLM и ИИ-ассистентов, RAG-поиск, Brave Search API, Google Vertex AI Search, Grounding with Google Search, Yandex Search API, SerpApi, Serper, DataForSEO, Exa, Tavily, Kagi, Perplexity Search API, цепочки провайдеров с fallback, агрегатор поиска, OpenAPI 3.1, Go-микросервис, один Docker-контейнер.</sub>
