# 🐍 SerpentSeek — Open WebUI、MCP、40以上の検索API向けのセルフホスト型検索ゲートウェイ

[English](README.md "Read in English") | [Русский](README.ru.md "README по-русски") | [Polski](README.pl.md "Czytaj po polsku") | [Deutsch](README.de.md "Auf Deutsch lesen") | [Français](README.fr.md "Lire en français") | **日本語** | [中文](README.zh.md "阅读中文版")

[![License: GPL-3.0](https://img.shields.io/badge/license-GPL--3.0-blue.svg)](LICENSE "Project license — GNU GPL v3.0")
[![Go 1.26](https://img.shields.io/badge/Go-1.26-00ADD8?logo=go&logoColor=white)](go.mod "Backend written in Go")
[![CI](https://github.com/liberide/serpent-seek/actions/workflows/ci.yml/badge.svg)](https://github.com/liberide/serpent-seek/actions/workflows/ci.yml "CI: build, vet, tests")
[![Docker — single container](https://img.shields.io/badge/docker-single_container-2496ED?logo=docker&logoColor=white)](Dockerfile "Ships as a single Docker container")
[![OpenAPI 3.1](https://img.shields.io/badge/API-OpenAPI_3.1-6BA539)](api/openapi.yaml "OpenAPI 3.1 specification")
[![40 search providers](https://img.shields.io/badge/built--in_search_providers-40-orange)](#providers "40 built-in search provider drivers")

**SerpentSeek** は、オープンソースの **セルフホスト型検索ゲートウェイ** です。Go で書かれた統合 Web 検索 API（メタサーチプロキシ）で、**Open WebUI** 向けに作られた多言語 Web 管理 UI を備えています。すべてのクエリは **40 の組み込み検索プロバイダー**（SearXNG、Brave、Google Vertex AI Search / Grounding、Yandex、SerpApi、Serper、DataForSEO、Exa、Tavily、Kagi、Perplexity など）を組み合わせた設定可能なチェーンを通り、ビジュアルなノード＆エッジエディターで設計できます。標準搭載: ライブトレーシング（SSE）付きのリクエスト履歴、API キーとパスキー（WebAuthn）、AI アシスタント（Claude、Cursor、VS Code など）向けの **MCP 検索ツール**（ストリーミング可能な HTTP）、Open WebUI 互換の `/search` エンドポイント、SQLite ⟷ PostgreSQL ストレージ、バックグラウンドクリーンアップジョブ。すべてが単一の Docker コンテナで提供されます。

プロジェクトライセンス — **GNU 3.0**（GPL-3.0-only、`LICENSE` を参照）。帰属情報は `NOTICE`/`AUTHORS` にあります。バイナリと Web バンドルにコンパイルされるすべてのサードパーティコンポーネントのライセンスは `THIRD_PARTY_LICENSES.md` にまとめられています。脆弱性は非公開で報告してください（`SECURITY.md` を参照）。コントリビューションのルールは `CONTRIBUTING.md` にあります（DCO sign-off + [CLA](CLA.md)）。

本ソフトウェアは **「現状のまま」、いかなる保証もなく** 提供されます。リクエストはサービスをデプロイした人のキーを使ってサードパーティの検索 API に送信され、課金はオペレーターとプロバイダー間の問題です。作者はそのような課金やその他の物質的損害に対して責任を負いません（GPLv3 §§15–16、詳細は `NOTICE`）。

## 主な機能

* 🔎 **40 の検索プロバイダーを 1 つのエンドポイントで** — Web/SERP（SearXNG、Brave、Google、Yandex、SerpApi など）、AI/ニューラル（Exa、Tavily、Perplexity、Kagi など）、学術（OpenAlex、PubMed、Crossref、Semantic Scholar など）、エンタープライズ（Azure AI Search、Vertex AI Search、Vectara など）
* 🧩 **ビジュアルチェーンエディター** — 実際のノード＆エッジグラフ: 論理的な **Start** ブロックが複数のプロバイダーに分岐し、論理的な **Join** ブロックがブランチをマージして重複 URL を除去し、残りはフォールバック/リトライポリシーが制御します。ドライバーごとに複数の名前付きインスタンスを持てます
* 🔗 **トークンごとのチェーンルーティング** — 各 API キーを独自のチェーンにバインドし、アクティブなチェーンとは独立させられます（未バインドのトークンは既定/アクティブチェーンに従います）。チェーンを削除してもバインドされたトークンは安全に既定へ戻ります
* 🔌 **Open WebUI 対応** — そのまま使える `/search` エンドポイント: 常に HTTP 200、`[{"link","title","snippet"}]`
* 🤖 **MCP サーバー** — Claude、Cursor、VS Code など任意の MCP クライアント向けの、ストリーミング可能な HTTP 上の `search` ツール。プレーンな JSON-RPC 2.0 も受け付けます
* 🛠️ **開発者モードとトークン** — 管理者は開発者モードを有効にして、完全な管理者権限を持つ専用の `seek_dev_…` トークンを発行でき、サードパーティのシステムがプロバイダー、チェーン、ユーザー、キー、設定をすべて API 経由で管理できます。モードをオフにすると、これらのトークンは即座に拒否されます
* 🔭 **ライブリクエストトレーシング** — SSE によるステップごとのタイムラインと、`X-Serpent-Rid` / `X-Serpent-Api` 診断ヘッダー、履歴ページ
* 📊 **Prometheus メトリクス** — `GET /metrics` が検索結果、プロバイダーの試行/所要時間、実行中リクエスト、SSE クライアントをテキスト形式で公開します
* 🔔 **更新通知** — 新しいリリースの任意チェック。サイドバーのバッジと **設定 → 情報** ページの詳細
* 🔐 **セキュリティ第一** — argon2id ハッシュの API キー、WebAuthn パスキー、CSRF 付き SameSite=Strict セッション、書き込み専用シークレット、任意の AES-GCM 保存時暗号化
* 📋 **監査ログ** — すべての管理操作（設定、プロバイダー、チェーン、キー、ユーザー）を実行者、IP、非シークレットの詳細とともに記録
* 💱 **プロバイダーコスト追跡** — 任意のインスタンスごとの `cost_per_request` を分析でプロバイダー別に合計（未設定時は完全に非表示）
* 📦 **構成のエクスポート/インポート** — インスタンス間の移行のための、プロバイダー、チェーン、非シークレット設定のシークレットを含まないスナップショット
* 💾 **SQLite ⟷ PostgreSQL** — 既定は WAL モードの SQLite。`extdb` compose プロファイルでマルチレプリカ構成用の PostgreSQL を利用可能
* 🌍 **多言語 UI** — English, Русский, Polski, Deutsch, Français, 日本語, 中文
* 🐳 **単一 Docker コンテナ** — 埋め込み SvelteKit SPA を備えた Go バックエンド

## 目次

* [クイックスタート](#クイックスタート)
  * [Open WebUI との連携](#open-webui-との連携)
  * [MCP 連携（Model Context Protocol）](#mcp-連携model-context-protocol)
* [プロバイダー](#プロバイダー)
  * [SearXNG: 外部インスタンスのみ](#searxng-外部インスタンスのみ)
* [チェーンブロック](#チェーンブロック)
* [PostgreSQL（プロファイル `extdb`）](#postgresqlプロファイル-extdb)
* [設定](#設定)
* [インターフェースとローカリゼーション](#インターフェースとローカリゼーション)
* [認証](#認証)
* [バックグラウンドジョブ](#バックグラウンドジョブ)
* [開発](#開発)
* [運用](docs/operations.md) — バックアップ、復元、アップグレード、診断
* [依存関係とライセンス](#依存関係とライセンス)
* [リスクと注意事項](#リスクと注意事項)
* [構成](#構成)

## クイックスタート

```bash
cp .env.example .env
# open .env and set provider keys if needed (SEARXNG_URL etc.)
docker compose up --build
```

UI: <http://localhost:8080>。ヘルスチェック: `curl http://localhost:8080/healthz`
（DB を考慮したレディネスプローブは `/readyz`、純粋なライブネスは `/livez`）。

ソースからビルドせず、Docker Hub の公式ビルド済みイメージを使う場合:

```bash
docker pull sallend/serpent-seek:latest
docker run -d --name serpentseek -p 8080:8080 \
  --env-file .env -v serpentseek_data:/app/data \
  sallend/serpent-seek:latest
```

その他のデプロイ方法 — [DEPLOY.md](DEPLOY.md) を参照。バックアップ、復元、
アップグレード、診断 — [docs/operations.md](docs/operations.md) を参照。

初回起動時に一度だけ有効なトークンがログに出力されます:

```text
SETUP_TOKEN: ff1089a6
```

`/setup` を開き、トークンを入力して管理者を作成します。**API キーは一度しか
表示されません** — 保存してください。その後 `/login` でそのキーを使ってログインします。

### Open WebUI との連携

Open WebUI → *設定 → Web 検索* で **External**（または SearXNG）を選択し、次を設定します:

* URL: `http://<host>:8080/search`
* API キー: あなたの `seek_ak_...` キー

`POST /search` エンドポイントは Open WebUI 互換です: ボディ `{"query":"...","count":5}`、
レスポンスは HTTP 200 で配列 `[{"link","title","snippet"}]`（失敗/結果なしのときは
空の `[]`）。唯一の例外はスロットリングです: 呼び出し元が
`SEARCH_RATE_LIMIT_PER_MINUTE` または `SEARCH_DAILY_QUOTA` を超えると、エンドポイントは
`Retry-After` 付きの HTTP 429 を返します（[設定](#設定)を参照）。診断 —
`X-Serpent-Rid` と `X-Serpent-Api` ヘッダーおよび履歴ページから。

```bash
curl -s http://localhost:8080/search \
  -H "Authorization: Bearer seek_ak_..." \
  -H "Content-Type: application/json" \
  -d '{"query":"hello world","count":3}'
```

### MCP 連携（Model Context Protocol）

SerpentSeek は `/mcp` で MCP 上の `search` ツールを公開します（公式の **streamable HTTP**
トランスポート: GET で SSE ストリーム、POST で JSON-RPC、DELETE でセッションを閉じる）。
同じエンドポイントは、完全なストリーミングトランスポートをサポートしないクライアント向けに、
プレーンな HTTP JSON-RPC 2.0 のリクエスト/レスポンス URL としても機能します。API キーを
Bearer トークンとして渡してください。

**ネイティブ HTTP MCP クライアント**（Claude Desktop、一部の Cursor ビルド、Open WebUI MCP ツールなど）向け:

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

**stdio 専用 MCP クライアント**（古い Cursor、VS Code 拡張など）向けには、`mcp-remote`
ラッパーを使って、リモート HTTP エンドポイントをローカル stdio サーバーのように見せます:

```bash
npx mcp-remote http://<host>:8080/mcp --header "Authorization=Bearer seek_ak_..."
```

その後、クライアントには URL の代わりにラッパーコマンドを指定します。

ツールは `search` のみで、`query`（必須）と `count`（任意、0 = すべて、既定 5）を取ります。
各呼び出しはアクティブなプロバイダーチェーンを実行し、結果ごとに
`title` / `url` / `snippet` を返します。エンドポイントは管理 UI の **MCP** ページで
説明されています（コピー可能な設定スニペットとライブテスト付き）。

## プロバイダー

組み込み検索ドライバーの全一覧。認証情報は書き込み専用で、**プロバイダー** ページで
インスタンスごとに設定します（`google_vertex` のシードだけが環境変数 `GOOGLE_*` を読み取ります）。

### Web 検索 / SERP

| コード | 用途 | 認証情報 | 備考 |
| --- | --- | --- | --- |
| `apiserpent` | apiserpent.com クイック検索 | `api_key` | エンジンローテーション google/bing/yahoo/ddg/brave |
| `serpbase` | serpbase.dev | `api_key` | 致命的/リトライステータステーブルを設定可能 |
| `searxng` | SearXNG — **外部インスタンス** | `api_key`（任意、ゲートトークン） | URL は UI/`SEARXNG_URL` で設定。`format=json` と `limiter: false` が必要 |
| `yandex` | Yandex Search API v2（Yandex Cloud） | `api_key`, `folder_id` | `/v2/web/search`、`rawData` 内の base64 XML |
| `google_vertex` | Google 検索 / Vertex AI Search による Gemini Grounding | `api_key`, `project_id`, `engine_id` | Google の主要パス。env `GOOGLE_API_KEY`、`GOOGLE_MODEL`、`GOOGLE_DRIVER` |
| `google` | Google（ディスパッチャー） | `api_key`, `cx`, `project_id`, `engine_id` | `driver` パラメータで vertex/cse を切り替え |
| `google_cse` | Google Custom Search JSON API | `api_key`, `cx` | **非推奨**、2027-01-01 終了 |
| `brave` | Brave Search | `api_key` | |
| `serper` | Serper | `api_key` | |
| `serpapi` | SerpApi | `api_key` | マルチエンジン（`engine` パラメータ） |
| `searchapi` | SearchApi.io | `api_key` | |
| `dataforseo` | DataForSEO | `login`, `password` | HTTP Basic 認証 |
| `youcom` | You.com | `api_key` | |
| `mojeek` | Mojeek | `api_key` | キーはクエリ文字列で送信 |
| `marginalia` | Marginalia | `api_key` | 公開キーはレート制限あり（503 はリトライ可能） |
| `hn` | Hacker News（Algolia） | — | キー不要 |

### AI / ニューラル検索

| コード | 用途 | 認証情報 | 備考 |
| --- | --- | --- | --- |
| `exa` | Exa | `api_key` | 既定でハイライトを要求 |
| `tavily` | Tavily | `api_key` | 高度な検索は 2 クレジット |
| `jina` | Jina AI Search | `api_key` | クエリは URL パスに入る |
| `firecrawl` | Firecrawl | `api_key` | |
| `linkup` | Linkup | `api_key` | `searchResults` 出力 |
| `perplexity` | Perplexity Search API | `api_key` | スタンドアロンの `/search`、Sonar ではない |
| `valyu` | Valyu | `api_key` | 独自検索はサブスクリプションが必要 |
| `parallel` | Parallel | `api_key` | クエリから `search_queries` を構築 |
| `ollama` | Ollama Web Search | `api_key` | 既定 5 件、最大 10 件 |
| `kagi` | Kagi | `api_key` | FastGPT / Search API |
| `kimi_search` | Kimi（Moonshot）Web Search | `api_key` | title/url/snippet。`include_content` でページ本文を取得 |
| `kimi_search_pro` | Kimi（Moonshot）Web Search Pro | `api_key` | ランク付きコンテンツチャンク。`sites`/`time_window` フィルター |

### 学術・開発者向け

| コード | 用途 | 認証情報 | 備考 |
| --- | --- | --- | --- |
| `openalex` | OpenAlex | — | 転置インデックスから要約を再構成 |
| `semanticscholar` | Semantic Scholar | `api_key`（任意） | キーなしで約 1 rps |
| `crossref` | Crossref | — | DOI リンクは `https://doi.org/` に正規化 |
| `github` | GitHub Search | `api_key` | リポジトリ/コード/issue/コミット/ユーザー |
| `stackexchange` | Stack Exchange | `api_key`（任意） | API の `backoff` を尊重 |
| `pubmed` | PubMed（NCBI） | `api_key`（任意）、env `NCBI_API_KEY` | 2 段階の esearch→esummary（+ 任意の efetch） |

### 法人 / エンタープライズ

| コード | 用途 | 認証情報 | 備考 |
| --- | --- | --- | --- |
| `azure_search` | Azure AI Search | `api_key` または Entra（`tenant_id`, `client_id`, `client_secret`） | `title_field`/`url_field`/`snippet_field` を設定可能 |
| `vertex_search` | Vertex AI Search（Discovery Engine） | `service_account_json` | OAuth2 のみ、API キー不可 |
| `vectara` | Vectara | `api_key` | 生成は無効（Row 出力） |
| `kendra` | Amazon Kendra | `access_key_id`, `secret_access_key` | **非推奨** — AWS SigV4 は未実装 |

### 生成回答ノード

これらのドライバーは **`mode: answer`** のチェーンノードに配置します: 生成されたテキストは
リクエストの回答パネルに送られ、ソースは疎な行（リンク+タイトル）になります。

| コード | 用途 | 認証情報 | 備考 |
| --- | --- | --- | --- |
| `anthropic` | Anthropic Web 検索 | `api_key` | `web_search` ツール + 引用 |
| `yandex_gen` | Yandex AI Studio（生成回答） | `api_key`, `folder_id` | `/v2/gen/search` |

各プロバイダーの `base_url` は UI で編集できます（シークレットではありません）。シークレットは
書き込み専用です: 値は決して返されず、「設定済み」マーカーのみが表示されます。「テスト」ボタンは
プローブクエリ `q=test` を実行し、`http/kind/ms` と最初のリンクを表示します。
任意パラメータ `cost_per_request`（正の小数、単位は任意）を設定すると、インスタンスの
推定コストが試行ごとに記録され、分析で合計されます。未設定のままにすると、そのプロバイダーは
コストレポートから除外されます。

**プロバイダーはインスタンスです。** プロバイダーページでは、任意の数のプロバイダーを追加でき、
**同じ種類（ドライバー）で名前と `api_key` が異なる複数** も可能です — 例えば
「apiserpent work」と「apiserpent personal」。各インスタンスはスイッチでオン/オフを切り替え、
無効なものはチェーンエディターのパレットに表示されません。チェーンから参照されている
インスタンスの削除は `409` とチェーンのリストで拒否されます。古い DB バージョンから
アップグレードすると、以前のレコードは自動的にインスタンスになり（`id = code`）、
チェーンの参照は保持されます。

**初回起動時** は `google_vertex`（Vertex/Grounding）だけが作成・有効化され、既定の
チェーンは単一の `google_vertex` ブロックで構成されます。他のインスタンスは作成されません —
必要なドライバーはプロバイダーページで追加してください。

### SearXNG: 外部インスタンスのみ

SerpentSeek は **SearXNG 自体を実行しません** — compose にそのようなサービスはありません。
すでに稼働している SearXNG の URL を指定してください:

* `searxng` プロバイダーの `base_url` フィールド（プロバイダーページ、再起動不要）、または
* `.env` の `SEARXNG_URL`。

プレーンな URL（`http://host:port/search`）と、`{query}`/`<query>` を含むテンプレートの
両方をサポートします。URL が設定されるまで、プロバイダーは dead（`skip`）と見なされ、
チェーンはそれなしで継続します。

インスタンスの要件: `search.formats: [html, json]` と `limiter: false`
（そうでないと 403/429 になります）。設定例 — `deploy/searxng/settings.yml`。
インスタンスの所有者はそれを実行する人です。

> **⚠️ SearXNG は AGPLv3 の下でライセンスされています。** これは外部サービスです。
> SerpentSeek は HTTP 経由で通信するだけで、何もリンクしていません。SearXNG ライセンスの
> 遵守責任は、それをデプロイする人にあります。

## チェーンブロック

ビジュアルエディターは 3 種類のブロックのグラフを構築します:

* **Start**（`kind: start`）— 論理的な入口。プロバイダーを持たず、複数のプロバイダーに
  一度に分岐します。チェーンごとに Start ブロックはちょうど 1 つです。
* **Provider**（`kind: provider`、既定）— 設定済みのプロバイダーインスタンスを実行します。
  タイムアウト、リトライ、バックオフ、結果ポリシーはここにあります。
* **Join**（`kind: join`）— 論理的なマージ点: そこに流れ込むすべてのブランチを待ち、
  結果をマージして重複 URL を除去します（`utm_*` などのトラッキングパラメータは
  比較前に除去されます）。

**モード** セレクターがグラフの実行方法を決めます:

* **First success** — 線形フォールバック。最初の使用可能な結果でリクエストが終了します。
  `on_empty`/`on_fail` が次のブロックを選びます。`on_success: next|edge` はブロックの
  行をマージして進行を続けます。
* **Full chain** — Start の後のブロックが分岐します: 兄弟ブランチは並列で実行され
  （`MAX_CONCURRENCY` で制限）、Join がそれらを同期し、すべてのプロバイダーの行が
  1 つの重複排除された結果にマージされます。

**Join** ブロックは結果件数を無視することもできます（`ignore_count`）: これをオンにすると、
プロバイダーは制限なしでクエリされ、リクエストの `count` で制限する代わりに、見つかった
すべての一意の結果が返されます。実行トレースでは、Join ブロックは合計リクエスト時間も
表示します。

### 後方互換性

既存のチェーンはデータベース内で移行されません。マイグレーション `00008` は既定値
`provider` を持つ `kind` を追加するだけで、従来の `is_start` フラグは引き続き機能します。
そのため、アップグレードしたインスタンスは、保存されたすべてのチェーンを以前とまったく同じに
実行します:

* 明示的な Start ブロックを持たないチェーンは、従来の線形ウォークを維持します
  （full-chain モードでは、これまでどおりブロックごとに 1 本の中立な `next` エッジをたどります）。
* `on_success` はそれらのチェーンでは no-op のままです（成功したブロックは常にリクエストを
  終了していたため）、新しく実装された継続はオプトインです。
* そのようなチェーンをエディターで開くと、可視の Start ブロックが表示され、古い入口に
  接続され、`on_success: stop` が固定されます — グラフはより明示的に見えますが、
  結果は変わりません。

新しい並列ファンアウト、Join マージ、`on_success` 継続は、実際に Start ブロックを
含むチェーンにのみ適用されます。

### トークンごとのチェーンバインディング

各 API キーは、どのチェーンがアクティブかとは独立して、特定のチェーンにバインドできます。
バインディングは管理者が **ユーザーとキー** ページで設定し、次のリクエストから有効になります —
キーのローテーションは不要です:

* バインドされたトークンは、非アクティブなものでも常に自分のチェーンを実行します。複数の
  トークンが同じチェーンを共有できます。
* 未バインドのトークンは **既定** チェーンにフォールバックします。これは単一の `active`
  チェーンです（別個の「既定チェーン」エンティティはありません）。
* バインドされたチェーンが消えた場合、バインディングはクリアされ、トークンは安全に
  アクティブチェーンへフォールバックします。意図的にバインドされたチェーンが存在しない場合は、
  黙って別のチェーンを実行するのではなく、クローズドに失敗します。
* チェーンページには、そのチェーンを使用するトークンの数（`🔑 N`、アクティブなものを含む）が
  表示され、アクティブなトークンがバインドされたチェーンを削除する前に警告します —
  確認後、それらのトークンは既定チェーンに移動します。アクティブチェーン自体は削除できません。

## PostgreSQL（プロファイル `extdb`）

```bash
# in .env: POSTGRES_PASSWORD=... STORAGE_DRIVER=postgres \
#          DATABASE_URL=postgres://serpent:...@postgres:5432/serpentseek?sslmode=disable
docker compose --profile extdb up -d --build
```

PostgreSQL は別個のコンテナ（PostgreSQL License）であり、SerpentSeek のコード依存では
**ありません**。SQLite が既定です: `journal_mode=WAL`、`foreign_keys=ON`、
`busy_timeout=5000`、`synchronous=NORMAL`、単一ライター。マルチレプリカ構成には
PostgreSQL が必要です。

## 設定

すべての変数は [`.env.example`](.env.example) にあります。環境変数の値が優先され、
そのようなフィールドは UI で `env` バッジ付きで読み取り専用として表示されます。その他の
すべての設定は、再起動なしで設定ページから変更できます（UI が正直に警告する
`STORAGE_DRIVER` を除く）。

プロバイダーキー（`api_key`、`folder_id`、`cx` など）と `base_url` は **インスタンスの
プロパティ** です: プロバイダーページで設定します。初回起動時にプロバイダーをシードする
唯一の環境変数は `GOOGLE_API_KEY`、`GOOGLE_MODEL`、`GOOGLE_DRIVER`（既定の
`google_vertex` インスタンス用）です。他のすべての認証情報はインスタンスごとで、
env の既定値はありません。設定ページでは、プロバイダーは **リトライコード**
（`RETRY_HTTP_CODES`、`AP_RETRY_CODES`）のみを保持します — すべてのインスタンスが自身の
`params` で上書きできるグローバルな既定値です。致命的コードテーブル（`fatal_codes`、
`fatal_http`）はインスタンスごとのパラメータで、プロバイダーページで編集します。

主な変数: `PORT`、`HOST`、`DATA_DIR`/`SQLITE_PATH`、`STORAGE_DRIVER`、
`DATABASE_URL`、`AUTH_ENABLED`、`PUBLIC_ORIGIN`、`RP_ID`、`RP_NAME`、
`HISTORY_RETENTION_DAYS`、`LOGS_RETENTION_DAYS`、`USAGE_RETENTION_DAYS`、
`AUDIT_RETENTION_DAYS`、`MAX_CONCURRENCY`、`MAX_ATTEMPTS`、
`USER_AGENT`、`LOG_LEVEL`、`LOG_FORMAT`、`TREAT_EMPTY_AS_FAIL`、
`SEARCH_RATE_LIMIT_PER_MINUTE`、`SEARCH_DAILY_QUOTA`、`CHAIN_TIMEOUT`、
`PROVIDER_FAILURE_THRESHOLD`、`PROVIDER_COOLDOWN_SECONDS`、`DEBUG`、
`UPDATE_CHECK_ENABLED`、`UPDATE_CHECK_URL`、`UPDATE_CHECK_INTERVAL_HOURS`。

`IGNORE_REQUEST_COUNT=true` にすると、ゲートウェイはリクエストの `count` を無視し、
プロバイダーに `num=0` を渡します — 制限なしで見つかったすべてを返します。同じ値（`0`）は
**プレイグラウンド** ページの「結果」フィールドで手動設定もできます（フィールドに
ホバーするとヒントが表示されます）。

## インターフェースとローカリゼーション

UI は 7 言語に翻訳されています:

| コード | 言語 |
| --- | --- |
| `ru` | Русский |
| `en` | English |
| `pl` | Polski |
| `de` | Deutsch |
| `fr` | Français |
| `ja` | 日本語 |
| `zh` | 中文 |

言語切り替えは右上隅、「クイック検索」ボタンの隣にあります。選択した言語は `localStorage`
に保存され、初回オープン時は `navigator.language` から検出され、それ以外は英語が
使われます。日付は選択したロケールの規則（`Intl.DateTimeFormat`）に従って
フォーマットされます。

新しい言語を追加するには: `web/src/lib/i18n/<code>.json` に辞書を置き（キーセットは
`en.json` と一致する必要があります）、`web/src/lib/i18n/languages.json` にエントリを追加し、
`web/src/lib/i18n.svelte.ts` の `Locale` 型にコードを追加します。

## 認証

* **キー:** `seek_ak_<prefix>_<secret>`（scope search）および `seek_admin_<prefix>_<secret>`
  （scope admin）。保存されるのは argon2id ハッシュのみで、UI/API にはプレフィックスのみが
  表示されます。
* **セッション:** httpOnly + Secure + SameSite=Strict Cookie、30 日間のローリング、すべての
  変更に対する CSRF ダブルサブミット。
* **パスキー（WebAuthn）:** キーでサインインした後に登録し、その後はパスワードなしで
  サインインできます。HTTPS + ドメイン（`PUBLIC_ORIGIN`/`RP_ID`）が必要です。プレーンな
  HTTP/IP ではボタンは警告とともに非表示になります。
* **`AUTH_ENABLED=false`** — 管理 API 全体がサインインなしで動作します（UI にバナーが
  表示されます）。

## バックグラウンドジョブ

* `cleanup`（`HISTORY_CLEANUP_CRON`、既定 `0 4 * * *`）— 保持期間より古い履歴とログ、
  期限切れセッションを削除し、検索クォータカウンター（`usage_daily`、90 日）と監査記録
  （180 日）を整理し、日次集計を再計算します。
* `vacuum`（`VACUUM_CRON`、既定 `0 3 * * 0`）— SQLite のみ。

どちらも設定 → メンテナンスから手動で実行できます（または `POST /api/maintenance/*`）。
パニックしたジョブがプロセスをクラッシュさせることはありません。

## 開発

```bash
make build      # SPA + Go binary (embed)
make test       # go test ./... -cover
make check      # svelte-check
make lint       # go vet + golangci-lint
make dev        # Vite dev server proxied to :8080
```

バックエンドは `:8080` でリッスンします。SPA は `go:embed web/build` で埋め込まれます。

## 依存関係とライセンス

許可されるのは MIT / Apache-2.0 / BSD-2/3 / ISC のみです。監査:

```bash
go run github.com/google/go-licenses@latest report ./...
cd web && npx license-checker --production --excludePrivatePackages
```

| 依存関係 | バージョン | ライセンス |
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
| swagger-ui-dist | 5.33.1 | Apache-2.0 |

サテライトコンテナ（コード依存ではありません）: `postgres:17-alpine`（PostgreSQL License）と
外部の `searxng/searxng`（このリポジトリには含まれません。**AGPLv3** — 別個のサービスで、
コード依存ではありません）。

## リスクと注意事項

* **パスキーには HTTPS + ドメインが必要です**（WebAuthn rpID）。素の IP/HTTP ではキーのみが
  利用でき、UI はこれについて警告します。
* **Google Custom Search JSON API** は新規顧客への提供を終了しており、**2027-01-01** に
  終了します — ドライバーは非推奨とされ、既定は Vertex/Grounding です。
* **プロバイダーは有料サービスです。** すべてのリクエストは **サービスをデプロイした人の
  API キー** で行われ、プロバイダー（Yandex Cloud、Google Cloud、apiserpent、SerpBase）に
  よって課金されます。予算、アラート、キークォータは **プロバイダー側で** 設定し、
  リトライを制御してください（最大試行回数、チェーンのリトライポリシー）: プロジェクトの
  作者は課金に対して責任を負いません（`NOTICE` を参照）。
* **Yandex Search API v2** は有料です: Api-Key + folderId（Yandex Cloud）が必要です。
* **SearXNG** は外部サービスです: `search.formats: [html, json]` と `limiter: false` が
  必要です。ゲートトークン（`X-API-Key`）は `server.secret_key` では **ありません**。
  SearXNG 自体は **AGPLv3** の下でライセンスされており、このリポジトリの一部ではありません。
* **SQLite** は単一ライターです。WAL + busy_timeout は 1 レプリカには十分です。
* **プロバイダーの認証情報** は書き込み専用で、API とログではマスクされます。`base_url` は
  シークレットではありません。任意の `ENCRYPTION_KEY` が AES-256-GCM の保存時暗号化を
  有効にします。
* **検索スロットリングはオプトインです。** `SEARCH_RATE_LIMIT_PER_MINUTE` と
  `SEARCH_DAILY_QUOTA`（どちらも `0` = 無効/無制限）は、`/search`、`/mcp`、プレイグラウンド
  全体で、API キーごと（キーなしの場合はユーザー/IP ごと）のリクエストを制限します。
  カウンターはデータベース（`usage_daily`）に保存され、90 日後に整理されます。
* **更新チェックはオプトインです。** `UPDATE_CHECK_ENABLED=true` の場合、サーバーは
  公開 GitHub リリースフィードを 1 日 1 回程度取得し（テレメトリなし）、新しいバージョンが
  存在すると UI に通知が表示されます。エアギャップ環境では `false` に設定してください。
  `UPDATE_CHECK_URL` は環境変数でのみ変更できます（UI からは不可）。カスタムフィードを
  指定すると、サイドバーでインスタンスが **非公式** と表示されます。
* **信頼性の調整。** `CHAIN_TIMEOUT` は 1 回の検索を端から端まで制限します。
  `PROVIDER_FAILURE_THRESHOLD` + `PROVIDER_COOLDOWN_SECONDS` がプロバイダーごとの
  サーキットブレーカーを作動させます（クールダウン既定値 60 秒を除き、すべて `0` = 無効）。
* `TRUSTED_PROXIES` は転送ヘッダーが尊重されるプロキシを列挙します。
  `BLOCK_PRIVATE_NETWORKS=true` は外向き接続をパブリックアドレスに制限します。

## 構成

```
cmd/serpentseek/      # wiring and seed
internal/
  config logging store providers engine sse auth httpd jobs metrics version
api/openapi.yaml      # OpenAPI 3.1
web/                  # SvelteKit SPA (build is embedded into the binary)
deploy/
  searxng/settings.yml
  caddy.example
docs/
  operations.md       # backup, restore, upgrades, diagnostics
```

---

<sub>**キーワード:** セルフホスト型検索ゲートウェイ、統合検索 API、メタサーチプロキシ、メタ検索エンジン、Open WebUI Web 検索、Open WebUI 検索バックエンド、SearXNG JSON API、MCP サーバー検索ツール、Model Context Protocol Web 検索、LLM Web 検索 API、RAG 検索バックエンド、Brave Search API、Google Vertex AI Search、Grounding with Google Search、Yandex Search API、SerpApi、Serper、DataForSEO、Exa、Tavily、Kagi、Perplexity Search API、検索プロバイダーフォールバックチェーン、検索アグリゲーター、OpenAPI 3.1、Go マイクロサービス、単一 Docker コンテナ。</sub>
