# Data types (hash, list, set)

Beyond plain string keys, the in-memory store supports three Redis-style
container types. Every key has exactly one type; calling an operation of the
wrong type on a key returns a `WRONGTYPE` error (HTTP `409`). All container
mutations are atomic and survive restarts via the encrypted snapshot.

## Hash

A key holding a map of field → value.

| HTTP | SDK (Go / TS) |
|------|----------------|
| `PUT /api/hash/{key}/{field}` `{value}` | `KV.HSet` / `hash.set` |
| `PUT /api/hash/{key}` `{fields}` | `KV.HSetMany` / `hash.setMany` |
| `GET /api/hash/{key}/{field}` | `KV.HGet` / `hash.get` |
| `GET /api/hash/{key}` | `KV.HGetAll` / `hash.getAll` |
| `DELETE /api/hash/{key}/{field}` | `KV.HDel` / `hash.del` |
| `GET /api/hash/{key}/_keys` | `KV.HKeys` / `hash.keys` |
| `GET /api/hash/{key}/_len` | `KV.HLen` / `hash.len` |
| `POST /api/hash/{key}/{field}/incr` `{delta}` | `KV.HIncrBy` / `hash.incrBy` |

```ts
await db.hash.set("user:1", "name", "Ada");
await db.hash.incrBy("user:1", "logins", 1);
const all = await db.hash.getAll("user:1"); // { name: "Ada", logins: "1" }
```

```go
client.KV.HSet("user:1", "name", "Ada")
client.KV.HIncrBy("user:1", "logins", 1)
all, _ := client.KV.HGetAll("user:1")
```

## List

A key holding an ordered sequence. Head = index 0 (left), tail = right.
Negative indices count from the tail (`-1` is the last element).

| HTTP | SDK |
|------|-----|
| `POST /api/list/{key}/lpush` `{values}` | `LPush` / `list.lpush` |
| `POST /api/list/{key}/rpush` `{values}` | `RPush` / `list.rpush` |
| `POST /api/list/{key}/lpop` | `LPop` / `list.lpop` |
| `POST /api/list/{key}/rpop` | `RPop` / `list.rpop` |
| `GET /api/list/{key}?start=&stop=` | `LRange` / `list.range` |
| `GET /api/list/{key}/len` | `LLen` / `list.len` |
| `GET /api/list/{key}/index/{i}` | `LIndex` / `list.index` |

```ts
await db.list.rpush("queue", "a", "b", "c");
await db.list.lpush("queue", "z");
const items = await db.list.range("queue", 0, -1); // ["z","a","b","c"]
const next = await db.list.lpop("queue");          // "z"
```

```go
client.KV.RPush("queue", "a", "b", "c")
items, _ := client.KV.LRange("queue", 0, -1)
next, ok, _ := client.KV.LPop("queue")
```

## Set

A key holding an unordered collection of unique strings.

| HTTP | SDK |
|------|-----|
| `POST /api/set/{key}/add` `{members}` | `SAdd` / `set.add` |
| `POST /api/set/{key}/rem` `{members}` | `SRem` / `set.rem` |
| `GET /api/set/{key}` | `SMembers` / `set.members` |
| `GET /api/set/{key}/ismember/{member}` | `SIsMember` / `set.isMember` |
| `GET /api/set/{key}/card` | `SCard` / `set.card` |

```ts
await db.set.add("tags", "go", "ts", "go"); // adds 2
const isMember = await db.set.isMember("tags", "go"); // true
const all = await db.set.members("tags");
```

```go
client.KV.SAdd("tags", "go", "ts", "go")
ok, _ := client.KV.SIsMember("tags", "go")
```

## Shared key semantics

TTL, `DELETE`, `EXISTS`, and key listing (`/api/kv/keys`) work uniformly across
all types — a hash, list, or set key can be given a TTL with
`POST /api/kv/{key}/expire` and appears in `GET /api/kv/keys`. Removing the last
element/field/member of a container deletes the key entirely.

Next: [Pub/Sub](pubsub.md).
