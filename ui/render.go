package main

import (
	"context"
	"fmt"
	"image"

	"github.com/odevine/mimic/engine/card"
	"github.com/odevine/mimic/engine/template"
)

// renderPipeline holds the pieces reused across every render: the client that
// fetches art, the template, and the asset provider. They are safe to share
// across concurrent Render calls, so one pipeline serves the whole app
type renderPipeline struct {
	client *card.Client
	tmpl   template.Template
	assets template.AssetProvider
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
	buf, err := p.tmpl.Render(ctx, template.RenderRequest{
		Card:   d,
		Art:    art,
		Assets: p.assets,
	})
	if err != nil {
		return nil, fmt.Errorf("rendering %q: %w", d.Name, err)
	}
	return buf.ToImage(8), nil
}
