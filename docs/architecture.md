# Architecture

```
                         ┌─────────────────────────────┐
   Go SDK  ─────────────▶│                             │
   TS SDK  ─────HTTP─────▶│   HTTP API (handlers/)      │
   Client  ─────────────▶│                             │
                         └──────────────┬──────────────┘
                                        │
                              ┌─────────▼──────────┐
                              │  Database (db.go)  │
                              └───┬────────────┬───┘
                                  │            │
                     ┌────────────▼──┐    ┌────▼──────────────┐
                     │ Collections   │    │ MemStore (KV)     │
                     │  - segments   │    │  - sharded LRU    │
                     │  - docindex   │    │  - striped RMW    │
                     │  - indexes    │    │  - snapshot       │
                     │  - doc cache  │    └────────┬──────────┘
                     └───────┬───────┘             │
                             │                     │
                   AES-256-GCM at rest    AES-256-GCM snapshot
                   (segs + indexes)        (<data-dir>/.kvstore)
```

> По-русски, максимально просто: [architecture.ru.md](architecture.ru.md) ·
> [performance.ru.md](performance.ru.md).

## Server (`server/`)

- `main.go` — flags (`-sync-mode`, bind/API-key), routing, shutdown.
- `middleware.go` — API key auth, CORS, rate limit, body size, security headers.
- `handlers/` — HTTP handlers.
- `internal/db/` — engine:
  - `collection.go` — CRUD, Find (equality + **range** index paths), compaction, hot cache.
  - `segment.go` — append-only segments, FD cache, read buffer pool.
  - `docindex.go` — id→location map; binary on-disk format (`EDIX`) with JSON fallback.
  - `index.go` — equality sets + sorted numeric/string buckets for ranges.
  - `durability.go` — `SyncNone` / `SyncEverySecond` / `SyncEveryWrite`.
  - `memstore*.go`, `lru.go`, `shardedlru.go` — KV store.
  - `atrest.go` — AES-GCM with reused AEAD instances.

## Data layout (on disk)

```
<data-dir>/
├── .key                     # 32-byte AES key (unless DB_ENCRYPTION_KEY is set)
├── .kvstore                 # encrypted KV snapshot
└── <collection>/
    ├── .docindex            # encrypted id → {seg, offset, length} (binary)
    ├── segments/NNNNNN.seg  # append-only encrypted document payloads
    ├── docs/                # legacy per-doc files (still readable)
    └── indexes/<field>.idx  # encrypted secondary index
```

## Write path (documents)

1. JSON → AES-GCM → append to current segment (optional fsync per `-sync-mode`).
2. Update in-memory docindex + secondary indexes.
3. Background ~1s flush of dirty meta; always flush on `Close`.
4. Updates leave orphaned bytes until **compaction**.

## Read path

1. Hot document LRU (per collection).
2. Docindex lookup → cached segment FD → decrypt.
3. Find: equality index → range index → full scan fallback.

## Concurrency

- Collection: `RWMutex` around document/index/segment state.
- KV: sharded LRU + per-key striped locks for RMW (INCR/HSET/…).

## Durability

| `-sync-mode` | Behavior |
|--------------|----------|
| `none` | Fastest; no per-write fsync |
| `every_sec` | Fsync dirty segments from the 1s loop |
| `every_write` | Fsync after each document append |
