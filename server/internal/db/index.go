package db

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strconv"
)

// Index is an equality + range secondary index.
// Equality: canonical field value → set of doc IDs (O(1)).
// Range: parallel sorted numeric / string buckets for $gt/$gte/$lt/$lte.
type Index struct {
	field string
	data  map[string]map[string]struct{}
	nums  []numBucket // sorted by v ascending
	strs  []strBucket // sorted by s ascending
}

type numBucket struct {
	v   float64
	ids map[string]struct{}
}

type strBucket struct {
	s   string
	ids map[string]struct{}
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
	idx := NewIndex("")
	for k, ids := range idxData {
		set := make(map[string]struct{}, len(ids))
		for _, id := range ids {
			set[id] = struct{}{}
		}
		idx.data[k] = set
		if len(k) >= 2 && k[0] == '"' {
			var s string
			if json.Unmarshal([]byte(k), &s) == nil {
				for id := range set {
					idx.addStr(s, id)
				}
			}
		} else if n, err := strconv.ParseFloat(k, 64); err == nil {
			for id := range set {
				idx.addNum(n, id)
			}
		}
	}
	return idx, nil
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
	if n, ok := toFloat64(val); ok {
		idx.addNum(n, id)
	} else if s, ok := val.(string); ok {
		idx.addStr(s, id)
	}
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
	if n, ok := toFloat64(val); ok {
		idx.delNum(n, id)
	} else if s, ok := val.(string); ok {
		idx.delStr(s, id)
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

// FindRange returns IDs whose numeric or string field value falls in the range.
// Pass nil for an open bound.
func (idx *Index) FindRange(lo, hi interface{}, loIncl, hiIncl bool) []string {
	if lo != nil {
		if _, ok := toFloat64(lo); ok {
			return idx.findNumRange(lo, hi, loIncl, hiIncl)
		}
		if _, ok := lo.(string); ok {
			return idx.findStrRange(lo, hi, loIncl, hiIncl)
		}
	}
	if hi != nil {
		if _, ok := toFloat64(hi); ok {
			return idx.findNumRange(lo, hi, loIncl, hiIncl)
		}
		if _, ok := hi.(string); ok {
			return idx.findStrRange(lo, hi, loIncl, hiIncl)
		}
	}
	return nil
}

func (idx *Index) findNumRange(lo, hi interface{}, loIncl, hiIncl bool) []string {
	var out []string
	for _, b := range idx.nums {
		if lo != nil {
			ln, _ := toFloat64(lo)
			if loIncl {
				if b.v < ln {
					continue
				}
			} else if b.v <= ln {
				continue
			}
		}
		if hi != nil {
			hn, _ := toFloat64(hi)
			if hiIncl {
				if b.v > hn {
					break
				}
			} else if b.v >= hn {
				break
			}
		}
		for id := range b.ids {
			out = append(out, id)
		}
	}
	return out
}

func (idx *Index) findStrRange(lo, hi interface{}, loIncl, hiIncl bool) []string {
	var loS, hiS string
	var hasLo, hasHi bool
	if lo != nil {
		loS, hasLo = lo.(string)
	}
	if hi != nil {
		hiS, hasHi = hi.(string)
	}
	var out []string
	for _, b := range idx.strs {
		if hasLo {
			if loIncl {
				if b.s < loS {
					continue
				}
			} else if b.s <= loS {
				continue
			}
		}
		if hasHi {
			if hiIncl {
				if b.s > hiS {
					break
				}
			} else if b.s >= hiS {
				break
			}
		}
		for id := range b.ids {
			out = append(out, id)
		}
	}
	return out
}

func (idx *Index) addNum(v float64, id string) {
	i := sort.Search(len(idx.nums), func(i int) bool { return idx.nums[i].v >= v })
	if i < len(idx.nums) && idx.nums[i].v == v {
		idx.nums[i].ids[id] = struct{}{}
		return
	}
	idx.nums = append(idx.nums, numBucket{})
	copy(idx.nums[i+1:], idx.nums[i:])
	idx.nums[i] = numBucket{v: v, ids: map[string]struct{}{id: {}}}
}

func (idx *Index) delNum(v float64, id string) {
	i := sort.Search(len(idx.nums), func(i int) bool { return idx.nums[i].v >= v })
	if i >= len(idx.nums) || idx.nums[i].v != v {
		return
	}
	delete(idx.nums[i].ids, id)
	if len(idx.nums[i].ids) == 0 {
		idx.nums = append(idx.nums[:i], idx.nums[i+1:]...)
	}
}

func (idx *Index) addStr(s, id string) {
	i := sort.Search(len(idx.strs), func(i int) bool { return idx.strs[i].s >= s })
	if i < len(idx.strs) && idx.strs[i].s == s {
		idx.strs[i].ids[id] = struct{}{}
		return
	}
	idx.strs = append(idx.strs, strBucket{})
	copy(idx.strs[i+1:], idx.strs[i:])
	idx.strs[i] = strBucket{s: s, ids: map[string]struct{}{id: {}}}
}

func (idx *Index) delStr(s, id string) {
	i := sort.Search(len(idx.strs), func(i int) bool { return idx.strs[i].s >= s })
	if i >= len(idx.strs) || idx.strs[i].s != s {
		return
	}
	delete(idx.strs[i].ids, id)
	if len(idx.strs[i].ids) == 0 {
		idx.strs = append(idx.strs[:i], idx.strs[i+1:]...)
	}
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

func toString(v interface{}) string { return toIndexKey(v) }
