# Getting started

## Prerequisites

- Go 1.21+ (for the server and Go SDK)
- Node.js 18+ (for the TypeScript SDK and the desktop client frontend)

## 1. Build and run the server

```bash
cd server
go build -o db.exe .
./db.exe -port 8080 -data-dir ./data
```

You should see:

```
Starting server on 127.0.0.1:8080 (loopback=true, apiKey=false)
Data directory: ./data
GOMAXPROCS = 8
```

The server creates the data directory and an encryption key (`.key`) on first
run. It listens on **localhost by default**. Stop it with `Ctrl+C` — it flushes
a final KV snapshot and document indexes before exiting.

## 2. Smoke test with curl

```bash
# Health
curl http://127.0.0.1:8080/health           # -> ok

# Document store
curl -X POST http://127.0.0.1:8080/api/collections -d '{"name":"users"}'
curl -X POST http://127.0.0.1:8080/api/collections/users/docs \
     -d '{"name":"Ada","age":36}'
curl -X POST http://127.0.0.1:8080/api/collections/users/find -d '{}'

# In-memory KV store
curl -X PUT  http://127.0.0.1:8080/api/kv/greeting -d '{"value":"hello","ttlSeconds":60}'
curl       http://127.0.0.1:8080/api/kv/greeting       # -> {"key":"greeting","value":"hello"}
curl -X POST http://127.0.0.1:8080/api/kv/visits/incr -d '{"delta":1}'
```

## 3. Use a SDK

### Go

```bash
cd sdk/go
go get .
```

```go
client := sdk.New("http://127.0.0.1:8080")
client.KV.Set("greeting", "hello", 60)
v, _, _ := client.KV.Get("greeting")
fmt.Println(v) // hello
```

### TypeScript

```bash
cd sdk/typescript
npm install
npm run build
```

```ts
import { EvelentClient } from "@evelent/db-sdk";

const db = new EvelentClient("http://127.0.0.1:8080");
await db.kv.set("greeting", "hello", 60);
console.log(await db.kv.get("greeting")); // hello
```

## 4. Run the desktop client (optional)

```bash
cd client
wails dev      # development
wails build    # production binary
```

Next: [Configuration](configuration.md).
