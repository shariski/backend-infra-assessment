# Auth API — ZENTARA Backend Infrastructure Assessment

[![Tests](https://github.com/shariski/backend-infra-assessment/actions/workflows/test.yml/badge.svg)](https://github.com/shariski/backend-infra-assessment/actions/workflows/test.yml)

A JWT authentication REST API for a cybersecurity platform, written in Go. It runs on GKE behind a Cloudflare Tunnel, so the cluster has no public IP, and ships its logs and metrics to Grafana Cloud.

## Live Demo

- **API (staging)**: `https://auth-staging.shariski.com`
- **API (production)**: `https://auth.shariski.com`
- **Health**: [`GET /livez`](https://auth-staging.shariski.com/livez) — process alive
- **Readiness**: [`GET /readyz`](https://auth-staging.shariski.com/readyz) — DB + Redis reachable
- **Interactive docs (Swagger)**: <https://auth-staging.shariski.com/swagger/index.html> (staging only; production hides its API catalog on purpose, covered in [Key design decisions](#key-design-decisions))
- **Monitoring Dashboard**: <https://shariski.grafana.net/public-dashboards/f63a038232084b678d72572f291e37ea>

```bash
curl https://auth-staging.shariski.com/livez
# {"status":"ok"}

curl https://auth-staging.shariski.com/readyz
# {"checks":{"db":"ok","redis":"ok"},"ready":true}
```

---

## Architecture

```
                                  +-------------------+
   Client (HTTPS) --------------> |  Cloudflare Edge  |
                                  |  (anycast, WAF,   |
                                  |   TLS termination)|
                                  +---------+---------+
                                            |
                          mTLS outbound tunnel (HTTP/2)
                                            |
   +----------------------------------------v-----------------------------------+
   |  GKE Standard cluster  (asia-southeast2-a, zonal, 3 x e2-small spot)       |
   |                                                                            |
   |  +-----------------+    +----------------+    +------------------------+   |
   |  | cloudflared     |--->|  auth Service  |--->|  auth Deployment       |   |
   |  | Deployment (x2) |    |  (ClusterIP)   |    |  Gin + Gorm + slog     |   |
   |  +-----------------+    +----------------+    +-----------+------------+   |
   |                                                           |                |
   |                                                           v                |
   |              +---------------------------+    +------------------------+   |
   |              | postgres StatefulSet      |<---+ HPA (CPU 70%, 1-3 pods)|   |
   |              | (PVC, ReadWriteOnce)      |    +------------------------+   |
   |              +---------------------------+                                 |
   |                                                                            |
   |              +---------------------------+                                 |
   |              | redis Deployment          |                                 |
   |              +---------------------------+                                 |
   |                                                                            |
   |  +---------------------------------------------------+                     |
   |  |  promtail DaemonSet  ----- log shipping (Loki) ---|---> Grafana Cloud   |
   |  +---------------------------------------------------+                     |
   +----------------------------------------------------------------------------+
```

### Key design decisions

| Decision | Rationale |
|---|---|
| **Cloudflare Tunnel** instead of a public Ingress/LB | The origin has no public IP. All traffic enters through Cloudflare's edge, which adds WAF, rate-limiting, and DDoS protection and shrinks the attack surface. |
| **GKE Standard** instead of Autopilot | The free zonal management credit covers the control-plane fee, and spot e2-small nodes run about $3.50/mo each. Autopilot's per-pod billing came out significantly more expensive. |
| **Single-zone (zonal) cluster** | Running in `asia-southeast2-a` qualifies for the one free zonal management credit; a regional control plane would not. The trade-off is no cross-zone HA: a zone outage takes the cluster down, which is acceptable for an assessment. |
| **Split `/livez` and `/readyz`** | Liveness is dependency-free and only confirms the process is up; readiness checks DB and Redis. The split keeps a downstream blip from triggering cascading pod restarts. |
| **Grafana Cloud Loki** instead of in-cluster Loki | Keeps log storage off the cluster's tight compute budget, and the free tier covers under 50 GiB/mo. The dashboard is a public URL, so reviewers can open it without a GCP IAM grant. |
| **`Recreate` deploy strategy** | Capacity on e2-small nodes is tight, and a rolling update's surge replica can blow past CPU/memory limits. `Recreate` trades ~15–30s of downtime per rollout for reliable deploys on small hardware. |
| **Image substituted by CI** | The deployment manifest carries a placeholder image (`<REGION>-docker.pkg.dev/<PROJECT_ID>/<REPO>/api:<TAG>`); CI swaps in the real SHA-tagged image at apply time, so the moving reference never lands in version control. |
| **Swagger disabled in production** | `/swagger/*` is only registered when `APP_ENV != "production"` (`internal/router/router.go`). In production the endpoint catalog and schemas mostly help an attacker map the API, and real clients call it by contract anyway. Reviewers explore on staging, and `docs/swagger.json` is committed for offline reading. |
| **LLM on a VPS, not in GKE** | The threat-analysis model (Ollama `llama3.2:1b`) runs on a separate VPS behind a Cloudflare Tunnel. A model on the 2 GB e2-small nodes risks OOM-killing the auth pods, and a GPU node pool would break the budget. CPU inference on the VPS is plenty for an admin-only, cached endpoint, and it's still a self-hosted, open-weights LLM. |

### A note on the GPU-optimization bonus

The Part 4 bonus asks for GPU-optimized inference. This runs CPU-only, by design.

The threat-summary endpoint is Admin-only, read-only, and cached in Redis, so it
serves a handful of calls an hour, not a sustained throughput workload. A 1B model
answers that in a few seconds on CPU. A managed GPU node pool would cost more than
the rest of the stack combined and sit mostly idle, which is hard to justify on a
sub-$25/month budget for latency this endpoint doesn't need.

Moving to GPU later is a config change, not a redesign: add a GPU node pool with
the NVIDIA device plugin, request `nvidia.com/gpu` on the inference pod, swap in a
larger quantized model (say an 8B `Q4_K_M`) sized to VRAM, offload layers with
Ollama's `num_gpu`, keep the model resident via `OLLAMA_KEEP_ALIVE`, and autoscale
on GPU utilization instead of CPU.

The real engineering in this bonus is the pipeline around the model: prompts built
from actual audit and login signals, cached responses, and failure isolation, so a
dead model returns `503` without ever blocking the auth path. The accelerator is
just a knob, set to CPU for this budget.

---

## Tech Stack

Go 1.25 · Gin · Gorm + PostgreSQL · golang-migrate · Viper · log/slog ·
golang-jwt/jwt v5 · bcrypt.

## Implementation status

- **Authentication**: register, login, refresh, logout; RBAC across Admin / Analyst / Viewer; bcrypt hashing; per-IP login rate limiting; and account-level brute-force lockout. An optional admin bootstrap (`BOOTSTRAP_ADMIN_EMAIL` / `BOOTSTRAP_ADMIN_PASSWORD`) seeds or promotes an Admin on startup, so the Admin-only routes work on a fresh deployment.
- **Security analytics**: request/response logging; a durable audit trail (every non-probe request written to `audit_events` with actor, action, status, IP, and request ID); and per-role API rate limiting via a per-user token bucket on all authenticated routes (Admin 120, Analyst 60, Viewer 30 req/min).
- **Caching**: per-user response caching on read-heavy GETs (`/auth/me`, `/admin/users`). Successful 200s are stored in Redis, keyed by route template plus user ID, and replayed with an `X-Cache: HIT/MISS` header; TTL comes from `CACHE_TTL` (default 60s). Anonymous requests skip the cache, and a Redis outage fails open to the handler.
- **AI threat analysis (bonus)**: `GET /admin/users/:id/threat-summary` (Admin-only) turns a user's recent login attempts and audit events into a plain-language risk summary, using a self-hosted Ollama model (`llama3.2:1b`). It's read-only and off the auth path, and the result is cached per user in Redis (`X-Cache: HIT/MISS`, TTL `LLM_SUMMARY_TTL`). The model lives on a separate VPS behind a Cloudflare Tunnel; if it's down, the endpoint returns `503` and nothing else is affected.
- **Infrastructure**: GKE Standard cluster; Cloudflare Tunnel for ingress; Promtail shipping logs to Grafana Cloud Loki; GCP Cloud Monitoring for metrics; and an HPA for autoscaling. The BackendConfig for L7 health checks stays in the manifests even though prod uses the tunnel.
- **CI/CD**: a GitHub Actions workflow builds the image, pushes it to Artifact Registry, then deploys to staging on a push to `develop` and to production on a push to `main`.

---

## Git workflow

The repo follows **GitFlow** with [Conventional Commits](https://www.conventionalcommits.org/):

- **`main`** — production line; every push deploys to the `production` namespace.
- **`develop`** — integration line; every push deploys to `staging`.
- **`feat/*` · `fix/*` · `docs/*` · `refactor/*`** — short-lived branches, one per
  component/change, merged into `develop` via pull request.
- **Conventional commit subjects** (`feat:`, `fix:`, `test:`, `docs:`, `refactor:`)
  — see `git log` for the history.

### Quality gates

| Gate | Tool | When |
|---|---|---|
| Format · imports · vet · lint | goimports, `go vet`, golangci-lint | pre-commit (lefthook), staged `*.go` |
| Unit tests (race) | `go test -short -race ./...` | pre-push (lefthook) |
| Full tests + coverage | `go test -race -covermode=atomic -coverprofile` | CI (`.github/workflows/test.yml`) |
| Lint | golangci-lint | CI (`test.yml`) |

**Coverage** runs on every push: CI prints a per-package `go tool cover -func`
summary to the GitHub Actions **job summary** and uploads an HTML report
(`coverage.html`) as a downloadable artifact. Reproduce locally:

```sh
go test -cover ./...                                  # quick per-package %
go test -coverprofile=coverage.out ./... \
  && go tool cover -html=coverage.out                # browsable HTML report
```

---

## Local development

```bash
cp .env.example .env          # then edit JWT_SECRET
make setup                    # install dev tools + activate Git hooks (once per clone)
make db-up                    # start PostgreSQL via docker-compose
make migrate-up               # apply migrations
make run                      # start the API on :8080
```

### Make targets

```bash
make test          # unit tests
make lint          # golangci-lint
make build         # binary into ./bin
make migrate-up    # apply pending migrations
make migrate-down  # roll back one migration
```

### Git hooks (lefthook)

`make setup` installs [lefthook](https://github.com/evilmartians/lefthook) and
runs `lefthook install`, which wires up:

- **pre-commit** (staged `*.go` files): `goimports`, `go vet`, `golangci-lint`
- **pre-push**: `go test -short -race ./...`

Hooks must be re-activated per clone; committing `lefthook.yml` alone isn't enough.

---

## Deployment

### Prerequisites

- GCP project with billing enabled
- Cloudflare account with a zone (for the public domain)
- `gcloud`, `kubectl`, `gke-gcloud-auth-plugin` installed locally

### One-time GCP setup

```bash
gcloud config set project <PROJECT_ID>

# Artifact Registry for container images
gcloud artifacts repositories create auth \
  --repository-format=docker \
  --location=asia-southeast2

# Workload Identity Federation for GitHub Actions OIDC
# (see .github/workflows/build-and-push.yml for the SA + provider used)
```

### Cluster creation

```bash
gcloud container clusters create auth-cluster \
  --zone=asia-southeast2-a \
  --num-nodes=3 \
  --machine-type=e2-small \
  --spot \
  --disk-size=15 \
  --release-channel=regular \
  --monitoring=SYSTEM \
  --logging=SYSTEM,WORKLOAD
```

### Namespaces and secrets

```bash
gcloud container clusters get-credentials auth-cluster --zone asia-southeast2-a

kubectl apply -f k8s/namespaces.yaml

kubectl -n staging create secret generic auth-secrets \
  --from-literal=DB_USER=postgres \
  --from-literal=DB_PASSWORD="$(openssl rand -base64 24)" \
  --from-literal=REDIS_PASSWORD="$(openssl rand -base64 24)" \
  --from-literal=JWT_SECRET="$(openssl rand -base64 48)" \
  --from-literal=BOOTSTRAP_ADMIN_EMAIL='admin@zentara.demo' \
  --from-literal=BOOTSTRAP_ADMIN_PASSWORD='ZentaraAdmin#2026'

# Grafana Cloud Loki credentials (from your Grafana Cloud stack)
kubectl -n monitoring create secret generic grafana-cloud \
  --from-literal=LOKI_URL='https://logs-prod-XXX.grafana.net/loki/api/v1/push' \
  --from-literal=LOKI_USERNAME='<numeric instance ID>' \
  --from-literal=LOKI_PASSWORD='<glc_ token with logs:write scope>'

# Cloudflare Tunnel token (from Zero Trust > Networks > Tunnels)
kubectl -n staging create secret generic cloudflared-token \
  --from-literal=token='<tunnel token>'
```

> **Admin bootstrap:** `BOOTSTRAP_ADMIN_EMAIL` / `BOOTSTRAP_ADMIN_PASSWORD` seed an
> Admin on startup (created if absent, promoted if present), so the Admin-only routes
> and the [Postman collection](#try-it-with-postman)'s admin flow work out of the box.
> Match these to the Postman environment's `admin_email` / `admin_password`, or omit
> both keys to disable seeding. `admin@zentara.demo` is a staging-only demo credential;
> rotate it for real use. The Secret is injected via `envFrom`, so a
> `kubectl rollout restart deploy/auth` picks up new keys.

### Apply manifests

```bash
# Staging app stack (note: deployment.yaml has an image placeholder — CI substitutes it)
kubectl apply -f k8s/staging/

# Monitoring (Promtail DaemonSet shipping to Grafana Cloud Loki)
kubectl apply -f k8s/monitoring/

# Run database migrations
for f in migrations/*.up.sql; do
  cat "$f" | kubectl -n staging exec -i statefulset/postgres -- psql -U postgres -d auth
done
```

### Configure the Cloudflare Tunnel route

In the Cloudflare dashboard:

1. Zero Trust → Networks → Tunnels → select the `zentara-staging` tunnel
2. Routes / Public Hostname tab → Add a route
3. Hostname: `auth-staging.shariski.com`, Service: `http://auth.staging.svc.cluster.local:80`

Cloudflare auto-creates the DNS CNAME. The domain resolves through Cloudflare's edge into the in-cluster `cloudflared` connectors.

### LLM backend (VPS + Cloudflare Tunnel)

The threat-analysis model runs on a small CPU-only VPS as a self-contained Docker
Compose stack, reached from the cluster over a Cloudflare Tunnel. Ollama has no
published port and no auth of its own, so a small nginx proxy in the same stack
checks a shared secret before forwarding. That's a free stand-in for a Cloudflare
Access service token (Zero Trust Access needs a billing method to turn on), and it
takes no app code change because the API already sends the `CF-Access-Client-Secret`
header.

Request path: `cloudflared → ollama-proxy (nginx, checks secret) → ollama`.
`cloudflared` dials out to Cloudflare, so the VPS opens no inbound ports and its
origin IP stays hidden.

`~/ollama-stack/docker-compose.yml` on the VPS:

```yaml
services:
  ollama:                                   # no host port — only reachable in-stack
    image: ollama/ollama:latest
    restart: unless-stopped
    environment: ["OLLAMA_KEEP_ALIVE=5m"]   # unload the model when idle
    volumes: ["ollama_models:/root/.ollama"]
    networks: [ollama_net]
  ollama-proxy:                             # nginx: 403 unless the secret header matches
    image: nginx:alpine
    restart: unless-stopped
    environment:
      - OLLAMA_PROXY_SECRET=${OLLAMA_PROXY_SECRET}
      - NGINX_ENVSUBST_FILTER=OLLAMA_PROXY
    volumes: ["./nginx.conf.template:/etc/nginx/templates/default.conf.template:ro"]
    depends_on: [ollama]
    networks: [ollama_net]
  cloudflared:
    image: cloudflare/cloudflared:latest
    restart: unless-stopped
    command: tunnel --no-autoupdate run --token ${CF_TUNNEL_TOKEN}
    depends_on: [ollama-proxy]
    networks: [ollama_net]
volumes: {ollama_models: {}}
networks: {ollama_net: {}}
```

`~/ollama-stack/nginx.conf.template` (the auth gate):

```nginx
server {
  listen 80;
  location / {
    if ($http_cf_access_client_secret != "${OLLAMA_PROXY_SECRET}") { return 403; }
    proxy_pass http://ollama:11434;
    proxy_read_timeout 120s;   # CPU generations can be slow
  }
}
```

Deploy:

```bash
# On the VPS, in ~/ollama-stack (with the two files above):
SECRET=$(openssl rand -hex 32)                          # the shared service-token secret
printf 'OLLAMA_PROXY_SECRET=%s\nCF_TUNNEL_TOKEN=%s\n' "$SECRET" "<tunnel-token>" >> .env
docker compose up -d
docker compose exec ollama ollama pull llama3.2:1b

# 4 GB swapfile (OOM cushion for CPU inference; run as root):
fallocate -l 4G /swapfile && chmod 600 /swapfile && mkswap /swapfile && swapon /swapfile
echo '/swapfile none swap sw 0 0' >> /etc/fstab
```

In Cloudflare **Zero Trust → Networks → Tunnels**, create a tunnel, copy its
token into `CF_TUNNEL_TOKEN` above, and add a public hostname
`ollama.shariski.com → http://ollama-proxy:80`. No Access application is needed;
the nginx proxy is the gate.

Finally, put the same `$SECRET` into the cluster Secret as
`CF_ACCESS_CLIENT_SECRET` (with any non-empty `CF_ACCESS_CLIENT_ID`) in each
environment:

```bash
kubectl -n staging patch secret auth-secrets --type merge \
  -p '{"stringData":{"CF_ACCESS_CLIENT_ID":"auth-api","CF_ACCESS_CLIENT_SECRET":"<SECRET>"}}'
# repeat for -n production
```

To run it locally instead (no VPS), use the docker-compose `llm` profile:

```bash
docker compose --profile llm up -d ollama
docker compose --profile llm exec ollama ollama pull llama3.2:1b
# .env already points OLLAMA_URL at http://localhost:11434
```

### Verify

```bash
kubectl -n staging get pods               # all Ready
kubectl -n staging get hpa                # min/max replicas
curl https://auth-staging.shariski.com/readyz
```

---

## CI/CD

On a push to `develop` or `main`, GitHub Actions (`.github/workflows/build-and-deploy.yml`) runs four sequential jobs, so a deploy retry doesn't rebuild and a failed migration never rolls out a new image:

```
test → build → migrate → deploy
```

1. **test** — `go test`, `golangci-lint`
2. **build** — auth to GCP via Workload Identity Federation, build image, push to Artifact Registry, emit the SHA-tagged URI as a job output
3. **migrate** — apply `k8s/<env>/jobs/migrate-job.yaml` (a one-shot K8s `Job` that runs `migrate -path /migrations -database $DSN up`), `kubectl wait` for `condition=complete`, dump logs on failure
4. **deploy** — `sed` the image tag into `k8s/<env>/deployment.yaml`, `kubectl apply -f k8s/<env>/` (non-recursive, so the `jobs/` subfolder isn't re-applied)

`develop` → staging, `main` → production.

### Database migrations

Migrations are golang-migrate SQL files in `migrations/`. They're baked into the API Docker image at build time (along with the `migrate` binary), so the migrate Job runs the same image as the API itself.

To add a new migration:

```sh
# locally
migrate create -ext sql -dir migrations -seq <description>
# edit the new _up.sql / _down.sql files
git commit && git push            # CI runs them automatically before the next deploy
```

Properties of this setup:

- **Always-on, idempotent**: `migrate up` is a no-op when the schema is current, so we run it on every push.
- **Fail-closed**: if the Job fails (`Complete=False` or timeout), the deploy stage is skipped and the existing pods keep serving on the old schema.
- **Concurrency-safe**: golang-migrate uses Postgres advisory locks, and the migrate stage has a `concurrency:` group keyed by branch ref so two rapid pushes serialize cleanly.
- **Audit**: each migration run's logs are dumped to the Actions log, and the Job lingers in-cluster for 10 minutes (`ttlSecondsAfterFinished: 600`) before garbage-collecting itself.

---

## Project layout

```
cmd/api              entrypoint
config               environment-based configuration loader
internal/domain      entities and repository/service interfaces
internal/repository  Gorm implementations
internal/service     business logic (auth, JWT)
internal/handler     Gin HTTP handlers, DTOs, error envelope
internal/middleware  auth, RBAC, recovery, rate limiting, analytics
internal/router      route + middleware wiring
internal/server      http.Server with graceful shutdown
pkg/logger           slog setup
pkg/hash             bcrypt + SHA-256 helpers
pkg/database         Gorm connection
pkg/redis            go-redis client wrapper
pkg/ratelimit        Redis-backed rate limiter
migrations           golang-migrate SQL files
k8s/
  namespaces.yaml      staging, production, monitoring
  staging/             auth, postgres, redis, services, HPA, ingress (unused, kept for reference), cloudflared, BackendConfig
  staging/jobs/        one-shot Jobs (migrate); kept in a subfolder so `kubectl apply -f staging/` doesn't re-apply them
  production/          full mirror of staging (auth, postgres, redis, services, HPA, cloudflared) with prod-tier resource limits
  production/jobs/     production migrate Job
  monitoring/          Promtail RBAC, config, DaemonSet, Grafana Cloud dashboard JSON
```

---

## API Endpoints

Interactive docs are served at **`/swagger/index.html`** in any non-production environment (staging + local). Click **Authorize** in the UI and paste `Bearer <access_token>` to call the protected endpoints.

To regenerate the OpenAPI spec after changing handler annotations:

```sh
make swagger
```

The generated `docs/` package is committed so the binary stays self-contained.

| Method | Path             | Auth            | Description                       |
|--------|------------------|-----------------|-----------------------------------|
| POST   | `/auth/register` | public          | Create a user (Viewer role)       |
| POST   | `/auth/login`    | public          | Issue access + refresh token      |
| POST   | `/auth/refresh`  | refresh token   | Rotate the token pair             |
| POST   | `/auth/logout`   | bearer          | Revoke a refresh token            |
| GET    | `/auth/me`       | bearer          | Current user profile              |
| GET    | `/admin/users`   | bearer + Admin  | Example RBAC-protected route      |
| GET    | `/admin/users/{id}/threat-summary` | bearer + Admin | AI risk summary for a user (LLM) |
| GET    | `/livez`         | public          | Liveness (no dependency checks)   |
| GET    | `/readyz`        | public          | Readiness (checks DB + Redis)     |

### Try it with Postman

A ready-to-run collection lives in [`docs/postman/`](docs/postman):

- `zentara-auth.postman_collection.json`
- `zentara-auth.staging.postman_environment.json`

Import both, select the **ZENTARA Auth — Staging** environment, then run the folders
top-to-bottom (Collection Runner) or click through in order. Tokens are captured
automatically, so there's no manual copy-paste:

1. **Health** — `/livez`, `/readyz`.
2. **Auth lifecycle** — register → login → `/auth/me` (watch `X-Cache` go MISS → HIT)
   → refresh → logout.
3. **RBAC proof** — a Viewer is denied `/admin/users` with `403 FORBIDDEN`.
4. **Admin + AI threat analysis** — admin login → list users → `…/threat-summary`
   (the LLM feature).

Folder 4 needs an Admin account, seeded by the
[admin bootstrap](#namespaces-and-secrets); the environment's `admin_email` /
`admin_password` must match `BOOTSTRAP_ADMIN_*` on the server. The threat-summary
request tolerates a `503` when the self-hosted model is cold or disabled.

---

## Operational notes

- **Cost**: about $10/mo for the cluster (3 × spot e2-small, fixed node count), plus the Grafana Cloud and Cloudflare free tiers, so under $15/mo for the assessment window.
- **Public dashboard**: it updates live and needs no Grafana account, so the URL is safe to share with reviewers.
- **Production**: uses the manifests under `k8s/production/`. The separation from staging is structural (its own namespace, Secret/ConfigMap, and Cloudflare Tunnel) rather than resource-tier; specs and replica counts match staging, since there's no real production traffic to size for. CI/CD promotes to the `production` namespace on a push to `main`. To bring it up, create the `auth-secrets` and `cloudflared-token` Secrets in the `production` namespace (with a separate tunnel for the prod hostname), then `kubectl apply -f k8s/production/`.
- **Known limitation**: the GKE Ingress controller (`gce` class) never engaged on this cluster, even with the HTTP Load Balancing add-on enabled. Cloudflare Tunnel became the ingress path instead, which is a stronger security posture anyway (no public IPs). The Ingress manifests stay in `k8s/staging/ingress.yaml` for reference and would work on a cluster with a functioning GLBC.
