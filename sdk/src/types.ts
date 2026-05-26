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
