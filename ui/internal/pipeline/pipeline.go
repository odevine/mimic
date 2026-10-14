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
	"time"

	"github.com/odevine/impasto/raster"
	"github.com/odevine/mimic/engine/card"
	"github.com/odevine/mimic/engine/fonts"
	"github.com/odevine/mimic/engine/frame"
	"github.com/odevine/mimic/engine/render"
	"github.com/odevine/mimic/engine/template"
	"github.com/odevine/mimic/ui/internal/prefs"
)

// Template is a loaded template the app renders with, and the layer cache over
// its assets. Name is the template's id, such as "mimic.m15". Version is "" for
// the placeholder, LocalVersion for a loose directory, and the bundle version
// otherwise
type Template struct {
	Name     string
	Version  string
	tmpl     *template.Template
	provider template.AssetProvider
	cleanup  func()
	// dir and stamp are set for a loose developer folder: where it is, and what its
	// files looked like when it was loaded, so a change to them is noticed
	dir, stamp string
}

// Fonts is what the template says about its text roles and fonts
func (t *Template) Fonts() *fonts.Catalog { return t.tmpl.Fonts() }

// IsPlaceholder reports whether this is the built-in stand-in the app runs on when
// nothing is installed, which renders no card
func (t *Template) IsPlaceholder() bool { return t.Name == render.PlaceholderID }

// Info describes the template to a person, without drawing with it
func (t *Template) Info() template.Info { return t.tmpl.Info() }

// Serves reports whether the template renders a shape
func (t *Template) Serves(sh template.Shape) bool { return t.tmpl.Serves(sh) }

// Shapes lists the shapes the template renders, sorted
func (t *Template) Shapes() []template.Shape { return t.tmpl.Shapes() }

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
	// features returns the values chosen for a template's features. Nil means
	// none are chosen
	features func(id string) map[string]any
	// standIn lets the stand-in render, for a launch check that tests the app's own
	// plumbing and not a template
	standIn bool
	// checked is when the loose folders were last compared with what was loaded
	checked time.Time
	// fonts returns the pool of fonts a user can assign to a role and what is
	// assigned, which every render uses. Nil means none are assigned. It is caller
	// configuration rather than part of any template, so it is shared by all of
	// them
	fonts func() (*fonts.Pool, map[string]string)
}

// New returns a pipeline that fetches art through client. preferences returns
// the preferred template for each face shape, and may be nil
func New(client *card.Client, preferences func() map[string]prefs.TemplateChoice) *Pipeline {
	return &Pipeline{client: client, preferences: preferences}
}

// SetStandInRenders makes the stand-in template count as one that renders, which
// the launch check needs, since it runs with nothing installed
func (p *Pipeline) SetStandInRenders(on bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.standIn = on
}

func (p *Pipeline) standInRenders() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.standIn
}

// SetFeatures sets where the values chosen for a template's features come from.
// A render reads them when it starts, so a change applies to the next render
func (p *Pipeline) SetFeatures(f func(id string) map[string]any) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.features = f
}

func (p *Pipeline) chosenFeatures(id string) frame.Features {
	p.mu.Lock()
	f := p.features
	p.mu.Unlock()
	if f == nil {
		return nil
	}
	return frame.Features(f(id))
}

// SetFonts sets where the pool of fonts and the role assignments come from. A
// render reads them when it starts, so a change applies to the next render
func (p *Pipeline) SetFonts(f func() (*fonts.Pool, map[string]string)) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.fonts = f
}

// chosenFonts is the pool and the assignments a render starts with
func (p *Pipeline) chosenFonts() (*fonts.Pool, map[string]string) {
	p.mu.Lock()
	f := p.fonts
	p.mu.Unlock()
	if f == nil {
		return nil, nil
	}
	return f()
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
// until shutdown so an in-flight render is never reading a closed backing, but
// its layer cache is emptied since nothing new will use it
func (p *Pipeline) Install(at *Template) {
	if at.cleanup != nil {
		p.mu.Lock()
		p.cleanups = append(p.cleanups, at.cleanup)
		p.mu.Unlock()
	}
	if old := p.active.Swap(at); old != nil && old != at {
		old.dropLayers()
	}
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

// Layout is the active template's layout for a standard single card, or the first
// it has, which is what turns a dpi into the pixel size a render produces
func (p *Pipeline) Layout() *template.Layout {
	return p.active.Load().tmpl.Primary()
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
	buf, err := p.render(ctx, d, face, art, dpi, progress)
	if err != nil {
		return nil, err
	}
	img := buf.ToImage(8)
	if progress != nil {
		progress("", 1)
	}
	return img, nil
}

// RenderYCbCr is Render for an image headed to a JPEG. It converts the finished
// render straight to the planes a JPEG stores, which skips the 8-bit RGB image
// Render builds and the encoder would only convert again
func (p *Pipeline) RenderYCbCr(ctx context.Context, d *card.Data, face int, art image.Image, dpi int, progress func(step string, frac float64)) (*image.YCbCr, error) {
	buf, err := p.render(ctx, d, face, art, dpi, progress)
	if err != nil {
		return nil, err
	}
	img := buf.ToYCbCr()
	if progress != nil {
		progress("", 1)
	}
	return img, nil
}

// render composites the card and reports the encoding step
func (p *Pipeline) render(ctx context.Context, d *card.Data, face int, art image.Image, dpi int, progress func(step string, frac float64)) (*raster.Buffer, error) {
	p.reloadChanged()
	shapes := template.Classify(d)
	if face < 0 || face >= len(shapes) {
		return nil, fmt.Errorf("face %d out of range, %q renders %d", face, d.Name, len(shapes))
	}
	at, err := p.templateFor(shapes[face])
	if err != nil {
		return nil, err
	}
	return p.renderWith(ctx, at, d, face, art, dpi, progress)
}

// renderWith composites the card through at and reports the encoding step
func (p *Pipeline) renderWith(ctx context.Context, at *Template, d *card.Data, face int, art image.Image, dpi int, progress func(step string, frac float64)) (*raster.Buffer, error) {
	pool, overrides := p.chosenFonts()
	req := template.RenderRequest{
		Card:          d,
		Face:          face,
		Art:           art,
		Features:      p.chosenFeatures(at.Name),
		FontPool:      pool,
		FontOverrides: overrides,
		DPI:           dpi,
	}
	if progress != nil {
		// The engine render is the bulk of the work, so it owns the bar up to
		// 0.9 and the encoding below fills the rest
		req.Progress = func(step string, frac float64) { progress(step, frac*0.9) }
	}
	buf, err := render.Render(ctx, at.tmpl, req)
	if err != nil {
		return nil, fmt.Errorf("rendering %q: %w", FaceName(d, face), err)
	}
	if progress != nil {
		progress("Encoding image", 0.92)
	}
	return buf, nil
}
