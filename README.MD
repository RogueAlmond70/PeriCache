# PeriCache — In-Memory Cache with TTL

Take-home version of the technical assessment: an in-memory cache with TTL, safe concurrent access, and the bonus features from the brief. An HTTP server is included so you can exercise the API with Postman, Insomnia, curl, or similar.

## How to run

```bash
go run .          # HTTP server on :8080 (seeded with 40 sample profiles)
go test ./...     # unit + HTTP tests
```

Default store is **in-memory** (matches the brief).

Flags / env:

| Flag | Env | Default | Meaning |
|---|---|---|---|
| `-addr` | `CACHE_ADDR` | `:8080` | Listen address |
| `-max-size` | — | `100` | Max live entries (LRU eviction above this) |
| `-cleanup-interval` | — | `30` | Background expired-entry sweep interval (seconds) |
| `-replace` | — | `false` | If `true` and the listen port is busy, stop the process holding it and bind (`go run . -replace`) |

---

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
- **Layering:** cache package handles validation and metrics. Persistence is a pluggable `Store` (entries are exported `Record` values) with an in-memory implementation; metrics use a pluggable `metrics.Recorder` (optional `Snapshotter` for `/cache/stats`).
- **Profile isolation:** `LoyaltyProfile` reward slices are cloned on set and get so callers cannot mutate cached state through shared backing arrays.
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

## Operations (complete list)

### Go cache API (`cache.Cache`)

| Operation | Signature (conceptual) | Behaviour |
|---|---|---|
| **Set** | `Set(ctx, key, profile, ttlSeconds)` | Store / overwrite a `LoyaltyProfile`. TTL must be ≥ 1. Empty key or empty `customer_id` → error. Insert updates LRU access time. Enforces max size via eviction. |
| **Get** | `Get(ctx, key) → profile` | Return live entry. Missing or expired → `ErrNotFound` (counted as miss, not failure). Updates LRU access time on hit. |
| **Delete** | `Delete(ctx, key)` | Remove key if present. Missing key is success (idempotent). Empty key → error. |
| **Clear** | `Clear(ctx)` | Remove all entries. |
| **Size** | `Size(ctx) → int` | Count of **non-expired** entries only. |
| **Keys** | `Keys(ctx) → []string` | List of **non-expired** keys (order not guaranteed). |
| **SetMany** | `SetMany(ctx, []BatchRequest)` | Set each item. `ttl_seconds == 0` defaults to **60**; negative TTL rejected. Continues after per-item errors; returns `batch partially applied: …` if any failed. |
| **GetMany** | `GetMany(ctx, []BatchRequest) → []BatchGetItem` | Per key: `{ key, found, profile? }`. Misses: `found: false`, no profile. |
| **DeleteMany** | `DeleteMany(ctx, []BatchRequest)` | Delete each key; same partial-error behaviour as SetMany. |
| **Stats** | `Stats() → Snapshot` | Hit/miss counts & ratios; per-op attempted/success/failure ratios. |
| **StartCleanup** | `StartCleanup(ctx, intervalSeconds)` | Background loop deleting expired entries. |

### HTTP API

Base URL: `http://localhost:8080`

| Method | Path | Body | Success | Notes |
|---|---|---|---|---|
| `GET` | `/healthz` | — | `200` `{"status":"ok"}` | Liveness |
| `PUT` | `/cache/{key}?ttl=60` | `LoyaltyProfile` JSON | `204` | Omit `ttl` → default **60**. `ttl<1` or non-int → `400` |
| `GET` | `/cache/{key}` | — | `200` profile | Miss/expired → `404` |
| `DELETE` | `/cache/{key}` | — | `204` | Missing key still `204` |
| `DELETE` | `/cache` | — | `204` | Clear entire cache |
| `GET` | `/cache/size` | — | `200` `{"size":N}` | Active entries only |
| `GET` | `/cache/keys` | — | `200` `{"keys":[...]}` | Active keys |
| `GET` | `/cache/stats` | — | `200` stats snapshot | Includes seed traffic |
| `POST` | `/cache/batch` | `[]BatchRequest` | `204` | Set many |
| `POST` | `/cache/batch/get` | `[]BatchRequest` (keys) | `200` `[]BatchGetItem` | Get many |
| `POST` | `/cache/batch/delete` | `[]BatchRequest` (keys) | `204` | Delete many |

