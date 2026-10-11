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
| `tools/` | Standalone diagnostics (e.g. the viewer's visual-nonsense meter) |

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

## Match viewer

`make sample` makes the engine write a fresh match dump to
`web/public/samples/match-42.json` (gitignored) — a **different match every run** (the seed varies);
pin a specific one with `make sample SEED=42`. Open <http://localhost:5173/viewer> — the
sample auto-loads when present, and any `simcli -out match.json` dump can be dropped onto
the page. Space plays/pauses, the timeline scrubs, playback speed goes up to 180×.
The replay starts paused at 30× speed. Score and commentary reveal as the match unfolds;
live event statistics update in time, while final possession and xG appear at full time.

## Match engine tools

The engine is deterministic: the same input and seed always produce the same match. `simcli`
(run from `server/`) is the workbench for inspecting it:

```sh
go run ./cmd/simcli -seed 42              # simulate twice, verify identical digests
go run ./cmd/simcli -matches 800          # result distribution over consecutive seeds
go run ./cmd/simcli -audit -matches 150   # tactic-preset balance matrix, flags violations
go run ./cmd/simcli -kpi 100              # behavioural KPIs (chains, passes, regime mix)
go run ./cmd/simcli -story -seed 42       # one match as a possession-by-possession narrative
go run ./cmd/simcli -analyze 30           # football-IQ smell metrics
go run ./cmd/simcli -out match.json       # dump a match for the viewer
```

`go run ./cmd/simcli -h` lists every mode. `node tools/visual-nonsense.mjs <dump.json>` measures
what a viewer would feel as wrong in a dump (frozen players, jumping dots).
`go test -bench BenchmarkSimulate ./internal/engine` tracks the per-match speed budget.

## Development

```sh
make lint      # go vet + golangci-lint, eslint + tsc
make test      # go test, vitest
make fmt       # gofmt + prettier
```

`make test` runs the race-enabled Go suite with a raised per-package timeout
(`GO_TEST_TIMEOUT`, default 60m): the calibration sweeps simulate thousands of matches and
race instrumentation costs ~11.6x per match, so the stock 10m `go test` timeout panics
mid-suite. Raise it on slow machines (`make test GO_TEST_TIMEOUT=90m`) instead of lowering
test sample counts.

The workflow is **spec-first**: change `api/openapi.yaml`, run `make gen`, then implement.
