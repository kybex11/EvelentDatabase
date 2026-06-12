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
                     │  - documents  │    │  - LRU + TTL      │
                     │  - indexes    │    │  - batch ops      │
                     │  - find/query │    │  - snapshot       │
                     └───────┬───────┘    └────────┬──────────┘
                             │                     │
                   AES-256-GCM at rest    AES-256-GCM snapshot
                   (docs + indexes)        (<data-dir>/.kvstore)
```

## Server (`server/`)

- `main.go` — flag parsing, routing, graceful shutdown.
- `handlers/` — HTTP handlers (`api.go`, `kv.go`, `health.go`).
- `internal/db/` — the engine:
  - `db.go` — `Database`, collection registry, KV lifecycle.
  - `collection.go` — document CRUD, find, stats.
  - `index.go`, `query.go`, `filter_eq.go`, `findopts.go`, `projection.go` — querying.
  - `atrest.go` — AES-256-GCM encryption + atomic file writes.
  - `lru.go` — the cache primitive.
  - `shardedlru.go` — striped multi-shard cache for concurrency.
  - `memstore.go`, `memstore_ops.go` — the KV store.
  - `shard.go` — document path sharding helpers.

## Data layout (on disk)

```
<data-dir>/
├── .key                     # 32-byte AES key (unless DB_ENCRYPTION_KEY is set)
├── .kvstore                 # encrypted KV snapshot
└── <collection>/
    ├── docs/<id>.json       # encrypted documents
    └── indexes/<field>.idx  # encrypted index
```

## Request lifecycle

1. The HTTP server receives a request; CORS and method routing happen in `main.go`.
2. The handler resolves the collection (lazily created/loaded) or the KV store.
3. Reads/writes go through AES-GCM encode/decode and atomic file writes.
4. KV writes mark the store dirty; a background loop snapshots it, and a final
   snapshot is written on shutdown.

## Concurrency model

- `Database` guards its collection map with an `RWMutex`.
- Each `Collection` guards its files/indexes with an `RWMutex`.
- The KV store uses a **sharded cache** (`ShardedLRU`): keys are striped across
  N independent `LRU` shards, each with its own mutex, so concurrent operations
  on different shards never contend. A single-shard mode (`Shards: 1`) keeps
  exact global eviction ordering when that matters more than throughput.

## SDKs

Both SDKs are thin, typed wrappers over the same HTTP API and are kept in sync:

- **Go** (`sdk/go`) — `net/http` with a pooled transport.
- **TypeScript** (`sdk/typescript`) — `axios`, modular endpoint classes.
