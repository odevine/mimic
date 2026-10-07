package template

import (
	"fmt"
	"image"

	// Layer assets are PNGs
	_ "image/png"
)

// LoadImage opens a provider-relative path and decodes it. It is the one place
// a template turns a manifest path into pixels, so a provider's Open guard
// covers every layer load. A provider that decodes for itself, such as a
// *CachedAssets, is asked to. The image may be shared with other loads, so a
// caller must not write to it
func LoadImage(p AssetProvider, relPath string) (image.Image, error) {
	if l, ok := p.(imageLoader); ok {
		return l.LoadImage(relPath)
	}
	return decodeImage(p, relPath)
}

// imageLoader is a provider that supplies decoded layers itself
type imageLoader interface {
	LoadImage(relPath string) (image.Image, error)
}

// LoadIndexedImage is LoadImage for a layer that is only to be composited. A
// provider that keeps an index of where an image's pixels are visible, as a
// *CachedAssets does, hands back the image with it, as a *blend.Indexed, so
// compositing can skip the transparent stretches. Any other provider gives the
// plain image. The result is shared and must not be written to, and it is not a
// *image.NRGBA even when the image is one, so it belongs in a composite and not in
// code that inspects the pixels
func LoadIndexedImage(p AssetProvider, relPath string) (image.Image, error) {
	if l, ok := p.(indexedLoader); ok {
		return l.LoadIndexed(relPath)
	}
	return LoadImage(p, relPath)
}

// indexedLoader is a provider that supplies decoded layers with their index
type indexedLoader interface {
	LoadIndexed(relPath string) (image.Image, error)
}

func decodeImage(p AssetProvider, relPath string) (image.Image, error) {
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
