import { HttpClient, enc } from "../http";
import { EvelentError } from "../errors";

/** HashApi — Redis-style hash commands (a key holding field → value). */
export class HashApi {
  constructor(private readonly http: HttpClient) {}

  private path(key: string): string {
    return `/api/hash/${enc(key)}`;
  }

  /** Set a single field. Returns the number of newly-created fields (0 or 1). */
  async set(key: string, field: string, value: string): Promise<number> {
    const r = await this.http.request<{ added: number }>(
      `${this.path(key)}/${enc(field)}`,
      { method: "PUT", body: JSON.stringify({ value }) },
    );
    return r.added;
  }

  /** Set many fields at once. Returns the number of newly-created fields. */
  async setMany(key: string, fields: Record<string, string>): Promise<number> {
    const r = await this.http.request<{ added: number }>(this.path(key), {
      method: "PUT",
      body: JSON.stringify({ fields }),
    });
    return r.added;
  }

  /** Get a field value, or `null` when the field/key is absent. */
  async get(key: string, field: string): Promise<string | null> {
    try {
      const r = await this.http.request<{ value: string }>(
        `${this.path(key)}/${enc(field)}`,
        { method: "GET" },
      );
      return r.value;
    } catch (e) {
      if (e instanceof EvelentError && e.status === 404) return null;
      throw e;
    }
  }

  /** Get the whole hash. */
  async getAll(key: string): Promise<Record<string, string>> {
    const r = await this.http.request<{ fields: Record<string, string> }>(
      this.path(key),
      { method: "GET" },
    );
    return r.fields ?? {};
  }

  /** Delete a field. Returns the number removed (0 or 1). */
  async del(key: string, field: string): Promise<number> {
    const r = await this.http.request<{ removed: number }>(
      `${this.path(key)}/${enc(field)}`,
      { method: "DELETE" },
    );
    return r.removed;
  }

  /** List the field names. */
  async keys(key: string): Promise<string[]> {
    const r = await this.http.request<{ keys: string[] }>(
      `${this.path(key)}/_keys`,
      { method: "GET" },
    );
    return r.keys ?? [];
  }

  /** Number of fields in the hash. */
  async len(key: string): Promise<number> {
    const r = await this.http.request<{ length: number }>(
      `${this.path(key)}/_len`,
      { method: "GET" },
    );
    return r.length;
  }

  /** Atomically add `delta` to an integer field; returns the new value. */
  async incrBy(key: string, field: string, delta = 1): Promise<number> {
    const r = await this.http.request<{ value: number }>(
      `${this.path(key)}/${enc(field)}/incr`,
      { method: "POST", body: JSON.stringify({ delta }) },
    );
    return r.value;
  }
}
