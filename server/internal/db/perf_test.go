package db

import (
	"os"
	"path/filepath"
	"testing"
)

func TestBatchedDocIndexFlush(t *testing.T) {
	dir := t.TempDir()
	key := make([]byte, 32)
	for i := range key {
		key[i] = byte(i + 7)
	}
	coll, err := NewCollection("batch", dir, key)
	if err != nil {
		t.Fatal(err)
	}

	const n = 50
	ids := make([]string, 0, n)
	for i := 0; i < n; i++ {
		id, err := coll.Insert(map[string]interface{}{"i": float64(i)})
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, id)
	}
	if err := coll.Close(); err != nil {
		t.Fatal(err)
	}

	coll2, err := NewCollection("batch", dir, key)
	if err != nil {
		t.Fatal(err)
	}
	defer coll2.Close()
	if coll2.docIndex.LiveCount() != n {
		t.Fatalf("live=%d want %d", coll2.docIndex.LiveCount(), n)
	}
	for _, id := range ids {
		if _, err := coll2.FindByID(id); err != nil {
			t.Fatalf("missing %s: %v", id, err)
		}
	}
}

func TestDocCacheHit(t *testing.T) {
	dir := t.TempDir()
	key := make([]byte, 32)
	for i := range key {
		key[i] = byte(i + 9)
	}
	coll, err := NewCollection("cache", dir, key)
	if err != nil {
		t.Fatal(err)
	}
	defer coll.Close()

	id, err := coll.Insert(map[string]interface{}{"name": "cached"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := coll.FindByID(id); err != nil {
		t.Fatal(err)
	}
	st := coll.docCache.Stats()
	if st.Hits < 1 {
		t.Fatalf("expected cache hit, stats=%+v", st)
	}
}

func TestCompactReclaimsSpace(t *testing.T) {
	dir := t.TempDir()
	key := make([]byte, 32)
	for i := range key {
		key[i] = byte(i + 11)
	}
	coll, err := NewCollection("compact", dir, key)
	if err != nil {
		t.Fatal(err)
	}

	id, err := coll.Insert(map[string]interface{}{"v": "one"})
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 20; i++ {
		if err := coll.Update(id, map[string]interface{}{"v": "x", "n": float64(i)}); err != nil {
			t.Fatal(err)
		}
	}
	before, _ := listSegmentNumbers(coll.segDir)
	var beforeBytes int64
	for _, n := range before {
		beforeBytes += fileSize(segmentPath(coll.segDir, n))
	}
	if err := coll.Compact(); err != nil {
		t.Fatal(err)
	}
	got, err := coll.FindByID(id)
	if err != nil {
		t.Fatal(err)
	}
	if got["n"].(float64) != 19 {
		t.Fatalf("got %#v", got)
	}
	after, _ := listSegmentNumbers(coll.segDir)
	var afterBytes int64
	for _, n := range after {
		afterBytes += fileSize(segmentPath(coll.segDir, n))
	}
	if afterBytes >= beforeBytes {
		t.Fatalf("expected smaller segments after compact: before=%d after=%d", beforeBytes, afterBytes)
	}
	if err := coll.Close(); err != nil {
		t.Fatal(err)
	}

	// Reload and verify.
	coll2, err := NewCollection("compact", dir, key)
	if err != nil {
		t.Fatal(err)
	}
	defer coll2.Close()
	got, err = coll2.FindByID(id)
	if err != nil {
		t.Fatal(err)
	}
	if got["n"].(float64) != 19 {
		t.Fatalf("reload %#v", got)
	}
	_ = os.RemoveAll(filepath.Join(dir, "x"))
}

func TestIndexSetDelete(t *testing.T) {
	idx := NewIndex("name")
	d1 := map[string]interface{}{"_id": "a", "name": "Ada"}
	d2 := map[string]interface{}{"_id": "b", "name": "Ada"}
	idx.Insert(d1)
	idx.Insert(d2)
	if len(idx.Find("Ada")) != 2 {
		t.Fatalf("find=%v", idx.Find("Ada"))
	}
	idx.Delete(d1)
	got := idx.Find("Ada")
	if len(got) != 1 || got[0] != "b" {
		t.Fatalf("after delete=%v", got)
	}
}
