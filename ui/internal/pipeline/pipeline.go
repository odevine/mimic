// Package pipeline pairs templates with their assets and renders cards through
// them. It resolves a template from a loose developer folder, a cached bundle,
// or generated placeholders, and picks the template each face renders through
package pipeline

import (
	"context"
	"fmt"
	"image"
	"sync"
	"sync/atomic"

	"github.com/odevine/mimic/engine/card"
	"github.com/odevine/mimic/engine/template"
	"github.com/odevine/mimic/ui/internal/prefs"
)

// Template is a template the app renders with and the assets it draws from.
// The two are a pair: the template's Go code expects its own layer set, so
// swapping one without the other would render a template against the wrong
// assets. Version is "" for placeholders, LocalVersion for a loose directory,
// and the bundle version otherwise
type Template struct {
	Name     string
	Version  string
	template template.Template
	provider template.AssetProvider
	cleanup  func()
}

// Pipeline holds the pieces reused across every render: the client that
// fetches art, the active template the top-bar picker chose, and any other
// template a face renders through. They are safe to share across concurrent
// Render calls, so one pipeline serves the whole app. The active template is
// held behind an atomic pointer so a background download or a manual switch can
// swap it in without a lock on the render path
type Pipeline struct {
	client *card.Client
	active atomic.Pointer[Template]

	// cleanups accumulates every installed template's cleanup. A swapped-out
	// provider is not closed at swap time, since a concurrent render may still
	// be reading it; all cleanups run once at shutdown instead
	mu       sync.Mutex
	cleanups []func()
	// loaded holds the templates other than the active one that some face
	// renders through, one per name, loaded on first use. Guarded by mu
	loaded map[string]*Template
	// preferences returns the preferred template for each face shape, keyed by
	// ShapeKey. Nil means no preferences
	preferences func() map[string]prefs.TemplateChoice
	// fontDir is the font override directory every render uses, "" for the
	// engine's embedded defaults. It is caller configuration rather than part of
	// any template, so it is shared by all of them
	fontDir string
}

// New returns a pipeline that fetches art through client. preferences returns
// the preferred template for each face shape, and may be nil
func New(client *card.Client, preferences func() map[string]prefs.TemplateChoice) *Pipeline {
	return &Pipeline{client: client, preferences: preferences}
}

// SetFontDir sets the font override directory for later renders. The engine
// reads the folder on each render, so fonts added to it later apply without
// calling this again
func (p *Pipeline) SetFontDir(dir string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.fontDir = dir
}

// FontDir is the font override directory renders use
func (p *Pipeline) FontDir() string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.fontDir
}

// Client is the card client the pipeline fetches art through, shared with
// searches so every Scryfall call is paced together
func (p *Pipeline) Client() *card.Client { return p.client }

// Active is the template the top-bar picker chose
func (p *Pipeline) Active() *Template { return p.active.Load() }

// Close releases the template's assets. A pipeline closes the templates it
// installs itself, so this is for one that was never installed
func (t *Template) Close() {
	if t.cleanup != nil {
		t.cleanup()
	}
}

// Install makes at the template for later renders. The previous one is left open
// until shutdown so an in-flight render is never reading a closed backing
func (p *Pipeline) Install(at *Template) {
	if at.cleanup != nil {
		p.mu.Lock()
		p.cleanups = append(p.cleanups, at.cleanup)
		p.mu.Unlock()
	}
	p.active.Store(at)
}

// Close releases every provider the pipeline has installed. Called once at app
// shutdown
func (p *Pipeline) Close() {
	p.mu.Lock()
	defer p.mu.Unlock()
	for _, c := range p.cleanups {
		c()
	}
	p.cleanups = nil
}

// Manifest reads the active template's manifest, which is what turns a dpi into
// the pixel size a render produces. It re-reads and re-parses on every call, so
// it belongs on a settings request rather than in a hot loop
func (p *Pipeline) Manifest() (*template.Manifest, error) {
	return p.active.Load().provider.Manifest()
}

// FetchArt downloads the card's art crop. A card with no artwork URL is not an
// error, it returns a nil image the render places nothing for. Art is fetched
// once per selected card so later edits re-render without a network round trip
func (p *Pipeline) FetchArt(ctx context.Context, d *card.Data) (image.Image, error) {
	if d.ArtworkURL == "" {
		return nil, nil
	}
	return p.client.FetchArt(ctx, d)
}

// Render composes face of the card and its art into a finished image through
// the template that renders that face, at dpi. Art may be nil, which renders a frame with no artwork. A dpi past what
// the template was authored at renders at the authored size, so a caller can
// pass a saved preference through without checking it first. progress, when set,
// receives step updates: the engine's own steps scaled into the first 90% of the
// bar, then a final downscale step. progress may be nil
func (p *Pipeline) Render(ctx context.Context, d *card.Data, face int, art image.Image, dpi int, progress func(step string, frac float64)) (image.Image, error) {
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
		FontDir: p.FontDir(),
		DPI:     dpi,
	}
	if progress != nil {
		// The engine render is the bulk of the work, so it owns the bar up to
		// 0.9 and the downscale below fills the rest
		req.Progress = func(step string, frac float64) { progress(step, frac*0.9) }
	}
	buf, err := at.template.Render(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("rendering %q: %w", FaceName(d, face), err)
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
