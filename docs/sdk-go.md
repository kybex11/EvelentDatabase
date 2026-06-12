# Go SDK guide

Module: `sdk/go` (Go module `evelent.dev/db/sdk`).

```go
import sdk "evelent.dev/db/sdk"
```

## Connect

```go
client := sdk.New("http://127.0.0.1:8080")
```

`client` exposes:

- `client.Collections` — collection management
- `client.Collection(name)` — a scope for documents/indexes/stats
- `client.KV` — the in-memory key/value store
- `client.Health` — `Ping()`
- `client.HTTP` — the underlying HTTP client

## Collections

```go
client.Collections.Create("users")
names, _ := client.Collections.List()
client.Collections.Drop("users")
```

## Documents

```go
users := client.Collection("users").Documents()

res, _ := users.InsertOne(map[string]interface{}{"name": "Ada", "age": 36})
fmt.Println(res.ID)

many, _ := users.InsertMany([]map[string]interface{}{
    {"name": "Linus"}, {"name": "Grace"},
})
fmt.Println(many.Inserted)

doc, _ := users.GetByID(res.ID)
users.ReplaceByID(res.ID, map[string]interface{}{"name": "Ada Lovelace"})
users.DeleteByID(res.ID)

rows, _ := users.Find(sdk.FindQuery{
    "filter": map[string]interface{}{"age": map[string]interface{}{"$gte": 18}},
    "limit":  100,
    "sort":   map[string]interface{}{"field": "age", "order": -1},
})
```

## Indexes & stats

```go
idx := client.Collection("users").Indexes()
idx.Create("email")
list, _ := idx.List()
idx.Drop("email")

st, _ := client.Collection("users").Stats()
fmt.Println(st.Count, st.StorageSize)
```

## KV store

```go
kv := client.KV

kv.Set("greeting", "hello", 60)          // 60s TTL
v, found, _ := kv.Get("greeting")
ok, _ := kv.SetNX("lock:job", "1", 30)   // false if it already exists
n, _ := kv.Incr("visits", 1)
kv.Expire("greeting", 120)
ttl, persists, _ := kv.TTL("greeting")
keys, _ := kv.Keys("session:")           // prefix scan

// batch
kv.MSet([]sdk.KVItem{{Key: "a", Value: "1"}, {Key: "b", Value: "2"}}, 0)
items, missing, _ := kv.MGet([]string{"a", "b", "c"})
deleted, _ := kv.MDelete([]string{"a", "b"})

stats, _ := kv.Stats()
fmt.Printf("hit ratio: %.3f\n", stats.HitRatio)
kv.Flush()
```

## Error handling

Non-2xx responses return `*sdk.APIError`:

```go
if _, _, err := kv.Get("missing"); err != nil {
    if ae, ok := err.(*sdk.APIError); ok {
        fmt.Println(ae.Status, string(ae.Body))
    }
}
```

`Get`, `Delete`, and `SetNX` already translate `404`/`409` into a clean
`found`/`ok` boolean instead of an error.

Next: [TypeScript/JavaScript SDK guide](sdk-typescript.md).
