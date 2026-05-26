package db

import (
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
)

type collMeta struct {
	Count int64 `json:"count"`
	Bytes int64 `json:"bytes"`
}

func (c *Collection) metaFile() string {
	return filepath.Join(c.dir, "meta.json")
}

func (c *Collection) readMeta() (collMeta, error) {
	data, err := os.ReadFile(c.metaFile())
	if err != nil {
		return collMeta{}, err
	}
	var m collMeta
	if err := json.Unmarshal(data, &m); err != nil {
		return collMeta{}, err
	}
	return m, nil
}

func (c *Collection) writeMeta(m collMeta) error {
	b, err := json.Marshal(m)
	if err != nil {
		return err
	}
	return AtomicWriteFile(c.metaFile(), b, 0644)
}

func (c *Collection) rebuildMeta() (collMeta, error) {
	var m collMeta
	err := filepath.WalkDir(c.docsDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if filepath.Ext(path) != ".json" {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return nil
		}
		m.Count++
		m.Bytes += info.Size()
		return nil
	})
	return m, err
}

func (c *Collection) ensureMeta() (collMeta, error) {
	m, err := c.readMeta()
	if err == nil && m.Count >= 0 {
		return m, nil
	}
	m, err = c.rebuildMeta()
	if err != nil {
		return collMeta{}, err
	}
	_ = c.writeMeta(m)
	return m, nil
}
