package db

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"sync/atomic"

	"github.com/cockroachdb/pebble"
)

// Key layout inside the per-collection Pebble store (TB-scale metadata):
//
//	d/<id>                         → DocLoc (17B)
//	f/<field>                      → 1  (index registered)
//	i/<field>\x00e/<eqKey>\x00<id> → []  equality posting
//	i/<field>\x00n/<float8>\x00<id>→ []  numeric range posting
//	i/<field>\x00s/<str>\x00<id>   → []  string range posting
//	c/count                        → int64 live docs
//	c/bytes                        → int64 approx live payload bytes
//
// Document payloads stay in append-only segment files; only pointers + indexes
// live in Pebble so RAM stays bounded by the LSM cache, not by doc count.

const (
	metaDirName = "meta.pebble"

	pfxDoc    byte = 'd'
	pfxField  byte = 'f'
	pfxIndex  byte = 'i'
	pfxCount  byte = 'c'
)

// MetaStore is the on-disk catalog for one collection.
type MetaStore struct {
	db    *pebble.DB
	dir   string
	cache *pebble.Cache

	// Hot counters mirrored in memory; authoritative copy is in Pebble.
	liveCount atomic.Int64
	liveBytes atomic.Int64
}

func metaPath(collDir string) string {
	return filepath.Join(collDir, metaDirName)
}

// OpenMetaStore opens (or creates) the Pebble catalog for a collection.
// cacheBytes bounds the block cache (0 → 256 MiB default).
func OpenMetaStore(collDir string, cacheBytes int64) (*MetaStore, error) {
	if cacheBytes <= 0 {
		cacheBytes = DefaultMetaCacheBytes
	}
	dir := metaPath(collDir)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, err
	}
	cache := pebble.NewCache(cacheBytes)
	opts := &pebble.Options{
		Cache:                       cache,
		MemTableSize:                64 << 20,
		MemTableStopWritesThreshold: 4,
		LBaseMaxBytes:               256 << 20,
		Levels:                      make([]pebble.LevelOptions, 7),
		MaxOpenFiles:                4000,
	}
	for i := range opts.Levels {
		l := &opts.Levels[i]
		l.Compression = pebble.SnappyCompression
		l.TargetFileSize = 32 << 20
		if i > 0 {
			l.TargetFileSize = opts.Levels[i-1].TargetFileSize * 2
		}
	}
	opts.EnsureDefaults()

	pdb, err := pebble.Open(dir, opts)
	if err != nil {
		cache.Unref()
		return nil, err
	}
	ms := &MetaStore{db: pdb, dir: dir, cache: cache}
	ms.loadCounters()
	return ms, nil
}

func (m *MetaStore) loadCounters() {
	if v, closer, err := m.db.Get([]byte{pfxCount, '/', 'n'}); err == nil {
		m.liveCount.Store(decodeInt64(v))
		_ = closer.Close()
	}
	if v, closer, err := m.db.Get([]byte{pfxCount, '/', 'b'}); err == nil {
		m.liveBytes.Store(decodeInt64(v))
		_ = closer.Close()
	}
}

func (m *MetaStore) persistCounters(batch *pebble.Batch) {
	batch.Set([]byte{pfxCount, '/', 'n'}, encodeInt64(m.liveCount.Load()), nil)
	batch.Set([]byte{pfxCount, '/', 'b'}, encodeInt64(m.liveBytes.Load()), nil)
}

func docKey(id string) []byte {
	b := make([]byte, 2+len(id))
	b[0] = pfxDoc
	b[1] = '/'
	copy(b[2:], id)
	return b
}

func fieldKey(field string) []byte {
	b := make([]byte, 2+len(field))
	b[0] = pfxField
	b[1] = '/'
	copy(b[2:], field)
	return b
}

