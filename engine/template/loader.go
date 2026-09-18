package template

import (
	"fmt"
	"image"

	// Layer assets are PNGs
	_ "image/png"
)

// LoadImage opens a provider-relative path and decodes it. It is the one place
// a template turns a manifest path into pixels, so a provider's Open guard
// covers every layer load
func LoadImage(p AssetProvider, relPath string) (image.Image, error) {
	rc, err := p.Open(relPath)
	if err != nil {
		return nil, fmt.Errorf("template: opening %q: %w", relPath, err)
	}
	defer rc.Close()
	img, _, err := image.Decode(rc)
	if err != nil {
		return nil, fmt.Errorf("template: decoding %q: %w", relPath, err)
	}
	return img, nil
}
