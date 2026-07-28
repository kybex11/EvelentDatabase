package db

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
)

// Index is an equality secondary index: canonical field value → set of doc IDs.
// On disk it is still stored as map[string][]string for compatibility; in memory
// IDs live in a set so Delete is O(1) and duplicates cannot accumulate.
type Index struct {
	field string
	data  map[string]map[string]struct{}
}

func NewIndex(field string) *Index {
	return &Index{
		field: field,
		data:  make(map[string]map[string]struct{}),
	}
}

func LoadIndex(path string, encKey []byte) (*Index, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	plain, err := DecodePayload(encKey, raw)
	if err != nil {
		return nil, err
	}
	var idxData map[string][]string
	if err := json.Unmarshal(plain, &idxData); err != nil {
		return nil, err
	}
	data := make(map[string]map[string]struct{}, len(idxData))
	for k, ids := range idxData {
		set := make(map[string]struct{}, len(ids))
		for _, id := range ids {
			set[id] = struct{}{}
		}
		data[k] = set
	}
	return &Index{data: data}, nil
}

func (idx *Index) Save(path string, encKey []byte) error {
	disk := make(map[string][]string, len(idx.data))
	for k, set := range idx.data {
		ids := make([]string, 0, len(set))
		for id := range set {
			ids = append(ids, id)
		}
		disk[k] = ids
	}
	data, err := json.Marshal(disk)
	if err != nil {
		return err
	}
	blob, err := EncodePayload(encKey, data)
	if err != nil {
		return err
	}
	return AtomicWriteFile(path, blob, 0644)
}

func (idx *Index) Insert(doc map[string]interface{}) {
	val, ok := doc[idx.field]
	if !ok {
		return
	}
	key := toIndexKey(val)
	id := docIDString(doc)
	if id == "" {
		return
	}
	set, ok := idx.data[key]
	if !ok {
		set = make(map[string]struct{}, 1)
		idx.data[key] = set
	}
	set[id] = struct{}{}
}

func (idx *Index) Delete(doc map[string]interface{}) {
	val, ok := doc[idx.field]
	if !ok {
		return
	}
	key := toIndexKey(val)
	id := docIDString(doc)
	set, ok := idx.data[key]
	if !ok {
		return
	}
	delete(set, id)
	if len(set) == 0 {
		delete(idx.data, key)
	}
}

// Find returns the document IDs whose indexed field equals value.
func (idx *Index) Find(value interface{}) []string {
	set := idx.data[toIndexKey(value)]
	if len(set) == 0 {
		return nil
	}
	out := make([]string, 0, len(set))
	for id := range set {
		out = append(out, id)
	}
	return out
}

func docIDString(doc map[string]interface{}) string {
	v, ok := doc["_id"]
	if !ok {
		return ""
	}
	if s, ok := v.(string); ok {
		return s
	}
	return fmt.Sprint(v)
}

// toIndexKey builds a stable canonical key matching historical json.Marshal
// output for common scalars (so on-disk .idx files stay valid), without
// allocating a full encoder for bool/number.
func toIndexKey(v interface{}) string {
	switch x := v.(type) {
	case nil:
		return "null"
	case bool:
		if x {
			return "true"
		}
		return "false"
	case float64:
		return strconv.FormatFloat(x, 'g', -1, 64)
	case float32:
		return strconv.FormatFloat(float64(x), 'g', -1, 32)
	case int:
		return strconv.Itoa(x)
	case int64:
		return strconv.FormatInt(x, 10)
	case int32:
		return strconv.FormatInt(int64(x), 10)
	case json.Number:
		return string(x)
	case string:
		b, _ := json.Marshal(x)
		return string(b)
	default:
		b, err := json.Marshal(v)
		if err != nil {
			return fmt.Sprint(v)
		}
		return string(b)
	}
}

// Deprecated alias kept for any external callers; prefer toIndexKey.
func toString(v interface{}) string {
	return toIndexKey(v)
}
