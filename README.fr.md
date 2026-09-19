# 🐍 SerpentSeek — passerelle de recherche auto-hébergée pour Open WebUI, MCP et 37+ API

[English](README.md "Read in English") | [Русский](README.ru.md "README по-русски") | [Deutsch](README.de.md "Auf Deutsch lesen") | **Français** | [中文](README.zh.md "阅读中文版")

[![License: GPL-3.0](https://img.shields.io/badge/license-GPL--3.0-blue.svg)](LICENSE "Licence du projet — GNU GPL v3.0")
[![Go 1.26](https://img.shields.io/badge/Go-1.26-00ADD8?logo=go&logoColor=white)](go.mod "Backend écrit en Go")
[![CI](https://github.com/liberide/serpent-seek/actions/workflows/ci.yml/badge.svg)](https://github.com/liberide/serpent-seek/actions/workflows/ci.yml "CI : build, vet, tests")
[![Docker — single container](https://img.shields.io/badge/docker-single_container-2496ED?logo=docker&logoColor=white)](Dockerfile "Tout tient dans un seul conteneur Docker")
[![OpenAPI 3.1](https://img.shields.io/badge/API-OpenAPI_3.1-6BA539)](api/openapi.yaml "Spécification OpenAPI 3.1")
[![37 search providers](https://img.shields.io/badge/built--in_search_providers-37-orange)](#fournisseurs "37 fournisseurs de recherche intégrés")

**SerpentSeek** est une **passerelle de recherche** open source et
auto-hébergée — une API unifiée de recherche web (proxy de métarecherche)
écrite en Go, avec une interface d'administration web multilingue conçue pour
**Open WebUI**. Chaque requête transite par des chaînes configurables de **37
fournisseurs de recherche intégrés** (SearXNG, Brave, Google Vertex AI Search /
Grounding, Yandex, SerpApi, Serper, DataForSEO, Exa, Tavily, Kagi, Perplexity,
etc.), assemblées dans un éditeur visuel de nœuds et d'arêtes. Inclus :
historique des requêtes avec traçage en direct (SSE), clés API et Passkeys
(WebAuthn), un **outil MCP de recherche** (HTTP streamable) pour les assistants
IA (Claude, Cursor, VS Code…), un point d'accès `/search` compatible Open
WebUI, SQLite ⟷ PostgreSQL et des tâches de nettoyage en arrière-plan. Le tout
tient dans un seul conteneur Docker.

Licence du projet — **GNU 3.0** (GPL-3.0-only, voir `LICENSE`). L'attribution se
trouve dans `NOTICE`/`AUTHORS` ; les licences de tous les composants tiers compilés
dans le binaire et dans le bundle web sont rassemblées dans
`THIRD_PARTY_LICENSES.md`. Signalez les vulnérabilités en privé (voir
`SECURITY.md`) ; les règles de contribution sont dans `CONTRIBUTING.md` (sign-off
DCO + [CLA](CLA.md)).

Le logiciel est fourni **« en l'état », sans aucune garantie**. Les requêtes sont
envoyées à des API de recherche tierces avec les clés de la personne qui a déployé
le service ; les éventuels frais relèvent de l'exploitant et de son fournisseur.
Les auteurs ne sauraient être tenus responsables de ces frais ni d'aucun autre
dommage matériel (GPLv3 §§15–16, détails dans `NOTICE`).

## Fonctionnalités clés

* 🔎 **Un point d'accès pour 37 fournisseurs** — web/SERP (SearXNG, Brave,
  Google, Yandex, SerpApi…), IA/neuronale (Exa, Tavily, Perplexity, Kagi…),
  académique (OpenAlex, PubMed, Crossref, Semantic Scholar…) et entreprise
  (Azure AI Search, Vertex AI Search, Vectara…)
* 🧩 **Éditeur visuel de chaînes** — chaînes de fallback/réessai sous forme de
  graphe nœuds/arêtes ; plusieurs instances nommées par pilote
* 🔌 **Prêt pour Open WebUI** — point d'accès `/search` clé en main : toujours
  HTTP 200, `[{"link","title","snippet"}]`
* 🤖 **Serveur MCP** — outil `search` en HTTP streamable pour Claude, Cursor,
  VS Code et tout client MCP ; JSON-RPC 2.0 simple accepté également
* 🔭 **Traçage en direct** — frise chronologique des étapes via SSE, en-têtes
  de diagnostic `X-Serpent-Rid` / `X-Serpent-Api` et page d'historique
* 🔐 **Sécurité** — clés API hachées en argon2id, Passkeys (WebAuthn), sessions
  SameSite=Strict avec CSRF, secrets en écriture seule, AES-GCM optionnel
* 💾 **SQLite ⟷ PostgreSQL** — SQLite en mode WAL par défaut ; le profil
  `extdb` fournit PostgreSQL pour les déploiements multi-réplicas
* 🌍 **UI multilingue** — English, Русский, Polski, Deutsch, Français, 中文
* 🐳 **Un seul conteneur Docker** — backend Go avec la SPA SvelteKit embarquée

## Sommaire

* [Démarrage rapide](#démarrage-rapide)
  * [Intégration Open WebUI](#intégration-open-webui)
  * [Intégration MCP (Model Context Protocol)](#intégration-mcp-model-context-protocol)
* [Fournisseurs](#fournisseurs)
  * [SearXNG : instance externe uniquement](#searxng--instance-externe-uniquement)
* [PostgreSQL (profil `extdb`)](#postgresql-profil-extdb)
* [Réglages](#réglages)
* [Interface et localisation](#interface-et-localisation)
* [Authentification](#authentification)
* [Tâches d'arrière-plan](#tâches-darrière-plan)
* [Développement](#développement)
* [Dépendances et licences](#dépendances-et-licences)
* [Risques et mises en garde](#risques-et-mises-en-garde)
* [Captures d'écran](#captures-décran)
* [Structure](#structure)

## Démarrage rapide

```bash
cp .env.example .env
# ouvrez .env et renseignez les clés des fournisseurs si nécessaire (SEARXNG_URL, etc.)
docker compose up --build
```

Interface : <http://localhost:8080>. Contrôle de santé : `curl http://localhost:8080/healthz`.

Pour utiliser l'image préconstruite de Docker Hub au lieu de compiler depuis
les sources — voir [DEPLOY.md](DEPLOY.md).

Au premier démarrage, un jeton à usage unique est écrit dans les logs :

```text
SETUP_TOKEN: ff1089a6
```

Ouvrez `/setup`, saisissez le jeton, créez un administrateur. La **clé API n'est
affichée qu'une seule fois** — conservez-la. Connectez-vous ensuite sur `/login`
avec cette clé.

### Intégration Open WebUI

Dans Open WebUI → *Settings → Web Search*, choisissez **External** (ou SearXNG) et
renseignez :

* URL : `http://<host>:8080/search`
* Clé API : votre clé `seek_ak_...`

Le point d'entrée `POST /search` est compatible Open WebUI : corps
`{"query":"...","count":5}`, la réponse est toujours HTTP 200 avec un tableau
`[{"link","title","snippet"}]` (`[]` en cas d'échec ou de résultat vide). Les
diagnostics passent par les en-têtes `X-Serpent-Rid` et `X-Serpent-Api` et la page
History.

```bash
curl -s http://localhost:8080/search \
  -H "Authorization: Bearer seek_ak_..." \
  -H "Content-Type: application/json" \
  -d '{"query":"hello world","count":3}'
```

### Intégration MCP (Model Context Protocol)

SerpentSeek expose un outil `search` via MCP sur `/mcp` (transport officiel
**streamable HTTP** : GET pour le flux SSE, POST pour JSON-RPC, DELETE pour fermer
la session). Le même point d'entrée fonctionne aussi comme une simple URL
JSON-RPC 2.0 en HTTP requête/réponse pour les clients qui ne prennent pas en charge
le transport streamable complet. Passez votre clé API en Bearer.

Pour les **clients MCP HTTP natifs** (Claude Desktop, certaines versions de Cursor,
les outils MCP d'Open WebUI, etc.) :

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

Pour les **clients MCP uniquement stdio** (Cursor plus ancien, extensions VS Code,
etc.), utilisez l'enveloppe `mcp-remote` pour que le point d'entrée HTTP distant
ressemble à un serveur stdio local :

```bash
npx mcp-remote http://<host>:8080/mcp --header "Authorization=Bearer seek_ak_..."
```

Pointez ensuite le client vers la commande enveloppe plutôt que vers une URL.

Le seul outil est `search`, qui prend `query` (obligatoire) et `count` (optionnel,
0 = tout, défaut 5). Chaque appel exécute la chaîne de fournisseurs active et
renvoie `title` / `url` / `snippet` par résultat. Le point d'entrée est décrit
(avec des extraits de configuration prêts à copier et un test en direct) sur la
page **MCP** de l'interface d'administration.

## Fournisseurs

Liste complète des pilotes de recherche intégrés. Les identifiants sont en
écriture seule et se configurent par instance sur la page **Providers** (seul le
seed `google_vertex` lit `GOOGLE_*` depuis l'environnement).

### Recherche web / SERP

| Code | Rôle | Identifiants | Remarques |
| --- | --- | --- | --- |
| `apiserpent` | apiserpent.com quick search | `api_key` | rotation de moteurs google/bing/yahoo/ddg/brave |
| `serpbase` | serpbase.dev | `api_key` | tables fatal/retry configurables |
| `searxng` | SearXNG — **instance externe** | `api_key` (optionnel, jeton de passerelle) | URL dans l'UI/`SEARXNG_URL` ; nécessite `format=json` et `limiter: false` |
| `yandex` | Yandex Search API v2 (Yandex Cloud) | `api_key`, `folder_id` | `/v2/web/search`, XML base64 dans `rawData` |
| `google_vertex` | Gemini Grounding with Google Search / Vertex AI Search | `api_key`, `project_id`, `engine_id` | voie Google principale ; env `GOOGLE_API_KEY`, `GOOGLE_MODEL`, `GOOGLE_DRIVER` |
| `google` | Google (répartiteur) | `api_key`, `cx`, `project_id`, `engine_id` | le paramètre `driver` bascule vertex/cse |
| `google_cse` | Google Custom Search JSON API | `api_key`, `cx` | **deprecated**, arrêt le 01.01.2027 |
| `brave` | Brave Search | `api_key` | |
| `serper` | Serper | `api_key` | |
| `serpapi` | SerpApi | `api_key` | multi-moteur (`engine`) |
| `searchapi` | SearchApi.io | `api_key` | |
| `dataforseo` | DataForSEO | `login`, `password` | HTTP Basic auth |
| `youcom` | You.com | `api_key` | |
| `mojeek` | Mojeek | `api_key` | clé dans la chaîne de requête |
| `marginalia` | Marginalia | `api_key` | clé publique limitée en débit (503 réessayable) |
| `hn` | Hacker News (Algolia) | — | aucune clé requise |

### Recherche IA / neuronale

| Code | Rôle | Identifiants | Remarques |
| --- | --- | --- | --- |
| `exa` | Exa | `api_key` | highlights par défaut |
| `tavily` | Tavily | `api_key` | la recherche avancée coûte 2 crédits |
| `jina` | Jina AI Search | `api_key` | requête dans le chemin de l'URL |
| `firecrawl` | Firecrawl | `api_key` | |
| `linkup` | Linkup | `api_key` | sortie `searchResults` |
| `perplexity_search` | Perplexity Search API | `api_key` | `/search` autonome, pas Sonar |
| `valyu` | Valyu | `api_key` | recherche propriétaire sur abonnement |
| `parallel` | Parallel | `api_key` | `search_queries` construit depuis la requête |
| `kagi` | Kagi | `api_key` | points d'entrée FastGPT/enrich |

### Recherche académique et développement

| Code | Rôle | Identifiants | Remarques |
| --- | --- | --- | --- |
| `openalex` | OpenAlex | — | résumé reconstruit depuis l'index inversé |
| `semanticscholar` | Semantic Scholar | `api_key` (optionnel) | ~1 rps sans clé |
| `crossref` | Crossref | — | liens DOI normalisés vers `https://doi.org/` |
| `github` | GitHub Search | `api_key` | dépôts/code/issues/commits/utilisateurs |
| `stackexchange` | Stack Exchange | `api_key` (optionnel) | respecte le `backoff` de l'API |
| `pubmed` | PubMed (NCBI) | `api_key` (optionnel), env `NCBI_API_KEY` | flux en deux étapes esearch→esummary (+ efetch) |

### Recherche d'entreprise

| Code | Rôle | Identifiants | Remarques |
| --- | --- | --- | --- |
| `azure_search` | Azure AI Search | `api_key` ou Entra (`tenant_id`, `client_id`, `client_secret`) | `title_field`/`url_field`/`snippet_field` configurables |
| `vertex_search` | Vertex AI Search (Discovery Engine) | `service_account_json` | OAuth2 uniquement, pas de clés API |
| `vectara` | Vectara | `api_key` | génération désactivée (sortie Row) |
| `kendra` | Amazon Kendra | `access_key_id`, `secret_access_key` | **deprecated** — SigV4 AWS non implémenté |

### Nœuds de réponse générative

Ces pilotes s'utilisent sur un nœud **`mode: answer`** : le texte généré alimente
le panneau de réponse de la requête, et les sources deviennent des lignes
sparse (lien+titre).

| Code | Rôle | Identifiants | Remarques |
| --- | --- | --- | --- |
| `anthropic` | Anthropic web search | `api_key` | outil `web_search` + citations |
| `yandex_gen` | Yandex AI Studio (réponse générative) | `api_key`, `folder_id` | `/v2/gen/search` |

La `base_url` de chaque fournisseur est modifiable dans l'UI (ce n'est pas un
secret). Les secrets sont en écriture seule : la valeur n'est jamais renvoyée,
seulement un marqueur « défini ». Le bouton « Test » lance une requête sonde
`q=test` et affiche `http/kind/ms` ainsi que les premiers liens.

**Les fournisseurs sont des instances.** Sur la page Providers, vous pouvez ajouter
autant de fournisseurs que vous voulez, y compris **plusieurs du même type
(pilote) avec des noms et des `api_key` différents** — par exemple « apiserpent
work » et « apiserpent personal ». Chaque instance s'active/se désactive par un
interrupteur ; les instances désactivées n'apparaissent pas dans la palette de
l'éditeur de chaînes. La suppression d'une instance référencée par une chaîne est
refusée avec `409` et la liste des chaînes. Lors d'une mise à niveau depuis une
ancienne version de la base, les anciens enregistrements deviennent
automatiquement des instances (`id = code`) et les références des chaînes sont
conservées.

**Au premier démarrage**, seul `google_vertex` (Vertex/Grounding) est créé et
activé, et la chaîne par défaut se compose d'un unique bloc `google_vertex`. Aucune
autre instance n'est créée — ajoutez les pilotes dont vous avez besoin sur la page
Providers.

### SearXNG : instance externe uniquement

SerpentSeek **n'exécute pas SearXNG lui-même** — il n'y a pas un tel service dans
le compose. Pointez-le vers l'URL de votre SearXNG déjà en cours d'exécution :

* dans le champ `base_url` du fournisseur `searxng` (page Providers, sans
  redémarrage), ou
* via `SEARXNG_URL` dans `.env`.

Une URL simple (`http://host:port/search`) et un modèle avec `{query}`/`<query>`
sont pris en charge. Tant que l'URL n'est pas définie, le fournisseur est
considéré comme mort (`skip`) et la chaîne continue sans lui.

Exigences pour votre instance : `search.formats: [html, json]` et `limiter: false`
(sinon vous obtiendrez 403/429). Configuration d'exemple —
`deploy/searxng/settings.yml` ; le propriétaire de l'instance est celui qui
l'exploite.

> **⚠️ SearXNG est sous licence AGPLv3.** C'est un service externe ; SerpentSeek
> ne communique avec lui que par HTTP et ne lie rien. La responsabilité du respect
> de la licence SearXNG incombe à la personne qui le déploie.

## PostgreSQL (profil `extdb`)

```bash
# dans .env : POSTGRES_PASSWORD=... STORAGE_DRIVER=postgres \
#             DATABASE_URL=postgres://serpent:...@postgres:5432/serpentseek?sslmode=disable
docker compose --profile extdb up -d --build
```

PostgreSQL est un conteneur séparé (PostgreSQL License), **pas** une dépendance de
code de SerpentSeek. SQLite est le défaut : `journal_mode=WAL`, `foreign_keys=ON`,
`busy_timeout=5000`, `synchronous=NORMAL`, un seul écrivain. Les déploiements
multi-répliques nécessitent PostgreSQL.

## Réglages

Toutes les variables sont listées dans [`.env.example`](.env.example). Les valeurs
d'environnement sont prioritaires ; ces champs sont affichés dans l'UI avec un
badge `env` et sont en lecture seule. Tous les autres réglages sont modifiables sur
la page Settings sans redémarrage (sauf `STORAGE_DRIVER`, ce dont l'UI avertit
honnêtement).

Les clés de fournisseurs (`api_key`, `folder_id`, `cx`, …) et `base_url` sont des
**propriétés d'instance** : elles se configurent sur la page Providers. Les seules
variables d'environnement qui alimentent un fournisseur au premier démarrage sont
`GOOGLE_API_KEY`, `GOOGLE_MODEL` et `GOOGLE_DRIVER` (pour l'instance
`google_vertex` par défaut) ; tous les autres identifiants sont par instance et
n'ont pas de valeurs par défaut d'environnement. Sur la page Settings, les
fournisseurs ne gardent que les **codes de retry** (`RETRY_HTTP_CODES`,
`AP_RETRY_CODES`) — des valeurs globales que chaque instance peut surcharger via
ses propres `params`. Les tables de codes fatals (`fatal_codes`, `fatal_http`)
sont des paramètres d'instance et s'éditent sur la page Providers.

Variables clés : `PORT`, `HOST`, `DATA_DIR`/`SQLITE_PATH`, `STORAGE_DRIVER`,
`DATABASE_URL`, `AUTH_ENABLED`, `PUBLIC_ORIGIN`, `RP_ID`, `RP_NAME`,
`HISTORY_RETENTION_DAYS`, `LOGS_RETENTION_DAYS`, `MAX_CONCURRENCY`, `MAX_ATTEMPTS`,
`USER_AGENT`, `LOG_LEVEL`, `LOG_FORMAT`, `TREAT_EMPTY_AS_FAIL`.

`IGNORE_REQUEST_COUNT=true` fait que la passerelle ignore le `count` de la requête
et transmet `num=0` aux fournisseurs — renvoyer tout ce qui est trouvé, sans
limite. La même valeur (`0`) peut être saisie manuellement dans le champ
« Results » de la page **Playground** (une info-bulle apparaît au survol du champ).

## Interface et localisation

L'UI est traduite en 6 langues :

| Code | Langue |
| --- | --- |
| `ru` | Русский |
| `en` | English |
| `pl` | Polski |
| `de` | Deutsch |
| `fr` | Français |
| `zh` | 中文 |

Le sélecteur de langue est en haut à droite, à côté du bouton « Quick search ».
La langue choisie est conservée dans `localStorage` ; à la première ouverture elle
est détectée via `navigator.language`, sinon l'anglais est utilisé. Les dates sont
formatées selon les règles de la locale choisie (`Intl.DateTimeFormat`).

Pour ajouter une langue : déposez un dictionnaire dans
`web/src/lib/i18n/<code>.json` (le jeu de clés doit correspondre à `en.json`),
ajoutez une entrée dans `web/src/lib/i18n/languages.json` et le code au type
`Locale` dans `web/src/lib/i18n.svelte.ts`.

## Authentification

* **Clés :** `seek_ak_<prefix>_<secret>` (scope search) et
  `seek_admin_<prefix>_<secret>` (scope admin). Seul un hash argon2id est stocké ;
  seul le préfixe est visible dans l'UI/API.
* **Sessions :** cookie httpOnly + Secure + SameSite=Strict, glissant 30 jours,
  double-soumission CSRF sur toutes les mutations.
* **Passkeys (WebAuthn) :** à enregistrer après connexion avec une clé, puis
  connexion sans mot de passe. Nécessite HTTPS + un domaine
  (`PUBLIC_ORIGIN`/`RP_ID`) ; en HTTP/IP simple le bouton est masqué avec un
  avertissement.
* **`AUTH_ENABLED=false`** — toute l'API d'administration fonctionne sans
  authentification (une bannière est affichée dans l'UI).

## Tâches d'arrière-plan

* `cleanup` (`HISTORY_CLEANUP_CRON`, défaut `0 4 * * *`) — supprime l'historique
  et les logs plus anciens que la période de rétention, les sessions expirées, et
  recalcule les agrégats quotidiens.
* `vacuum` (`VACUUM_CRON`, défaut `0 3 * * 0`) — SQLite uniquement.

Les deux peuvent être déclenchés manuellement depuis Settings → Maintenance (ou via
`POST /api/maintenance/*`). Une tâche qui panique ne fait pas planter le processus.

## Développement

```bash
make build      # SPA + binaire Go (embed)
make test       # go test ./... -cover
make check      # svelte-check
make lint       # go vet + golangci-lint
make dev        # serveur de dev Vite proxyfié vers :8080
```

Le backend écoute sur `:8080` ; la SPA est embarquée via `go:embed web/build`.

## Dépendances et licences

Seules MIT / Apache-2.0 / BSD-2/3 / ISC sont autorisées. Audit :

```bash
go run github.com/google/go-licenses@latest report ./...
cd web && npx license-checker --production --excludePrivatePackages
```

| Dépendance | Version | Licence |
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

Conteneurs satellites (pas des dépendances de code) : `postgres:17-alpine`
(PostgreSQL License) et `searxng/searxng` externe (hors de ce dépôt ; **AGPLv3** —
un service séparé, pas une dépendance de code).

## Risques et mises en garde

* **Les Passkeys nécessitent HTTPS + un domaine** (WebAuthn rpID). Sur une simple
  IP/HTTP, seules les clés sont disponibles ; l'UI le signale.
* **Google Custom Search JSON API** est fermé aux nouveaux clients et s'arrête le
  **01.01.2027** — le pilote est marqué deprecated, le défaut est Vertex/Grounding.
* **Les fournisseurs sont des services payants.** Toutes les requêtes utilisent
  les **clés API de la personne qui a déployé le service** et sont facturées par
  le fournisseur (Yandex Cloud, Google Cloud, apiserpent, SerpBase). Configurez
  budgets, alertes et quotas de clés **côté fournisseur** et gardez les retries
  sous contrôle (tentatives max, politique de retry dans la chaîne) : les auteurs
  du projet ne sont pas responsables des frais (voir `NOTICE`).
* **Yandex Search API v2** est payant : une Api-Key + folderId (Yandex Cloud) sont
  nécessaires.
* **SearXNG** est un service externe : `search.formats: [html, json]` et
  `limiter: false` sont requis ; le jeton de passerelle (`X-API-Key`) n'est **pas**
  `server.secret_key`. SearXNG est sous licence **AGPLv3** et ne fait pas partie de
  ce dépôt.
* **SQLite** est mono-écrivain ; WAL + busy_timeout suffisent pour une réplique.
* **Les identifiants des fournisseurs** sont en écriture seule et masqués dans
  l'API et les logs ; `base_url` n'est pas un secret. `ENCRYPTION_KEY` (AES-GCM)
  optionnel pour le chiffrement au repos.

## Captures d'écran

Après le démarrage, vous obtenez les écrans suivants : Dashboard, History (avec un
graphe de chaîne coloré et une chronologie des étapes), Chains (éditeur visuel),
Providers, Playground, MCP, Logs, Users, Settings. Ajoutez vos captures d'écran
dans `docs/screenshots/` (elles ne sont pas incluses dans le dépôt pour ne pas
alourdir l'image).

## Structure

```
cmd/serpentseek/      # câblage et seed
internal/
  config logging store providers engine sse auth httpd jobs version
api/openapi.yaml      # OpenAPI 3.1
web/                  # SPA SvelteKit (le build est embarqué dans le binaire)
deploy/
  searxng/settings.yml
  caddy.example
```

---

<sub>**Mots-clés :** passerelle de recherche, auto-hébergé, API de recherche unifiée, métarecherche, proxy de métarecherche, recherche web Open WebUI, backend de recherche pour Open WebUI, SearXNG, serveur MCP, outil de recherche MCP, Model Context Protocol, recherche web pour LLM et assistants IA, recherche RAG, Brave Search API, Google Vertex AI Search, Grounding with Google Search, Yandex Search API, SerpApi, Serper, DataForSEO, Exa, Tavily, Kagi, Perplexity Search API, chaînes de fournisseurs avec fallback, agrégateur de recherche, OpenAPI 3.1, microservice Go, conteneur Docker unique.</sub>
