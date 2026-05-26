package sdk

type SortSpec struct {
	Field string  `json:"field"`
	Order float64 `json:"order,omitempty"`
}

type FindQuery map[string]interface{}

type InsertOneResult struct {
	ID string `json:"_id"`
}

type InsertManyResult struct {
	IDs      []string `json:"ids"`
	Inserted int      `json:"inserted"`
}

type CollectionStats struct {
	Count       int64 `json:"count"`
	StorageSize int64 `json:"storageSize"`
}
