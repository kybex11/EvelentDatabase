package db

// SyncMode controls how aggressively document segment writes are fsynced.
// Meta (docindex / secondary indexes) is still flushed on the background
// interval and on Close, independent of this setting.
type SyncMode int

const (
	// SyncNone never fsyncs segment appends on the write path (fastest).
	// OS page cache may delay durability until Close / rotate / crash recovery
	// window. Default.
	SyncNone SyncMode = iota

	// SyncEverySecond fsyncs dirty segment files from the meta flush loop
	// (~1s). Good balance of speed and durability.
	SyncEverySecond

	// SyncEveryWrite fsyncs after every document append. Slowest, safest
	// single-node durability for the payload itself.
	SyncEveryWrite
)

func ParseSyncMode(s string) SyncMode {
	switch s {
	case "every_sec", "every-second", "1s", "sec":
		return SyncEverySecond
	case "every_write", "every-write", "sync", "full":
		return SyncEveryWrite
	default:
		return SyncNone
	}
}

func (m SyncMode) String() string {
	switch m {
	case SyncEverySecond:
		return "every_sec"
	case SyncEveryWrite:
		return "every_write"
	default:
		return "none"
	}
}
