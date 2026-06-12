import { HttpClient, enc } from "../http";

/** SetApi — Redis-style set commands (unordered unique strings). */
export class SetApi {
  constructor(private readonly http: HttpClient) {}

  private path(key: string): string {
    return `/api/set/${enc(key)}`;
  }

  /** Add members; returns the number newly added. */
  async add(key: string, ...members: string[]): Promise<number> {
    const r = await this.http.request<{ added: number }>(
      `${this.path(key)}/add`,
      { method: "POST", body: JSON.stringify({ members }) },
    );
    return r.added;
  }

  /** Remove members; returns the number removed. */
  async rem(key: string, ...members: string[]): Promise<number> {
    const r = await this.http.request<{ removed: number }>(
      `${this.path(key)}/rem`,
      { method: "POST", body: JSON.stringify({ members }) },
    );
    return r.removed;
  }

  /** All members of the set. */
  async members(key: string): Promise<string[]> {
    const r = await this.http.request<{ members: string[] }>(this.path(key), {
      method: "GET",
    });
    return r.members ?? [];
  }

  /** Whether member is in the set. */
  async isMember(key: string, member: string): Promise<boolean> {
    const r = await this.http.request<{ member: boolean }>(
      `${this.path(key)}/ismember/${enc(member)}`,
      { method: "GET" },
    );
    return r.member;
  }

  /** Number of members. */
  async card(key: string): Promise<number> {
    const r = await this.http.request<{ count: number }>(
      `${this.path(key)}/card`,
      { method: "GET" },
    );
    return r.count;
  }
}
