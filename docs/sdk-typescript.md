# TypeScript / JavaScript SDK guide

Package source: `sdk/typescript` (published as `@evelent/db-sdk`). Works in
Node 18+ and modern bundlers. Built on `axios`.

## Install & build

```bash
cd sdk/typescript
npm install
npm run build      # emits dist/
```

## Connect

```ts
import { EvelentClient } from "@evelent/db-sdk";

const db = new EvelentClient("http://127.0.0.1:8080");
// or: const db = EvelentClient.connect("http://127.0.0.1:8080");
```

`db` exposes:

- `db.collections` — collection management
- `db.collection(name)` — scope with `.documents`, `.indexes`, `.stats`
- `db.kv` — the in-memory key/value store
- `db.health` — `ping()`
- `db.http` — the underlying HTTP client

## Collections

```ts
await db.collections.create("users");
const names = await db.collections.list();
await db.collections.drop("users");
```

## Documents

```ts
const users = db.collection("users").documents;

const { _id } = await users.insertOne({ name: "Ada", age: 36 });
const { inserted } = await users.insertMany([{ name: "Linus" }, { name: "Grace" }]);

const doc = await users.getById(_id);
await users.replaceById(_id, { name: "Ada Lovelace" });
await users.deleteById(_id);

const rows = await users.find({
  filter: { age: { $gte: 18 } },
  sort: { field: "age", order: -1 },
  limit: 100,
});
```

## Indexes & stats

```ts
const scope = db.collection("users");
await scope.indexes.create("email");
const list = await scope.indexes.list();
await scope.indexes.drop("email");

const stats = await scope.stats.get();
console.log(stats.count, stats.storageSize);
```

## KV store

```ts
const kv = db.kv;

await kv.set("greeting", "hello", 60);     // 60s TTL
const v = await kv.get("greeting");         // string | null
const ok = await kv.setNX("lock:job", "1", 30); // false if present
const n = await kv.incr("visits", 1);
await kv.expire("greeting", 120);
const { ttlSeconds, persists } = await kv.ttl("greeting");
const keys = await kv.keys("session:");

// batch
await kv.mset([{ key: "a", value: "1" }, { key: "b", value: "2" }], 0);
const { items, missing } = await kv.mget(["a", "b", "c"]);
const deleted = await kv.mdel(["a", "b"]);

const stats = await kv.stats();
console.log(stats.hitRatio);
await kv.flush();
```

## Data types (hash / list / set)

```ts
// hash
await db.hash.set("user:1", "name", "Ada");
await db.hash.incrBy("user:1", "logins", 1);
const profile = await db.hash.getAll("user:1");

// list
await db.list.rpush("queue", "a", "b", "c");
const items = await db.list.range("queue", 0, -1);
const head = await db.list.lpop("queue");

// set
await db.set.add("tags", "go", "ts", "go");
const isMember = await db.set.isMember("tags", "go");
const members = await db.set.members("tags");
```

## Pub/Sub

```ts
const unsubscribe = db.pubsub.subscribe("room:1", (msg) => {
  console.log("got", msg);
});

await db.pubsub.publish("room:1", "hello");

// later
unsubscribe();
```

## Error handling

Non-2xx responses throw `EvelentError`:

```ts
import { EvelentError } from "@evelent/db-sdk";

try {
  await db.collections.create("");
} catch (e) {
  if (e instanceof EvelentError) {
    console.error(e.status, e.statusText, e.body);
  }
}
```

`kv.get`, `kv.del`, and `kv.setNX` translate `404`/`409` into `null`/`false`
instead of throwing.

Next: [Architecture](architecture.md).
