package data

import (
	"fmt"
	"hash/fnv"
	"image"
	"os"
	"strconv"
	"sync"

	"github.com/odevine/mimic/engine/fonts"
	"github.com/odevine/mimic/engine/mana"
	"github.com/odevine/mimic/ui/internal/apierr"
)

// symbolMaxPx bounds the pip size a client can ask for, so a crafted request
// cannot make the app rasterize an enormous disc
const symbolMaxPx = 128

// pipCache holds the pip renderer for the mana font currently in use. It is the
// same renderer the templates draw costs with, so a pip under an input is the pip
// the card will print
type pipCache struct {
	mu  sync.Mutex
	key string
	sym *mana.Symbols
}

// manaKey identifies the mana font a render would use now: the file and its
// modification time for an assigned font, else the template that supplies it
func manaKey(res fonts.Resolution, template string) string {
	if res.Source == fonts.SourceOverride {
		if info, err := os.Stat(res.Path); err == nil {
			return fmt.Sprintf("%s:%d:%d", res.Path, info.Size(), info.ModTime().UnixNano())
		}
	}
	return fmt.Sprintf("%s:%s:%s:%s %s", res.Source, res.Path, template, res.Family, res.Style)
}

// manaFont resolves the mana role the way a render of the active template would
func (s *Service) manaFont() (*fonts.Sizer, string) {
	var cat *fonts.Catalog
	var template string
	if at := s.ws.Pipe.Active(); at != nil {
		cat, template = at.Fonts(), at.Name+"@"+at.Version
	}
	r := fonts.NewResolver(cat, s.ws.FontPool.Get(), s.ws.Prefs.Settings().FontOverrides)
	return r.Font(fonts.Mana), manaKey(r.Inspect(fonts.Mana), template)
}

// get returns the renderer for the mana font in use and the key it was built
// for, rebuilding when the font has changed since the last call
func (c *pipCache) get(sizer *fonts.Sizer, key string) *mana.Symbols {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.sym == nil || c.key != key {
		c.sym, c.key = mana.NewSymbols(sizer), key
	}
	return c.sym
}

// Symbol draws one braced code as a pip px pixels across, returning a not found
// error for a code the engine does not know, which is how the editor flags an
// unknown symbol. The returned tag changes with the mana font's identity, so a
// cache that revalidates on it picks up a newly added font without a reload.
// When known equals the current tag the image is nil, since the caller's copy is
// still good
func (s *Service) Symbol(code string, px int, known string) (image.Image, string, error) {
	if px < 8 {
		px = 32
	}
	px = min(px, symbolMaxPx)
	sizer, key := s.manaFont()
	sym := s.pips.get(sizer, key)
	if sym == nil {
		return nil, "", apierr.New(apierr.NotFound, "no mana font available")
	}
	// Symbol reports the room a code takes at a text size, and a pip's disc is
	// PipDiameter of that size, so this asks for the text size that yields px
	if _, ok := sym.Symbol(code, float64(px)/mana.PipDiameter); !ok {
		return nil, "", apierr.New(apierr.NotFound, "unknown symbol")
	}
	h := fnv.New64a()
	fmt.Fprintf(h, "%s|%s|%d", key, code, px)
	tag := strconv.Quote(strconv.FormatUint(h.Sum64(), 16))
	if known == tag {
		return nil, tag, nil
	}
	img := image.NewRGBA(image.Rect(0, 0, px, px))
	if err := sym.DrawSymbol(img, code, img.Bounds()); err != nil {
		return nil, "", apierr.Wrap(apierr.Internal, "drawing symbol", err)
	}
	return img, tag, nil
}
