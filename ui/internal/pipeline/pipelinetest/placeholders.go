// Package pipelinetest generates each template's placeholder assets once per
// test binary, since encoding the full-size PNGs dominates test time under the
// race detector
package pipelinetest

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/odevine/mimic/engine/render"
)

// placeholderCache generates each template's placeholder assets once and copies
// them into every directory a test asks for
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

// Run runs a package's tests with *write pointed at a shared placeholder cache,
// then exits. Call it from TestMain with the address of the package's
// placeholder writer
func Run(m *testing.M, write *func(dir, name string) error) {
	root, err := os.MkdirTemp("", "mimic-placeholder-cache-")
	if err != nil {
		fmt.Fprintln(os.Stderr, "creating placeholder cache:", err)
		os.Exit(1)
	}
	cache := &placeholderCache{root: root, done: map[string]bool{}}
	*write = cache.write
	code := m.Run()
	os.RemoveAll(root)
	os.Exit(code)
}
