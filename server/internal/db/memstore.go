package db

import (
	"encoding/json"
	"fmt"
	"strconv"
	"sync"
	"time"
)

const memOpStripes = 64

// MemStore is a built-in, Redis-like in-memory key/value store. It keeps string
// values in a bounded LRU cache with optional per-key TTL and persists its
// contents to an encrypted snapshot on disk so data survives restarts.
//
// It is deliberately *not* Redis: there is no separate process, no network
// protocol of its own, and no external dependency. It lives inside the database
// engine and is reached through the same HTTP API and SDKs.
type MemStore struct {
	cache        kvCache
	snapshotPath string
	encKey       []byte

	mu        sync.Mutex
	opStripes [memOpStripes]sync.Mutex // striped RMW locks (Incr, hash/list/set, …)
	stopCh    chan struct{}
	stopped   bool
	dirty     bool
	flushEach time.Duration
}

func (m *MemStore) lockKey(key string) func() {
	mu := &m.opStripes[fnv32a(key)&(memOpStripes-1)]
	mu.Lock()
	return mu.Unlock
}

// MemStoreConfig controls the in-memory store bounds and persistence.
type MemStoreConfig struct {
	MaxItems     int           // 0 = unbounded by count
	MaxBytes     int64         // 0 = unbounded by size
	Shards       int           // 0 = auto (sized to the machine); 1 = single lock
	SnapshotPath string        // empty = persistence disabled
	EncKey       []byte        // at-rest key for the snapshot (may be nil)
	FlushEvery   time.Duration // background snapshot interval (0 = manual only)
	SweepEvery   time.Duration // background TTL sweep interval (0 = lazy only)
}

// NewMemStore builds a key/value store and loads any existing snapshot. The
// backing cache is sharded (striped locks) for concurrent throughput; set
// Shards to 1 to force a single lock.
func NewMemStore(cfg MemStoreConfig) (*MemStore, error) {
	shards := cfg.Shards
	if shards == 0 {
		shards = defaultShardCount()
	}
	var backend kvCache
	if shards <= 1 {
		backend = NewLRU(cfg.MaxItems, cfg.MaxBytes)
	} else {
		backend = NewShardedLRU(shards, cfg.MaxItems, cfg.MaxBytes)
	}
	ms := &MemStore{
		cache:        backend,
		snapshotPath: cfg.SnapshotPath,
		encKey:       cfg.EncKey,
		stopCh:       make(chan struct{}),
		flushEach:    cfg.FlushEvery,
	}
	if ms.snapshotPath != "" {
		if err := ms.load(); err != nil {
			return nil, err
		}
	}
	if cfg.SweepEvery > 0 {
		go ms.sweepLoop(cfg.SweepEvery)
	}
	if cfg.FlushEvery > 0 && ms.snapshotPath != "" {
		go ms.flushLoop(cfg.FlushEvery)
	}
	return ms, nil
}

func kvSize(key string, v *Value) int64 {
	return int64(len(key)) + v.size()
}

// setValue stores a typed value under key and marks the store dirty.
func (m *MemStore) setValue(key string, v *Value, ttl time.Duration) {
	m.cache.Set(key, v, ttl, kvSize(key, v))
	m.markDirty()
}

// getValue returns the typed value stored at key.
func (m *MemStore) getValue(key string) (*Value, bool) {
	raw, ok := m.cache.Get(key)
	if !ok {
		return nil, false
	}
	v, ok := raw.(*Value)
	return v, ok
}

// Set stores a string value under key. ttl <= 0 stores it without expiry.
func (m *MemStore) Set(key, value string, ttl time.Duration) {
	m.setValue(key, newStringValue(value), ttl)
}

// Get returns the string value for key. ok is false when the key is absent or
// holds a non-string type.
func (m *MemStore) Get(key string) (string, bool) {
	v, ok := m.getValue(key)
	if !ok || v.Kind != KindString {
		return "", false
	}
	return v.Str, true
}

// Type reports the kind of value stored at key.
func (m *MemStore) Type(key string) (ValueKind, bool) {
	v, ok := m.getValue(key)
	if !ok {
		return 0, false
	}
	return v.Kind, true
}

// Delete removes key and reports whether it existed.
func (m *MemStore) Delete(key string) bool {
	ok := m.cache.Delete(key)
	if ok {
		m.markDirty()
	}
	return ok
}

// Exists reports whether key is present and not expired.
func (m *MemStore) Exists(key string) bool {
	_, ok := m.cache.Get(key)
	return ok
}

// TTL exposes the remaining lifetime for key.
func (m *MemStore) TTL(key string) (remaining time.Duration, persists bool, ok bool) {
	return m.cache.TTL(key)
}

