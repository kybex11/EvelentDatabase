import type { Document, FindQuery, FindResult, InsertManyResult, InsertOneResult, JsonObject } from "../types";
import { HttpClient, enc } from "../http";

export class DocumentsApi {
  constructor(
    private readonly http: HttpClient,
    private readonly collection: string,
  ) {}

  private base(): string {
    return `/api/collections/${enc(this.collection)}`;
  }

  async insertOne(doc: JsonObject): Promise<InsertOneResult> {
    return this.http.request<InsertOneResult>(`${this.base()}/docs`, {
      method: "POST",
      body: JSON.stringify(doc),
    });
  }

  async insertMany(documents: JsonObject[]): Promise<InsertManyResult> {
    return this.http.request<InsertManyResult>(`${this.base()}/docs/batch`, {
      method: "POST",
      body: JSON.stringify({ documents }),
    });
  }

  async getById(id: string): Promise<Document> {
    return this.http.request<Document>(
      `${this.base()}/docs/${enc(id)}`,
      { method: "GET" },
    );
  }

  async replaceById(id: string, doc: JsonObject): Promise<void> {
    await this.http.request(`${this.base()}/docs/${enc(id)}`, {
      method: "PUT",
      body: JSON.stringify(doc),
    });
  }

  async deleteById(id: string): Promise<void> {
    await this.http.request(`${this.base()}/docs/${enc(id)}`, {
      method: "DELETE",
    });
  }

  async find(query?: FindQuery | Record<string, unknown>): Promise<Document[]> {
    const page = await this.findPage(query);
    return page.documents;
  }

  async findPage(query?: FindQuery | Record<string, unknown>): Promise<FindResult> {
    const body = query ?? {};
    const raw = await this.http.request<FindResult | Document[]>(`${this.base()}/find`, {
      method: "POST",
      body: JSON.stringify(body),
    });
    if (Array.isArray(raw)) {
      return { documents: raw };
    }
    return {
      documents: raw.documents ?? [],
      nextCursor: raw.nextCursor || undefined,
    };
  }
}
