package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/odevine/mimic/engine/render"
)

// placeholderCache generates each template's placeholder assets once and copies
// them into every directory a test asks for, since encoding the full-size PNGs
// dominates test time under the race detector
type placeholderCache struct {
	root string
	mu   sync.Mutex
	done map[string]bool
}

func (c *placeholderCache) write(dir, name string) error {
	src := filepath.Join(c.root, name)
	c.mu.Lock()
	if !c.done[name] {
		if err := render.WritePlaceholderAssets(src, name); err != nil {
			c.mu.Unlock()
			return err
		}
		c.done[name] = true
	}
	c.mu.Unlock()
	return os.CopyFS(dir, os.DirFS(src))
}

func TestMain(m *testing.M) {
	root, err := os.MkdirTemp("", "mimic-placeholder-cache-")
	if err != nil {
		fmt.Fprintln(os.Stderr, "creating placeholder cache:", err)
		os.Exit(1)
	}
	cache := &placeholderCache{root: root, done: map[string]bool{}}
	writePlaceholders = cache.write
	code := m.Run()
	os.RemoveAll(root)
	os.Exit(code)
}
