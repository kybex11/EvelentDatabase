import { HttpClient } from "./http";
import { DocumentsApi } from "./endpoints/documents";
import { IndexesApi } from "./endpoints/indexes";
import { StatsApi } from "./endpoints/stats";

export class CollectionScope {
  readonly documents: DocumentsApi;
  readonly indexes: IndexesApi;
  readonly stats: StatsApi;

  constructor(http: HttpClient, readonly name: string) {
    this.documents = new DocumentsApi(http, name);
    this.indexes = new IndexesApi(http, name);
    this.stats = new StatsApi(http, name);
  }
}
