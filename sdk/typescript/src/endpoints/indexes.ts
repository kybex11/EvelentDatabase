import { HttpClient, enc } from "../http";

export class IndexesApi {
  constructor(
    private readonly http: HttpClient,
    private readonly collection: string,
  ) {}

  private base(): string {
    return `/api/collections/${enc(this.collection)}`;
  }

  async list(): Promise<string[]> {
    return this.http.request<string[]>(`${this.base()}/indexes`, {
      method: "GET",
    });
  }

  async create(field: string): Promise<void> {
    await this.http.request(`${this.base()}/indexes`, {
      method: "POST",
      body: JSON.stringify({ field }),
    });
  }

  async drop(field: string): Promise<void> {
    await this.http.request(`${this.base()}/indexes/${enc(field)}`, {
      method: "DELETE",
    });
  }
}
