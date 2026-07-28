# Configuration

The server is configured through command-line flags and environment variables.

## Command-line flags

| Flag | Default | Description |
|------|---------|-------------|
| `-addr` | `127.0.0.1:8080` | TCP address to listen on. **Defaults to localhost.** Use `0.0.0.0:port` only behind a reverse proxy with an API key. |
| `-port` | _(empty)_ | Listen port or address: `8080` → `127.0.0.1:8080`; `0.0.0.0:9090`. Overrides `-addr`. |
| `-data-dir` | _(exe dir)_`/data` | Directory for database files (collections, indexes, encryption key, KV snapshot). |
| `-gomaxprocs` | `0` | `GOMAXPROCS` value. `0` uses all CPU cores. |
| `-http-read-header-timeout` | `10s` | Max duration for reading request headers. |
| `-http-read-timeout` | `60s` | Max duration for reading the entire request. |
| `-http-write-timeout` | `0` | Max duration before timing out writes. `0` = no timeout. |
| `-http-idle-timeout` | `180s` | Keep-alive idle timeout. |
| `-api-key` | _(empty)_ | API key for authenticating `/api/*` requests. Also reads `DB_API_KEY`. **Required when binding a non-loopback address** (unless `-allow-insecure-open`). |
| `-allow-insecure-open` | `false` | Allow a non-loopback bind with no API key. Dangerous — only for controlled tests. |
| `-cors-origins` | _(empty)_ | Comma-separated allowed CORS origins. Empty = no browser CORS. Use an explicit origin in production; `*` allows all. |
| `-max-body-size` | `33554432` (32 MiB) | Maximum request body in bytes. Oversized → `413`. |
| `-rate-limit` | `200` | Token-bucket rate: requests per second per IP. |
| `-rate-burst` | `500` | Token-bucket burst (max tokens accumulated per IP). |
| `-sync-mode` | `none` | Document segment durability: `none` (fastest), `every_sec` (fsync ~1s), `every_write` (fsync each append). |

A bare positional argument is also accepted as the port: `./db.exe 9090` → `127.0.0.1:9090`.

### Examples

```bash
# Local only (safe default)
./db.exe -port 2026 -data-dir ./data

# Public / proxied — MUST set an API key
./db.exe -addr 0.0.0.0:7027 -api-key "$(openssl rand -hex 32)" -cors-origins "https://yourdomain.com"

# Behind nginx on the same host (recommended for domain.com/db)
./db.exe -addr 127.0.0.1:7027 -api-key "$DB_API_KEY" -cors-origins "https://domain.com"
```

## Fail-closed bind rules

| Bind | API key | Result |
|------|---------|--------|
| `127.0.0.1` / `::1` | missing | Starts (loopback-only notice) |
| `0.0.0.0` / `:port` / public IP | missing | **Refuses to start** |
| any non-loopback | missing + `-allow-insecure-open` | Starts with WARNING |
| any | set | Auth middleware on `/api/*` |

Prefer `X-API-Key` header. `?key=` still works but is **deprecated** (leaks into access logs).

## Environment variables

| Variable | Description |
|----------|-------------|
| `DB_ENCRYPTION_KEY` | 32-byte AES key as **64 hex characters**. When set, it overrides the on-disk `.key` file. |
| `DB_API_KEY` | API key for authenticating requests (same as `-api-key` flag, flag takes precedence). |

Generate a key:

```bash
openssl rand -hex 32
```

```bash
export DB_ENCRYPTION_KEY=$(openssl rand -hex 32)
export DB_API_KEY=$(openssl rand -hex 32)
./db.exe -port 8080
```

## In-memory store tuning (programmatic)

When embedding the engine directly (not via the binary), open the database with
bounds for the KV store:

```go
database, err := db.OpenDatabase("./data", db.DatabaseOptions{
    SyncMode: db.SyncEverySecond, // none | every_sec | every_write
    KV: db.MemStoreOptions{
        MaxItems:   1_000_000,        // 0 = unbounded by count
        MaxBytes:   512 << 20,        // 512 MiB; 0 = unbounded by size
        Shards:     0,                // 0 = auto (sized to the machine); 1 = single lock
        FlushEvery: 5 * time.Second,  // background snapshot interval
        SweepEvery: 1 * time.Second,  // background TTL sweep interval
    },
})
```

| Option | Default | Meaning |
|--------|---------|---------|
| `SyncMode` | `SyncNone` | Segment fsync policy (`none` / `every_sec` / `every_write`). |
| `MaxItems` | `0` (unbounded) | Evict LRU entries once the key count exceeds this. |
| `MaxBytes` | `0` (unbounded) | Evict LRU entries once approximate footprint exceeds this. |
| `Shards` | `0` (auto) | Number of striped cache shards. `0` sizes it to the machine (`GOMAXPROCS×4`, power of two, ≤256); `1` forces a single lock. Bounds are split evenly across shards. |
| `FlushEvery` | `5s` | How often a dirty store is snapshotted to disk. |
| `SweepEvery` | `1s` | How often expired keys are actively purged. |

The default binary uses these defaults; the snapshot lives at
`<data-dir>/.kvstore` and is AES-256-GCM encrypted with the same key as documents.

Russian guides: [architecture.ru.md](architecture.ru.md) · [performance.ru.md](performance.ru.md).

Next: [HTTP API reference](http-api.md) · [Large volumes](large-data.md).
