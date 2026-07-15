import { HttpClient, HttpClientOptions } from "./http";
import { CollectionsApi } from "./endpoints/collections";
import { HealthApi } from "./endpoints/health";
import { KVApi } from "./endpoints/kv";
import { HashApi } from "./endpoints/hash";
import { ListApi } from "./endpoints/list";
import { SetApi } from "./endpoints/set";
import { PubSubApi } from "./endpoints/pubsub";
import { CollectionScope } from "./collection-scope";

export type EvelentClientOptions = HttpClientOptions;

export class EvelentClient {
  readonly http: HttpClient;
  readonly collections: CollectionsApi;
  readonly health: HealthApi;
  /** In-memory key/value store (Redis-like), backed by the same engine. */
  readonly kv: KVApi;
  /** Redis-style hash commands. */
  readonly hash: HashApi;
  /** Redis-style list commands. */
  readonly list: ListApi;
  /** Redis-style set commands. */
  readonly set: SetApi;
  /** Publish/subscribe broker. */
  readonly pubsub: PubSubApi;

  constructor(baseUrl: string, options?: EvelentClientOptions) {
    this.http = new HttpClient(baseUrl, options);
    this.collections = new CollectionsApi(this.http);
    this.health = new HealthApi(this.http);
    this.kv = new KVApi(this.http);
    this.hash = new HashApi(this.http);
    this.list = new ListApi(this.http);
    this.set = new SetApi(this.http);
    this.pubsub = new PubSubApi(this.http);
  }

  static connect(baseUrl: string, options?: EvelentClientOptions): EvelentClient {
    return new EvelentClient(baseUrl, options);
  }

  collection(name: string): CollectionScope {
    return new CollectionScope(this.http, name);
  }
}
