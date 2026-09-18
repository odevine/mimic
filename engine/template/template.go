// Package template defines what a card template is and the registry that lets
// a new template be added without changing this package. A template is a new
// package that implements Template and calls Register from its init, and the
// only change anywhere else is one blank import in whatever assembles the
// binary
package template

import (
	"context"
	"fmt"
	"image"
	"sort"
	"sync"

	"github.com/odevine/impasto/raster"
	"github.com/odevine/mimic/engine/card"
)

// RenderRequest is everything a template needs to render one card
type RenderRequest struct {
	Card   *card.Data
	Art    image.Image // nil renders without art
	Assets AssetProvider
}

// Template renders a card into a finished pixel buffer. Implementations must
// be safe for concurrent use across independent Render calls. Batch-rendering
// a decklist is the expected usage, so a call constructs its own document and
// touches no shared mutable state
type Template interface {
	Name() string
	Render(ctx context.Context, req RenderRequest) (*raster.Buffer, error)
}

var (
	registryMu sync.RWMutex
	registry   = map[string]func() Template{}
)

// Register records a template factory under name. It is meant to be called
// from an implementation's init(). It panics on a duplicate name or a nil
// factory, since both are programming errors visible at startup
func Register(name string, factory func() Template) {
	registryMu.Lock()
	defer registryMu.Unlock()
	if factory == nil {
		panic("template: Register factory is nil for " + name)
	}
	if _, dup := registry[name]; dup {
		panic("template: Register called twice for " + name)
	}
	registry[name] = factory
}

// Get constructs a fresh template by name, or reports that none is registered
func Get(name string) (Template, error) {
	registryMu.RLock()
	factory, ok := registry[name]
	registryMu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("template: no template registered as %q", name)
	}
	return factory(), nil
}

// Names lists the registered template names, sorted
func Names() []string {
	registryMu.RLock()
	names := make([]string, 0, len(registry))
	for name := range registry {
		names = append(names, name)
	}
	registryMu.RUnlock()
	sort.Strings(names)
	return names
}
