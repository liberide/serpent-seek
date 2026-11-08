# Developer mode and developer tokens

Developer mode lets an administrator mint a **dedicated token** for a
third-party integration (n8n, CI, a provisioning script, a custom panel…) that
carries **full administrative scope**. With it, the integration can create and
manage everything the dashboard can — providers, chains, chain activation,
users, search API keys, settings, proxies — through the REST API.

---

## 1. Enabling the mode

Developer mode is a normal UI-editable setting (`developer_mode`, environment
variable `DEVELOPER_MODE`, default `false`). **Settings → Developer** contains
only the enable/disable switch; it is admin-only and is not part of the generic
settings form. Enabling it also shows a **Developer** shortcut in the sidebar
(and turning it off hides it again).

The **Developer** section itself — the sidebar shortcut, or `/developer` — is
where keys are issued and revoked and where the built-in Swagger UI is linked.
It shows the current mode as read-only: the switch lives only in Settings.

`DEVELOPER_MODE=true` in the environment pins the setting, exactly like other
environment-managed settings: it is then reported as read-only and the toggle
is hidden. To keep the mode available but off by default, simply omit it or set
`DEVELOPER_MODE=false`.

> Security note: while the mode is enabled, **every** developer token has full
> admin scope. Enable it only while you need it and turn it off when the
> integration no longer requires provisioning.

## 2. Generating a token

Only an admin can mint developer tokens. From the *Developer* page enter a
label and press **Generate token**; the plaintext is shown **once** and never
stored. The same operation over the API:

```bash
curl -s https://seek.example.com/api/developer/tokens \
  -H "Authorization: Bearer $ADMIN_KEY" \
  -H 'Content-Type: application/json' \
  -d '{"name":"n8n integration"}'
```

```json
{
  "token": { "id": "…", "name": "n8n integration", "prefix": "ab12cd", "created_at": "…" },
  "plaintext": "seek_dev_ab12cd.8f3a1c9d7b2e4a5f6c0d1e2b3a4c5d6e7f"
}
```

Tokens use the `seek_dev_` prefix to make them identifiable at a glance; the
stored record is an ordinary API-key row whose scopes include `developer`
(plus `admin,search,read`), so key listing, audit and revocation work the same
way as for regular keys.

## 3. Endpoint permissions

A developer token can be restricted to a subset of the API. Each token carries a
permission list of endpoint ids (`"METHOD /path"`), and the server enforces it on
every request after authentication and routing:

- **Full access** (the default) — the list is empty or `["*"]`; the token may
  call anything, exactly like before.
- **Restricted** — only the listed endpoints are allowed; anything else returns
  HTTP 403 `endpoint_forbidden`. An explicit empty list denies everything.

The *Developer* page offers a grouped picker (by OpenAPI tag, with search) both
when creating a token and in the per-token **Edit** dialog. `GET
/api/developer/endpoints` returns the grantable operations (public endpoints need
no permission and are omitted), and `PATCH /api/developer/tokens/{id}` updates the
name and/or the permission list.

A few endpoints are always allowed so the token stays usable (`GET /api/me`,
`POST /api/auth/logout`). A **restricted token cannot be used for browser
sign-in** (`POST /api/auth/login`): a session carries the owner's full role, so
allowing it would bypass the restrictions. Granting a token the token/key
management endpoints (`POST /api/developer/tokens`, `POST /api/keys`) effectively
lets it escalate to full access — grant them deliberately.

## 4. Using the token

Treat the developer token exactly like an admin key — as a Bearer token against
any `/api` endpoint:

```bash
TOKEN=seek_dev_ab12cd.…

# Provision a provider
curl -s https://seek.example.com/api/providers \
  -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
  -d '{"code":"searxng","name":"my-searxng","enabled":true,"base_url":"https://searx.example.com"}'

# Create a chain that starts at the provider and activate it
curl -s https://seek.example.com/api/chains \
  -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
  -d '{"name":"main","nodes":[{"key":"start","kind":"start","is_start":true},{"key":"n1","provider_id":"my-searxng"}],"edges":[{"from_key":"start","to_key":"n1","condition":"next"}]}'
curl -s -X POST https://seek.example.com/api/chains/<chain-id>/activate \
  -H "Authorization: Bearer $TOKEN"

# Mint a search key for an existing user
curl -s https://seek.example.com/api/keys \
  -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
  -d '{"user_id":"<user-id>","name":"integration","scope":"search"}'
```

The full, machine-readable contract is `api/openapi.yaml`, also served live at
`GET /openapi.yaml` (public, CORS-enabled). The *Developer* section describes
and links to the built-in API reference: an **Open API reference** button opens
`/swagger` — an in-app **Swagger UI** rendered locally from the bundled spec —
with a download button for the spec itself. The UI is bundled
with the frontend (no CDN and no other third-party resource at runtime; the
public validator is disabled).

## 5. API

| Method | Path | Purpose |
| --- | --- | --- |
| `GET` | `/api/developer` | Mode state (`enabled`, `env_locked`) and developer tokens |
| `PUT` | `/api/developer/mode` | Enable/disable the mode (`{"enabled": true\|false}`) |
| `GET` | `/api/developer/endpoints` | Grantable endpoint ids for the permission picker |
| `POST` | `/api/developer/tokens` | Create a token; returns `plaintext` once |
| `PATCH` | `/api/developer/tokens/{id}` | Edit the name and/or endpoint permissions |
| `DELETE` | `/api/developer/tokens/{id}` | Revoke a token (`?purge=true` deletes it permanently) |

All of them require the `admin` scope. `GET /api/developer` and the revoke
endpoint also work while the mode is off, so tokens can be inspected and
cleaned up without re-enabling it. The admin SPA reads `developer_mode` from
`GET /api/me` to decide whether to show the *Developer* sidebar entry.

## 6. Revocation and the kill switch

- **Revoke one token** — `DELETE /api/developer/tokens/{id}` (or the button in
  the UI). Revoked tokens are rejected at authentication time.
- **Delete one permanently** — after revoking, `DELETE
  /api/developer/tokens/{id}?purge=true` removes the row for good (the UI shows
  a *Delete* button on revoked tokens). Audit action `developer.token.delete`.
- **Kill switch** — turning developer mode off makes every developer token
  invalid immediately, without revoking anything. Nothing is deleted: flip the
  mode back on and the same tokens work again (unless they were revoked).
- Regular admin keys (`seek_admin_…`) are **not** affected by the mode; they
  keep working exactly as before.
- Every mode change and token change is written to the audit log
  (`developer.mode`, `developer.token.create`, `developer.token.update`,
  `developer.token.revoke`, `developer.token.delete`).

## 7. Storage & migrations

Developer tokens reuse the `api_keys` table and are recognised by the
`developer` scope, plus the key prefix (`seek_dev_`), which the authenticator
checks when enforcing the mode. Migration `00016_api_key_permissions` adds the
nullable `permissions` column holding the JSON permission list; regular admin
and search keys never carry the developer scope and ignore it.
