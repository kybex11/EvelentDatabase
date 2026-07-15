package db

import (
	"os"
	"path/filepath"
	"strings"
)

func shardSegment(id string) string {
	s := strings.ReplaceAll(strings.ToLower(strings.TrimSpace(id)), "-", "")
	if len(s) < 2 {
		return "_"
	}
	for _, r := range s[:2] {
		ok := (r >= '0' && r <= '9') || (r >= 'a' && r <= 'f')
		if !ok {
			return "_"
		}
	}
	return s[:2]
}

func (c *Collection) resolveDocPath(id string) string {
	sh := shardSegment(id)
	if sh != "" && sh != "_" {
		p := filepath.Join(c.docsDir, sh, id+".json")
		if st, err := os.Stat(p); err == nil && !st.IsDir() {
			return p
		}
	}
	flat := filepath.Join(c.docsDir, id+".json")
	if st, err := os.Stat(flat); err == nil && !st.IsDir() {
		return flat
	}
	// Prefer hex shard path for new legacy-style writes if needed.
	if sh != "" && sh != "_" {
		return filepath.Join(c.docsDir, sh, id+".json")
	}
	return flat
}

func (c *Collection) primaryWritePath(id string) string {
	sh := shardSegment(id)
	if sh == "" || sh == "_" {
		return filepath.Join(c.docsDir, id+".json")
	}
	return filepath.Join(c.docsDir, sh, id+".json")
}

// walkLegacyDocFiles visits every legacy per-document .json under docs/ (flat
// and hex-sharded). Paths under docs/segments are ignored because they live in
// a sibling directory.
func (c *Collection) walkLegacyDocFiles(fn func(path string) error) error {
	entries, err := os.ReadDir(c.docsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() {
			if name == "segments" {
				continue
			}
			sub := filepath.Join(c.docsDir, name)
			subs, err := os.ReadDir(sub)
			if err != nil {
				continue
			}
			for _, se := range subs {
				if se.IsDir() || filepath.Ext(se.Name()) != ".json" {
					continue
				}
				if err := fn(filepath.Join(sub, se.Name())); err != nil {
					return err
				}
			}
			continue
		}
		if filepath.Ext(name) != ".json" {
			continue
		}
		if err := fn(filepath.Join(c.docsDir, name)); err != nil {
			return err
		}
	}
	return nil
}

func legacyIDFromPath(path string) string {
	base := filepath.Base(path)
	return strings.TrimSuffix(base, filepath.Ext(base))
}
