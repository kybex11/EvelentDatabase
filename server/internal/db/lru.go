package db

import (
	"container/list"
	"sync"
	"time"
)

// lruEntry is a single slot in the LRU cache.
type lruEntry struct {
	key       string
	value     interface{}
	expiresAt int64 // unix nanoseconds; 0 means "never expires"
	size      int64 // approximate in-memory footprint in bytes
}

// LRU is a concurrency-safe, capacity-bounded cache with optional per-entry
// TTL. It backs the Redis-like key/value store and per-collection hot document
// caches. Bounds can be set on item count, on the approximate byte size, or
// both. When a bound is exceeded the least recently used entries are evicted.
type LRU struct {
	mu       sync.Mutex
	maxItems int
	maxBytes int64
	curBytes int64
	ll       *list.List
	items    map[string]*list.Element

	hits      uint64
	misses    uint64
	evictions uint64
	expiredN  uint64

	now func() time.Time
}

// LRUStats is a point-in-time snapshot of cache counters.
type LRUStats struct {
	Items     int     `json:"items"`
	Bytes     int64   `json:"bytes"`
	MaxItems  int     `json:"maxItems"`
	MaxBytes  int64   `json:"maxBytes"`
	Hits      uint64  `json:"hits"`
	Misses    uint64  `json:"misses"`
	Evictions uint64  `json:"evictions"`
	Expired   uint64  `json:"expired"`
	HitRatio  float64 `json:"hitRatio"`
}

// NewLRU builds a cache bounded by maxItems and/or maxBytes. A value <= 0
// disables that particular bound.
func NewLRU(maxItems int, maxBytes int64) *LRU {
	return &LRU{
		maxItems: maxItems,
		maxBytes: maxBytes,
		ll:       list.New(),
		items:    make(map[string]*list.Element),
		now:      time.Now,
	}
}

func (c *LRU) isExpired(e *lruEntry, nowNano int64) bool {
	return e.expiresAt != 0 && nowNano >= e.expiresAt
}

// Get returns the cached value for key and marks it most-recently-used.
// Expired entries are treated as a miss and dropped.
func (c *LRU) Get(key string) (interface{}, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	el, ok := c.items[key]
	if !ok {
		c.misses++
		return nil, false
	}
	ent := el.Value.(*lruEntry)
	if c.isExpired(ent, c.now().UnixNano()) {
		c.removeElement(el)
		c.expiredN++
		c.misses++
		return nil, false
	}
	c.ll.MoveToFront(el)
	c.hits++
	return ent.value, true
}

// Set inserts or replaces key with value. ttl <= 0 means no expiry. size is the
// approximate byte footprint used for the byte bound (pass 0 to ignore).
func (c *LRU) Set(key string, value interface{}, ttl time.Duration, size int64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	var exp int64
	if ttl > 0 {
		exp = c.now().Add(ttl).UnixNano()
	}
	if el, ok := c.items[key]; ok {
		ent := el.Value.(*lruEntry)
		c.curBytes += size - ent.size
		ent.value = value
		ent.expiresAt = exp
		ent.size = size
		c.ll.MoveToFront(el)
		c.evictIfNeeded()
		return
	}
	ent := &lruEntry{key: key, value: value, expiresAt: exp, size: size}
	el := c.ll.PushFront(ent)
	c.items[key] = el
	c.curBytes += size
	c.evictIfNeeded()
}

// Delete removes key and reports whether it existed.
func (c *LRU) Delete(key string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	el, ok := c.items[key]
	if !ok {
		return false
	}
	c.removeElement(el)
	return true
}

// TTL returns the remaining lifetime for key. ok is false when the key is
// absent or already expired; persists is true when the key has no expiry.
func (c *LRU) TTL(key string) (remaining time.Duration, persists bool, ok bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	el, exists := c.items[key]
	if !exists {
		return 0, false, false
	}
	ent := el.Value.(*lruEntry)
	nowNano := c.now().UnixNano()
	if c.isExpired(ent, nowNano) {
		c.removeElement(el)
		c.expiredN++
		return 0, false, false
	}
	if ent.expiresAt == 0 {
		return 0, true, true
	}
	return time.Duration(ent.expiresAt - nowNano), false, true
}

// Expire sets or clears the TTL on an existing key (ttl <= 0 clears it).
func (c *LRU) Expire(key string, ttl time.Duration) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	el, ok := c.items[key]
	if !ok {
		return false
	}
	ent := el.Value.(*lruEntry)
	if c.isExpired(ent, c.now().UnixNano()) {
		c.removeElement(el)
		c.expiredN++
		return false
	}
	if ttl > 0 {
		ent.expiresAt = c.now().Add(ttl).UnixNano()
	} else {
		ent.expiresAt = 0
	}
	return true
}

