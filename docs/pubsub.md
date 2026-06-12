# Publish / Subscribe

The database includes an in-process publish/subscribe broker — a live message
bus, independent of the key/value store. Publishers send messages to a named
channel; every current subscriber of that channel receives them. There is no
persistence: a message is delivered only to subscribers connected at the time
it is published.

## HTTP API

| Method | Path | Body | Response |
|--------|------|------|----------|
| `POST` | `/api/pubsub/{channel}` | `{"message":"..."}` | `{"receivers":N}` |
| `GET` | `/api/pubsub/{channel}` | — | `{"channel":"c","subscribers":N}` |
| `GET` | `/api/pubsub/{channel}/subscribe` | — | **SSE** stream of `data:` events |

The subscribe endpoint is a [Server-Sent Events](https://developer.mozilla.org/docs/Web/API/Server-sent_events)
stream: each published message arrives as a `data: <message>\n\n` event.
Newlines inside a message are escaped as `\n` on the wire.

### Delivery guarantees

- **At-most-once**, fire-and-forget. No acknowledgements, no replay.
- Each subscriber has a bounded buffer (64 messages). If a subscriber cannot
  keep up and its buffer fills, further messages to it are **dropped** rather
  than blocking the publisher (slow-consumer protection).

## TypeScript

`subscribe` uses the streaming `fetch` API (browsers and Node 18+).

```ts
// subscriber
const unsubscribe = db.pubsub.subscribe(
  "room:1",
  (msg) => console.log("got", msg),
  (err) => console.error("stream error", err),
);

// publisher (elsewhere)
const receivers = await db.pubsub.publish("room:1", "hello");

// later
unsubscribe();
```

## Go

```go
ctx, cancel := context.WithCancel(context.Background())
defer cancel()

sub, err := client.PubSub.Subscribe(ctx, "room:1")
if err != nil { log.Fatal(err) }
go func() {
    for msg := range sub.Messages {
        fmt.Println("got", msg)
    }
}()

n, _ := client.PubSub.Publish("room:1", "hello")
fmt.Println("delivered to", n)

// later: sub.Close() or cancel()
```

## Patterns

- **Cache invalidation** — publish a key name on `cache:invalidate`; subscribers
  drop their local copy.
- **Live dashboards** — publish metrics on a channel; the UI subscribes over SSE.
- **Fan-out notifications** — one publish reaches every connected worker.

For durable, replayable streams use a list key (`RPush` + `LRange`) instead;
pub/sub is for ephemeral, real-time delivery.

Next: [Go SDK guide](sdk-go.md).
