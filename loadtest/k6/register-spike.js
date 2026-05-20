// register-spike.js — drives the staging `auth` HPA to scale 1 -> 2 -> 3.
//
// Why POST /auth/register: it is the only public endpoint with no rate-limit
// middleware (internal/router/router.go) AND it runs bcrypt on every call.
// bcrypt is deliberately CPU-bound, so a modest number of VUs self-throttle on
// CPU and peg the pods' 500m limit, pushing average CPU past the HPA's target
// (70% of a 50m request ~= 35m) and triggering scale-up.
//
// Traffic path is origin-direct via `kubectl port-forward` (NOT the public
// Cloudflare URL), so the pods see the full load instead of being throttled at
// the edge. See loadtest/README.md for the full runbook.
//
// Usage:
//   k6 run -e RUN_ID=$(date +%s) loadtest/k6/register-spike.js
//   k6 run -e BASE_URL=http://localhost:8080 -e RUN_ID=demo register-spike.js

import http from 'k6/http';
import { check } from 'k6';

const BASE_URL = __ENV.BASE_URL || 'http://localhost:8080';
// RUN_ID tags every email so re-runs don't collide on the users.email UNIQUE
// constraint, and so cleanup is a single `DELETE ... WHERE email LIKE
// 'loadtest+%@k6.local'`. Defaults to a timestamp when not provided.
const RUN_ID = __ENV.RUN_ID || String(Date.now());
const PASSWORD = __ENV.PASSWORD || 'LoadTest!23456'; // satisfies binding min=8

export const options = {
  scenarios: {
    register_spike: {
      executor: 'ramping-vus',
      startVUs: 0,
      stages: [
        { duration: '1m', target: 20 }, // ramp up to 20 VUs
        { duration: '4m', target: 20 }, // hold -> HPA scales 1 -> 2 -> 3
      ],
      gracefulRampDown: '10s',
    },
  },
  thresholds: {
    // Gate: emails are unique, so registrations should almost all return 2xx.
    // A breach here means real failures (e.g. DB pool exhaustion), not just
    // slowness.
    http_req_failed: ['rate<0.02'],
    // Informational: latency rises sharply once pods are CPU-saturated and
    // requests queue behind bcrypt. That is expected under this test and does
    // not indicate a broken system — the scaling behaviour is the real signal.
    http_req_duration: ['p(95)<5000'],
  },
};

export default function () {
  const payload = JSON.stringify({
    email: `loadtest+${RUN_ID}-${__VU}-${__ITER}@k6.local`,
    password: PASSWORD,
  });

  const res = http.post(`${BASE_URL}/auth/register`, payload, {
    headers: { 'Content-Type': 'application/json' },
    tags: { endpoint: 'register' },
  });

  check(res, {
    'status is 2xx': (r) => r.status >= 200 && r.status < 300,
  });
}
