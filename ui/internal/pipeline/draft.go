package pipeline

import (
	"context"
	"fmt"
	"image"

	"github.com/odevine/mimic/engine/card"
	"github.com/odevine/mimic/engine/template"
	"github.com/odevine/mimic/ui/internal/catalog"
)

// DraftVersion is the version label of a template made in the editor
const DraftVersion = "draft"

// NewDraftTemplate wraps a template an editor draft opened, so the pipeline can
// render through it. The draft owns the images behind it, so nothing is released
// with the wrapper
func NewDraftTemplate(id string, t *template.Template) *Template {
	return &Template{Name: id, Version: DraftVersion, tmpl: t}
}

// Visibility is what a render leaves out and what it draws that its conditions would
// not, as the engine names items: "layer:frame", "text:title", "art:0" and "symbol:set"
// to hide, and "layer:crown" to show. It is for a preview, and changes nothing in a template
type Visibility struct {
	Hide []string
	Show []string
}

// RenderThrough is Render through a template the caller names, in place of the one
// the app chose for the face. A draft is rendered this way, since it is not one of
// the installed templates
func (p *Pipeline) RenderThrough(ctx context.Context, at *Template, d *card.Data, face int, art image.Image, dpi int, vis Visibility, progress func(step string, frac float64)) (image.Image, error) {
	buf, err := p.renderWith(ctx, at, d, face, art, dpi, vis, progress)
	if err != nil {
		return nil, err
	}
	img := buf.ToImage(8)
	if progress != nil {
		progress("", 1)
	}
	return img, nil
}

// OpenSource opens the files of an installed template version for reading back out,
// as a loose developer folder or as a cached bundle. The header names the bundle's
// version and the engine it needs, and is empty for a loose folder. Call close when
// done
func OpenSource(id, version string) (template.AssetProvider, template.BundleHeader, func(), error) {
	if version == LocalVersion {
		dir := LooseDir(id)
		if dir == "" {
			return nil, template.BundleHeader{}, nil, fmt.Errorf("no local assets for %q", id)
		}
		return template.NewFSAssetProvider(dir), template.BundleHeader{}, func() {}, nil
	}
	path, err := catalog.BundlePath(id, version)
	if err != nil {
		return nil, template.BundleHeader{}, nil, err
	}
	zp, err := template.NewZipAssetProvider(path)
	if err != nil {
		return nil, template.BundleHeader{}, nil, fmt.Errorf("%s %s is not installed: %w", id, version, err)
	}
	return zp, zp.Header(), func() { zp.Close() }, nil
}

// Forget lets go of a template other than the active one that was loaded for a face,
// so the next render opens it again from the files that are there now
func (p *Pipeline) Forget(id string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if at := p.loaded[id]; at != nil {
		delete(p.loaded, id)
		at.dropLayers()
	}
}
