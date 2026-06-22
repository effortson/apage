.PHONY: build test test-e2e vet infra-up infra-down up down api worker apage-cli tidy fmt dev

# One-command local debug: infra + api + worker + web in one terminal,
# colored per-service output, Ctrl+C stops everything. (apage-cli is built but
# not auto-started — run it manually with `apage-cli mcp`.)
dev:
	./scripts/dev.sh

build:
	go build ./...

# Unit tests — no infra required.
test:
	go test ./...

# In-process, multi-surface end-to-end tests (api + worker + visitor) against the
# live infra. Brings infra up first; skips cleanly if it is unreachable.
# See internal/e2e/.
test-e2e: infra-up
	go test -tags e2e -timeout 300s ./internal/e2e/...

vet:
	go vet ./...
	go vet -tags e2e ./internal/e2e/...

tidy:
	go mod tidy

fmt:
	gofmt -w .

# Bring up only the infra (postgres+redis+minio) for native dev.
infra-up:
	docker compose up -d postgres redis minio

infra-down:
	docker compose down

# Full stack via docker.
up:
	docker compose up -d --build

down:
	docker compose down

# Run services natively (after infra-up + .env exported).
api:
	go run ./cmd/api

worker:
	go run ./cmd/worker

# Build the customer-side CLI (MCP server). Run it manually via `apage-cli mcp`.
apage-cli:
	go build -o bin/apage-cli ./cmd/apage-cli