func eqIndexKey(field, eqKey, id string) []byte {
	// i/<field>\x00e/<eqKey>\x00<id>
	var buf bytes.Buffer
	buf.Grow(8 + len(field) + len(eqKey) + len(id))
	buf.WriteByte(pfxIndex)
	buf.WriteByte('/')
	buf.WriteString(field)
	buf.WriteByte(0)
	buf.WriteByte('e')
	buf.WriteByte('/')
	buf.WriteString(eqKey)
	buf.WriteByte(0)
	buf.WriteString(id)
	return buf.Bytes()
}

func numIndexKey(field string, n float64, id string) []byte {
	var buf bytes.Buffer
	buf.Grow(16 + len(field) + len(id))
	buf.WriteByte(pfxIndex)
	buf.WriteByte('/')
	buf.WriteString(field)
	buf.WriteByte(0)
	buf.WriteByte('n')
	buf.WriteByte('/')
	buf.Write(encodeFloatSortable(n))
	buf.WriteByte(0)
	buf.WriteString(id)
	return buf.Bytes()
}

func strIndexKey(field, s, id string) []byte {
	var buf bytes.Buffer
	buf.Grow(8 + len(field) + len(s) + len(id))
	buf.WriteByte(pfxIndex)
	buf.WriteByte('/')
	buf.WriteString(field)
	buf.WriteByte(0)
	buf.WriteByte('s')
	buf.WriteByte('/')
	buf.WriteString(s)
	buf.WriteByte(0)
	buf.WriteString(id)
	return buf.Bytes()
}

func indexPrefix(field string, kind byte) []byte {
	var buf bytes.Buffer
	buf.WriteByte(pfxIndex)
	buf.WriteByte('/')
	buf.WriteString(field)
	buf.WriteByte(0)
	buf.WriteByte(kind)
	buf.WriteByte('/')
	return buf.Bytes()
}

// GetLoc returns the segment location for id.
func (m *MetaStore) GetLoc(id string) (DocLoc, bool) {
	v, closer, err := m.db.Get(docKey(id))
	if err != nil {
		return DocLoc{}, false
	}
	defer closer.Close()
	loc, ok := decodeDocLoc(v)
	if !ok || loc.Dead {
		return DocLoc{}, false
	}
	return loc, true
}

func (m *MetaStore) Has(id string) bool {
	_, ok := m.GetLoc(id)
	return ok
}

// PutLoc stores/replaces a document location and adjusts live counters.
func (m *MetaStore) PutLoc(id string, loc DocLoc) error {
	loc.Dead = false
	key := docKey(id)
	batch := m.db.NewBatch()
	defer batch.Close()

	oldLive := false
	var oldBytes int64
	if v, closer, err := m.db.Get(key); err == nil {
		if old, ok := decodeDocLoc(v); ok && !old.Dead {
			oldLive = true
			oldBytes = int64(old.Length) + 4
		}
		_ = closer.Close()
	}
	newBytes := int64(loc.Length) + 4
	if err := batch.Set(key, encodeDocLoc(loc), nil); err != nil {
		return err
	}
	if !oldLive {
		m.liveCount.Add(1)
		m.liveBytes.Add(newBytes)
	} else {
		m.liveBytes.Add(newBytes - oldBytes)
	}
	m.persistCounters(batch)
	return batch.Commit(pebble.NoSync)
}

// DeleteLoc removes a document pointer.
func (m *MetaStore) DeleteLoc(id string) error {
	key := docKey(id)
	batch := m.db.NewBatch()
	defer batch.Close()
	if v, closer, err := m.db.Get(key); err == nil {
		if old, ok := decodeDocLoc(v); ok && !old.Dead {
			m.liveCount.Add(-1)
			m.liveBytes.Add(-(int64(old.Length) + 4))
		}
		_ = closer.Close()
	}
	if err := batch.Delete(key, nil); err != nil {
		return err
	}
	m.persistCounters(batch)
	return batch.Commit(pebble.NoSync)
}

func (m *MetaStore) LiveCount() int64 { return m.liveCount.Load() }
func (m *MetaStore) LiveBytes() int64 { return m.liveBytes.Load() }

