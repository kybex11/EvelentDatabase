import { HttpClient, enc } from "../http";
import { EvelentError } from "../errors";
import { KVItem, KVStats } from "../types";

/**
 * KVApi is a client for the embedded in-memory key/value store. It exposes a
 * Redis-like surface (set/get/del, TTL, atomic counters, batch ops) backed by
 * the database's own engine — no separate Redis process required.
 */
export class KVApi {
  constructor(private readonly http: HttpClient) {}

  private path(key: string): string {
    return `/api/kv/${enc(key)}`;
  }

  /** Store `value` under `key`. `ttlSeconds <= 0` means no expiry. */
  async set(key: string, value: string, ttlSeconds = 0): Promise<void> {
    await this.http.request(this.path(key), {
      method: "PUT",
      body: JSON.stringify({ value, ttlSeconds }),
    });
  }

  /** Store `value` only if `key` does not exist. Returns `true` when written. */
  async setNX(key: string, value: string, ttlSeconds = 0): Promise<boolean> {
    try {
      await this.http.request(this.path(key), {
        method: "PUT",
        body: JSON.stringify({ value, ttlSeconds, nx: true }),
      });
      return true;
    } catch (e) {
      if (e instanceof EvelentError && e.status === 409) return false;
      throw e;
    }
  }

  /** Read `key`. Returns `null` when the key is absent or expired. */
  async get(key: string): Promise<string | null> {
    try {
      const r = await this.http.request<{ value: string }>(this.path(key), {
        method: "GET",
      });
      return r.value;
    } catch (e) {
      if (e instanceof EvelentError && e.status === 404) return null;
      throw e;
    }
  }

  /** Delete `key`. Returns `true` when the key existed. */
  async del(key: string): Promise<boolean> {
    try {
      await this.http.request(this.path(key), { method: "DELETE" });
      return true;
    } catch (e) {
      if (e instanceof EvelentError && e.status === 404) return false;
      throw e;
    }
  }

  /** Atomically add `delta` to the integer at `key`; returns the new value. */
  async incr(key: string, delta = 1): Promise<number> {
    const r = await this.http.request<{ value: number }>(
      `${this.path(key)}/incr`,
      { method: "POST", body: JSON.stringify({ delta }) },
    );
    return r.value;
  }

  /** Atomically subtract `delta` from the integer at `key`. */
  async decr(key: string, delta = 1): Promise<number> {
    return this.incr(key, -delta);
  }

  /** Set or clear a key's TTL (`ttlSeconds <= 0` clears it). */
  async expire(key: string, ttlSeconds: number): Promise<void> {
    await this.http.request(`${this.path(key)}/expire`, {
      method: "POST",
      body: JSON.stringify({ ttlSeconds }),
    });
  }

  /** Remaining TTL. `persists` is true when the key has no expiry. */
  async ttl(key: string): Promise<{ ttlSeconds: number; persists: boolean }> {
    return this.http.request(`${this.path(key)}/ttl`, { method: "GET" });
  }

  /** List all live keys, optionally filtered by `prefix`. */
  async keys(prefix = ""): Promise<string[]> {
    const q = prefix ? `?prefix=${encodeURIComponent(prefix)}` : "";
    const r = await this.http.request<{ keys: string[] }>(
      `/api/kv/keys${q}`,
      { method: "GET" },
    );
    return r.keys ?? [];
  }

  /** In-memory store counters. */
  async stats(): Promise<KVStats> {
    return this.http.request<KVStats>("/api/kv/stats", { method: "GET" });
  }

  /** Remove every key. */
  async flush(): Promise<void> {
    await this.http.request("/api/kv/flush", { method: "POST" });
  }

  /** Write many pairs in one request (same `ttlSeconds` for all). */
  async mset(items: KVItem[], ttlSeconds = 0): Promise<number> {
    const r = await this.http.request<{ written: number }>("/api/kv/mset", {
      method: "POST",
      body: JSON.stringify({ items, ttlSeconds }),
    });
    return r.written;
  }

  /** Read many keys in one request. */
  async mget(keys: string[]): Promise<{ items: KVItem[]; missing: string[] }> {
    return this.http.request("/api/kv/mget", {
      method: "POST",
      body: JSON.stringify({ keys }),
    });
  }

  /** Delete many keys in one request; returns how many existed. */
  async mdel(keys: string[]): Promise<number> {
    const r = await this.http.request<{ deleted: number }>("/api/kv/mdel", {
      method: "POST",
      body: JSON.stringify({ keys }),
    });
    return r.deleted;
  }
}
