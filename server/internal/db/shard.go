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
	return filepath.Join(c.docsDir, id+".json")
}

func (c *Collection) primaryWritePath(id string) string {
	sh := shardSegment(id)
	if sh == "" || sh == "_" {
		return filepath.Join(c.docsDir, id+".json")
	}
	return filepath.Join(c.docsDir, sh, id+".json")
}
