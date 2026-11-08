# SerpentSeek Helm chart

Installs the single-container SerpentSeek gateway: Deployment + Service +
PVC (SQLite) or an external PostgreSQL (no PVC).

## Quick start

```sh
helm install seek ./deploy/helm/serpentseek \
  --set envSecrets.ENCRYPTION_KEY=<64-hex-or-base64-32-bytes>
kubectl port-forward svc/seek-serpentseek 8080:8080
```

The one-time `SETUP_TOKEN` is printed to the pod log on first run
(see `helm status` notes).

## Common values

| Value | Default | Notes |
|---|---|---|
| `persistence.enabled` | `true` | PVC for SQLite; ignored with `externalDatabase.enabled=true` |
| `externalDatabase.enabled` | `false` | sets `STORAGE_DRIVER=postgres` + `DATABASE_URL` from an existing secret |
| `env` | `{}` | plain env vars (see `.env.example` in the repo root) |
| `envSecrets` | `{}` | stored in a generated Secret (`ENCRYPTION_KEY`, provider keys…) |
| `ingress.enabled` | `false` | needed for passkeys (WebAuthn requires HTTPS + domain) |
| `metrics.serviceMonitor.enabled` | `false` | Prometheus Operator scrape of `/metrics` |
| `replicaCount` | `1` | read “Horizontal scaling caveats” in `DEPLOY.md` first |

## External PostgreSQL

```sh
kubectl create secret generic seek-pg \
  --from-literal=url='postgres://serpent:***@pg:5432/serpentseek?sslmode=require'
helm install seek ./deploy/helm/serpentseek \
  --set externalDatabase.enabled=true \
  --set externalDatabase.urlSecretName=seek-pg
```

## Key rotation

`ENCRYPTION_KEY` rotation follows the `serpentseek rotate-key` procedure from
`DEPLOY.md`: scale to 0, run the command in a one-off pod/job against the same
PVC (or database), scale back up with the new key in `envSecrets`.