**Error responses** are JSON: `{"error":"<message>"}` with `400` (validation), `404` (not found), or `500` (unexpected). Request bodies larger than **1 MiB** are rejected.

---

## Seed data

On startup the cache is loaded with **40** loyalty profiles (`cust-001` … `cust-040`), each with TTL **3600 seconds** (1 hour). Keys match `customer_id`.

Seeding uses the normal `SetMany` path, so `/cache/stats` already shows set successes before any client traffic.

After start:

```bash
curl http://localhost:8080/cache/size    # → {"size":40}
curl http://localhost:8080/cache/keys
curl http://localhost:8080/cache/cust-001
```

| Key | Name | Status | Tier | Points | Lifetime | Reward(s) |
|---|---|---|---|---|---|---|
| `cust-001` | Alex Flame | active | Hot | 120 | 400 | Free PERi-PERi Chips (50) |
| `cust-002` | Sam Extra | active | Extra Hot | 340 | 1200 | Free Butterfly Chicken (200) |
| `cust-003` | Jordan Lemon | active | Lemon & Herb | 75 | 180 | Free Soft Drink (30) |
| `cust-004` | Riley Mild | inactive | Mild | 10 | 10 | — |
| `cust-005` | Casey Spice | active | Hot | 210 | 650 | Free 4 Wings (80) |
| `cust-006` | Morgan Grill | active | Extra Hot | 455 | 2100 | Free Chicken Burger (250) |
| `cust-007` | Avery Herb | active | Lemon & Herb | 95 | 320 | Free PERi-PERi Chips (50) |
| `cust-008` | Quinn Cool | active | Mild | 40 | 90 | — |
| `cust-009` | Reese Blaze | active | Hot | 180 | 540 | Soft Drink (30), Chips (50) |
| `cust-010` | Skyler Inferno | active | Extra Hot | 600 | 3000 | Free Sharing Platter (500) |
| `cust-011` | Jamie Citrus | active | Lemon & Herb | 130 | 410 | Free 4 Wings (80) |
| `cust-012` | Taylor Soft | suspended | Mild | 5 | 5 | — |
| `cust-013` | Drew Pepper | active | Hot | 155 | 480 | Free PERi-PERi Chips (50) |
| `cust-014` | Cameron Volcano | active | Extra Hot | 290 | 980 | Free Butterfly Chicken (200) |
| `cust-015` | Parker Zest | active | Lemon & Herb | 88 | 220 | Free Soft Drink (30) |
| `cust-016` | Finley Gentle | active | Mild | 55 | 140 | — |
| `cust-017` | Hayden Chili | active | Hot | 200 | 700 | Free 4 Wings (80) |
| `cust-018` | Rowan Magma | active | Extra Hot | 375 | 1500 | Free Chicken Burger (250) |
| `cust-019` | Sage Lime | inactive | Lemon & Herb | 20 | 60 | — |
| `cust-020` | Blake Warm | active | Mild | 70 | 190 | Free Soft Drink (30) |
| `cust-021` | Emerson Scorch | active | Hot | 145 | 430 | Free PERi-PERi Chips (50) |
| `cust-022` | Kendall Phoenix | active | Extra Hot | 520 | 2400 | Free Sharing Platter (500) |
| `cust-023` | Logan Basil | active | Lemon & Herb | 110 | 350 | Free 4 Wings (80) |
| `cust-024` | Peyton Calm | active | Mild | 35 | 80 | — |
| `cust-025` | River Ember | active | Hot | 260 | 820 | Free Butterfly Chicken (200) |
| `cust-026` | Dakota Lava | active | Extra Hot | 410 | 1750 | Free Chicken Burger (250) |
| `cust-027` | Charlie Thyme | active | Lemon & Herb | 65 | 160 | Free Soft Drink (30) |
| `cust-028` | Harper Mellow | inactive | Mild | 15 | 40 | — |
| `cust-029` | Jordan Sizzle | active | Hot | 175 | 560 | Free PERi-PERi Chips (50) |
| `cust-030` | Alexis Torch | active | Extra Hot | 330 | 1100 | Free 4 Wings (80) |
| `cust-031` | Elliot Tang | active | Lemon & Herb | 100 | 300 | Free Soft Drink (30) |
| `cust-032` | Frankie Softly | suspended | Mild | 8 | 12 | — |
| `cust-033` | Gray Roast | active | Hot | 190 | 610 | Chips (50), Soft Drink (30) |
| `cust-034` | Hunter Wildfire | active | Extra Hot | 480 | 2000 | Free Sharing Platter (500) |
| `cust-035` | Indigo Marinade | active | Lemon & Herb | 125 | 390 | Free 4 Wings (80) |
| `cust-036` | Jules Breezy | active | Mild | 45 | 110 | — |
| `cust-037` | Kai Char | active | Hot | 225 | 750 | Free Butterfly Chicken (200) |
| `cust-038` | Lane Ash | active | Extra Hot | 305 | 1050 | Free Chicken Burger (250) |
| `cust-039` | Micah Garden | active | Lemon & Herb | 85 | 240 | Free Soft Drink (30) |
| `cust-040` | Noel Plain | inactive | Mild | 12 | 25 | — |

