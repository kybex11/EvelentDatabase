package db

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
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

	readers      *segReaders
	docCache     *LRU
	indexesDirty bool
	syncMode     SyncMode
	segDirty     bool

	stopCh  chan struct{}
	flushWG sync.WaitGroup
	stopped bool
	mu      sync.RWMutex
}

// SetSyncMode updates the durability policy for segment appends.
func (c *Collection) SetSyncMode(mode SyncMode) {
	c.mu.Lock()
	c.syncMode = mode
	c.mu.Unlock()
}

func (c *Collection) SyncMode() SyncMode {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.syncMode
}

func NewCollection(name, dir string, encKey []byte) (*Collection, error) {
	return NewCollectionWithSync(name, dir, encKey, SyncNone)
}

func NewCollectionWithSync(name, dir string, encKey []byte, syncMode SyncMode) (*Collection, error) {
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
		readers:  newSegReaders(segDir),
		docCache: NewLRU(DefaultDocCacheItems, DefaultDocCacheBytes),
		syncMode: syncMode,
		stopCh:   make(chan struct{}),
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
	coll.flushWG.Add(1)
	go coll.metaFlushLoop(DefaultMetaFlushEvery)
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
	switch c.syncMode {
	case SyncEveryWrite:
		_ = c.curFile.Sync()
	default:
		c.segDirty = true
	}
	return DocLoc{Seg: c.curSeg, Offset: off, Length: length}, nil
}

func (c *Collection) markIndexesDirty() {
	c.indexesDirty = true
}

func (c *Collection) metaFlushLoop(every time.Duration) {
	defer c.flushWG.Done()
	t := time.NewTicker(every)
	defer t.Stop()
	compactTick := 0
	for {
		select {
		case <-c.stopCh:
			return
		case <-t.C:
			c.mu.Lock()
			if c.segDirty && c.syncMode == SyncEverySecond && c.curFile != nil {
				_ = c.curFile.Sync()
				c.segDirty = false
			}
			_ = c.flushMetaLocked()
			compactTick++
			if compactTick >= 30 { // ~every 30s
				compactTick = 0
				_ = c.maybeCompactLocked()
			}
			c.mu.Unlock()
		}
	}
}

func (c *Collection) Drop() error {
	c.mu.Lock()
	if !c.stopped {
		c.stopped = true
		close(c.stopCh)
	}
	c.mu.Unlock()
	c.flushWG.Wait()

	c.mu.Lock()
	defer c.mu.Unlock()
	if c.curFile != nil {
		_ = c.curFile.Close()
		c.curFile = nil
	}
	if c.readers != nil {
		c.readers.closeAll()
	}
	return os.RemoveAll(c.dir)
}

func (c *Collection) Close() error {
	c.mu.Lock()
	if !c.stopped {
		c.stopped = true
		close(c.stopCh)
	}
	c.mu.Unlock()
	c.flushWG.Wait()

	c.mu.Lock()
	defer c.mu.Unlock()
	if c.segDirty && c.curFile != nil {
		_ = c.curFile.Sync()
		c.segDirty = false
	}
	if err := c.flushMetaLocked(); err != nil {
		return err
	}
	if c.readers != nil {
		c.readers.closeAll()
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
	if !c.indexesDirty {
		return nil
	}
	for field, idx := range c.indexes {
		idxPath := filepath.Join(c.dir, "indexes", field+".idx")
		if err := idx.Save(idxPath, c.encKey); err != nil {
			return err
		}
	}
	c.indexesDirty = false
	return nil
}

func (c *Collection) cachePut(id string, doc map[string]interface{}) {
	if c.docCache == nil || id == "" || doc == nil {
		return
	}
	c.docCache.Set(id, cloneDoc(doc), 0, approxDocBytes(id, doc))
}

func (c *Collection) cacheGet(id string) (map[string]interface{}, bool) {
	if c.docCache == nil {
		return nil, false
	}
	raw, ok := c.docCache.Get(id)
	if !ok {
		return nil, false
	}
	doc, ok := raw.(map[string]interface{})
	if !ok {
		return nil, false
	}
	return cloneDoc(doc), true
}

func (c *Collection) cacheInvalidate(id string) {
	if c.docCache != nil {
		c.docCache.Delete(id)
	}
}

func cloneDoc(doc map[string]interface{}) map[string]interface{} {
	out := make(map[string]interface{}, len(doc))
	for k, v := range doc {
		out[k] = v
	}
	return out
}

func approxDocBytes(id string, doc map[string]interface{}) int64 {
	// Cheap estimate: key + rough field overhead. Exact size is not required.
	n := int64(len(id) + 64)
	for k, v := range doc {
		n += int64(len(k) + 16)
		switch x := v.(type) {
		case string:
			n += int64(len(x))
		case []byte:
			n += int64(len(x))
		default:
			n += 24
		}
	}
	return n
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
	c.markIndexesDirty()
	c.cachePut(id, doc)
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
		c.cachePut(id, doc)
		ids = append(ids, id)
	}
	if len(ids) > 0 {
		c.markIndexesDirty()
	}
	return ids, nil
}