// ForEachDoc streams live id→loc pairs in id order without loading all keys.
func (m *MetaStore) ForEachDoc(fn func(id string, loc DocLoc) error) error {
	prefix := []byte{pfxDoc, '/'}
	iter, err := m.db.NewIter(&pebble.IterOptions{
		LowerBound: prefix,
		UpperBound: prefixUpper(prefix),
	})
	if err != nil {
		return err
	}
	defer iter.Close()
	for iter.First(); iter.Valid(); iter.Next() {
		k := iter.Key()
		if len(k) < 2 {
			continue
		}
		id := string(k[2:])
		loc, ok := decodeDocLoc(iter.Value())
		if !ok || loc.Dead {
			continue
		}
		if err := fn(id, loc); err != nil {
			return err
		}
	}
	return iter.Error()
}

// ForEachDocAfter streams docs with id > after (keyset pagination).
func (m *MetaStore) ForEachDocAfter(after string, fn func(id string, loc DocLoc) error) error {
	start := docKey(after)
	// Seek to first key strictly after `after`.
	prefix := []byte{pfxDoc, '/'}
	iter, err := m.db.NewIter(&pebble.IterOptions{
		LowerBound: prefix,
		UpperBound: prefixUpper(prefix),
	})
	if err != nil {
		return err
	}
	defer iter.Close()
	if after == "" {
		iter.First()
	} else {
		iter.SeekGE(start)
		if iter.Valid() && string(iter.Key()) == string(start) {
			iter.Next()
		}
	}
	for ; iter.Valid(); iter.Next() {
		k := iter.Key()
		if len(k) < 2 || k[0] != pfxDoc {
			break
		}
		id := string(k[2:])
		loc, ok := decodeDocLoc(iter.Value())
		if !ok || loc.Dead {
			continue
		}
		if err := fn(id, loc); err != nil {
			return err
		}
	}
	return iter.Error()
}

func prefixUpper(prefix []byte) []byte {
	u := make([]byte, len(prefix))
	copy(u, prefix)
	for i := len(u) - 1; i >= 0; i-- {
		if u[i] < 0xff {
			u[i]++
			return u[:i+1]
		}
	}
	return nil // no upper bound
}

// --- secondary indexes ---

func (m *MetaStore) HasIndex(field string) bool {
	_, closer, err := m.db.Get(fieldKey(field))
	if err != nil {
		return false
	}
	_ = closer.Close()
	return true
}

func (m *MetaStore) ListIndexes() ([]string, error) {
	prefix := []byte{pfxField, '/'}
	iter, err := m.db.NewIter(&pebble.IterOptions{
		LowerBound: prefix,
		UpperBound: prefixUpper(prefix),
	})
	if err != nil {
		return nil, err
	}
	defer iter.Close()
	var out []string
	for iter.First(); iter.Valid(); iter.Next() {
		k := iter.Key()
		if len(k) > 2 {
			out = append(out, string(k[2:]))
		}
	}
	return out, iter.Error()
}

func (m *MetaStore) RegisterIndex(field string) error {
	return m.db.Set(fieldKey(field), []byte{1}, pebble.NoSync)
}

func (m *MetaStore) UnregisterIndex(field string) error {
	batch := m.db.NewBatch()
	defer batch.Close()
	_ = batch.Delete(fieldKey(field), nil)
	// Delete all postings for field (eq + num + str).
	for _, kind := range []byte{'e', 'n', 's'} {
		pfx := indexPrefix(field, kind)
		if err := batch.DeleteRange(pfx, prefixUpper(pfx), nil); err != nil {
			return err
		}
	}
	return batch.Commit(pebble.Sync)
}

// IndexInsert adds inverted postings for one document field value.
func (m *MetaStore) IndexInsert(field, id string, val interface{}) error {
	batch := m.db.NewBatch()
	defer batch.Close()
	if err := m.indexInsertBatch(batch, field, id, val); err != nil {
		return err
	}
	return batch.Commit(pebble.NoSync)
}

