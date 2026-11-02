# Nandos Pair Coding — In-Memory / Mongo Cache with TTL

TTL cache for `LoyaltyProfile` values, with HTTP API, pluggable metrics, and optional MongoDB storage.

## Quick start (IDE / local)

```bash
go run .
```

Default: in-memory store, HTTP on `:8080`, metrics at `/metrics`.

Useful flags / env:

| Flag | Env | Default | Meaning |
|---|---|---|---|
| `-mode` | `CACHE_MODE` | `server` | `server` or `demo` |
| `-addr` | `CACHE_ADDR` | `:8080` | listen address |
| `-store` | `CACHE_STORE` | `memory` | `memory` or `mongo` |
| `-mongo-uri` | `MONGO_URI` | `mongodb://localhost:27017` | Mongo URI |
| `-metrics` | `METRICS_BACKEND` | `both` | `memory`, `prometheus`, or `both` |

```bash
go test ./...
```

## Full stack (Docker Compose)

```bash
docker compose up --build
```

Services:

| Service | URL |
|---|---|
| Cache API | http://localhost:8080 |
| Prometheus | http://localhost:9090 |
| Grafana | http://localhost:3000 |
| MongoDB | localhost:27017 |

Grafana login (see `config/grafana.env`):

- user: `admin`
- password: `nandos-local`

Provisioned dashboard: **PERi-PERi LRU Cache** (Prometheus datasource).

Example PromQL:

```promql
peri_peri_lru_cache_operations_total{operation="cleanup",outcome="success"}
```

## HTTP API (Postman)

- `GET /healthz`
- `PUT /cache/{key}?ttl=60` — JSON `LoyaltyProfile` body
- `GET /cache/{key}`
- `DELETE /cache/{key}`
- `DELETE /cache`
- `GET /cache/size` | `/cache/keys` | `/cache/stats`
- `POST /cache/batch` | `/cache/batch/get` | `/cache/batch/delete`
- `GET /metrics`

## Requirements implemented

**Core:** set / get / delete / clear / size, string keys, TTL seconds, expired entries omitted on get.

**Bonus:** automatic cleanup, statistics (+ Prometheus), keys, thread-safe memory store, max size + LRU, batch ops.

**Deviations / extensions:** values are `loyalty.LoyaltyProfile`; selectable `memory|mongo` store; HTTP + Docker observability stack.

## Design notes

- Cache layer = validation, metrics, orchestration.
- `Store` port = memory (mutex + LRU) or Mongo (TTL index + query eviction).
- `metrics.Recorder` = memory snapshot and/or Prometheus `CounterVec`s.
- Multi-pod: use Mongo for shared state; memory is per-process only.

## Complexity (memory store)

| Op | Typical |
|---|---|
| get / set / delete | O(1) average |
| keys / size / deleteExpired | O(n) |
| LRU eviction scan | O(n) per eviction |
