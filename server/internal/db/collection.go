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
	name    string
	dir     string
	docsDir string
	segDir  string
	encKey  []byte
	meta    *MetaStore
	curSeg  uint32
	curFile *os.File
	curSize int64

	readers  *segReaders
	docCache *LRU
	syncMode SyncMode
	segDirty bool

	// indexFields is a RAM mirror of registered secondary indexes (source of
	// truth is MetaStore); avoids a disk round-trip on every write.
	indexFields map[string]struct{}

	stopCh  chan struct{}
	flushWG sync.WaitGroup
	stopped bool
	mu      sync.RWMutex
}

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
	return NewCollectionWithOptions(name, dir, encKey, syncMode, DefaultMetaCacheBytes)
}

func NewCollectionWithOptions(name, dir string, encKey []byte, syncMode SyncMode, metaCacheBytes int64) (*Collection, error) {
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
	meta, err := OpenMetaStore(dir, metaCacheBytes)
	if err != nil {
		return nil, fmt.Errorf("open meta store: %w", err)
	}
	if err := meta.MigrateFromLegacy(dir, encKey); err != nil {
		_ = meta.Close()
		return nil, fmt.Errorf("migrate legacy meta: %w", err)
	}
	fields, _ := meta.ListIndexes()
	indexFields := make(map[string]struct{}, len(fields))
	for _, f := range fields {
		indexFields[f] = struct{}{}
	}
	coll := &Collection{
		name:        name,
		dir:         dir,
		docsDir:     docsDir,
		segDir:      segDir,
		encKey:      encKey,
		meta:        meta,
		readers:     newSegReaders(segDir),
		docCache:    NewLRU(DefaultDocCacheItems, DefaultDocCacheBytes),
		syncMode:    syncMode,
		indexFields: indexFields,
		stopCh:      make(chan struct{}),
	}
	if err := coll.openLatestSegment(); err != nil {
		_ = meta.Close()
		return nil, err
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
			if c.meta != nil && c.syncMode != SyncNone {
				_ = c.meta.Flush()
			}
			compactTick++
			if compactTick >= 30 {
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
	if c.meta != nil {
		_ = c.meta.Close()
		c.meta = nil
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
	if c.meta != nil {
		_ = c.meta.Flush()
	}
	if c.readers != nil {
		c.readers.closeAll()
	}
	var first error
	if c.curFile != nil {
		_ = c.curFile.Sync()
		if err := c.curFile.Close(); err != nil {
			first = err
		}
		c.curFile = nil
	}
	if c.meta != nil {
		if err := c.meta.Close(); err != nil && first == nil {
			first = err
		}
		c.meta = nil
	}
	return first
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

func (c *Collection) indexDocLocked(doc map[string]interface{}) {
	id := docIDString(doc)
	for field := range c.indexFields {
		if val, ok := doc[field]; ok {
			_ = c.meta.IndexInsert(field, id, val)
		}
	}
}

func (c *Collection) unindexDocLocked(doc map[string]interface{}) {
	id := docIDString(doc)
	for field := range c.indexFields {
		if val, ok := doc[field]; ok {
			_ = c.meta.IndexDelete(field, id, val)
		}
	}
}

func (c *Collection) Insert(doc map[string]interface{}) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	id, ok := doc["_id"].(string)
	if !ok || id == "" {
		id = generateID()
		doc["_id"] = id
	}
	if c.meta.Has(id) {
		return "", fmt.Errorf("document already exists")
	}
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
	if err := c.meta.PutLoc(id, loc); err != nil {
		return "", err
	}
	c.indexDocLocked(doc)
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
		if err := c.meta.PutLoc(id, loc); err != nil {
			return nil, err
		}
		c.indexDocLocked(doc)
		c.cachePut(id, doc)
		ids = append(ids, id)
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
	if err := c.meta.PutLoc(id, loc); err != nil {
		return err
	}
	_ = c.removeLegacyFile(id)
	c.unindexDocLocked(oldDoc)
	c.indexDocLocked(update)
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
	if err := c.meta.DeleteLoc(id); err != nil {
		return err
	}
	_ = c.removeLegacyFile(id)
	c.unindexDocLocked(doc)
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
	needSort := opts.SortField != ""
	limitHit := func() bool {
		if needSort {
			return len(results) >= MaxSortCollect
		}
		return opts.Skip == 0 && opts.Limit >= 0 && len(results) >= opts.Limit
	}

	// Fast path: indexed equality (streaming — no giant []string).
	if field, value, ok := EqualityIndexFilter(filter); ok {
		if _, has := c.indexFields[field]; has {
			err := c.meta.IndexFindEq(field, value, func(id string) error {
				if opts.AfterID != "" && id <= opts.AfterID {
					return nil
				}
				doc, err := c.readDocCached(id)
				if err != nil {
					return nil
				}
				results = append(results, doc)
				if limitHit() {
					return errFindLimitReached
				}
				return nil
			})
			if err != nil && err != errFindLimitReached {
				return nil, err
			}
			return c.finalizeFind(results, opts), nil
		}
	}

	// Fast path: indexed range.
	if spec, ok := RangeIndexFilter(filter); ok {
		if _, has := c.indexFields[spec.Field]; has {
			err := c.streamRange(spec, opts, &results, limitHit)
			if err != nil && err != errFindLimitReached {
				return nil, err
			}
			return c.finalizeFind(results, opts), nil
		}
	}

	// Full scan via Pebble iterator (does NOT load all IDs into RAM).
	after := opts.AfterID
	err := c.meta.ForEachDocAfter(after, func(id string, _ DocLoc) error {
		doc, err := c.readDocCached(id)
		if err != nil {
			return nil
		}
		if !MatchesFilter(doc, filter) {
			return nil
		}
		results = append(results, doc)
		if limitHit() {
			return errFindLimitReached
		}
		return nil
	})
	if err != nil && err != errFindLimitReached {
		return nil, err
	}

	// Legacy files not in meta (rare after migration).
	_ = c.walkLegacyDocFiles(func(path string) error {
		id := legacyIDFromPath(path)
		if c.meta.Has(id) {
			return nil
		}
		if after != "" && id <= after {
			return nil
		}
		doc, err := c.readDocByID(id)
		if err != nil {
			return nil
		}
		if !MatchesFilter(doc, filter) {
			return nil
		}
		results = append(results, doc)
		if limitHit() {
			return errFindLimitReached
		}
		return nil
	})

	return c.finalizeFind(results, opts), nil
}

func (c *Collection) streamRange(spec RangeSpec, opts *FindOptions, results *[]map[string]interface{}, limitHit func() bool) error {
	add := func(id string) error {
		if opts.AfterID != "" && id <= opts.AfterID {
			return nil
		}
		doc, err := c.readDocCached(id)
		if err != nil {
			return nil
		}
		*results = append(*results, doc)
		if limitHit() {
			return errFindLimitReached
		}
		return nil
	}

	var loF, hiF *float64
	var loS, hiS *string
	if spec.Lo != nil {
		if n, ok := toFloat64(spec.Lo); ok {
			loF = &n
		} else if s, ok := spec.Lo.(string); ok {
			loS = &s
		}
	}
	if spec.Hi != nil {
		if n, ok := toFloat64(spec.Hi); ok {
			hiF = &n
		} else if s, ok := spec.Hi.(string); ok {
			hiS = &s
		}
	}
	if loF != nil || hiF != nil || (spec.Lo == nil && spec.Hi == nil) {
		// Prefer numeric path when either bound is numeric.
		if loS == nil && hiS == nil {
			return c.meta.IndexFindRangeNum(spec.Field, loF, hiF, spec.LoIncl, spec.HiIncl, add)
		}
	}
	return c.meta.IndexFindRangeStr(spec.Field, loS, hiS, spec.LoIncl, spec.HiIncl, add)
}

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
	nextSkip := opts.Skip + len(docs)
	return docs, EncodeCursorSkip(nextSkip), nil
}

var errFindLimitReached = fmt.Errorf("find limit reached")

func (c *Collection) forEachDocumentLocked(fn func(id string, doc map[string]interface{}) error) error {
	err := c.meta.ForEachDoc(func(id string, _ DocLoc) error {
		doc, err := c.readDocCached(id)
		if err != nil {
			return nil
		}
		return fn(id, doc)
	})
	if err != nil {
		return err
	}
	return c.walkLegacyDocFiles(func(path string) error {
		id := legacyIDFromPath(path)
		if c.meta.Has(id) {
			return nil
		}
		doc, err := c.readDocByID(id)
		if err != nil {
			return nil
		}
		return fn(id, doc)
	})
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
	loc, ok := c.meta.GetLoc(id)
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
			cmp := CompareValues(results[i][field], results[j][field])
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

type CollectionStats struct {
	Docs     int64    `json:"docs"`
	Bytes    int64    `json:"bytes"`
	Segments int      `json:"segments"`
	Indexes  []string `json:"indexes"`
	SyncMode string   `json:"syncMode"`
	DocCache LRUStats `json:"docCache"`
	Engine   string   `json:"engine"`
}

func (c *Collection) Stats() (docCount int64, totalBytes int64, err error) {
	st, err := c.StatsDetailed()
	return st.Docs, st.Bytes, err
}

func (c *Collection) StatsDetailed() (CollectionStats, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	st := CollectionStats{
		Docs:     c.meta.LiveCount(),
		Bytes:    c.meta.LiveBytes(),
		SyncMode: c.syncMode.String(),
		Indexes:  make([]string, 0, len(c.indexFields)),
		Engine:   "pebble+segments",
	}
	if c.docCache != nil {
		st.DocCache = c.docCache.Stats()
	}
	for f := range c.indexFields {
		st.Indexes = append(st.Indexes, f)
	}
	sort.Strings(st.Indexes)
	if nums, err := listSegmentNumbers(c.segDir); err == nil {
		st.Segments = len(nums)
	}
	err := c.walkLegacyDocFiles(func(path string) error {
		id := legacyIDFromPath(path)
		if c.meta.Has(id) {
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
	out := make([]string, 0, len(c.indexFields))
	for f := range c.indexFields {
		out = append(out, f)
	}
	sort.Strings(out)
	return out
}

func (c *Collection) CreateIndex(field string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if _, ok := c.indexFields[field]; ok {
		return nil
	}
	if err := c.meta.RegisterIndex(field); err != nil {
		return err
	}
	// Backfill from existing docs (streaming).
	err := c.forEachDocumentLocked(func(_ string, doc map[string]interface{}) error {
		id := docIDString(doc)
		if val, ok := doc[field]; ok {
			return c.meta.IndexInsert(field, id, val)
		}
		return nil
	})
	if err != nil {
		return err
	}
	c.indexFields[field] = struct{}{}
	return nil
}

func (c *Collection) DropIndex(field string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, ok := c.indexFields[field]; !ok {
		return fmt.Errorf("index not found")
	}
	if err := c.meta.UnregisterIndex(field); err != nil {
		return err
	}
	delete(c.indexFields, field)
	return nil
}

func trimIdxFieldName(idxFileName string) string {
	return strings.TrimSuffix(idxFileName, ".idx")
}

func (c *Collection) Compact() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.compactLocked()
}

func (c *Collection) maybeCompactLocked() error {
	live := c.meta.LiveBytes()
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

// compactLocked rewrites live payloads into new segments without holding all
// documents in RAM — streams id→doc→append one at a time.
func (c *Collection) compactLocked() error {
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

	writtenSegs := map[uint32]struct{}{c.curSeg: {}}
	var liveN int
	err = c.meta.ForEachDoc(func(id string, _ DocLoc) error {
		doc, err := c.readDocByID(id)
		if err != nil {
			return nil
		}
		data, err := encodeDocument(c.encKey, doc)
		if err != nil {
			return err
		}
		loc, err := c.appendDocLocked(data)
		if err != nil {
			return err
		}
		if err := c.meta.PutLoc(id, loc); err != nil {
			return err
		}
		writtenSegs[loc.Seg] = struct{}{}
		c.cachePut(id, doc)
		liveN++
		return nil
	})
	if err != nil {
		return err
	}

	if c.curFile != nil {
		_ = c.curFile.Sync()
	}
	_ = c.meta.Flush()

	// Drop any FDs re-opened during the rewrite so Windows can unlink.
	c.readers.closeAll()

	for _, n := range oldNums {
		if _, keep := writtenSegs[n]; keep {
			continue
		}
		_ = os.Remove(segmentPath(c.segDir, n))
		c.readers.drop(n)
	}
	if liveN == 0 && c.curFile != nil {
		// keep empty writable segment
	}
	return nil
}
