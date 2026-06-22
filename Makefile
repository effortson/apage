.PHONY: build test vet infra-up infra-down up down api gateway worker tidy fmt dev

# One-command local debug: infra + api + gateway + worker + web in one terminal,
# colored per-service output, Ctrl+C stops everything.
dev:
	./scripts/dev.sh

build:
	go build ./...

test:
	go test ./...

vet:
	go vet ./...

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

gateway:
	go run ./cmd/gateway

worker:
	go run ./cmd/worker
