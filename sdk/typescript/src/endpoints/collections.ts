import { HttpClient, enc } from "../http";

export class CollectionsApi {
  constructor(private readonly http: HttpClient) {}

  async list(): Promise<string[]> {
    return this.http.request<string[]>("/api/collections", { method: "GET" });
  }

  async create(name: string): Promise<void> {
    await this.http.request("/api/collections", {
      method: "POST",
      body: JSON.stringify({ name }),
    });
  }

  async drop(name: string): Promise<void> {
    await this.http.request(`/api/collections/${enc(name)}`, {
      method: "DELETE",
    });
  }
}
