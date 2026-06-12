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
5. [Working with large volumes](large-data.md) — batching, streaming, indexes, tuning
6. [Go SDK guide](sdk-go.md)
7. [TypeScript/JavaScript SDK guide](sdk-typescript.md)
8. [Architecture](architecture.md) — how it all fits together

## Feature highlights

- **Document store** — collections of JSON documents, secondary indexes, rich filters.
- **Redis-like KV store** — `SET/GET/DEL`, per-key TTL, atomic counters, batch
  `MSET/MGET/MDEL`, prefix scans, LRU eviction by item count or byte size.
- **Encryption at rest** — AES-256-GCM for documents, indexes, and the KV snapshot.
- **Persistence** — the in-memory store snapshots to an encrypted file and reloads on boot.
- **Two SDKs** — Go and TypeScript, kept in sync with the HTTP API.
- **Graceful shutdown** — a final snapshot is flushed on `SIGINT`/`SIGTERM`.
