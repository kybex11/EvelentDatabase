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
	name     string
	dir      string
	docsDir  string
	segDir   string
	indexes  map[string]*Index
	encKey   []byte
	docIndex *DocIndex
	curSeg   uint32
	curFile  *os.File
	curSize  int64
	mu       sync.RWMutex
}

func NewCollection(name, dir string, encKey []byte) (*Collection, error) {
	docsDir := filepath.Join(dir, "docs")
	segDir := filepath.Join(dir, "segments")
	indexesDir := filepath.Join(dir, "indexes")
	if err := os.MkdirAll(docsDir, 0755); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(segDir, 0755); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(indexesDir, 0755); err != nil {
		return nil, err
	}
	didx, err := loadDocIndex(docIndexPath(dir), encKey)
	if err != nil {
		return nil, fmt.Errorf("load docindex: %w", err)
	}
	coll := &Collection{
		name:     name,
		dir:      dir,
		docsDir:  docsDir,
		segDir:   segDir,
		indexes:  make(map[string]*Index),
		encKey:   encKey,
		docIndex: didx,
	}
	if err := coll.openLatestSegment(); err != nil {
		return nil, err
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

func (c *Collection) openLatestSegment() error {
	nums, err := listSegmentNumbers(c.segDir)
	if err != nil {
		return err
	}
	if len(nums) == 0 {
		c.curSeg = 1
		path := segmentPath(c.segDir, c.curSeg)
		f, err := openAppend(path)
		if err != nil {
			return err
		}
		c.curFile = f
		c.curSize = 0
		return nil
	}
	c.curSeg = nums[len(nums)-1]
	path := segmentPath(c.segDir, c.curSeg)
	c.curSize = fileSize(path)
	f, err := openAppend(path)
	if err != nil {
		return err
	}
	c.curFile = f
	return nil
}

func (c *Collection) rotateSegmentLocked() error {
	if c.curFile != nil {
		_ = c.curFile.Sync()
		_ = c.curFile.Close()
		c.curFile = nil
	}
	c.curSeg++
	path := segmentPath(c.segDir, c.curSeg)
	f, err := openAppend(path)
	if err != nil {
		return err
	}
	c.curFile = f
	c.curSize = 0
	return nil
}

func (c *Collection) appendDocLocked(ciphertext []byte) (DocLoc, error) {
	need := int64(4 + len(ciphertext))
	if c.curFile == nil || (c.curSize > 0 && c.curSize+need > MaxSegmentBytes) {
		if err := c.rotateSegmentLocked(); err != nil {
			return DocLoc{}, err
		}
	}
	off, length, err := appendEncrypted(c.curFile, ciphertext)
	if err != nil {
		return DocLoc{}, err
	}
	c.curSize += int64(4 + length)
	return DocLoc{Seg: c.curSeg, Offset: off, Length: length}, nil
}

func (c *Collection) Drop() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.curFile != nil {
		_ = c.curFile.Close()
		c.curFile = nil
	}
	return os.RemoveAll(c.dir)
}

func (c *Collection) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if err := c.flushMetaLocked(); err != nil {
		return err
	}
	if c.curFile != nil {
		_ = c.curFile.Sync()
		err := c.curFile.Close()
		c.curFile = nil
		return err
	}
	return nil
}

func (c *Collection) flushMetaLocked() error {
	if err := c.docIndex.Save(); err != nil {
		return err
	}
	for field, idx := range c.indexes {
		idxPath := filepath.Join(c.dir, "indexes", field+".idx")
		if err := idx.Save(idxPath, c.encKey); err != nil {
			return err
		}
	}
	return nil
}

func (c *Collection) Insert(doc map[string]interface{}) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	id, ok := doc["_id"].(string)
	if !ok || id == "" {
		id = generateID()
		doc["_id"] = id
	}
	if c.docIndex.Has(id) {
		return "", fmt.Errorf("document already exists")
	}
	// Legacy file collision
	if _, err := os.Stat(c.resolveDocPath(id)); err == nil {
		return "", fmt.Errorf("document already exists")
	}

	data, err := encodeDocument(c.encKey, doc)
	if err != nil {
		return "", err
	}
	loc, err := c.appendDocLocked(data)
	if err != nil {
		return "", err
	}
	c.docIndex.Put(id, loc)
	for _, idx := range c.indexes {
		idx.Insert(doc)
	}
	_ = c.docIndex.Save()
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
		data, err := encodeDocument(c.encKey, doc)
		if err != nil {
			return nil, err
		}
		loc, err := c.appendDocLocked(data)
		if err != nil {
			return nil, err
		}
		c.docIndex.Put(id, loc)
		for _, idx := range c.indexes {
			idx.Insert(doc)
		}
		ids = append(ids, id)
	}
	_ = c.docIndex.Save()
	return ids, nil
}

func (c *Collection) FindByID(id string) (map[string]interface{}, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	doc, err := c.readDocByID(id)
	if err != nil {
		return nil, err
	}
	return doc, nil
}

func (c *Collection) Update(id string, update map[string]interface{}) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	oldDoc, err := c.readDocByID(id)
	if err != nil {
		return err
	}
	update["_id"] = id
	out, err := encodeDocument(c.encKey, update)
	if err != nil {
		return err
	}
	loc, err := c.appendDocLocked(out)
	if err != nil {
		return err
	}
	c.docIndex.Put(id, loc)
	// Remove legacy file if present (lazy migrate).
	_ = c.removeLegacyFile(id)
	for _, idx := range c.indexes {
		idx.Delete(oldDoc)
		idx.Insert(update)
	}
	return c.docIndex.Save()
}

