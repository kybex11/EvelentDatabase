# Configuration

The server is configured through command-line flags and environment variables.

## Command-line flags

| Flag | Default | Description |
|------|---------|-------------|
| `-addr` | `:8080` | TCP address to listen on (e.g. `:8080`, `0.0.0.0:9090`). Ignored if `-port` is set. |
| `-port` | _(empty)_ | Listen port or address: `8080` → `:8080`; `:9090`; `127.0.0.1:3000`. Overrides `-addr`. |
| `-data-dir` | _(exe dir)_`/data` | Directory for database files (collections, indexes, encryption key, KV snapshot). |
| `-gomaxprocs` | `0` | `GOMAXPROCS` value. `0` uses all CPU cores. |
| `-http-read-header-timeout` | `10s` | Max duration for reading request headers. |
| `-http-read-timeout` | `60s` | Max duration for reading the entire request. |
| `-http-write-timeout` | `0` | Max duration before timing out writes. `0` = no timeout. |
| `-http-idle-timeout` | `180s` | Keep-alive idle timeout. |

A bare positional argument is also accepted as the port: `./db.exe 9090`.

### Examples

```bash
./db.exe -port 9090 -data-dir /var/lib/evelent
./db.exe -addr 0.0.0.0:8080 -gomaxprocs 4
./db.exe 3000                       # listen on :3000
```

## Environment variables

| Variable | Description |
|----------|-------------|
| `DB_ENCRYPTION_KEY` | 32-byte AES key as **64 hex characters**. When set, it overrides the on-disk `.key` file. Use this to share one key across replicas or to keep the key out of the data directory. |

Generate a key:

```bash
# Go
go run -exec "" - <<'EOF'
package main
import ("crypto/rand";"encoding/hex";"fmt")
func main(){b:=make([]byte,32);rand.Read(b);fmt.Println(hex.EncodeToString(b))}
EOF

# OpenSSL
openssl rand -hex 32
```

```bash
export DB_ENCRYPTION_KEY=$(openssl rand -hex 32)
./db.exe -port 8080
```

## In-memory store tuning (programmatic)

When embedding the engine directly (not via the binary), open the database with
bounds for the KV store:

```go
database, err := db.NewDatabaseWithOptions("./data", db.MemStoreOptions{
    MaxItems:   1_000_000,        // 0 = unbounded by count
    MaxBytes:   512 << 20,        // 512 MiB; 0 = unbounded by size
    Shards:     0,                // 0 = auto (sized to the machine); 1 = single lock
    FlushEvery: 5 * time.Second,  // background snapshot interval
    SweepEvery: 1 * time.Second,  // background TTL sweep interval
})
```

| Option | Default | Meaning |
|--------|---------|---------|
| `MaxItems` | `0` (unbounded) | Evict LRU entries once the key count exceeds this. |
| `MaxBytes` | `0` (unbounded) | Evict LRU entries once approximate footprint exceeds this. |
| `Shards` | `0` (auto) | Number of striped cache shards. `0` sizes it to the machine (`GOMAXPROCS×4`, power of two, ≤256); `1` forces a single lock. Bounds are split evenly across shards. |
| `FlushEvery` | `5s` | How often a dirty store is snapshotted to disk. |
| `SweepEvery` | `1s` | How often expired keys are actively purged. |

The default binary uses these defaults; the snapshot lives at
`<data-dir>/.kvstore` and is AES-256-GCM encrypted with the same key as documents.

Next: [HTTP API reference](http-api.md).
