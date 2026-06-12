# HTTP API reference

Base URL: `http://<host>:<port>`. All request and response bodies are JSON
unless noted. CORS is enabled for all origins.

## Health

| Method | Path | Response |
|--------|------|----------|
| `GET` | `/health` | `200` text body `ok` |

## Collections

| Method | Path | Body | Response |
|--------|------|------|----------|
| `GET` | `/api/collections` | — | `["users","logs"]` |
| `POST` | `/api/collections` | `{"name":"users"}` | `201 Created` |
| `DELETE` | `/api/collections/{name}` | — | `204 No Content` |

## Documents

| Method | Path | Body | Response |
|--------|------|------|----------|
| `POST` | `/api/collections/{c}/docs` | a JSON document | `{"_id":"..."}` |
| `POST` | `/api/collections/{c}/docs/batch` | `{"documents":[...]}` | `{"ids":[...],"inserted":N}` |
| `GET` | `/api/collections/{c}/docs/{id}` | — | the document |
| `PUT` | `/api/collections/{c}/docs/{id}` | replacement document | `200 OK` |
| `DELETE` | `/api/collections/{c}/docs/{id}` | — | `204 No Content` |
| `POST` | `/api/collections/{c}/find` | query (below) | `[ ...documents ]` |

If a document has no `_id`, one is generated (UUID) and returned.

### Find query

```json
{
  "filter": { "age": { "$gte": 18 }, "active": true },
  "limit": 100,
  "skip": 0,
  "sort": { "field": "age", "order": -1 }
}
```

Supported filter operators: `$eq`, `$ne`, `$gt`, `$gte`, `$lt`, `$lte`, `$in`.
A bare value (e.g. `"active": true`) is shorthand for `$eq`. `order: -1` sorts
descending, `1` ascending.

## Indexes

| Method | Path | Body | Response |
|--------|------|------|----------|
| `POST` | `/api/collections/{c}/indexes` | `{"field":"name"}` | `201 Created` |
| `GET` | `/api/collections/{c}/indexes` | — | `["name","email"]` |
| `DELETE` | `/api/collections/{c}/indexes/{field}` | — | `204 No Content` |

## Collection stats

| Method | Path | Response |
|--------|------|----------|
| `GET` | `/api/collections/{c}/stats` | `{"count":N,"storageSize":bytes}` |

## In-memory KV store

### Per-key operations

| Method | Path | Body | Response |
|--------|------|------|----------|
| `GET` | `/api/kv/{key}` | — | `{"key":"k","value":"v"}` or `404` |
| `PUT` | `/api/kv/{key}` | `{"value":"v","ttlSeconds":60,"nx":false}` | `200 OK` (or `409` if `nx` and present) |
| `DELETE` | `/api/kv/{key}` | — | `204` or `404` |
| `GET` | `/api/kv/{key}/ttl` | — | `{"persists":false,"ttlSeconds":59.5}` |
| `POST` | `/api/kv/{key}/incr` | `{"delta":1}` | `{"value":42}` |
| `POST` | `/api/kv/{key}/expire` | `{"ttlSeconds":120}` | `200 OK` (or `404`) |

- `ttlSeconds <= 0` stores/keeps the key with no expiry.
- `nx: true` makes the write conditional — it only succeeds if the key is absent.
- `incr` creates the key at `0` when absent; a non-integer value yields `409`.

### Collection-level operations

| Method | Path | Body | Response |
|--------|------|------|----------|
| `GET` | `/api/kv/keys?prefix=` | — | `{"keys":[...]}` |
| `GET` | `/api/kv/stats` | — | cache counters (below) |
| `POST` | `/api/kv/flush` | — | `204 No Content` |
| `POST` | `/api/kv/mget` | `{"keys":[...]}` | `{"items":[{key,value}],"missing":[...]}` |
| `POST` | `/api/kv/mset` | `{"items":[{key,value}],"ttlSeconds":0}` | `{"written":N}` |
| `POST` | `/api/kv/mdel` | `{"keys":[...]}` | `{"deleted":N}` |

### Stats payload

```json
{
  "items": 1240,
  "bytes": 81920,
  "maxItems": 0,
  "maxBytes": 0,
  "hits": 9981,
  "misses": 19,
  "evictions": 0,
  "expired": 5,
  "hitRatio": 0.998
}
```

## Data types (hash / list / set)

Full details in [data-types.md](data-types.md). Summary:

| Method | Path | Purpose |
|--------|------|---------|
| `PUT` | `/api/hash/{key}/{field}` | set hash field |
| `GET` | `/api/hash/{key}` | get whole hash |
| `POST` | `/api/hash/{key}/{field}/incr` | atomic field increment |
| `POST` | `/api/list/{key}/lpush` \| `/rpush` | push to a list |
| `POST` | `/api/list/{key}/lpop` \| `/rpop` | pop from a list |
| `GET` | `/api/list/{key}?start=&stop=` | list range |
| `POST` | `/api/set/{key}/add` \| `/rem` | add/remove set members |
| `GET` | `/api/set/{key}` | set members |

A type mismatch (e.g. `GET /api/kv/{key}` on a hash key) returns `409 Conflict`
with a `WRONGTYPE` message.

## Pub/Sub

Full details in [pubsub.md](pubsub.md). Summary:

| Method | Path | Purpose |
|--------|------|---------|
| `POST` | `/api/pubsub/{channel}` | publish `{"message":"..."}` → `{"receivers":N}` |
| `GET` | `/api/pubsub/{channel}/subscribe` | Server-Sent Events stream |
| `GET` | `/api/pubsub/{channel}` | subscriber count |

## Errors

Non-2xx responses carry a plain-text or JSON body describing the problem. Both
SDKs surface these as typed errors (`*sdk.APIError` in Go, `EvelentError` in TS)
exposing the HTTP status and body.

Next: [In-memory KV store](kv-store.md).
