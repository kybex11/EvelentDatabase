import { HttpClient, enc } from "../http";

/** Handle returned by subscribe(); call it to unsubscribe. */
export type Unsubscribe = () => void;

/**
 * PubSubApi — the database's in-process publish/subscribe broker, exposed over
 * HTTP (publish) and Server-Sent Events (subscribe). Subscribe uses the global
 * `fetch` streaming API, available in browsers and Node 18+.
 */
export class PubSubApi {
  constructor(private readonly http: HttpClient) {}

  /** Publish a message to a channel; returns how many subscribers received it. */
  async publish(channel: string, message: string): Promise<number> {
    const r = await this.http.request<{ receivers: number }>(
      `/api/pubsub/${enc(channel)}`,
      { method: "POST", body: JSON.stringify({ message }) },
    );
    return r.receivers;
  }

  /** Current subscriber count for a channel. */
  async numSubscribers(channel: string): Promise<number> {
    const r = await this.http.request<{ subscribers: number }>(
      `/api/pubsub/${enc(channel)}`,
      { method: "GET" },
    );
    return r.subscribers;
  }

  /**
   * Subscribe to a channel. `onMessage` is called for each published message.
   * Returns an unsubscribe function that closes the stream.
   */
  subscribe(
    channel: string,
    onMessage: (message: string) => void,
    onError?: (err: unknown) => void,
  ): Unsubscribe {
    const controller = new AbortController();
    const url = `${this.http.normalizeBase()}/api/pubsub/${enc(channel)}/subscribe`;

    (async () => {
      try {
        const resp = await fetch(url, {
          method: "GET",
          headers: { Accept: "text/event-stream" },
          signal: controller.signal,
        });
        if (!resp.ok || !resp.body) {
          throw new Error(`subscribe failed: ${resp.status}`);
        }
        const reader = resp.body.getReader();
        const decoder = new TextDecoder();
        let buffer = "";
        for (;;) {
          const { value, done } = await reader.read();
          if (done) break;
          buffer += decoder.decode(value, { stream: true });
          let nl: number;
          while ((nl = buffer.indexOf("\n")) >= 0) {
            const line = buffer.slice(0, nl).trimEnd();
            buffer = buffer.slice(nl + 1);
            if (line.startsWith("data:")) {
              const msg = line.slice(5).trimStart().replace(/\\n/g, "\n");
              onMessage(msg);
            }
          }
        }
      } catch (err) {
        if (!controller.signal.aborted && onError) onError(err);
      }
    })();

    return () => controller.abort();
  }
}
