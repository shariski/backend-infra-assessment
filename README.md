# Auth API

A secure REST API boilerplate for JWT-based authentication, built with Go and Gin.

## Status

- **Implemented:** authentication system — register, login, refresh, logout,
  RBAC (Admin / Analyst / Viewer), bcrypt password hashing, per-IP login rate
  limiting, and account-level brute-force protection.
- **Stubbed (Security Analytics):** audit trails, per-role API rate limiting,
  and compliance request/response logging — see the `// TODO` markers in
  `internal/middleware/{audit,ratelimit,request_logger}.go`.

## Tech Stack

Go 1.25 · Gin · Gorm + PostgreSQL · golang-migrate · Viper · log/slog ·
golang-jwt/jwt v5 · bcrypt.

## Prerequisites

- Go 1.25+
- Docker (for local PostgreSQL)
- [`golang-migrate`](https://github.com/golang-migrate/migrate) CLI:
  `go install -tags 'postgres' github.com/golang-migrate/migrate/v4/cmd/migrate@latest`

## Getting Started

```bash
cp .env.example .env          # then edit JWT_SECRET
make setup                    # install dev tools + activate Git hooks (once per clone)
make db-up                    # start PostgreSQL
make migrate-up               # apply migrations
make run                      # start the API on :8080
```

## Configuration

All configuration is environment-based (see `.env.example`). `JWT_SECRET` is
required and has no default — the app fails fast if it is missing.

## Project Layout

```
cmd/api          entrypoint
config           environment-based configuration loader
internal/domain  entities and repository/service interfaces
internal/repository  Gorm implementations
internal/service     business logic (auth, JWT)
internal/handler     Gin HTTP handlers, DTOs, error envelope
internal/middleware  auth, RBAC, recovery, rate limiting, analytics stubs
internal/router      route and middleware wiring
internal/server      http.Server with graceful shutdown
pkg/logger           slog setup
pkg/hash             bcrypt + SHA-256 helpers
pkg/database         Gorm connection
migrations           golang-migrate SQL files
```

## API Endpoints

| Method | Path             | Auth            | Description                  |
|--------|------------------|-----------------|------------------------------|
| POST   | `/auth/register` | public          | Create a user (Viewer role)  |
| POST   | `/auth/login`    | public          | Issue access + refresh token |
| POST   | `/auth/refresh`  | refresh token   | Rotate the token pair        |
| POST   | `/auth/logout`   | bearer          | Revoke a refresh token       |
| GET    | `/auth/me`       | bearer          | Current user profile         |
| GET    | `/admin/users`   | bearer + Admin  | Example RBAC-protected route |
| GET    | `/healthz`       | public          | Liveness check               |

## Development

```bash
make test     # run unit tests
make lint     # run golangci-lint
make build    # build the binary into ./bin
```

### Git hooks (lefthook)

`make setup` installs [lefthook](https://github.com/evilmartians/lefthook) and
runs `lefthook install`, which wires up:

- **pre-commit** (on staged `*.go` files): `goimports -w`, `go vet ./...`,
  `golangci-lint run ./...`
- **pre-push**: `go test -short -race ./...`

The hooks must be re-activated per clone — committing `lefthook.yml` alone is
not enough. The same `golangci-lint` config (`.golangci.yml`) is also enforced
in CI via `.github/workflows/test.yml`.
