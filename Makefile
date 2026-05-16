.PHONY: setup run build test lint tidy db-up db-down migrate-up migrate-down

DB_URL ?= postgres://postgres:postgres@localhost:5432/auth?sslmode=disable

setup:
	@echo "==> Installing dev tools"
	go install golang.org/x/tools/cmd/goimports@latest
	go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@latest
	go install github.com/evilmartians/lefthook@latest
	@echo "==> Activating Git hooks"
	lefthook install
	@echo "==> Done. Pre-commit (fmt/vet/lint) and pre-push (tests) hooks are active."

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