Source of truth for fields: `seed.go`.

---

## Example JSON request bodies

Use these in Postman / Insomnia / curl. `Content-Type: application/json` on every body request.

### LoyaltyProfile — minimal valid PUT

```json
{
  "customer_id": "cust-demo-01",
  "first_name": "Pat",
  "last_name": "Demo",
  "email": "pat.demo@example.com",
  "account_status": "active",
  "loyalty": {
    "tier": "Hot",
    "points_balance": 100,
    "lifetime_points": 250,
    "rewards_available": []
  }
}
```

### LoyaltyProfile — with reward

```json
{
  "customer_id": "cust-demo-02",
  "first_name": "Nina",
  "last_name": "Heat",
  "email": "nina.heat@example.com",
  "account_status": "active",
  "created_at": "2026-01-15T10:00:00Z",
  "last_activity": "2026-11-01T18:30:00Z",
  "loyalty": {
    "tier": "Extra Hot",
    "points_balance": 420,
    "lifetime_points": 1800,
    "rewards_available": [
      {
        "reward_id": "r-butterfly",
        "name": "Free Butterfly Chicken",
        "points_required": 200
      }
    ]
  }
}
```

### LoyaltyProfile — multiple rewards

```json
{
  "customer_id": "cust-demo-03",
  "first_name": "Omar",
  "last_name": "Flame",
  "email": "omar.flame@example.com",
  "account_status": "active",
  "loyalty": {
    "tier": "Hot",
    "points_balance": 275,
    "lifetime_points": 900,
    "rewards_available": [
      { "reward_id": "r-chips", "name": "Free PERi-PERi Chips", "points_required": 50 },
      { "reward_id": "r-drink", "name": "Free Soft Drink", "points_required": 30 },
      { "reward_id": "r-wings", "name": "Free 4 Wings", "points_required": 80 }
    ]
  }
}
```

### LoyaltyProfile — inactive / mild

```json
{
  "customer_id": "cust-demo-04",
  "first_name": "Priya",
  "last_name": "Cool",
  "email": "priya.cool@example.com",
  "account_status": "inactive",
  "loyalty": {
    "tier": "Mild",
    "points_balance": 18,
    "lifetime_points": 45,
    "rewards_available": []
  }
}
```

### LoyaltyProfile — lemon & herb

```json
{
  "customer_id": "cust-demo-05",
  "first_name": "Lee",
  "last_name": "Zest",
  "email": "lee.zest@example.com",
  "account_status": "active",
  "loyalty": {
    "tier": "Lemon & Herb",
    "points_balance": 95,
    "lifetime_points": 310,
    "rewards_available": [
      { "reward_id": "r-drink", "name": "Free Soft Drink", "points_required": 30 }
    ]
  }
}
```

### LoyaltyProfile — high roller / platter

