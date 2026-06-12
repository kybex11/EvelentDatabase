package db

import (
	"hash/fnv"
	"runtime"
	"time"
)

// kvCache is the set of operations MemStore needs from its backing cache. Both
// *LRU (single lock) and *ShardedLRU (striped locks) satisfy it, so the store
// can transparently scale from one shard to many.
type kvCache interface {
	Get(key string) (interface{}, bool)
	Set(key string, value interface{}, ttl time.Duration, size int64)
	Delete(key string) bool
	TTL(key string) (time.Duration, bool, bool)
	Expire(key string, ttl time.Duration) bool
	Keys(prefix string) []string
	Flush()
	PurgeExpired() int
	Stats() LRUStats
	Snapshot() []LRUItem
	RestoreItem(it LRUItem, size int64)
}

// compile-time checks
var (
	_ kvCache = (*LRU)(nil)
	_ kvCache = (*ShardedLRU)(nil)
)

// ShardedLRU partitions keys across N independent LRU shards, each with its own
// lock. This removes the single-mutex bottleneck under concurrent access: keys
// hashing to different shards never contend. Item/byte bounds are split evenly
// across shards.
type ShardedLRU struct {
	shards []*LRU
	mask   uint32
}

// defaultShardCount picks a power-of-two shard count sized to the machine,
// clamped to [1, 256]. More shards = less contention, more overhead.
func defaultShardCount() int {
	n := runtime.GOMAXPROCS(0) * 4
	if n < 1 {
		n = 1
	}
	// round up to the next power of two
	p := 1
	for p < n {
		p <<= 1
	}
	if p > 256 {
		p = 256
	}
	return p
}

// NewShardedLRU builds a striped cache with the given shard count and total
// bounds (maxItems / maxBytes are divided across shards; <= 0 disables a bound).
func NewShardedLRU(shardCount, maxItems int, maxBytes int64) *ShardedLRU {
	// normalize shardCount to a power of two >= 1
	if shardCount < 1 {
		shardCount = 1
	}
	p := 1
	for p < shardCount {
		p <<= 1
	}
	shardCount = p

	perItems := 0
	if maxItems > 0 {
		perItems = (maxItems + shardCount - 1) / shardCount
		if perItems < 1 {
			perItems = 1
		}
	}
	perBytes := int64(0)
	if maxBytes > 0 {
		perBytes = (maxBytes + int64(shardCount) - 1) / int64(shardCount)
		if perBytes < 1 {
			perBytes = 1
		}
	}

	s := &ShardedLRU{
		shards: make([]*LRU, shardCount),
		mask:   uint32(shardCount - 1),
	}
	for i := range s.shards {
		s.shards[i] = NewLRU(perItems, perBytes)
	}
	return s
}

func (s *ShardedLRU) shardFor(key string) *LRU {
	h := fnv.New32a()
	_, _ = h.Write([]byte(key))
	return s.shards[h.Sum32()&s.mask]
}

func (s *ShardedLRU) Get(key string) (interface{}, bool) {
	return s.shardFor(key).Get(key)
}

func (s *ShardedLRU) Set(key string, value interface{}, ttl time.Duration, size int64) {
	s.shardFor(key).Set(key, value, ttl, size)
}

func (s *ShardedLRU) Delete(key string) bool {
	return s.shardFor(key).Delete(key)
}

func (s *ShardedLRU) TTL(key string) (time.Duration, bool, bool) {
	return s.shardFor(key).TTL(key)
}

func (s *ShardedLRU) Expire(key string, ttl time.Duration) bool {
	return s.shardFor(key).Expire(key, ttl)
}

func (s *ShardedLRU) Keys(prefix string) []string {
	out := make([]string, 0, 64)
	for _, sh := range s.shards {
		out = append(out, sh.Keys(prefix)...)
	}
	return out
}

func (s *ShardedLRU) Flush() {
	for _, sh := range s.shards {
		sh.Flush()
	}
}

func (s *ShardedLRU) PurgeExpired() int {
	total := 0
	for _, sh := range s.shards {
		total += sh.PurgeExpired()
	}
	return total
}

func (s *ShardedLRU) Stats() LRUStats {
	var agg LRUStats
	for _, sh := range s.shards {
		st := sh.Stats()
		agg.Items += st.Items
		agg.Bytes += st.Bytes
		agg.MaxItems += st.MaxItems
		agg.MaxBytes += st.MaxBytes
		agg.Hits += st.Hits
		agg.Misses += st.Misses
		agg.Evictions += st.Evictions
		agg.Expired += st.Expired
	}
	total := agg.Hits + agg.Misses
	if total > 0 {
		agg.HitRatio = float64(agg.Hits) / float64(total)
	}
	return agg
}

func (s *ShardedLRU) Snapshot() []LRUItem {
	out := make([]LRUItem, 0, 64)
	for _, sh := range s.shards {
		out = append(out, sh.Snapshot()...)
	}
	return out
}

func (s *ShardedLRU) RestoreItem(it LRUItem, size int64) {
	s.shardFor(it.Key).RestoreItem(it, size)
}
