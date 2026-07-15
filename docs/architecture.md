# Architecture

```
                         ┌─────────────────────────────┐
   Go SDK  ─────────────▶│                             │
   TS SDK  ─────HTTP─────▶│   HTTP API (handlers/)      │
   Client  ─────────────▶│                             │
                         └──────────────┬─────────── ───┘
                                        │
                              ┌─────────▼──────────┐
                              │  Database (db.go)  │
                              └───┬────────────┬───┘
                                  │            │
                     ┌────────────▼──┐    ┌────▼──────────────┐
                     │ Collections   │    │ MemStore (KV)     │
                     │  - segments   │    │  - LRU + TTL      │
                     │  - docindex   │    │  - batch ops      │
                     │  - indexes    │    │  - snapshot       │
                     └───────┬───────┘    └────────┬──────────┘
                             │                     │
                   AES-256-GCM at rest    AES-256-GCM snapshot
                   (segs + indexes)        (<data-dir>/.kvstore)
```

## Server (`server/`)

- `main.go` — flag parsing, fail-closed bind/API-key rules, routing, shutdown.
- `middleware.go` — API key auth, CORS, rate limit, body size, security headers.
- `handlers/` — HTTP handlers (`api.go`, `kv.go`, `health.go`, …).
- `internal/db/` — the engine:
  - `db.go` — `Database`, collection registry, KV lifecycle.
  - `collection.go` — document CRUD, find, stats over segments + legacy files.
  - `segment.go` / `docindex.go` — append-only segments and id→offset map.
  - `shard.go` — hex path helpers for legacy documents.
  - `index.go`, `query.go`, `filter_eq.go`, `findopts.go`, `projection.go` — querying.
  - `atrest.go` — AES-256-GCM encryption + atomic file writes.
  - `lru.go` / `shardedlru.go` / `memstore*.go` — KV store.

## Data layout (on disk)

```
<data-dir>/
├── .key                     # 32-byte AES key (unless DB_ENCRYPTION_KEY is set)
├── .kvstore                 # encrypted KV snapshot
└── <collection>/
    ├── .docindex            # encrypted id → {seg, offset, length}
    ├── segments/NNNNNN.seg  # append-only encrypted document payloads
    ├── docs/                # legacy per-doc files (flat or hex-sharded)
    │   └── ab/<id>.json
    └── indexes/<field>.idx  # encrypted secondary index
```

New writes go to segments. Reads check `.docindex` first, then legacy paths.

## Request lifecycle

1. Middleware: security headers → CORS → rate limit → max body → API key → mux.
2. Handler resolves the collection or KV store.
3. Document writes append encrypted payloads to the current segment and update `.docindex`.
4. KV writes mark the store dirty; a background loop snapshots; collections flush
   indexes/docindex on `Close`.

## Concurrency model

- `Database` guards its collection map with an `RWMutex`.
- Each `Collection` guards segments/indexes with an `RWMutex`.
- The KV store uses a **sharded cache** (`ShardedLRU`).

## Security (defaults)

- Listen on `127.0.0.1` by default.
- Non-loopback bind without an API key → process exits (unless `-allow-insecure-open`).
- Prefer `X-API-Key`; `?key=` is deprecated.

## SDKs

- **Go** (`sdk/go`) — `sdk.New(url, sdk.WithAPIKey(...))`.
- **TypeScript** (`sdk/typescript`) — `new EvelentClient(url, { apiKey })`.
