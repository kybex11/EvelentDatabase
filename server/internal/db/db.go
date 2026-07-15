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
	rootDir     string
	collections map[string]*Collection
	encKey      []byte
	kv          *MemStore
	broker      *Broker
	mu          sync.RWMutex
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

func NewDatabase(rootDir string) (*Database, error) {
	return NewDatabaseWithOptions(rootDir, MemStoreOptions{})
}

// NewDatabaseWithOptions opens (or creates) a database at rootDir and brings up
// the embedded in-memory store with the supplied bounds.
func NewDatabaseWithOptions(rootDir string, kvOpts MemStoreOptions) (*Database, error) {
	if err := os.MkdirAll(rootDir, 0755); err != nil {
		return nil, err
	}
	encKey, err := EnsureEncryptionKey(rootDir)
	if err != nil {
		return nil, err
	}
	db := &Database{
		rootDir:     rootDir,
		collections: make(map[string]*Collection),
		encKey:      encKey,
	}
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
		coll, err := NewCollection(e.Name(), collDir, db.encKey)
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
	coll, err := NewCollection(name, collDir, db.encKey)
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
