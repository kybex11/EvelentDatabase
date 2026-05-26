package db

import (
	"encoding/json"
	"fmt"
	"os"
)

type Index struct {
	field string
	data  map[interface{}][]string
}

func NewIndex(field string) *Index {
	return &Index{
		field: field,
		data:  make(map[interface{}][]string),
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
	idx := &Index{
		data: make(map[interface{}][]string),
	}
	for k, v := range idxData {
		idx.data[k] = v
	}
	return idx, nil
}

func (idx *Index) Save(path string, encKey []byte) error {
	out := make(map[string][]string)
	for k, v := range idx.data {
		out[toString(k)] = v
	}
	data, err := json.Marshal(out)
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
	id := docIDString(doc)
	idx.data[val] = append(idx.data[val], id)
}

func (idx *Index) Delete(doc map[string]interface{}) {
	val, ok := doc[idx.field]
	if !ok {
		return
	}
	id := docIDString(doc)
	list := idx.data[val]
	for i, v := range list {
		if v == id {
			idx.data[val] = append(list[:i], list[i+1:]...)
			if len(idx.data[val]) == 0 {
				delete(idx.data, val)
			}
			break
		}
	}
}

func (idx *Index) Find(value interface{}) []string {
	return idx.data[value]
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
