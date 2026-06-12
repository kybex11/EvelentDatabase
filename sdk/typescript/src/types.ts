export type JsonValue =
  | string
  | number
  | boolean
  | null
  | JsonObject
  | JsonArray;
export type JsonObject = { [key: string]: JsonValue };
export type JsonArray = JsonValue[];

export interface Document extends JsonObject {
  _id: string;
}

export interface Filter {
  [field: string]: JsonValue | FilterOperator;
}

export interface FilterOperator {
  $eq?: JsonValue;
  $ne?: JsonValue;
  $gt?: number;
  $gte?: number;
  $lt?: number;
  $lte?: number;
  $in?: JsonArray;
}

/** A single key/value pair used by the in-memory store batch operations. */
export interface KVItem {
  key: string;
  value: string;
}

/** Point-in-time counters for the in-memory key/value store. */
export interface KVStats {
  items: number;
  bytes: number;
  maxItems: number;
  maxBytes: number;
  hits: number;
  misses: number;
  evictions: number;
  expired: number;
  hitRatio: number;
}

export interface SortSpec {
  field: string;
  order?: number;
}

export interface Projection {
  [field: string]: 0 | 1 | boolean;
}

export interface FindQuery {
  filter?: Filter;
  limit?: number;
  skip?: number;
  sort?: SortSpec;
  projection?: Projection;
  cursor?: string;
}

export interface CollectionStats {
  count: number;
  storageSize: number;
}

export interface InsertOneResult {
  _id: string;
}

export interface InsertManyResult {
  ids: string[];
  inserted: number;
}
