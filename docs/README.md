# Evelent Database — Documentation

Evelent is an embeddable, encrypted document database with a built-in,
Redis-like in-memory key/value store. It speaks plain HTTP/JSON and ships with
first-class SDKs for **Go** and **TypeScript/JavaScript**.

## Components

| Component | Path | What it is |
|-----------|------|------------|
| Server | `server/` | The database engine + HTTP API |
| In-memory store | `server/internal/db/memstore.go`, `lru.go` | Redis-like KV cache with TTL & persistence |
| Go SDK | `sdk/go/` | Typed Go client |
| TypeScript SDK | `sdk/typescript/` | Typed TS/JS client (axios-based) |
| Desktop client | `client/` | Wails + Vue GUI (MongoDB-Compass style) |

## Documentation map

1. [Getting started](getting-started.md) — build and run in 5 minutes
2. [Configuration](configuration.md) — every flag and environment variable
3. [HTTP API reference](http-api.md) — all endpoints, requests and responses
4. [In-memory KV store](kv-store.md) — the Redis-like store, TTL, eviction, persistence
5. [Data types](data-types.md) — hash, list, and set commands
6. [Pub/Sub](pubsub.md) — the publish/subscribe message bus (SSE)
7. [Working with large volumes](large-data.md) — batching, streaming, indexes, tuning
8. [Go SDK guide](sdk-go.md)
9. [TypeScript/JavaScript SDK guide](sdk-typescript.md)
10. [Architecture](architecture.md) — how it all fits together
11. **[Архитектура (RU)](architecture.ru.md)** — устройство базы простыми словами
12. **[Производительность (RU)](performance.ru.md)** — что крутить, чтобы было быстро
13. **[Терабайтный масштаб (RU)](scale-tb.ru.md)** — Pebble meta, лимиты, флаги для TB

## Feature highlights

- **Document store** — collections of JSON documents, secondary indexes, rich filters.
- **Redis-like KV store** — `SET/GET/DEL`, per-key TTL, atomic counters, batch
  `MSET/MGET/MDEL`, prefix scans, LRU eviction by item count or byte size.
- **Data types** — hashes, lists, and sets with type-safe operations.
- **Pub/Sub** — in-process publish/subscribe over Server-Sent Events.
- **Sharded cache** — striped locks for high concurrent throughput.
- **Encryption at rest** — AES-256-GCM for documents, indexes, and the KV snapshot.
- **Persistence** — the in-memory store snapshots to an encrypted file and reloads on boot.
- **Two SDKs** — Go and TypeScript, kept in sync with the HTTP API.
- **Graceful shutdown** — a final snapshot is flushed on `SIGINT`/`SIGTERM`.