// Expire sets or clears a key's TTL (ttl <= 0 clears it).
func (m *MemStore) Expire(key string, ttl time.Duration) bool {
	ok := m.cache.Expire(key, ttl)
	if ok {
		m.markDirty()
	}
	return ok
}

// Incr atomically increments the integer value stored at key by delta,
// creating it at 0 when absent. Returns the new value.
func (m *MemStore) Incr(key string, delta int64) (int64, error) {
	unlock := m.lockKey(key)
	defer unlock()
	cur := int64(0)
	if v, ok := m.Get(key); ok {
		n, err := strconv.ParseInt(v, 10, 64)
		if err != nil {
			return 0, fmt.Errorf("value at %q is not an integer", key)
		}
		cur = n
	}
	cur += delta
	remaining, persists, ok := m.cache.TTL(key)
	ttl := time.Duration(0)
	if ok && !persists && remaining > 0 {
		ttl = remaining
	}
	m.Set(key, strconv.FormatInt(cur, 10), ttl)
	return cur, nil
}

// Keys returns all live keys, optionally filtered by prefix.
func (m *MemStore) Keys(prefix string) []string {
	return m.cache.Keys(prefix)
}

// Flush removes every key.
func (m *MemStore) Flush() {
	m.cache.Flush()
	m.markDirty()
}

// Stats returns cache counters for observability.
func (m *MemStore) Stats() LRUStats {
	return m.cache.Stats()
}

func (m *MemStore) markDirty() {
	m.mu.Lock()
	m.dirty = true
	m.mu.Unlock()
}

// persistItem is the on-disk representation of one entry, carrying the typed
// value so hashes/lists/sets survive a restart with their kinds intact.
type persistItem struct {
	Key       string `json:"k"`
	ExpiresAt int64  `json:"e,omitempty"`
	Value     *Value `json:"v"`
}

// snapshotFile is the on-disk layout of the persisted store.
type snapshotFile struct {
	Version int           `json:"version"`
	SavedAt int64         `json:"savedAt"`
	Items   []persistItem `json:"items"`
}

// Save writes the current contents to the encrypted snapshot file. It is a
// no-op when persistence is disabled.
func (m *MemStore) Save() error {
	if m.snapshotPath == "" {
		return nil
	}
	raw := m.cache.Snapshot()
	items := make([]persistItem, 0, len(raw))
	for _, it := range raw {
		v, ok := it.Value.(*Value)
		if !ok {
			continue
		}
		items = append(items, persistItem{Key: it.Key, ExpiresAt: it.ExpiresAt, Value: v})
	}
	snap := snapshotFile{
		Version: 2,
		SavedAt: time.Now().Unix(),
		Items:   items,
	}
	data, err := json.Marshal(snap)
	if err != nil {
		return err
	}
	blob, err := EncodePayload(m.encKey, data)
	if err != nil {
		// no key configured: persist as plaintext JSON
		blob = data
	}
	if err := AtomicWriteFile(m.snapshotPath, blob, 0600); err != nil {
		return err
	}
	m.mu.Lock()
	m.dirty = false
	m.mu.Unlock()
	return nil
}

func (m *MemStore) load() error {
	raw, err := readFileIfExists(m.snapshotPath)
	if err != nil {
		return err
	}
	if raw == nil {
		return nil
	}
	plain, err := DecodePayload(m.encKey, raw)
	if err != nil {
		plain = raw
	}
	var snap snapshotFile
	if err := json.Unmarshal(plain, &snap); err != nil {
		return fmt.Errorf("corrupt memstore snapshot: %w", err)
	}
	for _, it := range snap.Items {
		if it.Value == nil {
			continue
		}
		m.cache.RestoreItem(LRUItem{Key: it.Key, Value: it.Value, ExpiresAt: it.ExpiresAt}, kvSize(it.Key, it.Value))
	}
	return nil
}

func (m *MemStore) sweepLoop(every time.Duration) {
	t := time.NewTicker(every)
	defer t.Stop()
	for {
		select {
		case <-m.stopCh:
			return
		case <-t.C:
			m.cache.PurgeExpired()
		}
	}
}

func (m *MemStore) flushLoop(every time.Duration) {
	t := time.NewTicker(every)
	defer t.Stop()
	for {
		select {
		case <-m.stopCh:
			return
		case <-t.C:
			m.mu.Lock()
			dirty := m.dirty
			m.mu.Unlock()
			if dirty {
				_ = m.Save()
			}
		}
	}
}

// Close stops background workers and flushes a final snapshot to disk.
func (m *MemStore) Close() error {
	m.mu.Lock()
	if m.stopped {
		m.mu.Unlock()
		return nil
	}
	m.stopped = true
	close(m.stopCh)
	m.mu.Unlock()
	return m.Save()
}
