package db

import (
	"encoding/json"
	"fmt"
	"os"
)

type Index struct {
	field string
	data  map[string][]string
}

func NewIndex(field string) *Index {
	return &Index{
		field: field,
		data:  make(map[string][]string),
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
	if idxData == nil {
		idxData = make(map[string][]string)
	}
	return &Index{data: idxData}, nil
}

func (idx *Index) Save(path string, encKey []byte) error {
	data, err := json.Marshal(idx.data)
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
	key := toString(val)
	id := docIDString(doc)
	idx.data[key] = append(idx.data[key], id)
}

func (idx *Index) Delete(doc map[string]interface{}) {
	val, ok := doc[idx.field]
	if !ok {
		return
	}
	key := toString(val)
	id := docIDString(doc)
	list := idx.data[key]
	for i, v := range list {
		if v == id {
			idx.data[key] = append(list[:i], list[i+1:]...)
			if len(idx.data[key]) == 0 {
				delete(idx.data, key)
			}
			break
		}
	}
}

// Find returns the document IDs whose indexed field equals value. Lookup is
// O(1) on the canonical string form of value, so it works identically for
// freshly built and reloaded indexes.
func (idx *Index) Find(value interface{}) []string {
	return idx.data[toString(value)]
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

func toString(v interface{}) string {
	b, _ := json.Marshal(v)
	return string(b)
}