func (c *Collection) FindByID(id string) (map[string]interface{}, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if doc, ok := c.cacheGet(id); ok {
		return doc, nil
	}
	doc, err := c.readDocByID(id)
	if err != nil {
		return nil, err
	}
	// Upgrade to write briefly to populate cache? Skip — Set on LRU has its own lock.
	c.cachePut(id, doc)
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
	c.markIndexesDirty()
	c.cachePut(id, update)
	return nil
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
	c.markIndexesDirty()
	c.cacheInvalidate(id)
	return nil
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
				doc, err := c.readDocCached(id)
				if err != nil {
					continue
				}
				results = append(results, doc)
			}
			return c.finalizeFind(results, opts), nil
		}
	}

	// Fast path: indexed range ($gt/$gte/$lt/$lte on one field).
	if spec, ok := RangeIndexFilter(filter); ok {
		if idx, has := c.indexes[spec.Field]; has {
			ids := append([]string(nil), idx.FindRange(spec.Lo, spec.Hi, spec.LoIncl, spec.HiIncl)...)
			sort.Strings(ids)
			for _, id := range ids {
				if opts.AfterID != "" && id <= opts.AfterID {
					continue
				}
				doc, err := c.readDocCached(id)
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
		doc, err := c.readDocCached(id)
		if err != nil {
			continue
		}
		if err := fn(id, doc); err != nil {
			return err
		}
	}
	return nil
}

func (c *Collection) readDocCached(id string) (map[string]interface{}, error) {
	if doc, ok := c.cacheGet(id); ok {
		return doc, nil
	}
	doc, err := c.readDocByID(id)
	if err != nil {
		return nil, err
	}
	c.cachePut(id, doc)
	return doc, nil
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
	var raw []byte
	var err error
	if c.readers != nil {
		raw, err = c.readers.readAt(loc.Seg, loc.Offset, loc.Length)
	} else {
		raw, err = readAtSegment(segmentPath(c.segDir, loc.Seg), loc.Offset, loc.Length)
	}
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

// CollectionStats is a richer snapshot for observability endpoints.
type CollectionStats struct {
	Docs      int64    `json:"docs"`
	Bytes     int64    `json:"bytes"`
	Segments  int      `json:"segments"`
	Indexes   []string `json:"indexes"`
	SyncMode  string   `json:"syncMode"`
	DocCache  LRUStats `json:"docCache"`
}

func (c *Collection) Stats() (docCount int64, totalBytes int64, err error) {
	st, err := c.StatsDetailed()
	return st.Docs, st.Bytes, err
}

func (c *Collection) StatsDetailed() (CollectionStats, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	st := CollectionStats{
		Docs:     c.docIndex.LiveCount(),
		Bytes:    c.docIndex.ApproximateBytes(),
		SyncMode: c.syncMode.String(),
		Indexes:  make([]string, 0, len(c.indexes)),
	}
	if c.docCache != nil {
		st.DocCache = c.docCache.Stats()
	}
	for f := range c.indexes {
		st.Indexes = append(st.Indexes, f)
	}
	sort.Strings(st.Indexes)
	if nums, err := listSegmentNumbers(c.segDir); err == nil {
		st.Segments = len(nums)
	}

	err := c.walkLegacyDocFiles(func(path string) error {
		id := legacyIDFromPath(path)
		if c.docIndex.Has(id) {
			return nil
		}
		info, err := os.Stat(path)
		if err != nil {
			return nil
		}
		st.Docs++
		st.Bytes += info.Size()
		return nil
	})
	return st, err
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
	if err := idx.Save(idxPath, c.encKey); err != nil {
		return err
	}
	return nil
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

// Compact rewrites all live documents into fresh segment files and drops
// orphaned payloads left by updates/deletes. Safe to call concurrently with
// the meta flusher (takes the write lock).
func (c *Collection) Compact() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.compactLocked()
}

func (c *Collection) maybeCompactLocked() error {
	live := c.docIndex.ApproximateBytes()
	nums, err := listSegmentNumbers(c.segDir)
	if err != nil || len(nums) == 0 {
		return err
	}
	var total int64
	for _, n := range nums {
		total += fileSize(segmentPath(c.segDir, n))
	}
	if total < CompactMinSegmentBytes || live*2 >= total {
		return nil
	}
	return c.compactLocked()
}

func (c *Collection) compactLocked() error {
	ids := c.docIndex.LiveIDs()
	if len(ids) == 0 {
		// Drop empty segments except keep a fresh writable one.
		oldNums, _ := listSegmentNumbers(c.segDir)
		if c.curFile != nil {
			_ = c.curFile.Close()
			c.curFile = nil
		}
		c.readers.closeAll()
		for _, n := range oldNums {
			_ = os.Remove(segmentPath(c.segDir, n))
		}
		c.curSeg = 1
		f, err := openAppend(segmentPath(c.segDir, c.curSeg))
		if err != nil {
			return err
		}
		c.curFile = f
		c.curSize = 0
		c.docIndex.dirty = true
		return c.docIndex.Save()
	}

	// Read all live docs first (from old segments / cache / legacy).
	type liveDoc struct {
		id  string
		doc map[string]interface{}
	}
	lives := make([]liveDoc, 0, len(ids))
	for _, id := range ids {
		doc, err := c.readDocByID(id)
		if err != nil {
			continue
		}
		lives = append(lives, liveDoc{id: id, doc: doc})
	}

	oldNums, err := listSegmentNumbers(c.segDir)
	if err != nil {
		return err
	}
	if c.curFile != nil {
		_ = c.curFile.Sync()
		_ = c.curFile.Close()
		c.curFile = nil
	}
	c.readers.closeAll()

	// Start writing into a new segment number past the old ones.
	var next uint32 = 1
	for _, n := range oldNums {
		if n >= next {
			next = n + 1
		}
	}
	c.curSeg = next
	f, err := openAppend(segmentPath(c.segDir, c.curSeg))
	if err != nil {
		return err
	}
	c.curFile = f
	c.curSize = 0

	newIndex := newDocIndex(c.docIndex.path, c.encKey)
	for _, ld := range lives {
		data, err := encodeDocument(c.encKey, ld.doc)
		if err != nil {
			return err
		}
		loc, err := c.appendDocLocked(data)
		if err != nil {
			return err
		}
		newIndex.Put(ld.id, loc)
		c.cachePut(ld.id, ld.doc)
	}
	newIndex.dirty = true
	c.docIndex = newIndex
	if err := c.docIndex.Save(); err != nil {
		return err
	}
	if err := c.curFile.Sync(); err != nil {
		return err
	}

	// Remove old segment files (not the ones we just wrote).
	written := make(map[uint32]struct{})
	for _, loc := range c.docIndex.locs {
		if !loc.Dead {
			written[loc.Seg] = struct{}{}
		}
	}
	written[c.curSeg] = struct{}{}
	for _, n := range oldNums {
		if _, keep := written[n]; keep {
			continue
		}
		_ = os.Remove(segmentPath(c.segDir, n))
		c.readers.drop(n)
	}
	return nil
}
