package db

import (
	"strconv"
	"sync/atomic"
	"testing"
	"time"
)

// These benchmarks contrast the single-lock LRU with the striped ShardedLRU
// under concurrent access. Run with:
//
//	go test ./internal/db -bench . -benchmem
//	go test ./internal/db -bench ParallelSet -cpu 1,4,8

func benchSetParallel(b *testing.B, c kvCache) {
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		var i int64
		for pb.Next() {
			n := atomic.AddInt64(&i, 1)
			k := "key:" + strconv.FormatInt(n&4095, 10)
			c.Set(k, newStringValue("v"), 0, 32)
		}
	})
}

func benchGetParallel(b *testing.B, c kvCache) {
	for i := 0; i < 4096; i++ {
		c.Set("key:"+strconv.Itoa(i), newStringValue("v"), 0, 32)
	}
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		var i int64
		for pb.Next() {
			n := atomic.AddInt64(&i, 1)
			c.Get("key:" + strconv.FormatInt(n&4095, 10))
		}
	})
}

func BenchmarkSingleLRU_ParallelSet(b *testing.B) {
	benchSetParallel(b, NewLRU(0, 0))
}

func BenchmarkShardedLRU_ParallelSet(b *testing.B) {
	benchSetParallel(b, NewShardedLRU(defaultShardCount(), 0, 0))
}

func BenchmarkSingleLRU_ParallelGet(b *testing.B) {
	benchGetParallel(b, NewLRU(0, 0))
}

func BenchmarkShardedLRU_ParallelGet(b *testing.B) {
	benchGetParallel(b, NewShardedLRU(defaultShardCount(), 0, 0))
}

func BenchmarkSingleLRU_MixedParallel(b *testing.B) {
	benchMixedParallel(b, NewLRU(0, 0))
}

func BenchmarkShardedLRU_MixedParallel(b *testing.B) {
	benchMixedParallel(b, NewShardedLRU(defaultShardCount(), 0, 0))
}

// benchMixedParallel runs a ~80/20 read/write mix, the common cache workload.
func benchMixedParallel(b *testing.B, c kvCache) {
	for i := 0; i < 4096; i++ {
		c.Set("key:"+strconv.Itoa(i), newStringValue("v"), 0, 32)
	}
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		var i int64
		for pb.Next() {
			n := atomic.AddInt64(&i, 1)
			k := "key:" + strconv.FormatInt(n&4095, 10)
			if n%5 == 0 {
				c.Set(k, newStringValue("v2"), 0, 32)
			} else {
				c.Get(k)
			}
		}
	})
}

// BenchmarkMemStoreIncr measures the atomic counter path end to end.
func BenchmarkMemStoreIncr(b *testing.B) {
	ms, err := NewMemStore(MemStoreConfig{})
	if err != nil {
		b.Fatal(err)
	}
	defer ms.Close()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := ms.Incr("counter", 1); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkLRUSetWithTTL exercises the TTL bookkeeping on the write path.
func BenchmarkLRUSetWithTTL(b *testing.B) {
	c := NewLRU(0, 0)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		c.Set("k", newStringValue("v"), time.Minute, 32)
	}
}
