package db

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSegmentRoundTrip(t *testing.T) {
	dir := t.TempDir()
	key := make([]byte, 32)
	for i := range key {
		key[i] = byte(i + 1)
	}
	coll, err := NewCollection("demo", dir, key)
	if err != nil {
		t.Fatal(err)
	}
	defer coll.Close()

	id, err := coll.Insert(map[string]interface{}{"name": "Ada", "n": float64(1)})
	if err != nil {
		t.Fatal(err)
	}
	got, err := coll.FindByID(id)
	if err != nil {
		t.Fatal(err)
	}
	if got["name"] != "Ada" {
		t.Fatalf("got %#v", got)
	}

	// Segment file should exist; legacy flat file should not.
	if _, err := os.Stat(filepath.Join(dir, "docs", id+".json")); !os.IsNotExist(err) {
		t.Fatalf("unexpected legacy file: %v", err)
	}
	segs, _ := os.ReadDir(filepath.Join(dir, "segments"))
	if len(segs) == 0 {
		t.Fatal("expected segment file")
	}

	if err := coll.Update(id, map[string]interface{}{"name": "Bob", "n": float64(2)}); err != nil {
		t.Fatal(err)
	}
	got, err = coll.FindByID(id)
	if err != nil {
		t.Fatal(err)
	}
	if got["name"] != "Bob" {
		t.Fatalf("after update got %#v", got)
	}

	docs, next, err := coll.FindPage(map[string]interface{}{}, &FindOptions{Limit: 1})
	if err != nil {
		t.Fatal(err)
	}
	if len(docs) != 1 {
		t.Fatalf("len=%d", len(docs))
	}
	_ = next

	if err := coll.Delete(id); err != nil {
		t.Fatal(err)
	}
	if _, err := coll.FindByID(id); err == nil {
		t.Fatal("expected not found")
	}
}

func TestLegacyFileStillReadable(t *testing.T) {
	dir := t.TempDir()
	key := make([]byte, 32)
	for i := range key {
		key[i] = byte(i + 3)
	}
	docsDir := filepath.Join(dir, "docs")
	_ = os.MkdirAll(docsDir, 0755)
	_ = os.MkdirAll(filepath.Join(dir, "indexes"), 0755)
	_ = os.MkdirAll(filepath.Join(dir, "segments"), 0755)

	doc := map[string]interface{}{"_id": "aabbccddeeff0011", "x": float64(9)}
	raw, err := encodeDocument(key, doc)
	if err != nil {
		t.Fatal(err)
	}
	sh := shardSegment("aabbccddeeff0011")
	_ = os.MkdirAll(filepath.Join(docsDir, sh), 0755)
	if err := AtomicWriteFile(filepath.Join(docsDir, sh, "aabbccddeeff0011.json"), raw, 0644); err != nil {
		t.Fatal(err)
	}

	coll, err := NewCollection("legacy", dir, key)
	if err != nil {
		t.Fatal(err)
	}
	defer coll.Close()

	got, err := coll.FindByID("aabbccddeeff0011")
	if err != nil {
		t.Fatal(err)
	}
	if got["x"].(float64) != 9 {
		t.Fatalf("%#v", got)
	}

	count, _, err := coll.Stats()
	if err != nil || count != 1 {
		t.Fatalf("stats count=%d err=%v", count, err)
	}
}
