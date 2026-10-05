.DEFAULT_GOAL := help

.PHONY: help dev dev-api dev-web gen fmt lint test db-up db-down

help: ## Show available targets
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "  %-10s %s\n", $$1, $$2}'

dev: ## Run API (:8080) and web (:5173) together
	@trap 'kill 0' EXIT; $(MAKE) dev-api & $(MAKE) dev-web & wait

dev-api: ## Run the Go API server
	cd server && go run ./cmd/api

dev-web: ## Run the Vite dev server
	cd web && pnpm dev

gen: ## Regenerate API code from api/openapi.yaml
	cd server && go generate ./...
	cd web && pnpm gen:api

fmt: ## Format Go and web sources
	cd server && gofmt -w .
	cd web && pnpm exec prettier --write .

lint: ## Lint Go and web sources
	cd server && go vet ./... && go tool golangci-lint run ./...
	cd web && pnpm lint && pnpm typecheck

test: ## Run Go and web tests
	cd server && go test ./...
	cd web && pnpm test

db-up: ## Start local Postgres
	docker compose -f deploy/docker-compose.yml up -d

db-down: ## Stop local Postgres
	docker compose -f deploy/docker-compose.yml down