func (m *MetaStore) indexInsertBatch(batch *pebble.Batch, field, id string, val interface{}) error {
	eq := toIndexKey(val)
	if err := batch.Set(eqIndexKey(field, eq, id), nil, nil); err != nil {
		return err
	}
	if n, ok := toFloat64(val); ok {
		return batch.Set(numIndexKey(field, n, id), nil, nil)
	}
	if s, ok := val.(string); ok {
		return batch.Set(strIndexKey(field, s, id), nil, nil)
	}
	return nil
}

func (m *MetaStore) IndexDelete(field, id string, val interface{}) error {
	batch := m.db.NewBatch()
	defer batch.Close()
	eq := toIndexKey(val)
	_ = batch.Delete(eqIndexKey(field, eq, id), nil)
	if n, ok := toFloat64(val); ok {
		_ = batch.Delete(numIndexKey(field, n, id), nil)
	}
	if s, ok := val.(string); ok {
		_ = batch.Delete(strIndexKey(field, s, id), nil)
	}
	return batch.Commit(pebble.NoSync)
}

// IndexFindEq streams matching ids for equality (stops when fn returns err).
func (m *MetaStore) IndexFindEq(field string, value interface{}, fn func(id string) error) error {
	eq := toIndexKey(value)
	pfx := eqIndexKey(field, eq, "")
	// pfx ends with id=""; keys are pfx+id
	iter, err := m.db.NewIter(&pebble.IterOptions{
		LowerBound: pfx,
		UpperBound: prefixUpper(pfx),
	})
	if err != nil {
		return err
	}
	defer iter.Close()
	for iter.First(); iter.Valid(); iter.Next() {
		k := iter.Key()
		if !bytes.HasPrefix(k, pfx) {
			break
		}
		id := string(k[len(pfx):])
		if id == "" {
			continue
		}
		if err := fn(id); err != nil {
			return err
		}
	}
	return iter.Error()
}

// IndexFindRangeNum streams ids for numeric range queries.
func (m *MetaStore) IndexFindRangeNum(field string, lo, hi *float64, loIncl, hiIncl bool, fn func(id string) error) error {
	pfx := indexPrefix(field, 'n')
	var lower []byte
	if lo != nil {
		lower = append(append([]byte{}, pfx...), encodeFloatSortable(*lo)...)
	} else {
		lower = pfx
	}
	iter, err := m.db.NewIter(&pebble.IterOptions{
		LowerBound: lower,
		UpperBound: prefixUpper(pfx),
	})
	if err != nil {
		return err
	}
	defer iter.Close()

	start := iter.First
	if lo != nil {
		start = func() bool { return iter.SeekGE(lower) }
	}
	for ok := start(); ok; ok = iter.Next() {
		k := iter.Key()
		if !bytes.HasPrefix(k, pfx) {
			break
		}
		rest := k[len(pfx):]
		if len(rest) < 9 {
			continue
		}
		n, ok := decodeFloatSortable(rest[:8])
		if !ok || rest[8] != 0 {
			continue
		}
		id := string(rest[9:])
		if lo != nil {
			if loIncl {
				if n < *lo {
					continue
				}
			} else if n <= *lo {
				continue
			}
		}
		if hi != nil {
			if hiIncl {
				if n > *hi {
					break
				}
			} else if n >= *hi {
				break
			}
		}
		if err := fn(id); err != nil {
			return err
		}
	}
	return iter.Error()
}

