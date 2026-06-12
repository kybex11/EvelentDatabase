package db

import "fmt"

// ValueKind tags the concrete type stored under a key in the in-memory store.
type ValueKind uint8

const (
	KindString ValueKind = iota
	KindHash
	KindList
	KindSet
)

func (k ValueKind) String() string {
	switch k {
	case KindString:
		return "string"
	case KindHash:
		return "hash"
	case KindList:
		return "list"
	case KindSet:
		return "set"
	default:
		return "unknown"
	}
}

// Value is the tagged union stored in the cache. Exactly one of the typed
// fields is meaningful, selected by Kind. It is JSON-serializable so the whole
// store can be snapshotted to disk with its types intact.
type Value struct {
	Kind ValueKind           `json:"k"`
	Str  string              `json:"s,omitempty"`
	Hash map[string]string   `json:"h,omitempty"`
	List []string            `json:"l,omitempty"`
	Set  map[string]struct{} `json:"e,omitempty"`
}

func newStringValue(s string) *Value { return &Value{Kind: KindString, Str: s} }

// size returns the approximate in-memory footprint of the value in bytes, used
// for the cache's byte bound.
func (v *Value) size() int64 {
	if v == nil {
		return 0
	}
	const overhead = 16
	switch v.Kind {
	case KindString:
		return int64(len(v.Str)) + overhead
	case KindHash:
		n := int64(overhead)
		for k, val := range v.Hash {
			n += int64(len(k)+len(val)) + overhead
		}
		return n
	case KindList:
		n := int64(overhead)
		for _, s := range v.List {
			n += int64(len(s)) + 8
		}
		return n
	case KindSet:
		n := int64(overhead)
		for k := range v.Set {
			n += int64(len(k)) + 8
		}
		return n
	default:
		return overhead
	}
}

// clone returns a deep copy so read-modify-write operations never mutate the
// object still held by the cache.
func (v *Value) clone() *Value {
	if v == nil {
		return nil
	}
	c := &Value{Kind: v.Kind, Str: v.Str}
	if v.Hash != nil {
		c.Hash = make(map[string]string, len(v.Hash))
		for k, val := range v.Hash {
			c.Hash[k] = val
		}
	}
	if v.List != nil {
		c.List = make([]string, len(v.List))
		copy(c.List, v.List)
	}
	if v.Set != nil {
		c.Set = make(map[string]struct{}, len(v.Set))
		for k := range v.Set {
			c.Set[k] = struct{}{}
		}
	}
	return c
}

// wrongType builds a Redis-style WRONGTYPE error.
func wrongType(want, got ValueKind) error {
	return fmt.Errorf("WRONGTYPE key holds a %s, not a %s", got, want)
}
