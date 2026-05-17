# Auth API — ZENTARA Backend Infrastructure Assessment

A JWT-authenticated REST API with end-to-end encrypted public access, deployed on GKE with Cloudflare Tunnel for zero-public-IP origin and centralized observability.

## Live Demo

- **API**: `https://auth-staging.shariski.com`
- **Health**: [`GET /livez`](https://auth-staging.shariski.com/livez) — process alive
- **Readiness**: [`GET /readyz`](https://auth-staging.shariski.com/readyz) — DB + Redis reachable
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
   |  GKE Standard cluster  (asia-southeast2, regional, 3 x e2-small spot)      |
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
| **Cloudflare Tunnel** instead of public Ingress/LB | Origin has zero public IP. Reduces attack surface; all traffic enters through Cloudflare's edge with WAF, rate-limit, and DDoS protection. Aligns with security-platform positioning. |
| **GKE Standard** (vs. Autopilot) | Standard cluster's free zonal credit covers the management fee; spot e2-small nodes are ~$3.50/mo each. Autopilot's per-pod billing was significantly more expensive. |
| **Regional cluster (3 zones)** | HA across 3 zones; ~$10/mo extra over zonal but provides smoother HPA scaling and a stronger HA narrative. |
| **`/livez` + `/readyz` split** | Liveness is dep-free (just confirms process is up). Readiness checks DB + Redis. Prevents cascading restarts when a downstream blip would otherwise mark the app unhealthy. |
| **Promtail + Grafana Cloud Loki** (not in-cluster Loki) | No cluster compute for log storage. Free tier covers <50 GiB/mo. Public dashboard URL shareable with reviewers without granting GCP IAM. |
| **`Recreate` Deployment strategy** | Single-node-class capacity is tight; rolling updates with surge can hit CPU/memory limits. `Recreate` accepts ~15-30s of downtime per rollout in exchange for reliability on constrained hardware. |
| **Image substitution via CI `sed`** | Deployment manifest stores `<REGION>-docker.pkg.dev/<PROJECT_ID>/<REPO>/api:<TAG>` as a placeholder; CI substitutes with the git SHA tag at apply time. Avoids committing the moving image reference into version control. |

---

## Tech Stack

Go 1.25 · Gin · Gorm + PostgreSQL · golang-migrate · Viper · log/slog ·
golang-jwt/jwt v5 · bcrypt.

## Implementation status

- **Authentication**: register, login, refresh, logout, RBAC (Admin / Analyst / Viewer), bcrypt password hashing, per-IP login rate limiting, account-level brute-force protection.
- **Security Analytics**: request/response logging implemented; audit trails and per-role API rate limiting are scaffolded (see `// TODO` markers in `internal/middleware/{audit,ratelimit}.go`).
- **Infrastructure**: GKE Standard cluster, Cloudflare Tunnel for ingress, Promtail → Grafana Cloud Loki for logs, GCP Cloud Monitoring for metrics, HPA for auto-scaling, BackendConfig for L7 health (retained in manifests though tunnel is used in prod).
- **CI/CD**: GitHub Actions workflow builds Docker image, pushes to Artifact Registry, deploys to staging on push to `develop`, to production on push to `main`.

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

Hooks must be re-activated per clone — committing `lefthook.yml` alone is not enough.

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
  --region=asia-southeast2 \
  --num-nodes=1 \
  --machine-type=e2-small \
  --spot \
  --disk-size=15 \
  --release-channel=regular \
  --monitoring=SYSTEM \
  --logging=SYSTEM,WORKLOAD
```

### Namespaces and secrets

```bash
gcloud container clusters get-credentials auth-cluster --region asia-southeast2

kubectl apply -f k8s/namespaces.yaml

kubectl -n staging create secret generic auth-secrets \
  --from-literal=DB_USER=postgres \
  --from-literal=DB_PASSWORD="$(openssl rand -base64 24)" \
  --from-literal=REDIS_PASSWORD="$(openssl rand -base64 24)" \
  --from-literal=JWT_SECRET="$(openssl rand -base64 48)"

# Grafana Cloud Loki credentials (from your Grafana Cloud stack)
kubectl -n monitoring create secret generic grafana-cloud \
  --from-literal=LOKI_URL='https://logs-prod-XXX.grafana.net/loki/api/v1/push' \
  --from-literal=LOKI_USERNAME='<numeric instance ID>' \
  --from-literal=LOKI_PASSWORD='<glc_ token with logs:write scope>'

# Cloudflare Tunnel token (from Zero Trust > Networks > Tunnels)
kubectl -n staging create secret generic cloudflared-token \
  --from-literal=token='<tunnel token>'
```

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

### Verify

```bash
kubectl -n staging get pods               # all Ready
kubectl -n staging get hpa                # min/max replicas
curl https://auth-staging.shariski.com/readyz
```

---

## CI/CD

GitHub Actions (`.github/workflows/build-and-deploy.yml`) on push to `develop` or `main` runs four sequential jobs — a deploy retry doesn't rebuild, and a failed migration doesn't roll out a new image:

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
| GET    | `/livez`         | public          | Liveness (no dependency checks)   |
| GET    | `/readyz`        | public          | Readiness (checks DB + Redis)     |

---

## Operational notes

- **Cost**: cluster ~$3.50-11/mo (spot e2-small × 1-3 nodes) + Grafana Cloud free tier + Cloudflare free tier ≈ **<$15/mo for the assessment window**.
- **Public dashboard** updates live; share the URL with reviewers — no Grafana account needed.
- **Production environment** uses the manifests under `k8s/production/` — same resource specs and replica counts as staging since this is an assessment with no real production workload. The separation is structural (separate namespace, separate Secret/ConfigMap, separate Cloudflare Tunnel) rather than resource-tier — in a real production deployment those values would be tuned upward, but spending more here would inflate cost without serving the assessment goal. CI/CD promotes builds to the `production` namespace on push to `main`. To activate: create `auth-secrets` and `cloudflared-token` Secrets in the `production` namespace (with a separate Cloudflare Tunnel for the prod hostname), then `kubectl apply -f k8s/production/`.
- **Known limitation**: GKE Ingress controller (`gce` class) did not engage on this specific cluster despite the HTTP Load Balancing addon being enabled. Cloudflare Tunnel was chosen as the production ingress path, which gives a stronger security posture anyway (no public IPs). Ingress manifests are preserved in `k8s/staging/ingress.yaml` for reference and would work on a cluster with functioning GLBC.
