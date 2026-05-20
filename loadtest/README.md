# Load testing — HPA autoscaling demo

A [k6](https://k6.io) load test that proves the staging `auth` Deployment
autoscales **1 → 2 → 3 pods** under CPU pressure, satisfying the high-traffic /
auto-scaling requirement (Part 3 of the assessment).

## What it does

`k6/register-spike.js` hammers `POST /auth/register` for ~5 minutes
(1m ramp to 20 VUs, then 4m hold). Registration runs **bcrypt** on every call,
which is CPU-bound, so the pods saturate their `500m` CPU limit and the HPA
(target: 70% of a `50m` request) scales out to its `maxReplicas: 3`.

We hit the API **origin-direct via `kubectl port-forward`**, deliberately
bypassing Cloudflare so the pods receive the full load instead of being
throttled at the edge.

> Why `/auth/register` and not `/auth/login` or an authenticated route?
> `/login` is per-IP rate limited (5/min) and authenticated routes are
> per-user/per-role token buckets — from a single source they return `429`
> before HPA ever sees CPU load. `/auth/register` is the only public endpoint
> with no rate-limit middleware, and it's bcrypt-heavy. The trade-off is that it
> writes real rows; see [Cleanup](#cleanup).

## Prerequisites

- `k6` installed — `brew install k6`
- `kubectl` pointed at the staging cluster:
  ```sh
  gcloud container clusters get-credentials auth-cluster --region asia-southeast2
  kubectl config current-context   # should be the auth-cluster context
  ```

## Run it

Use three terminals.

**Terminal A — port-forward the Service:**
```sh
kubectl -n staging port-forward svc/auth 8080:80
```

**Terminal B — watch autoscaling (this is your evidence):**
```sh
kubectl -n staging get hpa auth -w
# in another pane:
kubectl -n staging get pods -l app=auth -w
```

**Terminal C — run the load test:**
```sh
# from the repo root
k6 run -e RUN_ID=$(date +%s) --summary-export loadtest/summary.json loadtest/k6/register-spike.js
# or:
make loadtest
```

`RUN_ID` tags every generated email so re-runs don't collide on the
`users.email` UNIQUE constraint. Override the target with
`-e BASE_URL=http://localhost:8080` if you forwarded to a different port.

## Capture the evidence

During the 4-minute hold you should see, in Terminal B, `REPLICAS` climb
`1 → 2 → 3` and the HPA `TARGETS` column exceed `70%`. Capture:

1. The `kubectl get hpa -w` / `get pods -w` output showing the replica climb.
2. The k6 end-of-test summary (and `loadtest/summary.json`).
3. A **Grafana** screenshot of CPU utilization + replica count over the run
   window — public dashboard:
   <https://shariski.grafana.net/public-dashboards/f63a038232084b678d72572f291e37ea>

Screenshots dropped in this folder (`*.png` / `screenshots/`) are git-ignored;
reference them from the architecture doc / submission instead.

## Reading the results

- **`REPLICAS 1 → 3`** in the HPA watch = the deliverable is met.
- **Rising `http_req_duration`** in the k6 summary is *expected* — once pods are
  CPU-saturated, requests queue behind bcrypt. The `p(95)<5000` threshold is
  informational, not a system-health gate.
- **`http_req_failed` should stay under 2%.** If it breaches, that's a real
  signal (e.g. DB connection-pool exhaustion under load), not just slowness —
  investigate before trusting the run.

## Cleanup

The test creates one user (and one `audit_events` row) per request. Purge them
after the run.

**Required — delete the test users:**
```sh
kubectl -n staging exec -i statefulset/postgres -- \
  psql -U postgres -d auth -c \
  "DELETE FROM users WHERE email LIKE 'loadtest+%@k6.local';"
# or:
make loadtest-clean
```

**Optional — purge the audit rows from the run window** (safe on staging, which
has no real registrations during the test; `audit_events.actor_id` has no FK to
`users`, so order doesn't matter):
```sh
kubectl -n staging exec -i statefulset/postgres -- \
  psql -U postgres -d auth -c \
  "DELETE FROM audit_events WHERE action = 'POST /auth/register' AND created_at > now() - interval '1 hour';"
```

## Tuning

| Want | Change in `k6/register-spike.js` |
|---|---|
| Scale faster / harder | raise `target` in the `stages` (e.g. 40 VUs) |
| Longer soak | extend the hold `duration` |
| Also see scale-DOWN | add a `{ duration: '6m', target: 0 }` stage and keep watching — HPA scale-down has a 300s stabilization window |