// Keys returns every live key. When prefix is non-empty only matching keys are
// returned. Expired keys are skipped (and lazily purged).
func (c *LRU) Keys(prefix string) []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	nowNano := c.now().UnixNano()
	out := make([]string, 0, len(c.items))
	for el := c.ll.Back(); el != nil; {
		prev := el.Prev()
		ent := el.Value.(*lruEntry)
		if c.isExpired(ent, nowNano) {
			c.removeElement(el)
			c.expiredN++
			el = prev
			continue
		}
		if prefix == "" || hasPrefix(ent.key, prefix) {
			out = append(out, ent.key)
		}
		el = prev
	}
	return out
}

// Flush drops every entry but keeps the configured bounds and counters.
func (c *LRU) Flush() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.ll.Init()
	c.items = make(map[string]*list.Element)
	c.curBytes = 0
}

// PurgeExpired actively removes entries whose TTL has elapsed and returns how
// many were dropped. Intended to be called from a background sweeper.
func (c *LRU) PurgeExpired() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	nowNano := c.now().UnixNano()
	removed := 0
	for el := c.ll.Back(); el != nil; {
		prev := el.Prev()
		ent := el.Value.(*lruEntry)
		if c.isExpired(ent, nowNano) {
			c.removeElement(el)
			c.expiredN++
			removed++
		}
		el = prev
	}
	return removed
}

// Stats returns a snapshot of the cache counters.
func (c *LRU) Stats() LRUStats {
	c.mu.Lock()
	defer c.mu.Unlock()
	total := c.hits + c.misses
	var ratio float64
	if total > 0 {
		ratio = float64(c.hits) / float64(total)
	}
	return LRUStats{
		Items:     len(c.items),
		Bytes:     c.curBytes,
		MaxItems:  c.maxItems,
		MaxBytes:  c.maxBytes,
		Hits:      c.hits,
		Misses:    c.misses,
		Evictions: c.evictions,
		Expired:   c.expiredN,
		HitRatio:  ratio,
	}
}

func (c *LRU) evictIfNeeded() {
	for c.maxItems > 0 && c.ll.Len() > c.maxItems {
		if !c.evictOldest() {
			break
		}
	}
	for c.maxBytes > 0 && c.curBytes > c.maxBytes && c.ll.Len() > 0 {
		if !c.evictOldest() {
			break
		}
	}
}

func (c *LRU) evictOldest() bool {
	el := c.ll.Back()
	if el == nil {
		return false
	}
	c.removeElement(el)
	c.evictions++
	return true
}

func (c *LRU) removeElement(el *list.Element) {
	ent := el.Value.(*lruEntry)
	c.ll.Remove(el)
	delete(c.items, ent.key)
	c.curBytes -= ent.size
	if c.curBytes < 0 {
		c.curBytes = 0
	}
}

func hasPrefix(s, prefix string) bool {
	if len(prefix) > len(s) {
		return false
	}
	return s[:len(prefix)] == prefix
}

// LRUItem is an exported view of a live cache entry, used for persistence.
type LRUItem struct {
	Key       string      `json:"k"`
	Value     interface{} `json:"v"`
	ExpiresAt int64       `json:"e,omitempty"` // unix nanoseconds, 0 = never
}

// Snapshot returns all live (non-expired) entries, oldest first. Expired
// entries are purged as a side effect.
func (c *LRU) Snapshot() []LRUItem {
	c.mu.Lock()
	defer c.mu.Unlock()
	nowNano := c.now().UnixNano()
	out := make([]LRUItem, 0, len(c.items))
	for el := c.ll.Back(); el != nil; {
		prev := el.Prev()
		ent := el.Value.(*lruEntry)
		if c.isExpired(ent, nowNano) {
			c.removeElement(el)
			c.expiredN++
			el = prev
			continue
		}
		out = append(out, LRUItem{Key: ent.key, Value: ent.value, ExpiresAt: ent.expiresAt})
		el = prev
	}
	return out
}

// RestoreItem reinserts a previously snapshotted entry, recomputing its TTL
// from the absolute expiry timestamp. Entries already past expiry are skipped.
func (c *LRU) RestoreItem(it LRUItem, size int64) {
	if it.ExpiresAt != 0 {
		now := c.now().UnixNano()
		if it.ExpiresAt <= now {
			return
		}
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if el, ok := c.items[it.Key]; ok {
		c.removeElement(el)
	}
	ent := &lruEntry{key: it.Key, value: it.Value, expiresAt: it.ExpiresAt, size: size}
	el := c.ll.PushFront(ent)
	c.items[it.Key] = el
	c.curBytes += size
	c.evictIfNeeded()
}