func (c *Collection) Delete(id string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	doc, err := c.readDocByID(id)
	if err != nil {
		return err
	}
	c.docIndex.Delete(id)
	_ = c.removeLegacyFile(id)
	for _, idx := range c.indexes {
		idx.Delete(doc)
	}
	return c.docIndex.Save()
}

func (c *Collection) removeLegacyFile(id string) error {
	candidates := []string{
		filepath.Join(c.docsDir, id+".json"),
	}
	sh := shardSegment(id)
	if sh != "" && sh != "_" {
		candidates = append(candidates, filepath.Join(c.docsDir, sh, id+".json"))
	}
	var last error
	for _, p := range candidates {
		if err := os.Remove(p); err != nil && !os.IsNotExist(err) {
			last = err
		}
	}
	return last
}

func (c *Collection) Find(filter map[string]interface{}, opts *FindOptions) ([]map[string]interface{}, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if opts == nil {
		opts = &FindOptions{Limit: -1}
	}

	var results []map[string]interface{}

	// Fast path: indexed equality.
	if field, value, ok := EqualityIndexFilter(filter); ok {
		if idx, has := c.indexes[field]; has {
			ids := append([]string(nil), idx.Find(value)...)
			sort.Strings(ids)
			for _, id := range ids {
				if opts.AfterID != "" && id <= opts.AfterID {
					continue
				}
				doc, err := c.readDocByID(id)
				if err != nil {
					continue
				}
				results = append(results, doc)
			}
			return c.finalizeFind(results, opts), nil
		}
	}

	// Streaming scan: segment index + legacy files, ordered by id when unsorted.
	needSort := opts.SortField != ""
	after := opts.AfterID

	err := c.forEachDocumentLocked(func(id string, doc map[string]interface{}) error {
		if after != "" && id <= after {
			return nil
		}
		if !MatchesFilter(doc, filter) {
			return nil
		}
		results = append(results, doc)
		if !needSort && opts.Skip == 0 && opts.Limit >= 0 && len(results) >= opts.Limit {
			return errFindLimitReached
		}
		if needSort && len(results) >= MaxSortCollect {
			return errFindLimitReached
		}
		return nil
	})
	if err != nil && err != errFindLimitReached {
		return nil, err
	}

	return c.finalizeFind(results, opts), nil
}

// FindPage is like Find but also returns a nextCursor for keyset pagination.
func (c *Collection) FindPage(filter map[string]interface{}, opts *FindOptions) (docs []map[string]interface{}, nextCursor string, err error) {
	docs, err = c.Find(filter, opts)
	if err != nil {
		return nil, "", err
	}
	if opts == nil {
		opts = &FindOptions{Limit: DefaultFindLimit}
	}
	if len(docs) == 0 || opts.Limit < 0 || len(docs) < opts.Limit {
		return docs, "", nil
	}
	last := docs[len(docs)-1]
	if id, ok := last["_id"].(string); ok && id != "" && opts.SortField == "" {
		return docs, EncodeCursorAfter(id), nil
	}
	// Fall back to skip-based cursor when sorted.
	nextSkip := opts.Skip + len(docs)
	return docs, EncodeCursorSkip(nextSkip), nil
}

var errFindLimitReached = fmt.Errorf("find limit reached")

func (c *Collection) forEachDocumentLocked(fn func(id string, doc map[string]interface{}) error) error {
	ids := make([]string, 0, len(c.docIndex.locs)+64)
	seen := make(map[string]struct{})

	for _, id := range c.docIndex.LiveIDs() {
		ids = append(ids, id)
		seen[id] = struct{}{}
	}

	_ = c.walkLegacyDocFiles(func(path string) error {
		id := legacyIDFromPath(path)
		if _, ok := seen[id]; ok {
			return nil
		}
		seen[id] = struct{}{}
		ids = append(ids, id)
		return nil
	})
	sort.Strings(ids)

	for _, id := range ids {
		doc, err := c.readDocByID(id)
		if err != nil {
			continue
		}
		if err := fn(id, doc); err != nil {
			return err
		}
	}
	return nil
}

func (c *Collection) readDocByID(id string) (map[string]interface{}, error) {
	if doc, err := c.readSegDoc(id); err == nil {
		return doc, nil
	}
	path := c.resolveDocPath(id)
	data, err := os.ReadFile(path)
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

func (c *Collection) readSegDoc(id string) (map[string]interface{}, error) {
	loc, ok := c.docIndex.Get(id)
	if !ok {
		return nil, fmt.Errorf("document not found")
	}
	raw, err := readAtSegment(segmentPath(c.segDir, loc.Seg), loc.Offset, loc.Length)
	if err != nil {
		return nil, err
	}
	var doc map[string]interface{}
	if err := decodeDocument(c.encKey, raw, &doc); err != nil {
		return nil, err
	}
	return doc, nil
}

func (c *Collection) finalizeFind(results []map[string]interface{}, opts *FindOptions) []map[string]interface{} {
	if opts.SortField != "" {
		field := opts.SortField
		desc := opts.SortDesc
		sort.SliceStable(results, func(i, j int) bool {
			vi := results[i][field]
			vj := results[j][field]
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
	return results
}

func (c *Collection) Stats() (docCount int64, totalBytes int64, err error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	docCount = c.docIndex.LiveCount()
	totalBytes = c.docIndex.ApproximateBytes()

	err = c.walkLegacyDocFiles(func(path string) error {
		id := legacyIDFromPath(path)
		if c.docIndex.Has(id) {
			return nil
		}
		info, err := os.Stat(path)
		if err != nil {
			return nil
		}
		docCount++
		totalBytes += info.Size()
		return nil
	})
	return docCount, totalBytes, err
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
	err := c.forEachDocumentLocked(func(_ string, doc map[string]interface{}) error {
		idx.Insert(doc)
		return nil
	})
	if err != nil {
		return err
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
