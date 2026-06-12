import type { CollectionStats } from "../types";
import { HttpClient, enc } from "../http";

export class StatsApi {
  constructor(
    private readonly http: HttpClient,
    private readonly collection: string,
  ) {}

  async get(): Promise<CollectionStats> {
    return this.http.request<CollectionStats>(
      `/api/collections/${enc(this.collection)}/stats`,
      { method: "GET" },
    );
  }
}
