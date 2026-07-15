# Cloud Commerce — Developer Makefile

DATABASE_URL ?= postgres://app:app@localhost:5432/app?sslmode=disable

.PHONY: setup dev-infra down test test-integration migrate migrate-down migrate-create lint build setup-hooks ratchet help

help: ## Show this help message
	@grep -E '^[a-zA-Z_%-]+:.*##' $(MAKEFILE_LIST) \
		| awk 'BEGIN {FS = ":.*##"}; {printf "  \033[36m%-22s\033[0m %s\n", $$1, $$2}'

setup: ## Sync the go workspace and install git hooks
	go work sync
	bash scripts/install-hooks.sh

dev-infra: ## Start local infrastructure via docker compose (detached)
	docker compose up -d --wait postgres

down: ## Stop and remove all docker compose containers
	docker compose down

run-%: ## Run a service locally (e.g. make run-example)
	go run ./services/$*/cmd

test: ## Run all Go unit tests
	@set -e; \
	for dir in services/* pkg/*; do \
		if [ -f "$$dir/go.mod" ]; then \
			echo "=== Testing $$dir ==="; \
			(cd "$$dir" && go test ./...); \
		fi; \
	done

test-integration: ## Run all integration tests (testcontainers locally, TEST_POSTGRES_DSN in CI)
	@set -e; \
	for dir in services/* pkg/*; do \
		if [ -f "$$dir/go.mod" ]; then \
			echo "=== Integration tests: $$dir ==="; \
			(cd "$$dir" && go test -tags integration ./...); \
		fi; \
	done

migrate: ## Apply all pending example-service migrations (requires golang-migrate)
	migrate -path ./services/example/migrations -database "$(DATABASE_URL)" up

migrate-down: ## Roll back the last example-service migration
	migrate -path ./services/example/migrations -database "$(DATABASE_URL)" down 1

migrate-create: ## Create a new migration file (usage: make migrate-create NAME=create_foo)
	migrate create -ext sql -dir ./services/example/migrations -seq $(NAME)

lint: ## Run golangci-lint per module (GOWORK=off, mirrors CI)
	@set -e; \
	CONFIG="$$(pwd)/.golangci.yml"; \
	for dir in services/* pkg/*; do \
		if [ -f "$$dir/go.mod" ]; then \
			echo "=== Linting $$dir ==="; \
			(cd "$$dir" && GOWORK=off golangci-lint run --config="$$CONFIG" --timeout=5m ./...); \
		fi; \
	done

ratchet: ## Check that no new gin.H success bodies were added
	bash scripts/check_ginh_success_ratchet.sh

setup-hooks: ## Install git pre-commit hooks from scripts/hooks/
	bash scripts/install-hooks.sh

build: ## Build all Go modules
	@set -e; \
	for dir in services/* pkg/*; do \
		if [ -f "$$dir/go.mod" ]; then \
			echo "=== Building $$dir ==="; \
			(cd "$$dir" && go build ./...); \
		fi; \
	done

types: ## Regenerate TypeScript types from Go wire structs (tygo)
	go run github.com/gzuidhof/tygo@v0.2.18 generate
