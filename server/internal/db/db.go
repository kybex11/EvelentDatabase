package db

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

type Database struct {
	rootDir     string
	collections map[string]*Collection
	encKey      []byte
	mu          sync.RWMutex
}

func NewDatabase(rootDir string) (*Database, error) {
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