```json
{
  "customer_id": "cust-demo-06",
  "first_name": "Samira",
  "last_name": "Inferno",
  "email": "samira.inferno@example.com",
  "account_status": "active",
  "loyalty": {
    "tier": "Extra Hot",
    "points_balance": 850,
    "lifetime_points": 4200,
    "rewards_available": [
      { "reward_id": "r-platter", "name": "Free Sharing Platter", "points_required": 500 },
      { "reward_id": "r-burger", "name": "Free Chicken Burger", "points_required": 250 }
    ]
  }
}
```

### LoyaltyProfile — overwrite existing seed key

```json
{
  "customer_id": "cust-001",
  "first_name": "Alex",
  "last_name": "Flame",
  "email": "alex.flame.updated@example.com",
  "account_status": "active",
  "loyalty": {
    "tier": "Extra Hot",
    "points_balance": 500,
    "lifetime_points": 900,
    "rewards_available": [
      { "reward_id": "r-burger", "name": "Free Chicken Burger", "points_required": 250 }
    ]
  }
}
```

### Invalid — empty customer_id (expect 400)

```json
{
  "customer_id": "",
  "first_name": "Bad",
  "last_name": "Data",
  "email": "bad@example.com",
  "account_status": "active",
  "loyalty": {
    "tier": "Hot",
    "points_balance": 1,
    "lifetime_points": 1,
    "rewards_available": []
  }
}
```

### Invalid — not JSON (expect 400)

```text
{not-json
```

### Batch set — two profiles

```json
[
  {
    "key": "batch-a",
    "ttl_seconds": 120,
    "data": {
      "customer_id": "batch-a",
      "first_name": "Ada",
      "last_name": "Batch",
      "email": "ada.batch@example.com",
      "account_status": "active",
      "loyalty": {
        "tier": "Hot",
        "points_balance": 50,
        "lifetime_points": 50,
        "rewards_available": []
      }
    }
  },
  {
    "key": "batch-b",
    "ttl_seconds": 120,
    "data": {
      "customer_id": "batch-b",
      "first_name": "Ben",
      "last_name": "Batch",
      "email": "ben.batch@example.com",
      "account_status": "active",
      "loyalty": {
        "tier": "Mild",
        "points_balance": 20,
        "lifetime_points": 20,
        "rewards_available": []
      }
    }
  }
]
```

### Batch set — default TTL (`ttl_seconds: 0` → 60)

```json
[
  {
    "key": "batch-default-ttl",
    "ttl_seconds": 0,
    "data": {
      "customer_id": "batch-default-ttl",
      "first_name": "Default",
      "last_name": "TTL",
      "email": "default.ttl@example.com",
      "account_status": "active",
      "loyalty": {
        "tier": "Lemon & Herb",
        "points_balance": 33,
        "lifetime_points": 33,
        "rewards_available": []
      }
    }
  }
]
```

### Batch set — mixed valid + invalid (partial apply)

```json
[
  {
    "key": "batch-ok-1",
    "ttl_seconds": 60,
    "data": {
      "customer_id": "batch-ok-1",
      "first_name": "Ok",
      "last_name": "One",
      "email": "ok1@example.com",
      "account_status": "active",
      "loyalty": { "tier": "Hot", "points_balance": 1, "lifetime_points": 1, "rewards_available": [] }
    }
  },
  {
    "key": "",
    "ttl_seconds": 60,
    "data": {
      "customer_id": "will-fail",
      "first_name": "No",
      "last_name": "Key",
      "email": "nokey@example.com",
      "account_status": "active",
      "loyalty": { "tier": "Hot", "points_balance": 1, "lifetime_points": 1, "rewards_available": [] }
    }
  },
  {
    "key": "batch-ok-2",
    "ttl_seconds": 60,
    "data": {
      "customer_id": "batch-ok-2",
      "first_name": "Ok",
      "last_name": "Two",
      "email": "ok2@example.com",
      "account_status": "active",
      "loyalty": { "tier": "Mild", "points_balance": 2, "lifetime_points": 2, "rewards_available": [] }
    }
  }
]
```

### Batch set — negative TTL (expect 400)

