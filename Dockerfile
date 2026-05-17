# syntax=docker/dockerfile:1
FROM golang:1.25-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /app/bin/api ./cmd/api

# Source for the standalone `migrate` binary. The image is built on scratch
# and ships the binary at /migrate.
FROM migrate/migrate:v4.17.0 AS migrate

FROM alpine:3.20
RUN apk add --no-cache ca-certificates
WORKDIR /app
COPY --from=builder /app/bin/api .
COPY --from=migrate /migrate /usr/local/bin/migrate
COPY migrations /migrations
EXPOSE 8080
ENTRYPOINT ["/app/api"]
