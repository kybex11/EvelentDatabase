package db

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type Database struct {
	rootDir        string
	collections    map[string]*Collection
	encKey         []byte
	kv             *MemStore
	broker         *Broker
	syncMode       SyncMode
	metaCacheBytes int64
	mu             sync.RWMutex
}

// MemStoreOptions tunes the embedded in-memory key/value store. The zero value
// is valid and yields an unbounded store that snapshots every few seconds.
type MemStoreOptions struct {
	MaxItems   int
	MaxBytes   int64
	Shards     int
	FlushEvery time.Duration
	SweepEvery time.Duration
}

// DatabaseOptions configures both the KV store and document durability.
type DatabaseOptions struct {
	KV             MemStoreOptions
	SyncMode       SyncMode
	MetaCacheBytes int64 // Pebble block cache per collection; 0 = 256 MiB
}

func NewDatabase(rootDir string) (*Database, error) {
	return NewDatabaseWithOptions(rootDir, MemStoreOptions{})
}

// NewDatabaseWithOptions opens (or creates) a database at rootDir and brings up
// the embedded in-memory store with the supplied bounds.
func NewDatabaseWithOptions(rootDir string, kvOpts MemStoreOptions) (*Database, error) {
	return OpenDatabase(rootDir, DatabaseOptions{KV: kvOpts})
}

// OpenDatabase opens a database with full options (KV + sync mode).
func OpenDatabase(rootDir string, opts DatabaseOptions) (*Database, error) {
	if err := os.MkdirAll(rootDir, 0755); err != nil {
		return nil, err
	}
	encKey, err := EnsureEncryptionKey(rootDir)
	if err != nil {
		return nil, err
	}
	db := &Database{
		rootDir:        rootDir,
		collections:    make(map[string]*Collection),
		encKey:         encKey,
		syncMode:       opts.SyncMode,
		metaCacheBytes: opts.MetaCacheBytes,
	}
	kvOpts := opts.KV
	flushEvery := kvOpts.FlushEvery
	if flushEvery == 0 {
		flushEvery = 5 * time.Second
	}
	sweepEvery := kvOpts.SweepEvery
	if sweepEvery == 0 {
		sweepEvery = time.Second
	}
	kv, err := NewMemStore(MemStoreConfig{
		MaxItems:     kvOpts.MaxItems,
		MaxBytes:     kvOpts.MaxBytes,
		Shards:       kvOpts.Shards,
		SnapshotPath: filepath.Join(rootDir, ".kvstore"),
		EncKey:       encKey,
		FlushEvery:   flushEvery,
		SweepEvery:   sweepEvery,
	})
	if err != nil {
		return nil, err
	}
	db.kv = kv
	db.broker = NewBroker(64)
	entries, err := os.ReadDir(rootDir)
	if err != nil {
		return nil, err
	}
	for _, e := range entries {
		if !e.IsDir() || strings.HasPrefix(e.Name(), ".") {
			continue
		}
		collDir := filepath.Join(rootDir, e.Name())
		coll, err := NewCollectionWithOptions(e.Name(), collDir, db.encKey, db.syncMode, db.metaCacheBytes)
		if err != nil {
			fmt.Printf("Warning: cannot load collection %s: %v\n", e.Name(), err)
			continue
		}
		db.collections[e.Name()] = coll
	}
	return db, nil
}

func (db *Database) GetCollection(name string) (*Collection, error) {
	db.mu.RLock()
	coll, ok := db.collections[name]
	db.mu.RUnlock()
	if ok {
		return coll, nil
	}
	db.mu.Lock()
	defer db.mu.Unlock()
	if coll, ok = db.collections[name]; ok {
		return coll, nil
	}
	collDir := filepath.Join(db.rootDir, name)
	coll, err := NewCollectionWithOptions(name, collDir, db.encKey, db.syncMode, db.metaCacheBytes)
	if err != nil {
		return nil, err
	}
	db.collections[name] = coll
	return coll, nil
}

func (db *Database) DropCollection(name string) error {
	db.mu.Lock()
	defer db.mu.Unlock()
	coll, ok := db.collections[name]
	if !ok {
		return fmt.Errorf("collection %s does not exist", name)
	}
	if err := coll.Drop(); err != nil {
		return err
	}
	delete(db.collections, name)
	return nil
}

func (db *Database) ListCollections() ([]string, error) {
	db.mu.RLock()
	defer db.mu.RUnlock()
	names := make([]string, 0, len(db.collections))
	for n := range db.collections {
		names = append(names, n)
	}
	return names, nil
}

// SetSyncMode applies a durability policy to all currently open collections
// and to collections opened later.
func (db *Database) SetSyncMode(mode SyncMode) {
	db.mu.Lock()
	db.syncMode = mode
	for _, coll := range db.collections {
		coll.SetSyncMode(mode)
	}
	db.mu.Unlock()
}

func (db *Database) SyncMode() SyncMode {
	db.mu.RLock()
	defer db.mu.RUnlock()
	return db.syncMode
}

// CompactAll runs compaction on every open collection.
func (db *Database) CompactAll() error {
	db.mu.RLock()
	colls := make([]*Collection, 0, len(db.collections))
	for _, c := range db.collections {
		colls = append(colls, c)
	}
	db.mu.RUnlock()
	var first error
	for _, c := range colls {
		if err := c.Compact(); err != nil && first == nil {
			first = err
		}
	}
	return first
}

// KV returns the embedded in-memory key/value store.
func (db *Database) KV() *MemStore {
	return db.kv
}

// PubSub returns the in-process publish/subscribe broker.
func (db *Database) PubSub() *Broker {
	return db.broker
}

// Close flushes the in-memory store and all open collections.
func (db *Database) Close() error {
	db.mu.Lock()
	defer db.mu.Unlock()
	var first error
	for name, coll := range db.collections {
		if err := coll.Close(); err != nil && first == nil {
			first = fmt.Errorf("close collection %s: %w", name, err)
		}
	}
	if db.kv != nil {
		if err := db.kv.Close(); err != nil && first == nil {
			first = err
		}
	}
	return first
}
