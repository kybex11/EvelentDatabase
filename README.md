# Evelent DB

An embeddable, encrypted document database with a built-in, Redis-like
in-memory store. It speaks plain HTTP/JSON, ships first-class SDKs for **Go**
and **TypeScript/JavaScript**, and comes with a desktop GUI client.

No external services. No separate cache process. One binary holds the document
store, the key/value store, the data types, and the pub/sub bus.

---

## Features

- **Document store** — collections of JSON documents, secondary indexes, rich
  filters (`$eq`, `$ne`, `$gt`, `$gte`, `$lt`, `$lte`, `$in`), sorting,
  pagination, and index-accelerated equality queries.
- **Redis-like KV store** — `SET/GET/DEL`, per-key TTL, atomic counters,
  `SETNX`, `APPEND`, batch `MSET/MGET/MDEL`, prefix scans, LRU eviction by item
  count or byte size.
- **Data types** — hashes, lists, and sets with type-safe operations and
  `WRONGTYPE` protection.
- **Pub/Sub** — an in-process publish/subscribe bus delivered over
  Server-Sent Events.
- **Sharded cache** — striped locks for high concurrent throughput
  (≈2.7× faster reads than a single lock on 8 cores).
- **Encryption at rest** — AES-256-GCM for documents, indexes, and the KV snapshot.
- **Persistence** — the in-memory store snapshots to an encrypted file and
  reloads on boot; a final snapshot is flushed on graceful shutdown.
- **Two SDKs + a GUI** — Go and TypeScript clients kept in sync with the API,
  plus a Wails/Vue desktop app.

---

## Quick start

```bash
# 1. Run the server
cd server
go build -o db.exe .
./db.exe -port 8080 -data-dir ./data

# 2. Try it
curl -X POST http://127.0.0.1:8080/api/collections -d '{"name":"users"}'
curl -X POST http://127.0.0.1:8080/api/collections/users/docs -d '{"name":"Ada"}'
curl -X PUT  http://127.0.0.1:8080/api/kv/greeting -d '{"value":"hi","ttlSeconds":60}'
curl         http://127.0.0.1:8080/api/kv/greeting
```

### Go SDK

```go
client := sdk.New("http://127.0.0.1:8080")
client.KV.Set("greeting", "hello", 60)
client.KV.HSet("user:1", "name", "Ada")
client.KV.RPush("queue", "a", "b")
```

### TypeScript SDK

```ts
import { EvelentClient } from "@evelent/db-sdk";

const db = new EvelentClient("http://127.0.0.1:8080");
await db.kv.set("greeting", "hello", 60);
await db.hash.set("user:1", "name", "Ada");
db.pubsub.subscribe("room:1", (msg) => console.log(msg));
```

---

## Project layout

```
EvelentDatabase/
├── server/              # the database engine + HTTP API
│   ├── main.go          # flags, routing, graceful shutdown
│   ├── handlers/        # HTTP handlers (collections, kv, datatypes, pubsub)
│   └── internal/db/     # engine: collections, indexes, LRU/sharded cache,
│                        #         memstore, data types, pub/sub, encryption
├── sdk/
│   ├── go/              # Go SDK  (module evelent.dev/db/sdk)
│   └── typescript/      # TypeScript SDK (@evelent/db-sdk)
├── client/              # Wails + Vue desktop GUI
└── docs/                # full documentation (see below)
```

---

## Documentation

Full docs live in [`docs/`](docs/README.md):

| Guide | What it covers |
|-------|----------------|
| [Getting started](docs/getting-started.md) | Build and run in 5 minutes |
| [Configuration](docs/configuration.md) | Every flag and environment variable |
| [HTTP API reference](docs/http-api.md) | All endpoints |
| [KV store](docs/kv-store.md) | TTL, eviction, persistence |
| [Data types](docs/data-types.md) | Hash, list, set commands |
| [Pub/Sub](docs/pubsub.md) | The SSE message bus |
| [Large volumes](docs/large-data.md) | Batching, indexes, tuning, benchmarks |
| [Go SDK](docs/sdk-go.md) / [TS SDK](docs/sdk-typescript.md) | Client guides |
| [Architecture](docs/architecture.md) | How it all fits together |

---

## Performance

Sharded cache vs a single global lock (8 cores, Intel i5-11600):

| Benchmark | Single LRU | Sharded LRU | Speedup |
|-----------|-----------:|------------:|:-------:|
| Parallel GET | ~125 ns/op | ~46 ns/op | ~2.7× |
| Parallel SET | ~173 ns/op | ~99 ns/op | ~1.7× |
| Mixed 80/20 | ~141 ns/op | ~54 ns/op | ~2.6× |

```bash
cd server && go test ./internal/db -bench . -benchmem
```

Numbers vary by machine — run them on your target hardware.

---

## Security notes

- The HTTP API has **no built-in authentication** — run it on a trusted network
  or behind an authenticating reverse proxy. Do not expose it directly to the
  public internet.
- Encryption at rest uses a key from `DB_ENCRYPTION_KEY` (64 hex chars) or an
  auto-generated `.key` in the data directory. Back up the key — without it the
  data is unrecoverable.

---

## Requirements

- Go 1.21+ (server and Go SDK)
- Node.js 18+ (TypeScript SDK and desktop client frontend)
