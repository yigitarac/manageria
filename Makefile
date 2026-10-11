.DEFAULT_GOAL := help

.PHONY: help dev dev-api dev-web gen fmt lint test bench sample db-up db-down

# Sample matches vary per run by default; pin one with: make sample SEED=42
SEED ?= $(shell date +%s%N)

# Race-enabled engine sweeps simulate >12k matches (calibration/menu/hold alone: 10.6k).
# Race instrumentation measures ~11.6x per-match cost (~572 ms vs ~49 ms, BenchmarkSimulate),
# projecting the engine package past go test's default 10m per-package timeout (it panics
# mid-suite). Bounded budget with headroom; raise on slower machines with:
#   make test GO_TEST_TIMEOUT=90m
GO_TEST_TIMEOUT ?= 60m

# Lightweight per-match engine benchmark (benchmarks only, tests excluded with -run '^$$').
# Sample size is a wall-clock budget ('1s', '100x') and a repetition count for spread:
#   make bench BENCH_TIME=1s BENCH_COUNT=5
BENCH ?= BenchmarkSimulate
BENCH_TIME ?= 10x
BENCH_COUNT ?= 3

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
	cd server && out=$$(gofmt -l .); if [ -n "$$out" ]; then echo "$$out"; echo "gofmt needed (run: make fmt)"; exit 1; fi
	cd web && pnpm lint && pnpm typecheck

test: ## Run Go and web tests (package timeout: GO_TEST_TIMEOUT, default 60m)
	cd server && go test -race -timeout=$(GO_TEST_TIMEOUT) ./...
	cd web && pnpm test

bench: ## Benchmark engine matches (BENCH=BenchmarkSimulate BENCH_TIME=10x BENCH_COUNT=3)
	cd server && go test ./internal/engine -run '^$$' -bench $(BENCH) -benchtime=$(BENCH_TIME) -count=$(BENCH_COUNT) -benchmem

sample: ## Generate a fresh sample match (override with SEED=<n> for a reproducible match)
	mkdir -p web/public/samples
	cd server && go run ./cmd/simcli -seed $(SEED) -runs 1 -out ../web/public/samples/match-42.json
	@echo "open http://localhost:5173/viewer — a different match every run (same one with SEED=<n>)"

db-up: ## Start local Postgres
	docker compose -f deploy/docker-compose.yml up -d

db-down: ## Stop local Postgres
	docker compose -f deploy/docker-compose.yml down