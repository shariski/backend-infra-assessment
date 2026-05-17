.PHONY: setup run build test lint tidy db-up db-down migrate-up migrate-down swagger

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
