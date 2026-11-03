# In-Memory Cache with TTL

Take-home version of the technical assessment: an in-memory cache with TTL, safe concurrent access, and the bonus features from the brief. An HTTP server is included so you can exercise the API with Postman or curl.

## How to run

```bash
go run .          # HTTP server on :8080 (seeded with sample data)
go test ./...     # unit + HTTP tests
```

Default store is **in-memory** (matches the brief).

## Seed data

On startup the cache is loaded with four loyalty profiles (TTL **3600 seconds** / 1 hour). Keys match `customer_id`.

Seeding uses the normal `SetMany` path, so `/cache/stats` will already show set successes before any client traffic.

| Key | Name | Status | Tier | Points | Lifetime | Reward |
|---|---|---|---|---|---|---|
| `cust-001` | Alex Flame | active | Hot | 120 | 400 | Free PERi-PERi Chips (50 pts) |
| `cust-002` | Sam Extra | active | Extra Hot | 340 | 1200 | Free Butterfly Chicken (200 pts) |
| `cust-003` | Jordan Lemon | active | Lemon & Herb | 75 | 180 | Free Soft Drink (30 pts) |
| `cust-004` | Riley Mild | inactive | Mild | 10 | 10 | _(none)_ |

Example:

```bash
curl http://localhost:8080/cache/cust-001
curl http://localhost:8080/cache/keys
curl http://localhost:8080/cache/size
```

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
| Automatic cleanup of expired entries | Done (`StartCleanup`) |
| Statistics (hit/miss, operation totals) | Done (`Stats()`, success/failure ratios) |
| `keys()` — active keys | Done |
| Thread safety | Done |
| Max size + LRU eviction | Done |
| Batch: `set_many` / `get_many` / `delete_many` | Done |

## Design decisions / trade-offs

- **Lazy + periodic expiry:** expired entries are dropped on read paths (`get` / `size` / `keys`) and by a background cleanup loop.
- **Layering:** cache package handles validation and metrics; persistence sits behind a `Store` interface with an in-memory implementation.
- **LRU:** on overflow, evict least-recently-accessed (`set` and `get` both update access time). Equal access times break ties on earliest expiry. Expired entries are removed before live eviction.
- **Batch partial failure:** `set_many` / `delete_many` continue after per-item errors; the call returns `batch partially applied: …` while successful items in that request remain applied.
- **Misses vs errors:** a missing/expired key is not a system failure; the API surfaces `ErrNotFound` so callers (and HTTP) can distinguish 404 from 500.
- **Stats:** hit/miss and per-operation success/failure are recorded via a `metrics.Recorder` interface; the concrete implementation is an in-memory recorder exposed through `/cache/stats`.

## Time complexity (in-memory store)

| Operation | Typical |
|---|---|
| `get` / `set` / `delete` | O(1) average |
| `size` / `keys` / expired sweep | O(n) |
| LRU eviction (per overflow) | O(n) |

---

## HTTP API

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

`POST /cache/batch/get` returns `[{ "key", "found", "profile"? }, ...]`. Misses set `"found": false` and omit `profile`.

Batch set items with missing/`0` `ttl_seconds` default to **60** (same as `PUT` without `?ttl=`). Negative TTLs are rejected.

Flags / env: `-addr` (default `:8080`), `-max-size`, `-cleanup-interval`.
