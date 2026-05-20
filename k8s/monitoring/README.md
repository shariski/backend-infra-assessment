# Observability — Grafana Cloud + Promtail

Ships pod logs from `staging` and `production` namespaces to Grafana Cloud Loki, where a public dashboard can be shared with reviewers.

## Setup

### 1. Sign up for Grafana Cloud (free tier)

- https://grafana.com/auth/sign-up/create-user
- No credit card required
- Free tier: 50 GB logs/month, 10K metrics series, 14-day retention

### 2. Get Loki push credentials

In your Grafana Cloud stack:
- Left nav: **Connections → Add new connection → Hosted logs**
- Or directly: `https://<your-stack>.grafana.net/connections/datasources/loki`
- Copy:
  - **URL** (e.g., `https://logs-prod-020.grafana.net/loki/api/v1/push`)
  - **User** (numeric instance ID, e.g., `1234567`)
  - **API token** — create one with `logs:write` scope

### 3. Apply manifests

```bash
# Namespaces (idempotent)
kubectl apply -f k8s/namespaces.yaml

# Create the secret from the example template (replace placeholders first!)
# DO NOT commit the populated file.
cp k8s/monitoring/grafana-cloud-secret.example.yaml /tmp/grafana-cloud-secret.yaml
# ... edit /tmp/grafana-cloud-secret.yaml with real values ...
kubectl apply -f /tmp/grafana-cloud-secret.yaml
rm /tmp/grafana-cloud-secret.yaml

# Promtail
kubectl apply -f k8s/monitoring/promtail-rbac.yaml
kubectl apply -f k8s/monitoring/promtail-config.yaml
kubectl apply -f k8s/monitoring/promtail-daemonset.yaml
```

### 4. Verify logs are flowing

```bash
kubectl -n monitoring logs ds/promtail | grep "msg=" | head -20
```

Then in Grafana Cloud → **Explore** → select Loki → run:

```
{cluster="auth-cluster", container="auth"} | json
```

You should see your `slog` JSON output, with `level` available as a filterable label and `namespace` (`staging` / `production`) distinguishing the two environments.

### 5. Build the public dashboard

Recommended panels for the assessment rubric:

| Panel | Query | Why |
|---|---|---|
| Recent logs | `{container="auth"} \| json` | Catch-all log viewer (shows `namespace`) |
| Error rate | `sum by (namespace) (rate({container="auth", level="ERROR"}[5m]))` | Per-env health |
| Login activity | `{container="auth"} \|= "login"` | Demonstrates filterability |
| Request volume | `sum by (namespace) (count_over_time({container="auth"}[1m]))` | RED method's "Rate", split by env |

The committed [`dashboard.json`](dashboard.json) encodes these panels split `by (namespace)` so one board shows **staging and production** side by side. It is in Grafana's v13 dashboard schema (`elements` / `layout`). To update the live board: open it → **Edit → Settings → JSON Model**, paste, and **Save** — this updates the dashboard in place, preserving its identity and public URL. (This schema does not load through the legacy *Dashboards → Import* flow; use JSON Model.)

Then **Share dashboard → Public dashboards → Enable**. Copy the public URL and put it in the main `README.md` so the reviewer doesn't need a Grafana Cloud account. (Grafana public dashboards don't expose interactive template variables, which is why both environments are shown as fixed `by (namespace)` series rather than via an env dropdown.)

## Why this setup

- **No cluster compute for Grafana/Loki** — both hosted on Grafana Cloud, free tier
- **Promtail is ~64 Mi memory per node**, runs as DaemonSet so it scales with the cluster
- **Namespace globs** (`staging_*` + `production_*`) in `promtail-config.yaml` ship both app environments while excluding kube-system/system noise to stay within free tier
- **JSON parsing pipeline** promotes `slog`'s `level` field to a Loki label, enabling fast filtering in Grafana
