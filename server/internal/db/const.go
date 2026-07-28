package db

import "time"

const (
	DefaultFindLimit = 500
	MaxFindLimit     = 100000
	MaxSortCollect   = 200000

	// Per-collection hot document cache bounds.
	DefaultDocCacheItems = 8192
	DefaultDocCacheBytes = int64(64 << 20) // 64 MiB

	// How often dirty docindex / secondary indexes are flushed to disk.
	DefaultMetaFlushEvery = time.Second

	// Compact when live payload is less than half of on-disk segment bytes
	// and total segment size exceeds this threshold.
	CompactMinSegmentBytes = int64(8 << 20) // 8 MiB
)