```json
[
  {
    "key": "batch-bad-ttl",
    "ttl_seconds": -5,
    "data": {
      "customer_id": "batch-bad-ttl",
      "first_name": "Bad",
      "last_name": "TTL",
      "email": "bad.ttl@example.com",
      "account_status": "active",
      "loyalty": { "tier": "Hot", "points_balance": 1, "lifetime_points": 1, "rewards_available": [] }
    }
  }
]
```

### Batch get — seeds + miss

```json
[
  { "key": "cust-001" },
  { "key": "cust-010" },
  { "key": "cust-040" },
  { "key": "does-not-exist" }
]
```

Example response shape:

```json
[
  { "key": "cust-001", "found": true, "profile": { "customer_id": "cust-001", "first_name": "Alex" } },
  { "key": "cust-010", "found": true, "profile": { "customer_id": "cust-010", "first_name": "Skyler" } },
  { "key": "cust-040", "found": true, "profile": { "customer_id": "cust-040", "first_name": "Noel" } },
  { "key": "does-not-exist", "found": false }
]
```

### Batch get — after you created batch-a / batch-b

```json
[
  { "key": "batch-a" },
  { "key": "batch-b" },
  { "key": "batch-default-ttl" }
]
```

### Batch delete

```json
[
  { "key": "batch-a" },
  { "key": "batch-b" },
  { "key": "batch-ok-1" },
  { "key": "batch-ok-2" }
]
```

### Short-lived entry (expiry demo) — PUT body

```json
{
  "customer_id": "cust-expire",
  "first_name": "Soon",
  "last_name": "Gone",
  "email": "soon.gone@example.com",
  "account_status": "active",
  "loyalty": {
    "tier": "Mild",
    "points_balance": 1,
    "lifetime_points": 1,
    "rewards_available": []
  }
}
```

Use with `PUT /cache/cust-expire?ttl=1`, then `GET` immediately (200) and again after ~2s (404).

---

## Testing with Postman (or Insomnia / Bruno)

### 1. Start the server

```bash
go run .
```

You should see logs like `cache seeded entries=40` and `listening addr=:8080`.

### 2. Create an environment

In Postman: **Environments → Create**.

| Variable | Initial value |
|---|---|
| `baseUrl` | `http://localhost:8080` |

Select that environment for the collection.

### 3. Suggested collection folder structure

```
PeriCache
├── Health
│   └── Healthz
├── Seed reads
│   ├── Get cust-001
│   ├── Get cust-022
│   ├── Size
│   ├── Keys
│   └── Stats
├── Single key CRUD
│   ├── PUT profile (ttl=60)
│   ├── PUT profile (default ttl)
│   ├── GET profile
│   ├── PUT overwrite
│   ├── DELETE profile
│   └── GET missing (404)
├── Validation
│   ├── PUT ttl=0
│   ├── PUT empty customer_id
│   └── PUT invalid JSON
├── Batch
│   ├── POST batch set
│   ├── POST batch get
│   ├── POST batch set partial
│   ├── POST batch bad ttl
│   └── POST batch delete
├── Expiry
│   ├── PUT ttl=1
│   ├── GET immediate
│   └── GET after wait
└── Admin
    ├── Clear cache
    └── Size after clear
```

### 4. Request recipes

