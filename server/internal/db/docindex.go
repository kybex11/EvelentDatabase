package db

import (
	"bytes"
	"encoding/gob"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
)

var docIndexMagic = []byte("EDIX")

const docIndexBinVersion byte = 2

// DocLoc locates an encrypted document payload inside a segment file.
type DocLoc struct {
	Seg    uint32 `json:"s"`
	Offset int64  `json:"o"`
	Length uint32 `json:"l"`
	Dead   bool   `json:"d,omitempty"`
}

// DocIndex is an encrypted on-disk map of document id → segment location.
// On disk it prefers a compact binary (gob) layout; older JSON blobs still load.
type DocIndex struct {
	path   string
	encKey []byte
	locs   map[string]DocLoc
	dirty  bool
}

func newDocIndex(path string, encKey []byte) *DocIndex {
	return &DocIndex{
		path:   path,
		encKey: encKey,
		locs:   make(map[string]DocLoc),
	}
}

func loadDocIndex(path string, encKey []byte) (*DocIndex, error) {
	idx := newDocIndex(path, encKey)
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return idx, nil
		}
		return nil, err
	}
	plain, err := DecodePayload(encKey, raw)
	if err != nil {
		return nil, err
	}
	locs, err := decodeDocIndexBlob(plain)
	if err != nil {
		return nil, err
	}
	idx.locs = locs
	return idx, nil
}

func decodeDocIndexBlob(plain []byte) (map[string]DocLoc, error) {
	if len(plain) >= 5 && bytes.Equal(plain[:4], docIndexMagic) && plain[4] == docIndexBinVersion {
		dec := gob.NewDecoder(bytes.NewReader(plain[5:]))
		var locs map[string]DocLoc
		if err := dec.Decode(&locs); err != nil {
			return nil, fmt.Errorf("binary docindex: %w", err)
		}
		if locs == nil {
			locs = make(map[string]DocLoc)
		}
		return locs, nil
	}
	// Legacy JSON
	var locs map[string]DocLoc
	if err := json.Unmarshal(plain, &locs); err != nil {
		return nil, err
	}
	if locs == nil {
		locs = make(map[string]DocLoc)
	}
	return locs, nil
}

func encodeDocIndexBlob(locs map[string]DocLoc) ([]byte, error) {
	var body bytes.Buffer
	body.Write(docIndexMagic)
	body.WriteByte(docIndexBinVersion)
	if err := gob.NewEncoder(&body).Encode(locs); err != nil {
		return nil, err
	}
	return body.Bytes(), nil
}

func (d *DocIndex) Save() error {
	if d == nil {
		return nil
	}
	if !d.dirty {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(d.path), 0755); err != nil {
		return err
	}
	data, err := encodeDocIndexBlob(d.locs)
	if err != nil {
		return err
	}
	blob, err := EncodePayload(d.encKey, data)
	if err != nil {
		return err
	}
	if err := AtomicWriteFile(d.path, blob, 0644); err != nil {
		return err
	}
	d.dirty = false
	return nil
}

func (d *DocIndex) Get(id string) (DocLoc, bool) {
	loc, ok := d.locs[id]
	if !ok || loc.Dead {
		return DocLoc{}, false
	}
	return loc, true
}

func (d *DocIndex) Put(id string, loc DocLoc) {
	loc.Dead = false
	d.locs[id] = loc
	d.dirty = true
}

func (d *DocIndex) MarkDead(id string) {
	if loc, ok := d.locs[id]; ok {
		loc.Dead = true
		d.locs[id] = loc
		d.dirty = true
	}
}

func (d *DocIndex) Delete(id string) {
	if _, ok := d.locs[id]; ok {
		delete(d.locs, id)
		d.dirty = true
	}
}

func (d *DocIndex) LiveIDs() []string {
	ids := make([]string, 0, len(d.locs))
	for id, loc := range d.locs {
		if !loc.Dead {
			ids = append(ids, id)
		}
	}
	sort.Strings(ids)
	return ids
}

func (d *DocIndex) LiveIDsUnsorted() []string {
	ids := make([]string, 0, len(d.locs))
	for id, loc := range d.locs {
		if !loc.Dead {
			ids = append(ids, id)
		}
	}
	return ids
}

func (d *DocIndex) LiveCount() int64 {
	var n int64
	for _, loc := range d.locs {
		if !loc.Dead {
			n++
		}
	}
	return n
}

func (d *DocIndex) ApproximateBytes() int64 {
	var n int64
	for _, loc := range d.locs {
		if !loc.Dead {
			n += int64(loc.Length) + 4
		}
	}
	return n
}

func (d *DocIndex) Has(id string) bool {
	_, ok := d.Get(id)
	return ok
}

func docIndexPath(collDir string) string {
	return filepath.Join(collDir, ".docindex")
}

func ensureParentDir(path string) error {
	return os.MkdirAll(filepath.Dir(path), 0755)
}

func openAppend(path string) (*os.File, error) {
	if err := ensureParentDir(path); err != nil {
		return nil, err
	}
	return os.OpenFile(path, os.O_CREATE|os.O_RDWR|os.O_APPEND, 0644)
}

func fmtSegErr(n uint32, err error) error {
	return fmt.Errorf("segment %06d: %w", n, err)
}