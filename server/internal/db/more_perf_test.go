package db

import (
	"testing"
)

func TestRangeIndexFind(t *testing.T) {
	dir := t.TempDir()
	key := make([]byte, 32)
	for i := range key {
		key[i] = byte(i + 21)
	}
	coll, err := NewCollection("ages", dir, key)
	if err != nil {
		t.Fatal(err)
	}
	defer coll.Close()

	if err := coll.CreateIndex("age"); err != nil {
		t.Fatal(err)
	}
	for _, age := range []float64{10, 18, 25, 40, 65, 80} {
		if _, err := coll.Insert(map[string]interface{}{"age": age}); err != nil {
			t.Fatal(err)
		}
	}

	docs, err := coll.Find(map[string]interface{}{
		"age": map[string]interface{}{"$gte": float64(18), "$lt": float64(65)},
	}, &FindOptions{Limit: -1})
	if err != nil {
		t.Fatal(err)
	}
	if len(docs) != 3 { // 18, 25, 40
		t.Fatalf("got %d docs %#v", len(docs), docs)
	}
}

func TestBinaryDocIndexRoundTrip(t *testing.T) {
	dir := t.TempDir()
	key := make([]byte, 32)
	for i := range key {
		key[i] = byte(i + 31)
	}
	coll, err := NewCollection("bin", dir, key)
	if err != nil {
		t.Fatal(err)
	}
	id, err := coll.Insert(map[string]interface{}{"x": "y"})
	if err != nil {
		t.Fatal(err)
	}
	if err := coll.Close(); err != nil {
		t.Fatal(err)
	}

	coll2, err := NewCollection("bin", dir, key)
	if err != nil {
		t.Fatal(err)
	}
	defer coll2.Close()
	got, err := coll2.FindByID(id)
	if err != nil {
		t.Fatal(err)
	}
	if got["x"] != "y" {
		t.Fatalf("%#v", got)
	}
}

func TestSyncEveryWrite(t *testing.T) {
	dir := t.TempDir()
	key := make([]byte, 32)
	coll, err := NewCollectionWithSync("sync", dir, key, SyncEveryWrite)
	if err != nil {
		t.Fatal(err)
	}
	defer coll.Close()
	if _, err := coll.Insert(map[string]interface{}{"ok": true}); err != nil {
		t.Fatal(err)
	}
	if coll.SyncMode() != SyncEveryWrite {
		t.Fatal(coll.SyncMode())
	}
}

func TestValuesEqualFast(t *testing.T) {
	if !valuesEqual("a", "a") || valuesEqual("a", "b") {
		t.Fatal("string")
	}
	if !valuesEqual(float64(1), float64(1)) || !valuesEqual(float64(1), 1) {
		t.Fatal("num")
	}
	if !valuesEqual(true, true) || valuesEqual(true, false) {
		t.Fatal("bool")
	}
}
