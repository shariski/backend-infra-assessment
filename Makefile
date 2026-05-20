.PHONY: setup run build test lint tidy db-up db-down migrate-up migrate-down swagger loadtest loadtest-clean

DB_URL ?= postgres://postgres:postgres@localhost:5432/auth?sslmode=disable
GOLANGCI_LINT_VERSION ?= v2.11.4
SWAG_VERSION ?= v1.16.6

setup:
	@echo "==> Installing dev tools"
	go install golang.org/x/tools/cmd/goimports@latest
	go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(GOLANGCI_LINT_VERSION)
	go install github.com/evilmartians/lefthook@latest
	go install github.com/swaggo/swag/cmd/swag@$(SWAG_VERSION)
	@echo "==> Activating Git hooks"
	lefthook install
	@echo "==> Done. Pre-commit (fmt/vet/lint) and pre-push (tests) hooks are active."

swagger:
	swag init -g cmd/api/main.go -o docs --parseInternal --parseDependency

run:
	go run ./cmd/api

build:
	go build -o bin/api ./cmd/api

test:
	go test ./...

lint:
	golangci-lint run ./...

tidy:
	go mod tidy

db-up:
	docker compose up -d

db-down:
	docker compose down

migrate-up:
	migrate -path migrations -database "$(DB_URL)" up

migrate-down:
	migrate -path migrations -database "$(DB_URL)" down 1

# Load test: drives the staging HPA via port-forward. Requires a running
# `kubectl -n staging port-forward svc/auth 8080:80`. See loadtest/README.md.
loadtest:
	k6 run -e RUN_ID=$$(date +%s) --summary-export loadtest/summary.json loadtest/k6/register-spike.js

# Remove the users created by the load test.
loadtest-clean:
	kubectl -n staging exec -i statefulset/postgres -- \
		psql -U postgres -d auth -c \
		"DELETE FROM users WHERE email LIKE 'loadtest+%@k6.local';"
