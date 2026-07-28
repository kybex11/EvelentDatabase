package db

import (
	"strconv"
	"time"
)

// This file extends MemStore with additional Redis-like string commands and
// batch primitives. Batch operations exist specifically to move large volumes
// of data in a single round-trip instead of one key at a time.

// SetNX stores value under key only when the key does not already exist. It
// reports whether the write happened.
func (m *MemStore) SetNX(key, value string, ttl time.Duration) bool {
	unlock := m.lockKey(key)
	defer unlock()
	if m.Exists(key) {
		return false
	}
	m.Set(key, value, ttl)
	return true
}

// GetSet atomically sets key to value and returns the previous value (if any).
func (m *MemStore) GetSet(key, value string, ttl time.Duration) (prev string, existed bool) {
	unlock := m.lockKey(key)
	defer unlock()
	prev, existed = m.Get(key)
	m.Set(key, value, ttl)
	return prev, existed
}

// Append concatenates value to the existing string at key (creating it when
// absent) and returns the new length. The key's TTL is preserved.
func (m *MemStore) Append(key, value string) int {
	unlock := m.lockKey(key)
	defer unlock()
	cur, _ := m.Get(key)
	next := cur + value
	ttl := m.remainingTTL(key)
	m.Set(key, next, ttl)
	return len(next)
}

// Decr atomically decrements the integer at key by delta.
func (m *MemStore) Decr(key string, delta int64) (int64, error) {
	return m.Incr(key, -delta)
}

// remainingTTL returns the live TTL to re-apply on a read-modify-write, or 0
// when the key has no expiry / does not exist.
func (m *MemStore) remainingTTL(key string) time.Duration {
	remaining, persists, ok := m.cache.TTL(key)
	if ok && !persists && remaining > 0 {
		return remaining
	}
	return 0
}

// KVItem is a single key/value pair used by batch operations.
type KVItem struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

// MSet writes many pairs at once. A single ttl is applied to every pair
// (ttl <= 0 means no expiry). Returns the number of keys written.
func (m *MemStore) MSet(items []KVItem, ttl time.Duration) int {
	for _, it := range items {
		v := newStringValue(it.Value)
		m.cache.Set(it.Key, v, ttl, kvSize(it.Key, v))
	}
	if len(items) > 0 {
		m.markDirty()
	}
	return len(items)
}

// MGet reads many keys at once. The returned slice is index-aligned with keys;
// missing keys produce a KVItem with Value "" and are reported in the second
// return value.
func (m *MemStore) MGet(keys []string) (items []KVItem, missing []string) {
	items = make([]KVItem, 0, len(keys))
	for _, k := range keys {
		if v, ok := m.Get(k); ok {
			items = append(items, KVItem{Key: k, Value: v})
		} else {
			missing = append(missing, k)
		}
	}
	return items, missing
}

// DeleteMany removes many keys at once and returns how many existed.
func (m *MemStore) DeleteMany(keys []string) int {
	removed := 0
	for _, k := range keys {
		if m.cache.Delete(k) {
			removed++
		}
	}
	if removed > 0 {
		m.markDirty()
	}
	return removed
}

// IncrByFloat atomically adds a floating-point delta to the value at key.
func (m *MemStore) IncrByFloat(key string, delta float64) (float64, error) {
	unlock := m.lockKey(key)
	defer unlock()
	cur := float64(0)
	if v, ok := m.Get(key); ok {
		n, err := strconv.ParseFloat(v, 64)
		if err != nil {
			return 0, err
		}
		cur = n
	}
	cur += delta
	m.Set(key, strconv.FormatFloat(cur, 'g', -1, 64), m.remainingTTL(key))
	return cur, nil
}
