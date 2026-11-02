.PHONY: help dev worker build migrate test vet fmt tidy up down logs

help: ## list targets
	@grep -E '^[a-z-]+:.*##' $(MAKEFILE_LIST) | sed 's/:.*## /\t/'

# --- Native (needs local Postgres + Redis; reads .env) ---
dev: ## run the API server
	set -a && . ./.env && set +a && go run ./cmd/server

worker: ## run the background worker
	set -a && . ./.env && set +a && go run ./cmd/worker

migrate: ## apply database migrations
	set -a && . ./.env && set +a && go run ./cmd/migrate up

build: ## build all binaries into bin/
	go build -o bin/server ./cmd/server
	go build -o bin/migrate ./cmd/migrate
	go build -o bin/worker ./cmd/worker
	go build -o bin/thumbgen ./cmd/thumbgen
	go build -o bin/admin ./cmd/admin

# --- Quality ---
test: ## run unit tests (DB-backed tests skip without a database)
	go test ./...

vet: ## go vet
	go vet ./...

fmt: ## gofmt
	gofmt -w cmd internal

tidy: ## go mod tidy
	go mod tidy

# --- Docker ---
up: ## start db, redis, migrations, api and worker
	docker compose up -d --build

down: ## stop the stack and delete its volumes
	docker compose down -v

logs: ## follow worker logs
	docker compose logs -f worker
