package db

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
)

type Collection struct {
	name    string
	dir     string
	docsDir string
	indexes map[string]*Index
	encKey  []byte
	mu      sync.RWMutex
}

func NewCollection(name, dir string, encKey []byte) (*Collection, error) {
	docsDir := filepath.Join(dir, "docs")
	indexesDir := filepath.Join(dir, "indexes")
	if err := os.MkdirAll(docsDir, 0755); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(indexesDir, 0755); err != nil {
		return nil, err
	}
	coll := &Collection{
		name:    name,
		dir:     dir,
		docsDir: docsDir,
		indexes: make(map[string]*Index),
		encKey:  encKey,
	}
	entries, err := os.ReadDir(indexesDir)
	if err != nil {
		return nil, err
	}
	for _, e := range entries {
		if !e.IsDir() && filepath.Ext(e.Name()) == ".idx" {
			idxPath := filepath.Join(indexesDir, e.Name())
			idx, err := LoadIndex(idxPath, encKey)
			if err != nil {
				fmt.Printf("Warning: cannot load index %s: %v\n", e.Name(), err)
				continue
			}
			idx.field = trimIdxFieldName(e.Name())
			coll.indexes[idx.field] = idx
		}
	}
	return coll, nil
}

func (c *Collection) Drop() error {
	return os.RemoveAll(c.dir)
}

func (c *Collection) Insert(doc map[string]interface{}) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	id, ok := doc["_id"].(string)
	if !ok || id == "" {
		id = generateID()
		doc["_id"] = id
	}

	docPath := filepath.Join(c.docsDir, id+".json")
	data, err := encodeDocument(c.encKey, doc)
	if err != nil {
		return "", err
	}
	if err := AtomicWriteFile(docPath, data, 0644); err != nil {
		return "", err
	}

	for _, idx := range c.indexes {
		idx.Insert(doc)
	}
	return id, nil
}

func (c *Collection) InsertMany(docs []map[string]interface{}) ([]string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	ids := make([]string, 0, len(docs))
	for _, doc := range docs {
		if doc == nil {
			continue
		}
		id, ok := doc["_id"].(string)
		if !ok || id == "" {
			id = generateID()
			doc["_id"] = id
		}
		docPath := filepath.Join(c.docsDir, id+".json")
		data, err := encodeDocument(c.encKey, doc)
		if err != nil {
			return nil, err
		}
		if err := AtomicWriteFile(docPath, data, 0644); err != nil {
			return nil, err
		}
		for _, idx := range c.indexes {
			idx.Insert(doc)
		}
		ids = append(ids, id)
	}
	return ids, nil
}

func (c *Collection) FindByID(id string) (map[string]interface{}, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	docPath := filepath.Join(c.docsDir, id+".json")
	data, err := os.ReadFile(docPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("document not found")
		}
		return nil, err
	}
	var doc map[string]interface{}
	if err := decodeDocument(c.encKey, data, &doc); err != nil {
		return nil, err
	}
	return doc, nil
}

func (c *Collection) Update(id string, update map[string]interface{}) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	docPath := filepath.Join(c.docsDir, id+".json")
	data, err := os.ReadFile(docPath)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("document not found")
		}
		return err
	}
	var oldDoc map[string]interface{}
	if err := decodeDocument(c.encKey, data, &oldDoc); err != nil {
		return err
	}
	update["_id"] = id
	out, err := encodeDocument(c.encKey, update)
	if err != nil {
		return err
	}
	if err := AtomicWriteFile(docPath, out, 0644); err != nil {
		return err
	}
	for _, idx := range c.indexes {
		idx.Delete(oldDoc)
		idx.Insert(update)
	}
	return nil
}

func (c *Collection) Delete(id string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	docPath := filepath.Join(c.docsDir, id+".json")
	data, err := os.ReadFile(docPath)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("document not found")
		}
		return err
	}
	var doc map[string]interface{}
	if err := decodeDocument(c.encKey, data, &doc); err != nil {
		return err
	}
	if err := os.Remove(docPath); err != nil {
		return err
	}
	for _, idx := range c.indexes {
		idx.Delete(doc)
	}
	return nil
}

func (c *Collection) Find(filter map[string]interface{}, opts *FindOptions) ([]map[string]interface{}, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if opts == nil {
		opts = &FindOptions{Limit: -1}
	}

	var results []map[string]interface{}
	entries, err := os.ReadDir(c.docsDir)
	if err != nil {
		return nil, err
	}
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".json" {
			continue
		}
		path := filepath.Join(c.docsDir, e.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		var doc map[string]interface{}
		if err := decodeDocument(c.encKey, data, &doc); err != nil {
			continue
		}
		if MatchesFilter(doc, filter) {
			results = append(results, doc)
		}
	}

	if opts.SortField != "" {
		field := opts.SortField
		desc := opts.SortDesc
		sort.SliceStable(results, func(i, j int) bool {
			vi, _ := results[i][field]
			vj, _ := results[j][field]
			cmp := CompareValues(vi, vj)
			if desc {
				return cmp > 0
			}
			return cmp < 0
		})
	}

	if opts.Skip > 0 {
		if opts.Skip >= len(results) {
			results = []map[string]interface{}{}
		} else {
			results = results[opts.Skip:]
		}
	}
	if opts.Limit >= 0 && len(results) > opts.Limit {
		results = results[:opts.Limit]
	}

	if results == nil {
		results = []map[string]interface{}{}
	}
	return results, nil
}

func (c *Collection) Stats() (docCount int64, totalBytes int64, err error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	entries, err := os.ReadDir(c.docsDir)
	if err != nil {
		return 0, 0, err
	}
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".json" {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		docCount++
		totalBytes += info.Size()
	}
	return docCount, totalBytes, nil
}

func (c *Collection) ListIndexes() []string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	out := make([]string, 0, len(c.indexes))
	for f := range c.indexes {
		out = append(out, f)
	}
	sort.Strings(out)
	return out
}

func (c *Collection) CreateIndex(field string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if _, ok := c.indexes[field]; ok {
		return nil
	}
	idx := NewIndex(field)
	entries, err := os.ReadDir(c.docsDir)
	if err != nil {
		return err
	}
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".json" {
			continue
		}
		path := filepath.Join(c.docsDir, e.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		var doc map[string]interface{}
		if err := decodeDocument(c.encKey, data, &doc); err != nil {
			continue
		}
		idx.Insert(doc)
	}
	c.indexes[field] = idx
	idxPath := filepath.Join(c.dir, "indexes", field+".idx")
	return idx.Save(idxPath, c.encKey)
}

func (c *Collection) DropIndex(field string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if _, ok := c.indexes[field]; !ok {
		return fmt.Errorf("index not found")
	}
	delete(c.indexes, field)
	idxPath := filepath.Join(c.dir, "indexes", field+".idx")
	return os.Remove(idxPath)
}

func trimIdxFieldName(idxFileName string) string {
	return strings.TrimSuffix(idxFileName, ".idx")
}
