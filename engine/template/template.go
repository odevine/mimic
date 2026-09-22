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

// ProgressFunc reports render progress: a human-readable step and a fraction in
// [0,1]. It may be called many times per step, and is called from the render
// goroutine, so an implementation that touches UI state marshals it itself
type ProgressFunc func(step string, frac float64)

// RenderRequest is everything a template needs to render one card
type RenderRequest struct {
	Card   *card.Data
	Art    image.Image // nil renders without art
	Assets AssetProvider
	// Progress, when set, receives step updates during the render. Nil disables
	// reporting, which is the common case for a batch or a headless render
	Progress ProgressFunc
	// FontDir is an optional directory of user-supplied font overrides, one
	// file per role subfolder (title, body, body-italic, mana, type, info). An
	// empty FontDir uses the engine's embedded defaults. It is caller
	// configuration rather than the card's own data, so it travels on the
	// request instead of a template's own struct, which every template gets
	// for free
	FontDir string
	// Copyright is the boilerplate line a card's bottom carries. An empty
	// Copyright builds the printed one from the card's own year
	Copyright string
}

// Report forwards a progress update when a callback is set, so a template's call
// sites stay one line and nil-safe
func (r RenderRequest) Report(step string, frac float64) {
	if r.Progress != nil {
		r.Progress(step, frac)
	}
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
