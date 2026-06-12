import { HttpClient, enc } from "../http";
import { EvelentError } from "../errors";

/** ListApi — Redis-style list commands (an ordered sequence of strings). */
export class ListApi {
  constructor(private readonly http: HttpClient) {}

  private path(key: string): string {
    return `/api/list/${enc(key)}`;
  }

  /** Prepend values to the head (left). Returns the new length. */
  async lpush(key: string, ...values: string[]): Promise<number> {
    return this.push(key, "lpush", values);
  }

  /** Append values to the tail (right). Returns the new length. */
  async rpush(key: string, ...values: string[]): Promise<number> {
    return this.push(key, "rpush", values);
  }

  private async push(key: string, op: string, values: string[]): Promise<number> {
    const r = await this.http.request<{ length: number }>(
      `${this.path(key)}/${op}`,
      { method: "POST", body: JSON.stringify({ values }) },
    );
    return r.length;
  }

  /** Remove and return the head element, or `null` when empty. */
  async lpop(key: string): Promise<string | null> {
    return this.pop(key, "lpop");
  }

  /** Remove and return the tail element, or `null` when empty. */
  async rpop(key: string): Promise<string | null> {
    return this.pop(key, "rpop");
  }

  private async pop(key: string, op: string): Promise<string | null> {
    try {
      const r = await this.http.request<{ value: string }>(
        `${this.path(key)}/${op}`,
        { method: "POST" },
      );
      return r.value;
    } catch (e) {
      if (e instanceof EvelentError && e.status === 404) return null;
      throw e;
    }
  }

  /** Length of the list. */
  async len(key: string): Promise<number> {
    const r = await this.http.request<{ length: number }>(
      `${this.path(key)}/len`,
      { method: "GET" },
    );
    return r.length;
  }

  /** Element at index (negative counts from the tail), or `null`. */
  async index(key: string, index: number): Promise<string | null> {
    try {
      const r = await this.http.request<{ value: string }>(
        `${this.path(key)}/index/${index}`,
        { method: "GET" },
      );
      return r.value;
    } catch (e) {
      if (e instanceof EvelentError && e.status === 404) return null;
      throw e;
    }
  }

  /** Elements between start and stop inclusive (Redis-style negative indexing). */
  async range(key: string, start = 0, stop = -1): Promise<string[]> {
    const r = await this.http.request<{ values: string[] }>(
      `${this.path(key)}?start=${start}&stop=${stop}`,
      { method: "GET" },
    );
    return r.values ?? [];
  }
}