Set header `Content-Type: application/json` on every request with a body. Paste bodies from [Example JSON request bodies](#example-json-request-bodies).

| # | Method | URL | Body / notes | Expect |
|---|---|---|---|---|
| 1 | `GET` | `{{baseUrl}}/healthz` | — | 200 `status=ok` |
| 2 | `GET` | `{{baseUrl}}/cache/size` | — | 200 `size=40` (fresh start) |
| 3 | `GET` | `{{baseUrl}}/cache/keys` | — | 200, 40 keys |
| 4 | `GET` | `{{baseUrl}}/cache/cust-001` | — | 200 Alex Flame |
| 5 | `GET` | `{{baseUrl}}/cache/cust-040` | — | 200 Noel Plain |
| 6 | `GET` | `{{baseUrl}}/cache/nope` | — | 404 |
| 7 | `PUT` | `{{baseUrl}}/cache/cust-demo-01?ttl=60` | minimal valid profile | 204 |
| 8 | `GET` | `{{baseUrl}}/cache/cust-demo-01` | — | 200 |
| 9 | `PUT` | `{{baseUrl}}/cache/cust-demo-02` | profile with reward (no `ttl` query) | 204 |
| 10 | `PUT` | `{{baseUrl}}/cache/cust-001?ttl=60` | overwrite seed body | 204; GET shows updated email/tier |
| 11 | `PUT` | `{{baseUrl}}/cache/z?ttl=0` | any valid profile | 400 `invalid TTL` |
| 12 | `PUT` | `{{baseUrl}}/cache/z?ttl=60` | empty `customer_id` body | 400 `no data provided` |
| 13 | `PUT` | `{{baseUrl}}/cache/z?ttl=60` | `{not-json` raw | 400 |
| 14 | `DELETE` | `{{baseUrl}}/cache/cust-demo-01` | — | 204 |
| 15 | `GET` | `{{baseUrl}}/cache/cust-demo-01` | — | 404 |
| 16 | `DELETE` | `{{baseUrl}}/cache/missing-key` | — | 204 (idempotent) |
| 17 | `POST` | `{{baseUrl}}/cache/batch` | batch set two profiles | 204 |
| 18 | `POST` | `{{baseUrl}}/cache/batch/get` | seeds + miss | 200; last item `found:false` |
| 19 | `POST` | `{{baseUrl}}/cache/batch` | `ttl_seconds: 0` body | 204 |
| 20 | `POST` | `{{baseUrl}}/cache/batch` | mixed ok + empty key | 400 `batch partially applied`; ok keys still GET 200 |
| 21 | `POST` | `{{baseUrl}}/cache/batch` | negative `ttl_seconds` | 400 |
| 22 | `POST` | `{{baseUrl}}/cache/batch/delete` | batch delete keys | 204 |
| 23 | `PUT` | `{{baseUrl}}/cache/cust-expire?ttl=1` | short-lived body | 204 |
| 24 | `GET` | `{{baseUrl}}/cache/cust-expire` | immediately | 200 |
| 25 | `GET` | `{{baseUrl}}/cache/cust-expire` | wait ~2 seconds | 404 |
| 26 | `GET` | `{{baseUrl}}/cache/stats` | — | 200; hit/miss/set counters moved |
| 27 | `DELETE` | `{{baseUrl}}/cache` | clear all | 204 |
| 28 | `GET` | `{{baseUrl}}/cache/size` | — | 200 `size=0` |

### 5. Postman tips

- **Tests tab** (optional asserts), e.g. on Healthz:

```javascript
pm.test("status 200", () => pm.response.to.have.status(200));
pm.test("ok", () => pm.expect(pm.response.json().status).to.eql("ok"));
```

- On batch get:

```javascript
const body = pm.response.json();
pm.test("miss is found:false", () => {
  const miss = body.find(x => x.key === "does-not-exist");
  pm.expect(miss.found).to.eql(false);
});
```

- Use **Collection Runner** with the table order above for a smoke pass.
- After **Clear cache** (`DELETE /cache`), re-run `go run .` if you need seed data again (seed only loads at process start).

### 6. curl equivalents (quick)

```bash
curl http://localhost:8080/healthz
curl http://localhost:8080/cache/size
curl http://localhost:8080/cache/cust-001

curl -X PUT "http://localhost:8080/cache/cust-demo-01?ttl=60" \
  -H "Content-Type: application/json" \
  -d "{\"customer_id\":\"cust-demo-01\",\"first_name\":\"Pat\",\"last_name\":\"Demo\",\"email\":\"pat.demo@example.com\",\"account_status\":\"active\",\"loyalty\":{\"tier\":\"Hot\",\"points_balance\":100,\"lifetime_points\":250,\"rewards_available\":[]}}"

curl -X POST http://localhost:8080/cache/batch/get \
  -H "Content-Type: application/json" \
  -d "[{\"key\":\"cust-001\"},{\"key\":\"miss\"}]"

curl -X DELETE http://localhost:8080/cache
```

Same flows work in **Insomnia**, **Bruno**, **Thunder Client**, or the VS Code/Cursor REST client — same method, URL, headers, and JSON bodies.

---
