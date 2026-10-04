package server

import (
	"fmt"
	"hash/fnv"
	"image"
	"image/png"
	"net/http"
	"os"
	"strconv"
	"sync"

	"github.com/odevine/mimic/engine/fonts"
	"github.com/odevine/mimic/engine/mana"
)

// symbolMaxPx bounds the pip size a client can ask for, so a crafted request
// cannot make the server rasterize an enormous disc
const symbolMaxPx = 128

// pipCache holds the pip renderer for the mana font currently in use. It is
// the same renderer the templates draw costs with, so a pip under an input is
// the pip the card will print
type pipCache struct {
	mu  sync.Mutex
	key string
	sym *mana.Symbols
}

// manaKey identifies the mana font a render would use now: the file and its
// modification time for a user font, else the embedded default's path
func manaKey(dir string) string {
	res := fonts.Inspect(fonts.Mana, dir)
	if res.Source == fonts.SourceUser {
		if info, err := os.Stat(res.Path); err == nil {
			return fmt.Sprintf("%s:%d:%d", res.Path, info.Size(), info.ModTime().UnixNano())
		}
	}
	return string(res.Source) + ":" + res.Path
}

// get returns the renderer for the mana font in dir and the key it was built
// for, rebuilding when the font has changed since the last call
func (c *pipCache) get(dir string) (*mana.Symbols, string) {
	key := manaKey(dir)
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.sym == nil || c.key != key {
		c.sym, c.key = mana.NewSymbols(dir), key
	}
	return c.sym, key
}

// handleSymbol draws one braced code as a PNG pip, answering 404 for a code the
// engine does not know, which is how the editor flags an unknown symbol. The
// ETag carries the mana font's identity, so a browser revalidates and picks
// up a newly added font without a reload
func (s *Server) handleSymbol(w http.ResponseWriter, r *http.Request) {
	code := r.URL.Query().Get("code")
	px, err := strconv.Atoi(r.URL.Query().Get("px"))
	if err != nil || px < 8 {
		px = 32
	}
	px = min(px, symbolMaxPx)

	sym, key := s.pips.get(s.fonts.Path)
	if sym == nil {
		http.Error(w, "no mana font available", http.StatusNotFound)
		return
	}
	// Symbol reports the room a code takes at a text size, and a pip's disc is
	// PipDiameter of that size, so this asks for the text size that yields px
	if _, ok := sym.Symbol(code, float64(px)/mana.PipDiameter); !ok {
		http.Error(w, "unknown symbol", http.StatusNotFound)
		return
	}
	h := fnv.New64a()
	fmt.Fprintf(h, "%s|%s|%d", key, code, px)
	etag := strconv.Quote(strconv.FormatUint(h.Sum64(), 16))
	w.Header().Set("ETag", etag)
	w.Header().Set("Cache-Control", "no-cache")
	if r.Header.Get("If-None-Match") == etag {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	img := image.NewRGBA(image.Rect(0, 0, px, px))
	if err := sym.DrawSymbol(img, code, img.Bounds()); err != nil {
		http.Error(w, "drawing symbol: "+err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "image/png")
	_ = png.Encode(w, img)
}
