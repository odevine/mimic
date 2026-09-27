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
	// fontDir is the font-override directory resolved at construction time, if
	// any, applied to every render request rather than stored on the template
	// itself, since it is caller configuration rather than the template's own
	fontDir string
	cleanup func()
}

// renderPipeline holds the pieces reused across every render: the client that
// fetches art, the active template the top-bar picker chose, and any other
// template a face renders through. They are safe to share across concurrent
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
	// loaded holds the templates other than the active one that some face
	// renders through, one per name, loaded on first use. Guarded by mu
	loaded map[string]*activeTemplate
	// preferences returns the preferred template for each face shape, keyed by
	// shapeKey. Nil means no preferences
	preferences func() map[string]templateChoice
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

// manifest reads the active template's manifest, which is what turns a dpi into
// the pixel size a render produces. It re-reads and re-parses on every call, so
// it belongs on a settings request rather than in a hot loop
func (p *renderPipeline) manifest() (*template.Manifest, error) {
	return p.active.Load().provider.Manifest()
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

// render composes face of the card and its art into a finished image through
// the template that renders that face, at dpi. Art may be nil, which renders a frame with no artwork. A dpi past what
// the template was authored at renders at the authored size, so a caller can
// pass a saved preference through without checking it first. progress, when set,
// receives step updates: the engine's own steps scaled into the first 90% of the
// bar, then a final downscale step. progress may be nil
func (p *renderPipeline) render(ctx context.Context, d *card.Data, face int, art image.Image, dpi int, progress func(step string, frac float64)) (image.Image, error) {
	shapes := template.Classify(d)
	if face < 0 || face >= len(shapes) {
		return nil, fmt.Errorf("face %d out of range, %q renders %d", face, d.Name, len(shapes))
	}
	at, err := p.templateFor(shapes[face])
	if err != nil {
		return nil, err
	}
	req := template.RenderRequest{
		Card:    d,
		Face:    face,
		Art:     art,
		Assets:  at.provider,
		FontDir: at.fontDir,
		DPI:     dpi,
	}
	if progress != nil {
		// The engine render is the bulk of the work, so it owns the bar up to
		// 0.9 and the downscale below fills the rest
		req.Progress = func(step string, frac float64) { progress(step, frac*0.9) }
	}
	buf, err := at.template.Render(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("rendering %q: %w", faceName(d, face), err)
	}
	if progress != nil {
		progress("Encoding image", 0.92)
	}
	img := buf.ToImage(8)
	if progress != nil {
		progress("", 1)
	}
	return img, nil
}
