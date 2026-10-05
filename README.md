# Manageria

An online football management game: the accessibility and daily rhythm of *Top Eleven* with the
depth of *Football Manager* — and **zero pay-to-win**. All clubs, players and leagues are fictional.

## Layout

| Path | What |
|---|---|
| `api/` | OpenAPI 3 spec — single source of truth for the HTTP API |
| `server/` | Go module: HTTP/WebSocket API + deterministic match engine |
| `web/` | React + Vite + TypeScript client (future Capacitor/Tauri payload) |
| `deploy/` | Docker Compose (Postgres) and production config |

## Requirements

- Go 1.26+
- Node 22+ and pnpm (`corepack enable pnpm`)
- Docker with Compose

## Getting started

```sh
make db-up     # start local Postgres
make gen       # regenerate API code from api/openapi.yaml
make dev       # API on :8080 + web on :5173
```

Open <http://localhost:5173> — the page shows the API health status.

## Development

```sh
make lint      # go vet + golangci-lint, eslint + tsc
make test      # go test, vitest
make fmt       # gofmt + prettier
```

The workflow is **spec-first**: change `api/openapi.yaml`, run `make gen`, then implement.