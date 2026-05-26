import { Document, Filter, JsonObject } from "./types";
import { ApiError } from "./errors";

export class DBClient {
  private baseUrl: string;

  constructor(baseUrl: string = "http://localhost:8080") {
    this.baseUrl = baseUrl;
  }

  private async request<T>(path: string, options?: RequestInit): Promise<T> {
    const url = `${this.baseUrl}${path}`;
    const response = await fetch(url, {
      headers: { "Content-Type": "application/json" },
      ...options,
    });

    if (!response.ok) {
      let body;
      try {
        body = await response.json();
      } catch {
        body = await response.text();
      }
      throw new ApiError(response.status, response.statusText, body);
    }

    if (response.status === 204) {
      return {} as T;
    }

    return response.json() as Promise<T>;
  }

  async listCollections(): Promise<string[]> {
    return this.request<string[]>("/api/collections");
  }

  async createCollection(name: string): Promise<void> {
    await this.request("/api/collections", {
      method: "POST",
      body: JSON.stringify({ name }),
    });
  }

  async dropCollection(name: string): Promise<void> {
    await this.request(`/api/collections/${name}`, { method: "DELETE" });
  }

  async insertDocument(
    collection: string,
    doc: JsonObject,
  ): Promise<{ _id: string }> {
    return this.request(`/api/collections/${collection}/docs`, {
      method: "POST",
      body: JSON.stringify(doc),
    });
  }

  async getDocument(collection: string, id: string): Promise<Document> {
    return this.request(`/api/collections/${collection}/docs/${id}`);
  }

  async updateDocument(
    collection: string,
    id: string,
    update: JsonObject,
  ): Promise<void> {
    await this.request(`/api/collections/${collection}/docs/${id}`, {
      method: "PUT",
      body: JSON.stringify(update),
    });
  }

  async deleteDocument(collection: string, id: string): Promise<void> {
    await this.request(`/api/collections/${collection}/docs/${id}`, {
      method: "DELETE",
    });
  }

  async findDocuments(
    collection: string,
    query?: Filter & {
      filter?: Filter;
      limit?: number;
      skip?: number;
      sort?: { field: string; order?: number };
    },
  ): Promise<Document[]> {
    return this.request(`/api/collections/${collection}/find`, {
      method: "POST",
      body: JSON.stringify(query || {}),
    });
  }

  async createIndex(collection: string, field: string): Promise<void> {
    await this.request(`/api/collections/${collection}/indexes`, {
      method: "POST",
      body: JSON.stringify({ field }),
    });
  }

  async listIndexes(collection: string): Promise<string[]> {
    return this.request<string[]>(`/api/collections/${collection}/indexes`);
  }

  async dropIndex(collection: string, field: string): Promise<void> {
    await this.request(`/api/collections/${collection}/indexes/${encodeURIComponent(field)}`, {
      method: "DELETE",
    });
  }

  async collectionStats(collection: string): Promise<{
    count: number;
    storageSize: number;
  }> {
    return this.request(`/api/collections/${collection}/stats`);
  }
}
