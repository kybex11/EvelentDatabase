# Working with large volumes

Strategies and APIs for moving and querying large amounts of data efficiently.

## 1. Batch document inserts

Insert thousands of documents in a single request instead of one HTTP call per
document:

```
POST /api/collections/{c}/docs/batch
{ "documents": [ {...}, {...}, ... ] }
```

```go
ids, _ := client.Collection("events").Documents().InsertMany(batch)
```

Group inserts into chunks (e.g. 1,000–10,000 documents) to balance memory and
round-trips.

## 2. Batch KV operations

The KV store has dedicated batch primitives so you transfer N keys in one
round-trip rather than N requests:

```go
// Write 50k pairs at once
items := make([]sdk.KVItem, 0, 50_000)
for i := 0; i < 50_000; i++ {
    items = append(items, sdk.KVItem{Key: fmt.Sprintf("k:%d", i), Value: "v"})
}
client.KV.MSet(items, 0)

// Read them back in bulk
got, missing, _ := client.KV.MGet(keys)
```

```ts
await db.kv.mset(items, 0);
const { items: got, missing } = await db.kv.mget(keys);
await db.kv.mdel(keys);
```

## 3. Pagination over query results

Use `limit` + `skip` to page through large collections:

```json
{ "filter": {}, "sort": { "field": "_id", "order": 1 }, "limit": 1000, "skip": 0 }
```

Increment `skip` by `limit` each page. Always pair pagination with a stable
`sort` so pages do not overlap or skip rows.

## 4. Indexes

Create a secondary index on any field you filter or sort by frequently:

```
POST /api/collections/{c}/indexes  { "field": "email" }
```

Indexes are persisted (encrypted) and rebuilt incrementally as documents change.
Drop indexes you no longer need to keep writes fast.

## 5. Server tuning

| Setting | Effect |
|---------|--------|
| `-gomaxprocs` | Match to available cores for CPU-bound workloads. |
| `-http-read-timeout` | Raise for very large request bodies. |
| `-http-write-timeout 0` | Keep `0` for large streamed responses. |
| `MaxBytes` / `MaxItems` | Cap KV memory so it never exhausts the host. |

## 6. Memory bounds for the KV store

For large hot sets, bound the store by bytes so it self-evicts instead of
growing without limit:

```go
db.NewDatabaseWithOptions("./data", db.MemStoreOptions{
    MaxBytes:   2 << 30,         // 2 GiB ceiling
    FlushEvery: 10 * time.Second,
})
```

Watch `Stats()` — a rising `evictions` count with a falling `hitRatio` means the
working set no longer fits and the bound (or host memory) should grow.

## 7. Connection reuse

Both SDKs and the desktop client use a pooled HTTP transport
(`MaxIdleConnsPerHost` in the hundreds/thousands). Reuse a single client
instance across goroutines/requests rather than creating one per call.

## Performance notes & tradeoffs

- The KV store uses a **sharded (striped) cache**: keys are hashed across N
  independent LRU shards, each with its own lock, so operations on keys in
  different shards never contend. Shard count defaults to `GOMAXPROCS×4`
  (power of two, capped at 256). Set `Shards: 1` for a single global lock.
- Item/byte bounds are divided evenly across shards, so the effective ceiling is
  approximate (rounding can let totals drift slightly above the configured cap).
- **Indexed equality queries skip the full scan**: a `find` whose filter is a
  single equality on an indexed field (`{"city":"LA"}` or `{"city":{"$eq":"LA"}}`)
  loads only the matching documents by ID via the index. Create an index on the
  field first. Range/`$in`/multi-field filters still scan.
- Byte accounting is approximate and intended for ceilings, not exact metering.
- Container types (hash/list/set) serialize their read-modify-write operations
  through one op-mutex and copy-on-write, trading some throughput for atomicity
  and freedom from data races.

## Benchmarks

Cache benchmarks live in `server/internal/db/bench_test.go`:

```bash
cd server
go test ./internal/db -bench . -benchmem            # all benchmarks
go test ./internal/db -bench Parallel -cpu 1,4,8     # scaling with cores
```

Representative results (8 cores, Intel i5-11600) showing the sharded cache vs a
single global lock:

| Benchmark | Single LRU | Sharded LRU | Speedup |
|-----------|-----------:|------------:|:-------:|
| Parallel GET | ~125 ns/op | ~46 ns/op | ~2.7× |
| Parallel SET | ~173 ns/op | ~99 ns/op | ~1.7× |
| Mixed 80/20 | ~141 ns/op | ~54 ns/op | ~2.6× |

Numbers vary by machine; run them on your target hardware.

Next: [Go SDK guide](sdk-go.md).
