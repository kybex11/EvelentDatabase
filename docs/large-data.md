# Working with large volumes

Strategies for storing and querying tens to hundreds of gigabytes efficiently.

## Honest limits

| Layer | Where it lives | Scale note |
|-------|----------------|------------|
| Document store | Disk (append-only segments) | Designed for **150–500+ GiB** of documents on one host |
| Secondary indexes | RAM + encrypted sidecar | Cardinality of indexed values must fit in memory |
| KV / hash / list / set | RAM (+ snapshot) | Bound with `MaxBytes` / `MaxItems`; not for multi-hundred-GB working sets |
| Binary size (~9 MiB) | N/A | Unrelated to data size (MongoDB’s ~2 GiB installer is the engine + tools) |

## Storage layout (documents)

New writes go into **append-only segment files** (~64 MiB each) plus an encrypted
`.docindex` map (`id → segment/offset/length`). Legacy per-file documents under
`docs/` (flat or hex-sharded `docs/ab/id.json`) remain readable.

```
<data-dir>/<collection>/
├── .docindex              # encrypted id → location map
├── segments/000001.seg    # append-only encrypted payloads
├── docs/…                 # legacy files (still read; writes prefer segments)
└── indexes/<field>.idx
```

This avoids millions of tiny files (the usual filesystem cliff at huge
collections) while keeping O(1) reads by `_id`.

## 1. Batch document inserts

```
POST /api/collections/{c}/docs/batch
{ "documents": [ {...}, {...}, ... ] }
```

Group inserts into chunks (e.g. 1,000–10,000 documents) to balance memory and
round-trips.

## 2. Keyset (cursor) pagination — prefer over skip

`skip` on huge collections is O(n). Prefer `after` / `cursor`:

```json
{ "filter": {}, "limit": 1000, "after": "<last_id>" }
```

Or pass back `nextCursor` from the previous response:

```json
{ "filter": {}, "limit": 1000, "cursor": "<nextCursor>" }
```

Response shape:

```json
{ "documents": [ ... ], "nextCursor": "..." }
```

```go
page, _ := client.Collection("events").Documents().FindPage(sdk.FindQuery{
    "limit": 1000,
    "cursor": prev,
})
```

```ts
const page = await db.collection("events").documents().findPage({ limit: 1000, cursor });
```

Unsorted scans stream by `_id` order and stop at `limit` without loading the
whole collection into RAM. Sorted finds still collect then sort (capped).

## 3. Indexes

Create a secondary index on fields you filter by:

```
POST /api/collections/{c}/indexes  { "field": "email" }
```

- **Equality** `{ "email": "…" }` — O(1) hash lookup, then segment reads.
- **Range** `{ "age": { "$gte": 18, "$lt": 65 } }` — uses sorted index buckets
  when an index exists on that field.
- Without an index, Find decrypts every live document.

Also see `-sync-mode` (`none` / `every_sec` / `every_write`) and Russian guides:
[architecture.ru.md](architecture.ru.md), [performance.ru.md](performance.ru.md).

## 4. Batch KV operations

Hot caches belong in KV with a byte ceiling — not as a substitute for the
document store at hundreds of GB.

## 5. Server tuning

| Setting | Effect |
|---------|--------|
| `-gomaxprocs` | Match available cores |
| `-sync-mode` | `none` / `every_sec` / `every_write` — durability vs write speed |
| `-http-read-timeout` | Raise for very large request bodies |
| `-http-write-timeout 0` | Keep `0` for large responses |
| `MaxBytes` / `MaxItems` | Cap KV memory |

## 6. Production fronting

Keep the DB on loopback; terminate TLS on the proxy:

```nginx
location /db/ {
    proxy_pass http://127.0.0.1:7027/;
    proxy_set_header X-API-Key $http_x_api_key;
}
```

```bash
./db.exe -addr 127.0.0.1:7027 -api-key "$DB_API_KEY" -cors-origins "https://domain.com"
```

Never expose an unauthenticated non-loopback bind. The server refuses that by
default (see [Configuration](configuration.md)).

## Performance notes

- Segment + docindex: point gets stay O(1) disk reads regardless of collection size.
- Full collection scans iterate live IDs (and legacy files); use indexes + cursors.
- KV uses a **sharded LRU** for concurrent small-key workloads.
- Secondary indexes are in-memory maps — plan RAM accordingly.

```bash
cd server && go test ./internal/db -bench . -benchmem
```

Next: [Go SDK guide](sdk-go.md).
