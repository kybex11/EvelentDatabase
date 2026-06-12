# In-memory KV store (Redis-like)

Evelent ships with an embedded, Redis-like key/value store. It is **not** Redis:
there is no separate process, no network protocol of its own, and no external
dependency. It lives inside the database engine and is reached through the same
HTTP API and SDKs.

## Why it exists

- A hot cache in front of the document store.
- Counters, rate limits, feature flags, session data.
- Anything that wants microsecond reads with optional expiry.

## Design

```
HTTP API ─▶ MemStore ─▶ LRU (container/list + map, single mutex)
                │
                └─▶ encrypted snapshot on disk  (<data-dir>/.kvstore)
```

- **LRU core** (`lru.go`) — a concurrency-safe cache bounded by item count
  and/or byte size, with per-entry TTL, hit/miss/eviction counters and
  snapshot/restore.
- **MemStore** (`memstore.go`, `memstore_ops.go`) — the Redis-like surface on
  top of the LRU, plus background TTL sweeping and periodic persistence.

## Commands

| Command | Description |
|---------|-------------|
| `Set(key, value, ttl)` | Store a string, optional expiry. |
| `SetNX(key, value, ttl)` | Store only if absent. |
| `GetSet(key, value, ttl)` | Set and return the previous value. |
| `Get(key)` | Read a string. |
| `Delete(key)` | Remove a key. |
| `Exists(key)` | Presence check. |
| `Append(key, value)` | Concatenate, returns new length. |
| `Incr(key, delta)` / `Decr` | Atomic integer arithmetic. |
| `IncrByFloat(key, delta)` | Atomic float arithmetic. |
| `TTL(key)` | Remaining lifetime. |
| `Expire(key, ttl)` | Set/clear expiry. |
| `Keys(prefix)` | List keys, optional prefix filter. |
| `MSet / MGet / DeleteMany` | Batch operations (see [large data](large-data.md)). |
| `Flush()` | Drop everything. |
| `Stats()` | Cache counters. |

## TTL semantics

- TTL is stored as an absolute expiry timestamp (unix nanoseconds).
- Expired keys are removed **lazily** on access and **actively** by a background
  sweeper (`SweepEvery`, default `1s`).
- After a restart, TTLs are restored from the snapshot using absolute expiry, so
  keys that lapsed while the server was down do not come back.

## Eviction

When `MaxItems` or `MaxBytes` is exceeded, the least-recently-used entries are
evicted first. Each `Get`/`Set` moves the entry to the front of the list. Both
bounds can be active at once; either being exceeded triggers eviction.

```go
db.MemStoreOptions{ MaxItems: 1_000_000, MaxBytes: 512 << 20 }
```

A value of `0` disables that bound. Byte accounting is approximate
(`len(key) + len(value) + overhead`).

## Persistence

- The store snapshots to `<data-dir>/.kvstore` every `FlushEvery` (default `5s`)
  **only when dirty**, and once more on graceful shutdown.
- The snapshot is AES-256-GCM encrypted with the same key as documents. If no
  key is configured it falls back to plaintext JSON.
- On boot, the snapshot is loaded and expired entries are skipped.

## Quick reference (SDKs)

```go
client.KV.Set("session:42", token, 3600)
client.KV.Incr("rate:ip:1.2.3.4", 1)
n, _ := client.KV.MSet([]sdk.KVItem{{Key:"a",Value:"1"}}, 0)
```

```ts
await db.kv.set("session:42", token, 3600);
await db.kv.incr("rate:ip:1.2.3.4", 1);
await db.kv.mset([{ key: "a", value: "1" }], 0);
```

Next: [Working with large volumes](large-data.md).
