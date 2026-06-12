import { HttpClient } from "./http";
import { CollectionsApi } from "./endpoints/collections";
import { HealthApi } from "./endpoints/health";
import { KVApi } from "./endpoints/kv";
import { CollectionScope } from "./collection-scope";

export class EvelentClient {
  readonly http: HttpClient;
  readonly collections: CollectionsApi;
  readonly health: HealthApi;
  /** In-memory key/value store (Redis-like), backed by the same engine. */
  readonly kv: KVApi;

  constructor(baseUrl: string) {
    this.http = new HttpClient(baseUrl);
    this.collections = new CollectionsApi(this.http);
    this.health = new HealthApi(this.http);
    this.kv = new KVApi(this.http);
  }

  static connect(baseUrl: string): EvelentClient {
    return new EvelentClient(baseUrl);
  }

  collection(name: string): CollectionScope {
    return new CollectionScope(this.http, name);
  }
}
