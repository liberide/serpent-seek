# 🐍 SerpentSeek — 自托管搜索网关，面向 Open WebUI、MCP 及 37+ 搜索 API

[English](README.md "Read in English") | [Русский](README.ru.md "README по-русски") | [Deutsch](README.de.md "Auf Deutsch lesen") | [Français](README.fr.md "Lire en français") | **中文**

[![License: GPL-3.0](https://img.shields.io/badge/license-GPL--3.0-blue.svg)](LICENSE "项目许可证 — GNU GPL v3.0")
[![Go 1.26](https://img.shields.io/badge/Go-1.26-00ADD8?logo=go&logoColor=white)](go.mod "后端使用 Go 编写")
[![CI](https://github.com/liberide/serpent-seek/actions/workflows/ci.yml/badge.svg)](https://github.com/liberide/serpent-seek/actions/workflows/ci.yml "CI：构建、vet、测试")
[![Docker — single container](https://img.shields.io/badge/docker-single_container-2496ED?logo=docker&logoColor=white)](Dockerfile "所有内容打包在单个 Docker 容器中")
[![OpenAPI 3.1](https://img.shields.io/badge/API-OpenAPI_3.1-6BA539)](api/openapi.yaml "OpenAPI 3.1 规范")
[![37 search providers](https://img.shields.io/badge/built--in_search_providers-37-orange)](#提供方 "37 个内置搜索提供方")

**SerpentSeek** 是一个开源、可自托管的「**搜索网关**」——用 Go 编写的统一 Web
搜索 API（元搜索代理），带有面向 **Open WebUI** 的多语言 Web 管理界面。每个请求都会
经过在可视化节点/连线编辑器中配置的、由 **37 个内置搜索提供方**（SearXNG、Brave、
Google Vertex AI Search / Grounding、Yandex、SerpApi、Serper、DataForSEO、Exa、
Tavily、Kagi、Perplexity 等）组成的链路。内置：带实时追踪（SSE）的请求历史、API
密钥与 Passkeys（WebAuthn）、面向 AI 助手（Claude、Cursor、VS Code 等）的 **MCP 搜索
工具**（streamable HTTP）、与 Open WebUI 兼容的 `/search` 端点、SQLite ⟷ PostgreSQL
以及后台清理任务。所有内容打包在单个 Docker 容器中。

项目许可证 — **GNU 3.0**（GPL-3.0-only，见 `LICENSE`）。署名信息见 `NOTICE`/`AUTHORS`；
编译进二进制和 Web 包的所有第三方组件许可证汇总于 `THIRD_PARTY_LICENSES.md`。
请私下报告漏洞（见 `SECURITY.md`）；贡献规则见 `CONTRIBUTING.md`（DCO 签署 + [CLA](CLA.md)）。

本软件按 **「原样」提供，不附带任何形式的保证**。请求会使用部署者自己的密钥发往第三方搜索
API；任何费用均由运维方与其提供方之间解决。作者不对这些费用或任何其他物质损失负责
（GPLv3 §§15–16，详见 `NOTICE`）。

## 主要特性

* 🔎 **一个端点接入 37 个搜索提供方** — Web/SERP（SearXNG、Brave、Google、
  Yandex、SerpApi…）、AI/神经搜索（Exa、Tavily、Perplexity、Kagi…）、学术
  （OpenAlex、PubMed、Crossref、Semantic Scholar…）与企业级（Azure AI
  Search、Vertex AI Search、Vectara…）
* 🧩 **可视化链路编辑器** — 以节点/连线图配置回退与重试链路；同一驱动可创建多个命名实例
* 🔌 **开箱接入 Open WebUI** — `/search` 端点：始终返回 HTTP 200 与
  `[{"link","title","snippet"}]`
* 🤖 **MCP 服务器** — 通过 streamable HTTP 提供 `search` 工具，适用于 Claude、
  Cursor、VS Code 等 MCP 客户端；同时兼容普通 JSON-RPC 2.0
* 🔭 **实时请求追踪** — 基于 SSE 的分步时间线、`X-Serpent-Rid` /
  `X-Serpent-Api` 诊断头以及历史记录页面
* 🔐 **安全** — API 密钥使用 argon2id 哈希、Passkeys（WebAuthn）、
  SameSite=Strict 会话与 CSRF 防护、只写机密、可选 AES-GCM 静态加密
* 💾 **SQLite ⟷ PostgreSQL** — 默认 WAL 模式 SQLite；`extdb` profile 提供
  PostgreSQL 以支持多副本部署
* 🌍 **多语言界面** — English, Русский, Polski, Deutsch, Français, 中文
* 🐳 **单 Docker 容器** — Go 后端内嵌 SvelteKit SPA

## 目录

* [快速开始](#快速开始)
  * [Open WebUI 集成](#open-webui-集成)
  * [MCP 集成（Model Context Protocol）](#mcp-集成model-context-protocol)
* [提供方](#提供方)
  * [SearXNG：仅外部实例](#searxng仅外部实例)
* [PostgreSQL（`extdb` profile）](#postgresql-extdb-profile)
* [设置](#设置)
* [界面与本地化](#界面与本地化)
* [身份验证](#身份验证)
* [后台任务](#后台任务)
* [开发](#开发)
* [依赖与许可证](#依赖与许可证)
* [风险与注意事项](#风险与注意事项)
* [截图](#截图)
* [目录结构](#目录结构)

## 快速开始

```bash
cp .env.example .env
# 打开 .env，按需设置提供方密钥（SEARXNG_URL 等）
docker compose up --build
```

界面：<http://localhost:8080>。健康检查：`curl http://localhost:8080/healthz`。

若想使用 Docker Hub 上预构建的镜像而不是从源码构建 ——
请参阅 [DEPLOY.md](DEPLOY.md)。

首次启动时会在日志中打印一次性令牌：

```text
SETUP_TOKEN: ff1089a6
```

打开 `/setup`，输入令牌，创建管理员。**API 密钥只显示一次** —— 请保存。
然后用该密钥在 `/login` 登录。

### Open WebUI 集成

在 Open WebUI → *Settings → Web Search* 中选择 **External**（或 SearXNG），并设置：

* URL：`http://<host>:8080/search`
* API 密钥：你的 `seek_ak_...` 密钥

`POST /search` 端点兼容 Open WebUI：请求体 `{"query":"...","count":5}`，
响应始终为 HTTP 200，返回数组 `[{"link","title","snippet"}]`（失败或空结果时返回 `[]`）。
诊断信息通过 `X-Serpent-Rid` 与 `X-Serpent-Api` 响应头以及 History 页面查看。

```bash
curl -s http://localhost:8080/search \
  -H "Authorization: Bearer seek_ak_..." \
  -H "Content-Type: application/json" \
  -d '{"query":"hello world","count":3}'
```

### MCP 集成（Model Context Protocol）

SerpentSeek 在 `/mcp` 通过 MCP 暴露 `search` 工具（官方 **streamable HTTP** 传输：
GET 用于 SSE 流，POST 用于 JSON-RPC，DELETE 用于关闭会话）。同一个端点也可作为普通 HTTP
JSON-RPC 2.0 请求/响应用于不支持完整 streamable 传输的客户端。以 Bearer 方式传入 API 密钥。

对于 **原生 HTTP MCP 客户端**（Claude Desktop、部分 Cursor 版本、Open WebUI MCP 工具等）：

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

对于 **仅支持 stdio 的 MCP 客户端**（较旧的 Cursor、VS Code 扩展等），使用 `mcp-remote`
包装器，让远程 HTTP 端点看起来像本地 stdio 服务器：

```bash
npx mcp-remote http://<host>:8080/mcp --header "Authorization=Bearer seek_ak_..."
```

然后将客户端指向该包装命令而不是 URL。

唯一的工具是 `search`，接受 `query`（必填）和 `count`（可选，0 = 全部，默认 5）。
每次调用执行当前活动提供方链路，并为每个结果返回 `title` / `url` / `snippet`。
该端点在管理界面的 **MCP** 页面有说明（含可直接复制的配置片段与实时测试）。

## 提供方

内置搜索驱动的完整列表。凭据为只写，按实例在 **Providers** 页面设置
（只有 `google_vertex` 的种子会从环境读取 `GOOGLE_*`）。

### Web 搜索 / SERP

| 代码 | 用途 | 凭据 | 说明 |
| --- | --- | --- | --- |
| `apiserpent` | apiserpent.com 快速搜索 | `api_key` | 引擎轮换 google/bing/yahoo/ddg/brave |
| `serpbase` | serpbase.dev | `api_key` | 可配置的 fatal/retry 状态码表 |
| `searxng` | SearXNG — **外部实例** | `api_key`（可选，网关令牌） | URL 在 UI/`SEARXNG_URL` 设置；需要 `format=json` 和 `limiter: false` |
| `yandex` | Yandex Search API v2（Yandex Cloud） | `api_key`, `folder_id` | `/v2/web/search`，`rawData` 中的 base64 XML |
| `google_vertex` | Gemini Grounding with Google Search / Vertex AI Search | `api_key`, `project_id`, `engine_id` | Google 主路径；env `GOOGLE_API_KEY`, `GOOGLE_MODEL`, `GOOGLE_DRIVER` |
| `google` | Google（调度器） | `api_key`, `cx`, `project_id`, `engine_id` | `driver` 参数切换 vertex/cse |
| `google_cse` | Google Custom Search JSON API | `api_key`, `cx` | **已弃用**，2027-01-01 关停 |
| `brave` | Brave Search | `api_key` | |
| `serper` | Serper | `api_key` | |
| `serpapi` | SerpApi | `api_key` | 多引擎（`engine` 参数） |
| `searchapi` | SearchApi.io | `api_key` | |
| `dataforseo` | DataForSEO | `login`, `password` | HTTP Basic 认证 |
| `youcom` | You.com | `api_key` | |
| `mojeek` | Mojeek | `api_key` | 密钥放在查询字符串中 |
| `marginalia` | Marginalia | `api_key` | 公共密钥有限流（503 可重试） |
| `hn` | Hacker News（Algolia） | — | 无需密钥 |

### AI / 神经搜索

| 代码 | 用途 | 凭据 | 说明 |
| --- | --- | --- | --- |
| `exa` | Exa | `api_key` | 默认请求 highlights |
| `tavily` | Tavily | `api_key` | 高级搜索消耗 2 积分 |
| `jina` | Jina AI Search | `api_key` | 查询放在 URL 路径中 |
| `firecrawl` | Firecrawl | `api_key` | |
| `linkup` | Linkup | `api_key` | 输出 `searchResults` |
| `perplexity_search` | Perplexity Search API | `api_key` | 独立的 `/search`，非 Sonar |
| `valyu` | Valyu | `api_key` | 专有搜索需订阅 |
| `parallel` | Parallel | `api_key` | 由查询构造 `search_queries` |
| `kagi` | Kagi | `api_key` | FastGPT/enrich 端点 |

### 学术与开发者搜索

| 代码 | 用途 | 凭据 | 说明 |
| --- | --- | --- | --- |
| `openalex` | OpenAlex | — | 由倒排索引重建摘要 |
| `semanticscholar` | Semantic Scholar | `api_key`（可选） | 无密钥时约 1 rps |
| `crossref` | Crossref | — | DOI 链接规范化为 `https://doi.org/` |
| `github` | GitHub Search | `api_key` | 仓库/代码/issues/commits/用户 |
| `stackexchange` | Stack Exchange | `api_key`（可选） | 遵循 API 的 `backoff` |
| `pubmed` | PubMed（NCBI） | `api_key`（可选），env `NCBI_API_KEY` | 两步 esearch→esummary（+ 可选 efetch） |

### 企业搜索

| 代码 | 用途 | 凭据 | 说明 |
| --- | --- | --- | --- |
| `azure_search` | Azure AI Search | `api_key` 或 Entra（`tenant_id`, `client_id`, `client_secret`） | 可配置 `title_field`/`url_field`/`snippet_field` |
| `vertex_search` | Vertex AI Search（Discovery Engine） | `service_account_json` | 仅 OAuth2，不支持 API 密钥 |
| `vectara` | Vectara | `api_key` | 关闭生成（保持 Row 输出） |
| `kendra` | Amazon Kendra | `access_key_id`, `secret_access_key` | **已弃用** — 未实现 AWS SigV4 |

### 生成式回答节点

这些驱动应放在 **`mode: answer`** 节点上：生成的文本进入请求的回答面板，
来源则变为 sparse 行（链接+标题）。

| 代码 | 用途 | 凭据 | 说明 |
| --- | --- | --- | --- |
| `anthropic` | Anthropic web search | `api_key` | `web_search` 工具 + citations |
| `yandex_gen` | Yandex AI Studio（生成式回答） | `api_key`, `folder_id` | `/v2/gen/search` |

每个提供方的 `base_url` 可在 UI 中编辑（不是秘密）。秘密为只写：值永远不会返回，
只会显示「已设置」标记。「Test」按钮会执行探测查询 `q=test`，并显示 `http/kind/ms`
以及前几条链接。

**提供方即实例。** 在 Providers 页面可以添加任意数量的提供方，包括
**多个同类型（驱动）但名称和 `api_key` 不同**的实例 —— 例如「apiserpent work」和
「apiserpent personal」。每个实例用开关启用/停用；停用的实例不会出现在链路编辑器的
面板中。删除被链路引用的实例会被拒绝，返回 `409` 及链路列表。从旧版数据库升级时，
原有记录会自动变为实例（`id = code`），链路引用保持不变。

**首次启动时**只会创建并启用 `google_vertex`（Vertex/Grounding），
默认链路由单个 `google_vertex` 块组成。不会创建其他实例 ——
请在 Providers 页面添加所需的驱动。

### SearXNG：仅外部实例

SerpentSeek **不自行运行 SearXNG** —— compose 中没有该服务。请将其指向你已有 SearXNG
的 URL：

* 在 `searxng` 提供方的 `base_url` 字段中（Providers 页面，无需重启），或
* 通过 `.env` 中的 `SEARXNG_URL`。

既支持普通 URL（`http://host:port/search`），也支持带 `{query}`/`<query>` 的模板。
在设置 URL 之前，该提供方被视为不可用（`skip`），链路会跳过它继续执行。

你的实例要求：`search.formats: [html, json]` 和 `limiter: false`
（否则会得到 403/429）。示例配置见 `deploy/searxng/settings.yml`；
实例由部署它的人负责。

> **⚠️ SearXNG 使用 AGPLv3 许可证。** 它是外部服务；SerpentSeek 仅通过 HTTP 与之通信，
> 不进行任何链接。遵守 SearXNG 许可证的责任由部署它的人承担。

## PostgreSQL（`extdb` profile）

```bash
# 在 .env 中：POSTGRES_PASSWORD=... STORAGE_DRIVER=postgres \
#           DATABASE_URL=postgres://serpent:...@postgres:5432/serpentseek?sslmode=disable
docker compose --profile extdb up -d --build
```

PostgreSQL 是独立容器（PostgreSQL License），**不是** SerpentSeek 的代码依赖。
默认使用 SQLite：`journal_mode=WAL`、`foreign_keys=ON`、`busy_timeout=5000`、
`synchronous=NORMAL`、单写入者。多副本部署需要 PostgreSQL。

## 设置

所有变量见 [`.env.example`](.env.example)。环境变量优先；这些字段在 UI 中带有 `env`
徽标且为只读。其他设置均可在 Settings 页面修改，无需重启
（`STORAGE_DRIVER` 除外，UI 会如实提示）。

提供方密钥（`api_key`、`folder_id`、`cx` 等）和 `base_url` 是**实例属性**：
在 Providers 页面设置。首次启动时唯一会为提供方注入值的环境变量是
`GOOGLE_API_KEY`、`GOOGLE_MODEL` 和 `GOOGLE_DRIVER`（用于默认的 `google_vertex`
实例）；其他凭据均按实例设置，没有 env 默认值。在 Settings 页面，提供方只保留
**重试码**（`RETRY_HTTP_CODES`、`AP_RETRY_CODES`）—— 全局默认值，每个实例可通过自己的
`params` 覆盖。致命码表（`fatal_codes`、`fatal_http`）是实例参数，在 Providers 页面编辑。

关键变量：`PORT`、`HOST`、`DATA_DIR`/`SQLITE_PATH`、`STORAGE_DRIVER`、
`DATABASE_URL`、`AUTH_ENABLED`、`PUBLIC_ORIGIN`、`RP_ID`、`RP_NAME`、
`HISTORY_RETENTION_DAYS`、`LOGS_RETENTION_DAYS`、`MAX_CONCURRENCY`、`MAX_ATTEMPTS`、
`USER_AGENT`、`LOG_LEVEL`、`LOG_FORMAT`、`TREAT_EMPTY_AS_FAIL`。

`IGNORE_REQUEST_COUNT=true` 会让网关忽略请求中的 `count` 并向提供方传递 `num=0` ——
返回找到的全部结果，不设上限。也可以在 **Playground** 页面的「Results」字段中手动填入
相同的值（`0`）（悬停该字段时会显示提示）。

## 界面与本地化

UI 已翻译为 6 种语言：

| 代码 | 语言 |
| --- | --- |
| `ru` | Русский |
| `en` | English |
| `pl` | Polski |
| `de` | Deutsch |
| `fr` | Français |
| `zh` | 中文 |

语言切换器位于右上角、「Quick search」按钮旁。所选语言保存在 `localStorage` 中；
首次打开时根据 `navigator.language` 检测，否则使用英语。日期按所选区域设置的规则格式化
（`Intl.DateTimeFormat`）。

添加新语言：将字典放入 `web/src/lib/i18n/<code>.json`（键集合必须与 `en.json` 一致），
在 `web/src/lib/i18n/languages.json` 中添加条目，并在 `web/src/lib/i18n.svelte.ts` 的
`Locale` 类型中加入该代码。

## 身份验证

* **密钥：** `seek_ak_<prefix>_<secret>`（search scope）和
  `seek_admin_<prefix>_<secret>`（admin scope）。仅存储 argon2id 哈希；
  UI/API 中只可见前缀。
* **会话：** httpOnly + Secure + SameSite=Strict Cookie，滚动 30 天，
  所有变更请求进行 CSRF 双重提交校验。
* **Passkeys（WebAuthn）：** 用密钥登录后注册，之后可无密码登录。需要 HTTPS + 域名
  （`PUBLIC_ORIGIN`/`RP_ID`）；在普通 HTTP/IP 下按钮会隐藏并给出警告。
* **`AUTH_ENABLED=false`** —— 整个管理 API 无需登录即可使用（UI 中会显示提示横幅，
  可在设置中隐藏）。

## 后台任务

* `cleanup`（`HISTORY_CLEANUP_CRON`，默认 `0 4 * * *`）—— 删除超过保留期的历史与日志、
  过期会话，并重算每日聚合。
* `vacuum`（`VACUUM_CRON`，默认 `0 3 * * 0`）—— 仅 SQLite。

两者都可以在 Settings → Maintenance 手动触发（或通过 `POST /api/maintenance/*`）。
任务 panic 不会导致进程崩溃。

## 开发

```bash
make build      # SPA + Go 二进制（embed）
make test       # go test ./... -cover
make check      # svelte-check
make lint       # go vet + golangci-lint
make dev        # Vite 开发服务器代理到 :8080
```

后端监听 `:8080`；SPA 通过 `go:embed web/build` 嵌入。

## 依赖与许可证

仅允许 MIT / Apache-2.0 / BSD-2/3 / ISC。审计：

```bash
go run github.com/google/go-licenses@latest report ./...
cd web && npx license-checker --production --excludePrivatePackages
```

| 依赖 | 版本 | 许可证 |
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

卫星容器（非代码依赖）：`postgres:17-alpine`（PostgreSQL License）和外部
`searxng/searxng`（不在本仓库中；**AGPLv3** —— 独立服务，不是代码依赖）。

## 风险与注意事项

* **Passkeys 需要 HTTPS + 域名**（WebAuthn rpID）。在纯 IP/HTTP 下只能使用密钥；
  UI 会对此发出警告。
* **Google Custom Search JSON API** 已对新客户关闭，并将于 **2027-01-01** 关停 ——
  该驱动已标记为弃用，默认使用 Vertex/Grounding。
* **提供方是付费服务。** 所有请求都使用**部署者自己的 API 密钥**，并由提供方计费
  （Yandex Cloud、Google Cloud、apiserpent、SerpBase）。请在**提供方侧**配置预算、告警和
  密钥配额，并控制重试次数（最大尝试次数、链路重试策略）：项目作者不对费用负责
  （见 `NOTICE`）。
* **Yandex Search API v2** 为付费服务：需要 Api-Key + folderId（Yandex Cloud）。
* **SearXNG** 是外部服务：需要 `search.formats: [html, json]` 和 `limiter: false`；
  网关令牌（`X-API-Key`）**不是** `server.secret_key`。SearXNG 本身使用 **AGPLv3**
  许可证，不属于本仓库。
* **SQLite** 为单写入者；WAL + busy_timeout 对单副本足够。
* **提供方凭据**为只写，并在 API 和日志中脱敏；`base_url` 不是秘密。
  可选 `ENCRYPTION_KEY`（AES-GCM）用于静态加密。

## 截图

启动后可看到以下界面：Dashboard、History（带彩色链路图和步骤时间线）、Chains（可视化
编辑器）、Providers、Playground、MCP、Logs、Users、Settings。请将截图放入
`docs/screenshots/`（为避免镜像体积膨胀，仓库中未包含截图）。

## 目录结构

```
cmd/serpentseek/      # 组装与 seed
internal/
  config logging store providers engine sse auth httpd jobs version
api/openapi.yaml      # OpenAPI 3.1
web/                  # SvelteKit SPA（构建产物嵌入二进制）
deploy/
  searxng/settings.yml
  caddy.example
```

---

<sub>**关键词：** 搜索网关，自托管，统一搜索 API，元搜索，元搜索代理，Open WebUI 联网搜索，Open WebUI 搜索后端，SearXNG，MCP 服务器，MCP 搜索工具，Model Context Protocol 联网搜索，面向 LLM 与 AI 助手的搜索工具，RAG 搜索，Brave Search API，Google Vertex AI Search，Grounding with Google Search，Yandex Search API，SerpApi，Serper，DataForSEO，Exa，Tavily，Kagi，Perplexity Search API，提供方回退链路，搜索聚合器，OpenAPI 3.1，Go 微服务，单 Docker 容器。</sub>