// IndexFindRangeStr streams ids for string range queries.
func (m *MetaStore) IndexFindRangeStr(field string, lo, hi *string, loIncl, hiIncl bool, fn func(id string) error) error {
	pfx := indexPrefix(field, 's')
	var lower []byte
	if lo != nil {
		lower = append(append([]byte{}, pfx...), *lo...)
		lower = append(lower, 0)
	} else {
		lower = pfx
	}
	iter, err := m.db.NewIter(&pebble.IterOptions{
		LowerBound: lower,
		UpperBound: prefixUpper(pfx),
	})
	if err != nil {
		return err
	}
	defer iter.Close()
	start := iter.First
	if lo != nil {
		start = func() bool { return iter.SeekGE(lower) }
	}
	for ok := start(); ok; ok = iter.Next() {
		k := iter.Key()
		if !bytes.HasPrefix(k, pfx) {
			break
		}
		rest := k[len(pfx):]
		zi := bytes.IndexByte(rest, 0)
		if zi < 0 {
			continue
		}
		s := string(rest[:zi])
		id := string(rest[zi+1:])
		if lo != nil {
			if loIncl {
				if s < *lo {
					continue
				}
			} else if s <= *lo {
				continue
			}
		}
		if hi != nil {
			if hiIncl {
				if s > *hi {
					break
				}
			} else if s >= *hi {
				break
			}
		}
		if err := fn(id); err != nil {
			return err
		}
	}
	return iter.Error()
}

// Flush forces WAL sync (durable meta).
func (m *MetaStore) Flush() error {
	return m.db.Flush()
}

func (m *MetaStore) Close() error {
	err := m.db.Close()
	if m.cache != nil {
		m.cache.Unref()
	}
	return err
}

// MigrateFromLegacy imports an old in-memory/JSON DocIndex and .idx files once.
func (m *MetaStore) MigrateFromLegacy(collDir string, encKey []byte) error {
	// Already has data?
	if m.LiveCount() > 0 {
		return nil
	}
	legacyPath := docIndexPath(collDir)
	if raw, err := os.ReadFile(legacyPath); err == nil && len(raw) > 0 {
		plain, err := DecodePayload(encKey, raw)
		if err == nil {
			locs, err := decodeDocIndexBlob(plain)
			if err == nil && len(locs) > 0 {
				batch := m.db.NewBatch()
				var count, bytes int64
				for id, loc := range locs {
					if loc.Dead {
						continue
					}
					if err := batch.Set(docKey(id), encodeDocLoc(loc), nil); err != nil {
						batch.Close()
						return err
					}
					count++
					bytes += int64(loc.Length) + 4
				}
				m.liveCount.Store(count)
				m.liveBytes.Store(bytes)
				m.persistCounters(batch)
				if err := batch.Commit(pebble.Sync); err != nil {
					return err
				}
				_ = os.Rename(legacyPath, legacyPath+".migrated")
			}
		}
	}

	indexesDir := filepath.Join(collDir, "indexes")
	entries, err := os.ReadDir(indexesDir)
	if err != nil {
		return nil
	}
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".idx" {
			continue
		}
		field := trimIdxFieldName(e.Name())
		idx, err := LoadIndex(filepath.Join(indexesDir, e.Name()), encKey)
		if err != nil {
			continue
		}
		_ = m.RegisterIndex(field)
		batch := m.db.NewBatch()
		for eqKey, set := range idx.data {
			for id := range set {
				_ = batch.Set(eqIndexKey(field, eqKey, id), nil, nil)
			}
		}
		for _, b := range idx.nums {
			for id := range b.ids {
				_ = batch.Set(numIndexKey(field, b.v, id), nil, nil)
			}
		}
		for _, b := range idx.strs {
			for id := range b.ids {
				_ = batch.Set(strIndexKey(field, b.s, id), nil, nil)
			}
		}
		_ = batch.Commit(pebble.Sync)
		batch.Close()
		_ = os.Rename(filepath.Join(indexesDir, e.Name()), filepath.Join(indexesDir, e.Name()+".migrated"))
	}
	return nil
}

func (m *MetaStore) String() string {
	return fmt.Sprintf("MetaStore{docs=%d bytes=%d}", m.LiveCount(), m.LiveBytes())
}
