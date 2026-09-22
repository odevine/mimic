package main

import (
	"context"
	"fmt"
	"image"
	"sync"
	"sync/atomic"

	"github.com/odevine/mimic/engine/card"
	"github.com/odevine/mimic/engine/template"
)

// activeTemplate is the template the app renders with and the assets it draws
// from. The two are a pair: the template's Go code expects its own layer set, so
// swapping one without the other would render a template against the wrong
// assets. version is "" for a loose directory or placeholders, and the bundle
// version otherwise
type activeTemplate struct {
	name     string
	version  string
	template template.Template
	provider template.AssetProvider
	cleanup  func()
}

// renderPipeline holds the pieces reused across every render: the client that
// fetches art and the active template. They are safe to share across concurrent
// Render calls, so one pipeline serves the whole app. The active template is
// held behind an atomic pointer so a background download or a manual switch can
// swap it in without a lock on the render path
type renderPipeline struct {
	client *card.Client
	active atomic.Pointer[activeTemplate]

	// cleanups accumulates every installed template's cleanup. A swapped-out
	// provider is not closed at swap time, since a concurrent render may still
	// be reading it; all cleanups run once at shutdown instead
	mu       sync.Mutex
	cleanups []func()
}

// install makes at the template for later renders. The previous one is left open
// until shutdown so an in-flight render is never reading a closed backing
func (p *renderPipeline) install(at *activeTemplate) {
	if at.cleanup != nil {
		p.mu.Lock()
		p.cleanups = append(p.cleanups, at.cleanup)
		p.mu.Unlock()
	}
	p.active.Store(at)
}

// close releases every provider the pipeline has installed. Called once at app
// shutdown
func (p *renderPipeline) close() {
	p.mu.Lock()
	defer p.mu.Unlock()
	for _, c := range p.cleanups {
		c()
	}
	p.cleanups = nil
}

// fetchArt downloads the card's art crop. A card with no artwork URL is not an
// error, it returns a nil image the render places nothing for. Art is fetched
// once per selected card so later edits re-render without a network round trip
func (p *renderPipeline) fetchArt(ctx context.Context, d *card.Data) (image.Image, error) {
	if d.ArtworkURL == "" {
		return nil, nil
	}
	return p.client.FetchArt(ctx, d)
}

// render composes the card and art into a finished image through the template.
// Art may be nil, which renders a frame with no artwork
func (p *renderPipeline) render(ctx context.Context, d *card.Data, art image.Image) (image.Image, error) {
	at := p.active.Load()
	buf, err := at.template.Render(ctx, template.RenderRequest{
		Card:   d,
		Art:    art,
		Assets: at.provider,
	})
	if err != nil {
		return nil, fmt.Errorf("rendering %q: %w", d.Name, err)
	}
	return buf.ToImage(8), nil
}
