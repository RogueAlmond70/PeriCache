# In-Memory Cache with TTL

Take-home version of the technical assessment: an in-memory cache with TTL, safe concurrent access, and the bonus features from the brief. Extra production-style pieces (HTTP, Prometheus, Mongo, Docker) are documented at the end as stretch work.

## How to run

```bash
go run . -mode=demo          # exercise the cache API in-process
go run .                     # HTTP server on :8080 (optional stretch)
go test ./...                # unit + HTTP tests
```

Default store is **in-memory** (matches the brief). No Docker or Mongo required for the core solution.

## Requirements implemented

### Core

| Requirement | Status |
|---|---|
| `set(key, value, ttl_seconds)` | Done |
| `get(key)` — missing/expired not returned | Done (`ErrNotFound`) |
| `delete(key)` | Done |
| `clear()` | Done |
| `size()` — active (non-expired) only | Done |
| String keys | Done |
| TTL in seconds (integer) | Done |
| Concurrent access safety | Done (mutex in memory store; covered by race tests) |

**Value type:** the brief allows any language-supported type. This implementation stores `loyalty.LoyaltyProfile` (domain-shaped values) instead of a generic `any`.

### Bonus

| Requirement | Status |
|---|---|
| Automatic cleanup of expired entries | Done (`StartCleanup` / demo `Run`) |
| Statistics (hit/miss, operation totals) | Done (`Stats()`, success/failure ratios) |
| `keys()` — active keys | Done |
| Thread safety | Done |
| Max size + LRU eviction | Done |
| Batch: `set_many` / `get_many` / `delete_many` | Done |

## Design decisions / trade-offs

- **Lazy + periodic expiry:** expired entries are dropped on read paths (`get` / `size` / `keys`) and by a background cleanup loop.
- **Layering:** cache package handles validation and metrics; persistence sits behind a `Store` interface. Default implementation is an in-memory map.
- **LRU:** on overflow, evict least-recently-accessed; if nothing has been accessed yet, evict earliest expiry.
- **Misses vs errors:** a missing/expired key is not a system failure; the API surfaces `ErrNotFound` so callers (and HTTP) can distinguish 404 from 500.
- **Stats:** hit/miss and per-operation success/failure are recorded via a `metrics.Recorder` (in-memory snapshot by default).

## Time complexity (in-memory store)

| Operation | Typical |
|---|---|
| `get` / `set` / `delete` | O(1) average |
| `size` / `keys` / expired sweep | O(n) |
| LRU eviction (per overflow) | O(n) |

---

## Stretch / extras (beyond the brief)

These are optional enhancements for a more “service-like” review. **Ignore this section if you only want the assessment core.**

### HTTP API

```bash
go run .
```

| Method | Path |
|---|---|
| `GET` | `/healthz` |
| `PUT` | `/cache/{key}?ttl=60` (JSON `LoyaltyProfile` body) |
| `GET` | `/cache/{key}` |
| `DELETE` | `/cache/{key}` |
| `DELETE` | `/cache` |
| `GET` | `/cache/size`, `/cache/keys`, `/cache/stats` |
| `POST` | `/cache/batch`, `/cache/batch/get`, `/cache/batch/delete` |
| `GET` | `/metrics` |

Flags / env: `-mode`, `-addr`, `-store` (`memory`\|`mongo`), `-metrics` (`memory`\|`prometheus`\|`both`), `-mongo-uri`.

### Pluggable metrics

- `memory` — JSON via `/cache/stats`
- `prometheus` — scrape `/metrics` (`peri_peri_lru_cache_*`)
- `both` — fan-out to both (default)

### Mongo store + Docker Compose

Shared persistence and a local observability stack:

```bash
docker compose up --build
```

| Service | URL |
|---|---|
| Cache API | http://localhost:8080 |
| Prometheus | http://localhost:9090 |
| Grafana | http://localhost:3000 (`admin` / `nandos-local` in `config/grafana.env`) |
| MongoDB | localhost:27017 |

**Scaling note:** in-memory is per-process; use Mongo if multiple replicas should share cache state.
