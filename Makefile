SHELL := /bin/bash
GOFILES := $(shell find . -name '*.go' ! -path './web/*' 2>/dev/null)

.PHONY: up down logs build test vet test-integration fmt run-api run-worker run-migrate run-seed db-up db-down

# Full stack in Docker: postgres, redis, migrate, api, worker, web.
up:
	docker compose -f docker-compose.yml up -d --build

down:
	docker compose -f docker-compose.yml down

logs:
	docker compose -f docker-compose.yml logs -f api worker web

build:
	go build ./...

fmt:
	go fmt ./...

vet:
	go vet ./...

test: vet
	go test ./...

# Integration tests need PostgreSQL. Starts compose services first when
# the docker daemon is available, and exports TEST_DATABASE_URL.
test-integration: vet
	if docker info >/dev/null 2>&1; then \
		docker compose -f deploy/docker-compose.yml up -d postgres redis && \
		echo "waiting for postgres..." && \
		until docker compose -f deploy/docker-compose.yml exec -T postgres pg_isready -q; do sleep 1; done; \
		export TEST_DATABASE_URL=postgres://site24x7:site24x7@localhost:5432/site24x7_test?sslmode=disable; \
	else \
		echo "docker unavailable; relying on TEST_DATABASE_URL if exported"; \
	fi; \
	if [ -n "$$TEST_DATABASE_URL" ]; then go test -count=1 ./internal/... ; else \
		echo "no TEST_DATABASE_URL; integration tests will be skipped"; go test ./internal/...; fi

run-api:
	go run ./cmd/api

run-worker:
	go run ./cmd/worker

# Local PostgreSQL + Redis for development (docker compose).
db-up:
	docker compose -f deploy/docker-compose.yml up -d postgres redis

db-down:
	docker compose -f deploy/docker-compose.yml down

run-migrate:
	go run ./cmd/migrate

run-seed:
	go run ./cmd/seed
